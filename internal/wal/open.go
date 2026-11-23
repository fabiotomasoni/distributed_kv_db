package wal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

type Identity struct{ ClusterID, NodeID, ConfigHash string }

func identityBytes(id Identity) ([]byte, error) { return identityBytesVersion(id, 1) }
func identityBytesVersion(id Identity, version int) ([]byte, error) {
	b, _ := json.Marshal(map[string]any{"format_version": version, "cluster_id": id.ClusterID, "node_id": id.NodeID, "config_hash": id.ConfigHash})
	if version == 2 {
		return storageformat.SealSessionIdentity(b)
	}
	if version != 1 {
		return nil, ErrCorrupt
	}
	return storageformat.SealManifest("IDENTITY", b)
}

func walkFiles(root string, visit func(os.FileInfo) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			if info.Mode().Perm()&0077 != 0 {
				return storagefs.ErrUnsafe
			}
			return nil
		}
		return visit(info)
	})
}

// Open validates identity, CURRENT and the selected generation under the lock.
// It never creates a cluster, selects an old generation or applies an entry.
func Open(root string, id Identity) (*Store, error) { return openVersion(root, id, 1) }

// OpenSessions requires an IDENTITY v2 empty-origin session profile (ADR-010).
func OpenSessions(root string, id Identity) (*Store, error) { return openVersion(root, id, 2) }

// OpenSnapshotSessions explicitly enables the approved v2 base-S generation
// reader. Default Open/OpenSessions keep their base-zero compatibility guards.
func OpenSnapshotSessions(root string, id Identity) (*Store, error) {
	return openSnapshotVersion(root, id, 2, true)
}
func openVersion(root string, id Identity, version int) (*Store, error) {
	return openSnapshotVersion(root, id, version, false)
}
func openSnapshotVersion(root string, id Identity, version int, snapshots bool) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = abs
	if _, err := identityBytesVersion(id, version); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(root, "CURRENT")); err != nil {
		return nil, errors.Join(ErrNotInitialized, err)
	}
	owner, err := storagefs.Acquire(root)
	if err != nil {
		return nil, err
	}
	s, err := openOwnedSnapshotVersion(root, id, owner, version, snapshots)
	if err != nil {
		return nil, errors.Join(err, owner.Close())
	}
	return s, nil
}

func openOwned(root string, id Identity, owner *storagefs.Lock) (*Store, error) {
	return openOwnedVersion(root, id, owner, 1)
}
func openOwnedVersion(root string, id Identity, owner *storagefs.Lock, version int) (*Store, error) {
	return openOwnedSnapshotVersion(root, id, owner, version, false)
}
func openOwnedSnapshotVersion(root string, id Identity, owner *storagefs.Lock, version int, snapshots bool) (*Store, error) {
	expect, err := identityBytesVersion(id, version)
	if err != nil {
		return nil, err
	}
	identity, err := storagefs.ReadPrivate(filepath.Join(root, "IDENTITY"), storageformat.MaxManifest)
	if err != nil {
		return nil, err
	}
	if version == 2 {
		err = storageformat.ValidateSessionIdentity(identity)
	} else {
		err = storageformat.ValidateManifest("IDENTITY", identity)
	}
	if err != nil {
		return nil, errors.Join(ErrCorrupt, err)
	}
	if !bytes.Equal(identity, expect) {
		return nil, ErrCorrupt
	}
	current, err := storagefs.ReadPrivate(filepath.Join(root, "CURRENT"), storageformat.MaxManifest)
	if err != nil {
		return nil, err
	}
	if err = storageformat.ValidateManifest("CURRENT", current); err != nil {
		return nil, errors.Join(ErrCorrupt, err)
	}
	var c struct {
		Generation string
		SHA        string `json:"snapshot_sha256"`
		BaseIndex  string `json:"base_index"`
		BaseTerm   string `json:"base_term"`
	}
	if err = json.Unmarshal(current, &c); err != nil {
		return nil, err
	}
	if !snapshots && (c.BaseIndex != "0" || c.BaseTerm != "0") {
		return nil, ErrSnapshot
	}
	gen := filepath.Join(root, c.Generation)
	st, err := os.Lstat(gen)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return nil, storagefs.ErrUnsafe
	}
	limit := int64(storageformat.MaxManifest)
	if snapshots {
		limit = storageformat.MaxSnapshot
		files, e := os.ReadDir(gen)
		if e != nil {
			return nil, e
		}
		if len(files) != 2 || files[0].Name() != "snapshot" || files[1].Name() != "wal" {
			return nil, ErrCorrupt
		}
	}
	image, err := storagefs.ReadPrivate(filepath.Join(gen, "snapshot"), limit)
	if err != nil {
		return nil, err
	}
	empty, err := storageformat.EmptySnapshot(id.ClusterID, id.ConfigHash)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(image)
	var snapshot storageformat.Snapshot
	if snapshots {
		snapshot, err = storageformat.DecodeSnapshot(image)
		if err != nil {
			return nil, errors.Join(ErrCorrupt, err)
		}
		if snapshot.ClusterID != id.ClusterID || snapshot.ConfigHash != id.ConfigHash || c.BaseIndex != fmt.Sprint(snapshot.Index) || c.BaseTerm != fmt.Sprint(snapshot.Term) {
			return nil, ErrCorrupt
		}
	} else if !bytes.Equal(image, empty) {
		return nil, ErrCorrupt
	}
	if hex.EncodeToString(h[:]) != c.SHA {
		return nil, ErrCorrupt
	}
	if _, err = directoryBytes(root); err != nil {
		return nil, err
	}
	f, err := storagefs.OpenRegular(filepath.Join(gen, "wal"), false)
	if err != nil {
		return nil, err
	}
	failed := func(err error) (*Store, error) { return nil, errors.Join(err, f.Close()) }
	st, err = f.Stat()
	if err != nil {
		return failed(err)
	}
	if st.Size() > MaxWALBytes {
		return failed(ErrLimit)
	}
	r, err := Replay(f, snapshot.Index, snapshot.Term)
	if err != nil {
		return failed(err)
	}
	if r.TailBytes > 0 {
		if err = repairTail(f, r.ValidBytes); err != nil {
			return failed(err)
		}
	}
	// Re-synchronize complete surviving non-Sync frames before counting local
	// storage as recovered-durable. Their survival does not prove a prior ack.
	if r.TailBytes == 0 {
		if err = f.Sync(); err != nil {
			return failed(err)
		}
	}
	if _, err = f.Seek(r.ValidBytes, io.SeekStart); err != nil {
		return failed(err)
	}
	var retained []byte
	if snapshots {
		retained = bytes.Clone(image)
	}
	return &Store{identity: id, snapshotImage: retained, applicationVersion: version, root: root, generation: c.Generation, owner: owner, file: f, usage: directoryBytes, state: r.State, bytes: r.ValidBytes, repairedTail: r.TailBytes}, nil
}

func repairTail(f walFile, valid int64) error {
	if err := f.Truncate(valid); err != nil {
		return err
	}
	return f.Sync()
}
