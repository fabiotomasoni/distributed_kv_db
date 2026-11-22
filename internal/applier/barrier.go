package applier

import (
	"context"
	"errors"
	"sync"
)

var ErrPaused = errors.New("applier: capture barrier freezes admission")

type inspection struct {
	ctx   context.Context
	copy  func(Machine) (any, error)
	reply chan inspectionResult
}
type inspectionResult struct {
	value any
	err   error
}
type parkRequest struct {
	index                    uint64
	ready, finished, release chan struct{}
	requests                 chan inspection
	once                     sync.Once
	busy, releasing          bool // protected by Applier.mu
}

// Barrier is a single bounded lease. Park freezes scheduling at Index, drains
// accepted work and parks the SAME worker. Callers must Release after copying.
// It does not prove Raft commit or consume core completions for the caller.
type Barrier struct {
	owner   *Applier
	request *parkRequest
}

func (b *Barrier) Index() uint64 { return b.request.index }
func (a *Applier) Park(ctx context.Context) (*Barrier, error) {
	if ctx == nil {
		return nil, ErrContext
	}
	a.mu.Lock()
	if a.state != Running {
		a.mu.Unlock()
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	if a.barrier != nil {
		a.mu.Unlock()
		return nil, ErrPaused
	}
	r := &parkRequest{index: a.scheduled, ready: make(chan struct{}), finished: make(chan struct{}), release: make(chan struct{}), requests: make(chan inspection, 1)}
	a.barrier = r
	a.notify()
	a.mu.Unlock()
	select {
	case <-r.ready:
		if e := ctx.Err(); e != nil {
			a.releasePark(r)
			return nil, e
		}
		return &Barrier{a, r}, nil
	case <-ctx.Done():
		a.releasePark(r)
		return nil, ctx.Err()
	case <-r.finished:
		return nil, ErrClosed
	case <-a.done:
		return nil, ErrClosed
	}
}

// Inspect runs one trusted READ-ONLY copy callback in the parked worker, with
// no metadata mutex held. Cancellation abandons only the wait after acceptance.
// Returned data must be independently owned. Callbacks must terminate, must
// not call runtime.Goexit or wait on this applier. A panic fails closed.
func (b *Barrier) Inspect(ctx context.Context, copy func(Machine) (any, error)) (any, error) {
	if ctx == nil {
		return nil, ErrContext
	}
	if copy == nil {
		return nil, ErrConfig
	}
	a, r := b.owner, b.request
	a.mu.Lock()
	if a.state != Running || a.barrier != r || r.releasing {
		a.mu.Unlock()
		return nil, ErrClosed
	}
	if e := ctx.Err(); e != nil {
		a.mu.Unlock()
		return nil, e
	}
	if r.busy {
		a.mu.Unlock()
		return nil, ErrPaused
	}
	r.busy = true
	i := inspection{ctx, copy, make(chan inspectionResult, 1)}
	r.requests <- i
	a.mu.Unlock()
	select {
	case out := <-i.reply:
		return out.value, out.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.finished:
		select {
		case out := <-i.reply:
			return out.value, out.err
		default:
		}
		if e := a.terminalError(); e != nil {
			return nil, e
		}
		return nil, ErrClosed
	}
}
func (a *Applier) releasePark(r *parkRequest) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r.releasing = true
	r.once.Do(func() { close(r.release) })
}

// Release waits for the worker's acknowledgment before admission resumes. A
// canceled wait does not release ownership early; later calls may wait again.
func (b *Barrier) Release(ctx context.Context) error {
	if ctx == nil {
		return ErrContext
	}
	b.owner.releasePark(b.request)
	select {
	case <-b.request.finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func inspect(machine Machine, fn func(Machine) (any, error)) (v any, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			v = nil
			err = ErrApplyFailed
		}
	}()
	v, err = fn(machine)
	returned = true
	return
}
func (a *Applier) park(r *parkRequest) bool {
	defer func() {
		a.mu.Lock()
		if a.barrier == r {
			a.barrier = nil
		}
		close(r.finished)
		a.notify()
		a.mu.Unlock()
	}()
	for {
		select {
		case <-r.release:
			return true
		case i := <-r.requests:
			a.mu.Lock()
			valid := a.state == Running && !r.releasing
			a.mu.Unlock()
			var v any
			var err error
			if !valid {
				err = ErrClosed
			} else if err = i.ctx.Err(); err == nil {
				v, err = inspect(a.machine, i.copy)
			}
			a.mu.Lock()
			r.busy = false
			if errors.Is(err, ErrApplyFailed) {
				a.state = Failed
				a.failure = ErrApplyFailed
			}
			a.mu.Unlock()
			i.reply <- inspectionResult{v, err}
			if errors.Is(err, ErrApplyFailed) {
				return false
			}
		}
	}
}

// AdmissionFrozen reports frozen admission, including the drain before worker parking.
// It is diagnostic metadata, not proof that copying is already safe.
func (a *Applier) AdmissionFrozen() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.barrier != nil }
