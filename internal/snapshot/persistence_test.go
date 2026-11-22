//go:build darwin

package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func history() []storageformat.LogEntry {
	return []storageformat.LogEntry{
		entry(1, "put", 1, 1, "A", nil),
		entry(2, "cas", 2, 1, "B", map[string]any{"exists": true, "value_b64": "QQ=="}),
		entry(3, "cas", 2, 2, "C", map[string]any{"exists": true, "value_b64": "QQ=="}),
		entry(4, "read", 0, 0, "", nil), entry(5, "delete", 3, 1, "", nil),
		entry(6, "put", 1, 2, "D", nil), entry(7, "put", 1, 2, "D", nil),
		entry(8, "put", 1, 1, "A", nil), entry(9, "noop", 0, 0, "", nil),
		entry(10, "read", 0, 0, "", nil), entry(11, "put", 4, 1, "speculative", nil),
	}
}
func diskCaptures(t *testing.T) (storageformat.Snapshot, storageformat.Snapshot) {
	t.Helper()
	m, e := NewMachine(cfg(t))
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, m, 0)
	var first storageformat.Snapshot
	for _, v := range history()[:10] {
		applied(t, submit(t, a, v))
		if v.Index == 4 {
			first = capture(t, a, 4, 2, 10)
		}
	}
	return first, capture(t, a, 10, 2, 10)
}
func seededRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	c := cfg(t)
	s, e := wal.BootstrapSessions(root, wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if e != nil {
		t.Fatal(e)
	}
	rows := []json.RawMessage{}
	for _, v := range history() {
		rows = append(rows, v.Raw)
	}
	raw, _ := json.Marshal(map[string]any{"hard_state": map[string]any{"current_term": "4", "voted_for": "n2"}, "replace_suffix": map[string]any{"from_index": "1", "entries": rows}, "commit_index": "10"})
	if _, e = s.Append(raw); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	return root
}
func diskOwner(t *testing.T, root string) *Persistence {
	t.Helper()
	p, e := OpenPersistence(root, cfg(t))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func imageBytes(t *testing.T, s storageformat.Snapshot) []byte {
	t.Helper()
	b, e := storageformat.EncodeSnapshot(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func verifyRecovery(t *testing.T, r DiskRecovery, s storageformat.Snapshot) {
	t.Helper()
	if r.Machine == nil || r.Point != (Point{s.Index, s.Term}) {
		t.Fatal(r.Point)
	}
	if r.Log.HardState != (storageformat.HardState{Term: 4, Vote: "n2"}) || r.Log.CommitIndex != 10 || r.Log.LastIndex() != 11 || r.Log.BaseIndex != s.Index || r.Log.BaseTerm != s.Term {
		t.Fatal(r.Log)
	}
	a := worker(t, r.Machine, r.Point.Index)
	got := capture(t, a, s.Index, s.Term, 10)
	if !bytes.Equal(imageBytes(t, got), imageBytes(t, s)) {
		t.Fatal("restored state differs / suffix was applied")
	}
}
func TestPersistentCapturePublishRecoverAndPreserveGenerations(t *testing.T) {
	first, second := diskCaptures(t)
	root := seededRoot(t)
	oldSnap := readBytes(t, filepath.Join(root, "gen-1", "snapshot"))
	oldWAL := readBytes(t, filepath.Join(root, "gen-1", "wal"))
	p := diskOwner(t, root)
	var peak int64
	p.ops.cut = func(string) error {
		used, e := diskUsage(root)
		if used > peak {
			peak = used
		}
		return e
	}
	if _, e := OpenPersistence(root, cfg(t)); !errors.Is(e, storagefs.ErrLocked) {
		t.Fatal(e)
	}
	for i, s := range []storageformat.Snapshot{first, second} {
		want := fmt.Sprintf("gen-%d", i+2)
		name, e := p.Publish(deadline(t), s)
		if e != nil || name != want {
			t.Fatal(name, e)
		}
		got := readBytes(t, filepath.Join(root, name, "snapshot"))
		if !bytes.Equal(got, imageBytes(t, s)) {
			t.Fatal("persisted bytes")
		}
		for j := 0; j < 2; j++ {
			r, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			verifyRecovery(t, r, s)
		}
	}
	if peak+wal.ControlReserve > wal.MaxDirectoryBytes {
		t.Fatal("peak exceeded reserved budget", peak)
	}
	t.Logf("retained old+new+staging/control apparent peak: %d bytes, plus %d-byte control reserve", peak, wal.ControlReserve)
	// Later live writes happened before publication of first; its detached bytes
	// remain those of prefix 4, not the later value D at prefix 10.
	if !bytes.Equal(readBytes(t, filepath.Join(root, "gen-2", "snapshot")), imageBytes(t, first)) {
		t.Fatal("capture contaminated")
	}
	if !bytes.Equal(oldSnap, readBytes(t, filepath.Join(root, "gen-1", "snapshot"))) || !bytes.Equal(oldWAL, readBytes(t, filepath.Join(root, "gen-1", "wal"))) {
		t.Fatal("old generation changed")
	}
	if e := p.Close(); e != nil {
		t.Fatal(e)
	}
	// Production recovery guard is deliberately unchanged.
	c := cfg(t)
	if s, e := wal.OpenSessions(root, wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}); s != nil || !errors.Is(e, wal.ErrSnapshot) {
		t.Fatal(e)
	}
	p = diskOwner(t, root)
	r, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, r.Machine, r.Point.Index)
	// Successful and failed last results survive disk, as do stale/conflict rules.
	for _, v := range []storageformat.LogEntry{entry(11, "put", 1, 2, "D", nil), entry(12, "cas", 2, 2, "C", map[string]any{"exists": true, "value_b64": "QQ=="}), entry(13, "put", 1, 1, "A", nil), entry(14, "put", 1, 2, "other", nil)} {
		result, index := applied(t, submit(t, a, v))
		switch v.Index {
		case 11:
			if result.Code != statemachine.OK || index != 6 {
				t.Fatal(result, index)
			}
		case 12:
			if result.Code != statemachine.CompareFailed || index != 3 {
				t.Fatal(result, index)
			}
		case 13:
			if result.Code != committed.StaleSequence {
				t.Fatal(result)
			}
		case 14:
			if result.Code != committed.RequestConflict {
				t.Fatal(result)
			}
		}
	}
}

// Cut hooks stop after completed operations, modeling process interruption with
// OS alive. No rollback or simulated erasure of completed writes is performed.
func TestGenerationPublicationCutMatrix(t *testing.T) {
	first, _ := diskCaptures(t)
	cuts := []string{"generation.before-create", "snapshot.before-create", "snapshot.written", "snapshot.synced", "wal.before-create", "wal.written", "wal.synced", "generation.before-rename", "generation.renamed", "current.before-publication", "current.before-create", "current.written", "current.synced", "current.before-rename", "current.renamed", "current.directory-synced"}
	for _, cut := range cuts {
		t.Run(cut, func(t *testing.T) {
			root := seededRoot(t)
			p := diskOwner(t, root)
			old := readBytes(t, filepath.Join(root, "CURRENT"))
			sentinel := errors.New("interrupted at " + cut)
			p.ops.cut = func(s string) error {
				if s == cut {
					return sentinel
				}
				return nil
			}
			name, e := p.Publish(deadline(t), first)
			selectedNew := cut == "current.renamed" || cut == "current.directory-synced"
			if name != "" || !errors.Is(e, sentinel) || errors.Is(e, storagefs.ErrUncertain) != selectedNew {
				t.Fatal(name, e)
			}
			if _, e = p.Publish(deadline(t), first); !errors.Is(e, storagefs.ErrFailed) {
				t.Fatal("failed owner reused", e)
			}
			_ = p.Close()
			q := diskOwner(t, root)
			r, e := q.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			if selectedNew {
				verifyRecovery(t, r, first)
			} else if r.Point.Index != 0 || !bytes.Equal(old, readBytes(t, filepath.Join(root, "CURRENT"))) {
				t.Fatal("unpublished candidate selected")
			}
			if _, e = os.Stat(filepath.Join(root, "gen-1", "wal")); e != nil {
				t.Fatal(e)
			}
		})
	}
}

type faultyDiskFile struct {
	durableFile
	mode string
}

func (f *faultyDiskFile) Write(b []byte) (int, error) {
	switch f.mode {
	case "partial":
		n, e := f.durableFile.Write(b[:1])
		if e != nil {
			return n, e
		}
		return n, syscall.ENOSPC
	case "zero":
		return 0, nil
	case "short":
		if len(b) > 1 {
			return f.durableFile.Write(b[:len(b)/2])
		}
	case "corrupt":
		b = bytes.Clone(b)
		b[len(b)-1] ^= 1
	}
	return f.durableFile.Write(b)
}
func (f *faultyDiskFile) Sync() error {
	if f.mode == "sync" {
		return syscall.EIO
	}
	return f.durableFile.Sync()
}
func (f *faultyDiskFile) Close() error {
	e := f.durableFile.Close()
	if f.mode == "close" {
		return errors.Join(e, syscall.EBADF)
	}
	return e
}
func TestGenerationIOErrorsAndSuccessfulShortWrites(t *testing.T) {
	first, _ := diskCaptures(t)
	for _, target := range []string{"snapshot", "wal", "CURRENT.tmp"} {
		for _, mode := range []string{"create", "partial", "zero", "sync", "close", "corrupt", "short"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				root := seededRoot(t)
				p := diskOwner(t, root)
				old := readBytes(t, filepath.Join(root, "CURRENT"))
				create := p.ops.create
				p.ops.create = func(path string) (durableFile, error) {
					if filepath.Base(path) == target {
						if mode == "create" {
							return nil, os.ErrPermission
						}
						f, e := create(path)
						if e != nil {
							return nil, e
						}
						return &faultyDiskFile{durableFile: f, mode: mode}, nil
					}
					return create(path)
				}
				name, e := p.Publish(deadline(t), first)
				if mode == "short" {
					if e != nil || name != "gen-2" {
						t.Fatal(name, e)
					}
				} else {
					if e == nil || name != "" {
						t.Fatal("false success")
					}
					if !bytes.Equal(old, readBytes(t, filepath.Join(root, "CURRENT"))) {
						t.Fatal("published on error")
					}
				}
				_ = p.Close()
				q := diskOwner(t, root)
				r, e := q.Recover(deadline(t))
				if e != nil {
					t.Fatal(e)
				}
				if mode == "short" {
					verifyRecovery(t, r, first)
				} else if r.Point.Index != 0 {
					t.Fatal(r.Point)
				}
			})
		}
	}
	for _, mode := range []string{"rename-generation", "rename-current", "sync-generation", "sync-root-generation", "sync-root-current"} {
		t.Run(mode, func(t *testing.T) {
			root := seededRoot(t)
			p := diskOwner(t, root)
			rename, syncDir := p.ops.rename, p.ops.syncDir
			rootCalls := 0
			p.ops.rename = func(a, b string) error {
				if mode == "rename-generation" && filepath.Base(a) == "gen-2.tmp" || mode == "rename-current" && filepath.Base(a) == "CURRENT.tmp" {
					return syscall.EXDEV
				}
				return rename(a, b)
			}
			p.ops.syncDir = func(path string) error {
				if path == root {
					rootCalls++
				}
				if mode == "sync-generation" && path != root || mode == "sync-root-generation" && path == root && rootCalls == 1 || mode == "sync-root-current" && path == root && rootCalls == 2 {
					return syscall.EIO
				}
				return syncDir(path)
			}
			name, e := p.Publish(deadline(t), first)
			if e == nil || name != "" {
				t.Fatal("false success")
			}
			uncertain := mode == "rename-current" || mode == "sync-root-current"
			if errors.Is(e, storagefs.ErrUncertain) != uncertain {
				t.Fatal(e)
			}
			_ = p.Close()
			q := diskOwner(t, root)
			r, e := q.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			if mode == "sync-root-current" {
				verifyRecovery(t, r, first)
			} else if r.Point.Index != 0 {
				t.Fatal(r.Point)
			}
		})
	}
}
func reseal(t *testing.T, path, kind string, mutate func(map[string]any)) {
	t.Helper()
	raw := readBytes(t, path)
	var o map[string]any
	if e := json.Unmarshal(raw, &o); e != nil {
		t.Fatal(e)
	}
	delete(o, "checksum")
	mutate(o)
	raw, _ = json.Marshal(o)
	var sealed []byte
	var e error
	if kind == "IDENTITY" {
		sealed, e = storageformat.SealSessionIdentity(raw)
	} else {
		sealed, e = storageformat.SealManifest(kind, raw)
	}
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, sealed, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestDiskRecoveryRejectsCorruptionWithoutFallback(t *testing.T) {
	first, _ := diskCaptures(t)
	for _, mode := range []string{"current-missing", "current-malformed", "current-version", "current-checksum", "current-noncanonical", "manifest-index", "manifest-term", "manifest-hash", "snapshot-missing", "snapshot-truncated", "snapshot-checksum", "snapshot-version", "snapshot-profile", "snapshot-session", "snapshot-oversize", "wal-missing", "wal-checksum", "wal-empty", "wal-term", "identity-profile", "identity-node", "identity-missing", "generation-extra", "generation-symlink", "file-symlink", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			root := seededRoot(t)
			p := diskOwner(t, root)
			if _, e := p.Publish(deadline(t), first); e != nil {
				t.Fatal(e)
			}
			if e := p.Close(); e != nil {
				t.Fatal(e)
			}
			cur := filepath.Join(root, "CURRENT")
			snap := filepath.Join(root, "gen-2", "snapshot")
			log := filepath.Join(root, "gen-2", "wal")
			id := filepath.Join(root, "IDENTITY")
			write := func(path string, b []byte) {
				if e := os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			remove := func(path string) {
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
			}
			switch mode {
			case "current-missing":
				remove(cur)
			case "current-malformed":
				write(cur, []byte("{"))
			case "current-version":
				write(cur, bytes.Replace(readBytes(t, cur), []byte(`"format_version":1`), []byte(`"format_version":2`), 1))
			case "current-checksum":
				b := readBytes(t, cur)
				b[20] ^= 1
				write(cur, b)
			case "current-noncanonical":
				write(cur, append(readBytes(t, cur), '\n'))
			case "manifest-index":
				reseal(t, cur, "CURRENT", func(o map[string]any) { o["base_index"] = "5" })
			case "manifest-term":
				reseal(t, cur, "CURRENT", func(o map[string]any) { o["base_term"] = "3" })
			case "manifest-hash":
				reseal(t, cur, "CURRENT", func(o map[string]any) { o["snapshot_sha256"] = strings.Repeat("0", 64) })
			case "snapshot-missing":
				remove(snap)
			case "snapshot-truncated":
				b := readBytes(t, snap)
				write(snap, b[:len(b)-1])
			case "snapshot-checksum":
				b := readBytes(t, snap)
				b[len(b)-1] ^= 1
				write(snap, b)
			case "snapshot-version":
				b := readBytes(t, snap)
				b[3] = '2'
				write(snap, b)
			case "snapshot-profile":
				wrong := first
				wrong.ConfigHash = strings.Repeat("f", 64)
				write(snap, imageBytes(t, wrong))
			case "snapshot-session":
				b := readBytes(t, snap)
				raw := bytes.Replace(b[12:len(b)-32], []byte(`"sequence":1`), []byte(`"sequence":0`), 1)
				b = append([]byte(nil), b[:12]...)
				b = append(b, raw...)
				h := sha256.Sum256(b)
				b = append(b, h[:]...)
				write(snap, b)
			case "snapshot-oversize":
				f, e := os.OpenFile(snap, os.O_WRONLY, 0600)
				if e != nil {
					t.Fatal(e)
				}
				e = f.Truncate(storageformat.MaxSnapshot + 1)
				e = errors.Join(e, f.Close())
				if e != nil {
					t.Fatal(e)
				}
			case "wal-missing":
				remove(log)
			case "wal-checksum":
				b := readBytes(t, log)
				b[len(b)-1] ^= 1
				write(log, b)
			case "wal-empty":
				write(log, nil)
			case "wal-term":
				b, _ := storageformat.EncodeFrame(1, []byte(`{"commit_index":"4","hard_state":{"current_term":"1","voted_for":null}}`))
				write(log, b)
			case "identity-profile":
				b := bytes.Replace(readBytes(t, id), []byte(`"format_version":2`), []byte(`"format_version":1`), 1)
				write(id, b)
			case "identity-node":
				reseal(t, id, "IDENTITY", func(o map[string]any) { o["node_id"] = "n2" })
			case "identity-missing":
				remove(id)
			case "generation-extra":
				write(filepath.Join(root, "gen-2", "extra"), []byte("unexpected"))
			case "generation-symlink":
				if e := os.Rename(filepath.Join(root, "gen-2"), filepath.Join(root, "moved")); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink("moved", filepath.Join(root, "gen-2")); e != nil {
					t.Fatal(e)
				}
			case "file-symlink":
				remove(snap)
				if e := os.Symlink(filepath.Join(root, "gen-1", "snapshot"), snap); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(snap, filepath.Join(root, "alias")); e != nil {
					t.Fatal(e)
				}
			}
			// Make CURRENT's whole-file digest agree, so these cases exercise the
			// inner decoder/profile checks rather than only the outer hash comparison.
			if strings.HasPrefix(mode, "snapshot-") && mode != "snapshot-missing" && mode != "snapshot-oversize" {
				h := sha256.Sum256(readBytes(t, snap))
				reseal(t, cur, "CURRENT", func(o map[string]any) { o["snapshot_sha256"] = fmt.Sprintf("%x", h) })
			}
			q, e := OpenPersistence(root, cfg(t))
			if e == nil || q != nil {
				if q != nil {
					_ = q.Close()
				}
				t.Fatal("corruption accepted / old generation fallback", mode)
			}
			// No deletion of the retained valid older generation is allowed on failure.
			if _, e = os.Stat(filepath.Join(root, "gen-1", "wal")); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestPublicationAdmissionQuotaNamingAndShutdown(t *testing.T) {
	first, _ := diskCaptures(t)
	root := seededRoot(t)
	p := diskOwner(t, root)
	before := readBytes(t, filepath.Join(root, "CURRENT"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []context.Context{nil, ctx} {
		if name, e := p.Publish(c, first); e == nil || name != "" {
			t.Fatal(e)
		}
	}
	bad := first
	bad.ClusterID = strings.Repeat("f", 32)
	if _, e := p.Publish(deadline(t), bad); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	bad = first
	bad.Index = 12
	if _, e := p.Publish(deadline(t), bad); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
	bad = first
	bad.Term = 3
	if _, e := p.Publish(deadline(t), bad); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
	usage := p.ops.usage
	p.ops.usage = func(string) (int64, error) { return wal.MaxDirectoryBytes - wal.ControlReserve, nil }
	if _, e := p.Publish(deadline(t), first); !errors.Is(e, wal.ErrLimit) {
		t.Fatal(e)
	}
	p.ops.usage = usage
	if _, e := os.Stat(filepath.Join(root, "gen-2.tmp")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("quota created staging")
	}
	if !bytes.Equal(before, readBytes(t, filepath.Join(root, "CURRENT"))) {
		t.Fatal("admission changed authority")
	}
	// Never reuse an orphan or staging number, nor choose it during recovery.
	if e := os.Mkdir(filepath.Join(root, "gen-9.tmp"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(root, "gen-8"), 0700); e != nil {
		t.Fatal(e)
	}
	if n, e := p.Publish(deadline(t), first); e != nil || n != "gen-10" {
		t.Fatal(n, e)
	}
	if e := os.Mkdir(filepath.Join(root, "gen-18446744073709551615.tmp"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Publish(deadline(t), first); !errors.Is(e, wal.ErrLimit) {
		t.Fatal(e)
	}
	if e := p.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Recover(deadline(t)); !errors.Is(e, storagefs.ErrClosed) {
		t.Fatal(e)
	}
	if _, e := p.Publish(deadline(t), first); !errors.Is(e, storagefs.ErrClosed) {
		t.Fatal(e)
	}
	if _, e := OpenPersistence(root, config.Config{}); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}
func TestPublicationCloseWaitsForAcceptedIO(t *testing.T) {
	first, _ := diskCaptures(t)
	root := seededRoot(t)
	p := diskOwner(t, root)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p.ops.cut = func(s string) error {
		if s == "snapshot.written" {
			close(entered)
			<-release
		}
		return nil
	}
	published := make(chan error, 1)
	closed := make(chan error, 1)
	go func() { _, e := p.Publish(deadline(t), first); published <- e }()
	select {
	case <-entered:
	case <-deadline(t).Done():
		t.Fatal("publication gate")
	}
	go func() { closed <- p.Close() }()
	if _, e := storagefs.Acquire(root); !errors.Is(e, storagefs.ErrLocked) {
		t.Fatal(e)
	}
	select {
	case e := <-closed:
		t.Fatal("close passed active I/O", e)
	default:
	}
	once.Do(func() { close(release) })
	for _, ch := range []chan error{published, closed} {
		select {
		case e := <-ch:
			if e != nil {
				t.Fatal(e)
			}
		case <-deadline(t).Done():
			t.Fatal("join")
		}
	}
	q := diskOwner(t, root)
	r, e := q.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	verifyRecovery(t, r, first)
}
func TestIsolatedTailRepairAndMaxBoundary(t *testing.T) {
	first, _ := diskCaptures(t)
	root := seededRoot(t)
	p := diskOwner(t, root)
	if _, e := p.Publish(deadline(t), first); e != nil {
		t.Fatal(e)
	}
	_ = p.Close()
	path := filepath.Join(root, "gen-2", "wal")
	valid := readBytes(t, path)
	f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.Write([]byte("QW"))
	e = errors.Join(e, f.Close())
	if e != nil {
		t.Fatal(e)
	}
	p = diskOwner(t, root)
	r, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	verifyRecovery(t, r, first)
	if !bytes.Equal(readBytes(t, path), valid) {
		t.Fatal("tail not repaired exactly")
	}
	// MaxUint64 has no S+1. No wrapping index is encoded.
	s := storageformat.Snapshot{Index: math.MaxUint64, Term: 4}
	raw, e := generationWAL(wal.State{BaseIndex: s.Index, BaseTerm: 4, HardState: storageformat.HardState{Term: 4}, CommitIndex: s.Index}, s)
	if e != nil {
		t.Fatal(e)
	}
	rr, e := wal.Replay(bytes.NewReader(raw), s.Index, 4)
	if e != nil || rr.State.LastIndex() != math.MaxUint64 || len(rr.State.Entries) != 0 {
		t.Fatal(e)
	}
}

// Two real process exits corroborate the modeled selection boundary. Only the
// test binary started by this test is exited; no OS/power failure is simulated.
func TestGenerationProcessExitBoundary(t *testing.T) {
	for _, cut := range []string{"current.before-rename", "current.renamed"} {
		t.Run(cut, func(t *testing.T) {
			first, _ := diskCaptures(t)
			root := seededRoot(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGenerationChild$")
			cmd.Env = append(os.Environ(), "QUORUMKV_M13B_ROOT="+root, "QUORUMKV_M13B_CUT="+cut)
			output, e := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 77 {
				t.Fatal(string(output), e)
			}
			p := diskOwner(t, root)
			r, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			if cut == "current.renamed" {
				verifyRecovery(t, r, first)
			} else if r.Point.Index != 0 {
				t.Fatal(r.Point)
			}
		})
	}
}
func TestGenerationChild(t *testing.T) {
	root := os.Getenv("QUORUMKV_M13B_ROOT")
	if root == "" {
		return
	}
	first, _ := diskCaptures(t)
	p := diskOwner(t, root)
	p.ops.cut = func(s string) error {
		if s == os.Getenv("QUORUMKV_M13B_CUT") {
			os.Exit(77)
		}
		return nil
	}
	_, e := p.Publish(deadline(t), first)
	t.Fatal("cut not reached", e)
}

func TestPersistentSessionCapacityAndExplicitArtifactRetention(t *testing.T) {
	root := seededRoot(t)
	p := diskOwner(t, root)
	c := cfg(t)
	s := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: 10, Term: 2}
	for i := 0; i < 1024; i++ {
		s.Sessions = append(s.Sessions, storageformat.SnapshotSession{ClientID: fmt.Sprintf("%032x", i), Sequence: 1, CommandSHA256: strings.Repeat("a", 64), Kind: "put", Result: storageformat.SnapshotResult{Code: "RESOURCE_LIMIT", Index: 1}})
	}
	t.Logf("1024-session persisted image: %d bytes", len(imageBytes(t, s)))
	if _, e := p.Publish(deadline(t), s); e != nil {
		t.Fatal(e)
	}
	r, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Machine.sessions.CopySessions()) != 1024 {
		t.Fatal("lost sessions")
	}
	a := worker(t, r.Machine, 10)
	result, _ := applied(t, submit(t, a, entry(11, "put", 2048, 1, "new", nil)))
	if result.Code != committed.SessionLimit {
		t.Fatal(result)
	}
	result, index := applied(t, submit(t, a, entry(12, "put", 1, 2, "known", nil)))
	if result.Code != statemachine.OK || index != 12 {
		t.Fatal(result, index)
	}
	if e = p.Close(); e != nil {
		t.Fatal(e)
	}
	// An unselected CURRENT.tmp is evidence, never silently deleted/truncated.
	path := filepath.Join(root, "CURRENT.tmp")
	if e = os.WriteFile(path, []byte("retained"), 0600); e != nil {
		t.Fatal(e)
	}
	p = diskOwner(t, root)
	if _, e = p.Publish(deadline(t), s); !errors.Is(e, os.ErrExist) {
		t.Fatal(e)
	}
	if string(readBytes(t, path)) != "retained" {
		t.Fatal("staging removed")
	}
	// Account for sparse apparent bytes, without filling the SSD.
	pad := filepath.Join(root, "quota-probe")
	f, e := os.OpenFile(pad, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	e = f.Truncate(wal.MaxDirectoryBytes)
	e = errors.Join(e, f.Close())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = diskUsage(root); !errors.Is(e, wal.ErrLimit) {
		t.Fatal(e)
	}
}

func TestRecoveryFailureReturnsNoPartialMachineAndPoisonsOwner(t *testing.T) {
	first, _ := diskCaptures(t)
	root := seededRoot(t)
	p := diskOwner(t, root)
	if _, e := p.Publish(deadline(t), first); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "gen-2", "snapshot")
	raw := readBytes(t, path)
	raw[len(raw)-1] ^= 1
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	r, e := p.Recover(deadline(t))
	if e == nil || r.Machine != nil || r.Point != (Point{}) || r.Generation != "" || len(r.Log.Entries) != 0 {
		t.Fatal("partial recovery escaped", r, e)
	}
	if _, e = p.Recover(deadline(t)); !errors.Is(e, storagefs.ErrFailed) {
		t.Fatal(e)
	}
}
