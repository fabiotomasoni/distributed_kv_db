package snapshotinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func driveLeader(t *testing.T, core *raft.Core, store *wal.Store, out raft.Effects, e error) raft.Effects {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	for out.Persist != nil {
		p := out.Persist
		token, e := store.Append(p.Bytes)
		out, e = core.Durable(p.ID, token, e)
		if e != nil {
			t.Fatal(e)
		}
	}
	return out
}
func TestHigherSnapshotResponseUsesRealM05PersistentStepDown(t *testing.T) {
	c := cfg(t, "n1")
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	store, e := wal.BootstrapSessions(root, wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	rows := []json.RawMessage{}
	for _, v := range history() {
		rows = append(rows, v.Raw)
	}
	raw, _ := json.Marshal(map[string]any{"hard_state": map[string]any{"current_term": "4", "voted_for": "n2"}, "replace_suffix": map[string]any{"from_index": "1", "entries": rows}, "commit_index": "10"})
	if _, e = store.Append(raw); e != nil {
		t.Fatal(e)
	}
	core, e := raft.New(c, store.State(), "gen-1", strings.Repeat("a", 32), func(min, max uint64) (uint64, error) { return min, nil }, 0)
	if e != nil {
		t.Fatal(e)
	}
	if e = core.EnableReplication(16); e != nil {
		t.Fatal(e)
	}
	now := core.Status().ElectionDeadline
	out, e := core.Tick(now)
	out = driveLeader(t, core, store, out, e)
	if len(out.Send) == 0 {
		t.Fatal("no election request")
	}
	vote := out.Send[0]
	env := vote.Envelope()
	body := map[string]any{"protocol_version": 1, "cluster_id": c.ClusterID(), "config_hash": c.Hash(), "from": env.To, "to": "n1", "rpc_id": env.RPCID, "term": "5", "vote_granted": true}
	raw, _ = json.Marshal(body)
	res, e := peerprotocol.DecodeResponse(c, vote, raw)
	if e != nil {
		t.Fatal(e)
	}
	out, e = core.Response(now+1, vote, res)
	driveLeader(t, core, store, out, e)
	if core.Status().Role != raft.Leader || core.Status().Term != 5 {
		t.Fatal(core.Status())
	}
	image, _ := source(t)
	view := store.State()
	view.BaseIndex = 4
	view.BaseTerm = 2
	view.Entries = view.Entries[4:]
	tr, e := NewTransfer(c, "n2", "gen-1", image, view, 1, 0, 65536)
	if e != nil {
		t.Fatal(e)
	}
	req, e := tr.NextSnapshot(5, "gen-1")
	if e != nil {
		t.Fatal(e)
	}
	env = req.Envelope()
	raw, _ = json.Marshal(map[string]any{"protocol_version": 1, "cluster_id": c.ClusterID(), "config_hash": c.Hash(), "from": "n2", "to": "n1", "rpc_id": env.RPCID, "term": "6", "accepted": false, "next_offset": "0"})
	res, e = peerprotocol.DecodeResponse(c, req, raw)
	if e != nil {
		t.Fatal(e)
	}
	if action := tr.Observe(5, "gen-1", res); action == nil {
		t.Fatal("missing M05 step-down action")
	} else if h, ok := action.(HigherTerm); !ok || h.Term != 6 {
		t.Fatal(action)
	}
	out, e = core.Response(now+2, req, res)
	if e != nil || out.Persist == nil || store.State().HardState.Term != 5 || tr.Progress().MatchIndex != 0 {
		t.Fatal("higher term acknowledged before Sync", e)
	}
	driveLeader(t, core, store, out, nil)
	if core.Status().Role != raft.Follower || store.State().HardState.Term != 6 || store.State().HardState.Vote != "" {
		t.Fatal(core.Status())
	}
	if _, e = tr.NextSnapshot(core.Status().Term, store.Status().Generation); e == nil {
		t.Fatal(fmt.Sprint("old transfer survived new term"))
	}
}
