// Package clientapi implements the M08 PUT/DELETE subset, without read barriers
// or replicated sessions. Every call is one independent attempt.
package clientapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/transport"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

const MaxBody = 48 * 1024

type Backend interface {
	Status() raft.Status
	Write(context.Context, string, json.RawMessage) (raft.WriteOutcome, error)
}
type Response struct {
	Code      string `json:"code"`
	RequestID string `json:"request_id"`
	Outcome   string `json:"outcome"`
	Index     string `json:"index,omitempty"`
	Applied   *bool  `json:"applied,omitempty"`
	Deleted   *bool  `json:"deleted,omitempty"`
	LeaderID  string `json:"leader_id,omitempty"`
	LeaderURL string `json:"leader_url,omitempty"`
}
type Server struct {
	cfg     config.Config
	backend Backend
	server  *http.Server
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	started bool
	active  sync.WaitGroup
	slots   chan struct{}
	prefix  string
	counter atomic.Uint64
	finish  sync.Once
	done    chan struct{}
}

func New(c config.Config, b Backend) (*Server, error) {
	if !c.Valid() || b == nil {
		return nil, raft.ErrInput
	}
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	x := &Server{done: make(chan struct{}), cfg: c, backend: b, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 64), prefix: hex.EncodeToString(seed[:])}
	x.server = &http.Server{Handler: x, ReadHeaderTimeout: 500 * time.Millisecond, ReadTimeout: 5 * time.Second, WriteTimeout: 6 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	return x, nil
}
func (x *Server) Serve(l net.Listener) error {
	if l == nil {
		return raft.ErrInput
	}
	if l.Addr().String() != x.cfg.Local().Client {
		_ = l.Close()
		return config.ErrConfig
	}
	x.mu.Lock()
	if x.closed || x.started {
		x.mu.Unlock()
		_ = l.Close()
		return raft.ErrClosed
	}
	x.started = true
	x.mu.Unlock()
	err := x.server.Serve(transport.LimitListener(l, 32))
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// Shutdown prevents new handler admission and cancels their waits. It does not
// cancel proposals already recording; the Raft owner retains that obligation.
func (x *Server) Shutdown(ctx context.Context) error {
	x.mu.Lock()
	x.closed = true
	x.cancel()
	x.mu.Unlock()
	_ = x.server.Close()
	// Close joins the bounded handlers via their own admission accounting.
	x.finish.Do(func() { go func() { x.active.Wait(); close(x.done) }() })
	select {
	case <-x.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (x *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	out := Response{RequestID: fmt.Sprintf("%s-%d", x.prefix, x.counter.Add(1)), Outcome: raft.NotAdmitted}
	send := func(status int, code string) {
		out.Code = code
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(out)
	}
	x.mu.Lock()
	closed := x.closed
	if !closed {
		x.active.Add(1)
	}
	x.mu.Unlock()
	if closed {
		send(503, "UNAVAILABLE")
		return
	}
	defer x.active.Done()
	if r.Host != x.cfg.Local().Client || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ProtoMajor != 1 || r.ProtoMinor != 1 {
		send(400, "INVALID_ARGUMENT")
		return
	}
	if r.URL.Path == "/v1/status" && r.Method == "GET" {
		s := x.backend.Status()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"node_id": x.cfg.NodeID(), "role": s.Role, "term": fmt.Sprint(s.Term), "commit_index": fmt.Sprint(s.CommitIndex), "last_applied": fmt.Sprint(s.LastApplied)})
		return
	}
	kind := ""
	switch r.URL.Path {
	case "/v1/kv/put":
		kind = "put"
	case "/v1/kv/delete":
		kind = "delete"
	default:
		send(404, "INVALID_ARGUMENT")
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		send(405, "INVALID_ARGUMENT")
		return
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(params) > 1 || (len(params) == 1 && params["charset"] != "utf-8") || r.Header.Get("Content-Encoding") != "" {
		send(415, "INVALID_ARGUMENT")
		return
	}
	select {
	case x.slots <- struct{}{}:
		defer func() { <-x.slots }()
	default:
		send(429, "OVERLOADED")
		return
	}
	if r.ContentLength > MaxBody {
		send(413, "REQUEST_TOO_LARGE")
		return
	}
	// Read deadline is also measured from admission, including a slow body.
	_ = http.NewResponseController(w).SetReadDeadline(started.Add(5 * time.Second))
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	if len(raw) > MaxBody {
		send(413, "REQUEST_TOO_LARGE")
		return
	}
	if err != nil {
		send(400, "INVALID_ARGUMENT")
		return
	}
	command, timeout, code := Decode(kind, raw)
	if code != "" {
		send(400, code)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), started.Add(timeout))
	defer cancel()
	if ctx.Err() != nil {
		send(503, "UNAVAILABLE")
		return
	}
	// Include explicit Shutdown cancellation even when invoked with a test handler.
	stop := context.AfterFunc(x.ctx, cancel)
	defer stop()
	result, err := x.backend.Write(ctx, kind, command)
	out.Outcome = result.Outcome
	if out.Outcome != raft.Completed {
		if out.Outcome != raft.NotAdmitted {
			out.Outcome = raft.Unknown
		}
		if errors.Is(err, context.DeadlineExceeded) {
			if out.Outcome == raft.Unknown {
				send(504, "TIMEOUT")
			} else {
				send(503, "UNAVAILABLE")
			}
			return
		}
		if errors.Is(err, raft.ErrNotLeader) && out.Outcome == raft.NotAdmitted {
			s := x.backend.Status()
			code := "NO_LEADER"
			if m, ok := x.cfg.Member(s.Leader); ok {
				code = "NOT_LEADER"
				out.LeaderID = m.ID
				out.LeaderURL = "http://" + m.Client
			}
			send(503, code)
			return
		}
		if errors.Is(err, raft.ErrFull) && out.Outcome == raft.NotAdmitted {
			send(429, "OVERLOADED")
			return
		}
		if errors.Is(err, wal.ErrFailed) || errors.Is(err, wal.ErrClosed) || errors.Is(err, wal.ErrLimit) {
			send(507, "STORAGE_UNAVAILABLE")
			return
		}
		send(503, "UNAVAILABLE")
		return
	}
	// A conservative last published-state check supplements the serialized owner
	// fence. A later, unobserved loss cannot revoke a legally committed operation.
	s := x.backend.Status()
	if err != nil || ctx.Err() != nil || s.Closed || s.Failure != nil || s.Role != raft.Leader || s.Term != result.Applied.Term || s.CommitIndex < result.Applied.Index || s.LastApplied < result.Applied.Index || result.Applied.Index == 0 {
		out.Outcome = raft.Unknown
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			send(504, "TIMEOUT")
		} else {
			send(503, "UNAVAILABLE")
		}
		return
	}
	out.Index = fmt.Sprint(result.Applied.Index)
	switch result.Applied.Result.Code {
	case statemachine.OK:
		if kind == "put" {
			v := result.Applied.Result.Applied
			if !v {
				out.Outcome = raft.Unknown
				out.Index = ""
				send(503, "UNAVAILABLE")
				return
			}
			out.Applied = &v
		} else {
			v := result.Applied.Result.Deleted
			out.Deleted = &v
		}
		send(200, "OK")
	case statemachine.ResourceLimit:
		send(409, "RESOURCE_LIMIT")
	default:
		out.Outcome = raft.Unknown
		out.Index = ""
		send(503, "UNAVAILABLE")
	}
}

