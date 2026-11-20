// Package node orders M04 startup and shutdown: validate storage and hold its
// exclusive lock before listening, drain peer work before releasing the lock.
package node

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/transport"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type Node struct {
	peer   *transport.Peer
	store  *wal.Store
	once   sync.Once
	done   chan struct{}
	served chan error
	err    error
}

func Start(c config.Config, bootstrap bool, p transport.Processor) (*Node, error) {
	return start(c, bootstrap, p, net.Listen)
}
func start(c config.Config, bootstrap bool, p transport.Processor, listen func(string, string) (net.Listener, error)) (*Node, error) {
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
	peer, err := transport.New(c, p)
	if err != nil {
		return nil, errors.Join(err, s.Close())
	}
	l, err := listen("tcp", c.Local().Peer)
	if err != nil {
		_ = peer.Shutdown(context.Background())
		return nil, errors.Join(err, s.Close())
	}
	if l.Addr().String() != c.Local().Peer {
		_ = l.Close()
		_ = peer.Shutdown(context.Background())
		return nil, errors.Join(config.ErrConfig, s.Close())
	}
	n := &Node{peer: peer, store: s, done: make(chan struct{}), served: make(chan error, 1)}
	go func() { n.served <- peer.Serve(l); _ = n.Shutdown(context.Background()) }()
	return n, nil
}
func (n *Node) Peer() *transport.Peer { return n.peer }

// Recovered returns detached metadata/log; it neither applies nor commits entries.
func (n *Node) Recovered() wal.State { return n.store.State() }
func (n *Node) Shutdown(ctx context.Context) error {
	n.once.Do(func() {
		go func() {
			peerErr := n.peer.Shutdown(context.Background())
			serveErr := <-n.served
			if errors.Is(serveErr, transport.ErrClosed) {
				serveErr = nil
			}
			n.err = errors.Join(peerErr, serveErr, n.store.Close())
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
