package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func TestReconciliationWALCutpointsAndFollowerRecovery(t *testing.T) {
	c := configuration(t, "n2")
	id := wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := wal.Bootstrap(root, id)
	if err != nil {
		t.Fatal(err)
	}
	entries := []json.RawMessage{logEntry(1, 1).Raw, logEntry(2, 4).Raw, logEntry(3, 4).Raw}
	raw, _ := json.Marshal(map[string]any{"hard_state": map[string]any{"current_term": "5", "voted_for": "n3"}, "replace_suffix": map[string]any{"from_index": "1", "entries": entries}, "commit_index": "1"})
	if _, err = store.Append(raw); err != nil {
		t.Fatal(err)
	}
	core, err := New(c, store.State(), store.Status().Generation, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", minimum, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = core.EnableReplication(16); err != nil {
		t.Fatal(err)
	}
	req := appendRPC(t, core, 6, 1, 1, 3, []storageformat.LogEntry{logEntry(2, 2), logEntry(3, 2)})
	effects, err := core.Request(1, req)
	if err != nil || effects.Persist == nil || effects.Reply != nil {
		t.Fatal(err)
	}
	frame, err := storageformat.EncodeFrame(effects.Persist.ID, effects.Persist.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "gen-1", "wal")
	prefix, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{0, 20, 24, len(frame) - 1, len(frame)} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			// Own tiny byte images model interruption around the reconciliation frame;
			// a complete surviving write is not evidence of a previous acknowledgment.
			image := append(bytes.Clone(prefix), frame[:cut]...)
			if err := os.WriteFile(path, image, 0600); err != nil {
				t.Fatal(err)
			}
			s, err := wal.Open(root, id)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			state := s.State()
			if state.CommitIndex != 1 || !bytes.Equal(state.Entries[0].Raw, logEntry(1, 1).Raw) {
				t.Fatal("lost committed prefix")
			}
			want := uint64(4)
			if cut == len(frame) {
				want = 2
			}
			if state.Entries[1].Term != want {
				t.Fatal("partial transaction installed")
			}
			follower, err := New(c, state, s.Status().Generation, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", minimum, 50)
			if err != nil {
				t.Fatal(err)
			}
			if err = follower.EnableReplication(16); err != nil {
				t.Fatal(err)
			}
			e, err := follower.Request(51, req)
			if err != nil {
				t.Fatal(err)
			}
			for e.Persist != nil {
				if e.Reply != nil {
					t.Fatal("ack before Sync")
				}
				tok, err := s.Append(e.Persist.Bytes)
				e, err = follower.Durable(e.Persist.ID, tok, err)
				if err != nil {
					t.Fatal(err)
				}
			}
			if fields(t, e.Reply)["success"] != true || follower.state.CommitIndex != 1 || s.State().Entries[1].Term != 2 {
				t.Fatal("failed catch-up after restart")
			}
		})
	}
}
