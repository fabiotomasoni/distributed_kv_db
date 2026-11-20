// Package node orders M04 startup and shutdown: validate storage and hold its
// exclusive lock before listening, drain peer work before releasing the lock.
package node

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/transport"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type Node struct {
	peer     *transport.Peer
	store    *wal.Store
	election *raft.Service
	once     sync.Once
	done     chan struct{}
	served   chan error
	err      error
}

func Start(c config.Config, bootstrap bool, p transport.Processor) (*Node, error) {
	return start(c, bootstrap, p, net.Listen)
}

// StartElection adds only M05 roles, persistent votes and empty heartbeats.
// Start retains the injectable M04 processor contract for existing callers.
func StartElection(c config.Config, bootstrap bool) (*Node, error) {
	return startConfigured(c, bootstrap, nil, net.Listen, true)
}
func StartReplication(c config.Config, bootstrap bool) (*Node, error) {
	return startMode(c, bootstrap, nil, net.Listen, true, 16)
}
func (n *Node) AppendLocal(ctx context.Context, kind string, command json.RawMessage) (raft.LocalAppend, error) {
	if n.election == nil {
		return raft.LocalAppend{}, raft.ErrUnsupported
	}
	return n.election.AppendLocal(ctx, kind, command)
}
func start(c config.Config, bootstrap bool, p transport.Processor, listen func(string, string) (net.Listener, error)) (*Node, error) {
	return startConfigured(c, bootstrap, p, listen, false)
}
func startConfigured(c config.Config, bootstrap bool, p transport.Processor, listen func(string, string) (net.Listener, error), elections bool) (*Node, error) {
	return startMode(c, bootstrap, p, listen, elections, 0)
}
func startMode(c config.Config, bootstrap bool, p transport.Processor, listen func(string, string) (net.Listener, error), elections bool, batch int) (*Node, error) {
	if !c.Valid() {
		return nil, config.ErrConfig
	}
	id := wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}
	var s *wal.Store
	var err error
	if bootstrap {
		s, err = wal.Bootstrap(c.DataDir(), id)
	} else {
		s, err = wal.Open(c.DataDir(), id)
	}
	if err != nil {
		return nil, err
	}
	var election *raft.Service
	if elections {
		election, err = raft.NewService(c, s, raft.Options{BatchSize: batch})
		if err != nil {
			return nil, errors.Join(err, s.Close())
		}
		p = election.Handle
	}
	cleanupElection := func() {
		if election != nil {
			_ = election.Shutdown(context.Background())
		}
	}
	peer, err := transport.New(c, p)
	if err != nil {
		cleanupElection()
		return nil, errors.Join(err, s.Close())
	}
	l, err := listen("tcp", c.Local().Peer)
	if err != nil {
		cleanupElection()
		_ = peer.Shutdown(context.Background())
		return nil, errors.Join(err, s.Close())
	}
	if l.Addr().String() != c.Local().Peer {
		cleanupElection()
		_ = l.Close()
		_ = peer.Shutdown(context.Background())
		return nil, errors.Join(config.ErrConfig, s.Close())
	}
	if election != nil {
		if err = election.Start(peer); err != nil {
			cleanupElection()
			_ = l.Close()
			_ = peer.Shutdown(context.Background())
			return nil, errors.Join(err, s.Close())
		}
	}
	n := &Node{peer: peer, store: s, election: election, done: make(chan struct{}), served: make(chan error, 1)}
	go func() { n.served <- peer.Serve(l); _ = n.Shutdown(context.Background()) }()
	if election != nil {
		go func() { <-election.Done(); _ = n.Shutdown(context.Background()) }()
	}
	return n, nil
}
func (n *Node) Peer() *transport.Peer { return n.peer }
func (n *Node) ElectionStatus() (raft.Status, bool) {
	if n.election == nil {
		return raft.Status{}, false
	}
	return n.election.Status(), true
}

// Recovered returns detached metadata/log; it neither applies nor commits entries.
func (n *Node) Recovered() wal.State { return n.store.State() }
func (n *Node) Shutdown(ctx context.Context) error {
	n.once.Do(func() {
		go func() {
			var electionErr error
			if n.election != nil {
				electionErr = n.election.Shutdown(context.Background())
			}
			peerErr := n.peer.Shutdown(context.Background())
			serveErr := <-n.served
			if errors.Is(serveErr, transport.ErrClosed) {
				serveErr = nil
			}
			n.err = errors.Join(electionErr, peerErr, serveErr, n.store.Close())
			close(n.done)
		}()
	})
	select {
	case <-n.done:
		return n.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
