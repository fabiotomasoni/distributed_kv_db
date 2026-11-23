package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/snapshot"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

// Same tiny history on all replicas. Snapshot S=2 contains successful CAS;
// suffix repeats that CAS, contains a failed CAS and an ordered read, and leaves
// one speculative write at 6. Only entries 3..5 may be replayed at bootstrap.
func snapshotSeed(t *testing.T, c config.Config) {
	t.Helper()
	raw := []string{
		`{"command":{"client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expected":{"exists":false},"key":"x","new_value_b64":"QQ==","sequence":1},"index":"1","kind":"cas","term":"1"}`,
		`{"index":"2","kind":"noop","term":"1"}`,
		`{"command":{"client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expected":{"exists":false},"key":"x","new_value_b64":"QQ==","sequence":1},"index":"3","kind":"cas","term":"1"}`,
		`{"command":{"client_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","expected":{"exists":true,"value_b64":"Qg=="},"key":"x","new_value_b64":"Qw==","sequence":1},"index":"4","kind":"cas","term":"1"}`,
		`{"command":{"key":"x"},"index":"5","kind":"read","term":"1"}`,
		`{"command":{"client_id":"cccccccccccccccccccccccccccccccc","key":"pending","sequence":1,"value_b64":"cA=="},"index":"6","kind":"put","term":"1"}`,
	}
	m, e := snapshot.NewMachine(c)
	if e != nil {
		t.Fatal(e)
	}
	a, e := applier.New(m, applier.Config{Capacity: 2})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Shutdown(context.Background())
	for i := 0; i < 2; i++ {
		command, e := committed.ProjectWithSessions(storageformat.LogEntry{Index: uint64(i + 1), Term: 1, Raw: []byte(raw[i])})
		if e != nil {
			t.Fatal(e)
		}
		f, e := a.Submit(ctx(t), applier.Entry{Index: uint64(i + 1), Command: command})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Wait(ctx(t)); e != nil {
			t.Fatal(e)
		}
	}
	b, e := a.Park(ctx(t))
	if e != nil {
		t.Fatal(e)
	}
	image, e := snapshot.Capture(ctx(t), b, snapshot.Proof{LastApplied: 2, ScheduledApplyIndex: 2, CommitIndex: 5, ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return 1, nil }})
	release := b.Release(ctx(t))
	if e != nil || release != nil {
		t.Fatal(e, release)
	}
	s, e := wal.BootstrapSessions(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if e != nil {
		t.Fatal(e)
	}
	rows := make([]json.RawMessage, len(raw))
	for i, r := range raw {
		rows[i] = json.RawMessage(r)
	}
	tx := encode(map[string]any{"hard_state": map[string]any{"current_term": "3", "voted_for": "n2"}, "replace_suffix": map[string]any{"from_index": "1", "entries": rows}, "commit_index": "5"})
	if _, e = s.Append(tx); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	p, e := snapshot.OpenPersistence(c.DataDir(), c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Publish(ctx(t), image); e != nil {
		t.Fatal(e)
	}
	if e = p.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestSnapshotBootstrapFailureBeforeListenAndLockRelease(t *testing.T) {
	for _, mode := range []string{"snapshot", "wal", "current", "identity", "missing-current", "missing-snapshot", "base-mismatch", "bind-failure", "default-disabled"} {
		t.Run(mode, func(t *testing.T) {
			cs, ls := sessionConfigs(t)
			for _, l := range ls {
				_ = l.Close()
			}
			c := cs[0]
			snapshotSeed(t, c)
			path := filepath.Join(c.DataDir(), "gen-2", "snapshot")
			switch mode {
			case "snapshot":
				b, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				b[len(b)-1] ^= 1
				if e = os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			case "wal":
				if e := os.WriteFile(filepath.Join(c.DataDir(), "gen-2", "wal"), []byte("bad"), 0600); e != nil {
					t.Fatal(e)
				}
			case "current":
				if e := os.WriteFile(filepath.Join(c.DataDir(), "CURRENT"), []byte("bad"), 0600); e != nil {
					t.Fatal(e)
				}
			case "identity":
				if e := os.WriteFile(filepath.Join(c.DataDir(), "IDENTITY"), []byte("bad"), 0600); e != nil {
					t.Fatal(e)
				}
			case "missing-current":
				if e := os.Remove(filepath.Join(c.DataDir(), "CURRENT")); e != nil {
					t.Fatal(e)
				}
			case "missing-snapshot":
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
			case "base-mismatch":
				b, e := os.ReadFile(filepath.Join(c.DataDir(), "CURRENT"))
				if e != nil {
					t.Fatal(e)
				}
				var v map[string]any
				_ = json.Unmarshal(b, &v)
				delete(v, "checksum")
				v["base_term"] = "2"
				b, e = storageformat.SealManifest("CURRENT", encode(v))
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(c.DataDir(), "CURRENT"), b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			injected := errors.New("bind deliberately stopped")
			n, e := startNodeRecovery(c, false, nil, func(string, string) (net.Listener, error) { calls++; return nil, injected }, true, 16, true, true, mode != "default-disabled")
			if n != nil || e == nil {
				t.Fatal("partially started", e)
			}
			if (mode == "bind-failure") != (calls == 1) {
				t.Fatal("listener ordering", calls, e)
			}
			if mode == "bind-failure" && !errors.Is(e, injected) {
				t.Fatal(e)
			}
			if mode == "default-disabled" && !errors.Is(e, wal.ErrSnapshot) {
				t.Fatal(e)
			}
			// Reacquire LOCK directly even when corrupt files prevent normal recovery.
			l, e := storagefs.Acquire(c.DataDir())
			if e != nil {
				t.Fatal("lock leaked", e)
			}
			if e = l.Close(); e != nil {
				t.Fatal(e)
			}
			for _, addr := range []string{c.Local().Peer, c.Local().Client} {
				l, e := net.Listen("tcp", addr)
				if e != nil {
					t.Fatal("listener leaked", e)
				}
				_ = l.Close()
			}
		})
	}
}
func TestSnapshotRuntimeReadyCursorsSpeculativeExclusionAndImmediateShutdown(t *testing.T) {
	cs, ls := sessionConfigs(t)
	c := cs[0]
	snapshotSeed(t, c)
	for _, l := range ls {
		_ = l.Close()
	}
	for i := 0; i < 2; i++ {
		n, e := StartAPIWithSnapshotRecovery(c)
		if e != nil {
			t.Fatal(e)
		}
		st, _ := n.ElectionStatus()
		if st.Role != raft.Follower || st.Term != 3 || st.Vote != "n2" || st.LastIndex != 6 || st.CommitIndex != 5 || st.LastApplied != 5 || st.ScheduledApplyIndex != 5 || st.ApplyEpoch != 1 {
			t.Fatal(st)
		}
		// After immediate shutdown, reconstruct independently and inspect the
		// exact committed image; no speculative key may appear during boot.
		if e = n.Shutdown(ctx(t)); e != nil {
			t.Fatal(e)
		}
		p, e := snapshot.OpenPersistence(c.DataDir(), c)
		if e != nil {
			t.Fatal(e)
		}
		d, e := p.Recover(ctx(t))
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(c.DataDir(), d.Generation, "snapshot"))
		if e != nil {
			t.Fatal(e)
		}
		r, e := snapshot.RecoverCommitted(ctx(t), c, b, d.Log, 1)
		if e != nil {
			t.Fatal(e)
		}
		worker, e := applier.New(r.Machine, applier.Config{Capacity: 1, InitialIndex: r.LastApplied})
		if e != nil {
			t.Fatal(e)
		}
		lease, e := worker.Park(ctx(t))
		if e != nil {
			t.Fatal(e)
		}
		image, e := snapshot.Capture(ctx(t), lease, snapshot.Proof{LastApplied: 5, ScheduledApplyIndex: 5, CommitIndex: 5, ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return 1, nil }})
		if e != nil {
			t.Fatal(e)
		}
		_ = lease.Release(ctx(t))
		_ = worker.Shutdown(ctx(t))
		_ = p.Close()
		if len(image.KV) != 1 || image.KV[0].Key != "x" || !bytes.Equal(image.KV[0].Value, []byte("A")) || len(image.Sessions) != 2 || image.Sessions[0].Result.Index != 1 || image.Sessions[1].Result.Index != 4 {
			t.Fatal(fmt.Sprint(image))
		}
	}
}

func TestSnapshotOptInPreservesEmptyBaseWALOnlyAndRejectsLegacy(t *testing.T) {
	cs, ls := sessionConfigs(t)
	for _, l := range ls {
		_ = l.Close()
	}
	c := cs[0]
	s, e := wal.BootstrapSessions(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	for _, start := range []func(config.Config) (*Node, error){func(c config.Config) (*Node, error) { return StartAPI(c, false) }, StartAPIWithSnapshotRecovery} {
		n, e := start(c)
		if e != nil {
			t.Fatal(e)
		}
		st, _ := n.ElectionStatus()
		if st.LastApplied != 0 || st.ScheduledApplyIndex != 0 || st.CommitIndex != 0 || st.LastIndex != 0 {
			t.Fatal(st)
		}
		if e = n.Shutdown(ctx(t)); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.Remove(filepath.Join(c.DataDir(), "CURRENT")); e != nil {
		t.Fatal(e)
	}
	if n, e := StartAPIWithSnapshotRecovery(c); n != nil || !errors.Is(e, wal.ErrNotInitialized) {
		t.Fatal("missing CURRENT used fallback", e)
	}
	legacy, ll := apiConfigs(t)
	for _, l := range ll {
		_ = l.Close()
	}
	if n, e := StartAPIWithSnapshotRecovery(legacy[0]); n != nil || !errors.Is(e, raft.ErrUnsupported) {
		t.Fatal("legacy implicitly migrated", e)
	}
}
