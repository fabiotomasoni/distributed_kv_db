//go:build darwin

package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func TestInstallPlanPreservesCommittedSuffixOrRejectsConflict(t *testing.T) {
	image, _ := diskCaptures(t)
	raw := imageBytes(t, image)
	root := seededRoot(t)
	p := diskOwner(t, root)
	d, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	next, e := PlanInstallation(cfg(t), raw, d.Log, 2, 4)
	if e != nil || next.BaseIndex != 4 || next.BaseTerm != 2 || next.CommitIndex != 10 || next.LastIndex() != 11 || next.HardState != d.Log.HardState {
		t.Fatal(next, e)
	}
	for i, v := range next.Entries {
		if !bytes.Equal(v.Raw, d.Log.Entries[i+4].Raw) {
			t.Fatal("suffix changed")
		}
	}
	local := d.Log
	local.Entries = append([]storageformat.LogEntry(nil), local.Entries...)
	for i := 2; i < len(local.Entries); i++ {
		var o map[string]any
		_ = json.Unmarshal(local.Entries[i].Raw, &o)
		o["term"] = "3"
		local.Entries[i].Raw, _ = json.Marshal(o)
		local.Entries[i].Term = 3
	}
	if _, e = PlanInstallation(cfg(t), raw, local, 2, 4); !errors.Is(e, ErrBoundary) {
		t.Fatal("committed conflict accepted", e)
	}
	local.CommitIndex = 2
	next, e = PlanInstallation(cfg(t), raw, local, 2, 4)
	if e != nil || next.CommitIndex != 4 || next.LastIndex() != 4 || len(next.Entries) != 0 || next.HardState != local.HardState {
		t.Fatal(next, e)
	}
	local = d.Log
	local.Entries = local.Entries[:2]
	local.CommitIndex = 2
	next, e = PlanInstallation(cfg(t), raw, local, 2, 4)
	if e != nil || next.LastIndex() != 4 {
		t.Fatal(next, e)
	}
	if _, e = PlanInstallation(cfg(t), raw, d.Log, 10, 4); !errors.Is(e, ErrObsolete) {
		t.Fatal(e)
	}
	if _, e = PlanInstallation(cfg(t), raw, d.Log, 2, 3); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
	local = d.Log
	local.CommitIndex = 12
	if _, e = PlanInstallation(cfg(t), raw, local, 2, 4); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
}
func TestReceivedPublicationCutMatrixRetainsBothGenerations(t *testing.T) {
	image, _ := diskCaptures(t)
	raw := imageBytes(t, image)
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
			injected := errors.New("received-generation " + cut)
			p.ops.cut = func(stage string) error {
				if stage == cut {
					return injected
				}
				return nil
			}
			if generation, e := p.installReceivedOwned(deadline(t), d.Log, raw, 2, 4); !errors.Is(e, injected) || generation != "" {
				t.Fatal(generation, e)
			}
			_ = p.Close()
			q := diskOwner(t, root)
			selected, e := q.Recover(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			if (selected.Generation == "gen-2") != (cut == "current.renamed" || cut == "current.directory-synced") {
				t.Fatal(selected.Generation)
			}
			assertLogicalRows(t, want, recoveredImage(t, q))
			assertSourceFiles(t, root, original)
		})
	}
}
func TestReceivedGenerationFileFailuresAndInvalidManifestNeverPublish(t *testing.T) {
	image, _ := diskCaptures(t)
	raw := imageBytes(t, image)
	for _, target := range []string{"snapshot", "wal", "CURRENT.tmp"} {
		for _, mode := range []string{"partial", "sync", "close", "corrupt"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				root := seededRoot(t)
				original := sourceFiles(t, root)
				p := diskOwner(t, root)
				d, e := p.Recover(deadline(t))
				if e != nil {
					t.Fatal(e)
				}
				old := readBytes(t, filepath.Join(root, "CURRENT"))
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
				if generation, e := p.installReceivedOwned(deadline(t), d.Log, raw, 2, 4); e == nil || generation != "" {
					t.Fatal(generation, e)
				}
				if !bytes.Equal(old, readBytes(t, filepath.Join(root, "CURRENT"))) {
					t.Fatal("invalid received generation selected")
				}
				assertSourceFiles(t, root, original)
			})
		}
	}
	root := seededRoot(t)
	p := diskOwner(t, root)
	d, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "CURRENT"), []byte("bad"), 0600); e != nil {
		t.Fatal(e)
	}
	if generation, e := p.installReceivedOwned(deadline(t), d.Log, raw, 2, 4); e == nil || generation != "" {
		t.Fatal("invalid source manifest ignored", e)
	}
	if _, e = os.Stat(filepath.Join(root, "gen-2.tmp")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("manifest error wrote staging")
	}
}
func TestReceivedGenerationQuotaAndSnapshotProfileAreChecked(t *testing.T) {
	image, _ := diskCaptures(t)
	root := seededRoot(t)
	p := diskOwner(t, root)
	d, e := p.Recover(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	old := readBytes(t, filepath.Join(root, "CURRENT"))
	raw := imageBytes(t, image)
	p.ops.usage = func(string) (int64, error) { return wal.MaxDirectoryBytes - wal.ControlReserve, nil }
	if _, e = p.installReceivedOwned(deadline(t), d.Log, raw, 2, 4); !errors.Is(e, wal.ErrLimit) {
		t.Fatal(e)
	}
	if !bytes.Equal(old, readBytes(t, filepath.Join(root, "CURRENT"))) {
		t.Fatal("quota changed current")
	}
	if _, e = PublishReceived(deadline(t), cfg(t), root, nil, d.Log, raw, 2, 4); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
	image.ConfigHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, e = PlanInstallation(cfg(t), imageBytes(t, image), d.Log, 2, 4); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}
