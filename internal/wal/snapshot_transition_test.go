package wal

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

// Fixture-only construction, using the previously verified M03 publication
// primitive; the production receiver reuses the Phase B generation writer.
func publishTransitionFixture(t *testing.T, root string, owner *storagefs.Lock, _ State) error {
	t.Helper()
	gen := filepath.Join(root, "gen-2")
	if e := os.Mkdir(gen, 0700); e != nil {
		return e
	}
	for _, name := range []string{"snapshot", "wal"} {
		b, e := os.ReadFile(filepath.Join(root, "gen-1", name))
		if e != nil {
			return e
		}
		if e = writeNew(filepath.Join(gen, name), b); e != nil {
			return e
		}
	}
	if e := storagefs.SyncDirectory(gen); e != nil {
		return e
	}
	if e := storagefs.SyncDirectory(root); e != nil {
		return e
	}
	raw, e := os.ReadFile(filepath.Join(root, "CURRENT"))
	if e != nil {
		return e
	}
	var o map[string]any
	if e = json.Unmarshal(raw, &o); e != nil {
		return e
	}
	delete(o, "checksum")
	o["generation"] = "gen-2"
	raw, _ = json.Marshal(o)
	raw, e = storageformat.SealManifest("CURRENT", raw)
	if e != nil {
		return e
	}
	return owner.PublishCurrent(raw, func(b []byte) error { return storageformat.ValidateManifest("CURRENT", b) })
}
func TestSnapshotTransitionBorrowsStableLockAndContinuesExactTokens(t *testing.T) {
	root := tempRoot(t)
	s, e := BootstrapSessions(root, testIdentity)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	before, e := os.ReadFile(filepath.Join(root, "gen-1", "wal"))
	if e != nil {
		t.Fatal(e)
	}
	e = s.TransitionSnapshot(func(root string, owner *storagefs.Lock, view State) error {
		if l, e := storagefs.Acquire(root); l != nil || !errors.Is(e, storagefs.ErrLocked) {
			if l != nil {
				_ = l.Close()
			}
			return errors.New("lock gap")
		}
		return publishTransitionFixture(t, root, owner, view)
	})
	if e != nil || s.Status().Generation != "gen-2" {
		t.Fatal(e, s.Status())
	}
	token, e := s.Append([]byte(`{"hard_state":{"current_term":"1","voted_for":"n2"}}`))
	if e != nil || token.Generation != "gen-2" || token.Sequence != 2 {
		t.Fatal(token, e)
	}
	after, e := os.ReadFile(filepath.Join(root, "gen-1", "wal"))
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("old file modified", e)
	}
}
func TestSnapshotTransitionErrorsFailClosedWithoutUnlockingOrRollback(t *testing.T) {
	for _, mode := range []string{"callback", "no-publication", "reopen", "old-close"} {
		t.Run(mode, func(t *testing.T) {
			root := tempRoot(t)
			s, e := BootstrapSessions(root, testIdentity)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			sentinel := errors.New("injected old close")
			if mode == "old-close" {
				s.file = &transitionCloseFailure{walFile: s.file, failure: sentinel}
			}
			e = s.TransitionSnapshot(func(root string, owner *storagefs.Lock, view State) error {
				switch mode {
				case "callback":
					return sentinel
				case "no-publication":
					return nil
				case "reopen":
					return os.WriteFile(filepath.Join(root, "CURRENT"), []byte("bad"), 0600)
				default:
					return publishTransitionFixture(t, root, owner, view)
				}
			})
			if e == nil || !s.Status().Failed {
				t.Fatal("false success", e)
			}
			if l, e := storagefs.Acquire(root); l != nil || !errors.Is(e, storagefs.ErrLocked) {
				if l != nil {
					_ = l.Close()
				}
				t.Fatal("failed transition unlocked", e)
			}
			if _, e = s.Append([]byte(`{"commit_index":"0"}`)); !errors.Is(e, ErrFailed) {
				t.Fatal(e)
			}
			if mode == "old-close" && (!errors.Is(e, sentinel) || s.Status().Generation != "gen-2") {
				t.Fatal("published generation rolled back")
			}
			_ = s.Close()
			owner, e := storagefs.Acquire(root)
			if e != nil {
				t.Fatal(e)
			}
			_ = owner.Close()
		})
	}
	legacy, _ := newStore(t)
	if e := legacy.TransitionSnapshot(nil); !errors.Is(e, ErrTransition) {
		t.Fatal(e)
	}
}

type transitionCloseFailure struct {
	walFile
	failure error
}

func (f *transitionCloseFailure) Close() error { return errors.Join(f.walFile.Close(), f.failure) }
