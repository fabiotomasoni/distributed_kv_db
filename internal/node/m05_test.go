package node

import (
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
	"net"
	"testing"
	"time"
)

func TestElectionNodesUseRealTransportAndWAL(t *testing.T) {
	cs, ls := configs(t)
	var ns [3]*Node
	for i := range ns {
		n, err := startConfigured(cs[i], true, nil, func(string, string) (net.Listener, error) { return ls[i], nil }, true)
		if err != nil {
			t.Fatal(err)
		}
		ns[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	}
	// Poll only detached status on a bounded ticker. Actual timer expiry and RPC
	// completion use the production adapters; deterministic timing lives in raft.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := ctx(t)
	leader := -1
	for leader < 0 {
		select {
		case <-timeout.Done():
			t.Fatal("no loopback election", timeout.Err())
		case <-ticker.C:
			for i, n := range ns {
				st, ok := n.ElectionStatus()
				if !ok || st.Failure != nil {
					t.Fatal(st)
				}
				if st.Role == raft.Leader {
					leader = i
				}
			}
		}
	}
	state := ns[leader].Recovered()
	if state.HardState.Vote != cs[leader].NodeID() || len(state.Entries) != 1 || state.CommitIndex != 0 {
		t.Fatal("leader lacks synced local noop or changed commit", state)
	}
	for _, n := range ns {
		if err := n.Shutdown(ctx(t)); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range cs {
		s, err := wal.Open(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
		if err != nil {
			t.Fatal("lock not released", i, err)
		}
		if s.State().CommitIndex != 0 {
			t.Fatal("M05 committed application data")
		}
		_ = s.Close()
	}
}
