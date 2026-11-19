package applier

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	sm "github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

func TestT008ApplyBacklogExactlyOnce(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 8})
	futures := []*Future{submit(t, a, 1, put("k", "1"))}
	receive(t, g.entered)
	for i := 2; i <= 5; i++ {
		futures = append(futures, submit(t, a, uint64(i), put("k", fmt.Sprint(i))))
	}
	if p := a.Progress(); p.LastApplied != 0 || p.Scheduled != 5 || p.Queued != 4 || !p.Active {
		t.Fatalf("backlog before completion: %+v", p)
	}
	for i := 6; i <= 7; i++ {
		futures = append(futures, submit(t, a, uint64(i), put("k", fmt.Sprint(i))))
	}
	for _, index := range []uint64{0, 1, 5, 7} {
		f, err := a.TrySubmit(Entry{index, put("k", "wrong")})
		if f != nil || !errors.Is(err, ErrDuplicate) {
			t.Fatalf("duplicate index %d: %v", index, err)
		}
	}
	if p := a.Progress(); p.LastApplied != 0 || p.Scheduled != 7 || p.Queued != 6 {
		t.Fatalf("scheduling advanced incorrectly: %+v", p)
	}
	g.open()
	for _, f := range futures {
		completed(t, f, sm.OK)
	}
	if err := a.Shutdown(testContext(t)); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 7 {
		t.Fatalf("Apply called %d times, want 7", len(g.calls))
	}
	for i, c := range g.calls {
		if string(c.Value) != fmt.Sprint(i+1) {
			t.Fatalf("wrong apply order at %d: %q", i, c.Value)
		}
	}
	if p := a.Progress(); p.LastApplied != 7 || p.Scheduled != 7 || p.State != Stopped {
		t.Fatalf("final progress: %+v", p)
	}
}

func TestT008GapsBaseIndexAndExhaustion(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{InitialIndex: 41})
	if p := a.Progress(); p.LastApplied != 41 || p.Scheduled != 41 {
		t.Fatal(p)
	}
	if f, err := a.TrySubmit(Entry{43, put("k", "gap")}); f != nil || !errors.Is(err, ErrGap) {
		t.Fatalf("gap: %v", err)
	}
	completed(t, submit(t, a, 42, put("k", "ok")), sm.OK)
	if p := a.Progress(); p.Scheduled != 42 {
		t.Fatal(p)
	}
	b := newTestApplier(t, sm.New(), Config{InitialIndex: math.MaxUint64 - 1})
	completed(t, submit(t, b, math.MaxUint64, put("k", "max")), sm.OK)
	if f, err := b.TrySubmit(Entry{0, put("k", "wrap")}); f != nil || !errors.Is(err, ErrIndexExhausted) {
		t.Fatalf("index overflow was not refused: %v", err)
	}
}

func TestT008ConcurrentProducersRetainRejectedWork(t *testing.T) {
	const producers = 16
	m := sm.New()
	a := newTestApplier(t, m, Config{Capacity: producers})
	start := make(chan struct{})
	type admission struct {
		index  uint64
		future *Future
		err    error
	}
	answers := make(chan admission, producers)
	var group sync.WaitGroup
	for i := 1; i <= producers; i++ {
		group.Add(1)
		go func(index uint64) {
			defer group.Done()
			<-start
			f, err := a.TrySubmit(Entry{index, put(fmt.Sprintf("k%d", index), fmt.Sprint(index))})
			answers <- admission{index, f, err}
		}(uint64(i))
	}
	close(start)
	accepted := make(map[uint64]*Future)
	for i := 0; i < producers; i++ {
		answer := receive(t, answers)
		if answer.err == nil {
			accepted[answer.index] = answer.future
		} else if !errors.Is(answer.err, ErrGap) || answer.future != nil {
			t.Fatalf("unexpected concurrent admission: %+v", answer)
		}
	}
	group.Wait()
	// Accepted indices form a prefix. Retry retained gaps in the original order,
	// not in goroutine completion order; nothing is silently discarded.
	for i := 1; i <= producers; i++ {
		index := uint64(i)
		f := accepted[index]
		if f == nil {
			f = submit(t, a, index, put(fmt.Sprintf("k%d", index), fmt.Sprint(index)))
		}
		completed(t, f, sm.OK)
	}
	if err := a.Shutdown(testContext(t)); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= producers; i++ {
		result := m.Apply(sm.Command{Operation: sm.Get, Key: fmt.Sprintf("k%d", i)})
		if result.Code != sm.OK || string(result.Value) != fmt.Sprint(i) {
			t.Fatalf("lost producer %d: %+v", i, result)
		}
	}
}

