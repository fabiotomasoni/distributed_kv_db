package applier

import (
	"errors"

	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

const (
	DefaultCapacity = 64
	MaxCapacity     = 1024
	// MaxCommandBytes bounds copied input, not a wire or persistence format.
	MaxCommandBytes = 48 * 1024
)

var (
	ErrConfig         = errors.New("applier: invalid configuration")
	ErrContext        = errors.New("applier: nil context")
	ErrClosed         = errors.New("applier: admission closed")
	ErrFull           = errors.New("applier: queue full")
	ErrDuplicate      = errors.New("applier: index already scheduled")
	ErrGap            = errors.New("applier: missing preceding index")
	ErrIndexExhausted = errors.New("applier: index exhausted")
	ErrTooLarge       = errors.New("applier: command exceeds admission byte limit")
	ErrApplyFailed    = errors.New("applier: application panicked; effect uncertain")
	ErrAborted        = errors.New("applier: not applied after worker failure")
)

// Machine is the only production dependency. Apply must terminate, must not call
// back into this Applier, and must not use runtime.Goexit. A panic fails the worker
// closed; ordinary domain failures remain statemachine.Result values.
// Returning transfers the result buffer to the worker; the dependency must not
// mutate it afterward.
type Machine interface {
	Apply(statemachine.Command) statemachine.Result
}

// Config sets queue capacity (zero selects DefaultCapacity) and the already
// applied base index. InitialIndex must describe the supplied machine's state.
// This does not load a snapshot or establish whether any entry is committed.
type Config struct {
	Capacity     int
	InitialIndex uint64
}

// Entry is a caller-ordered command. The caller, not this package, is responsible
// for establishing commit before using this as a replicated-log applier.
type Entry struct {
	Index   uint64
	Command statemachine.Command
}

// Outcome correlates a completed application with its global index.
type Outcome struct {
	Index  uint64
	Result statemachine.Result
}

type State string

const (
	Running  State = "running"
	Draining State = "draining"
	Stopped  State = "stopped"
	Failed   State = "failed"
)

// Progress is a synchronized observation, not a source of consensus decisions.
// Queued excludes the at-most-one active call. Scheduled advances on admission;
// LastApplied advances only after Apply returns a domain result.
type Progress struct {
	State       State
	Capacity    int
	Queued      int
	Active      bool
	Scheduled   uint64
	LastApplied uint64
}
