package raft

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/diagnostics"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

// Storage is borrowed exclusively until Service.Shutdown completes. Append is
// synchronous and must return a token only after Sync. Service never closes it.
type Storage interface {
	State() wal.State
	Status() wal.Status
	Append([]byte) (wal.Token, error)
}
type Sender interface {
	Send(context.Context, peerprotocol.Message) (peerprotocol.Message, error)
}

// Clock belongs to Service after successful NewService. Now is monotonic
// milliseconds; Ticks
// may coalesce. Tests drive it explicitly without wall-clock sleeps.
type Clock interface {
	Now() uint64
	Ticks() <-chan time.Time
	Stop()
}
type Options struct {
	SnapshotRecovery bool // explicit v2 startup only; disabled by default.
	Epoch            string
	Draw             Draw
	Clock            Clock
	BatchSize        int // 0: M05 compatibility; 1..16: M06 replication.
	Commit           bool
	Sessions         bool // M11 requires the versioned session application profile.
	Reads            bool // M10 logged read capability; requires Commit.
	ApplyCapacity    int
	MachineFactory   func() applier.Machine // optional fresh, exclusively owned dependency
}
type requestEvent struct {
	ctx   context.Context
	msg   peerprotocol.Message
	reply chan outcome
	local *localProposal
}
type localProposal struct {
	kind    string
	command json.RawMessage
}
type outcome struct {
	msg   peerprotocol.Message
	err   error
	local LocalAppend
}
type result struct {
	peer              string
	request, response peerprotocol.Message
	err               error
}
type peerState struct {
	jobs                          chan peerprotocol.Message
	busy, attempted               bool
	lastAttempt, retryAt, backoff uint64
}

func (p *peerState) admit(now uint64) bool {
	if p.busy || now < p.retryAt || p.attempted && now == p.lastAttempt {
		return false
	}
	p.busy = true
	p.attempted = true
	p.lastAttempt = now
	return true
}
func (p *peerState) complete(now uint64, err error) {
	p.busy = false
	if err == nil {
		p.backoff = 0
		p.retryAt = 0
		return
	}
	if p.backoff == 0 {
		p.backoff = 50
	} else if p.backoff < 400 {
		p.backoff *= 2
	}
	p.retryAt = now + p.backoff
	if p.retryAt < now {
		p.retryAt = ^uint64(0)
	}
}

type Service struct {
	core            *Core
	diagnostic      *diagnostics.Recorder
	observed        Status
	electionStart   time.Time
	walBytes        atomic.Int64
	storageFailed   atomic.Bool
	sessions        bool
	storage         Storage
	clock           Clock
	cfg             config.Config
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	started, closed bool
	events          chan requestEvent
	results         chan result
	peers           map[string]*peerState
	workers         sync.WaitGroup
	done            chan struct{}
	published       atomic.Value
	application     *applicationRuntime
	writes          chan *writeTicket
	fences          chan writeFence
	writeSlots      chan struct{}
}
type realClock struct {
	start  time.Time
	ticker *time.Ticker
}

func (c *realClock) Now() uint64             { return uint64(time.Since(c.start) / time.Millisecond) }
func (c *realClock) Ticks() <-chan time.Time { return c.ticker.C }
func (c *realClock) Stop()                   { c.ticker.Stop() }
func randomDraw(min, max uint64) (uint64, error) {
	n, err := rand.Int(rand.Reader, new(big.Int).SetUint64(max-min+1))
	if err != nil {
		return 0, err
	}
	return min + n.Uint64(), nil
}