// Decode validates the wire object then projects only persistent command fields.
func Decode(kind string, raw []byte) (json.RawMessage, time.Duration, string) {
	bad := func(code string) (json.RawMessage, time.Duration, string) { return nil, 0, code }
	o, err := strictjson.Decode(raw, MaxBody)
	if err != nil {
		return bad("INVALID_ARGUMENT")
	}
	fields := "key client_id sequence"
	if kind == "put" {
		fields += " value_b64"
	} else if kind != "delete" {
		return bad("INVALID_ARGUMENT")
	}
	if _, err = strictjson.Fields(o, fields, "timeout_ms"); err != nil {
		return bad("INVALID_ARGUMENT")
	}
	seq, err := strictjson.Integer(o["sequence"])
	if err != nil || seq < 1 || seq > 9007199254740991 {
		return bad("INVALID_SEQUENCE")
	}
	timeout := uint64(2000)
	if v, ok := o["timeout_ms"]; ok {
		timeout, err = strictjson.Integer(v)
		if err != nil || timeout < 50 || timeout > 5000 {
			return bad("INVALID_ARGUMENT")
		}
	}
	delete(o, "timeout_ms")
	command, _ := json.Marshal(o)
	entry, _ := json.Marshal(map[string]any{"index": "1", "term": "1", "kind": kind, "command": json.RawMessage(command)})
	if storageformat.ValidateEntry(entry) != nil {
		return bad("INVALID_ARGUMENT")
	}
	return command, time.Duration(timeout) * time.Millisecond, ""
}
