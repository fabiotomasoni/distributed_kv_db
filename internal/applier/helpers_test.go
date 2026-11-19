package applier

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	sm "github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

type applyFunc func(sm.Command) sm.Result

func (f applyFunc) Apply(c sm.Command) sm.Result { return f(c) }

// gateMachine blocks only its first call. The test always releases it during
// cleanup, including when an assertion fails. Calls are inspected only after Done.
type gateMachine struct {
	inner   *sm.Machine
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   []sm.Command
	panicOn bool
}

func newGate(t *testing.T) *gateMachine {
	g := &gateMachine{inner: sm.New(), entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(g.open)
	return g
}

func (g *gateMachine) open() { g.once.Do(func() { close(g.release) }) }

func (g *gateMachine) Apply(c sm.Command) sm.Result {
	g.calls = append(g.calls, c)
	if len(g.calls) == 1 {
		close(g.entered)
		<-g.release
	}
	result := g.inner.Apply(c)
	if g.panicOn {
		panic("injected application failure")
	}
	return result
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newTestApplier(t *testing.T, m Machine, cfg Config) *Applier {
	t.Helper()
	a, err := New(m, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if g, ok := m.(*gateMachine); ok {
			g.open()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := a.Shutdown(ctx); err != nil && !errors.Is(err, ErrApplyFailed) {
			t.Errorf("cleanup shutdown: %v", err)
		}
	})
	return a
}

func receive[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-testContext(t).Done():
		t.Fatal("timed out waiting for synchronized test event")
		var zero T
		return zero
	}
}

func submit(t *testing.T, a *Applier, index uint64, command sm.Command) *Future {
	t.Helper()
	f, err := a.Submit(testContext(t), Entry{index, command})
	if err != nil {
		t.Fatalf("submit %d: %v", index, err)
	}
	return f
}

func completed(t *testing.T, f *Future, code sm.Code) Outcome {
	t.Helper()
	out, err := f.Wait(testContext(t))
	if err != nil || out.Result.Code != code || out.Index != f.index {
		t.Fatalf("completion: %#v, %v; want index %d code %s", out, err, f.index, code)
	}
	return out
}

func put(key, value string) sm.Command {
	return sm.Command{Operation: sm.Put, Key: key, Value: []byte(value)}
}

// observedContext signals when Submit evaluates the cancellation arm of its
// space wait. This proves backpressure without scheduler guesses or Sleep.
type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}
