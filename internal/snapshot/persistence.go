//go:build darwin

package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

var ErrGeneration = errors.New("snapshot: invalid generation or persistent metadata")

// DiskRecovery exposes a fresh machine at Point ONLY. Log is validated storage
// metadata, including a possibly committed suffix; no suffix is applied here.
// It grants neither a consensus acknowledgment nor permission to serve requests.
type DiskRecovery struct {
	Machine    *Machine
	Point      Point
	Generation string
	Log        wal.State
}

type durableFile interface {
	io.Writer
	Sync() error
	Close() error
}
type diskOps struct {
	create  func(string) (durableFile, error)
	rename  func(string, string) error
	syncDir func(string) error
	usage   func(string) (int64, error)
	cut     func(string) error
}

// Persistence owns an existing v2 directory exclusively. It is an isolated,
// synchronous tool, with no runtime caller, background worker or WAL append API.
// Do not copy it. Close serializes with all accepted I/O. Cancellation is checked
// before admission only; it cannot cancel or roll back a filesystem syscall.
type Persistence struct {
	mu      sync.Mutex
	root    string
	cfg     config.Config
	lock    *storagefs.Lock
	ops     diskOps
	failure error
	closed  bool
}

// OpenPersistence never bootstraps, migrates, selects an orphan or removes files.
// A caller must explicitly initialize a new private directory using M03 first.
func OpenPersistence(root string, c config.Config) (*Persistence, error) {
	if !c.Valid() || !c.Sessions() {
		return nil, ErrProfile
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// A missing identity must not create even LOCK.
	if _, err = storagefs.ReadPrivate(filepath.Join(root, "IDENTITY"), storageformat.MaxManifest); err != nil {
		return nil, err
	}
	l, err := storagefs.Acquire(root)
	if err != nil {
		return nil, err
	}
	p := &Persistence{root: root, cfg: c, lock: l, ops: diskOps{
		create: func(path string) (durableFile, error) { return storagefs.OpenRegular(path, true) },
		rename: os.Rename, syncDir: storagefs.SyncDirectory, usage: diskUsage,
	}}
	if _, err = p.recover(); err != nil {
		return nil, errors.Join(err, l.Close())
	}
	return p, nil
}
func (p *Persistence) admitted(ctx context.Context) error {
	if ctx == nil {
		return errors.New("snapshot: nil context")
	}
	if p.closed {
		return storagefs.ErrClosed
	}
	if p.failure != nil {
		return errors.Join(storagefs.ErrFailed, p.failure)
	}
	return ctx.Err()
}
func (p *Persistence) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return p.failure
	}
	p.closed = true
	p.failure = errors.Join(p.failure, p.lock.Close())
	return p.failure
}
func (p *Persistence) Recover(ctx context.Context) (DiskRecovery, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.admitted(ctx); err != nil {
		return DiskRecovery{}, err
	}
	r, err := p.recover()
	if err != nil {
		p.failure = err
	}
	return r, err
}

type currentManifest struct {
	Generation string
	SHA        string `json:"snapshot_sha256"`
	Index      string `json:"base_index"`
	Term       string `json:"base_term"`
}

