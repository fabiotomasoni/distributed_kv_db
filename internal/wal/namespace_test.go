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

func TestCurrentSelectsOnlyPublishedGeneration(t *testing.T) {
	s, root := newStore(t)
	closeOK(t, s)
	// Construct two tiny candidate directories in the harness, not a production
	// snapshot/rotation path. Both use the approved empty checkpoint.
	for _, name := range []string{"gen-2", "gen-3.tmp"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		image, err := os.ReadFile(filepath.Join(root, "gen-1/snapshot"))
		if err != nil {
			t.Fatal(err)
		}
		if err = writeNew(filepath.Join(dir, "snapshot"), image); err != nil {
			t.Fatal(err)
		}
		frame, err := storageformat.EncodeFrame(1, payload(t, map[string]any{"hard_state": hard(2, "n2"), "commit_index": "0"}))
		if err != nil {
			t.Fatal(err)
		}
		if err = writeNew(filepath.Join(dir, "wal"), frame); err != nil {
			t.Fatal(err)
		}
		if err = storagefs.SyncDirectory(dir); err != nil {
			t.Fatal(err)
		}
	}
	before, err := Open(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status().Generation != "gen-1" {
		t.Fatal("unpublished generation selected")
	}
	closeOK(t, before)
	owner, err := storagefs.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	var current map[string]any
	if err = json.Unmarshal([]byte(fixtures(t).Current), &current); err != nil {
		t.Fatal(err)
	}
	delete(current, "checksum")
	current["generation"] = "gen-2"
	manifest, err := storageformat.SealManifest("CURRENT", payload(t, current))
	if err != nil {
		t.Fatal(err)
	}
	if err = storagefs.SyncDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err = owner.PublishCurrent(manifest, func(b []byte) error { return storageformat.ValidateManifest("CURRENT", b) }); err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status().Generation != "gen-2" || r.Status().Term != 2 {
		t.Fatal("wrong manifest selected", r.Status())
	}
	closeOK(t, r)
	// Valid old gen-1 must never hide corruption of selected gen-2.
	path := filepath.Join(root, "gen-2/wal")
	bad, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad[len(bad)-1] ^= 1
	if err = os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := Open(root, testIdentity); r != nil || !errors.Is(err, ErrCorrupt) {
		t.Fatal("fell back to older generation", err)
	}
}

func TestLockOnlyBootstrapIsNotFresh(t *testing.T) {
	root := tempRoot(t)
	if err := writeNew(filepath.Join(root, "LOCK"), nil); err != nil {
		t.Fatal(err)
	}
	if s, err := Bootstrap(root, testIdentity); s != nil || !errors.Is(err, ErrNotEmpty) {
		t.Fatal("partial bootstrap erased", err)
	}
	if s, err := Open(root, testIdentity); s != nil || !errors.Is(err, ErrNotInitialized) {
		t.Fatal(err)
	}
}

func TestMetadataMismatchAndNoImplicitBootstrap(t *testing.T) {
	root := tempRoot(t)
	if s, err := Open(root, testIdentity); s != nil || !errors.Is(err, ErrNotInitialized) {
		t.Fatal(err)
	}
	s, root := newStore(t)
	closeOK(t, s)
	other := testIdentity
	other.NodeID = "n2"
	if s, err := Open(root, other); s != nil || !errors.Is(err, ErrCorrupt) {
		t.Fatal("identity mismatch accepted", err)
	}
	imagePath := filepath.Join(root, "gen-1/snapshot")
	before, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	bad := bytes.Clone(before)
	bad[len(bad)-1] ^= 1
	if err = os.WriteFile(imagePath, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(root, testIdentity); s != nil || !errors.Is(err, ErrCorrupt) {
		t.Fatal("corrupt checkpoint accepted", err)
	}
	if err = os.Remove(filepath.Join(root, "CURRENT")); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(root, testIdentity); s != nil || !errors.Is(err, ErrNotInitialized) {
		t.Fatal("missing CURRENT rebuilt implicitly", err)
	}
}
