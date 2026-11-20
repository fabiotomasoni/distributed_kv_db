package node

import (
	"errors"
	"net"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func TestElectionStartupFailureReleasesStorage(t *testing.T) {
	for _, wrongAddress := range []bool{false, true} {
		name := "bind failure"
		if wrongAddress {
			name = "mismatched listener"
		}
		t.Run(name, func(t *testing.T) {
			cs, ls := configs(t)
			injected := errors.New("injected election listener failure")
			n, err := startConfigured(cs[0], true, nil, func(string, string) (net.Listener, error) {
				if wrongAddress {
					return ls[1], nil
				}
				return nil, injected
			}, true)
			if err == nil || n != nil {
				t.Fatal("failed startup returned a usable node", err)
			}
			if !wrongAddress && !errors.Is(err, injected) {
				t.Fatal("bind error hidden", err)
			}
			c := cs[0]
			s, err := wal.Open(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
			if err != nil {
				t.Fatal("failed election startup leaked storage ownership", err)
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElectionServiceTerminationClosesNode(t *testing.T) {
	cs, ls := configs(t)
	n, err := startConfigured(cs[0], true, nil, func(string, string) (net.Listener, error) { return ls[0], nil }, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	// Terminate the service independently: its Done watcher must initiate the
	// same complete cleanup used for fatal service termination.
	if err = n.election.Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-n.done:
	case <-ctx(t).Done():
		t.Fatal("service termination left node resources active")
	}
	if err = n.Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	s, err := wal.Open(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if err != nil {
		t.Fatal("service termination leaked WAL lock", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}
