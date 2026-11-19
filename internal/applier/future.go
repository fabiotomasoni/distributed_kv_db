package applier

import (
	"context"

	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

// Future is a per-entry completion, safe for repeated/concurrent Wait calls. It
// retains no reference to the Applier. Retaining futures is the caller's choice;
// the worker does not keep an unbounded table of historical results.
// Use the pointer returned by admission; a Future must not be copied or created
// as a zero value.
type Future struct {
	index  uint64
	done   chan struct{}
	result statemachine.Result
	err    error
}

// Wait cancels only this wait, never the accepted entry. A context error means
// the outcome is still unknown to this caller; the same Future can be awaited
// again. Each completed read result has its own byte buffer.
func (f *Future) Wait(ctx context.Context) (Outcome, error) {
	if ctx == nil {
		return Outcome{Index: f.index}, ErrContext
	}
	select {
	case <-f.done:
		return f.outcome()
	default:
	}
	select {
	case <-f.done:
		return f.outcome()
	case <-ctx.Done():
		return Outcome{Index: f.index}, ctx.Err()
	}
}

func (f *Future) outcome() (Outcome, error) {
	result := f.result
	result.Value = cloneBytes(result.Value)
	return Outcome{Index: f.index, Result: result}, f.err
}

// finish has exactly one writer. Closing done publishes the immutable fields to
// all waiters; no waiter must receive for the worker to continue.
func (f *Future) finish(result statemachine.Result, err error) {
	result.Value = cloneBytes(result.Value)
	f.result, f.err = result, err
	close(f.done)
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	result := make([]byte, len(value))
	copy(result, value)
	return result
}
