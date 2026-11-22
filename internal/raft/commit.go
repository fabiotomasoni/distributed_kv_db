package raft

import (
	"fmt"
	"sort"

	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

// EnableCommit opts into ordered application before events. Its M07 default
// rejects reads; M10 explicitly enables them first. Snapshots remain unsupported.
func (x *Core) EnableCommit() error {
	if x.stepped || x.batchSize == 0 || x.commitEnabled || x.baseIndex != 0 || x.state.Closed {
		return ErrInput
	}
	for _, e := range x.entries {
		v, err := strictjson.Decode(e.Raw, 48*1024)
		if err != nil || (v["kind"] == "read" && !x.readsEnabled) {
			return ErrUnsupported
		}
	}
	x.commitEnabled = true
	x.state.ApplyEpoch = 1
	return nil
}
func (x *Core) persistCommit(index uint64, after func() (Effects, error)) (Effects, error) {
	if !x.commitEnabled || index <= x.state.CommitIndex {
		return after()
	}
	if index > x.state.LastIndex {
		return x.fail(ErrInput)
	}
	return x.persistBatch(map[string]any{"commit_index": fmt.Sprint(index)}, false, func() (Effects, error) { x.state.CommitIndex = index; return after() })
}
func (x *Core) maybeCommit() (Effects, error) {
	if !x.commitEnabled || x.state.Role != Leader {
		return Effects{}, nil
	}
	indexes := make([]uint64, 0, 3)
	for _, m := range x.cfg.Members() {
		indexes = append(indexes, x.progress[m.ID].MatchIndex)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] > indexes[j] })
	index := indexes[1]
	term, exists := x.term(index)
	if index <= x.state.CommitIndex || !exists || term != x.state.Term {
		return Effects{}, nil
	}
	return x.persistCommit(index, func() (Effects, error) { return Effects{}, nil })
}

// Scheduled advances only after successful M02 admission; Applied only after
// its future completes. Both are single-owner events, independent of leadership.
func (x *Core) Scheduled(index uint64) error {
	if !x.commitEnabled || index != x.state.ScheduledApplyIndex+1 || index > x.state.CommitIndex {
		return ErrInput
	}
	x.state.ScheduledApplyIndex = index
	return nil
}
func (x *Core) Applied(epoch, index uint64, err error) error {
	if epoch != x.state.ApplyEpoch {
		return nil
	}
	if err != nil {
		_, failure := x.fail(err)
		return failure
	}
	if !x.commitEnabled || index != x.state.LastApplied+1 || index > x.state.ScheduledApplyIndex || index > x.state.CommitIndex {
		_, failure := x.fail(ErrInput)
		return failure
	}
	x.state.LastApplied = index
	return nil
}

// EnableLoggedReads selects M10 at startup, before EnableCommit/events.
// Old commit-only modes retain their explicit read rejection.
func (x *Core) EnableLoggedReads() error {
	if x.stepped || x.batchSize == 0 || x.commitEnabled || x.readsEnabled || x.state.Closed || x.state.Failure != nil {
		return ErrInput
	}
	x.readsEnabled = true
	return nil
}
