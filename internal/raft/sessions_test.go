package raft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type sessionStorage struct{ *memoryStorage }

func (*sessionStorage) ApplicationVersion() int { return 2 }
func sessionConfiguration(t *testing.T, id string) config.Config {
	t.Helper()
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v2-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration map[string]any }
	_ = json.Unmarshal(raw, &f)
	for _, v := range f.Configuration["members"].([]any) {
		m := v.(map[string]any)
		if m["id"] == id {
			f.Configuration["node_id"] = id
			f.Configuration["client_addr"] = m["client"]
			f.Configuration["peer_addr"] = m["peer"]
		}
	}
	b, _ := json.Marshal(f.Configuration)
	c, e := config.Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func newSessionService(t *testing.T, d Storage, m applier.Machine) *Service {
	t.Helper()
	s, e := NewService(sessionConfiguration(t, "n1"), d, Options{Commit: true, Reads: true, Sessions: true, Clock: newClock(), Draw: minimum, MachineFactory: func() applier.Machine { return m }})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func casAbsent(client string, seq int) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"key": "x", "expected": map[string]any{"exists": false}, "new_value_b64": "Qg==", "client_id": client, "sequence": seq})
	return b
}
func TestSessionConcurrentCASWaitsApplyAndCancelsOnlyWait(t *testing.T) {
	d := &sessionStorage{&memoryStorage{s: initial()}}
	m := &observedMachine{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	s := newSessionService(t, d, m)
	electWriteService(t, s)
	command := casAbsent(strings.Repeat("a", 32), 1)
	type answer struct {
		r WriteOutcome
		e error
	}
	first, second := make(chan answer, 1), make(chan answer, 1)
	ctx, cancel := context.WithCancel(deadline(t))
	go func() { r, e := s.Write(ctx, "cas", command); first <- answer{r, e} }()
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("CAS did not enter worker")
	}
	go func() { r, e := s.Write(deadline(t), "cas", command); second <- answer{r, e} }()
	waitWrite(t, func() bool { return s.Status().CommitIndex >= 3 })
	select {
	case r := <-second:
		t.Fatal("duplicate completed before original apply", r)
	default:
	}
	cancel()
	a := <-first
	if a.r.Outcome != Unknown || !errors.Is(a.e, context.Canceled) {
		t.Fatal(a)
	}
	close(m.release)
	b := <-second
	if b.e != nil || b.r.Outcome != Completed || !b.r.Applied.Result.Applied || b.r.ResultIndex != 2 || b.r.Applied.Index != 3 {
		t.Fatal("duplicate lost original result", b)
	}
	retry, e := s.Write(deadline(t), "cas", command)
	if e != nil || retry.ResultIndex != 2 || !retry.Applied.Result.Applied {
		t.Fatal(retry, e)
	}
	originalTerm := retry.Applied.Term
	// A later term must fence the NEW retry index, not the original cached term.
	st := s.Status()
	if _, e = s.Handle(deadline(t), heartbeat(t, s.cfg, "n2", st.Term+1, st.LastIndex, st.LastTerm, st.CommitIndex)); e != nil {
		t.Fatal(e)
	}
	s.clock.(*manualClock).tick(s.Status().ElectionDeadline)
	waitWrite(t, func() bool { return s.Status().Role == Leader && s.Status().Term > originalTerm })
	retry, e = s.Write(deadline(t), "cas", command)
	if e != nil || retry.ResultIndex != 2 || retry.Applied.Term <= originalTerm || !retry.Applied.Result.Applied {
		t.Fatal("retry fence used old term", retry, e)
	}
	if e = s.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
	m.mu.Lock()
	if len(m.seen) != 1 {
		t.Fatal("CAS reapplied", m.seen)
	}
	m.mu.Unlock()
	// Fresh map/session replay through M02 reconstructs the original response.
	fresh := &observedMachine{machine: statemachine.New()}
	restarted := newSessionService(t, d, fresh)
	electWriteService(t, restarted)
	retry, e = restarted.Write(deadline(t), "cas", command)
	if e != nil || retry.ResultIndex != 2 || !retry.Applied.Result.Applied {
		t.Fatal(retry, e)
	}
	if e = restarted.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
	fresh.mu.Lock()
	defer fresh.mu.Unlock()
	if len(fresh.seen) != 1 {
		t.Fatal("replay or retry duplicated mutation", fresh.seen)
	}
}
func TestSessionSequenceErrorsAndSharedAdmission(t *testing.T) {
	d := &sessionStorage{&memoryStorage{s: initial()}}
	s := newSessionService(t, d, statemachine.New())
	electWriteService(t, s)
	for i := 0; i < 64; i++ {
		s.writeSlots <- struct{}{}
	}
	r, e := s.Write(deadline(t), "cas", casAbsent(strings.Repeat("a", 32), 1))
	if r.Outcome != NotAdmitted || !errors.Is(e, ErrFull) {
		t.Fatal(r, e)
	}
	for i := 0; i < 64; i++ {
		<-s.writeSlots
	}
	command := casAbsent(strings.Repeat("a", 32), 1)
	first, e := s.Write(deadline(t), "cas", command)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	_ = json.Unmarshal(command, &m)
	m["new_value_b64"] = "Qw=="
	changed, _ := json.Marshal(m)
	r, e = s.Write(deadline(t), "cas", changed)
	if e != nil || r.Applied.Result.Code != committed.RequestConflict || r.ResultIndex == first.ResultIndex {
		t.Fatal(r, e)
	}
	m["sequence"] = 3
	gap, _ := json.Marshal(m)
	r, e = s.Write(deadline(t), "cas", gap)
	if e != nil || r.Applied.Result.Code != committed.SequenceGap {
		t.Fatal(r, e)
	}
}
func TestSessionProfileRequiredBeforeReplay(t *testing.T) {
	c := sessionConfiguration(t, "n1")
	d := &memoryStorage{s: initial()}
	if _, e := NewService(c, d, Options{Commit: true, Reads: true}); !errors.Is(e, ErrInput) {
		t.Fatal("v2 configuration without session capability", e)
	}
	if _, e := NewService(c, d, Options{Commit: true, Reads: true, Sessions: true}); !errors.Is(e, ErrInput) {
		t.Fatal("unversioned storage accepted", e)
	}
	disk := &sessionStorage{d}
	if _, e := NewService(c, disk, Options{Commit: true, Reads: true}); !errors.Is(e, ErrInput) {
		t.Fatal("v2 replay without sessions", e)
	}
	if _, e := NewService(configuration(t, "n1"), disk, Options{Commit: true, Reads: true, Sessions: true}); !errors.Is(e, ErrInput) {
		t.Fatal("v1 config accepted", e)
	}
}

