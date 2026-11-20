package raft

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func logEntry(index, term uint64) storageformat.LogEntry {
	b, _ := json.Marshal(map[string]any{"index": fmt.Sprint(index), "term": fmt.Sprint(term), "kind": "noop"})
	return storageformat.LogEntry{Index: index, Term: term, Raw: b}
}
func logState(terms []uint64, commit uint64) wal.State {
	s := initial()
	s.HardState.Term = 5
	s.CommitIndex = commit
	for i, term := range terms {
		s.Entries = append(s.Entries, logEntry(uint64(i)+1, term))
	}
	return s
}
func replicated(t *testing.T, id string, terms []uint64, commit uint64) *Core {
	t.Helper()
	c := engine(t, id, logState(terms, commit))
	if err := c.EnableReplication(16); err != nil {
		t.Fatal(err)
	}
	return c
}
func elected(t *testing.T, terms []uint64) *Core {
	t.Helper()
	c := replicated(t, "n1", terms, 0)
	e, err := c.Tick(600)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	req := e.Send[0]
	e, err = c.Response(601, req, answer(t, c.cfg, req, 6, map[string]any{"vote_granted": true}))
	if err != nil {
		t.Fatal(err)
	}
	complete(t, c, e)
	return c
}
func appendRPC(t *testing.T, c *Core, term, prev, pt, commit uint64, entries []storageformat.LogEntry) peerprotocol.Message {
	t.Helper()
	a := []json.RawMessage{}
	for _, e := range entries {
		a = append(a, json.RawMessage(e.Raw))
	}
	return wire(t, c.cfg, peerprotocol.Append, "n1", term, map[string]any{"leader_id": "n1", "prev_log_index": fmt.Sprint(prev), "prev_log_term": fmt.Sprint(pt), "entries": a, "leader_commit": fmt.Sprint(commit)})
}
func offer(t *testing.T, c *Core, to string) peerprotocol.Message {
	t.Helper()
	e, err := c.broadcast()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range e.Send {
		if r.Envelope().To == to {
			if err = c.Sent(r); err != nil {
				t.Fatal(err)
			}
			return r
		}
	}
	t.Fatal("no offer", to)
	return peerprotocol.Message{}
}
func exchange(t *testing.T, leader, follower *Core, now uint64) {
	t.Helper()
	req := offer(t, leader, follower.cfg.NodeID())
	e, err := follower.Request(now, req)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, follower, e)
	if e.Reply == nil {
		t.Fatal("missing replica ack")
	}
	e, err = leader.Response(now, req, *e.Reply)
	if err != nil {
		t.Fatal(err)
	}
	complete(t, leader, e)
}