func NewService(c config.Config, s Storage, opt Options) (*Service, error) {
	if s == nil || c.Sessions() != opt.Sessions || opt.SnapshotRecovery && (!opt.Sessions || !opt.Commit || !opt.Reads || opt.MachineFactory != nil) {
		return nil, ErrInput
	}
	if opt.Sessions {
		profile, ok := s.(interface{ ApplicationVersion() int })
		if !opt.Commit || !opt.Reads || !c.Sessions() || !ok || profile.ApplicationVersion() != 2 {
			return nil, ErrInput
		}
	} else if profile, ok := s.(interface{ ApplicationVersion() int }); ok && profile.ApplicationVersion() == 2 {
		return nil, ErrInput
	}
	if opt.Reads && !opt.Commit {
		return nil, ErrInput
	}
	if opt.Commit && opt.BatchSize == 0 {
		opt.BatchSize = 16
	}
	if s == nil {
		return nil, ErrInput
	}
	status := s.Status()
	if status.Closed || status.Failed {
		return nil, ErrInput
	}
	if opt.Epoch == "" {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		opt.Epoch = hex.EncodeToString(b[:])
	}
	if opt.Draw == nil {
		opt.Draw = randomDraw
	}
	var now uint64
	if opt.Clock != nil {
		now = opt.Clock.Now()
	}
	core, err := New(c, s.State(), s.Status().Generation, opt.Epoch, opt.Draw, now)
	if err != nil {
		return nil, err
	}
	if opt.BatchSize != 0 {
		if _, ok := s.(interface {
			AppendProposal([]byte) (wal.Token, error)
		}); !ok {
			return nil, ErrInput
		}
		if err = core.EnableReplication(opt.BatchSize); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	recoveryStart := time.Now()
	x := &Service{diagnostic: diagnostics.New(c.NodeID(), c.ClusterID()), sessions: opt.Sessions, writes: make(chan *writeTicket, 64), fences: make(chan writeFence, 64), writeSlots: make(chan struct{}, 64), cfg: c, core: core, storage: s, clock: opt.Clock, ctx: ctx, cancel: cancel, events: make(chan requestEvent, 256), results: make(chan result, 2), peers: map[string]*peerState{}, done: make(chan struct{})}
	for _, m := range c.Members() {
		if m.ID != c.NodeID() {
			x.peers[m.ID] = &peerState{jobs: make(chan peerprotocol.Message, 1)}
		}
	}
	x.publish()
	if err = x.initApplication(opt); err != nil {
		x.cancel()
		return nil, err
	}
	x.publish()
	x.walBytes.Store(s.Status().WALBytes)
	x.diagnostic.Inc("recovered")
	x.diagnostic.Observe("recovery", time.Since(recoveryStart).Microseconds())
	x.diagnostic.Log(diagnostics.Record{Event: diagnostics.Recovery, Term: core.Status().Term, Role: string(core.Status().Role), Commit: core.Status().CommitIndex, Applied: core.Status().LastApplied})
	return x, nil
}
func (x *Service) Status() Status { return x.published.Load().(Status) }

// Done closes after the owner and both network workers terminate.
func (x *Service) Done() <-chan struct{} { return x.done }
func (x *Service) Start(sender Sender) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.closed || x.started || sender == nil {
		return ErrClosed
	}
	if x.clock == nil {
		h, _, _ := x.cfg.Timing()
		interval := h / 2
		if interval < 1 {
			interval = 1
		}
		if interval > 25 {
			interval = 25
		}
		x.clock = &realClock{start: time.Now(), ticker: time.NewTicker(time.Duration(interval) * time.Millisecond)}
	}
	x.started = true
	if x.application != nil {
		x.application.collecting = true
		go x.application.collect()
	}
	for id, p := range x.peers {
		x.workers.Add(1)
		go x.sendWorker(sender, id, p)
	}
	go x.run()
	return nil
}
func (x *Service) sendWorker(sender Sender, id string, p *peerState) {
	defer x.workers.Done()
	for {
		select {
		case <-x.ctx.Done():
			return
		case req, ok := <-p.jobs:
			if !ok {
				return
			}
			rpcStart := time.Now()
			x.diagnostic.Inc("rpc_sent")
			res, err := sender.Send(x.ctx, req)
			x.diagnostic.Observe("rpc", time.Since(rpcStart).Microseconds())
			if err != nil {
				x.diagnostic.Inc("rpc_failed")
			}
			select {
			case x.results <- result{id, req, res, err}:
			case <-x.ctx.Done():
				return
			}
		}
	}
}

// Handle has bounded admission; caller cancellation abandons only its wait once
// persistence has begun. An accepted vote remains durable if its reply is lost.
func (x *Service) Handle(ctx context.Context, msg peerprotocol.Message) (peerprotocol.Message, error) {
	if ctx == nil {
		return peerprotocol.Message{}, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return peerprotocol.Message{}, err
	}
	e := requestEvent{ctx: ctx, msg: msg, reply: make(chan outcome, 1)}
	x.mu.Lock()
	if x.closed || !x.started {
		x.mu.Unlock()
		return peerprotocol.Message{}, ErrClosed
	}
	if failure := x.Status().Failure; failure != nil {
		x.mu.Unlock()
		return peerprotocol.Message{}, failure
	}
	select {
	case x.events <- e:
		x.mu.Unlock()
	default:
		x.mu.Unlock()
		return peerprotocol.Message{}, ErrFull
	}
	select {
	case r := <-e.reply:
		return r.msg, r.err
	case <-ctx.Done():
		return peerprotocol.Message{}, ctx.Err()
	case <-x.done:
		return peerprotocol.Message{}, x.terminal()
	}
}

