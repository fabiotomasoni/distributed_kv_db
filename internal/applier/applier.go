package applier

import (
	"context"
	"math"
	"strings"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

type work struct {
	entry  Entry
	future *Future
}

// Applier serializes access to its exclusively owned Machine. It must not be
// copied. The mutex protects queue/lifecycle metadata, never a Machine.Apply
// call; the worker is the sole caller of that method.
type Applier struct {
	mu          sync.Mutex
	machine     Machine
	queue       []work
	head        int
	count       int
	active      bool
	scheduled   uint64
	lastApplied uint64
	state       State
	failure     error
	changed     chan struct{}
	done        chan struct{}
}

// New starts one worker. The concrete machine must be non-nil and must not be
// supplied to another owner. A nil interface or an invalid capacity is rejected.
func New(machine Machine, config Config) (*Applier, error) {
	capacity := config.Capacity
	if capacity == 0 {
		capacity = DefaultCapacity
	}
	if machine == nil || capacity < 1 || capacity > MaxCapacity {
		return nil, ErrConfig
	}
	a := &Applier{
		machine: machine, queue: make([]work, capacity),
		scheduled: config.InitialIndex, lastApplied: config.InitialIndex,
		state: Running, changed: make(chan struct{}), done: make(chan struct{}),
	}
	go a.run()
	return a, nil
}

// Submit waits for queue space, not missing indices. Duplicate/out-of-order
// indices fail immediately. A nil Future with an error means no admission and no
// index advancement. Once a Future is returned, ctx no longer controls the work.
// Cancellation racing admission may yield either a Future or a context error.
func (a *Applier) Submit(ctx context.Context, entry Entry) (*Future, error) {
	if ctx == nil {
		return nil, ErrContext
	}
	return a.submit(ctx, entry, true)
}

// TrySubmit never waits for space. ErrFull means the caller retains the entry
// and must retry or fail explicitly; an accepted committed entry is never dropped.
func (a *Applier) TrySubmit(entry Entry) (*Future, error) {
	return a.submit(context.Background(), entry, false)
}

func (a *Applier) submit(ctx context.Context, entry Entry, wait bool) (*Future, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for {
		if a.state != Running {
			return nil, ErrClosed
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if a.scheduled == math.MaxUint64 {
			return nil, ErrIndexExhausted
		}
		if entry.Index <= a.scheduled {
			return nil, ErrDuplicate
		}
		if entry.Index != a.scheduled+1 {
			return nil, ErrGap
		}
		if !bounded(entry.Command) {
			return nil, ErrTooLarge
		}
		if a.count < len(a.queue) {
			owned := cloneCommand(entry.Command)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			entry.Command = owned
			future := &Future{index: entry.Index, done: make(chan struct{})}
			a.queue[(a.head+a.count)%len(a.queue)] = work{entry, future}
			a.count++
			a.scheduled = entry.Index
			a.notify()
			return future, nil
		}
		if !wait {
			return nil, ErrFull
		}
		changed := a.changed
		a.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
		a.mu.Lock()
	}
}

// Shutdown closes admission and drains accepted work. Context cancellation limits
// the wait only: it cannot undo shutdown or interrupt a Machine.Apply call.
// Repeated/concurrent calls are safe. After a worker panic it returns ErrApplyFailed.
func (a *Applier) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return ErrContext
	}
	a.mu.Lock()
	if a.state == Running {
		a.state = Draining
		a.notify()
	}
	a.mu.Unlock()
	select {
	case <-a.done:
		return a.terminalError()
	default:
	}
	select {
	case <-a.done:
		return a.terminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done closes after all accepted futures have terminal outcomes and worker work
// has finished. No producer ever sends to or closes this channel.
func (a *Applier) Done() <-chan struct{} { return a.done }

func (a *Applier) Progress() Progress {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Progress{a.state, len(a.queue), a.count, a.active, a.scheduled, a.lastApplied}
}

func (a *Applier) terminalError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failure
}

// notify broadcasts a metadata change while mu is held. There are no concurrent
// sends to a work channel, so shutdown cannot race a send against channel close.
func (a *Applier) notify() {
	close(a.changed)
	a.changed = make(chan struct{})
}

func (a *Applier) pop() work {
	item := a.queue[a.head]
	a.queue[a.head] = work{}
	a.head = (a.head + 1) % len(a.queue)
	a.count--
	return item
}

func (a *Applier) run() {
	defer close(a.done)
	for {
		a.mu.Lock()
		if a.count == 0 {
			if a.state == Draining {
				a.state = Stopped
				a.notify()
				a.mu.Unlock()
				return
			}
			changed := a.changed
			a.mu.Unlock()
			<-changed
			continue
		}
		item := a.pop()
		a.active = true
		a.notify()
		a.mu.Unlock()

		result, err := invoke(a.machine, item.entry.Command)
		a.mu.Lock()
		a.active = false
		if err != nil {
			a.state, a.failure = Failed, err
			for a.count > 0 {
				a.pop().future.finish(statemachine.Result{}, ErrAborted)
			}
			a.notify()
			a.mu.Unlock()
			item.future.finish(statemachine.Result{}, err)
			return
		}
		a.lastApplied = item.entry.Index
		a.notify()
		a.mu.Unlock()
		item.future.finish(result, nil)
	}
}

func invoke(machine Machine, command statemachine.Command) (result statemachine.Result, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			result, err = statemachine.Result{}, ErrApplyFailed
		}
	}()
	result = machine.Apply(command)
	returned = true
	return result, nil
}

func bounded(command statemachine.Command) bool {
	remaining := MaxCommandBytes
	sizes := []int{len(command.Operation), len(command.Key), len(command.Value)}
	if command.Expected != nil {
		sizes = append(sizes, len(command.Expected.Value))
	}
	for _, size := range sizes {
		if size > remaining {
			return false
		}
		remaining -= size
	}
	return true
}

func cloneCommand(command statemachine.Command) statemachine.Command {
	command.Operation = statemachine.Operation(strings.Clone(string(command.Operation)))
	command.Key = strings.Clone(command.Key)
	command.Value = cloneBytes(command.Value)
	if command.Expected != nil {
		expected := *command.Expected
		expected.Value = cloneBytes(expected.Value)
		command.Expected = &expected
	}
	return command
}
