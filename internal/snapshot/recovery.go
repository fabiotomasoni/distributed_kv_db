package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

// CombinedRecovery transfers exclusive ownership of a fully reconstructed fresh
// machine after its recovery worker has stopped. A caller may start exactly one
// new applier at LastApplied. No old waiter/result ring is restored.
type CombinedRecovery struct {
	Machine                          *Machine
	Snapshot                         Point
	LastApplied, ScheduledApplyIndex uint64
}

// RecoverCommitted is the Phase C isolated step. State must come from validated
// WAL replay. Inputs must stay immutable until return. Full-prefix views need
// term agreement at S; entries through S are validated but NEVER applied. The
// layout remains the approved base-S WAL; no alternate disk format is added.
func RecoverCommitted(ctx context.Context, c config.Config, image []byte, state wal.State, capacity int) (CombinedRecovery, error) {
	bad := func(err error) (CombinedRecovery, error) { return CombinedRecovery{}, err }
	if ctx == nil {
		return bad(applier.ErrContext)
	}
	if err := ctx.Err(); err != nil {
		return bad(err)
	}
	if capacity < 1 || capacity > 64 {
		return bad(applier.ErrConfig)
	}
	s, err := storageformat.DecodeSnapshot(image)
	if err != nil {
		return bad(err)
	}
	if state.Sequence == 0 || state.BaseIndex > math.MaxUint64-uint64(len(state.Entries)) || (state.BaseIndex == 0) != (state.BaseTerm == 0) || state.BaseIndex > s.Index || state.CommitIndex < s.Index || state.CommitIndex > state.LastIndex() || state.HardState.Term < state.BaseTerm {
		return bad(ErrBoundary)
	}
	if state.HardState.Vote != "" {
		if _, ok := c.Member(state.HardState.Vote); !ok || state.HardState.Term == 0 {
			return bad(ErrBoundary)
		}
	}
	term := state.BaseTerm
	used := 0
	for i, e := range state.Entries {
		if len(e.Raw) > int(wal.MaxWALBytes)-used {
			return bad(ErrBoundary)
		}
		used += len(e.Raw)
		if e.Index != state.BaseIndex+uint64(i)+1 || e.Term < term || e.Term > state.HardState.Term || storageformat.ValidateEntry(e.Raw) != nil {
			return bad(ErrBoundary)
		}
		var fields struct{ Index, Term string }
		if json.Unmarshal(e.Raw, &fields) != nil || fields.Index != strconv.FormatUint(e.Index, 10) || fields.Term != strconv.FormatUint(e.Term, 10) {
			return bad(ErrBoundary)
		}
		term = e.Term
	}
	boundaryTerm := state.BaseTerm
	if s.Index > state.BaseIndex {
		boundaryTerm = state.Entries[s.Index-state.BaseIndex-1].Term
	}
	if boundaryTerm != s.Term {
		return bad(ErrBoundary)
	}
	machine, point, err := Restore(c, image, state.HardState.Term)
	if err != nil {
		return bad(err)
	}
	a, err := applier.New(machine, applier.Config{Capacity: capacity, InitialIndex: s.Index})
	if err != nil {
		return bad(err)
	}
	fail := func(err error) (CombinedRecovery, error) {
		return bad(errors.Join(err, a.Shutdown(context.Background())))
	}
	// Count offsets rather than S+1 to keep S=MaxUint64 safe.
	last := s.Index
	for offset := s.Index - state.BaseIndex; offset < state.CommitIndex-state.BaseIndex; offset++ {
		entry := state.Entries[offset]
		command, e := committed.ProjectWithSessions(entry)
		if e != nil {
			return fail(e)
		}
		f, e := a.Submit(ctx, applier.Entry{Index: entry.Index, Command: command})
		if e != nil {
			return fail(e)
		}
		out, e := f.Wait(ctx)
		if e != nil {
			return fail(e)
		}
		if _, _, e = committed.DecodeSessionResult(out.Result, entry.Index); e != nil {
			return fail(e)
		}
		if out.Index != entry.Index {
			return fail(ErrBoundary)
		}
		last = out.Index
	}
	if err = a.Shutdown(context.Background()); err != nil {
		return bad(err)
	}
	if err = ctx.Err(); err != nil {
		return bad(err)
	}
	return CombinedRecovery{Machine: machine, Snapshot: point, LastApplied: last, ScheduledApplyIndex: last}, nil
}
