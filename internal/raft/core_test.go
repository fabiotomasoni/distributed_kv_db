package raft

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func configuration(t *testing.T, id string) config.Config {
	t.Helper()
	raw, err := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Configuration map[string]any }
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, v := range f.Configuration["members"].([]any) {
		m := v.(map[string]any)
		if m["id"] == id {
			f.Configuration["node_id"] = id
			f.Configuration["client_addr"] = m["client"]
			f.Configuration["peer_addr"] = m["peer"]
		}
	}
	b, _ := json.Marshal(f.Configuration)
	c, err := config.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func initial() wal.State                      { return wal.State{Sequence: 1} }
func minimum(min, max uint64) (uint64, error) { return min, nil }
func engine(t *testing.T, id string, s wal.State) *Core {
	t.Helper()
	x, err := New(configuration(t, id), s, "gen-1", strings.Repeat("a", 32), minimum, 0)
	if err != nil {
		t.Fatal(err)
	}
	return x
}
func wire(t *testing.T, c config.Config, kind peerprotocol.Kind, from string, term uint64, fields map[string]any) peerprotocol.Message {
	t.Helper()
	fields["protocol_version"] = 1
	fields["cluster_id"] = c.ClusterID()
	fields["config_hash"] = c.Hash()
	fields["from"] = from
	fields["to"] = c.NodeID()
	fields["rpc_id"] = "test"
	fields["term"] = fmt.Sprint(term)
	b, _ := json.Marshal(fields)
	m, err := peerprotocol.DecodeRequest(c, kind, b, c.NodeID())
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func voteRequest(t *testing.T, c config.Config, from string, term, index, lastTerm uint64) peerprotocol.Message {
	return wire(t, c, peerprotocol.Vote, from, term, map[string]any{"candidate_id": from, "last_log_index": fmt.Sprint(index), "last_log_term": fmt.Sprint(lastTerm)})
}
func heartbeat(t *testing.T, c config.Config, from string, term, index, lastTerm, commit uint64) peerprotocol.Message {
	return wire(t, c, peerprotocol.Append, from, term, map[string]any{"leader_id": from, "prev_log_index": fmt.Sprint(index), "prev_log_term": fmt.Sprint(lastTerm), "entries": []any{}, "leader_commit": fmt.Sprint(commit)})
}
func answer(t *testing.T, c config.Config, req peerprotocol.Message, term uint64, fields map[string]any) peerprotocol.Message {
	t.Helper()
	env := req.Envelope()
	fields["protocol_version"] = 1
	fields["cluster_id"] = c.ClusterID()
	fields["config_hash"] = c.Hash()
	fields["from"] = env.To
	fields["to"] = env.From
	fields["rpc_id"] = env.RPCID
	fields["term"] = fmt.Sprint(term)
	b, _ := json.Marshal(fields)
	res, err := peerprotocol.DecodeResponse(c, req, b)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
func complete(t *testing.T, c *Core, e Effects) Effects {
	t.Helper()
	for e.Persist != nil {
		p := e.Persist
		next, err := c.Durable(p.ID, wal.Token{Generation: "gen-1", Sequence: p.ID}, nil)
		if err != nil {
			t.Fatal(err)
		}
		e = next
	}
	return e
}
func fields(t *testing.T, m *peerprotocol.Message) map[string]any {
	t.Helper()
	if m == nil {
		t.Fatal("missing reply")
	}
	var v map[string]any
	if err := json.Unmarshal(m.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestElectionAndNoopWaitForExactDurability(t *testing.T) {
	c := engine(t, "n1", initial())
	if c.Status().Role != Follower {
		t.Fatal(c.Status())
	}
	e, err := c.Tick(600)
	if err != nil || e.Persist == nil || len(e.Send) != 0 || len(c.votes) != 0 {
		t.Fatal(e, err)
	}
	if _, err = c.Tick(601); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	if len(e.Send) != 2 || !c.votes["n1"] || c.Status().Role != Candidate {
		t.Fatal(c.Status())
	}
	req := e.Send[0]
	res := answer(t, c.cfg, req, 1, map[string]any{"vote_granted": true})
	e, err = c.Response(601, req, res)
	if err != nil || e.Persist == nil || c.Status().Role != Candidate || len(e.Send) != 0 {
		t.Fatal(e, err)
	}
	e = complete(t, c, e)
	if c.Status().Role != Leader || c.Status().LastIndex != 1 || c.Status().CommitIndex != 0 || len(e.Send) != 2 {
		t.Fatal(c.Status())
	}
	for _, m := range e.Send {
		var body map[string]any
		_ = json.Unmarshal(m.Bytes(), &body)
		if m.Kind() != peerprotocol.Append || len(body["entries"].([]any)) != 0 {
			t.Fatal("replication escaped M05")
		}
	}
	before := c.Status()
	e, err = c.Response(602, req, res)
	if err != nil || e.Persist != nil || c.Status() != before {
		t.Fatal("duplicate reply affected leader", err)
	}
}

func TestVoteRestrictionAndRestartedBoundary(t *testing.T) {
	local := wal.State{Sequence: 1, HardState: storageformat.HardState{Term: 3}, Entries: []storageformat.LogEntry{{Index: 1, Term: 1}, {Index: 2, Term: 2}, {Index: 3, Term: 2}}}
	for _, tc := range []struct {
		name        string
		index, term uint64
		grant       bool
	}{{"lower term longer", 99, 1, false}, {"same term shorter", 2, 2, false}, {"same boundary", 3, 2, true}, {"higher last term", 1, 3, true}} {
		t.Run(tc.name, func(t *testing.T) {
			c := engine(t, "n1", local)
			e, err := c.Request(1, voteRequest(t, c.cfg, "n2", 3, tc.index, tc.term))
			if err != nil {
				t.Fatal(err)
			}
			if tc.grant && (e.Persist == nil || e.Reply != nil) {
				t.Fatal("vote before sync")
			}
			e = complete(t, c, e)
			if fields(t, e.Reply)["vote_granted"] != tc.grant {
				t.Fatal(tc)
			}
			if tc.grant {
				e, err = c.Request(2, voteRequest(t, c.cfg, "n3", 3, 3, 2))
				if err != nil || fields(t, e.Reply)["vote_granted"] != false {
					t.Fatal("second vote", err)
				}
			}
		})
	}
	boundary := wal.State{Sequence: 1, BaseIndex: 10, BaseTerm: 5, CommitIndex: 10, HardState: storageformat.HardState{Term: 5, Vote: "n2"}}
	c := engine(t, "n1", boundary)
	e, err := c.Request(0, voteRequest(t, c.cfg, "n3", 5, 100, 5))
	if err != nil || fields(t, e.Reply)["vote_granted"] != false {
		t.Fatal("recovered vote ignored", err)
	}
	boundary.HardState.Vote = ""
	c = engine(t, "n1", boundary)
	e, err = c.Request(0, voteRequest(t, c.cfg, "n2", 5, 9, 5))
	if err != nil || fields(t, e.Reply)["vote_granted"] != false {
		t.Fatal("snapshot election boundary ignored")
	}
}

func TestTimerResetAndSameTermStepDown(t *testing.T) {
	c := engine(t, "n1", initial())
	e, _ := c.Tick(600)
	complete(t, c, e)
	deadline := c.Status().ElectionDeadline
	e, err := c.Request(700, voteRequest(t, c.cfg, "n2", 1, 0, 0))
	if err != nil || fields(t, e.Reply)["vote_granted"] != false || c.Status().ElectionDeadline != deadline {
		t.Fatal("denied vote reset timer")
	}
	e, err = c.Request(701, heartbeat(t, c.cfg, "n2", 0, 0, 0, 0))
	if err != nil || fields(t, e.Reply)["success"] != false || c.Status().ElectionDeadline != deadline {
		t.Fatal("stale heartbeat reset")
	}
	e, err = c.Request(702, heartbeat(t, c.cfg, "n2", 1, 1, 1, 0))
	if err != nil || fields(t, e.Reply)["success"] != false || c.Status().Role != Follower || c.Status().Vote != "n1" || c.Status().ElectionDeadline != 1302 {
		t.Fatal(c.Status(), err)
	}
	e, err = c.Request(703, heartbeat(t, c.cfg, "n3", 1, 0, 0, 0))
	if !errors.Is(err, ErrFatal) || e.Reply != nil {
		t.Fatal("two recognized leaders not detected")
	}
	c = engine(t, "n1", initial())
	e, _ = c.Request(100, voteRequest(t, c.cfg, "n2", 1, 0, 0))
	complete(t, c, e)
	e, _ = c.Request(200, voteRequest(t, c.cfg, "n2", 1, 0, 0))
	if e.Persist != nil || fields(t, e.Reply)["vote_granted"] != true || c.Status().ElectionDeadline != 800 {
		t.Fatal("repeat granted vote")
	}
}

func TestLateHigherTermResponseAlwaysStepsDown(t *testing.T) {
	c := engine(t, "n1", initial())
	e, _ := c.Tick(600)
	e = complete(t, c, e)
	old := e.Send[0]
	e, _ = c.Tick(1200)
	complete(t, c, e)
	e, err := c.Response(1201, old, answer(t, c.cfg, old, 1, map[string]any{"vote_granted": true}))
	if err != nil || e.Persist != nil || c.Status().Role != Candidate || len(c.votes) != 1 {
		t.Fatal("obsolete vote counted")
	}
	e, err = c.Response(1202, old, answer(t, c.cfg, old, 9, map[string]any{"vote_granted": true}))
	if err != nil || e.Persist == nil || c.Status().Role != Follower || e.Reply != nil {
		t.Fatal("late higher term ignored")
	}
	complete(t, c, e)
	if c.Status().Term != 9 || c.Status().Vote != "" {
		t.Fatal(c.Status())
	}
}

func TestPersistenceFailureAndTokenMismatchFailClosed(t *testing.T) {
	injected := errors.New("injected sync error")
	for _, tc := range []struct {
		name  string
		token wal.Token
		err   error
	}{{"io", wal.Token{}, injected}, {"generation", wal.Token{Generation: "gen-2", Sequence: 2}, nil}, {"sequence", wal.Token{Generation: "gen-1", Sequence: 3}, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			c := engine(t, "n1", initial())
			e, _ := c.Tick(600)
			out, err := c.Durable(e.Persist.ID, tc.token, tc.err)
			if !errors.Is(err, ErrFatal) || out.Reply != nil || len(out.Send) != 0 || c.Status().Role != Follower {
				t.Fatal(out, err)
			}
			if _, err = c.Tick(2000); !errors.Is(err, ErrFatal) {
				t.Fatal("continued after uncertain storage")
			}
		})
	}
	for _, mode := range []string{"vote", "higher denied", "noop"} {
		t.Run(mode, func(t *testing.T) {
			c := engine(t, "n1", initial())
			var e Effects
			switch mode {
			case "vote":
				e, _ = c.Request(1, voteRequest(t, c.cfg, "n2", 1, 0, 0))
			case "higher denied":
				s := initial()
				s.HardState.Term = 2
				s.Entries = []storageformat.LogEntry{{Index: 1, Term: 2}}
				c = engine(t, "n1", s)
				e, _ = c.Request(1, voteRequest(t, c.cfg, "n2", 3, 0, 0))
			case "noop":
				e, _ = c.Tick(600)
				e = complete(t, c, e)
				r := e.Send[0]
				e, _ = c.Response(601, r, answer(t, c.cfg, r, 1, map[string]any{"vote_granted": true}))
			}
			out, err := c.Durable(e.Persist.ID, wal.Token{}, injected)
			if !errors.Is(err, injected) || out.Reply != nil || len(out.Send) != 0 || c.Status().Role == Leader {
				t.Fatal(err)
			}
		})
	}
}

func TestHeartbeatsNeverReplicateCommitOrTruncate(t *testing.T) {
	s := initial()
	s.HardState.Term = 2
	s.Entries = []storageformat.LogEntry{{Index: 1, Term: 1}, {Index: 2, Term: 2}}
	s.CommitIndex = 1
	c := engine(t, "n1", s)
	e, err := c.Request(10, heartbeat(t, c.cfg, "n2", 2, 1, 1, 2))
	if err != nil || fields(t, e.Reply)["success"] != true || e.Persist != nil || c.Status().LastIndex != 2 || c.Status().CommitIndex != 1 {
		t.Fatal("heartbeat changed log or commit", err)
	}
	req := wire(t, c.cfg, peerprotocol.Append, "n2", 3, map[string]any{"leader_id": "n2", "prev_log_index": "2", "prev_log_term": "2", "entries": []any{map[string]any{"index": "3", "term": "3", "kind": "noop"}}, "leader_commit": "1"})
	e, err = c.Request(11, req)
	if err != nil || e.Persist == nil {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	if !errors.Is(e.Rejected, ErrUnsupported) || e.Reply != nil || c.Status().LastIndex != 2 {
		t.Fatal("implemented M06")
	}
	// A higher-term denied vote must persist the term but must not reset timer.
	deadline := c.Status().ElectionDeadline
	e, err = c.Request(12, voteRequest(t, c.cfg, "n3", 4, 0, 0))
	if err != nil || e.Persist == nil || c.Status().ElectionDeadline != deadline {
		t.Fatal("higher denied vote")
	}
	e = complete(t, c, e)
	if fields(t, e.Reply)["vote_granted"] != false || c.Status().Vote != "" {
		t.Fatal(c.Status())
	}
}

func TestInputClockOverflowAndStop(t *testing.T) {
	c := engine(t, "n1", initial())
	if _, err := c.Tick(1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Tick(0); !errors.Is(err, ErrInput) {
		t.Fatal("backwards time")
	}
	if _, err := c.Request(1, peerprotocol.Message{}); err == nil {
		t.Fatal("zero RPC")
	}
	c.Stop()
	if _, err := c.Tick(2); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	s := initial()
	s.HardState.Term = ^uint64(0)
	c = engine(t, "n1", s)
	if _, err := c.Tick(600); !errors.Is(err, ErrFatal) {
		t.Fatal("term wrapped")
	}
	s = initial()
	s.Sequence = ^uint64(0)
	c = engine(t, "n1", s)
	if _, err := c.Tick(600); !errors.Is(err, ErrFatal) {
		t.Fatal("physical sequence wrapped")
	}
	for _, s := range []wal.State{{}, {Sequence: 1, CommitIndex: 1}, {Sequence: 1, HardState: storageformat.HardState{Vote: "n2"}}, {Sequence: 1, BaseIndex: 1}, {Sequence: 1, HardState: storageformat.HardState{Term: 1}, Entries: []storageformat.LogEntry{{Index: 2, Term: 1}}}} {
		if _, err := New(configuration(t, "n1"), s, "gen-1", strings.Repeat("a", 32), minimum, 0); err == nil {
			t.Fatal("invalid recovered view")
		}
	}
	if _, err := New(configuration(t, "n1"), initial(), "gen-1", strings.Repeat("a", 32), func(uint64, uint64) (uint64, error) { return 1, nil }, 0); err == nil {
		t.Fatal("out of range random timeout")
	}
}

func TestUnsupportedSnapshotObservesTermAndForeignRPCDoesNot(t *testing.T) {
	c := engine(t, "n1", initial())
	raw, _ := os.ReadFile("../../docs/fixtures/peer-v1.json")
	var vectors []struct{ Request string }
	_ = json.Unmarshal(raw, &vectors)
	var body map[string]any
	_ = json.Unmarshal([]byte(vectors[2].Request), &body)
	body["leader_id"] = "n2"
	req := wire(t, c.cfg, peerprotocol.Snapshot, "n2", 5, body)
	e, err := c.Request(1, req)
	if err != nil || e.Persist == nil || e.Rejected != nil {
		t.Fatal("snapshot term was not synchronized")
	}
	e = complete(t, c, e)
	if !errors.Is(e.Rejected, ErrUnsupported) || c.Status().Term != 5 || c.Status().Leader != "n2" || c.Status().LastIndex != 0 {
		t.Fatal(c.Status())
	}
	raw, _ = os.ReadFile("../../docs/fixtures/configuration-v1.json")
	var f struct{ Configuration map[string]any }
	_ = json.Unmarshal(raw, &f)
	f.Configuration["cluster_id"] = strings.Repeat("2", 32)
	raw, _ = json.Marshal(f.Configuration)
	foreign, err := config.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	deadline := c.Status().ElectionDeadline
	e, err = c.Request(2, voteRequest(t, foreign, "n2", 100, 0, 0))
	if err == nil || e.Persist != nil || c.Status().Term != 5 || c.Status().ElectionDeadline != deadline {
		t.Fatal("foreign RPC changed state")
	}
	foreignReq := voteRequest(t, foreign, "n2", 100, 0, 0)
	var out map[string]any
	_ = json.Unmarshal(foreignReq.Bytes(), &out)
	out["from"] = "n1"
	out["to"] = "n2"
	out["candidate_id"] = "n1"
	raw, _ = json.Marshal(out)
	foreignReq, err = peerprotocol.DecodeRequest(foreign, peerprotocol.Vote, raw, "n2")
	if err != nil {
		t.Fatal(err)
	}
	res := answer(t, c.cfg, foreignReq, 100, map[string]any{"vote_granted": true})
	e, err = c.Response(3, foreignReq, res)
	if err == nil || e.Persist != nil || c.Status().Term != 5 {
		t.Fatal("foreign request context changed term")
	}
}

func TestElectionCountersAndRandomFailureCannotWrap(t *testing.T) {
	s := wal.State{Sequence: 1, BaseIndex: ^uint64(0), BaseTerm: 1, CommitIndex: ^uint64(0), HardState: storageformat.HardState{Term: 1}}
	c := engine(t, "n1", s)
	e, _ := c.Tick(600)
	e = complete(t, c, e)
	req := e.Send[0]
	e, err := c.Response(601, req, answer(t, c.cfg, req, 2, map[string]any{"vote_granted": true}))
	if !errors.Is(err, ErrFatal) || e.Persist != nil {
		t.Fatal("noop index wrapped")
	}
	c = engine(t, "n1", initial())
	c.counter = ^uint64(0)
	e, _ = c.Tick(600)
	out, err := c.Durable(e.Persist.ID, wal.Token{Generation: "gen-1", Sequence: e.Persist.ID}, nil)
	if !errors.Is(err, ErrFatal) || len(out.Send) != 0 {
		t.Fatal("RPC counter wrapped")
	}
	c = engine(t, "n1", initial())
	c.draw = func(uint64, uint64) (uint64, error) { return 0, errors.New("entropy unavailable") }
	if _, err = c.Tick(600); !errors.Is(err, ErrFatal) {
		t.Fatal("random failure ignored")
	}
	c = engine(t, "n1", initial())
	e, _ = c.Tick(600)
	if _, err = c.Durable(e.Persist.ID+1, wal.Token{Generation: "gen-1", Sequence: e.Persist.ID}, nil); !errors.Is(err, ErrFatal) {
		t.Fatal("wrong completion identifier accepted")
	}
}