func TestSessionShutdownDuringCommittedCASAndFreshRetry(t *testing.T) {
	d := &sessionStorage{&memoryStorage{s: initial()}}
	m := &observedMachine{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	s := newSessionService(t, d, m)
	electWriteService(t, s)
	done := make(chan WriteOutcome, 1)
	command := casAbsent(strings.Repeat("a", 32), 1)
	go func() { r, _ := s.Write(deadline(t), "cas", command); done <- r }()
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("CAS not applying")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if e := s.Shutdown(canceled); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(m.release)
	if r := <-done; r.Outcome != Unknown {
		t.Fatal("shutdown acknowledged client prematurely", r)
	}
	if e := s.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
	restarted := newSessionService(t, d, statemachine.New())
	electWriteService(t, restarted)
	r, e := restarted.Write(deadline(t), "cas", command)
	if e != nil || r.ResultIndex != 2 || !r.Applied.Result.Applied {
		t.Fatal("shutdown lost committed session", r, e)
	}
}
func TestSessionUncommittedSuffixNotRegisteredBeforeLegalCommit(t *testing.T) {
	command := casAbsent(strings.Repeat("a", 32), 1)
	entry := func(i uint64) storageformat.LogEntry {
		b, _ := json.Marshal(map[string]any{"command": command, "index": fmt.Sprint(i), "term": "5", "kind": "cas"})
		return storageformat.LogEntry{Index: i, Term: 5, Raw: b}
	}
	state := wal.State{Sequence: 2, HardState: storageformat.HardState{Term: 5}, CommitIndex: 1, Entries: []storageformat.LogEntry{logEntry(1, 5), entry(2), entry(3)}}
	d := &sessionStorage{&memoryStorage{s: state}}
	m := &observedMachine{machine: statemachine.New()}
	s := newSessionService(t, d, m)
	if s.Status().LastApplied != 1 || s.Status().CommitIndex != 1 {
		t.Fatal("suffix replayed")
	}
	m.mu.Lock()
	if len(m.seen) != 0 {
		t.Fatal("uncommitted CAS applied")
	}
	m.mu.Unlock()
	electWriteService(t, s)
	r, e := s.Write(deadline(t), "cas", command)
	if e != nil || r.ResultIndex != 2 || !r.Applied.Result.Applied {
		t.Fatal(r, e)
	}
	if e = s.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.seen) != 1 {
		t.Fatal("duplicate suffix effects", m.seen)
	}
}

