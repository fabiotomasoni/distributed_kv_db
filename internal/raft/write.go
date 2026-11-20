package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type WriteOutcome struct {
	Outcome string
	Applied AppliedResult
}

const (
	Completed   = "completed"
	NotAdmitted = "not_admitted"
	Unknown     = "unknown"
)

type writeTicket struct {
	mu                  sync.Mutex
	ctx                 context.Context
	kind                string
	command             json.RawMessage
	recording, canceled bool
	reply               chan outcome
}
type writeFence struct {
	receipt LocalAppend
	reply   chan error
}

// Write is one attempt, without sessions or retries. Before recording, cancellation
// can exclude the ticket under its lock. After that boundary all failures are
// uncertain (except WAL's explicit pre-write reserve rejection).
func (x *Service) Write(ctx context.Context, kind string, command json.RawMessage) (WriteOutcome, error) {
	no := WriteOutcome{Outcome: NotAdmitted}
	if ctx == nil || (kind != "put" && kind != "delete") || len(command) > storageformat.MaxEntry {
		return no, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return no, err
	}
	if x.application == nil {
		return no, ErrUnsupported
	}
	select {
	case x.writeSlots <- struct{}{}:
		defer func() { <-x.writeSlots }()
	default:
		return no, ErrFull
	}
	t := &writeTicket{ctx: ctx, kind: kind, command: bytes.Clone(command), reply: make(chan outcome, 1)}
	x.mu.Lock()
	if x.closed || !x.started {
		x.mu.Unlock()
		return no, ErrClosed
	}
	if err := x.Status().Failure; err != nil {
		x.mu.Unlock()
		return no, err
	}
	select {
	case x.writes <- t:
		x.mu.Unlock()
	default:
		x.mu.Unlock()
		return no, ErrFull
	}
	uncertain := func(err error) (WriteOutcome, error) {
		t.mu.Lock()
		defer t.mu.Unlock()
		if !t.recording {
			t.canceled = true
			return no, err
		}
		return WriteOutcome{Outcome: Unknown}, err
	}
	var r outcome
	select {
	case r = <-t.reply:
	case <-ctx.Done():
		return uncertain(ctx.Err())
	case <-x.ctx.Done():
		return uncertain(x.terminal())
	}
	if r.err != nil {
		return uncertain(r.err)
	}
	applied, err := x.WaitApplied(ctx, r.local)
	if err != nil {
		return WriteOutcome{Outcome: Unknown}, err
	}
	f := writeFence{r.local, make(chan error, 1)}
	select {
	case x.fences <- f:
	case <-ctx.Done():
		return WriteOutcome{Outcome: Unknown}, ctx.Err()
	case <-x.ctx.Done():
		return WriteOutcome{Outcome: Unknown}, x.terminal()
	}
	select {
	case err = <-f.reply:
	case <-ctx.Done():
		err = ctx.Err()
	case <-x.ctx.Done():
		err = x.terminal()
	}
	if err != nil {
		return WriteOutcome{Outcome: Unknown}, err
	}
	if err = ctx.Err(); err != nil {
		return WriteOutcome{Outcome: Unknown}, err
	}
	return WriteOutcome{Outcome: Completed, Applied: applied}, nil
}
func (x *Service) processWrite(t *writeTicket) {
	t.mu.Lock()
	if t.canceled || t.ctx.Err() != nil {
		t.canceled = true
		t.mu.Unlock()
		t.reply <- outcome{err: context.Canceled}
		return
	}
	e, err := x.core.AppendLocal(x.clock.Now(), t.kind, t.command)
	// The pure core has not performed I/O yet. Publish uncertainty before the
	// blocking disk effect, without holding this lock over filesystem calls.
	t.recording = e.Persist != nil
	t.mu.Unlock()
	e, err = x.effects(e, err)
	if err == nil {
		err = e.Rejected
	}
	if errors.Is(err, wal.ErrLimit) {
		t.mu.Lock()
		t.recording = false
		t.mu.Unlock()
	}
	r := outcome{err: err}
	if e.Local != nil {
		r.local = *e.Local
	} else if err == nil {
		r.err = ErrUnsupported
	}
	t.reply <- r
}
func (x *Service) processFence(f writeFence) {
	s := x.core.Status()
	err := s.Failure
	if err == nil && (x.ctx.Err() != nil || s.Closed) {
		err = ErrClosed
	}
	if err == nil && (s.Role != Leader || s.Term != f.receipt.Term) {
		err = ErrNotLeader
	}
	if err == nil && (s.CommitIndex < f.receipt.Index || s.LastApplied < f.receipt.Index) {
		err = ErrInput
	}
	f.reply <- err
}
