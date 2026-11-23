//go:build darwin

package snapshot

import (
	"bytes"
	"context"
	"errors"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
	"math"
	"os"
)

var ErrObsolete = errors.New("snapshot: incoming boundary already applied")

// PlanInstallation is a pure isolated reconciliation, never quorum evidence.
// Local is a validated final follower WAL view; applied is the drained actual
// owner cursor. Current term/vote always remain LOCAL, never copied from image.
func PlanInstallation(c config.Config, image []byte, local wal.State, applied, leaderTerm uint64) (wal.State, error) {
	s, err := storageformat.DecodeSnapshot(image)
	if err != nil {
		return wal.State{}, err
	}
	if !c.Valid() || !c.Sessions() || s.ClusterID != c.ClusterID() || s.ConfigHash != c.Hash() {
		return wal.State{}, ErrProfile
	}
	if local.BaseIndex > math.MaxUint64-uint64(len(local.Entries)) || (local.BaseIndex == 0) != (local.BaseTerm == 0) || local.Sequence == 0 || applied < local.BaseIndex || applied > local.CommitIndex || local.CommitIndex > local.LastIndex() || local.HardState.Term < local.BaseTerm || local.HardState.Term != leaderTerm || s.Term > leaderTerm {
		return wal.State{}, ErrBoundary
	}
	if s.Index <= applied {
		return wal.State{}, ErrObsolete
	}
	term := local.BaseTerm
	match := s.Index == local.BaseIndex
	if s.Index > local.BaseIndex && s.Index-local.BaseIndex <= uint64(len(local.Entries)) {
		term = local.Entries[s.Index-local.BaseIndex-1].Term
		match = true
	}
	match = match && term == s.Term
	if !match && s.Index <= local.CommitIndex {
		return wal.State{}, ErrBoundary
	}
	next := wal.State{BaseIndex: s.Index, BaseTerm: s.Term, HardState: local.HardState, CommitIndex: s.Index, Sequence: 1}
	if match {
		if local.CommitIndex > next.CommitIndex {
			next.CommitIndex = local.CommitIndex
		}
		for _, e := range local.Entries[s.Index-local.BaseIndex:] {
			e.Raw = append([]byte(nil), e.Raw...)
			next.Entries = append(next.Entries, e)
		}
	}
	// Reuse the approved WAL generator and reader as the state validation gate.
	raw, err := generationWAL(next, s)
	if err != nil {
		return wal.State{}, err
	}
	replay, err := wal.Replay(bytes.NewReader(raw), s.Index, s.Term)
	if err != nil {
		return wal.State{}, err
	}
	return replay.State, nil
}

// PublishReceived is dormant and called only within Store.TransitionSnapshot by
// the isolated receiver. It borrows the already-held lock; it NEVER closes it.
// No public server handler, trigger, automatic GC or file deletion is added.
func PublishReceived(ctx context.Context, c config.Config, root string, owner *storagefs.Lock, local wal.State, image []byte, applied, leaderTerm uint64) (string, error) {
	if owner == nil {
		return "", ErrBoundary
	}

	p := &Persistence{root: root, cfg: c, lock: owner, ops: diskOps{create: func(path string) (durableFile, error) { return storagefs.OpenRegular(path, true) }, rename: os.Rename, syncDir: storagefs.SyncDirectory, usage: diskUsage}}
	return p.installReceivedOwned(ctx, local, image, applied, leaderTerm)
}

// installReceivedOwned uses the same isolated publication seams as Phase B.
func (p *Persistence) installReceivedOwned(ctx context.Context, local wal.State, image []byte, applied, leaderTerm uint64) (string, error) {
	next, err := PlanInstallation(p.cfg, image, local, applied, leaderTerm)
	if err != nil {
		return "", err
	}
	actual, err := p.recoverWithRepair(false)
	if err != nil {
		return "", err
	}
	if actual.Log.Sequence != local.Sequence || actual.Log.BaseIndex != local.BaseIndex || actual.Log.BaseTerm != local.BaseTerm || actual.Log.HardState != local.HardState || actual.Log.CommitIndex != local.CommitIndex || actual.Log.LastIndex() != local.LastIndex() {
		return "", ErrBoundary
	}
	s, err := storageformat.DecodeSnapshot(image)
	if err != nil {
		return "", err
	}
	return p.publishViewOwned(ctx, s, next)
}
