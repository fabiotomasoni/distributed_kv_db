// Package node orders M04 startup and shutdown: validate storage and hold its
// exclusive lock before listening, drain peer work before releasing the lock.
package node

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/transport"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type Node struct {
	peer         *transport.Peer
	client       *clientapi.Server
	clientServed chan error
	store        *wal.Store
	election     *raft.Service
	once         sync.Once
	done         chan struct{}
	served       chan error
	err          error
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

// StartCommitted enables M07 and rebuilds a fresh machine from the persisted
// committed prefix before binding. It exposes no client HTTP/read API.
func StartCommitted(c config.Config, bootstrap bool) (*Node, error) {
	return startRuntime(c, bootstrap, nil, net.Listen, true, 16, true)
}

// StartAPI enables PUT/DELETE and logged GET with fresh replay; v2 also enables
// atomic CAS and ordered session deduplication.
func StartAPI(c config.Config, bootstrap bool) (*Node, error) {
	return startNode(c, bootstrap, nil, net.Listen, true, 16, true, true)
}
func (n *Node) WaitApplied(ctx context.Context, r raft.LocalAppend) (raft.AppliedResult, error) {
	if n.election == nil {
		return raft.AppliedResult{}, raft.ErrUnsupported
	}
	return n.election.WaitApplied(ctx, r)
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
	return startRuntime(c, bootstrap, p, listen, elections, batch, false)
}
func startRuntime(c config.Config, bootstrap bool, p transport.Processor, listen func(string, string) (net.Listener, error), elections bool, batch int, commit bool) (*Node, error) {
	return startNode(c, bootstrap, p, listen, elections, batch, commit, false)
}
func startNode(c config.Config, bootstrap bool, p transport.Processor, listen func(string, string) (net.Listener, error), elections bool, batch int, commit, api bool) (*Node, error) {
	if !c.Valid() {
		return nil, config.ErrConfig
	}
	if c.Sessions() && (!api || !commit || !elections) {
		return nil, raft.ErrUnsupported
	}
	id := wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}
	var s *wal.Store
	var err error
	if bootstrap {
		if c.Sessions() {
			s, err = wal.BootstrapSessions(c.DataDir(), id)
		} else {
			s, err = wal.Bootstrap(c.DataDir(), id)
		}
	} else {
		if c.Sessions() {
			s, err = wal.OpenSessions(c.DataDir(), id)
		} else {
			s, err = wal.Open(c.DataDir(), id)
		}
	}
	if err != nil {
		return nil, err
	}
	var election *raft.Service
	if elections {
		election, err = raft.NewService(c, s, raft.Options{BatchSize: batch, Commit: commit, Reads: api, Sessions: c.Sessions()})
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

	var client *clientapi.Server
	var cl net.Listener
	if api {
		client, err = clientapi.New(c, election)
		if err == nil {
			cl, err = listen("tcp", c.Local().Client)
		}
		if err == nil && cl.Addr().String() != c.Local().Client {
			err = config.ErrConfig
		}
		if err != nil {
			if cl != nil {
				_ = cl.Close()
			}
			if client != nil {
				_ = client.Shutdown(context.Background())
			}
			_ = l.Close()
			cleanupElection()
			_ = peer.Shutdown(context.Background())
			return nil, errors.Join(err, s.Close())
		}
	}
	if election != nil {
		if err = election.Start(peer); err != nil {
			if cl != nil {
				_ = cl.Close()
			}
			if client != nil {
				_ = client.Shutdown(context.Background())
			}
			cleanupElection()
			_ = l.Close()
			_ = peer.Shutdown(context.Background())
			return nil, errors.Join(err, s.Close())
		}
	}
	n := &Node{client: client, clientServed: make(chan error, 1), peer: peer, store: s, election: election, done: make(chan struct{}), served: make(chan error, 1)}
	go func() { n.served <- peer.Serve(l); _ = n.Shutdown(context.Background()) }()
	if client != nil {
		go func() { n.clientServed <- client.Serve(cl); _ = n.Shutdown(context.Background()) }()
	}
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
			var clientErr error
			if n.client != nil {
				clientErr = n.client.Shutdown(context.Background())
				serveClientErr := <-n.clientServed
				if errors.Is(serveClientErr, raft.ErrClosed) {
					serveClientErr = nil
				}
				clientErr = errors.Join(clientErr, serveClientErr)
			}
			var electionErr error
			if n.election != nil {
				electionErr = n.election.Shutdown(context.Background())
			}
			peerErr := n.peer.Shutdown(context.Background())
			serveErr := <-n.served
			if errors.Is(serveErr, transport.ErrClosed) {
				serveErr = nil
			}
			n.err = errors.Join(clientErr, electionErr, peerErr, serveErr, n.store.Close())
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

// Done closes only after listener/worker drain, storage close and LOCK release.
// It also exposes automatic fatal shutdown to the owning server process.
func (n *Node) Done() <-chan struct{} { return n.done }