func (p *Persistence) identity() error {
	raw, err := storagefs.ReadPrivate(filepath.Join(p.root, "IDENTITY"), storageformat.MaxManifest)
	if err != nil {
		return err
	}
	if err = storageformat.ValidateSessionIdentity(raw); err != nil {
		return errors.Join(ErrProfile, err)
	}
	data, _ := json.Marshal(map[string]any{"format_version": 2, "cluster_id": p.cfg.ClusterID(), "node_id": p.cfg.NodeID(), "config_hash": p.cfg.Hash()})
	want, err := storageformat.SealSessionIdentity(data)
	if err != nil || !bytes.Equal(raw, want) {
		return ErrProfile
	}
	return nil
}
func privateGen(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return storagefs.ErrUnsafe
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 2 || entries[0].Name() != "snapshot" || entries[1].Name() != "wal" {
		return ErrGeneration
	}
	return nil
}
func (p *Persistence) recover() (DiskRecovery, error) {
	bad := func(err error) (DiskRecovery, error) { return DiskRecovery{}, err }
	if err := p.identity(); err != nil {
		return bad(err)
	}
	// Count every retained file, even orphan/staging, and reject unsafe paths.
	if _, err := p.ops.usage(p.root); err != nil {
		return bad(err)
	}
	raw, err := storagefs.ReadPrivate(filepath.Join(p.root, "CURRENT"), storageformat.MaxManifest)
	if err != nil {
		return bad(err)
	}
	if err = storageformat.ValidateManifest("CURRENT", raw); err != nil {
		return bad(errors.Join(ErrGeneration, err))
	}
	var c currentManifest
	if err = json.Unmarshal(raw, &c); err != nil {
		return bad(err)
	}
	gen := filepath.Join(p.root, c.Generation)
	if err = privateGen(gen); err != nil {
		return bad(err)
	}
	image, err := storagefs.ReadPrivate(filepath.Join(gen, "snapshot"), storageformat.MaxSnapshot)
	if err != nil {
		return bad(err)
	}
	digest := sha256.Sum256(image)
	if hex.EncodeToString(digest[:]) != c.SHA {
		return bad(ErrGeneration)
	}
	state, err := storageformat.DecodeSnapshot(image)
	if err != nil {
		return bad(err)
	}
	if c.Index != strconv.FormatUint(state.Index, 10) || c.Term != strconv.FormatUint(state.Term, 10) {
		return bad(ErrGeneration)
	}
	f, err := storagefs.OpenRegular(filepath.Join(gen, "wal"), false)
	if err != nil {
		return bad(err)
	}
	st, err := f.Stat()
	if err == nil && st.Size() > wal.MaxWALBytes {
		err = wal.ErrLimit
	}
	var replay wal.Recovery
	if err == nil {
		replay, err = wal.Replay(f, state.Index, state.Term)
	}
	// Identity and image must agree before any permitted tail repair.
	if err == nil && (state.ClusterID != p.cfg.ClusterID() || state.ConfigHash != p.cfg.Hash()) {
		err = ErrProfile
	}
	if err == nil && state.Term > replay.State.HardState.Term {
		err = ErrBoundary
	}
	if err == nil && replay.TailBytes > 0 {
		err = f.Truncate(replay.ValidBytes)
	}
	// Surviving complete frames are synchronized, just as in M03 recovery.
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return bad(err)
	}
	machine, point, err := Restore(p.cfg, image, replay.State.HardState.Term)
	if err != nil {
		return bad(err)
	}
	return DiskRecovery{Machine: machine, Point: point, Generation: c.Generation, Log: replay.State}, nil
}

