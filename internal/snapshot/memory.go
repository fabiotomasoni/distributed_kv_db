// Package snapshot implements bounded M13 capture, generation persistence and
// explicitly enabled recovery. It never triggers publication or peer transfer.
package snapshot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

var ErrProfile = errors.New("snapshot: v2 profile/identity required")
var ErrBoundary = errors.New("snapshot: unproven or stale capture boundary")

type Point struct{ Index, Term uint64 }

// Proof must be obtained from the SERIAL coordinator after consuming every
// completion through Barrier.Index. TermAt must read that same owned log view.
// These assertions do not themselves establish Raft commit/quorum evidence.
type Proof struct {
	LastApplied, ScheduledApplyIndex, CommitIndex, ApplyEpoch uint64
	TermAt                                                    func(uint64) (uint64, error)
}

// Machine belongs exclusively to one applier. Initial creation/restore are
// offline and return a fresh instance; callers must not retain a second owner.
type Machine struct {
	cfg          config.Config
	kv           *statemachine.Machine
	sessions     *committed.Sessions
	delegate     applier.Machine
	base         Point
	applied      uint64
	boundaryTerm uint64
	knownTerm    uint64
}

func NewMachine(c config.Config) (*Machine, error) {
	if !c.Valid() || !c.Sessions() {
		return nil, ErrProfile
	}
	kv := statemachine.New()
	sessions := committed.NewSessions(kv)
	return &Machine{cfg: c, kv: kv, sessions: sessions, delegate: committed.Wrap(sessions)}, nil
}
func (m *Machine) Apply(c statemachine.Command) statemachine.Result {
	if m.applied == ^uint64(0) {
		panic(ErrBoundary)
	}
	var term uint64
	if c.Operation == "raft/session" {
		var entry struct{ Index, Term string }
		if json.Unmarshal(c.Value, &entry) != nil {
			panic(ErrBoundary)
		}
		index, e := strconv.ParseUint(entry.Index, 10, 64)
		if e != nil || index != m.applied+1 {
			panic(ErrBoundary)
		}
		term, e = strconv.ParseUint(entry.Term, 10, 64)
		if e != nil || term == 0 || term < m.knownTerm {
			panic(ErrBoundary)
		}
	}
	r := m.delegate.Apply(c)
	if r.Code == statemachine.InvalidArgument {
		panic(ErrBoundary)
	}
	m.applied++
	m.boundaryTerm = term
	if term > m.knownTerm {
		m.knownTerm = term
	}
	return r
}

// Capture copies inside the parked worker. The caller owns the lease and must
// Release it before encoding. Cancellation never exposes the live state.
func Capture(ctx context.Context, b *applier.Barrier, p Proof) (storageformat.Snapshot, error) {
	if ctx == nil {
		return storageformat.Snapshot{}, applier.ErrContext
	}
	if err := ctx.Err(); err != nil {
		return storageformat.Snapshot{}, err
	}
	if b == nil {
		return storageformat.Snapshot{}, ErrBoundary
	}
	index := b.Index()
	if p.LastApplied != index || p.ScheduledApplyIndex != index || p.CommitIndex < index || p.ApplyEpoch != 1 {
		return storageformat.Snapshot{}, ErrBoundary
	}
	var term uint64
	if index > 0 {
		if p.TermAt == nil {
			return storageformat.Snapshot{}, ErrBoundary
		}
		var err error
		term, err = p.TermAt(index)
		if err != nil || term == 0 {
			return storageformat.Snapshot{}, ErrBoundary
		}
	} else if p.TermAt != nil {
		var err error
		term, err = p.TermAt(0)
		if err != nil || term != 0 {
			return storageformat.Snapshot{}, ErrBoundary
		}
	}
	value, err := b.Inspect(ctx, func(machine applier.Machine) (any, error) {
		m, ok := machine.(*Machine)
		if !ok || term < m.knownTerm || index != m.applied || m.boundaryTerm != 0 && term != m.boundaryTerm || index < m.base.Index || index == m.base.Index && term != m.base.Term {
			return nil, ErrBoundary
		}
		state := m.kv.CopyState()
		sessions := m.sessions.CopySessions()
		image := storageformat.Snapshot{ClusterID: m.cfg.ClusterID(), ConfigHash: m.cfg.Hash(), Index: index, Term: term, LogicalBytes: state.LogicalBytes, KV: make([]storageformat.SnapshotKV, 0, len(state.Records)), Sessions: make([]storageformat.SnapshotSession, 0, len(sessions))}
		for _, row := range state.Records {
			image.KV = append(image.KV, storageformat.SnapshotKV{Key: row.Key, Value: row.Value})
		}
		for _, s := range sessions {
			if s.Index > index {
				return nil, ErrBoundary
			}
			r := storageformat.SnapshotResult{Code: string(s.Result.Code), Index: s.Index, Applied: s.Result.Applied, Deleted: s.Result.Deleted}
			image.Sessions = append(image.Sessions, storageformat.SnapshotSession{ClientID: s.ClientID, Sequence: s.Sequence, CommandSHA256: hex.EncodeToString(s.Digest[:]), Kind: s.Kind, Result: r})
		}
		return image, nil
	})
	if err != nil {
		return storageformat.Snapshot{}, err
	}
	return value.(storageformat.Snapshot), nil
}

// Restore reconstructs only a NEW isolated instance. currentTerm comes from
// its owner's validated hard-state view, not from the snapshot. No replay,
// listener, WAL opening or live map replacement is performed here.
func Restore(c config.Config, image []byte, currentTerm uint64) (*Machine, Point, error) {
	if !c.Valid() || !c.Sessions() {
		return nil, Point{}, ErrProfile
	}
	state, err := storageformat.DecodeSnapshot(image)
	if err != nil {
		return nil, Point{}, err
	}
	if state.ClusterID != c.ClusterID() || state.ConfigHash != c.Hash() {
		return nil, Point{}, ErrProfile
	}
	if state.Term > currentTerm {
		return nil, Point{}, ErrBoundary
	}
	records := make([]statemachine.Record, 0, len(state.KV))
	for _, r := range state.KV {
		records = append(records, statemachine.Record{Key: r.Key, Value: r.Value})
	}
	kv, err := statemachine.FromState(statemachine.StateImage{Records: records, LogicalBytes: state.LogicalBytes})
	if err != nil {
		return nil, Point{}, err
	}
	states := make([]committed.SessionState, 0, len(state.Sessions))
	for _, s := range state.Sessions {
		var digest [32]byte
		raw, _ := hex.DecodeString(s.CommandSHA256)
		copy(digest[:], raw)
		r := statemachine.Result{Code: statemachine.Code(s.Result.Code), Applied: s.Result.Applied, Deleted: s.Result.Deleted}
		states = append(states, committed.SessionState{ClientID: s.ClientID, Sequence: s.Sequence, Index: s.Result.Index, Digest: digest, Kind: s.Kind, Result: r})
	}
	sessions, err := committed.SessionsFromState(kv, states)
	if err != nil {
		return nil, Point{}, err
	}
	point := Point{state.Index, state.Term}
	return &Machine{cfg: c, kv: kv, sessions: sessions, delegate: committed.Wrap(sessions), base: point, applied: point.Index, knownTerm: point.Term}, point, nil
}
