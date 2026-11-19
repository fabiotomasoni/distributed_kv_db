package applier

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	sm "github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

func TestT018CancellationBeforeAdmission(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if f, err := a.Submit(ctx, Entry{1, put("k", "cancelled")}); f != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled admission: %v", err)
	}
	if a.Progress().Scheduled != 0 {
		t.Fatal("cancelled request consumed index")
	}
	completed(t, submit(t, a, 1, put("k", "accepted")), sm.OK)
}

func TestT018CancellationWhileWaitingForSpace(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 1})
	first := submit(t, a, 1, put("k", "one"))
	receive(t, g.entered)
	second := submit(t, a, 2, put("k", "two"))
	base, cancel := context.WithCancel(testContext(t))
	defer cancel()
	ctx := &observedContext{Context: base, observed: make(chan struct{})}
	type response struct {
		f   *Future
		err error
	}
	answer := make(chan response, 1)
	go func() {
		f, err := a.Submit(ctx, Entry{3, put("k", "not accepted")})
		answer <- response{f, err}
	}()
	receive(t, ctx.observed)
	cancel()
	got := receive(t, answer)
	if got.f != nil || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("cancel space wait: %+v", got)
	}
	if p := a.Progress(); p.Scheduled != 2 || p.Queued != 1 {
		t.Fatalf("cancelled request entered queue: %+v", p)
	}
	g.open()
	completed(t, first, sm.OK)
	completed(t, second, sm.OK)
}

func TestT018CancellationAfterAdmissionDoesNotRemoveWork(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 1})
	first := submit(t, a, 1, put("k", "one"))
	receive(t, g.entered) // First entry is applying; the second remains queued.
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	second, err := a.Submit(ctx, Entry{2, put("k", "two")})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	for _, f := range []*Future{first, second} {
		if out, err := f.Wait(ctx); !errors.Is(err, context.Canceled) || out.Index != f.index {
			t.Fatalf("cancelled waiter: %#v %v", out, err)
		}
	}
	if p := a.Progress(); p.Queued != 1 || p.LastApplied != 0 || p.Scheduled != 2 {
		t.Fatal(p)
	}
	g.open()
	completed(t, first, sm.OK)
	completed(t, second, sm.OK)
	if out, err := second.Wait(ctx); err != nil || out.Result.Code != sm.OK {
		t.Fatalf("completed result must be retrievable despite cancelled context: %#v %v", out, err)
	}
}

func TestT018ShutdownDrainsUnobservedResults(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{Capacity: 2})
	futures := make([]*Future, 16)
	for i := range futures {
		futures[i] = submit(t, a, uint64(i+1), put("k", fmt.Sprint(i)))
	}
	// No result has a receiver yet. Shutdown must still complete.
	if err := a.Shutdown(testContext(t)); err != nil {
		t.Fatal(err)
	}
	receive(t, a.Done())
	for _, f := range futures {
		select {
		case <-f.done:
		default:
			t.Fatal("Done closed before an accepted result became terminal")
		}
		completed(t, f, sm.OK)
	}
	if p := a.Progress(); p.State != Stopped || p.LastApplied != 16 || p.Active || p.Queued != 0 {
		t.Fatal(p)
	}
	if f, err := a.TrySubmit(Entry{17, put("k", "late")}); f != nil || !errors.Is(err, ErrClosed) {
		t.Fatal("admission remained open")
	}
	if err := a.Shutdown(testContext(t)); err != nil {
		t.Fatal("repeated shutdown:", err)
	}
}

func TestT018ShutdownDuringApplyAndConcurrentWaiters(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 1})
	first := submit(t, a, 1, put("k", "one"))
	receive(t, g.entered)
	second := submit(t, a, 2, put("k", "two"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown wait must cancel, not the worker: %v", err)
	}
	if p := a.Progress(); p.State != Draining || !p.Active || p.Queued != 1 {
		t.Fatal(p)
	}
	if f, err := a.Submit(testContext(t), Entry{3, put("k", "late")}); f != nil || !errors.Is(err, ErrClosed) {
		t.Fatal("admitted work after shutdown began")
	}
	answers := make(chan error, 8)
	var group sync.WaitGroup
	waitCtx := testContext(t)
	for i := 0; i < cap(answers); i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			answers <- a.Shutdown(waitCtx)
		}()
	}
	g.open()
	for i := 0; i < cap(answers); i++ {
		if err := receive(t, answers); err != nil {
			t.Fatal(err)
		}
	}
	group.Wait()
	completed(t, first, sm.OK)
	completed(t, second, sm.OK)
	receive(t, a.Done())
}

func TestT018ShutdownWakesBlockedProducers(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 1})
	submit(t, a, 1, put("k", "one"))
	receive(t, g.entered)
	submit(t, a, 2, put("k", "two"))
	ctx := &observedContext{Context: testContext(t), observed: make(chan struct{})}
	answer := make(chan error, 1)
	go func() {
		f, err := a.Submit(ctx, Entry{3, put("k", "three")})
		if f != nil {
			answer <- errors.New("unexpected admission")
			return
		}
		answer <- err
	}()
	receive(t, ctx.observed)
	stopCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Shutdown(stopCtx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := receive(t, answer); !errors.Is(err, ErrClosed) {
		t.Fatalf("blocked producer not woken by shutdown: %v", err)
	}
	g.open()
	if err := a.Shutdown(testContext(t)); err != nil {
		t.Fatal(err)
	}
}

