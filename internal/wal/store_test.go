package wal

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

var testIdentity = Identity{"11111111111111111111111111111111", "n1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

func tempRoot(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	if err := os.Chmod(r, 0700); err != nil {
		t.Fatal(err)
	}
	return r
}
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := tempRoot(t)
	s, err := Bootstrap(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, root
}
func closeOK(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapGoldenAppendAndRepeatedRecovery(t *testing.T) {
	s, root := newStore(t)
	f := fixtures(t)
	for path, want := range map[string][]byte{"IDENTITY": []byte(f.Identity), "CURRENT": []byte(f.Current), "gen-1/snapshot": rawHex(t, f.Snapshot.Hex), "gen-1/wal": rawHex(t, f.WAL[0].Hex)} {
		got, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("bootstrap bytes differ", path, err)
		}
	}
	if other, err := Open(root, testIdentity); other != nil || !errors.Is(err, storagefs.ErrLocked) {
		t.Fatal("double owner", err)
	}
	if token, err := s.Append(appendTx(t, 1, 1, true)); err != nil || token.Generation != "gen-1" || token.Sequence != 2 {
		t.Fatal(token, err)
	}
	if token, err := s.Append(appendTx(t, 1, 2, false)); err != nil || token.Sequence != 3 {
		t.Fatal(token, err)
	}
	view := s.State()
	view.Entries[0].Raw[0] = 'X'
	if s.State().Entries[0].Raw[0] == 'X' {
		t.Fatal("state aliases caller")
	}
	closeOK(t, s)
	before, err := os.ReadFile(filepath.Join(root, "gen-1/wal"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		r, err := Open(root, testIdentity)
		if err != nil {
			t.Fatal(err)
		}
		v := r.State()
		if v.Sequence != 3 || v.CommitIndex != 1 || v.LastIndex() != 2 || len(v.CommittedEntries()) != 1 {
			t.Fatal(v)
		}
		closeOK(t, r)
	}
	after, err := os.ReadFile(filepath.Join(root, "gen-1/wal"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("recovery duplicated or rewrote valid frames", err)
	}
}

func TestRecoverRepairsOnlyFinalIncompleteFrame(t *testing.T) {
	s, root := newStore(t)
	closeOK(t, s)
	path := filepath.Join(root, "gen-1/wal")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("QWA")); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status().RepairedTailBytes != 3 || r.Status().Sequence != 1 {
		t.Fatal(r.Status())
	}
	closeOK(t, r)
	r, err = Open(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status().RepairedTailBytes != 0 {
		t.Fatal("repair not idempotent")
	}
	closeOK(t, r)
	// A complete bad frame is never truncated, even if a valid frame follows.
	a, _ := storageformat.EncodeFrame(2, appendTx(t, 1, 1, false))
	a[len(a)-1] ^= 1
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := append(original, a...)
	if err = os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := Open(root, testIdentity); r != nil || !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, bad) {
		t.Fatal("corruption repaired destructively", err)
	}
}

type failingFile struct {
	walFile
	writeErr, syncErr, closeErr error
	short                       bool
}

func (f *failingFile) Write(b []byte) (int, error) {
	if f.writeErr != nil {
		if f.short {
			n, err := f.walFile.Write(b[:5])
			return n, errors.Join(f.writeErr, err)
		}
		return 0, f.writeErr
	}
	return f.walFile.Write(b)
}
func (f *failingFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.walFile.Sync()
}
func (f *failingFile) Close() error { return errors.Join(f.closeErr, f.walFile.Close()) }

func TestWriterFailureUnknownOutcomeAndRestart(t *testing.T) {
	for _, kind := range []string{"partial-write", "sync"} {
		t.Run(kind, func(t *testing.T) {
			s, root := newStore(t)
			broken := &failingFile{walFile: s.file}
			if kind == "sync" {
				broken.syncErr = syscall.EIO
			} else {
				broken.writeErr = syscall.ENOSPC
				broken.short = true
			}
			s.file = broken
			token, err := s.Append(appendTx(t, 1, 1, false))
			if !errors.Is(err, ErrFailed) || token != (Token{}) || s.Status().Sequence != 1 || !s.Status().Failed {
				t.Fatal(token, err, s.Status())
			}
			if _, err = s.Append(appendTx(t, 1, 1, false)); !errors.Is(err, ErrFailed) {
				t.Fatal("failed writer resumed", err)
			}
			if err = s.Close(); err == nil {
				t.Fatal("close hid prior I/O failure")
			}
			r, err := Open(root, testIdentity)
			if err != nil {
				t.Fatal(err)
			}
			defer closeOK(t, r)
			want := uint64(1)
			if kind == "sync" {
				want = 2
			}
			if r.Status().Sequence != want || r.Status().CommitIndex != 0 || len(r.State().CommittedEntries()) != 0 {
				t.Fatal("unknown append treated as committed", r.Status())
			}
		})
	}
}

func TestAdmissionFailureAndReserve(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Append([]byte(`{"commit_index":"99"}`)); !errors.Is(err, ErrTransition) || s.Status().Failed {
		t.Fatal(err)
	}
	actual := s.bytes
	s.bytes = MaxWALBytes - ConsensusReserve - 1
	if _, err := s.AppendProposal(appendTx(t, 1, 1, false)); !errors.Is(err, ErrLimit) || s.Status().Failed {
		t.Fatal(err)
	}
	s.bytes = MaxWALBytes - 1
	if _, err := s.Append(appendTx(t, 1, 1, false)); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	s.bytes = actual
	if _, err := s.AppendProposal(appendTx(t, 1, 1, false)); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryQuotaAndInspectionFailure(t *testing.T) {
	s, root := newStore(t)
	path := filepath.Join(root, "gen-1/wal")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s.usage = func(string) (int64, error) { return MaxDirectoryBytes - ControlReserve, nil }
	if _, err = s.Append(appendTx(t, 1, 1, false)); !errors.Is(err, ErrLimit) || s.Status().Failed {
		t.Fatal("directory reserve lost", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("quota rejection wrote bytes", err)
	}
	s.usage = func(string) (int64, error) { return 0, syscall.EIO }
	if _, err = s.Append(appendTx(t, 1, 1, false)); !errors.Is(err, ErrFailed) || !errors.Is(err, syscall.EIO) {
		t.Fatal("directory I/O error hidden", err)
	}
	if !s.Status().Failed {
		t.Fatal("failed inspection allowed participation")
	}
}

func TestConcurrentAppendAndClose(t *testing.T) {
	s, root := newStore(t)
	start := make(chan struct{})
	results := make(chan error, 9)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Append([]byte(`{"hard_state":{"current_term":"1","voted_for":"n1"}}`))
			results <- err
		}()
	}
	wg.Add(1)
	go func() { defer wg.Done(); <-start; results <- s.Close() }()
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	}
	r, err := Open(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	closeOK(t, r)
}

func TestBootstrapCutpointsDoNotImplicitlyResume(t *testing.T) {
	for _, stage := range []string{"identity.synced", "snapshot.synced", "wal.synced", "generation.directory.synced", "generation.renamed", "generation.published", "current.published"} {
		t.Run(stage, func(t *testing.T) {
			root := tempRoot(t)
			injected := errors.New("cut " + stage)
			s, err := bootstrap(root, testIdentity, func(s string) error {
				if s == stage {
					return injected
				}
				return nil
			})
			if s != nil || !errors.Is(err, injected) {
				t.Fatal(err)
			}
			if stage == "current.published" {
				r, err := Open(root, testIdentity)
				if err != nil {
					t.Fatal(err)
				}
				closeOK(t, r)
			} else if r, err := Open(root, testIdentity); r != nil || err == nil {
				t.Fatal("partial bootstrap opened", err)
			}
			if s, err := Bootstrap(root, testIdentity); s != nil || !errors.Is(err, ErrNotEmpty) {
				t.Fatal("partial directory implicitly overwritten", err)
			}
		})
	}
}

func TestCloseErrorAndRepairFailure(t *testing.T) {
	s, _ := newStore(t)
	s.file = &failingFile{walFile: s.file, closeErr: syscall.EIO}
	if err := s.Close(); !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	base := &memoryFile{}
	f := &repairFailure{memoryFile: base}
	if err := repairTail(f, 0); err == nil || f.synced {
		t.Fatal("failed truncate was synchronized as success")
	}
	if err := repairTail(&failingFile{walFile: &memoryFile{}, syncErr: syscall.EIO}, 0); !errors.Is(err, syscall.EIO) {
		t.Fatal("repair Sync failure hidden", err)
	}
}

type memoryFile struct{ bytes.Buffer }

func (*memoryFile) Seek(int64, int) (int64, error) { return 0, nil }
func (*memoryFile) Sync() error                    { return nil }
func (*memoryFile) Truncate(int64) error           { return nil }
func (*memoryFile) Close() error                   { return nil }

type repairFailure struct {
	*memoryFile
	synced bool
}

func (*repairFailure) Truncate(int64) error { return io.ErrClosedPipe }
func (f *repairFailure) Sync() error        { f.synced = true; return nil }
