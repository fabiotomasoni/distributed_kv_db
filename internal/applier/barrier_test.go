package applier

import (
	"context"
	"errors"
	"sync"
	"testing"

	sm "github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

func waitBarrier(t *testing.T, a *Applier) {
	t.Helper()
	for {
		a.mu.Lock()
		pending := a.barrier != nil
		ch := a.changed
		a.mu.Unlock()
		if pending {
			return
		}
		select {
		case <-ch:
		case <-testContext(t).Done():
			t.Fatal("park not admitted")
		}
	}
}
func TestParkDrainsFreezesAndDoesNotConsumeIndex(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 2})
	f1 := submit(t, a, 1, put("x", "a"))
	receive(t, g.entered)
	f2 := submit(t, a, 2, put("x", "b"))
	leases := make(chan *Barrier, 1)
	fail := make(chan error, 1)
	go func() {
		b, e := a.Park(testContext(t))
		if e != nil {
			fail <- e
		} else {
			leases <- b
		}
	}()
	waitBarrier(t, a)
	if _, e := a.TrySubmit(Entry{3, put("x", "c")}); !errors.Is(e, ErrPaused) {
		t.Fatal(e)
	}
	select {
	case <-leases:
		t.Fatal("park preceded accepted Apply")
	default:
	}
	g.open()
	completed(t, f1, sm.OK)
	completed(t, f2, sm.OK)
	var b *Barrier
	select {
	case b = <-leases:
	case e := <-fail:
		t.Fatal(e)
	case <-testContext(t).Done():
		t.Fatal("park wait")
	}
	if b.Index() != 2 || a.Progress().Scheduled != 2 || a.Progress().LastApplied != 2 || a.Progress().Active || a.Progress().Queued != 0 {
		t.Fatal(a.Progress())
	}
	v, e := b.Inspect(testContext(t), func(m Machine) (any, error) { return m.(*gateMachine).inner.CopyState(), nil })
	if e != nil || string(v.(sm.StateImage).Records[0].Value) != "b" {
		t.Fatal(v, e)
	}
	if e = b.Release(testContext(t)); e != nil {
		t.Fatal(e)
	}
	if e = b.Release(testContext(t)); e != nil {
		t.Fatal(e)
	}
	completed(t, submit(t, a, 3, put("x", "c")), sm.OK)
	if _, e = b.Inspect(testContext(t), func(Machine) (any, error) { return nil, nil }); !errors.Is(e, ErrClosed) {
		t.Fatal("expired lease", e)
	}
}
func TestParkCancellationAndPanicWhileDraining(t *testing.T) {
	for _, panicOn := range []bool{false, true} {
		g := newGate(t)
		g.panicOn = panicOn
		a := newTestApplier(t, g, Config{Capacity: 1})
		f := submit(t, a, 1, put("x", "a"))
		receive(t, g.entered)
		ctx, cancel := context.WithCancel(testContext(t))
		defer cancel()
		answer := make(chan error, 1)
		go func() { _, e := a.Park(ctx); answer <- e }()
		waitBarrier(t, a)
		if !panicOn {
			cancel()
			if e := receive(t, answer); !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		}
		g.open()
		_, e := f.Wait(testContext(t))
		if panicOn {
			if !errors.Is(e, ErrApplyFailed) {
				t.Fatal(e)
			}
			if e = receive(t, answer); !errors.Is(e, ErrClosed) {
				t.Fatal(e)
			}
			cancel()
			continue
		}
		// A release request never resumes admission before the worker acknowledges.
		for {
			a.mu.Lock()
			ch := a.changed
			done := a.barrier == nil
			a.mu.Unlock()
			if done {
				break
			}
			select {
			case <-ch:
			case <-testContext(t).Done():
				t.Fatal("canceled park retained ownership")
			}
		}
		completed(t, submit(t, a, 2, put("x", "b")), sm.OK)
	}
}
func TestInspectionCancellationShutdownAndSingleCapacity(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{Capacity: 1})
	b, e := a.Park(testContext(t))
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	ctx, cancel := context.WithCancel(testContext(t))
	out := make(chan error, 1)
	go func() {
		_, e := b.Inspect(ctx, func(Machine) (any, error) { close(entered); <-release; return "copy", nil })
		out <- e
	}()
	receive(t, entered)
	if _, e = a.Park(testContext(t)); !errors.Is(e, ErrPaused) {
		t.Fatal(e)
	}
	if _, e = b.Inspect(testContext(t), func(Machine) (any, error) { t.Fatal("second copier"); return nil, nil }); !errors.Is(e, ErrPaused) {
		t.Fatal(e)
	}
	cancel()
	if e = receive(t, out); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = b.Release(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = a.TrySubmit(Entry{1, put("x", "a")}); !errors.Is(e, ErrPaused) {
		t.Fatal("early release", e)
	}
	shutdown, c := context.WithCancel(testContext(t))
	c()
	if e = a.Shutdown(shutdown); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = b.Inspect(testContext(t), func(Machine) (any, error) { return nil, nil }); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	once.Do(func() { close(release) })
	if e = b.Release(testContext(t)); e != nil {
		t.Fatal(e)
	}
	if e = a.Shutdown(testContext(t)); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Park(testContext(t)); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}
func TestInspectionErrorsAndValidation(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{Capacity: 1})
	if _, e := a.Park(nil); !errors.Is(e, ErrContext) {
		t.Fatal(e)
	}
	ctx, c := context.WithCancel(testContext(t))
	c()
	if _, e := a.Park(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	b, e := a.Park(testContext(t))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.Inspect(nil, func(Machine) (any, error) { return nil, nil }); !errors.Is(e, ErrContext) {
		t.Fatal(e)
	}
	if _, e = b.Inspect(testContext(t), nil); !errors.Is(e, ErrConfig) {
		t.Fatal(e)
	}
	injected := errors.New("copy refused")
	if _, e = b.Inspect(testContext(t), func(Machine) (any, error) { return nil, injected }); !errors.Is(e, injected) {
		t.Fatal(e)
	}
	if _, e = b.Inspect(testContext(t), func(Machine) (any, error) { panic("copy panic") }); !errors.Is(e, ErrApplyFailed) {
		t.Fatal(e)
	}
	if e = a.Shutdown(testContext(t)); !errors.Is(e, ErrApplyFailed) {
		t.Fatal(e)
	}
}
