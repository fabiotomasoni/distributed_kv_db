package raft

import (
	"errors"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type gatedSnapshotStorage struct {
	*sessionStorage
	image            []byte
	entered, release chan struct{}
}

func (s *gatedSnapshotStorage) SnapshotImage() []byte {
	close(s.entered)
	<-s.release
	return append([]byte(nil), s.image...)
}
func TestSnapshotServiceRecoveryHandoffHasNoEarlyActivationOrOldWaiters(t *testing.T) {
	c := sessionConfiguration(t, "n1")
	image, e := storageformat.EncodeSnapshot(storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: 2, Term: 1})
	if e != nil {
		t.Fatal(e)
	}
	state := wal.State{BaseIndex: 2, BaseTerm: 1, CommitIndex: 3, Sequence: 1, HardState: storageformat.HardState{Term: 3, Vote: "n2"}, Entries: []storageformat.LogEntry{{Index: 3, Term: 1, Raw: []byte(`{"command":{"key":"x"},"index":"3","kind":"read","term":"1"}`)}}}
	disk := &gatedSnapshotStorage{sessionStorage: &sessionStorage{&memoryStorage{s: state}}, image: image, entered: make(chan struct{}), release: make(chan struct{})}
	type answer struct {
		s *Service
		e error
	}
	done := make(chan answer, 1)
	go func() {
		s, e := NewService(c, disk, Options{Commit: true, Reads: true, Sessions: true, SnapshotRecovery: true, Clock: newClock(), Draw: minimum})
		done <- answer{s, e}
	}()
	select {
	case <-disk.entered:
	case r := <-done:
		t.Fatal("recovery failed before gate", r.e)
	case <-deadline(t).Done():
		t.Fatal("recovery gate timed out")
	}
	select {
	case r := <-done:
		t.Fatal("service escaped incomplete recovery", r.e)
	default:
	}
	close(disk.release)
	var r answer
	select {
	case r = <-done:
	case <-deadline(t).Done():
		t.Fatal("recovery did not complete")
	}
	if r.e != nil {
		t.Fatal(r.e)
	}
	s := r.s
	t.Cleanup(func() { _ = s.Shutdown(deadline(t)) })
	st := s.Status()
	if st.Role != Follower || st.Term != 3 || st.Vote != "n2" || st.LastApplied != 3 || st.ScheduledApplyIndex != 3 || st.CommitIndex != 3 || st.ApplyEpoch != 1 {
		t.Fatal(st)
	}
	if s.started || s.application.collecting {
		t.Fatal("runtime activated during reconstruction")
	}
	if _, e = s.WaitApplied(deadline(t), LocalAppend{Index: 2, Term: 1}); !errors.Is(e, ErrResultExpired) {
		t.Fatal("old waiter restored", e)
	}
	if e = s.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
}
