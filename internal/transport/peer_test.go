package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
)

func setup(t *testing.T) (config.Config, net.Listener) {
	t.Helper()
	raw, err := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Configuration map[string]any }
	_ = json.Unmarshal(raw, &f)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f.Configuration["members"].([]any)[1].(map[string]any)["peer"] = l.Addr().String()
	b, _ := json.Marshal(f.Configuration)
	c, err := config.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return c, l
}
func request(t *testing.T, c config.Config) peerprotocol.Message {
	t.Helper()
	m := map[string]any{"protocol_version": 1, "cluster_id": c.ClusterID(), "config_hash": c.Hash(), "from": "n1", "to": "n2", "rpc_id": "probe", "term": "1", "candidate_id": "n1", "last_log_index": "0", "last_log_term": "0"}
	b, _ := json.Marshal(m)
	r, err := peerprotocol.DecodeRequest(c, peerprotocol.Vote, b, "n2")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func deadline(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestClientTimeoutMalformedResponseAndNoRetry(t *testing.T) {
	c, l := setup(t)
	p, _ := New(c, nil)
	defer p.Shutdown(deadline(t))
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	mode := atomic.Int32{}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		switch mode.Load() {
		case 0:
			entered <- struct{}{}
			<-r.Context().Done()
		case 1:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"invalid":true}`))
		case 2:
			w.Header().Set("Location", "http://127.0.0.1:1/")
			w.WriteHeader(307)
		case 3:
			w.WriteHeader(503)
		}
	})}
	done := make(chan error, 1)
	go func() { done <- server.Serve(l) }()
	defer func() { _ = server.Close(); <-done }()
	result := make(chan error, 1)
	go func() { _, err := p.Send(deadline(t), request(t, c)); result <- err }()
	select {
	case <-entered:
	case <-deadline(t).Done():
		t.Fatal("not entered")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-deadline(t).Done():
		t.Fatal("missing 300ms deadline")
	}
	for _, m := range []int32{1, 2, 3} {
		mode.Store(m)
		if _, err := p.Send(deadline(t), request(t, c)); err == nil {
			t.Fatal("accepted response", m)
		}
	}
	if calls.Load() != 4 {
		t.Fatal("hidden retries", calls.Load())
	}
}

// A finite listener injects already accepted sockets without consuming OS ports.
type finiteListener struct{ items []net.Conn }

func (l *finiteListener) Accept() (net.Conn, error) {
	if len(l.items) == 0 {
		return nil, net.ErrClosed
	}
	c := l.items[0]
	l.items = l.items[1:]
	return c, nil
}
func (l *finiteListener) Close() error   { return nil }
func (l *finiteListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1} }
func TestAcceptedConnectionCapAndRelease(t *testing.T) {
	// Config parsing itself needs no socket; reuse the fixture directly.
	raw, err := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Configuration json.RawMessage }
	_ = json.Unmarshal(raw, &f)
	c, err := config.Parse(f.Configuration)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := New(c, nil)
	defer p.Shutdown(deadline(t))
	var sides []net.Conn
	l := &finiteListener{}
	for i := 0; i < 17; i++ {
		a, b := net.Pipe()
		l.items = append(l.items, a)
		sides = append(sides, b)
		defer b.Close()
	}
	bl := boundedListener{l, p}
	var admitted []net.Conn
	for i := 0; i < 16; i++ {
		c, err := bl.Accept()
		if err != nil {
			t.Fatal(err)
		}
		admitted = append(admitted, c)
	}
	if _, err := bl.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := sides[16].Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("17th connection not rejected", err)
	}
	if len(p.connections) != 16 {
		t.Fatal("cap accounting")
	}
	_ = admitted[0].Close()
	if len(p.connections) != 15 {
		t.Fatal("not released")
	}
	a, b := net.Pipe()
	defer b.Close()
	l.items = append(l.items, a)
	replacement, err := bl.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_ = replacement.Close()
	for _, c := range admitted {
		_ = c.Close()
	}
}

func TestLifecycleBeforeServeAndInvalidProcessor(t *testing.T) {
	c, l := setup(t)
	p, _ := New(c, nil)
	if err := p.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	if err := p.Serve(l); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := New(config.Config{}, nil); err == nil {
		t.Fatal("zero config")
	}
	// A processor cannot send an unvalidated/fabricated zero message.
	raw, _ := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	var f struct{ Configuration map[string]any }
	_ = json.Unmarshal(raw, &f)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	f.Configuration["members"].([]any)[1].(map[string]any)["peer"] = ln.Addr().String()
	f.Configuration["node_id"] = "n2"
	f.Configuration["peer_addr"] = ln.Addr().String()
	f.Configuration["client_addr"] = "127.0.0.1:7402"
	b, _ := json.Marshal(f.Configuration)
	receiver, err := config.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	server, _ := New(receiver, func(context.Context, peerprotocol.Message) (peerprotocol.Message, error) {
		return peerprotocol.Message{}, nil
	})
	done := make(chan error, 1)
	go func() { done <- server.Serve(ln) }()
	defer func() { _ = server.Shutdown(deadline(t)); <-done }()
	f.Configuration["node_id"] = "n1"
	f.Configuration["peer_addr"] = "127.0.0.1:7501"
	f.Configuration["client_addr"] = "127.0.0.1:7401"
	b, _ = json.Marshal(f.Configuration)
	senderConfig, err := config.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	sender, _ := New(senderConfig, nil)
	defer sender.Shutdown(deadline(t))
	if _, err := sender.Send(deadline(t), request(t, senderConfig)); !errors.Is(err, ErrHTTP) {
		t.Fatal(err)
	}
}
