package wal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

const initialTransaction = `{"commit_index":"0","hard_state":{"current_term":"0","voted_for":null}}`

// Bootstrap initializes only a pre-existing empty private directory, explicitly.
// Any failure retains files for diagnosis; ordinary Open never resumes bootstrap.
func Bootstrap(root string, id Identity) (*Store, error) { return bootstrap(root, id, nil) }

func bootstrap(root string, id Identity, cut func(string) error) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = abs
	identity, err := identityBytes(id)
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if len(files) != 0 {
		return nil, ErrNotEmpty
	}
	owner, err := storagefs.Acquire(root)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Store, error) { return nil, errors.Join(err, owner.Close()) }
	files, err = os.ReadDir(root)
	if err != nil {
		return fail(err)
	}
	if len(files) != 1 || files[0].Name() != "LOCK" {
		return fail(ErrNotEmpty)
	}
	hook := func(stage string) error {
		if cut != nil {
			return cut(stage)
		}
		return nil
	}
	if err = writeNew(filepath.Join(root, "IDENTITY"), identity); err != nil {
		return fail(err)
	}
	if err = storagefs.SyncDirectory(root); err != nil {
		return fail(err)
	}
	if err = hook("identity.synced"); err != nil {
		return fail(err)
	}
	staging := filepath.Join(root, "gen-1.tmp")
	if err = os.Mkdir(staging, 0700); err != nil {
		return fail(err)
	}
	image, err := storageformat.EmptySnapshot(id.ClusterID, id.ConfigHash)
	if err != nil {
		return fail(err)
	}
	if err = writeNew(filepath.Join(staging, "snapshot"), image); err != nil {
		return fail(err)
	}
	if err = hook("snapshot.synced"); err != nil {
		return fail(err)
	}
	frame, err := storageformat.EncodeFrame(1, []byte(initialTransaction))
	if err != nil {
		return fail(err)
	}
	if err = writeNew(filepath.Join(staging, "wal"), frame); err != nil {
		return fail(err)
	}
	if err = hook("wal.synced"); err != nil {
		return fail(err)
	}
	if err = storagefs.SyncDirectory(staging); err != nil {
		return fail(err)
	}
	if err = hook("generation.directory.synced"); err != nil {
		return fail(err)
	}
	if err = os.Rename(staging, filepath.Join(root, "gen-1")); err != nil {
		return fail(err)
	}
	if err = hook("generation.renamed"); err != nil {
		return fail(err)
	}
	if err = storagefs.SyncDirectory(root); err != nil {
		return fail(err)
	}
	if err = hook("generation.published"); err != nil {
		return fail(err)
	}
	h := sha256.Sum256(image)
	data, _ := json.Marshal(map[string]any{"format_version": 1, "generation": "gen-1", "snapshot_sha256": hex.EncodeToString(h[:]), "base_index": "0", "base_term": "0"})
	current, err := storageformat.SealManifest("CURRENT", data)
	if err != nil {
		return fail(err)
	}
	if err = owner.PublishCurrent(current, func(b []byte) error { return storageformat.ValidateManifest("CURRENT", b) }); err != nil {
		return fail(err)
	}
	if err = hook("current.published"); err != nil {
		return fail(err)
	}
	s, err := openOwned(root, id, owner)
	if err != nil {
		return fail(err)
	}
	return s, nil
}

func writeNew(path string, data []byte) error {
	f, err := storagefs.OpenRegular(path, true)
	if err != nil {
		return err
	}
	if err = storagefs.WriteAll(f, data); err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}
