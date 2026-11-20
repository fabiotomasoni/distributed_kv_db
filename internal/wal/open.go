package wal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

type Identity struct{ ClusterID, NodeID, ConfigHash string }

func identityBytes(id Identity) ([]byte, error) {
	b, _ := json.Marshal(map[string]any{"format_version": 1, "cluster_id": id.ClusterID, "node_id": id.NodeID, "config_hash": id.ConfigHash})
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
func Open(root string, id Identity) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = abs
	if _, err := identityBytes(id); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(root, "CURRENT")); err != nil {
		return nil, errors.Join(ErrNotInitialized, err)
	}
	owner, err := storagefs.Acquire(root)
	if err != nil {
		return nil, err
	}
	s, err := openOwned(root, id, owner)
	if err != nil {
		return nil, errors.Join(err, owner.Close())
	}
	return s, nil
}

func openOwned(root string, id Identity, owner *storagefs.Lock) (*Store, error) {
	expect, err := identityBytes(id)
	if err != nil {
		return nil, err
	}
	identity, err := storagefs.ReadPrivate(filepath.Join(root, "IDENTITY"), storageformat.MaxManifest)
	if err != nil {
		return nil, err
	}
	if err = storageformat.ValidateManifest("IDENTITY", identity); err != nil {
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
	if c.BaseIndex != "0" || c.BaseTerm != "0" {
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
	image, err := storagefs.ReadPrivate(filepath.Join(gen, "snapshot"), storageformat.MaxManifest)
	if err != nil {
		return nil, err
	}
	empty, err := storageformat.EmptySnapshot(id.ClusterID, id.ConfigHash)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(image)
	if !bytes.Equal(image, empty) || hex.EncodeToString(h[:]) != c.SHA {
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
	r, err := Replay(f, 0, 0)
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
	return &Store{root: root, generation: c.Generation, owner: owner, file: f, usage: directoryBytes, state: r.State, bytes: r.ValidBytes, repairedTail: r.TailBytes}, nil
}

func repairTail(f walFile, valid int64) error {
	if err := f.Truncate(valid); err != nil {
		return err
	}
	return f.Sync()
}
