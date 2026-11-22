package raft

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func readService(t *testing.T, d Storage, m applier.Machine) *Service {
	t.Helper()
	s, e := NewService(configuration(t, "n1"), d, Options{Commit: true, Reads: true, Clock: newClock(), Draw: minimum, MachineFactory: func() applier.Machine { return m }})
	if e != nil {
		t.Fatal(e)
	}
	return s
}

type readGate struct {
	machine          *statemachine.Machine
	entered, release chan struct{}
	once             sync.Once
}

func (m *readGate) Apply(c statemachine.Command) statemachine.Result {
	if c.Operation == statemachine.Get {
		m.once.Do(func() { close(m.entered); <-m.release })
	}
	return m.machine.Apply(c)
}
func TestLoggedReadCapturesExactSlotBeforeLaterWrite(t *testing.T) {
	m := &readGate{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	s := readService(t, &memoryStorage{s: initial()}, m)
	electWriteService(t, s)
	if _, e := s.Write(deadline(t), "put", writeCommand()); e != nil {
		t.Fatal(e)
	}
	type answer struct {
		r WriteOutcome
		e error
	}
	read := make(chan answer, 1)
	write := make(chan answer, 1)
	go func() { r, e := s.Read(deadline(t), json.RawMessage(`{"key":"x"}`)); read <- answer{r, e} }()
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("read did not reach worker")
	}
	go func() {
		r, e := s.Write(deadline(t), "put", json.RawMessage(`{"key":"x","value_b64":"eQ==","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":2}`))
		write <- answer{r, e}
	}()
	waitWrite(t, func() bool { return s.Status().CommitIndex >= 4 })
	if s.Status().LastApplied != 2 {
		t.Fatal("scheduled read treated as applied", s.Status())
	}
	select {
	case r := <-read:
		t.Fatal("premature read", r)
	default:
	}
	close(m.release)
	later := <-write
	if later.e != nil || later.r.Outcome != Completed {
		t.Fatal(later)
	}
	r := <-read
	if r.e != nil || r.r.Outcome != Completed || r.r.Applied.Index != 3 || string(r.r.Applied.Result.Value) != "x" {
		t.Fatal("read sampled after later write", r)
	}
	r.r.Applied.Result.Value[0] = 'z'
	retained, e := s.WaitApplied(deadline(t), LocalAppend{3, r.r.Applied.Term})
	if e != nil || string(retained.Result.Value) != "x" {
		t.Fatal("result alias", retained, e)
	}
	now, e := s.Read(deadline(t), json.RawMessage(`{"key":"x"}`))
	if e != nil || string(now.Applied.Result.Value) != "y" {
		t.Fatal(now, e)
	}
	missing, e := s.Read(deadline(t), json.RawMessage(`{"key":"absent"}`))
	if e != nil || missing.Outcome != Completed || missing.Applied.Result.Code != statemachine.KeyNotFound {
		t.Fatal(missing, e)
	}
}
func TestLoggedReadCancellationAndObservedStepdownRemainUnknown(t *testing.T) {
	for _, demote := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "higher_term"}[demote], func(t *testing.T) {
			m := &readGate{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
			s := readService(t, &memoryStorage{s: initial()}, m)
			electWriteService(t, s)
			ctx, cancel := context.WithCancel(deadline(t))
			defer cancel()
			done := make(chan WriteOutcome, 1)
			go func() { r, _ := s.Read(ctx, json.RawMessage(`{"key":"missing"}`)); done <- r }()
			select {
			case <-m.entered:
			case <-deadline(t).Done():
				t.Fatal("no read apply")
			}
			if demote {
				st := s.Status()
				if _, e := s.Handle(deadline(t), heartbeat(t, s.cfg, "n2", st.Term+1, st.LastIndex, st.LastTerm, st.CommitIndex)); e != nil {
					t.Fatal(e)
				}
				close(m.release)
			} else {
				cancel()
			}
			if r := <-done; r.Outcome != Unknown {
				t.Fatal("invalid completion certainty", r)
			}
			if !demote {
				close(m.release)
			}
			waitWrite(t, func() bool { return s.Status().LastApplied == 2 })
		})
	}
}
func TestLoggedReadCapabilityAndIOFailures(t *testing.T) {
	d := &memoryStorage{s: initial()}
	s := newAppService(t, d, statemachine.New(), 1)
	if r, e := s.Read(deadline(t), json.RawMessage(`{"key":"x"}`)); r.Outcome != NotAdmitted || !errors.Is(e, ErrUnsupported) {
		t.Fatal(r, e)
	}
	_ = s.Shutdown(deadline(t))
	if _, e := NewService(configuration(t, "n1"), d, Options{Reads: true}); !errors.Is(e, ErrInput) {
		t.Fatal(e)
	}
	for _, failure := range []error{wal.ErrLimit, wal.ErrFailed} {
		disk := &memoryStorage{s: initial()}
		service := readService(t, disk, statemachine.New())
		electWriteService(t, service)
		disk.failure = failure
		r, e := service.Read(deadline(t), json.RawMessage(`{"key":"x"}`))
		want := Unknown
		if errors.Is(failure, wal.ErrLimit) {
			want = NotAdmitted
		}
		if !errors.Is(e, failure) || r.Outcome != want {
			t.Fatal(r, e)
		}
	}
}

func TestLoggedReadCheckpointFailureNeverApplies(t *testing.T) {
	d := &armedCheckpointFailure{memoryStorage: &memoryStorage{s: initial()}, failure: wal.ErrFailed}
	m := &observedMachine{machine: statemachine.New()}
	s := readService(t, d, m)
	electWriteService(t, s)
	d.armed.Store(true)
	r, e := s.Read(deadline(t), json.RawMessage(`{"key":"x"}`))
	if r.Outcome != Unknown || !errors.Is(e, wal.ErrFailed) {
		t.Fatal(r, e)
	}
	if s.Status().LastApplied != 1 || s.Status().CommitIndex != 1 {
		t.Fatal("failed checkpoint authorized read", s.Status())
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.seen) != 0 {
		t.Fatal("read applied without checkpoint", m.seen)
	}
}
