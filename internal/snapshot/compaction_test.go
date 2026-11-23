//go:build darwin

package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

// The A path deliberately uses direct full-prefix M02/M11 application, not
// RecoverCommitted or PrepareCompaction. Its expected rows diagnose differences.
func fullReplayReference(t *testing.T, s wal.State) storageformat.Snapshot {
	t.Helper()
	if s.BaseIndex != 0 {
		t.Fatal("reference requires original full log")
	}
	m, e := NewMachine(cfg(t))
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, m, 0)
	for _, v := range s.CommittedEntries() {
		applied(t, submit(t, a, v))
	}
	term := uint64(0)
	if s.CommitIndex > 0 {
		term = s.Entries[s.CommitIndex-1].Term
	}
	return capture(t, a, s.CommitIndex, term, s.CommitIndex)
}
func recoveredImage(t *testing.T, p *Persistence) storageformat.Snapshot {
	t.Helper()
	d, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	image := readBytes(t, filepath.Join(p.root, d.Generation, "snapshot"))
	r, e := RecoverCommitted(deadline(t), p.cfg, image, d.Log, 1)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, r.Machine, r.LastApplied)
	term := d.Log.BaseTerm
	if d.Log.CommitIndex > d.Log.BaseIndex {
		term = d.Log.Entries[d.Log.CommitIndex-d.Log.BaseIndex-1].Term
	}
	return capture(t, a, r.LastApplied, term, d.Log.CommitIndex)
}
func assertLogicalRows(t *testing.T, want, got storageformat.Snapshot) {
	t.Helper()
	if want.ClusterID != got.ClusterID || want.ConfigHash != got.ConfigHash || want.Index != got.Index || want.Term != got.Term || want.LogicalBytes != got.LogicalBytes {
		t.Fatal("metadata mismatch", want, got)
	}
	if !reflect.DeepEqual(want.KV, got.KV) {
		t.Fatalf("KV rows differ: want=%#v got=%#v", want.KV, got.KV)
	}
	if !reflect.DeepEqual(want.Sessions, got.Sessions) {
		t.Fatalf("session rows differ: want=%#v got=%#v", want.Sessions, got.Sessions)
	}
}
func sourceFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, name := range []string{"IDENTITY", "gen-1/snapshot", "gen-1/wal"} {
		out[name] = readBytes(t, filepath.Join(root, name))
	}
	return out
}
func assertSourceFiles(t *testing.T, root string, want map[string][]byte) {
	t.Helper()
	for name, b := range want {
		if !bytes.Equal(b, readBytes(t, filepath.Join(root, name))) {
			t.Fatal("source file changed", name)
		}
	}
}
func TestIsolatedCompactionEveryCommittedBoundaryAndExactEquivalence(t *testing.T) {
	for index := uint64(0); index <= 10; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			root := seededRoot(t)
			original := sourceFiles(t, root)
			p := diskOwner(t, root)
			before, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			want := fullReplayReference(t, before.Log)
			evidence, e := p.compactIsolated(deadline(t), index)
			if e != nil {
				t.Fatal(e)
			}
			term := uint64(0)
			if index > 0 {
				term = history()[index-1].Term
			}
			if evidence.Before != (Point{}) || evidence.After != (Point{index, term}) || evidence.Generation != "gen-2" || evidence.CommitIndex != 10 || evidence.LastApplied != 10 || evidence.ScheduledApplyIndex != 10 || evidence.LastIndex != 11 || evidence.HardState != (storageformat.HardState{Term: 4, Vote: "n2"}) {
				t.Fatal(evidence)
			}
			for retry := 0; retry < 2; retry++ {
				assertLogicalRows(t, want, recoveredImage(t, p))
			}
			after, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			if after.Log.BaseIndex != index || after.Log.BaseTerm != term || len(after.Log.Entries) != 11-int(index) {
				t.Fatal(after.Log)
			}
			for i, v := range after.Log.Entries {
				old := before.Log.Entries[int(index)+i]
				if v.Index != old.Index || v.Term != old.Term || !bytes.Equal(v.Raw, old.Raw) {
					t.Fatal("suffix changed", v.Index)
				}
			}
			assertSourceFiles(t, root, original)
			if e = p.Close(); e != nil {
				t.Fatal(e)
			}
			p = diskOwner(t, root)
			assertLogicalRows(t, want, recoveredImage(t, p))
		})
	}
}
func TestIsolatedCompactionRepeatedNonzeroBaseAndImmutableGenerations(t *testing.T) {
	root := seededRoot(t)
	p := diskOwner(t, root)
	original := sourceFiles(t, root)
	for _, index := range []uint64{4, 10} {
		if _, e := p.compactIsolated(deadline(t), index); e != nil {
			t.Fatal(e)
		}
	}
	secondSnap := readBytes(t, filepath.Join(root, "gen-2", "snapshot"))
	secondWAL := readBytes(t, filepath.Join(root, "gen-2", "wal"))
	if _, e := p.compactIsolated(deadline(t), 9); !errors.Is(e, ErrBoundary) {
		t.Fatal("backwards base accepted", e)
	}
	if !bytes.Equal(secondSnap, readBytes(t, filepath.Join(root, "gen-2", "snapshot"))) || !bytes.Equal(secondWAL, readBytes(t, filepath.Join(root, "gen-2", "wal"))) {
		t.Fatal("previous compact generation changed")
	}
	assertSourceFiles(t, root, original)
	used, e := diskUsage(root)
	if e != nil || used+wal.ControlReserve > wal.MaxDirectoryBytes {
		t.Fatal(used, e)
	}
	t.Logf("three retained generations: %d apparent bytes plus %d control reserve", used, wal.ControlReserve)
}
func TestPrepareCompactionRejectsUnappliedOrInvalidSources(t *testing.T) {
	root := seededRoot(t)
	p := diskOwner(t, root)
	d, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	image := readBytes(t, filepath.Join(root, d.Generation, "snapshot"))
	oldCurrent := readBytes(t, filepath.Join(root, "CURRENT"))
	for _, mode := range []string{"beyond-commit", "gap", "commit", "term", "raw", "snapshot", "vote", "sequence"} {
		t.Run(mode, func(t *testing.T) {
			state := d.Log
			state.Entries = append([]storageformat.LogEntry(nil), state.Entries...)
			raw := bytes.Clone(image)
			index := uint64(4)
			switch mode {
			case "beyond-commit":
				index = 11
			case "gap":
				state.Entries[5].Index++
			case "commit":
				state.CommitIndex = 12
			case "term":
				state.Entries[3].Term = 3
			case "raw":
				state.Entries[0].Raw = []byte("bad")
			case "snapshot":
				raw[len(raw)-1] ^= 1
			case "vote":
				state.HardState.Vote = "n4"
			case "sequence":
				state.Sequence = 0
			}
			s, e := PrepareCompaction(deadline(t), cfg(t), raw, state, index)
			if e == nil || len(s.KV) != 0 || len(s.Sessions) != 0 || s.Index != 0 {
				t.Fatal("partial candidate escaped", s, e)
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []context.Context{nil, cancelled} {
		if _, e := p.compactIsolated(c, 4); e == nil {
			t.Fatal(e)
		}
	}
	if !bytes.Equal(oldCurrent, readBytes(t, filepath.Join(root, "CURRENT"))) {
		t.Fatal("invalid source published")
	}
	if _, e = os.Stat(filepath.Join(root, "gen-2.tmp")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("invalid source wrote staging")
	}
}
func TestCompactionEquivalenceRejectsChangedKVSessionAndSpeculativeSuffix(t *testing.T) {
	root := seededRoot(t)
	p := diskOwner(t, root)
	d, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	image := readBytes(t, filepath.Join(root, d.Generation, "snapshot"))
	s, e := PrepareCompaction(deadline(t), cfg(t), image, d.Log, 10)
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"kv", "session-sequence", "session-digest", "session-result", "session-index", "vote", "speculative-bytes", "missing-speculative"} {
		t.Run(mode, func(t *testing.T) {
			c := s
			c.KV = append([]storageformat.SnapshotKV(nil), s.KV...)
			c.Sessions = append([]storageformat.SnapshotSession(nil), s.Sessions...)
			switch mode {
			case "kv":
				c.KV[0].Value = []byte("X")
			case "session-sequence":
				c.Sessions[0].Sequence++
			case "session-digest":
				c.Sessions[0].CommandSHA256 = strings.Repeat("f", 64)
			case "session-result":
				c.Sessions[1].Result.Code = "OK"
				c.Sessions[1].Result.Applied = true
			case "session-index":
				c.Sessions[0].Result.Index = 5
			}
			raw := imageBytes(t, c)
			log, e := generationWAL(d.Log, c)
			if e != nil {
				t.Fatal(e)
			}
			after, e := wal.Replay(bytes.NewReader(log), 10, 2)
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "vote":
				after.State.HardState.Vote = "n3"
			case "speculative-bytes":
				after.State.Entries[0] = entry(11, "put", 4, 1, "changed", nil)
			case "missing-speculative":
				after.State.Entries = nil
			}
			e = verifyCompactRecovery(deadline(t), cfg(t), image, d.Log, raw, after.State)
			if !errors.Is(e, ErrEquivalence) {
				t.Fatal("undetected divergence", mode, e)
			}
		})
	}
}
func TestIsolatedCompactionCutpointsPreserveAuthoritativeGeneration(t *testing.T) {
	cuts := []string{"generation.before-create", "snapshot.before-create", "snapshot.written", "snapshot.synced", "wal.before-create", "wal.written", "wal.synced", "generation.before-rename", "generation.renamed", "current.before-publication", "current.before-create", "current.written", "current.synced", "current.before-rename", "current.renamed", "current.directory-synced"}
	for _, cut := range cuts {
		t.Run(cut, func(t *testing.T) {
			root := seededRoot(t)
			original := sourceFiles(t, root)
			p := diskOwner(t, root)
			d, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			want := fullReplayReference(t, d.Log)
			injected := errors.New("cut " + cut)
			p.ops.cut = func(stage string) error {
				if stage == cut {
					return injected
				}
				return nil
			}
			result, e := p.compactIsolated(deadline(t), 7)
			if !errors.Is(e, injected) || result.Generation != "" {
				t.Fatal(result, e)
			}
			_ = p.Close()
			p = diskOwner(t, root)
			d, e = p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			selectedNew := cut == "current.renamed" || cut == "current.directory-synced"
			if (d.Generation == "gen-2") != selectedNew {
				t.Fatal("wrong selection", d.Generation)
			}
			assertLogicalRows(t, want, recoveredImage(t, p))
			assertSourceFiles(t, root, original)
		})
	}
}
func TestIsolatedCompactionIOErrorsQuotaAndRetainedTemporaryArtifacts(t *testing.T) {
	for _, target := range []string{"snapshot", "wal", "CURRENT.tmp"} {
		for _, mode := range []string{"partial", "sync", "close"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				root := seededRoot(t)
				original := sourceFiles(t, root)
				p := diskOwner(t, root)
				current := readBytes(t, filepath.Join(root, "CURRENT"))
				create := p.ops.create
				p.ops.create = func(path string) (durableFile, error) {
					f, e := create(path)
					if e != nil {
						return nil, e
					}
					if filepath.Base(path) == target {
						return &faultyDiskFile{durableFile: f, mode: mode}, nil
					}
					return f, nil
				}
				if r, e := p.compactIsolated(deadline(t), 7); e == nil || r.Generation != "" {
					t.Fatal(r, e)
				}
				if !bytes.Equal(current, readBytes(t, filepath.Join(root, "CURRENT"))) {
					t.Fatal("failed file published")
				}
				assertSourceFiles(t, root, original)
			})
		}
	}
	for _, mode := range []string{"generation-dir", "generation-rename", "root-generation", "current-rename", "root-current"} {
		t.Run(mode, func(t *testing.T) {
			root := seededRoot(t)
			original := sourceFiles(t, root)
			p := diskOwner(t, root)
			syncDir, rename := p.ops.syncDir, p.ops.rename
			roots := 0
			p.ops.syncDir = func(path string) error {
				if path == root {
					roots++
				}
				if mode == "generation-dir" && path != root || mode == "root-generation" && path == root && roots == 1 || mode == "root-current" && path == root && roots == 2 {
					return syscall.EIO
				}
				return syncDir(path)
			}
			p.ops.rename = func(a, b string) error {
				if mode == "generation-rename" && filepath.Base(a) == "gen-2.tmp" || mode == "current-rename" && filepath.Base(a) == "CURRENT.tmp" {
					return syscall.EXDEV
				}
				return rename(a, b)
			}
			if r, e := p.compactIsolated(deadline(t), 7); e == nil || r.Generation != "" {
				t.Fatal(r, e)
			}
			_ = p.Close()
			p = diskOwner(t, root)
			d, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			if (d.Generation == "gen-2") != (mode == "root-current") {
				t.Fatal(d.Generation)
			}
			assertSourceFiles(t, root, original)
		})
	}
	root := seededRoot(t)
	p := diskOwner(t, root)
	original := sourceFiles(t, root)
	usage := p.ops.usage
	p.ops.usage = func(string) (int64, error) { return wal.MaxDirectoryBytes - wal.ControlReserve, nil }
	if _, e := p.compactIsolated(deadline(t), 7); !errors.Is(e, wal.ErrLimit) {
		t.Fatal(e)
	}
	p.ops.usage = usage
	if _, e := os.Stat(filepath.Join(root, "gen-2.tmp")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("quota wrote")
	}
	temp := filepath.Join(root, "CURRENT.tmp")
	if e := os.WriteFile(temp, []byte("evidence"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := p.compactIsolated(deadline(t), 7); !errors.Is(e, os.ErrExist) {
		t.Fatal(e)
	}
	if string(readBytes(t, temp)) != "evidence" {
		t.Fatal("temporary deleted")
	}
	assertSourceFiles(t, root, original)
}
func TestCompactedGenerationCorruptionNeverFallsBackOrRepairsSource(t *testing.T) {
	for _, mode := range []string{"snapshot-truncated", "snapshot-crc", "wal-truncated", "wal-crc", "wal-gap", "wal-boundary-term", "manifest", "current", "profile", "version"} {
		t.Run(mode, func(t *testing.T) {
			root := seededRoot(t)
			original := sourceFiles(t, root)
			p := diskOwner(t, root)
			if _, e := p.compactIsolated(deadline(t), 7); e != nil {
				t.Fatal(e)
			}
			snap := filepath.Join(root, "gen-2", "snapshot")
			log := filepath.Join(root, "gen-2", "wal")
			cur := filepath.Join(root, "CURRENT")
			id := filepath.Join(root, "IDENTITY")
			write := func(path string, b []byte) {
				if e := os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			switch mode {
			case "snapshot-truncated":
				b := readBytes(t, snap)
				write(snap, b[:len(b)-1])
			case "snapshot-crc":
				b := readBytes(t, snap)
				b[len(b)-1] ^= 1
				write(snap, b)
			case "wal-truncated":
				b := readBytes(t, log)
				write(log, b[:len(b)-1])
			case "wal-crc":
				b := readBytes(t, log)
				b[len(b)-1] ^= 1
				write(log, b)
			case "wal-gap":
				first, _ := storageformat.EncodeFrame(1, []byte(`{"commit_index":"7","hard_state":{"current_term":"4","voted_for":"n2"}}`))
				tx, _ := json.Marshal(map[string]any{"replace_suffix": map[string]any{"from_index": "9", "entries": []json.RawMessage{history()[8].Raw}}})
				next, _ := storageformat.EncodeFrame(2, tx)
				write(log, append(first, next...))
			case "wal-boundary-term":
				first, _ := storageformat.EncodeFrame(1, []byte(`{"commit_index":"7","hard_state":{"current_term":"1","voted_for":null}}`))
				write(log, first)
			case "manifest":
				reseal(t, cur, "CURRENT", func(o map[string]any) { o["base_term"] = "3" })
			case "current":
				write(cur, []byte("bad"))
			case "profile":
				b := readBytes(t, id)
				write(id, bytes.Replace(b, []byte(`"format_version":2`), []byte(`"format_version":1`), 1))
			case "version":
				b := readBytes(t, snap)
				b[3] = '2'
				write(snap, b)
			}
			damaged := readBytes(t, log)
			if r, e := p.compactIsolated(deadline(t), 8); e == nil || r.Generation != "" {
				t.Fatal("corruption accepted", r, e)
			}
			if !bytes.Equal(damaged, readBytes(t, log)) {
				t.Fatal("known corruption silently repaired")
			}
			// IDENTITY is intentionally corrupted only in the profile case.
			if mode == "profile" {
				delete(original, "IDENTITY")
			}
			assertSourceFiles(t, root, original)
		})
	}
}
func TestCompactionRealSessionCapacityWithoutEviction(t *testing.T) {
	c := cfg(t)
	empty := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash()}
	source := wal.State{HardState: storageformat.HardState{Term: 2}, Sequence: 2, CommitIndex: 1024}
	for i := uint64(1); i <= 1024; i++ {
		source.Entries = append(source.Entries, entry(i, "put", int(i-1), 1, "A", nil))
	}
	// Validate actual full-log bytes with the existing generator/reader. This
	// fixture is a real 1024-client history, kept entirely in memory.
	log, e := generationWAL(source, empty)
	if e != nil {
		t.Fatal(e)
	}
	r, e := wal.Replay(bytes.NewReader(log), 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	source = r.State
	s, e := PrepareCompaction(deadline(t), c, imageBytes(t, empty), source, 512)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Sessions) != 512 || s.Index != 512 {
		t.Fatal("wrong applied cut")
	}
	compact, e := generationWAL(source, s)
	if e != nil {
		t.Fatal(e)
	}
	r, e = wal.Replay(bytes.NewReader(compact), 512, 2)
	if e != nil {
		t.Fatal(e)
	}
	rebuilt, e := RecoverCommitted(deadline(t), c, imageBytes(t, s), r.State, 1)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, rebuilt.Machine, 1024)
	result, _ := applied(t, submit(t, a, entry(1025, "put", 2048, 1, "blocked", nil)))
	if string(result.Code) != "SESSION_LIMIT" {
		t.Fatal(result)
	}
	result, _ = applied(t, submit(t, a, entry(1026, "put", 0, 2, "known", nil)))
	if result.Code != statemachine.OK {
		t.Fatal(result)
	}
	final := capture(t, a, 1026, 2, 1026)
	if len(final.Sessions) != 1024 {
		t.Fatal("eviction")
	}
}

func TestIsolatedCompactionPostPublicationFailureNeverRollsBack(t *testing.T) {
	for _, mode := range []string{"corrupt", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root := seededRoot(t)
			original := sourceFiles(t, root)
			p := diskOwner(t, root)
			c, cancel := context.WithCancel(deadline(t))
			defer cancel()
			p.ops.cut = func(stage string) error {
				if stage == "current.directory-synced" {
					if mode == "cancel" {
						cancel()
					} else {
						path := filepath.Join(root, "gen-2", "wal")
						b := readBytes(t, path)
						b[len(b)-1] ^= 1
						if e := os.WriteFile(path, b, 0600); e != nil {
							return e
						}
					}
				}
				return nil
			}
			r, e := p.compactIsolated(c, 7)
			if !errors.Is(e, storagefs.ErrUncertain) || r.Generation != "" {
				t.Fatal("post-publication success/rollback", r, e)
			}
			var cur currentManifest
			if e = json.Unmarshal(readBytes(t, filepath.Join(root, "CURRENT")), &cur); e != nil || cur.Generation != "gen-2" {
				t.Fatal("CURRENT rolled back", e)
			}
			assertSourceFiles(t, root, original)
			_ = p.Close()
			if mode == "corrupt" {
				if q, e := OpenPersistence(root, cfg(t)); e == nil || q != nil {
					t.Fatal("corrupt selected generation fell back", e)
				}
			} else {
				q := diskOwner(t, root)
				if recoveredImage(t, q).Index != 10 {
					t.Fatal("cancelled waiter lost committed state")
				}
			}
		})
	}
}
func TestIsolatedCompactionSerializesCloseAndReleasesLockAfterDrain(t *testing.T) {
	root := seededRoot(t)
	p := diskOwner(t, root)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	p.ops.cut = func(stage string) error {
		if stage == "snapshot.written" {
			close(entered)
			<-release
		}
		return nil
	}
	finished, closed := make(chan error, 1), make(chan error, 1)
	go func() { _, e := p.compactIsolated(deadline(t), 7); finished <- e }()
	select {
	case <-entered:
	case <-deadline(t).Done():
		t.Fatal("compaction gate timeout")
	}
	go func() { closed <- p.Close() }()
	if l, e := storagefs.Acquire(root); l != nil || !errors.Is(e, storagefs.ErrLocked) {
		if l != nil {
			_ = l.Close()
		}
		t.Fatal("lock released before drain", e)
	}
	select {
	case e := <-closed:
		t.Fatal("close passed active compaction", e)
	default:
	}
	once.Do(func() { close(release) })
	for _, ch := range []chan error{finished, closed} {
		select {
		case e := <-ch:
			if e != nil {
				t.Fatal(e)
			}
		case <-deadline(t).Done():
			t.Fatal("compaction/close did not finish")
		}
	}
	q := diskOwner(t, root)
	if recoveredImage(t, q).Index != 10 {
		t.Fatal("lost compact state")
	}
}

func TestPrepareCompactionMaximumBoundaryWithoutIndexWrap(t *testing.T) {
	c := cfg(t)
	base := ^uint64(0)
	source := wal.State{BaseIndex: base, BaseTerm: 4, HardState: storageformat.HardState{Term: 4}, CommitIndex: base, Sequence: 1}
	original := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: base, Term: 4}
	candidate, e := PrepareCompaction(deadline(t), c, imageBytes(t, original), source, base)
	if e != nil || candidate.Index != base || candidate.Term != 4 || len(candidate.KV) != 0 || len(candidate.Sessions) != 0 {
		t.Fatal(candidate, e)
	}
}