// AppendLocal waits only for local Sync. Cancellation after admission may leave
// an entry present: this is not a client commit/deduplication interface.
func (x *Service) AppendLocal(ctx context.Context, kind string, command json.RawMessage) (LocalAppend, error) {
	if len(command) > storageformat.MaxEntry || len(kind) > 6 {
		return LocalAppend{}, ErrInput
	}
	if ctx == nil {
		return LocalAppend{}, ErrInput
	}
	if err := ctx.Err(); err != nil {
		return LocalAppend{}, err
	}
	e := requestEvent{ctx: ctx, reply: make(chan outcome, 1), local: &localProposal{kind: kind, command: bytes.Clone(command)}}
	x.mu.Lock()
	if x.closed || !x.started {
		x.mu.Unlock()
		return LocalAppend{}, ErrClosed
	}
	if failure := x.Status().Failure; failure != nil {
		x.mu.Unlock()
		return LocalAppend{}, failure
	}
	select {
	case x.events <- e:
		x.mu.Unlock()
	default:
		x.mu.Unlock()
		return LocalAppend{}, ErrFull
	}
	select {
	case r := <-e.reply:
		return r.local, r.err
	case <-ctx.Done():
		return LocalAppend{}, ctx.Err()
	case <-x.done:
		return LocalAppend{}, x.terminal()
	}
}
func (x *Service) terminal() error {
	if err := x.Status().Failure; err != nil {
		return err
	}
	return ErrClosed
}
func (x *Service) effects(e Effects, err error) (Effects, error) {
	for err == nil && e.Persist != nil {
		x.publish()
		// This adapter is the single serial disk worker. Core advancement pauses,
		// but there is no core/global mutex held across the blocking filesystem call.
		syncStart := time.Now()
		var token wal.Token
		var writeErr error
		if e.Persist.Proposal {
			token, writeErr = x.storage.(interface {
				AppendProposal([]byte) (wal.Token, error)
			}).AppendProposal(e.Persist.Bytes)
		} else {
			token, writeErr = x.storage.Append(e.Persist.Bytes)
		}
		x.diagnostic.Observe("sync", time.Since(syncStart).Microseconds())
		x.walBytes.Store(x.storage.Status().WALBytes)
		if writeErr != nil {
			x.diagnostic.Inc("storage_errors")
			if !errors.Is(writeErr, wal.ErrLimit) || errors.Is(writeErr, wal.ErrFailed) {
				x.storageFailed.Store(true)
				x.diagnostic.Log(diagnostics.Record{Event: diagnostics.StorageError, Code: "STORAGE_UNAVAILABLE", Term: x.core.Status().Term})
			}
		}
		e, err = x.core.Durable(e.Persist.ID, token, writeErr)
		if x.ctx.Err() != nil {
			x.core.Stop()
			x.publish()
			return Effects{}, errors.Join(ErrClosed, err)
		}
	}
	x.publish()
	if err != nil {
		return e, err
	}
	for _, msg := range e.Send {
		p := x.peers[msg.Envelope().To]
		now := x.clock.Now()
		if p == nil || !p.admit(now) {
			continue
		}
		if err := x.core.Sent(msg); err != nil {
			p.busy = false
			return Effects{}, err
		}
		p.jobs <- msg
	}
	return e, nil
}
func (x *Service) run() {
	defer func() {
		x.mu.Lock()
		x.closed = true
		x.mu.Unlock()
		x.cancel()
		x.clock.Stop()
		for _, p := range x.peers {
			close(p.jobs)
		}
		x.workers.Wait()
		x.finishApplication()
		x.core.Stop()
		x.publish()
		x.mu.Lock()
		x.closed = true
		x.mu.Unlock()
		close(x.done)
	}()
	for {
		if x.ctx.Err() != nil {
			return
		}
		select {
		case <-x.ctx.Done():
			return
		case t := <-x.writes:
			x.processWrite(t)
		case f := <-x.fences:
			x.processFence(f)
		case completion := <-x.completionChannel():
			x.applyCompleted(completion)
		case _, ok := <-x.clock.Ticks():
			if !ok {
				_, _ = x.core.fail(ErrInput)
				x.publish()
				return
			}
			e, err := x.core.Tick(x.clock.Now())
			_, _ = x.effects(e, err)
		case r := <-x.results:
			p := x.peers[r.peer]
			now := x.clock.Now()
			p.complete(now, r.err)
			if r.err == nil {
				e, err := x.core.Response(now, r.request, r.response)
				if err != nil {
					x.core.Failed(r.request)
				}
				_, _ = x.effects(e, err)
			} else {
				x.core.Failed(r.request)
			}
		case r := <-x.events:
			if err := r.ctx.Err(); err != nil {
				r.reply <- outcome{err: err}
				continue
			}
			var e Effects
			var err error
			if r.local != nil {
				e, err = x.core.AppendLocal(x.clock.Now(), r.local.kind, r.local.command)
			} else {
				e, err = x.core.Request(x.clock.Now(), r.msg)
			}
			e, err = x.effects(e, err)
			if err == nil {
				err = e.Rejected
			}
			if err == nil && e.Reply == nil && e.Local == nil {
				err = ErrUnsupported
			}
			var msg peerprotocol.Message
			if e.Reply != nil {
				msg = *e.Reply
			}
			result := outcome{msg: msg, err: err}
			if e.Local != nil {
				result.local = *e.Local
			}
			r.reply <- result
		}
		_ = x.scheduleApply()
		x.publish()
		if x.core.Status().Failure != nil {
			return
		}
	}
}

// Shutdown rejects admission and cancels networking, then waits for storage and
// the M07 committed application drain. A canceled waiter never closes storage.
func (x *Service) Shutdown(ctx context.Context) error {
	x.mu.Lock()
	if !x.closed {
		x.closed = true
		x.cancel()
		if !x.started {
			if x.clock != nil {
				x.clock.Stop()
			}
			x.finishApplication()
			x.core.Stop()
			x.publish()
			close(x.done)
		}
	}
	x.mu.Unlock()
	select {
	case <-x.done:
		return x.Status().Failure
	case <-ctx.Done():
		return ctx.Err()
	}
}
