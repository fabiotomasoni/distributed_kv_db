package node

import (
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
)

func TestReplicatedNodesCatchUpAfterFollowerRestart(t *testing.T) {
	cs, ls := configs(t)
	var nodes [3]*Node
	for i := range nodes {
		n, err := startMode(cs[i], true, nil, func(string, string) (net.Listener, error) { return ls[i], nil }, true, 2)
		if err != nil {
			t.Fatal(err)
		}
		nodes[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := ctx(t)
	wait := func(check func() bool) {
		t.Helper()
		for {
			if check() {
				return
			}
			select {
			case <-ticker.C:
			case <-deadline.Done():
				t.Fatal("replication deadline", deadline.Err())
			}
		}
	}
	leader := -1
	wait(func() bool {
		for i, n := range nodes {
			s, _ := n.ElectionStatus()
			if s.Failure != nil {
				t.Fatal(s.Failure)
			}
			if s.Role == raft.Leader {
				leader = i
				return true
			}
		}
		return false
	})
	var last raft.LocalAppend
	for i := 0; i < 3; i++ {
		var err error
		last, err = nodes[leader].AppendLocal(deadline, "read", json.RawMessage(`{"key":"k"}`))
		if err != nil {
			t.Fatal(err)
		}
	}
	converged := func() bool {
		l := nodes[leader].Recovered()
		for _, n := range nodes {
			f := n.Recovered()
			if f.CommitIndex != 0 {
				t.Fatal("M06 advanced commit")
			}
			if f.LastIndex() != l.LastIndex() {
				return false
			}
			for i := range l.Entries {
				if !bytes.Equal(l.Entries[i].Raw, f.Entries[i].Raw) {
					return false
				}
			}
		}
		status, _ := nodes[leader].ElectionStatus()
		for _, p := range status.Progress {
			if p.MatchIndex < last.Index {
				return false
			}
		}
		return true
	}
	wait(converged)
	follower := (leader + 1) % 3
	if err := nodes[follower].Shutdown(deadline); err != nil {
		t.Fatal(err)
	}
	var err error
	last, err = nodes[leader].AppendLocal(deadline, "read", json.RawMessage(`{"key":"after-restart"}`))
	if err != nil {
		t.Fatal(err)
	}
	// Restart uses the actual production entry point after the owned listener
	// has closed. At most three ordinary nodes are active at any instant.
	restarted, err := StartReplication(cs[follower], false)
	if err != nil {
		t.Fatal(err)
	}
	nodes[follower] = restarted
	t.Cleanup(func() { _ = restarted.Shutdown(ctx(t)) })
	wait(converged)
	for _, n := range nodes {
		if err = n.Shutdown(deadline); err != nil {
			t.Fatal(err)
		}
	}
}
