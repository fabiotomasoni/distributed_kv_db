// Package transport provides bounded HTTP peer I/O. It owns no consensus state.
package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
)

var ErrClosed = errors.New("transport: closed")
var ErrHTTP = errors.New("transport: peer HTTP failure")

// Processor must honor cancellation. Consensus decisions belong to its caller;
// nil means unavailable (503), never an invented vote or replication success.
type Processor func(context.Context, peerprotocol.Message) (peerprotocol.Message, error)
type Peer struct {
	cfg             config.Config
	processor       Processor
	server          *http.Server
	client          *http.Client
	pool            *http.Transport
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	started, closed bool
	listener        net.Listener
	connections     map[net.Conn]bool
	active          sync.WaitGroup
	slots           map[string]chan struct{}
	done            chan struct{}
	finish          sync.Once
}

func New(c config.Config, p Processor) (*Peer, error) {
	if !c.Valid() {
		return nil, config.ErrConfig
	}
	ctx, cancel := context.WithCancel(context.Background())
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 100 * time.Millisecond}).DialContext, DisableCompression: true, ForceAttemptHTTP2: false, MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2, MaxIdleConns: 4, IdleConnTimeout: time.Second, ResponseHeaderTimeout: 300 * time.Millisecond}
	x := &Peer{cfg: c, processor: p, pool: tr, ctx: ctx, cancel: cancel, connections: map[net.Conn]bool{}, slots: map[string]chan struct{}{}, done: make(chan struct{})}
	for _, m := range c.Members() {
		if m.ID != c.NodeID() {
			x.slots[m.ID] = make(chan struct{}, 1)
		}
	}
	x.client = &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	x.server = &http.Server{Handler: http.HandlerFunc(x.handle), ReadHeaderTimeout: 500 * time.Millisecond, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	return x, nil
}

func (x *Peer) admit() bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.closed {
		return false
	}
	x.active.Add(1)
	return true
}

func (x *Peer) handle(w http.ResponseWriter, r *http.Request) {
	if !x.admit() {
		http.Error(w, "unavailable", 503)
		return
	}
	defer x.active.Done()
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if r.Host != x.cfg.Local().Peer || r.URL.RawQuery != "" || r.ProtoMajor != 1 || r.ProtoMinor != 1 {
		http.Error(w, "invalid request target", 400)
		return
	}
	var kind peerprotocol.Kind
	for _, k := range []peerprotocol.Kind{peerprotocol.Vote, peerprotocol.Append, peerprotocol.Snapshot} {
		if r.URL.Path == k.Path() {
			kind = k
		}
	}
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method", 405)
		return
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") || r.Header.Get("Content-Encoding") != "" {
		http.Error(w, "content type", 415)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, peerprotocol.MaxBody+1))
	if len(raw) > peerprotocol.MaxBody {
		http.Error(w, "body limit", 413)
		return
	}
	if err != nil {
		http.Error(w, "body", 400)
		return
	}
	msg, err := peerprotocol.DecodeRequest(x.cfg, kind, raw, x.cfg.NodeID())
	if err != nil {
		status := 400
		if errors.Is(err, peerprotocol.ErrIdentity) {
			status = 409
		}
		http.Error(w, "invalid RPC", status)
		return
	}
	if x.processor == nil {
		http.Error(w, "core unavailable", 503)
		return
	}
	reply, err := x.processor(ctx, msg)
	if err != nil || ctx.Err() != nil {
		http.Error(w, "processor unavailable", 503)
		return
	}
	validated, err := peerprotocol.DecodeResponse(x.cfg, msg, reply.Bytes())
	if err != nil {
		http.Error(w, "invalid processor response", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write(validated.Bytes())
}

// Send performs one attempt with a 300 ms deadline, including per-peer admission.
// Queue waiters remain owned by their callers; no retry or helper worker exists.
func (x *Peer) Send(ctx context.Context, msg peerprotocol.Message) (peerprotocol.Message, error) {
	if ctx == nil {
		return peerprotocol.Message{}, ErrHTTP
	}
	if !x.admit() {
		return peerprotocol.Message{}, ErrClosed
	}
	defer x.active.Done()
	env := msg.Envelope()
	member, ok := x.cfg.Member(env.To)
	slot := x.slots[env.To]
	if !ok || slot == nil || env.From != x.cfg.NodeID() {
		return peerprotocol.Message{}, peerprotocol.ErrIdentity
	}
	checked, err := peerprotocol.DecodeRequest(x.cfg, msg.Kind(), msg.Bytes(), env.To)
	if err != nil {
		return peerprotocol.Message{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	stop := context.AfterFunc(x.ctx, cancel)
	defer stop()
	select {
	case slot <- struct{}{}:
		defer func() { <-slot }()
	case <-ctx.Done():
		return peerprotocol.Message{}, ctx.Err()
	}
	if ctx.Err() != nil {
		return peerprotocol.Message{}, ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+member.Peer+checked.Kind().Path(), bytes.NewReader(checked.Bytes()))
	if err != nil {
		return peerprotocol.Message{}, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res, err := x.client.Do(req)
	if err != nil {
		return peerprotocol.Message{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "" {
		return peerprotocol.Message{}, ErrHTTP
	}
	media, params, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") {
		return peerprotocol.Message{}, ErrHTTP
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, peerprotocol.MaxBody+1))
	if err != nil {
		return peerprotocol.Message{}, err
	}
	if len(raw) > peerprotocol.MaxBody {
		return peerprotocol.Message{}, ErrHTTP
	}
	return peerprotocol.DecodeResponse(x.cfg, checked, raw)
}

type boundedListener struct {
	net.Listener
	p *Peer
}
type connection struct {
	net.Conn
	p    *Peer
	once sync.Once
}

func (c *connection) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.p.mu.Lock(); delete(c.p.connections, c); c.p.mu.Unlock() })
	return err
}
func (l boundedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		l.p.mu.Lock()
		if l.p.closed || len(l.p.connections) >= 16 {
			l.p.mu.Unlock()
			_ = c.Close()
			continue
		}
		wrapped := &connection{Conn: c, p: l.p}
		l.p.connections[wrapped] = true
		l.p.mu.Unlock()
		return wrapped, nil
	}
}

// Serve takes ownership of a matching, already bound loopback listener.
func (x *Peer) Serve(l net.Listener) error {
	if l == nil {
		return ErrHTTP
	}
	x.mu.Lock()
	if x.closed || x.started || l.Addr().String() != x.cfg.Local().Peer {
		x.mu.Unlock()
		_ = l.Close()
		return ErrClosed
	}
	x.started = true
	x.listener = l
	x.mu.Unlock()
	err := x.server.Serve(boundedListener{l, x})
	x.close()
	x.complete()
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
func (x *Peer) close() {
	x.mu.Lock()
	x.closed = true
	x.cancel()
	l := x.listener
	conns := make([]net.Conn, 0, len(x.connections))
	for c := range x.connections {
		conns = append(conns, c)
	}
	x.mu.Unlock()
	if l != nil {
		_ = l.Close()
	}
	for _, c := range conns {
		_ = c.Close()
	}
	_ = x.server.Close()
	x.pool.CloseIdleConnections()
}
func (x *Peer) complete() { x.finish.Do(func() { go func() { x.active.Wait(); close(x.done) }() }) }

// Shutdown cancels active I/O and processors, then waits for their completion.
// A timed-out caller may retry waiting; admission stays permanently closed.
func (x *Peer) Shutdown(ctx context.Context) error {
	x.close()
	x.mu.Lock()
	started := x.started
	x.mu.Unlock()
	if !started {
		x.complete()
	}
	select {
	case <-x.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
