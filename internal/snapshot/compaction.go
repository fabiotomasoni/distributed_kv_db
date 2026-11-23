//go:build darwin

package snapshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

var ErrEquivalence = errors.New("snapshot: compact recovery differs from source")

// PrepareCompaction is a pure in-memory Phase D operation on immutable synthetic
// source views. It reconstructs a genuinely applied S, captures in its owner and
// checks full committed recovery equivalence. No files, runtime state, triggers,
// replication progress or source buffers are modified. State comes from Replay.
func PrepareCompaction(ctx context.Context, c config.Config, image []byte, source wal.State, index uint64) (storageformat.Snapshot, error) {
	bad := func(err error) (storageformat.Snapshot, error) { return storageformat.Snapshot{}, err }
	if ctx == nil {
		return bad(applier.ErrContext)
	}
	if err := ctx.Err(); err != nil {
		return bad(err)
	}
	if index < source.BaseIndex || index > source.CommitIndex {
		return bad(ErrBoundary)
	}
	prefix := source
	prefix.CommitIndex = index
	r, err := RecoverCommitted(ctx, c, image, prefix, 1)
	if err != nil {
		return bad(err)
	}
	a, err := applier.New(r.Machine, applier.Config{Capacity: 1, InitialIndex: index})
	if err != nil {
		return bad(err)
	}
	lease, err := a.Park(ctx)
	if err != nil {
		return bad(errors.Join(err, a.Shutdown(context.Background())))
	}
	term := source.BaseTerm
	if index > source.BaseIndex {
		term = source.Entries[index-source.BaseIndex-1].Term
	}
	candidate, err := Capture(ctx, lease, Proof{LastApplied: index, ScheduledApplyIndex: index, CommitIndex: source.CommitIndex, ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return term, nil }})
	err = errors.Join(err, lease.Release(context.Background()), a.Shutdown(context.Background()))
	if err != nil {
		return bad(err)
	}
	raw, err := storageformat.EncodeSnapshot(candidate)
	if err != nil {
		return bad(err)
	}
	log, err := generationWAL(source, candidate)
	if err != nil {
		return bad(err)
	}
	replay, err := wal.Replay(bytes.NewReader(log), index, term)
	if err != nil {
		return bad(err)
	}
	if err = verifyCompactRecovery(ctx, c, image, source, raw, replay.State); err != nil {
		return bad(err)
	}
	return candidate, nil
}

// compareRecovered compares every logical field; hashes/counts alone cannot
// hide a missing cached result, empty value, original index or changed suffix.
// Both recovery workers have terminated before owner-only state copies here.
func compareRecovered(a, b CombinedRecovery) error {
	if a.Machine == nil || b.Machine == nil || a.LastApplied != b.LastApplied || a.ScheduledApplyIndex != b.ScheduledApplyIndex {
		return ErrEquivalence
	}
	av, bv := a.Machine.kv.CopyState(), b.Machine.kv.CopyState()
	if av.LogicalBytes != bv.LogicalBytes || len(av.Records) != len(bv.Records) {
		return fmt.Errorf("%w: KV shape", ErrEquivalence)
	}
	for i, v := range av.Records {
		if v.Key != bv.Records[i].Key || !bytes.Equal(v.Value, bv.Records[i].Value) {
			return fmt.Errorf("%w: KV row %d", ErrEquivalence, i)
		}
	}
	as, bs := a.Machine.sessions.CopySessions(), b.Machine.sessions.CopySessions()
	if len(as) != len(bs) {
		return fmt.Errorf("%w: session count", ErrEquivalence)
	}
	for i, s := range as {
		t := bs[i]
		if s.ClientID != t.ClientID || s.Sequence != t.Sequence || s.Index != t.Index || s.Digest != t.Digest || s.Kind != t.Kind || s.Result.Code != t.Result.Code || s.Result.Applied != t.Result.Applied || s.Result.Deleted != t.Result.Deleted || s.Result.Exists != t.Result.Exists || !bytes.Equal(s.Result.Value, t.Result.Value) {
			return fmt.Errorf("%w: session row %d", ErrEquivalence, i)
		}
	}
	return nil
}
func verifyCompactRecovery(ctx context.Context, c config.Config, beforeImage []byte, before wal.State, afterImage []byte, after wal.State) error {
	a, err := RecoverCommitted(ctx, c, beforeImage, before, 1)
	if err != nil {
		return err
	}
	b, err := RecoverCommitted(ctx, c, afterImage, after, 1)
	if err != nil {
		return err
	}
	if after.BaseIndex < before.BaseIndex || after.BaseIndex > before.CommitIndex || b.Snapshot != (Point{after.BaseIndex, after.BaseTerm}) || before.CommitIndex != after.CommitIndex || before.HardState != after.HardState || before.LastIndex() != after.LastIndex() {
		return fmt.Errorf("%w: log metadata", ErrEquivalence)
	}
	offset := after.BaseIndex - before.BaseIndex
	if offset > uint64(len(before.Entries)) || len(before.Entries)-int(offset) != len(after.Entries) {
		return fmt.Errorf("%w: suffix length", ErrEquivalence)
	}
	for i, e := range after.Entries {
		old := before.Entries[int(offset)+i]
		if e.Index != old.Index || e.Term != old.Term || !bytes.Equal(e.Raw, old.Raw) {
			return fmt.Errorf("%w: suffix row %d", ErrEquivalence, i)
		}
	}
	return compareRecovered(a, b)
}

