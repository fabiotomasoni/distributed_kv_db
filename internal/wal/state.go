package wal

import (
	"bytes"
	"errors"
	"math"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

var (
	ErrCorrupt        = errors.New("wal: corrupt or inconsistent persistent state")
	ErrTransition     = errors.New("wal: invalid persistent transition")
	ErrIncomplete     = errors.New("wal: no valid initial transaction")
	ErrLimit          = errors.New("wal: physical quota exceeded")
	ErrFailed         = errors.New("wal: writer failed; outcome uncertain, restart required")
	ErrClosed         = errors.New("wal: closed")
	ErrNotInitialized = errors.New("wal: no initialized storage; explicit bootstrap required")
	ErrNotEmpty       = errors.New("wal: bootstrap requires an empty directory")
	ErrSnapshot       = errors.New("wal: nonempty snapshot requires the later snapshot phase")
)

const (
	MaxWALBytes       int64 = 8 * 1024 * 1024
	ConsensusReserve  int64 = 512 * 1024
	MaxDirectoryBytes int64 = 32 * 1024 * 1024
	ControlReserve    int64 = 64 * 1024
)

// State is a detached log-level view. Entries can include an uncommitted suffix;
// neither presence here nor a storage token grants permission to apply them.
type State struct {
	BaseIndex, BaseTerm   uint64
	HardState             storageformat.HardState
	CommitIndex, Sequence uint64
	Entries               []storageformat.LogEntry
}

func cloneEntry(e storageformat.LogEntry) storageformat.LogEntry {
	e.Raw = bytes.Clone(e.Raw)
	return e
}
func (s State) clone() State {
	s.Entries = append([]storageformat.LogEntry(nil), s.Entries...)
	for i := range s.Entries {
		s.Entries[i] = cloneEntry(s.Entries[i])
	}
	return s
}
func (s State) LastIndex() uint64 { return s.BaseIndex + uint64(len(s.Entries)) }

// CommittedEntries is a copy of just the recovered checkpoint-covered suffix.
// A future owner still coordinates sessions/noops/epochs before any application.
func (s State) CommittedEntries() []storageformat.LogEntry {
	if s.CommitIndex < s.BaseIndex || s.CommitIndex-s.BaseIndex > uint64(len(s.Entries)) {
		return nil
	}
	n := s.CommitIndex - s.BaseIndex
	out := make([]storageformat.LogEntry, n)
	copy(out, s.Entries[:n])
	for i := range out {
		out[i] = cloneEntry(out[i])
	}
	return out
}

func (s State) next(tx storageformat.Transaction, seq uint64) (State, error) {
	bad := func() (State, error) { return State{}, ErrTransition }
	if s.Sequence == math.MaxUint64 || seq != s.Sequence+1 {
		return bad()
	}
	if s.Sequence == 0 {
		if tx.HardState == nil || tx.Commit == nil || *tx.Commit != s.BaseIndex || tx.Replace != nil || tx.HardState.Term < s.BaseTerm {
			return bad()
		}
	}
	n := s
	n.Sequence = seq
	if h := tx.HardState; h != nil {
		if h.Term < s.HardState.Term || h.Term == s.HardState.Term && s.HardState.Vote != "" && h.Vote != s.HardState.Vote {
			return bad()
		}
		n.HardState = *h
	}
	if r := tx.Replace; r != nil {
		last := s.LastIndex()
		if r.From <= s.CommitIndex || r.From <= s.BaseIndex || r.From > last && (last == math.MaxUint64 || r.From != last+1) {
			return bad()
		}
		cut := r.From - s.BaseIndex - 1
		if cut > uint64(len(s.Entries)) {
			return bad()
		}
		term := s.BaseTerm
		if cut > 0 {
			term = s.Entries[cut-1].Term
		}
		entries := make([]storageformat.LogEntry, int(cut), int(cut)+len(r.Entries))
		copy(entries, s.Entries[:cut])
		for _, e := range r.Entries {
			if e.Term < term || e.Term > n.HardState.Term {
				return bad()
			}
			term = e.Term
			entries = append(entries, e)
		}
		n.Entries = entries
	}
	if tx.Commit != nil {
		if *tx.Commit < s.CommitIndex || *tx.Commit > n.LastIndex() {
			return bad()
		}
		n.CommitIndex = *tx.Commit
	}
	if n.HardState.Term < n.BaseTerm {
		return bad()
	}
	return n, nil
}
