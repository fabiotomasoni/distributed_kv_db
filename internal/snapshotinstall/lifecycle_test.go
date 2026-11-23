package snapshotinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

type gatedMachine struct {
	inner            applier.Machine
	entered, release chan struct{}
	once             sync.Once
}

func (g *gatedMachine) Apply(c statemachine.Command) statemachine.Result {
	g.once.Do(func() { close(g.entered); <-g.release })
	return g.inner.Apply(c)
}
func TestInstallRechecksAppliedAfterActualWorkerDrain(t *testing.T) {
	root := seed(t, nil, 0)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	r, e := open(root, cfg(t, "n2"), func(m applier.Machine) applier.Machine {
		return &gatedMachine{inner: m, entered: entered, release: release}
	})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { once.Do(func() { close(release) }); _ = r.Close() })
	if _, e = r.Handle(deadline(t), 1, appendMessage(t, 0, 0, 1, history()[:1], 5)); e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-deadline(t).Done():
		t.Fatal("apply gate")
	}
	raw := imageBytes(t, makeImage(t, cfg(t, "n1"), history(), 1))
	req := chunk(t, raw, 0, len(raw), "n1", 5, func(b map[string]any) { b["last_included_index"] = "1"; b["last_included_term"] = "1" })
	type answer struct {
		message peerprotocol.Message
		err     error
	}
	done := make(chan answer, 1)
	go func() { m, e := r.Handle(deadline(t), 2, req); done <- answer{m, e} }()
	select {
	case a := <-done:
		t.Fatal("install escaped active old worker", a.err)
	default:
	}
	once.Do(func() { close(release) })
	select {
	case a := <-done:
		if a.err != nil || response(t, a.message)["reason"] != "obsolete" {
			t.Fatal(a.err)
		}
	case <-deadline(t).Done():
		t.Fatal("drain timed out")
	}
	if r.Status().ApplyEpoch != 1 || r.Status().Raft.LastApplied != 1 || r.Status().Generation != "gen-1" {
		t.Fatal("drain failed to reclassify obsolete", r.Status())
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	r = receiver(t, root)
	v, e := r.View(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	assertRows(t, makeImage(t, cfg(t, "n1"), history(), 1), v.Image)
}
func TestInstallSameTermKeepsExistingVoteAndContinuousLock(t *testing.T) {
	root := seed(t, history()[:2], 2)
	r := receiver(t, root)
	image, _ := source(t)
	r.cut = func(stage string) error {
		if stage == "before-generation" || stage == "generation.published" || stage == "memory.installed" {
			if l, e := storagefs.Acquire(root); l != nil || !errors.Is(e, storagefs.ErrLocked) {
				if l != nil {
					_ = l.Close()
				}
				return fmt.Errorf("LOCK released at %s: %v", stage, e)
			}
		}
		return nil
	}
	res, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 4, nil))
	if e != nil || response(t, res)["installed_index"] != "4" {
		t.Fatal(e)
	}
	v, e := r.View(deadline(t))
	if e != nil || v.Log.HardState.Term != 4 || v.Log.HardState.Vote != "n2" {
		t.Fatal(v.Log.HardState, e)
	}
}
func TestInstallHandoffErrorsNeverAcknowledgeOrRollBack(t *testing.T) {
	for _, cut := range []string{"image.validated", "before-generation", "generation.published", "memory.installed"} {
		t.Run(cut, func(t *testing.T) {
			root := seed(t, history()[:2], 2)
			r := receiver(t, root)
			image, _ := source(t)
			sentinel := errors.New("stop at " + cut)
			r.cut = func(stage string) error {
				if stage == cut {
					return sentinel
				}
				return nil
			}
			res, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 5, nil))
			if !errors.Is(e, sentinel) || len(res.Bytes()) != 0 {
				t.Fatal("failure acknowledged", e)
			}
			_ = r.Close()
			r = receiver(t, root)
			v, e := r.View(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			published := cut == "generation.published" || cut == "memory.installed"
			if (v.Log.BaseIndex == 4) != published || v.Log.HardState.Term != 5 {
				t.Fatal(v.Log)
			}
			want := uint64(2)
			if published {
				want = 4
			}
			assertRows(t, makeImage(t, cfg(t, "n1"), history(), want), v.Image)
		})
	}
}
func TestInstallCrashChild(t *testing.T) {
	root := os.Getenv("QUORUMKV_M13E_ROOT")
	if root == "" {
		return
	}
	r := receiver(t, root)
	image, _ := source(t)
	r.cut = func(stage string) error {
		if stage == os.Getenv("QUORUMKV_M13E_CUT") {
			os.Exit(77)
		}
		return nil
	}
	_, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 5, nil))
	t.Fatal("crash cut not reached", e)
}
func TestInstallProcessCrashAndRetryAroundPublication(t *testing.T) {
	for _, cut := range []string{"before-generation", "generation.published", "memory.installed"} {
		t.Run(cut, func(t *testing.T) {
			root := seed(t, history()[:2], 2)
			oldWAL, e := os.ReadFile(filepath.Join(root, "gen-1", "wal"))
			if e != nil {
				t.Fatal(e)
			}
			c, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(c, os.Args[0], "-test.run=^TestInstallCrashChild$")
			cmd.Env = append(os.Environ(), "QUORUMKV_M13E_ROOT="+root, "QUORUMKV_M13E_CUT="+cut, "GORACE=atexit_sleep_ms=0")
			cmd.Stderr = io.Discard
			out, e := cmd.Output()
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 77 {
				t.Fatal(string(out), e)
			}
			r := receiver(t, root)
			v, e := r.View(deadline(t))
			if e != nil {
				t.Fatal(e)
			}
			applied := uint64(4)
			if cut == "before-generation" {
				applied = 2
			}
			assertRows(t, makeImage(t, cfg(t, "n1"), history(), applied), v.Image)
			if v.Log.HardState.Term != 5 || v.Log.HardState.Vote != "" {
				t.Fatal("M05 hard state lost")
			}
			after, e := os.ReadFile(filepath.Join(root, "gen-1", "wal"))
			if e != nil || !bytes.HasPrefix(after, oldWAL) {
				t.Fatal("old WAL deleted/overwritten", e)
			}
			image, _ := source(t)
			res, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 5, nil))
			if e != nil {
				t.Fatal(e)
			}
			if cut == "before-generation" {
				if response(t, res)["installed_index"] != "4" {
					t.Fatal(response(t, res))
				}
			} else if response(t, res)["reason"] != "obsolete" {
				t.Fatal(response(t, res))
			}
		})
	}
}
func TestCatchupTraceReplaysDeterministically(t *testing.T) {
	var expected [][]byte
	for run := 0; run < 2; run++ {
		r := receiver(t, seed(t, history()[:2], 2))
		image, log := source(t)
		tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 127)
		if e != nil {
			t.Fatal(e)
		}
		tr.epoch = "deterministic-m13e" // fixed process nonce in this bounded model
		trace := [][]byte{}
		now := uint64(0)
		for step := 0; step < 100 && !tr.Progress().Finished; step++ {
			q, e := tr.NextSnapshot(5, "gen-9")
			if e != nil {
				t.Fatal(e)
			}
			now++
			a, e := r.Handle(deadline(t), now, q)
			if e != nil {
				t.Fatal(e)
			}
			trace = append(trace, q.Bytes(), a.Bytes())
			if e = tr.Observe(5, "gen-9", a); e != nil {
				t.Fatal(e)
			}
		}
		q, e := tr.NextAppend(5, "gen-9")
		if e != nil {
			t.Fatal(e)
		}
		a, e := r.Handle(deadline(t), now+1, q)
		if e != nil {
			t.Fatal(e)
		}
		trace = append(trace, q.Bytes(), a.Bytes())
		v, e := r.View(deadline(t))
		if e != nil {
			t.Fatal(e)
		}
		trace = append(trace, imageBytes(t, v.Image))
		state, _ := json.Marshal(v.Log)
		trace = append(trace, state)
		if run == 0 {
			expected = trace
		} else {
			if len(expected) != len(trace) {
				t.Fatal("trace length diverged")
			}
			for step := range trace {
				if !bytes.Equal(expected[step], trace[step]) {
					t.Fatalf("first divergence at trace record %d", step)
				}
			}
		}
		if e = r.Close(); e != nil {
			t.Fatal(e)
		}
	}
}
