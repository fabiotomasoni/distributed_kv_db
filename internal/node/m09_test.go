package node

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func TestRestartAllExcludesSuffixUntilNewTermNoopCommits(t *testing.T) {
	cs, listeners := apiConfigs(t)
	safe := json.RawMessage(`{"index":"1","term":"5","kind":"put","command":{"key":"safe","value_b64":"cw==","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1}}`)
	pending := json.RawMessage(`{"index":"2","term":"5","kind":"put","command":{"key":"pending","value_b64":"cA==","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":2}}`)
	tx, _ := json.Marshal(map[string]any{"hard_state": map[string]any{"current_term": "5", "voted_for": nil}, "replace_suffix": map[string]any{"from_index": "1", "entries": []json.RawMessage{safe, pending}}, "commit_index": "1"})
	var canonical any
	if e := json.Unmarshal(tx, &canonical); e != nil {
		t.Fatal(e)
	}
	tx, _ = json.Marshal(canonical)
	for _, c := range cs {
		w, e := wal.Bootstrap(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Append(tx); e != nil {
			t.Fatal(e)
		}
		if e = w.Close(); e != nil {
			t.Fatal(e)
		}
	}
	var ns [3]*Node
	for i := range ns {
		n, e := startNode(cs[i], false, nil, func(_, addr string) (net.Listener, error) { return listeners[addr], nil }, true, 16, true, true)
		if e != nil {
			t.Fatal(e)
		}
		ns[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
		st, _ := n.ElectionStatus()
		if st.Role != raft.Follower || st.Term != 5 || st.CommitIndex != 1 || st.LastApplied != 1 || st.ScheduledApplyIndex != 1 || st.LastIndex != 2 {
			t.Fatal("speculative suffix replayed on startup", st)
		}
		r, e := n.WaitApplied(ctx(t), raft.LocalAppend{Index: 1, Term: 5})
		if e != nil || !r.Result.Applied {
			t.Fatal(r, e)
		}
	}
	// The inherited suffix is allowed to commit later, only through a new-term
	// quorum boundary. Unknown is neither a promise of failure nor of exclusion.
	for _, n := range ns {
		r, e := n.WaitApplied(ctx(t), raft.LocalAppend{Index: 2, Term: 5})
		if e != nil || !r.Result.Applied {
			t.Fatal(r, e)
		}
		st, _ := n.ElectionStatus()
		if st.Term <= 5 || st.CommitIndex < 3 || st.LastApplied < 2 {
			t.Fatal("old term committed without current noop", st)
		}
	}
	for _, n := range ns {
		if e := n.Shutdown(ctx(t)); e != nil {
			t.Fatal(e)
		}
		select {
		case <-n.Done():
		case <-time.After(time.Second):
			t.Fatal("node Done before/after resource completion mismatch")
		}
	}
}