// generationWAL derives bytes from the current, locked WAL view, never from a
// caller-supplied stale vote. It preserves ALL entries S+1..N, including C+1..N.
func generationWAL(source wal.State, s storageformat.Snapshot) ([]byte, error) {
	if s.Index < source.BaseIndex || s.Index > source.CommitIndex {
		return nil, ErrBoundary
	}
	term := source.BaseTerm
	if s.Index > source.BaseIndex {
		offset := s.Index - source.BaseIndex - 1
		if offset >= uint64(len(source.Entries)) {
			return nil, ErrBoundary
		}
		term = source.Entries[offset].Term
	}
	if term != s.Term || s.Term > source.HardState.Term {
		return nil, ErrBoundary
	}
	seq := uint64(0)
	var out []byte
	add := func(tx map[string]any) error {
		b, e := json.Marshal(tx)
		if e != nil {
			return e
		}
		frame, e := storageformat.EncodeFrame(seq+1, b)
		if e != nil {
			return e
		}
		if int64(len(out))+int64(len(frame)) > wal.MaxWALBytes {
			return wal.ErrLimit
		}
		out = append(out, frame...)
		seq++
		return nil
	}
	var vote any
	if source.HardState.Vote != "" {
		vote = source.HardState.Vote
	}
	err := add(map[string]any{"hard_state": map[string]any{"current_term": fmt.Sprint(source.HardState.Term), "voted_for": vote}, "commit_index": fmt.Sprint(s.Index)})
	if err != nil {
		return nil, err
	}
	start := s.Index - source.BaseIndex
	if start > uint64(len(source.Entries)) {
		return nil, ErrBoundary
	}
	entries := source.Entries[start:]
	for len(entries) > 0 {
		size := 0
		n := 0
		for n < len(entries) && n < 16 && size+len(entries[n].Raw) <= 262144 {
			size += len(entries[n].Raw)
			n++
		}
		if n == 0 {
			return nil, wal.ErrLimit
		}
		rows := make([]json.RawMessage, n)
		for i := range rows {
			rows[i] = entries[i].Raw
		}
		if err = add(map[string]any{"replace_suffix": map[string]any{"from_index": fmt.Sprint(entries[0].Index), "entries": rows}}); err != nil {
			return nil, err
		}
		entries = entries[n:]
	}
	if err = add(map[string]any{"commit_index": fmt.Sprint(source.CommitIndex)}); err != nil {
		return nil, err
	}
	replay, err := wal.Replay(bytes.NewReader(out), s.Index, s.Term)
	if err != nil || replay.TailBytes != 0 {
		return nil, errors.Join(ErrGeneration, err)
	}
	return out, nil
}
func diskUsage(root string) (int64, error) {
	var used int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Mode().Perm()&0077 != 0 {
				return storagefs.ErrUnsafe
			}
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ok || st.Nlink != 1 {
			return storagefs.ErrUnsafe
		}
		// Every retained file is charged once and must have a private inode.
		if info.Size() < 0 || info.Size() > wal.MaxDirectoryBytes-used {
			return wal.ErrLimit
		}
		used += info.Size()
		return nil
	})
	return used, err
}
func (p *Persistence) nextGeneration() (string, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return "", err
	}
	var highest uint64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "gen-") {
			continue
		}
		name = strings.TrimSuffix(strings.TrimPrefix(name, "gen-"), ".tmp")
		n, err := strconv.ParseUint(name, 10, 64)
		if err != nil || n == 0 || fmt.Sprint(n) != name {
			return "", ErrGeneration
		}
		if n > highest {
			highest = n
		}
	}
	if highest == math.MaxUint64 {
		return "", wal.ErrLimit
	}
	return "gen-" + fmt.Sprint(highest+1), nil
}
func (p *Persistence) cut(stage string) error {
	if p.ops.cut != nil {
		return p.ops.cut(stage)
	}
	return nil
}
func (p *Persistence) write(path, name string, data []byte) error {
	if err := p.cut(name + ".before-create"); err != nil {
		return err
	}
	f, err := p.ops.create(path)
	if err != nil {
		return err
	}
	err = storagefs.WriteAll(f, data)
	if err == nil {
		err = p.cut(name + ".written")
	}
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = p.cut(name + ".synced")
	}
	return errors.Join(err, f.Close())
}