type sessionCheckpointDisk struct{ *armedCheckpointFailure }

func (*sessionCheckpointDisk) ApplicationVersion() int { return 2 }
func TestSessionCASIOFailureAndUncertainRecovery(t *testing.T) {
	for _, checkpoint := range []bool{false, true} {
		t.Run(fmt.Sprint(checkpoint), func(t *testing.T) {
			memory := &memoryStorage{s: initial()}
			ordinary := &sessionStorage{memory}
			fault := &sessionCheckpointDisk{&armedCheckpointFailure{memoryStorage: memory, failure: wal.ErrFailed}}
			var disk Storage = ordinary
			if checkpoint {
				disk = fault
			}
			m := &observedMachine{machine: statemachine.New()}
			s := newSessionService(t, disk, m)
			electWriteService(t, s)
			if checkpoint {
				fault.armed.Store(true)
			} else {
				memory.failure = wal.ErrFailed
			}
			command := casAbsent(strings.Repeat("a", 32), 1)
			r, e := s.Write(deadline(t), "cas", command)
			if !errors.Is(e, wal.ErrFailed) || r.Outcome != Unknown {
				t.Fatal(r, e)
			}
			if e = s.Shutdown(deadline(t)); !errors.Is(e, wal.ErrFailed) {
				t.Fatal(e)
			}
			m.mu.Lock()
			if len(m.seen) != 0 {
				t.Fatal("failed I/O registered mutation", m.seen)
			}
			m.mu.Unlock()
			if s.Status().CommitIndex != 1 || s.Status().LastApplied != 1 {
				t.Fatal("failed checkpoint applied CAS")
			}
			fault.armed.Store(false)
			memory.failure = nil
			fresh := &observedMachine{machine: statemachine.New()}
			restarted := newSessionService(t, disk, fresh)
			fresh.mu.Lock()
			if len(fresh.seen) != 0 {
				t.Fatal("uncommitted recovery registered session")
			}
			fresh.mu.Unlock()
			electWriteService(t, restarted)
			// Drive the manual clock while awaiting retry: an earlier in-flight
			// empty append may require the next heartbeat before sending it.
			type answer struct {
				result WriteOutcome
				err    error
			}
			done := make(chan answer, 1)
			ctx := deadline(t)
			go func() { result, err := restarted.Write(ctx, "cas", command); done <- answer{result, err} }()
			ticks := time.NewTicker(time.Millisecond)
			defer ticks.Stop()
		waiting:
			for {
				select {
				case a := <-done:
					r, e = a.result, a.err
					break waiting
				case <-ticks.C:
					clock := restarted.clock.(*manualClock)
					clock.now.Add(100)
					select {
					case clock.ticks <- time.Time{}:
					default:
					}
				case <-ctx.Done():
					t.Fatal("retry did not finish", restarted.Status())
				}
			}
			want := uint64(3)
			if checkpoint {
				want = 2
			}
			if e != nil || r.ResultIndex != want || !r.Applied.Result.Applied {
				t.Fatal("uncertain retry recovery", r, e, restarted.Status(), memory.State())
			}
			if e = restarted.Shutdown(deadline(t)); e != nil {
				t.Fatal(e)
			}
			fresh.mu.Lock()
			defer fresh.mu.Unlock()
			if len(fresh.seen) != 1 {
				t.Fatal("I/O retry double effect", fresh.seen)
			}
		})
	}
}
