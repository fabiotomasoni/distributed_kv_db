package wal

import (
	"errors"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

// TransitionSnapshot is a dormant, explicitly invoked isolated owner handoff.
// It serializes with Append/Close and lends the SAME stable LOCK to one trusted,
// finite, non-reentrant publication callback. The callback must not close the
// lock or call this Store. No ordinary node/service path invokes this method.
// Any callback/reopen/close failure poisons this owner; CURRENT is never rolled
// back. All old files remain intact. Close still releases owned descriptors.
func (s *Store) TransitionSnapshot(publish func(string, *storagefs.Lock, State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.failure != nil {
		return errors.Join(ErrFailed, s.failure)
	}
	if s.applicationVersion != 2 || publish == nil {
		return ErrTransition
	}
	fail := func(err error) error { s.failure = err; return errors.Join(ErrFailed, err) }
	if err := publish(s.root, s.owner, s.state.clone()); err != nil {
		return fail(err)
	}
	next, err := openOwnedSnapshotVersion(s.root, s.identity, s.owner, 2, true)
	if err != nil {
		return fail(err)
	}
	if next.generation == s.generation {
		return fail(errors.Join(ErrTransition, next.file.Close()))
	}
	old := s.file
	s.file = next.file
	s.state = next.state
	s.generation = next.generation
	s.bytes = next.bytes
	s.repairedTail = next.repairedTail
	s.snapshotImage = next.snapshotImage
	if err = old.Close(); err != nil {
		return fail(err)
	}
	return nil
}