func TestT008ResultCorrelationAndDomainFailure(t *testing.T) {
	a := newTestApplier(t, sm.New(), Config{Capacity: 8})
	commands := []sm.Command{
		put("k", "one"),
		{Operation: sm.Get, Key: "k"},
		{Operation: sm.CAS, Key: "k", Value: []byte("two"), Expected: &sm.Expectation{Exists: true, Value: []byte("wrong")}},
		{}, // Invalid, but this accepted slot still gets a domain result.
		{Operation: sm.Delete, Key: "k"},
		{Operation: sm.Get, Key: "k"},
	}
	codes := []sm.Code{sm.OK, sm.OK, sm.CompareFailed, sm.InvalidArgument, sm.OK, sm.KeyNotFound}
	futures := make([]*Future, len(commands))
	for i, command := range commands {
		futures[i] = submit(t, a, uint64(i+1), command)
	}
	for i := len(futures) - 1; i >= 0; i-- {
		out := completed(t, futures[i], codes[i])
		if out.Index != uint64(i+1) {
			t.Fatal("result associated with another entry")
		}
		if i == 1 && string(out.Result.Value) != "one" {
			t.Fatal("read was not captured in its own apply slot")
		}
		if i == 4 && !out.Result.Deleted {
			t.Fatal("DELETE result missing")
		}
	}
	if p := a.Progress(); p.LastApplied != 6 {
		t.Fatalf("domain failure stopped progress: %+v", p)
	}
}

func TestT008QueueFullAndBlockingBackpressure(t *testing.T) {
	g := newGate(t)
	a := newTestApplier(t, g, Config{Capacity: 1})
	first := submit(t, a, 1, put("k", "one"))
	receive(t, g.entered)
	second := submit(t, a, 2, put("k", "two"))
	if f, err := a.TrySubmit(Entry{3, put("k", "three")}); f != nil || !errors.Is(err, ErrFull) {
		t.Fatalf("full queue: %v", err)
	}
	if p := a.Progress(); p.Scheduled != 2 || p.Queued != 1 {
		t.Fatalf("full refusal advanced scheduling: %+v", p)
	}
	ctx := &observedContext{Context: testContext(t), observed: make(chan struct{})}
	type response struct {
		f   *Future
		err error
	}
	answer := make(chan response, 1)
	go func() {
		f, err := a.Submit(ctx, Entry{3, put("k", "three")})
		answer <- response{f, err}
	}()
	receive(t, ctx.observed)
	select {
	case <-answer:
		t.Fatal("blocking Submit returned while capacity was unavailable")
	default:
	}
	g.open()
	third := receive(t, answer)
	if third.err != nil {
		t.Fatal(third.err)
	}
	for _, f := range []*Future{first, second, third.f} {
		completed(t, f, sm.OK)
	}
}

func TestT008CommandAndResultOwnership(t *testing.T) {
	g := newGate(t)
	g.inner.Apply(put("k", "old"))
	a := newTestApplier(t, g, Config{Capacity: 4})
	first := submit(t, a, 1, sm.Command{Operation: sm.Get, Key: "k"})
	receive(t, g.entered)
	expected, replacement := []byte("old"), []byte("new")
	condition := &sm.Expectation{Exists: true, Value: expected}
	second := submit(t, a, 2, sm.Command{Operation: sm.CAS, Key: "k", Value: replacement, Expected: condition})
	// Submit returned: mutation is legal now and must not affect queued work.
	expected[0], replacement[0], condition.Exists = 'X', 'X', false
	third := submit(t, a, 3, sm.Command{Operation: sm.Get, Key: "k"})
	g.open()
	completed(t, first, sm.OK)
	completed(t, second, sm.OK)
	if out := completed(t, third, sm.OK); string(out.Result.Value) != "new" {
		t.Fatalf("queued command retained caller buffers: %#v", out)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			out, err := third.Wait(context.Background())
			if err != nil || string(out.Result.Value) != "new" {
				t.Errorf("future result alias: %#v %v", out, err)
				return
			}
			out.Result.Value[0] = 'X'
		}()
	}
	group.Wait()
	if out := completed(t, third, sm.OK); string(out.Result.Value) != "new" {
		t.Fatal("result changed through another waiter")
	}
}

func TestT008AdmissionBoundsAndConfiguration(t *testing.T) {
	for _, capacity := range []int{-1, MaxCapacity + 1} {
		if a, err := New(sm.New(), Config{Capacity: capacity}); a != nil || !errors.Is(err, ErrConfig) {
			t.Fatalf("configuration %d: %v", capacity, err)
		}
	}
	if a, err := New(nil, Config{}); a != nil || !errors.Is(err, ErrConfig) {
		t.Fatalf("nil dependency: %v", err)
	}
	a := newTestApplier(t, sm.New(), Config{})
	if a.Progress().Capacity != DefaultCapacity {
		t.Fatal("incorrect default capacity")
	}
	for _, c := range []sm.Command{
		{Operation: sm.Put, Key: "k", Value: make([]byte, MaxCommandBytes+1)},
		{Operation: sm.Operation(strings.Repeat("x", MaxCommandBytes+1))},
		{Operation: sm.CAS, Key: "k", Expected: &sm.Expectation{Value: make([]byte, MaxCommandBytes+1)}},
	} {
		if f, err := a.TrySubmit(Entry{1, c}); f != nil || !errors.Is(err, ErrTooLarge) {
			t.Fatalf("oversized admission: %v", err)
		}
	}
	if a.Progress().Scheduled != 0 {
		t.Fatal("refused entry advanced scheduling")
	}
	completed(t, submit(t, a, 1, put("k", "ok")), sm.OK)
}
