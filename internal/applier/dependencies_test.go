package applier

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	sm "github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

// These deliberately small test-only boundaries are not a clock service,
// network protocol, WAL or consensus implementation. Nothing touches the OS.
type manualClock struct {
	tick chan struct{}
	once sync.Once
}

func (c *manualClock) advance() { c.once.Do(func() { close(c.tick) }) }

type fakeTransport struct{ delivery chan Entry }

func (n *fakeTransport) deliver(entry Entry) { n.delivery <- entry }

type fakeStorage struct {
	entries []Entry
	fail    bool
}

func (s *fakeStorage) persist(entry Entry) error {
	if s.fail {
		return errors.New("injected storage refusal")
	}
	s.entries = append(s.entries, entry)
	return nil
}

func TestT028FakeBoundariesAndControlledDelay(t *testing.T) {
	clock := &manualClock{tick: make(chan struct{})}
	network := &fakeTransport{delivery: make(chan Entry, 2)}
	storage := &fakeStorage{fail: true}
	machine := sm.New()
	entered := make(chan struct{}, 1)
	apply := applyFunc(func(c sm.Command) sm.Result {
		entered <- struct{}{}
		<-clock.tick
		return machine.Apply(c)
	})
	a := newTestApplier(t, apply, Config{Capacity: 1})
	// Registered after applier cleanup, so a failed test releases the fake first.
	t.Cleanup(clock.advance)
	entry := Entry{1, put("k", "recorded")}
	if err := storage.persist(entry); err == nil {
		t.Fatal("fake refusal was not injected")
	}
	if a.Progress().Scheduled != 0 {
		t.Fatal("a rejected upstream entry reached the applier")
	}
	storage.fail = false
	if err := storage.persist(entry); err != nil {
		t.Fatal(err)
	}
	network.deliver(storage.entries[0])
	f, err := a.Submit(testContext(t), receive(t, network.delivery))
	if err != nil {
		t.Fatal(err)
	}
	receive(t, entered)
	if p := a.Progress(); p.Scheduled != 1 || p.LastApplied != 0 || !p.Active {
		t.Fatal(p)
	}
	// Duplicate delivery is a rejected admission, not a second application.
	network.deliver(storage.entries[0])
	if duplicate, err := a.TrySubmit(receive(t, network.delivery)); duplicate != nil || !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
	clock.advance()
	completed(t, f, sm.OK)
}

// manualDeadline implements deterministic expiration without waiting for a real
// duration; production uses only Context cancellation, never wall-clock time.
type manualDeadline struct{ done chan struct{} }

func (d *manualDeadline) Deadline() (time.Time, bool) { return time.Time{}, false }
func (d *manualDeadline) Done() <-chan struct{}       { return d.done }
func (d *manualDeadline) Value(any) any               { return nil }
func (d *manualDeadline) Err() error {
	select {
	case <-d.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestT018ManuallyExpiredWaitPreservesAcceptedEntry(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{})
	f := submit(t, a, 1, put("k", "eventual"))
	receive(t, g.entered)
	deadline := &manualDeadline{done: make(chan struct{})}
	close(deadline.done)
	if _, err := f.Wait(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	g.open()
	completed(t, f, sm.OK)
}
