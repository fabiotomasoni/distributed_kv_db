package raft

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/snapshot"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func compactedView(t *testing.T, id string) wal.State {
	t.Helper()
	c := sessionConfiguration(t, id)
	source := wal.State{HardState: storageformat.HardState{Term: 4, Vote: "n2"}, Sequence: 2, CommitIndex: 3, Entries: []storageformat.LogEntry{logEntry(1, 1), logEntry(2, 1), logEntry(3, 2), logEntry(4, 3), logEntry(5, 3)}}
	image, e := storageformat.EncodeSnapshot(storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash()})
	if e != nil {
		t.Fatal(e)
	}
	candidate, e := snapshot.PrepareCompaction(deadline(t), c, image, source, 3)
	if e != nil || candidate.Index != 3 || candidate.Term != 2 {
		t.Fatal(candidate, e)
	}
	// Decoded view of the approved base-S representation. No file deletion,
	// runtime rotation, production core change or snapshot transfer is performed.
	return wal.State{BaseIndex: 3, BaseTerm: 2, HardState: source.HardState, CommitIndex: 3, Sequence: 3, Entries: source.Entries[3:]}
}
func compactedCore(t *testing.T, id string, s wal.State) *Core {
	t.Helper()
	x, e := New(sessionConfiguration(t, id), s, "gen-1", strings.Repeat("a", 32), minimum, 0)
	if e != nil {
		t.Fatal(e)
	}
	if e = x.EnableReplication(16); e != nil {
		t.Fatal(e)
	}
	return x
}
func TestCompactedBoundarySentinelAndLaggingFollowerNeedsInstall(t *testing.T) {
	x := compactedCore(t, "n1", compactedView(t, "n1"))
	if term, ok := x.term(3); !ok || term != 2 {
		t.Fatal(term, ok)
	}
	if _, ok := x.term(2); ok {
		t.Fatal("invented compacted entry")
	}
	// Select a leader role in this pure request-builder harness only. Election
	// safety remains covered by the existing full core suites.
	x.state.Role = Leader
	x.initProgress()
	p := x.progress["n2"]
	p.NextIndex = 3
	x.progress["n2"] = p
	if req, e := x.appendRequest("n2"); !errors.Is(e, ErrUnsupported) || len(req.Bytes()) != 0 {
		t.Fatal("missing prefix was fabricated", e)
	}
	if x.progress["n2"].MatchIndex != 0 {
		t.Fatal("snapshot existence counted as replication")
	}
	p.NextIndex = 4
	x.progress["n2"] = p
	req, e := x.appendRequest("n2")
	if e != nil {
		t.Fatal(e)
	}
	body, e := strictjson.Decode(req.Bytes(), 1<<20)
	if e != nil || body["prev_log_index"] != "3" || body["prev_log_term"] != "2" {
		t.Fatal(body, e)
	}
	if e = x.Sent(req); e != nil {
		t.Fatal(e)
	}
	shorter := compactedCore(t, "n2", wal.State{HardState: storageformat.HardState{Term: 4}, Sequence: 1, CommitIndex: 2, Entries: []storageformat.LogEntry{logEntry(1, 1), logEntry(2, 1)}})
	out, e := shorter.Request(1, req)
	if e != nil {
		t.Fatal(e)
	}
	out = complete(t, shorter, out)
	if out.Reply == nil {
		t.Fatal("missing rejection")
	}
	b, e := strictjson.Decode(out.Reply.Bytes(), 1<<20)
	if e != nil || b["success"] != false {
		t.Fatal(b, e)
	}
	if _, e = x.Response(1, req, *out.Reply); e != nil {
		t.Fatal(e)
	}
	if x.progress["n2"].MatchIndex != 0 || shorter.state.CommitIndex != 2 || shorter.state.LastIndex != 2 {
		t.Fatal("lagging follower falsely caught up")
	}
}
func TestCompactedFollowerReconcilesOnlyUncommittedSuffix(t *testing.T) {
	for _, mode := range []string{"shorter", "divergent", "committed-conflict"} {
		t.Run(mode, func(t *testing.T) {
			state := compactedView(t, "n2")
			if mode == "shorter" {
				state.Entries = nil
			}
			if mode == "committed-conflict" {
				state.CommitIndex = 4
			}
			x := compactedCore(t, "n2", state)
			wrong := appendRPC(t, x, 4, 3, 1, state.CommitIndex, nil)
			out, e := x.Request(1, wrong)
			if e != nil {
				t.Fatal(e)
			}
			out = complete(t, x, out)
			b, e := strictjson.Decode(out.Reply.Bytes(), 1<<20)
			if e != nil || b["success"] != false || x.state.CommitIndex != state.CommitIndex {
				t.Fatal(b, e)
			}
			request := appendRPC(t, x, 5, 3, 2, state.CommitIndex, []storageformat.LogEntry{logEntry(4, 5), logEntry(5, 5)})
			out, e = x.Request(2, request)
			if mode == "committed-conflict" {
				if e == nil || !bytes.Equal(x.LogEntries()[0].Raw, state.Entries[0].Raw) {
					t.Fatal("known commit replaced", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			out = complete(t, x, out)
			b, e = strictjson.Decode(out.Reply.Bytes(), 1<<20)
			if e != nil || b["success"] != true || x.state.Term != 5 || x.state.Vote != "" || x.state.CommitIndex != 3 || x.state.LastIndex != 5 || x.entries[0].Index != 4 || x.entries[0].Term != 5 {
				t.Fatal(x.state, b, e)
			}
			if term, ok := x.term(3); !ok || term != 2 {
				t.Fatal("boundary sentinel changed")
			}
		})
	}
}
