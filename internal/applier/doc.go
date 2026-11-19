// Package applier owns a sequential state machine and applies caller-ordered
// entries through one worker and a bounded queue. It does not determine commit,
// implement Raft, or make statemachine.Machine safe for direct concurrent use.
//
// New transfers exclusive machine ownership until Done closes. Producers may
// submit concurrently, but must supply contiguous global indices. A successful
// admission transfers private copies of command buffers; callers must not mutate
// inputs while Submit/TrySubmit is running. Cancellation after admission never
// removes work. Shutdown stops admission and drains all accepted entries.
package applier
