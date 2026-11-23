package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func combinedSource(t *testing.T) ([]byte, wal.State, storageformat.Snapshot) {
	t.Helper()
	first, want := diskCaptures(t)
	root := seededRoot(t)
	p := diskOwner(t, root)
	if _, e := p.Publish(deadline(t), first); e != nil {
		t.Fatal(e)
	}
	r, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	return imageBytes(t, first), r.Log, want
}
func TestCombinedRecoveryOnlyCommittedSuffixAndRepeatedFreshState(t *testing.T) {
	image, source, want := combinedSource(t)
	for j := 0; j < 2; j++ {
		r, e := RecoverCommitted(deadline(t), cfg(t), image, source, 2)
		if e != nil {
			t.Fatal(e)
		}
		if r.Snapshot != (Point{4, 2}) || r.LastApplied != 10 || r.ScheduledApplyIndex != 10 {
			t.Fatal(r)
		}
		a := worker(t, r.Machine, 10)
		got := capture(t, a, 10, 2, 10)
		if !bytes.Equal(imageBytes(t, got), imageBytes(t, want)) {
			t.Fatal("suffix missing, prefix replayed, or speculative entry applied")
		}
	}
}
func TestCombinedRecoveryFullPrefixSkipsCASAlreadyInSnapshot(t *testing.T) {
	// Snapshot S=3 contains a successful CAS at 2 and a failed CAS at 3.
	// Retry of CAS(2) in suffix must retain index 3 even after another client
	// changes x back to the expected A. Replaying prefix 1..3 is forbidden.
	c := cfg(t)
	m, _ := NewMachine(c)
	a := worker(t, m, 0)
	for _, e := range history()[:3] {
		applied(t, submit(t, a, e))
	}
	s := capture(t, a, 3, 2, 3)
	entries := append([]storageformat.LogEntry(nil), history()[:3]...)
	entries = append(entries, entry(4, "put", 3, 1, "A", nil), entry(5, "cas", 2, 2, "C", map[string]any{"exists": true, "value_b64": "QQ=="}), entry(6, "read", 0, 0, "", nil), entry(7, "put", 4, 1, "uncommitted", nil))
	rows := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		rows[i] = e.Raw
	}
	tx, _ := json.Marshal(map[string]any{"hard_state": map[string]any{"current_term": "4", "voted_for": "n2"}, "commit_index": "0"})
	initial, _ := storageformat.EncodeFrame(1, tx)
	tx, _ = json.Marshal(map[string]any{"replace_suffix": map[string]any{"from_index": "1", "entries": rows}, "commit_index": "6"})
	frame, _ := storageformat.EncodeFrame(2, tx)
	log, e := wal.Replay(bytes.NewReader(append(initial, frame...)), 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	r, e := RecoverCommitted(deadline(t), c, imageBytes(t, s), log.State, 1)
	if e != nil {
		t.Fatal(e)
	}
	w := worker(t, r.Machine, 6)
	got := capture(t, w, 6, 2, 6)
	if string(got.KV[0].Value) != "A" || got.Sessions[1].Result.Index != 3 || got.Sessions[1].Result.Code != "COMPARE_FAILED" {
		t.Fatal(got)
	}
	result, index := applied(t, submit(t, w, entry(7, "cas", 2, 2, "C", map[string]any{"exists": true, "value_b64": "QQ=="})))
	if string(result.Code) != "COMPARE_FAILED" || index != 3 {
		t.Fatal(result, index)
	}
}
func TestCombinedRecoveryRejectsInvalidMetadataAndNoPartialResult(t *testing.T) {
	image, source, _ := combinedSource(t)
	for _, mode := range []string{"commit-before-snapshot", "commit-after-log", "gap", "boundary-term", "hard-term", "vote", "raw-entry", "raw-index", "snapshot", "missing-suffix", "base-overflow", "sequence"} {
		t.Run(mode, func(t *testing.T) {
			s := source
			s.Entries = append([]storageformat.LogEntry(nil), source.Entries...)
			img := bytes.Clone(image)
			switch mode {
			case "commit-before-snapshot":
				s.CommitIndex = 3
			case "commit-after-log":
				s.CommitIndex = 12
			case "gap":
				s.Entries[0].Index++
			case "boundary-term":
				s.BaseTerm++
			case "hard-term":
				s.HardState.Term = 1
			case "vote":
				s.HardState.Vote = "other"
			case "raw-entry":
				s.Entries[0].Raw = []byte("bad")
			case "raw-index":
				s.Entries[0].Raw = history()[0].Raw
			case "snapshot":
				img[len(img)-1] ^= 1
			case "missing-suffix":
				s.Entries = s.Entries[:1]
			case "base-overflow":
				s.BaseIndex = math.MaxUint64
			case "sequence":
				s.Sequence = 0
			}
			r, e := RecoverCommitted(deadline(t), cfg(t), img, s, 2)
			if e == nil || r.Machine != nil || r.LastApplied != 0 {
				t.Fatal(r, e)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{ctx, nil} {
		if r, e := RecoverCommitted(ctx, cfg(t), image, source, 1); e == nil || r.Machine != nil {
			t.Fatal(e)
		}
	}
	if _, e := RecoverCommitted(deadline(t), cfg(t), image, source, 0); e == nil {
		t.Fatal("capacity")
	}
}
func TestCombinedRecoveryEmptyAndMaximumBoundary(t *testing.T) {
	c := cfg(t)
	for _, base := range []uint64{0, math.MaxUint64} {
		term := uint64(0)
		if base > 0 {
			term = 4
		}
		image := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: base, Term: term}
		s := wal.State{BaseIndex: base, BaseTerm: term, HardState: storageformat.HardState{Term: term}, CommitIndex: base, Sequence: 1}
		r, e := RecoverCommitted(deadline(t), c, imageBytes(t, image), s, 1)
		if e != nil || r.LastApplied != base {
			t.Fatal(r, e)
		}
	}
}
func TestCombinedRecoverySessionCapacityPreserved(t *testing.T) {
	c := cfg(t)
	s := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: 1, Term: 1}
	for i := 0; i < committed.MaxSessions; i++ {
		s.Sessions = append(s.Sessions, storageformat.SnapshotSession{ClientID: fmt.Sprintf("%032x", i), Sequence: 1, CommandSHA256: string(bytes.Repeat([]byte("a"), 64)), Kind: "put", Result: storageformat.SnapshotResult{Code: "RESOURCE_LIMIT", Index: 1}})
	}
	state := wal.State{BaseIndex: 1, BaseTerm: 1, HardState: storageformat.HardState{Term: 2}, CommitIndex: 2, Sequence: 2, Entries: []storageformat.LogEntry{entry(2, "put", 2048, 1, "blocked", nil)}}
	r, e := RecoverCommitted(deadline(t), c, imageBytes(t, s), state, 1)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, r.Machine, 2)
	got := capture(t, a, 2, 1, 2)
	if len(got.KV) != 0 || len(got.Sessions) != 1024 {
		t.Fatal("quota lost")
	}
}

func TestCombinedRecoveryPublicationCutsSelectOnlyCurrent(t *testing.T) {
	first, want := diskCaptures(t)
	for _, cut := range []string{"generation.before-create", "snapshot.written", "snapshot.synced", "generation.renamed", "current.before-publication", "current.before-rename", "current.renamed"} {
		t.Run(cut, func(t *testing.T) {
			root := seededRoot(t)
			p := diskOwner(t, root)
			p.ops.cut = func(stage string) error {
				if stage == cut {
					return ErrGeneration
				}
				return nil
			}
			if _, e := p.Publish(deadline(t), first); e == nil {
				t.Fatal("cut not reached")
			}
			_ = p.Close()
			p = diskOwner(t, root)
			d, e := p.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			image := readBytes(t, filepath.Join(root, d.Generation, "snapshot"))
			r, e := RecoverCommitted(deadline(t), cfg(t), image, d.Log, 1)
			if e != nil {
				t.Fatal(e)
			}
			a := worker(t, r.Machine, r.LastApplied)
			s := capture(t, a, 10, 2, 10)
			if !bytes.Equal(imageBytes(t, s), imageBytes(t, want)) {
				t.Fatal("wrong post-cut state")
			}
			if (d.Generation == "gen-2") != (cut == "current.renamed") {
				t.Fatal(d.Generation)
			}
		})
	}
}
func TestCombinedRecoverySuccessfulSuffixCASAndDelete(t *testing.T) {
	c := cfg(t)
	empty := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash()}
	entries := []storageformat.LogEntry{entry(1, "put", 1, 1, "A", nil), entry(2, "cas", 2, 1, "B", map[string]any{"exists": true, "value_b64": "QQ=="}), entry(3, "delete", 3, 1, "", nil), entry(4, "read", 0, 0, "", nil)}
	state := wal.State{HardState: storageformat.HardState{Term: 2}, CommitIndex: 4, Sequence: 2, Entries: entries}
	r, e := RecoverCommitted(deadline(t), c, imageBytes(t, empty), state, 1)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, r.Machine, 4)
	s := capture(t, a, 4, 2, 4)
	if len(s.KV) != 0 || len(s.Sessions) != 3 || !s.Sessions[1].Result.Applied || !s.Sessions[2].Result.Deleted {
		t.Fatal(s)
	}
}
