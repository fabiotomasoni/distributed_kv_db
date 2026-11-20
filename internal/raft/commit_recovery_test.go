package raft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type checkpointFailure struct {
	*memoryStorage
	failure error
}
type armedCheckpointFailure struct {
	*memoryStorage
	armed   atomic.Bool
	failure error
}

func (s *armedCheckpointFailure) Append(raw []byte) (wal.Token, error) {
	tx, err := storageformat.DecodeTransaction(raw)
	if err != nil {
		return wal.Token{}, err
	}
	if tx.Commit != nil && s.armed.Load() {
		return wal.Token{}, s.failure
	}
	return s.memoryStorage.Append(raw)
}
func TestFatalAdmissionClosesBeforeBlockedApplierDrains(t *testing.T) {
	injected := errors.New("second checkpoint failed")
	disk := &armedCheckpointFailure{memoryStorage: &memoryStorage{s: initial()}, failure: injected}
	m := &observedMachine{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	s := newAppService(t, disk, m, 1)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(m.release) }); _ = s.Shutdown(deadline(t)) })
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Handle(deadline(t), appRequest(t, s, 0, 2, []storageformat.LogEntry{logEntry(1, 6), mutation(2, 6, "put", "x", "safe", nil)})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("no active application")
	}
	disk.armed.Store(true)
	if _, err := s.Handle(deadline(t), appRequest(t, s, 2, 3, []storageformat.LogEntry{logEntry(3, 6)})); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	if _, err := s.Handle(deadline(t), appRequest(t, s, 3, 3, nil)); !errors.Is(err, injected) && !errors.Is(err, ErrClosed) {
		t.Fatal("fatal node admitted while waiting for application drain", err)
	}
	release.Do(func() { close(m.release) })
	if err := s.Shutdown(deadline(t)); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	if s.Status().CommitIndex != 2 {
		t.Fatal("uncertain checkpoint became committed")
	}
}

func (s checkpointFailure) Append(raw []byte) (wal.Token, error) {
	tx, err := storageformat.DecodeTransaction(raw)
	if err != nil {
		return wal.Token{}, err
	}
	if tx.Commit != nil {
		return wal.Token{}, s.failure
	}
	return s.memoryStorage.Append(raw)
}
func TestFailedCheckpointNeverAppliesOrAcknowledges(t *testing.T) {
	injected := errors.New("injected checkpoint EIO")
	disk := checkpointFailure{&memoryStorage{s: initial()}, injected}
	m := &observedMachine{machine: statemachine.New()}
	s := newAppService(t, disk, m, 1)
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	reply, err := s.Handle(deadline(t), appRequest(t, s, 0, 2, []storageformat.LogEntry{logEntry(1, 6), mutation(2, 6, "put", "x", "bad", nil)}))
	if err == nil || !errors.Is(err, injected) || len(reply.Bytes()) != 0 {
		t.Fatal("failed checkpoint ack", err)
	}
	if err = s.Shutdown(deadline(t)); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	if s.Status().CommitIndex != 0 || s.Status().LastApplied != 0 || disk.State().LastIndex() != 2 || disk.State().CommitIndex != 0 {
		t.Fatal(s.Status())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.seen) != 0 {
		t.Fatal("applied without durable checkpoint")
	}
}

func TestCommitFrameCutsReplayOnlyFreshCommittedPrefix(t *testing.T) {
	c := configuration(t, "n1")
	id := wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := wal.Bootstrap(root, id)
	if err != nil {
		t.Fatal(err)
	}
	entries := []storageformat.LogEntry{logEntry(1, 5), mutation(2, 5, "cas", "x", "committed", map[string]any{"exists": false}), mutation(3, 5, "put", "secret", "speculative", nil)}
	a := []json.RawMessage{}
	for _, e := range entries {
		a = append(a, e.Raw)
	}
	raw, _ := json.Marshal(map[string]any{"hard_state": map[string]any{"current_term": "5", "voted_for": "n2"}, "replace_suffix": map[string]any{"from_index": "1", "entries": a}})
	if _, err = store.Append(raw); err != nil {
		t.Fatal(err)
	}
	seq := store.State().Sequence + 1
	checkpoint, err := storageformat.EncodeFrame(seq, []byte(`{"commit_index":"2"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "gen-1", "wal")
	prefix, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var previous *statemachine.Machine
	for _, cut := range []int{0, 20, 24, len(checkpoint) - 1, len(checkpoint), len(checkpoint)} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			image := append(bytes.Clone(prefix), checkpoint[:cut]...)
			if err := os.WriteFile(path, image, 0600); err != nil {
				t.Fatal(err)
			}
			disk, err := wal.Open(root, id)
			if err != nil {
				t.Fatal(err)
			}
			defer disk.Close()
			var machine *statemachine.Machine
			s, err := NewService(c, disk, Options{Commit: true, Clock: newClock(), Draw: minimum, MachineFactory: func() applier.Machine { machine = statemachine.New(); return machine }})
			if err != nil {
				t.Fatal(err)
			}
			if machine == previous {
				t.Fatal("replay reused a mutated machine")
			}
			previous = machine
			want := uint64(0)
			if cut == len(checkpoint) {
				want = 2
			}
			if s.Status().CommitIndex != want || s.Status().LastApplied != want || s.Status().ScheduledApplyIndex != want {
				t.Fatal("bad reconstruction", s.Status())
			}
			if want == 2 {
				r, err := s.WaitApplied(deadline(t), LocalAppend{2, 5})
				if err != nil || r.Result.Code != statemachine.OK {
					t.Fatal("fresh CAS replay changed result", r, err)
				}
			}
			if err = s.Shutdown(deadline(t)); err != nil {
				t.Fatal(err)
			}
			// The M02 owner has finished; test-only inspection now cannot race Apply.
			got := machine.Apply(statemachine.Command{Operation: statemachine.Get, Key: "x"})
			if want == 0 && got.Code != statemachine.KeyNotFound || want == 2 && string(got.Value) != "committed" {
				t.Fatal(got)
			}
			if got = machine.Apply(statemachine.Command{Operation: statemachine.Get, Key: "secret"}); got.Code != statemachine.KeyNotFound {
				t.Fatal("speculative WAL suffix applied")
			}
		})
	}
}
