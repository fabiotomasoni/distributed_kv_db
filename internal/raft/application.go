package raft

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

var ErrResultExpired = errors.New("raft: applied result outside bounded retention")
var ErrSuperseded = errors.New("raft: applied entry differs from local receipt")

type AppliedResult struct {
	Index, Term uint64
	Result      statemachine.Result
}
type applyTicket struct {
	future      *applier.Future
	epoch, term uint64
}
type applyCompletion struct {
	outcome     applier.Outcome
	epoch, term uint64
	err         error
}
type applicationRuntime struct {
	applier     *applier.Applier
	tickets     chan applyTicket
	completions chan applyCompletion
	done        chan struct{}
	collecting  bool
	inflight    int // service owner only
	mu          sync.Mutex
	results     [64]AppliedResult
	last        uint64
	failure     error
	closed      bool
	changed     chan struct{}
}

func (x *Service) initApplication(opt Options) error {
	if !opt.Commit {
		return nil
	}
	if opt.Reads {
		if err := x.core.EnableLoggedReads(); err != nil {
			return err
		}
	}
	if err := x.core.EnableCommit(); err != nil {
		return err
	}
	for _, e := range x.core.entries {
		if _, err := x.project(e); err != nil {
			return err
		}
	}
	capacity := opt.ApplyCapacity
	if capacity == 0 {
		capacity = 64
	}
	if capacity < 1 || capacity > 64 {
		return ErrInput
	}
	var machine applier.Machine = statemachine.New()
	if opt.MachineFactory != nil {
		machine = opt.MachineFactory()
		if machine == nil {
			return ErrInput
		}
	}
	a, err := applier.New(committed.Wrap(machine), applier.Config{Capacity: capacity})
	if err != nil {
		return err
	}
	app := &applicationRuntime{applier: a, tickets: make(chan applyTicket, 64), completions: make(chan applyCompletion, 64), done: make(chan struct{}), changed: make(chan struct{})}
	x.application = app
	// Replay uses a brand-new machine before timers/listeners; never apply the
	// uncommitted suffix or layer replay onto a previously mutated instance.
	for index := uint64(1); index <= x.core.state.CommitIndex; index++ {
		entry := x.core.entry(index)
		command, err := x.project(entry)
		if err != nil {
			_ = a.Shutdown(context.Background())
			return err
		}
		f, err := a.TrySubmit(applier.Entry{Index: index, Command: command})
		if err != nil {
			_ = a.Shutdown(context.Background())
			return err
		}
		if err = x.core.Scheduled(index); err != nil {
			_ = a.Shutdown(context.Background())
			return err
		}
		outcome, err := f.Wait(context.Background())
		if err = x.core.Applied(1, index, err); err != nil {
			_ = a.Shutdown(context.Background())
			return err
		}
		app.record(AppliedResult{index, entry.Term, outcome.Result})
	}
	return nil
}
func (a *applicationRuntime) collect() {
	defer close(a.done)
	for ticket := range a.tickets {
		outcome, err := ticket.future.Wait(context.Background())
		a.completions <- applyCompletion{outcome, ticket.epoch, ticket.term, err}
	}
}
func (a *applicationRuntime) record(r AppliedResult) {
	r.Result.Value = bytes.Clone(r.Result.Value)
	a.mu.Lock()
	a.results[r.Index%64] = r
	a.last = r.Index
	close(a.changed)
	a.changed = make(chan struct{})
	a.mu.Unlock()
}
func (x *Service) completionChannel() <-chan applyCompletion {
	if x.application == nil {
		return nil
	}
	return x.application.completions
}
func (x *Service) scheduleApply() error {
	a := x.application
	if a == nil || x.core.state.Failure != nil {
		return nil
	}
	for x.core.state.ScheduledApplyIndex < x.core.state.CommitIndex && a.inflight < 64 {
		index := x.core.state.ScheduledApplyIndex + 1
		entry := x.core.entry(index)
		command, err := x.project(entry)
		if err != nil {
			_, err = x.core.fail(err)
			return err
		}
		f, err := a.applier.TrySubmit(applier.Entry{Index: index, Command: command})
		if errors.Is(err, applier.ErrFull) {
			return nil
		}
		if err != nil {
			_, err = x.core.fail(err)
			return err
		}
		if err = x.core.Scheduled(index); err != nil {
			_, err = x.core.fail(err)
			return err
		}
		a.inflight++
		a.tickets <- applyTicket{f, x.core.state.ApplyEpoch, entry.Term}
	}
	return nil
}
func (x *Service) applyCompleted(c applyCompletion) {
	a := x.application
	a.inflight--
	if c.epoch != x.core.state.ApplyEpoch {
		return
	}
	if x.core.state.Failure == nil {
		if err := x.core.Applied(c.epoch, c.outcome.Index, c.err); err == nil {
			a.record(AppliedResult{c.outcome.Index, c.term, c.outcome.Result})
		}
	}
	x.published.Store(x.core.Status())
}
func (x *Service) finishApplication() {
	a := x.application
	if a == nil {
		return
	}
	if a.collecting {
		for x.core.state.Failure == nil && x.core.state.LastApplied < x.core.state.CommitIndex {
			if err := x.scheduleApply(); err != nil {
				break
			}
			if a.inflight == 0 {
				_, _ = x.core.fail(ErrInput)
				break
			}
			x.applyCompleted(<-a.completions)
		}
		for a.inflight > 0 {
			x.applyCompleted(<-a.completions)
		}
		close(a.tickets)
	}
	if err := a.applier.Shutdown(context.Background()); err != nil && x.core.state.Failure == nil {
		_, _ = x.core.fail(err)
	}
	if a.collecting {
		<-a.done
	}
	a.mu.Lock()
	a.closed = true
	a.failure = x.core.state.Failure
	close(a.changed)
	a.changed = make(chan struct{})
	a.mu.Unlock()
}

// WaitApplied is internal result delivery, not a client success or read API.
// Cancellation abandons the wait; it cannot revoke a committed application.
func (x *Service) WaitApplied(ctx context.Context, receipt LocalAppend) (AppliedResult, error) {
	if ctx == nil || receipt.Index == 0 || receipt.Term == 0 {
		return AppliedResult{}, ErrInput
	}
	a := x.application
	if a == nil {
		return AppliedResult{}, ErrUnsupported
	}
	for {
		a.mu.Lock()
		if receipt.Index <= a.last {
			r := a.results[receipt.Index%64]
			a.mu.Unlock()
			if r.Index != receipt.Index {
				return AppliedResult{}, ErrResultExpired
			}
			if r.Term != receipt.Term {
				return AppliedResult{}, ErrSuperseded
			}
			r.Result.Value = bytes.Clone(r.Result.Value)
			return r, nil
		}
		if a.closed {
			err := a.failure
			if err == nil {
				err = ErrClosed
			}
			a.mu.Unlock()
			return AppliedResult{}, err
		}
		changed := a.changed
		a.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return AppliedResult{}, ctx.Err()
		}
	}
}

func (x *Service) project(e storageformat.LogEntry) (statemachine.Command, error) {
	if x.core.readsEnabled {
		return committed.ProjectWithReads(e)
	}
	return committed.Project(e)
}
