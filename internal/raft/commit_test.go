package raft

import (
	"errors"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func commitLeader(t *testing.T, terms []uint64) *Core {
	t.Helper()
	c := replicated(t, "n1", terms, 0)
	if err := c.EnableCommit(); err != nil {
		t.Fatal(err)
	}
	e, err := c.Tick(600)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	r := e.Send[0]
	e, err = c.Response(601, r, answer(t, c.cfg, r, 6, map[string]any{"vote_granted": true}))
	if err != nil {
		t.Fatal(err)
	}
	complete(t, c, e)
	return c
}
func TestPreviousTermMajorityWaitsForCurrentTerm(t *testing.T) {
	c := commitLeader(t, []uint64{1, 2})
	f := replicated(t, "n2", nil, 0)
	// Backtrack once, then batch=1 creates a real proof of the old-term prefix
	// without yet acknowledging the new-term noop at index 3.
	exchange(t, c, f, 602)
	c.batchSize = 1
	exchange(t, c, f, 603)
	exchange(t, c, f, 604)
	if c.progress["n2"].MatchIndex != 2 || c.state.CommitIndex != 0 {
		t.Fatal("old term committed by majority", c.Status())
	}
	req := offer(t, c, "n2")
	e, err := f.Request(605, req)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, f, e)
	e, err = c.Response(605, req, *e.Reply)
	if err != nil || e.Persist == nil || c.state.CommitIndex != 0 {
		t.Fatal("commit before checkpoint Sync")
	}
	tx, err := storageformat.DecodeTransaction(e.Persist.Bytes)
	if err != nil || tx.Commit == nil || *tx.Commit != 3 {
		t.Fatal("wrong commit checkpoint")
	}
	complete(t, c, e)
	if c.state.CommitIndex != 3 || c.state.LastApplied != 0 {
		t.Fatal("commit/apply conflated")
	}
	if _, err = c.maybeCommit(); err != nil {
		t.Fatal(err)
	}
}
func TestCommitRequiresQuorumAndCurrentCorrelation(t *testing.T) {
	c := commitLeader(t, nil)
	if e, err := c.maybeCommit(); err != nil || e.Persist != nil || c.state.CommitIndex != 0 {
		t.Fatal("committed with two peers offline")
	}
	old := offer(t, c, "n2")
	c.Failed(old)
	newer := offer(t, c, "n2")
	e, err := c.Response(602, old, answer(t, c.cfg, old, 6, map[string]any{"success": true, "match_index": "1"}))
	if err != nil || e.Persist != nil || c.state.CommitIndex != 0 {
		t.Fatal("obsolete response committed")
	}
	e, err = c.Response(603, newer, answer(t, c.cfg, newer, 6, map[string]any{"success": true, "match_index": "1"}))
	if err != nil || e.Persist == nil {
		t.Fatal(err)
	}
	complete(t, c, e)
	if c.state.CommitIndex != 1 || c.progress["n3"].MatchIndex != 0 {
		t.Fatal("one healthy follower not enough")
	}
	// Demote first, then even a valid old response cannot commit more.
	req := offer(t, c, "n3")
	e, err = c.Response(604, req, answer(t, c.cfg, req, 7, map[string]any{"success": true, "match_index": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	complete(t, c, e)
	if c.state.Role != Follower || c.state.CommitIndex != 1 {
		t.Fatal(c.Status())
	}
}
func TestFollowerCommitLimitedToVerifiedPrefixAndDurable(t *testing.T) {
	c := replicated(t, "n2", []uint64{1, 2, 3}, 0)
	if err := c.EnableCommit(); err != nil {
		t.Fatal(err)
	}
	req := appendRPC(t, c, 5, 1, 1, 99, nil)
	e, err := c.Request(1, req)
	if err != nil || e.Persist == nil || e.Reply != nil || c.state.CommitIndex != 0 {
		t.Fatal(err)
	}
	e = complete(t, c, e)
	if c.state.CommitIndex != 1 || fields(t, e.Reply)["match_index"] != "1" {
		t.Fatal("committed unverified suffix")
	}
	e, err = c.Request(2, req)
	if err != nil || e.Persist != nil {
		t.Fatal("duplicate checkpoint rewrite")
	}
	e, err = c.Request(3, appendRPC(t, c, 4, 3, 3, 3, nil))
	if err != nil || e.Persist != nil || c.state.CommitIndex != 1 {
		t.Fatal("stale leader advanced commit")
	}
	e, err = c.Request(4, appendRPC(t, c, 5, 3, 3, 3, nil))
	if err != nil || e.Persist == nil {
		t.Fatal(err)
	}
	out, err := c.Durable(e.Persist.ID, wal.Token{}, errors.New("checkpoint Sync failed"))
	if !errors.Is(err, ErrFatal) || out.Reply != nil || c.state.CommitIndex != 1 {
		t.Fatal("failed checkpoint applied")
	}
}
func TestCommitAfterSuffixAndApplyCursorInvariants(t *testing.T) {
	c := replicated(t, "n2", []uint64{1}, 0)
	if err := c.EnableCommit(); err != nil {
		t.Fatal(err)
	}
	req := appendRPC(t, c, 6, 1, 1, 2, []storageformat.LogEntry{logEntry(2, 6)})
	e, err := c.Request(1, req)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := storageformat.DecodeTransaction(e.Persist.Bytes)
	if tx.Replace == nil || tx.Commit != nil {
		t.Fatal("commit preceded log")
	}
	e, err = c.Durable(e.Persist.ID, wal.Token{Generation: "gen-1", Sequence: e.Persist.ID}, nil)
	if err != nil || e.Persist == nil || e.Reply != nil || c.state.LastIndex != 2 || c.state.CommitIndex != 0 {
		t.Fatal(err)
	}
	complete(t, c, e)
	if err = c.Scheduled(2); err == nil {
		t.Fatal("apply gap allowed")
	}
	if err = c.Scheduled(1); err != nil {
		t.Fatal(err)
	}
	if err = c.Applied(0, 1, nil); err != nil || c.state.LastApplied != 0 {
		t.Fatal("old epoch changed apply")
	}
	if err = c.Applied(1, 1, nil); err != nil {
		t.Fatal(err)
	}
	if c.state.LastApplied > c.state.CommitIndex {
		t.Fatal("uncommitted apply")
	}
	if err = c.Applied(1, 1, nil); !errors.Is(err, ErrFatal) {
		t.Fatal("double application allowed")
	}
}