func TestT018ConcurrentAdmissionAndShutdown(t *testing.T) {
	const producers = 16
	a := newTestApplier(t, sm.New(), Config{Capacity: producers})
	start := make(chan struct{})
	type answer struct {
		f   *Future
		err error
	}
	answers := make(chan answer, producers)
	stopped := make(chan error, 1)
	var group sync.WaitGroup
	for i := 1; i <= producers; i++ {
		group.Add(1)
		go func(index uint64) {
			defer group.Done()
			<-start
			f, err := a.TrySubmit(Entry{index, put("k", fmt.Sprint(index))})
			answers <- answer{f, err}
		}(uint64(i))
	}
	ctx := testContext(t)
	go func() { <-start; stopped <- a.Shutdown(ctx) }()
	close(start)
	for i := 0; i < producers; i++ {
		got := receive(t, answers)
		if got.err == nil {
			completed(t, got.f, sm.OK)
		} else if !errors.Is(got.err, ErrClosed) && !errors.Is(got.err, ErrGap) {
			t.Fatal(got.err)
		}
	}
	group.Wait()
	if err := receive(t, stopped); err != nil {
		t.Fatal(err)
	}
	if p := a.Progress(); p.State != Stopped || p.LastApplied != p.Scheduled || p.Queued != 0 || p.Active {
		t.Fatal(p)
	}
}

func TestT018ApplicationPanicFailsClosed(t *testing.T) {
	g := newGate(t)
	g.panicOn = true
	a := newTestApplier(t, g, Config{Capacity: 2})
	first := submit(t, a, 1, put("k", "effect before panic"))
	receive(t, g.entered)
	second := submit(t, a, 2, put("k", "not applied"))
	third := submit(t, a, 3, put("k", "not applied either"))
	g.open()
	if out, err := first.Wait(testContext(t)); !errors.Is(err, ErrApplyFailed) || out.Index != 1 {
		t.Fatalf("panic outcome: %#v %v", out, err)
	}
	for _, f := range []*Future{second, third} {
		if out, err := f.Wait(testContext(t)); !errors.Is(err, ErrAborted) || out.Index != f.index {
			t.Fatalf("pending failure outcome: %#v %v", out, err)
		}
	}
	if err := a.Shutdown(testContext(t)); !errors.Is(err, ErrApplyFailed) {
		t.Fatal(err)
	}
	if p := a.Progress(); p.State != Failed || p.LastApplied != 0 || p.Scheduled != 3 || p.Queued != 0 || p.Active {
		t.Fatal(p)
	}
	if len(g.calls) != 1 {
		t.Fatal("applied more work after an uncertain effect")
	}
	if f, err := a.TrySubmit(Entry{4, put("k", "late")}); f != nil || !errors.Is(err, ErrClosed) {
		t.Fatal("failed worker accepted work")
	}
}

func TestT018PanicNilIsNotSuccess(t *testing.T) {
	a := newTestApplier(t, applyFunc(func(sm.Command) sm.Result { panic(nil) }), Config{})
	f := submit(t, a, 1, put("k", "v"))
	if _, err := f.Wait(testContext(t)); !errors.Is(err, ErrApplyFailed) {
		t.Fatal(err)
	}
	receive(t, a.Done())
}

func TestT018IdleWorkersStopAndInstancesAreIndependent(t *testing.T) {
	for i := 0; i < 8; i++ {
		a := newTestApplier(t, sm.New(), Config{})
		if err := a.Shutdown(testContext(t)); err != nil {
			t.Fatal(err)
		}
		receive(t, a.Done())
	}
	a := newTestApplier(t, sm.New(), Config{})
	b := newTestApplier(t, sm.New(), Config{})
	fa := submit(t, a, 1, put("same", "a"))
	fb := submit(t, b, 1, put("same", "b"))
	completed(t, fa, sm.OK)
	completed(t, fb, sm.OK)
	for _, pair := range []struct {
		a    *Applier
		want string
	}{{a, "a"}, {b, "b"}} {
		out := completed(t, submit(t, pair.a, 2, sm.Command{Operation: sm.Get, Key: "same"}), sm.OK)
		if string(out.Result.Value) != pair.want {
			t.Fatalf("independent owner shared state: %q", out.Result.Value)
		}
	}
}

func TestT018NilContextsAreRejected(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{})
	if f, err := a.Submit(nil, Entry{1, put("k", "v")}); f != nil || !errors.Is(err, ErrContext) {
		t.Fatal(err)
	}
	if err := a.Shutdown(nil); !errors.Is(err, ErrContext) || a.Progress().State != Running {
		t.Fatal("invalid shutdown changed lifecycle")
	}
	f := submit(t, a, 1, put("k", "v"))
	if _, err := f.Wait(nil); !errors.Is(err, ErrContext) {
		t.Fatal(err)
	}
	completed(t, f, sm.OK)
}
