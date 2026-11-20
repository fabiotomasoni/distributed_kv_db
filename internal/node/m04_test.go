package node

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/transport"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func fixture(t *testing.T) (map[string]any, string, string) {
	t.Helper()
	raw, err := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Configuration  map[string]any
		SharedPreimage string `json:"shared_preimage"`
		Hash           string `json:"config_hash"`
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f.Configuration, f.SharedPreimage, f.Hash
}
func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func parse(t *testing.T, m map[string]any) config.Config {
	t.Helper()
	c, err := config.Parse(encode(m))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}
func private(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Chmod(d, 0700); err != nil {
		t.Fatal(err)
	}
	return d
}
func listen(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}
func configs(t *testing.T) ([3]config.Config, [3]net.Listener) {
	t.Helper()
	m, _, _ := fixture(t)
	a := m["members"].([]any)
	var ls [3]net.Listener
	for i := range ls {
		ls[i] = listen(t)
		a[i].(map[string]any)["peer"] = ls[i].Addr().String()
		client := listen(t)
		a[i].(map[string]any)["client"] = client.Addr().String()
	}
	var out [3]config.Config
	for i := range out {
		member := a[i].(map[string]any)
		m["node_id"] = member["id"]
		m["peer_addr"] = member["peer"]
		m["client_addr"] = member["client"]
		m["data_dir"] = private(t)
		out[i] = parse(t, m)
	}
	return out, ls
}
func vote(t *testing.T, c config.Config, from, to, id string) peerprotocol.Message {
	t.Helper()
	m := map[string]any{"protocol_version": 1, "cluster_id": c.ClusterID(), "config_hash": c.Hash(), "from": from, "to": to, "rpc_id": id, "term": "3", "candidate_id": from, "last_log_index": "0", "last_log_term": "0"}
	p, err := peerprotocol.DecodeRequest(c, peerprotocol.Vote, encode(m), to)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func reply(c config.Config, r peerprotocol.Message) (peerprotocol.Message, error) {
	var m map[string]any
	_ = json.Unmarshal(r.Bytes(), &m)
	delete(m, "candidate_id")
	delete(m, "last_log_index")
	delete(m, "last_log_term")
	m["from"], m["to"] = m["to"], m["from"]
	m["vote_granted"] = false
	return peerprotocol.DecodeResponse(c, r, encode(m))
}

func TestConfigurationCanonicalFixtureAndRejections(t *testing.T) {
	m, pre, h := fixture(t)
	c := parse(t, m)
	if c.Hash() != h || string(c.SharedBytes()) != pre {
		t.Fatal("canonical fixture mismatch")
	}
	a := m["members"].([]any)
	a[0], a[2] = a[2], a[0]
	m["node_id"] = "n2"
	m["client_addr"] = "127.0.0.1:7402"
	m["peer_addr"] = "127.0.0.1:7502"
	m["data_dir"] = "other"
	m["heartbeat_ms"] = 101
	if parse(t, m).Hash() != h {
		t.Fatal("local/permutation variation changed shared hash")
	}
	members := c.Members()
	members[0].ID = "mutated"
	if c.Members()[0].ID != "n1" {
		t.Fatal("mutable config")
	}
	for name, mutate := range map[string]func(map[string]any){"version": func(m map[string]any) { m["config_version"] = 2 }, "quota": func(m map[string]any) { m["max_keys"] = 1 }, "public": func(m map[string]any) { m["peer_addr"] = "0.0.0.0:7501" }, "path": func(m map[string]any) { m["data_dir"] = "../data" }, "timer": func(m map[string]any) { m["election_min_ms"] = 1 }, "unknown": func(m map[string]any) { m["other"] = 1 }, "duplicate": func(m map[string]any) { a := m["members"].([]any); a[1] = a[0] }, "alias": func(m map[string]any) {
		a := m["members"].([]any)
		a[1].(map[string]any)["client"] = "[::ffff:127.0.0.1]:7401"
	}} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := fixture(t)
			mutate(m)
			if _, err := config.Parse(encode(m)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	for _, s := range []string{"localhost:12", "0.0.0.0:1", "127.0.0.1:0", "127.0.0.1:01", "[::]:1", "[::1%lo0]:1", "8.8.8.8:1"} {
		if config.Address(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"127.0.0.1:1", "[::1]:65535"} {
		if !config.Address(s) {
			t.Fatal(s)
		}
	}
}

func TestProtocolFixturesAndInvalidMessages(t *testing.T) {
	m, _, _ := fixture(t)
	c := parse(t, m)
	raw, err := os.ReadFile("../../docs/fixtures/peer-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Kind              peerprotocol.Kind
		Request, Response string
	}
	if err = json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		req, err := peerprotocol.DecodeRequest(c, v.Kind, []byte(v.Request), "n2")
		if err != nil {
			t.Fatal(err)
		}
		if string(req.Bytes()) != v.Request {
			t.Fatal("request fixture")
		}
		res, err := peerprotocol.DecodeResponse(c, req, []byte(v.Response))
		if err != nil || string(res.Bytes()) != v.Response {
			t.Fatal("reply fixture", err)
		}
		if _, err := peerprotocol.DecodeResponse(c, res, []byte(v.Response)); !errors.Is(err, peerprotocol.ErrMessage) {
			t.Fatal("response accepted as request context", err)
		}
		b := req.Bytes()
		b[0] = 0
		if req.Bytes()[0] != '{' {
			t.Fatal("ownership")
		}
	}
	base := vectors[0].Request
	for name, raw := range map[string]string{"duplicate": strings.TrimSuffix(base, "}") + `,"term":"3"}`, "case": strings.Replace(base, `"term"`, `"Term"`, 1), "numeric": strings.Replace(base, `"term":"3"`, `"term":3`, 1), "leading": strings.Replace(base, `"term":"3"`, `"term":"03"`, 1), "cluster": strings.Replace(base, strings.Repeat("1", 32), strings.Repeat("2", 32), 1), "hash": strings.Replace(base, c.Hash(), strings.Repeat("a", 64), 1), "candidate": strings.Replace(base, `"candidate_id":"n1"`, `"candidate_id":"n3"`, 1), "concatenated": base + `{}`, "version": strings.Replace(base, `"protocol_version":1`, `"protocol_version":2`, 1)} {
		t.Run(name, func(t *testing.T) {
			if _, err := peerprotocol.DecodeRequest(c, peerprotocol.Vote, []byte(raw), "n2"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	req, _ := peerprotocol.DecodeRequest(c, peerprotocol.Vote, []byte(base), "n2")
	if _, err := peerprotocol.DecodeResponse(c, req, []byte(strings.Replace(vectors[0].Response, "n1-42", "stale", 1))); !errors.Is(err, peerprotocol.ErrIdentity) {
		t.Fatal("correlation", err)
	}
	var appendBody map[string]any
	_ = json.Unmarshal([]byte(vectors[1].Request), &appendBody)
	for name, mutate := range map[string]func(map[string]any){"gap": func(m map[string]any) { m["entries"].([]any)[0].(map[string]any)["index"] = "2" }, "futureterm": func(m map[string]any) { m["entries"].([]any)[0].(map[string]any)["term"] = "4" }, "null": func(m map[string]any) { m["entries"] = nil }, "too many": func(m map[string]any) {
		e := m["entries"].([]any)[0]
		a := make([]any, 17)
		for i := range a {
			a[i] = e
		}
		m["entries"] = a
	}} {
		t.Run(name, func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(encode(appendBody), &m)
			mutate(m)
			if _, err := peerprotocol.DecodeRequest(c, peerprotocol.Append, encode(m), "n2"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	// Legal alternative wire escapes/order normalize into owned canonical bytes.
	alternative := strings.Replace(base, `"candidate_id":"n1"`, `"candidate_id":"\u006e1"`, 1)
	if p, err := peerprotocol.DecodeRequest(c, peerprotocol.Vote, []byte(alternative), "n2"); err != nil || !bytes.Equal(p.Bytes(), req.Bytes()) {
		t.Fatal("wire escape", err)
	}
}

func TestThreeNodeStartupIdentityAndLock(t *testing.T) {
	cs, ls := configs(t)
	var ns [3]*Node
	for i := range ns {
		var err error
		ns[i], err = start(cs[i], true, nil, func(string, string) (net.Listener, error) { return ls[i], nil })
		if err != nil {
			t.Fatal(err)
		}
		n := ns[i]
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
		if n.Recovered().LastIndex() != 0 {
			t.Fatal("unexpected applied/recovered data")
		}
	}
	called := false
	factory := func(string, string) (net.Listener, error) { called = true; return nil, errors.New("should not bind") }
	if _, err := start(cs[0], false, nil, factory); err == nil || called {
		t.Fatal("second owner reached listener", err)
	}
	if err := ns[0].Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	m, _, _ := fixture(t)
	m["data_dir"] = cs[0].DataDir()
	m["cluster_id"] = strings.Repeat("2", 32)
	if _, err := start(parse(t, m), false, nil, factory); err == nil || called {
		t.Fatal("identity mismatch reached listener")
	}
	// A bind failure must release ownership without erasing initialized storage.
	if _, err := start(cs[0], false, nil, func(string, string) (net.Listener, error) { return nil, errors.New("injected bind failure") }); err == nil {
		t.Fatal("bind failure")
	}
	s, err := wal.Open(cs[0].DataDir(), wal.Identity{ClusterID: cs[0].ClusterID(), NodeID: cs[0].NodeID(), ConfigHash: cs[0].Hash()})
	if err != nil {
		t.Fatal("lock leaked", err)
	}
	_ = s.Close()
	if _, err = os.Stat(filepath.Join(cs[0].DataDir(), "CURRENT")); err != nil {
		t.Fatal(err)
	}
}

func TestTransportRoundTripValidationCancellationAndShutdown(t *testing.T) {
	cs, ls := configs(t)
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	blocking := atomic.Bool{}
	p, err := transport.New(cs[1], func(c context.Context, m peerprotocol.Message) (peerprotocol.Message, error) {
		calls.Add(1)
		if blocking.Load() {
			entered <- struct{}{}
			<-c.Done()
			<-release
			return peerprotocol.Message{}, c.Err()
		}
		return reply(cs[1], m)
	})
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- p.Serve(ls[1]) }()
	t.Cleanup(func() { _ = p.Shutdown(ctx(t)) })
	sender, _ := transport.New(cs[0], nil)
	t.Cleanup(func() { _ = sender.Shutdown(ctx(t)) })
	msg := vote(t, cs[0], "n1", "n2", "test-1")
	if _, err = sender.Send(ctx(t), msg); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	for name, tc := range map[string]struct {
		body, method, ctype, host string
		status                    int
	}{"badjson": {"{", "POST", "application/json", "", 400}, "identity": {strings.Replace(string(msg.Bytes()), cs[0].Hash(), strings.Repeat("a", 64), 1), "POST", "application/json", "", 409}, "oversize": {strings.Repeat(" ", peerprotocol.MaxBody+1), "POST", "application/json", "", 413}, "method": {"{}", "GET", "application/json", "", 405}, "media": {"{}", "POST", "text/plain", "", 415}, "host": {"{}", "POST", "application/json", "localhost", 400}} {
		t.Run(name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(ctx(t), tc.method, "http://"+cs[1].Local().Peer+peerprotocol.Vote.Path(), strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.ctype)
			if tc.host != "" {
				req.Host = tc.host
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.StatusCode != tc.status {
				t.Fatal(res.StatusCode, tc.status)
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatal("invalid input reached processor", calls.Load())
	}
	blocking.Store(true)
	sendDone := make(chan error, 1)
	cancelCtx, cancel := context.WithCancel(ctx(t))
	go func() { _, err := sender.Send(cancelCtx, msg); sendDone <- err }()
	select {
	case <-entered:
	case <-ctx(t).Done():
		t.Fatal("processor not entered")
	}
	// The same peer's admission is canceled without starting another RPC.
	canceled, cc := context.WithCancel(ctx(t))
	cc()
	if _, err := sender.Send(canceled, msg); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cancel()
	if err := <-sendDone; err == nil {
		t.Fatal("cancellation lost")
	}
	short, stop := context.WithCancel(context.Background())
	stop()
	if err := p.Shutdown(short); !errors.Is(err, context.Canceled) {
		t.Fatal("should still drain", err)
	}
	close(release)
	if err = p.Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if err = <-serveDone; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("hidden retry", calls.Load())
	}
	if _, err = sender.Send(ctx(t), msg); err == nil {
		t.Fatal("closed listener")
	}
	if err = sender.Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if _, err = sender.Send(ctx(t), msg); !errors.Is(err, transport.ErrClosed) {
		t.Fatal(err)
	}
}

func TestNodeShutdownKeepsLockUntilProcessorDrains(t *testing.T) {
	cs, ls := configs(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	n, err := start(cs[1], true, func(c context.Context, m peerprotocol.Message) (peerprotocol.Message, error) {
		close(entered)
		<-c.Done()
		<-release
		return peerprotocol.Message{}, c.Err()
	}, func(string, string) (net.Listener, error) { return ls[1], nil })
	if err != nil {
		t.Fatal(err)
	}
	sender, _ := transport.New(cs[0], nil)
	defer sender.Shutdown(ctx(t))
	sent := make(chan error, 1)
	go func() { _, err := sender.Send(ctx(t), vote(t, cs[0], "n1", "n2", "shutdown")); sent <- err }()
	select {
	case <-entered:
	case <-ctx(t).Done():
		t.Fatal("not entered")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_ = n.Shutdown(canceled)
	id := wal.Identity{ClusterID: cs[1].ClusterID(), NodeID: "n2", ConfigHash: cs[1].Hash()}
	if s, err := wal.Open(cs[1].DataDir(), id); err == nil {
		_ = s.Close()
		t.Fatal("lock released before drain")
	}
	close(release)
	if err = n.Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	if err = <-sent; err == nil {
		t.Fatal("shutdown request succeeded")
	}
	s, err := wal.Open(cs[1].DataDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

func TestProtocolSizeAndSnapshotBoundaries(t *testing.T) {
	m, _, _ := fixture(t)
	c := parse(t, m)
	raw, _ := os.ReadFile("../../docs/fixtures/peer-v1.json")
	var v []struct {
		Kind              peerprotocol.Kind
		Request, Response string
	}
	_ = json.Unmarshal(raw, &v)
	var body map[string]any
	_ = json.Unmarshal([]byte(v[1].Request), &body)
	entries := make([]any, 16)
	for i := range entries {
		entries[i] = map[string]any{"index": fmt.Sprint(i + 1), "term": "3", "kind": "put", "command": map[string]any{"key": "k", "value_b64": base64.StdEncoding.EncodeToString(make([]byte, 16384)), "client_id": strings.Repeat("a", 32), "sequence": i + 1}}
	}
	body["entries"] = entries
	// Individually valid entries and body below 384 KiB exceed 256 KiB commands.
	if len(encode(body)) >= peerprotocol.MaxBody {
		t.Fatal("test accidentally checks outer cap")
	}
	if _, err := peerprotocol.DecodeRequest(c, peerprotocol.Append, encode(body), "n2"); err == nil {
		t.Fatal("command aggregate cap")
	}
	body["entries"] = entries[:1]
	if _, err := peerprotocol.DecodeRequest(c, peerprotocol.Append, encode(body), "n2"); err != nil {
		t.Fatal("single max value", err)
	}
	body["prev_log_index"] = "18446744073709551615"
	body["prev_log_term"] = "3"
	if _, err := peerprotocol.DecodeRequest(c, peerprotocol.Append, encode(body), "n2"); err == nil {
		t.Fatal("index wrap")
	}
	for name, mutate := range map[string]func(map[string]any){"image": func(m map[string]any) { m["total_bytes"] = "8388609" }, "chunk": func(m map[string]any) { m["data_b64"] = base64.StdEncoding.EncodeToString(make([]byte, 65537)) }, "offset": func(m map[string]any) { m["offset"] = "45" }, "done": func(m map[string]any) { m["done"] = false }, "base64": func(m map[string]any) { m["data_b64"] = m["data_b64"].(string) + "\n" }, "digest": func(m map[string]any) { m["snapshot_id"] = strings.Repeat("a", 64) }} {
		t.Run(name, func(t *testing.T) {
			var b map[string]any
			_ = json.Unmarshal([]byte(v[2].Request), &b)
			mutate(b)
			if _, err := peerprotocol.DecodeRequest(c, peerprotocol.Snapshot, encode(b), "n2"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	req, _ := peerprotocol.DecodeRequest(c, peerprotocol.Append, []byte(v[1].Request), "n2")
	if _, err := peerprotocol.DecodeResponse(c, req, []byte(strings.Replace(v[1].Response, `"match_index":"1"`, `"match_index":"2"`, 1))); err == nil {
		t.Fatal("wrong match index")
	}
	// Negative AppendEntries uses a separate exact response union.
	var response map[string]any
	_ = json.Unmarshal([]byte(v[1].Response), &response)
	delete(response, "match_index")
	response["success"] = false
	response["conflict_index"] = "1"
	response["conflict_term"] = "2"
	if _, err := peerprotocol.DecodeResponse(c, req, encode(response)); err != nil {
		t.Fatal(err)
	}
	response["conflict_index"] = "0"
	if _, err := peerprotocol.DecodeResponse(c, req, encode(response)); err == nil {
		t.Fatal("zero conflict")
	}
}

type failedListener struct {
	net.Listener
	failure error
}

func (l failedListener) Accept() (net.Conn, error) { return nil, l.failure }
func TestListenerFailureDrainsAndReleasesStorage(t *testing.T) {
	cs, ls := configs(t)
	injected := errors.New("injected accept failure")
	n, err := start(cs[0], true, nil, func(string, string) (net.Listener, error) { return failedListener{ls[0], injected}, nil })
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-n.done:
	case <-ctx(t).Done():
		t.Fatal("failed listener left node running")
	}
	if err = n.Shutdown(ctx(t)); !errors.Is(err, injected) {
		t.Fatal("serve failure hidden", err)
	}
	s, err := wal.Open(cs[0].DataDir(), wal.Identity{ClusterID: cs[0].ClusterID(), NodeID: cs[0].NodeID(), ConfigHash: cs[0].Hash()})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}
