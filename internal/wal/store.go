package wal

import (
	"errors"
	"io"
	"math"
	"os"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

type walFile interface {
	io.Reader
	io.Writer
	io.Seeker
	Sync() error
	Truncate(int64) error
	Close() error
}

// Token is returned only after successful Sync, scoped by generation and physical
// sequence. It certifies local storage completion, never distributed commit.
type Token struct {
	Generation string
	Sequence   uint64
}

// Store serializes synchronous I/O and Close. No callbacks run under a Raft lock
// here: there is no consensus integration. Do not copy Store; inputs must remain
// immutable throughout Append. Invalid admission does not poison the writer.
type Store struct {
	snapshotImage       []byte
	applicationVersion  int
	mu                  sync.Mutex
	root, generation    string
	owner               *storagefs.Lock
	file                walFile
	usage               func(string) (int64, error)
	state               State
	bytes, repairedTail int64
	failure             error
	closed              bool
}

type Status struct {
	Generation                             string
	Sequence, Term, CommitIndex, LastIndex uint64
	WALBytes, RepairedTailBytes            int64
	Closed, Failed                         bool
}

func (s *Store) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{s.generation, s.state.Sequence, s.state.HardState.Term, s.state.CommitIndex, s.state.LastIndex(), s.bytes, s.repairedTail, s.closed, s.failure != nil}
}

func (s *Store) State() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state.clone() }

// Append is the replication/control path, bounded by the absolute WAL cap.
// Callers must use AppendProposal for new proposal admission to preserve reserve.
func (s *Store) Append(payload []byte) (Token, error)         { return s.append(payload, false) }
func (s *Store) AppendProposal(payload []byte) (Token, error) { return s.append(payload, true) }

func (s *Store) append(payload []byte, proposal bool) (Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Token{}, ErrClosed
	}
	if s.failure != nil {
		return Token{}, errors.Join(ErrFailed, s.failure)
	}
	if s.state.Sequence == math.MaxUint64 {
		return Token{}, ErrLimit
	}
	seq := s.state.Sequence + 1
	frame, err := storageformat.EncodeFrame(seq, payload)
	if err != nil {
		return Token{}, err
	}
	tx, err := storageformat.DecodeTransaction(payload)
	if err != nil {
		return Token{}, err
	}
	next, err := s.state.next(tx, seq)
	if err != nil {
		return Token{}, err
	}
	limit := MaxWALBytes
	if proposal {
		limit -= ConsensusReserve
	}
	if int64(len(frame)) > limit-s.bytes {
		return Token{}, ErrLimit
	}
	used, err := s.usage(s.root)
	if err != nil {
		s.failure = err
		return Token{}, errors.Join(ErrFailed, err)
	}
	if int64(len(frame))+ControlReserve > MaxDirectoryBytes-used {
		return Token{}, ErrLimit
	}
	if err = storagefs.WriteAll(s.file, frame); err == nil {
		err = s.file.Sync()
	}
	if err != nil {
		s.failure = err
		return Token{}, errors.Join(ErrFailed, err)
	}
	s.state = next
	s.bytes += int64(len(frame))
	return Token{s.generation, seq}, nil
}

// Close waits for in-progress Append, closes the WAL before releasing the lock,
// and retains prior failure. It never syncs an uncertain failed append as success.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.failure
	}
	s.closed = true
	err := errors.Join(s.failure, s.file.Close(), s.owner.Close())
	s.failure = err
	return err
}

func directoryBytes(root string) (int64, error) {
	var total int64
	err := walkFiles(root, func(info os.FileInfo) error {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return storagefs.ErrUnsafe
		}
		if info.Size() < 0 || info.Size() > MaxDirectoryBytes-total {
			return ErrLimit
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// ApplicationVersion is immutable after successful open/bootstrap.
func (s *Store) ApplicationVersion() int {
	if s.applicationVersion == 0 {
		return 1
	}
	return s.applicationVersion
}

// SnapshotImage returns owned validated bytes only for explicit snapshot opens.
// The file is immutable for this generation; appends cannot alter this image.
func (s *Store) SnapshotImage() []byte { return append([]byte(nil), s.snapshotImage...) }
