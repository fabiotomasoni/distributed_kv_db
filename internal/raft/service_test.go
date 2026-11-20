package raft

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type manualClock struct {
	now     atomic.Uint64
	ticks   chan time.Time
	stopped atomic.Bool
}

func newClock() *manualClock                   { return &manualClock{ticks: make(chan time.Time, 1)} }
func (c *manualClock) Now() uint64             { return c.now.Load() }
func (c *manualClock) Ticks() <-chan time.Time { return c.ticks }
func (c *manualClock) Stop()                   { c.stopped.Store(true) }
func (c *manualClock) tick(n uint64)           { c.now.Store(n); c.ticks <- time.Time{} }

type senderFunc func(context.Context, peerprotocol.Message) (peerprotocol.Message, error)

func (f senderFunc) Send(ctx context.Context, m peerprotocol.Message) (peerprotocol.Message, error) {
	return f(ctx, m)
}

type memoryStorage struct {
	mu      sync.Mutex
	s       wal.State
	entered chan struct{}
	release chan struct{}
	failure error
}

func (d *memoryStorage) State() wal.State {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.s
	s.Entries = append([]storageformat.LogEntry(nil), s.Entries...)
	return s
}
func (d *memoryStorage) Status() wal.Status { return wal.Status{Generation: "gen-1"} }
func (d *memoryStorage) Append(raw []byte) (wal.Token, error) {
	if d.entered != nil {
		d.entered <- struct{}{}
		<-d.release
	}
	if d.failure != nil {
		return wal.Token{}, d.failure
	}
	tx, err := storageformat.DecodeTransaction(raw)
	if err != nil {
		return wal.Token{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.s.Sequence++
	if tx.HardState != nil {
		d.s.HardState = *tx.HardState
	}
	if tx.Replace != nil {
		d.s.Entries = append(d.s.Entries[:tx.Replace.From-d.s.BaseIndex-1], tx.Replace.Entries...)
	}
	return wal.Token{Generation: "gen-1", Sequence: d.s.Sequence}, nil
}
func (d *memoryStorage) AppendProposal(b []byte) (wal.Token, error) { return d.Append(b) }
func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func service(t *testing.T, d Storage, clock Clock) *Service {
	t.Helper()
	s, err := NewService(configuration(t, "n1"), d, Options{Epoch: strings.Repeat("a", 32), Draw: minimum, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func noNetwork(ctx context.Context, m peerprotocol.Message) (peerprotocol.Message, error) {
	return peerprotocol.Message{}, errors.New("offline peer")
}

func TestServicePersistenceBarrierAdmissionAndShutdown(t *testing.T) {
	for _, failure := range []error{nil, errors.New("sync failed during shutdown")} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			d := &memoryStorage{s: initial(), entered: make(chan struct{}, 1), release: make(chan struct{}), failure: failure}
			clock := newClock()
			s := service(t, d, clock)
			if err := s.Start(senderFunc(noNetwork)); err != nil {
				t.Fatal(err)
			}
			req := voteRequest(t, s.cfg, "n2", 1, 0, 0)
			done := make(chan outcome, 1)
			go func() { m, err := s.Handle(deadline(t), req); done <- outcome{msg: m, err: err} }()
			select {
			case <-d.entered:
			case <-deadline(t).Done():
				t.Fatal("no write")
			}
			if !s.Status().Pending || d.State().HardState.Vote != "" {
				t.Fatal("vote counted before storage")
			}
			select {
			case r := <-done:
				t.Fatal("returned before sync", r)
			default:
			}
			// Fill bounded admission without creating hundreds of waiting goroutines.
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			for i := 0; i < 256; i++ {
				s.events <- requestEvent{ctx: canceled, reply: make(chan outcome, 1)}
			}
			if _, err := s.Handle(deadline(t), req); !errors.Is(err, ErrFull) {
				t.Fatal(err)
			}
			if _, err := s.Handle(canceled, req); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if err := s.Shutdown(canceled); !errors.Is(err, context.Canceled) {
				t.Fatal("shutdown did not report in-progress disk", err)
			}
			select {
			case <-s.Done():
				t.Fatal("abandoned disk call")
			default:
			}
			close(d.release)
			err := s.Shutdown(deadline(t))
			if failure != nil && !errors.Is(err, failure) {
				t.Fatal("lost storage failure on shutdown", err)
			}
			if failure == nil && err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-done:
				if r.err == nil {
					t.Fatal("positive response during shutdown")
				}
			case <-deadline(t).Done():
				t.Fatal("handler leaked")
			}
			if !clock.stopped.Load() || !s.Status().Closed {
				t.Fatal("lifecycle")
			}
			if failure == nil {
				restarted := engine(t, "n1", d.State())
				e, err := restarted.Request(0, voteRequest(t, restarted.cfg, "n3", 1, 0, 0))
				if err != nil || fields(t, e.Reply)["vote_granted"] != false {
					t.Fatal("second vote after abandoned reply")
				}
			}
		})
	}
}

func TestServiceFatalStorageAndNetworkWorkers(t *testing.T) {
	d := &memoryStorage{s: initial(), failure: errors.New("injected EIO")}
	s := service(t, d, newClock())
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	_, err := s.Handle(deadline(t), voteRequest(t, s.cfg, "n2", 1, 0, 0))
	if !errors.Is(err, d.failure) {
		t.Fatal(err)
	}
	if err = s.Shutdown(deadline(t)); !errors.Is(err, d.failure) {
		t.Fatal(err)
	}
	// Stop while both network workers are waiting on a peer. Cancellation must
	// join them, not merely close a status channel.
	clock := newClock()
	s = service(t, &memoryStorage{s: initial()}, clock)
	entered := make(chan string, 2)
	exited := make(chan string, 2)
	if err = s.Start(senderFunc(func(ctx context.Context, m peerprotocol.Message) (peerprotocol.Message, error) {
		entered <- m.Envelope().To
		<-ctx.Done()
		exited <- m.Envelope().To
		return peerprotocol.Message{}, ctx.Err()
	})); err != nil {
		t.Fatal(err)
	}
	clock.tick(600)
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-deadline(t).Done():
			t.Fatal("network not dispatched")
		}
	}
	if err = s.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	if len(exited) != 2 {
		t.Fatal("network worker leak")
	}
	if err = s.Start(senderFunc(noNetwork)); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestServiceBackoffAndLifecycleBoundaries(t *testing.T) {
	p := &peerState{}
	now := uint64(1)
	for _, want := range []uint64{50, 100, 200, 400, 400} {
		if !p.admit(now) || p.admit(now+1) {
			t.Fatal("parallel RPC")
		}
		p.complete(now, errors.New("connection"))
		if p.backoff != want || p.admit(now+want-1) {
			t.Fatal("retry bound", p)
		}
		now += want
	}
	if !p.admit(now) {
		t.Fatal("no retry")
	}
	p.complete(now, nil)
	if p.backoff != 0 || p.admit(now) {
		t.Fatal("same tick or success backoff")
	}
	if !p.admit(now + 1) {
		t.Fatal("next tick")
	}
	s := service(t, &memoryStorage{s: initial()}, newClock())
	if _, err := s.Handle(deadline(t), peerprotocol.Message{}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := s.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(configuration(t, "n1"), nil, Options{}); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		n, err := randomDraw(600, 1000)
		if err != nil || n < 600 || n > 1000 {
			t.Fatal(n, err)
		}
	}
	clock := newClock()
	s = service(t, &memoryStorage{s: initial()}, clock)
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	close(clock.ticks)
	select {
	case <-s.Done():
	case <-deadline(t).Done():
		t.Fatal("closed clock caused busy loop")
	}
	if err := s.Shutdown(deadline(t)); !errors.Is(err, ErrInput) {
		t.Fatal("closed clock not reported", err)
	}
}

func TestRealWALVoteSurvivesReopen(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	c := configuration(t, "n1")
	id := wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}
	store, err := wal.Bootstrap(root, id)
	if err != nil {
		t.Fatal(err)
	}
	s := service(t, store, newClock())
	if err = s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	reply, err := s.Handle(deadline(t), voteRequest(t, c, "n2", 7, 0, 0))
	if err != nil || fields(t, &reply)["vote_granted"] != true {
		t.Fatal(err)
	}
	if store.State().HardState.Vote != "n2" {
		t.Fatal("response without durable vote")
	}
	if err = s.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = wal.Open(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s = service(t, store, newClock())
	if s.Status().Role != Follower || s.Status().Term != 7 || s.Status().Vote != "n2" {
		t.Fatal(s.Status())
	}
	if err = s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(deadline(t))
	reply, err = s.Handle(deadline(t), voteRequest(t, c, "n3", 7, 0, 0))
	if err != nil || fields(t, &reply)["vote_granted"] != false {
		t.Fatal("reopened second vote", err)
	}
}