// Publish consumes an already detached capture. Caller must retain exclusive
// ownership of its buffers until return. The locked source WAL proves boundary
// metadata and supplies final hard-state/commit/suffix; historical application
// correctness still depends on trusted Phase A capture. All old generations and
// failed staging are retained. There is no rollback, cleanup, GC or live switch.
func (p *Persistence) Publish(ctx context.Context, s storageformat.Snapshot) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.admitted(ctx); err != nil {
		return "", err
	}
	image, err := storageformat.EncodeSnapshot(s)
	if err != nil {
		return "", err
	}
	if s.ClusterID != p.cfg.ClusterID() || s.ConfigHash != p.cfg.Hash() {
		return "", ErrProfile
	}
	source, err := p.recover()
	if err != nil {
		p.failure = err
		return "", err
	}
	log, err := generationWAL(source.Log, s)
	if err != nil {
		return "", err
	}
	name, err := p.nextGeneration()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(image)
	data, _ := json.Marshal(map[string]any{"format_version": 1, "generation": name, "snapshot_sha256": hex.EncodeToString(hash[:]), "base_index": fmt.Sprint(s.Index), "base_term": fmt.Sprint(s.Term)})
	current, err := storageformat.SealManifest("CURRENT", data)
	if err != nil {
		return "", err
	}
	used, err := p.ops.usage(p.root)
	if err != nil {
		return "", err
	}
	// Old CURRENT is counted in used; count its temporary replacement too. Keep
	// the entire approved control reserve available, without deleting for space.
	if int64(len(image))+int64(len(log))+int64(len(current))+wal.ControlReserve > wal.MaxDirectoryBytes-used {
		return "", wal.ErrLimit
	}
	if _, err = os.Lstat(filepath.Join(p.root, "CURRENT.tmp")); err == nil {
		return "", os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	fail := func(e error) (string, error) { p.failure = e; return "", e }
	staging := filepath.Join(p.root, name+".tmp")
	if err = p.cut("generation.before-create"); err != nil {
		return fail(err)
	}
	if err = os.Mkdir(staging, 0700); err != nil {
		return fail(err)
	}
	if err = p.write(filepath.Join(staging, "snapshot"), "snapshot", image); err != nil {
		return fail(err)
	}
	// Validate persisted bytes, not just the pre-write buffer.
	persisted, err := storagefs.ReadPrivate(filepath.Join(staging, "snapshot"), storageformat.MaxSnapshot)
	if err != nil {
		return fail(err)
	}
	if !bytes.Equal(persisted, image) {
		return fail(ErrGeneration)
	}
	if err = storageformat.ValidateSnapshot(persisted); err != nil {
		return fail(err)
	}
	if err = p.write(filepath.Join(staging, "wal"), "wal", log); err != nil {
		return fail(err)
	}
	persisted, err = storagefs.ReadPrivate(filepath.Join(staging, "wal"), wal.MaxWALBytes)
	if err != nil {
		return fail(err)
	}
	if !bytes.Equal(persisted, log) {
		return fail(ErrGeneration)
	}
	r, err := wal.Replay(bytes.NewReader(persisted), s.Index, s.Term)
	if err != nil || r.TailBytes != 0 {
		return fail(errors.Join(ErrGeneration, err))
	}
	if err = p.ops.syncDir(staging); err != nil {
		return fail(err)
	}
	if err = p.cut("generation.before-rename"); err != nil {
		return fail(err)
	}
	final := filepath.Join(p.root, name)
	if _, err = os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
		return fail(errors.Join(os.ErrExist, err))
	}
	if err = p.ops.rename(staging, final); err != nil {
		return fail(err)
	}
	if err = p.cut("generation.renamed"); err != nil {
		return fail(err)
	}
	if err = p.ops.syncDir(p.root); err != nil {
		return fail(err)
	}
	if err = p.cut("current.before-publication"); err != nil {
		return fail(err)
	}
	if err = p.write(filepath.Join(p.root, "CURRENT.tmp"), "current", current); err != nil {
		return fail(err)
	}
	persisted, err = storagefs.ReadPrivate(filepath.Join(p.root, "CURRENT.tmp"), storageformat.MaxManifest)
	if err != nil {
		return fail(err)
	}
	if !bytes.Equal(persisted, current) {
		return fail(ErrGeneration)
	}
	if err = storageformat.ValidateManifest("CURRENT", persisted); err != nil {
		return fail(err)
	}
	if err = p.cut("current.before-rename"); err != nil {
		return fail(err)
	}
	if err = p.ops.rename(filepath.Join(p.root, "CURRENT.tmp"), filepath.Join(p.root, "CURRENT")); err != nil {
		return fail(errors.Join(storagefs.ErrUncertain, err))
	}
	if err = p.cut("current.renamed"); err != nil {
		return fail(errors.Join(storagefs.ErrUncertain, err))
	}
	if err = p.ops.syncDir(p.root); err != nil {
		return fail(errors.Join(storagefs.ErrUncertain, err))
	}
	if err = p.cut("current.directory-synced"); err != nil {
		return fail(errors.Join(storagefs.ErrUncertain, err))
	}
	return name, nil
}