type compactionEvidence struct {
	Generation                                               string
	Before, After                                            Point
	CommitIndex, LastIndex, LastApplied, ScheduledApplyIndex uint64
	HardState                                                storageformat.HardState
}

// compactIsolated is deliberately package-private. Only test harnesses call it
// on their own synthetic directories/copies. It cannot be enabled by node or
// Service APIs. No deletion/cleanup, original WAL rewrite, live switch or RPC.
// Build, proof and publication serialize under the isolated owner's mutex.
func (p *Persistence) compactIsolated(ctx context.Context, index uint64) (compactionEvidence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	bad := func(err error) (compactionEvidence, error) { return compactionEvidence{}, err }
	if err := p.admitted(ctx); err != nil {
		return bad(err)
	}
	before, err := p.recoverWithRepair(false)
	if err != nil {
		p.failure = err
		return bad(err)
	}
	image, err := storagefs.ReadPrivate(filepath.Join(p.root, before.Generation, "snapshot"), storageformat.MaxSnapshot)
	if err != nil {
		p.failure = err
		return bad(err)
	}
	candidate, err := PrepareCompaction(ctx, p.cfg, image, before.Log, index)
	if err != nil {
		return bad(err)
	}
	generation, err := p.publishOwned(ctx, candidate)
	if err != nil {
		return bad(err)
	}
	// Selection is already published here. A failed post-publication proof is
	// uncertain/fatal, with all candidates retained and NEVER a rollback.
	fail := func(err error) (compactionEvidence, error) {
		p.failure = errors.Join(storagefs.ErrUncertain, err)
		return bad(p.failure)
	}
	after, err := p.recoverWithRepair(false)
	if err != nil {
		return fail(err)
	}
	if after.Generation != generation {
		return fail(ErrEquivalence)
	}
	persisted, err := storagefs.ReadPrivate(filepath.Join(p.root, generation, "snapshot"), storageformat.MaxSnapshot)
	if err != nil {
		return fail(err)
	}
	if err = verifyCompactRecovery(ctx, p.cfg, image, before.Log, persisted, after.Log); err != nil {
		return fail(err)
	}
	return compactionEvidence{Generation: generation, Before: before.Point, After: after.Point, CommitIndex: after.Log.CommitIndex, LastIndex: after.Log.LastIndex(), LastApplied: after.Log.CommitIndex, ScheduledApplyIndex: after.Log.CommitIndex, HardState: after.Log.HardState}, nil
}