func TestAppendEntriesReconcilesOnlyFirstConflict(t *testing.T) {
	for _, tc := range []struct {
		name             string
		terms            []uint64
		commit, prev, pt uint64
		incoming         []storageformat.LogEntry
		want             uint64
	}{{"empty", nil, 0, 0, 0, []storageformat.LogEntry{logEntry(1, 1)}, 1}, {"behind", []uint64{1}, 1, 1, 1, []storageformat.LogEntry{logEntry(2, 2), logEntry(3, 3)}, 3}, {"divergent", []uint64{1, 2, 4, 4}, 1, 1, 1, []storageformat.LogEntry{logEntry(2, 3), logEntry(3, 3)}, 3}, {"duplicate", []uint64{1, 2, 3}, 1, 1, 1, []storageformat.LogEntry{logEntry(2, 2)}, 3}, {"short heartbeat", []uint64{1, 2, 3}, 1, 1, 1, nil, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			c := replicated(t, "n2", tc.terms, tc.commit)
			before := c.LogEntries()
			req := appendRPC(t, c, 5, tc.prev, tc.pt, 99, tc.incoming)
			e, err := c.Request(1, req)
			if err != nil {
				t.Fatal(err)
			}
			if e.Persist != nil && e.Reply != nil {
				t.Fatal("ack before Sync")
			}
			e = complete(t, c, e)
			if fields(t, e.Reply)["success"] != true || c.Status().LastIndex != tc.want || c.Status().CommitIndex != tc.commit {
				t.Fatal(c.Status())
			}
			for i := uint64(0); i < tc.commit; i++ {
				if !bytes.Equal(before[i].Raw, c.entries[i].Raw) {
					t.Fatal("committed prefix changed")
				}
			}
			e, err = c.Request(2, req)
			if err != nil || e.Persist != nil || fields(t, e.Reply)["success"] != true {
				t.Fatal("duplicate rewrote suffix", err)
			}
		})
	}
}
func TestReplicationRejectsInconsistentAndCommittedPrefix(t *testing.T) {
	c := replicated(t, "n2", []uint64{1, 2, 2, 4}, 2)
	req := appendRPC(t, c, 6, 3, 3, 0, nil)
	e, err := c.Request(1, req)
	if err != nil || e.Persist == nil {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	f := fields(t, e.Reply)
	if f["success"] != false || f["conflict_index"] != "2" || f["conflict_term"] != "2" {
		t.Fatal(f)
	}
	req = appendRPC(t, c, 6, 8, 6, 0, nil)
	e, err = c.Request(2, req)
	if err != nil || fields(t, e.Reply)["conflict_index"] != "5" {
		t.Fatal(err)
	}
	req = appendRPC(t, c, 1, 0, 0, 0, nil)
	deadline := c.Status().ElectionDeadline
	e, err = c.Request(3, req)
	if err != nil || e.Persist != nil || fields(t, e.Reply)["success"] != false || c.Status().ElectionDeadline != deadline {
		t.Fatal("obsolete term")
	}
	req = appendRPC(t, c, 6, 1, 1, 0, []storageformat.LogEntry{logEntry(2, 3)})
	before := c.LogEntries()
	e, err = c.Request(4, req)
	if !errors.Is(err, ErrFatal) || e.Persist != nil || !bytes.Equal(before[1].Raw, c.entries[1].Raw) {
		t.Fatal("committed conflict overwritten")
	}
	c = replicated(t, "n2", []uint64{1}, 0)
	e1 := logEntry(1, 1)
	e1.Raw = []byte(`{"command":{"key":"k"},"index":"1","kind":"read","term":"1"}`)
	e, err = c.Request(1, appendRPC(t, c, 5, 0, 0, 0, []storageformat.LogEntry{e1}))
	if !errors.Is(err, ErrFatal) || e.Persist != nil {
		t.Fatal("same index/term different command accepted")
	}
}
func TestTrackingRetriesCatchUpAndRejectsObsoleteReplies(t *testing.T) {
	l := elected(t, []uint64{1, 2, 2})
	f := replicated(t, "n2", nil, 0)
	first := offer(t, l, "n2")
	if l.progress["n2"].MatchIndex != 0 {
		t.Fatal("send counted as replication")
	}
	e, err := f.Request(602, first)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, f, e)
	oldReply := *e.Reply
	e, err = l.Response(602, first, oldReply)
	if err != nil {
		t.Fatal(err)
	}
	if l.progress["n2"].NextIndex != 1 {
		t.Fatal(l.progress)
	}
	exchange(t, l, f, 603)
	p := l.progress["n2"]
	if p.MatchIndex != 4 || p.NextIndex != 5 || f.Status().LastIndex != 4 {
		t.Fatal(p)
	}
	e, err = l.Response(604, first, oldReply)
	if err != nil || l.progress["n2"] != p || e.Persist != nil {
		t.Fatal("obsolete failure rolled progress back")
	}
	req := offer(t, l, "n2")
	l.Failed(req)
	newer := offer(t, l, "n2")
	stale := answer(t, l.cfg, req, 6, map[string]any{"success": true, "match_index": "4"})
	_, err = l.Response(605, req, stale)
	if err != nil || l.active["n2"].Envelope().RPCID != newer.Envelope().RPCID {
		t.Fatal("stale request completed new RPC")
	}
	_, err = l.Response(606, req, answer(t, l.cfg, req, 9, map[string]any{"success": true, "match_index": "4"}))
	if err != nil {
		t.Fatal(err)
	}
	if l.Status().Role != Follower || !l.Status().Pending {
		t.Fatal("late higher term ignored")
	}
}
func TestLocalAppendIsDurableOnlyAndQuotaPreservesReserve(t *testing.T) {
	l := elected(t, nil)
	e, err := l.AppendLocal(602, "read", json.RawMessage(`{"key":"k"}`))
	if err != nil || e.Persist == nil || !e.Persist.Proposal || e.Local != nil || l.state.LastIndex != 1 {
		t.Fatal(e, err)
	}
	id := e.Persist.ID
	_, err = l.Durable(id, wal.Token{}, wal.ErrLimit)
	if !errors.Is(err, wal.ErrLimit) || l.state.Failure != nil || l.state.LastIndex != 1 {
		t.Fatal("quota poisoned leader", err)
	}
	e, err = l.AppendLocal(603, "read", json.RawMessage(`{"key":"k"}`))
	if err != nil || e.Persist.ID != id {
		t.Fatal(err)
	}
	e = complete(t, l, e)
	if e.Local == nil || e.Local.Index != 2 || l.progress["n1"].MatchIndex != 2 || l.progress["n2"].MatchIndex != 0 || l.state.CommitIndex != 0 {
		t.Fatal("local write confused with commit")
	}
	copies := l.LogEntries()
	copies[0].Raw[0] = 0
	if l.entries[0].Raw[0] != '{' {
		t.Fatal("log buffer ownership")
	}
	for _, kind := range []string{"unknown", "put"} {
		if e, err = l.AppendLocal(604, kind, nil); err == nil || e.Persist != nil {
			t.Fatal("invalid local entry")
		}
	}
	f := replicated(t, "n2", nil, 0)
	if _, err = f.AppendLocal(0, "noop", nil); !errors.Is(err, ErrNotLeader) {
		t.Fatal(err)
	}
}
func TestReplicationPersistenceFailureNeverAcknowledges(t *testing.T) {
	for _, failure := range []error{errors.New("Sync EIO"), errors.Join(wal.ErrFailed, wal.ErrLimit)} {
		c := replicated(t, "n2", []uint64{1, 4}, 1)
		e, err := c.Request(1, appendRPC(t, c, 6, 1, 1, 0, []storageformat.LogEntry{logEntry(2, 6)}))
		if err != nil || e.Reply != nil || e.Persist == nil {
			t.Fatal(err)
		}
		out, err := c.Durable(e.Persist.ID, wal.Token{}, failure)
		if !errors.Is(err, ErrFatal) || out.Reply != nil || c.state.LastTerm != 4 || c.state.CommitIndex != 1 {
			t.Fatal("failed replica success", err)
		}
	}
}
func TestMaximumWireBatchSplitsIntoBoundedWALFrames(t *testing.T) {
	c := replicated(t, "n2", nil, 0)
	entries := []storageformat.LogEntry{}
	sum := 0
	for i := 1; i <= 12; i++ {
		cmd := map[string]any{"key": "k", "value_b64": "", "client_id": strings.Repeat("a", 32), "sequence": i}
		if i < 12 {
			cmd["value_b64"] = base64.StdEncoding.EncodeToString(make([]byte, 16384))
		} else {
			empty, _ := json.Marshal(cmd)
			cmd["value_b64"] = strings.Repeat("AAAA", (256*1024-sum-len(empty)-32)/4)
		}
		b, _ := json.Marshal(cmd)
		sum += len(b)
		raw, _ := json.Marshal(map[string]any{"index": fmt.Sprint(i), "term": "5", "kind": "put", "command": cmd})
		entries = append(entries, storageformat.LogEntry{Index: uint64(i), Term: 5, Raw: raw})
	}
	total := 0
	for _, e := range entries {
		total += len(e.Raw)
	}
	if sum > 256*1024 || total <= 256*1024 {
		t.Fatal("test did not reach WAL/RPC boundary", sum, total)
	}
	e, err := c.Request(1, appendRPC(t, c, 5, 0, 0, 0, entries))
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	for e.Persist != nil {
		writes++
		tx, err := storageformat.DecodeTransaction(e.Persist.Bytes)
		if err != nil || len(tx.Replace.Entries) > 16 || e.Reply != nil {
			t.Fatal(err)
		}
		e, err = c.Durable(e.Persist.ID, wal.Token{Generation: "gen-1", Sequence: e.Persist.ID}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if writes != 2 || fields(t, e.Reply)["match_index"] != "12" || c.state.CommitIndex != 0 {
		t.Fatal("chunked durable batch", writes)
	}
}

func TestNewLeadershipResetsProgressAndOldGenerationCannotAdvance(t *testing.T) {
	c := elected(t, nil)
	old := offer(t, c, "n2")
	e, err := c.Response(602, old, answer(t, c.cfg, old, 6, map[string]any{"success": true, "match_index": "1"}))
	if err != nil || c.progress["n2"].MatchIndex != 1 {
		t.Fatal(err)
	}
	complete(t, c, e)
	e, err = c.Request(603, voteRequest(t, c.cfg, "n3", 7, 1, 6))
	if err != nil {
		t.Fatal(err)
	}
	complete(t, c, e)
	if c.state.Role != Follower || c.progress != nil {
		t.Fatal("term change retained progress")
	}
	e, err = c.Tick(c.state.ElectionDeadline)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	req := e.Send[0]
	e, err = c.Response(c.now, req, answer(t, c.cfg, req, 8, map[string]any{"vote_granted": true}))
	if err != nil {
		t.Fatal(err)
	}
	complete(t, c, e)
	p := c.progress["n2"]
	if p.MatchIndex != 0 || p.NextIndex != 3 || c.state.CommitIndex != 0 {
		t.Fatal("new generation reused acknowledgments", p)
	}
	e, err = c.Response(c.now, old, answer(t, c.cfg, old, 6, map[string]any{"success": true, "match_index": "1"}))
	if err != nil || e.Persist != nil || c.progress["n2"] != p {
		t.Fatal("old generation advanced tracker")
	}
}

func TestReplicationCapabilityAndBatchLimits(t *testing.T) {
	started := engine(t, "n1", initial())
	_, _ = started.Tick(0)
	if err := started.EnableReplication(1); err == nil {
		t.Fatal("late capability change")
	}
	c := engine(t, "n1", initial())
	if err := c.EnableReplication(17); err == nil {
		t.Fatal("unbounded batch")
	}
	if err := c.EnableReplication(1); err != nil {
		t.Fatal(err)
	}
	if err := c.EnableReplication(1); err == nil {
		t.Fatal("runtime reconfiguration")
	}
	l := elected(t, nil)
	l.batchSize = 1
	for i := 0; i < 3; i++ {
		e, err := l.AppendLocal(602+uint64(i), "noop", nil)
		if err != nil {
			t.Fatal(err)
		}
		complete(t, l, e)
	}
	p := l.progress["n2"]
	p.NextIndex = 1
	l.progress["n2"] = p
	r := offer(t, l, "n2")
	body := fields(t, &r)
	if len(body["entries"].([]any)) != 1 {
		t.Fatal("batch setting ignored")
	}
	if err := l.Sent(r); err == nil {
		t.Fatal("concurrent dispatch allowed")
	}
	l.Failed(r)
	r = offer(t, l, "n2")
	_, err := l.Response(606, r, answer(t, l.cfg, r, 6, map[string]any{"success": false, "conflict_index": "99", "conflict_term": "6"}))
	if err != nil || l.progress["n2"].NextIndex < 1 || l.progress["n2"].NextIndex > l.state.LastIndex+1 {
		t.Fatal("unbounded hint", err)
	}
}
