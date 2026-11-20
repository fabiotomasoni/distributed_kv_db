package clientapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	b, e := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration json.RawMessage }
	_ = json.Unmarshal(b, &f)
	c, e := config.Parse(f.Configuration)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

type backend struct {
	s     raft.Status
	fn    func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error)
	calls atomic.Int32
}

func (b *backend) Status() raft.Status { return b.s }
func (b *backend) Write(ctx context.Context, k string, c json.RawMessage) (raft.WriteOutcome, error) {
	b.calls.Add(1)
	return b.fn(ctx, k, c)
}
func healthy() *backend {
	return &backend{s: raft.Status{Role: raft.Leader, Term: 6, CommitIndex: 8, LastApplied: 8}, fn: func(_ context.Context, k string, _ json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: statemachine.OK, Applied: k == "put", Deleted: true}}}, nil
	}}
}
func input(k string) string {
	s := `{"key":"x","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1`
	if k == "put" {
		s += `,"value_b64":""`
	}
	return s + `}`
}
func request(t *testing.T, s *Server, method, path, raw string) (*httptest.ResponseRecorder, Response) {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+s.cfg.Local().Client+path, strings.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var out Response
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err, w.Body.String())
	}
	return w, out
}
func TestHTTPValidationAndProjection(t *testing.T) {
	b := healthy()
	s, _ := New(testConfig(t), b)
	defer s.Shutdown(context.Background())
	for _, tc := range []struct {
		raw  string
		code string
	}{
		{strings.Replace(input("put"), `"sequence":1`, `"sequence":1.0`, 1), "INVALID_SEQUENCE"},
		{strings.Replace(input("put"), `"sequence":1`, `"sequence":9007199254740992`, 1), "INVALID_SEQUENCE"},
		{strings.Replace(input("put"), `"sequence":1`, `"sequence":0`, 1), "INVALID_SEQUENCE"},
		{strings.Replace(input("put"), `"key":"x"`, `"key":"x","\u006bey":"y"`, 1), "INVALID_ARGUMENT"},
		{strings.Replace(input("put"), `"key":"x"`, `"Key":"x"`, 1), "INVALID_ARGUMENT"},
		{strings.Replace(input("put"), `"key":"x"`, `"key":"\ud800"`, 1), "INVALID_ARGUMENT"},
		{strings.Replace(input("put"), `"key":"x"`, `"key":"`+string([]byte{255})+`"`, 1), "INVALID_ARGUMENT"},
		{strings.Replace(input("put"), `"key":"x"`, `"key":" x"`, 1), "INVALID_ARGUMENT"},
		{strings.Replace(input("put"), `"value_b64":""`, `"value_b64":"AB=="`, 1), "INVALID_ARGUMENT"},
		{strings.Replace(input("put"), `"value_b64":""`, `"value_b64":null`, 1), "INVALID_ARGUMENT"},
		{input("put") + ` {}`, "INVALID_ARGUMENT"},
		{strings.TrimSuffix(input("put"), "}") + `,"timeout_ms":49}`, "INVALID_ARGUMENT"},
		{strings.TrimSuffix(input("put"), "}") + `,"timeout_ms":5001}`, "INVALID_ARGUMENT"},
		{strings.TrimSuffix(input("put"), "}") + `,"timeout_ms":1e3}`, "INVALID_ARGUMENT"},
		{strings.TrimSuffix(input("put"), "}") + `,"request_id":"external"}`, "INVALID_ARGUMENT"},
	} {
		w, o := request(t, s, "POST", "/v1/kv/put", tc.raw)
		if w.Code != 400 || o.Code != tc.code || o.Outcome != raft.NotAdmitted || o.RequestID == "" {
			t.Fatal(w.Code, o, tc)
		}
	}
	if b.calls.Load() != 0 {
		t.Fatal("invalid input reached raft")
	}
	for _, size := range []int{256, 257} {
		raw := strings.Replace(input("put"), `"key":"x"`, `"key":"`+strings.Repeat("a", size)+`"`, 1)
		w, _ := request(t, s, "POST", "/v1/kv/put", raw)
		want := 200
		if size > 256 {
			want = 400
		}
		if w.Code != want {
			t.Fatal(size, w.Code)
		}
	}
	for _, size := range []int{16384, 16385} {
		raw := strings.Replace(input("put"), `"value_b64":""`, `"value_b64":"`+base64.StdEncoding.EncodeToString(make([]byte, size))+`"`, 1)
		w, _ := request(t, s, "POST", "/v1/kv/put", raw)
		want := 200
		if size > 16384 {
			want = 400
		}
		if w.Code != want {
			t.Fatal(size, w.Code)
		}
	}
	for _, n := range []int{MaxBody, MaxBody + 1} {
		raw := input("put") + strings.Repeat(" ", n-len(input("put")))
		w, _ := request(t, s, "POST", "/v1/kv/put", raw)
		want := 200
		if n > MaxBody {
			want = 413
		}
		if w.Code != want {
			t.Fatal(n, w.Code)
		}
	}
	b.fn = func(ctx context.Context, k string, c json.RawMessage) (raft.WriteOutcome, error) {
		if bytes.Contains(c, []byte("timeout_ms")) {
			t.Fatal("transport field persisted")
		}
		d, ok := ctx.Deadline()
		if !ok || time.Until(d) > 50*time.Millisecond {
			t.Fatal("deadline missing")
		}
		return raft.WriteOutcome{Outcome: raft.NotAdmitted}, raft.ErrFull
	}
	w, o := request(t, s, "POST", "/v1/kv/put", strings.TrimSuffix(input("put"), "}")+`,"timeout_ms":50}`)
	if w.Code != 429 || o.Outcome != raft.NotAdmitted {
		t.Fatal(w.Code, o)
	}
}
func TestHTTPOutcomeMatrixAndLeadership(t *testing.T) {
	for _, tc := range []struct {
		out    string
		err    error
		status int
		code   string
	}{
		{raft.Unknown, context.DeadlineExceeded, 504, "TIMEOUT"}, {raft.NotAdmitted, context.DeadlineExceeded, 503, "UNAVAILABLE"},
		{raft.Unknown, wal.ErrFailed, 507, "STORAGE_UNAVAILABLE"}, {raft.NotAdmitted, wal.ErrLimit, 507, "STORAGE_UNAVAILABLE"},
		{raft.Unknown, raft.ErrNotLeader, 503, "UNAVAILABLE"}, {raft.NotAdmitted, raft.ErrNotLeader, 503, "NO_LEADER"},
		{raft.NotAdmitted, raft.ErrFull, 429, "OVERLOADED"}, {raft.Unknown, context.Canceled, 503, "UNAVAILABLE"},
		{raft.Unknown, errors.New("application failed"), 503, "UNAVAILABLE"},
	} {
		t.Run(tc.out+tc.code, func(t *testing.T) {
			b := healthy()
			b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
				return raft.WriteOutcome{Outcome: tc.out}, tc.err
			}
			s, _ := New(testConfig(t), b)
			defer s.Shutdown(context.Background())
			w, o := request(t, s, "POST", "/v1/kv/put", input("put"))
			if w.Code != tc.status || o.Code != tc.code || o.Outcome != tc.out || o.Index != "" {
				t.Fatal(w.Code, o)
			}
		})
	}
	b := healthy()
	s, _ := New(testConfig(t), b)
	defer s.Shutdown(context.Background())
	w, o := request(t, s, "POST", "/v1/kv/delete", input("delete"))
	if w.Code != 200 || o.Deleted == nil || !*o.Deleted || o.Index != "8" {
		t.Fatal(w.Code, o)
	}
	previous := o.RequestID
	b.s.Role = raft.Follower
	w, o = request(t, s, "POST", "/v1/kv/put", input("put"))
	if w.Code != 503 || o.Outcome != raft.Unknown || o.RequestID == previous {
		t.Fatal(w.Code, o)
	}
	b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.NotAdmitted}, raft.ErrNotLeader
	}
	b.s.Leader = "n2"
	w, o = request(t, s, "POST", "/v1/kv/put", input("put"))
	m, _ := s.cfg.Member("n2")
	if o.Code != "NOT_LEADER" || o.LeaderURL != "http://"+m.Client || w.Header().Get("Location") != "" {
		t.Fatal(o)
	}
	b.s.Leader = "invented"
	_, o = request(t, s, "POST", "/v1/kv/put", input("put"))
	if o.LeaderURL != "" || o.Code != "NO_LEADER" {
		t.Fatal(o)
	}
}
func TestHTTPRoutesAdmissionAndShutdown(t *testing.T) {
	b := healthy()
	s, _ := New(testConfig(t), b)
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"POST", "/v1/kv/get", 404}, {"POST", "/v1/kv/cas", 404}, {"GET", "/v1/kv/put", 405}, {"POST", "/v1/kv/put?x=1", 400}} {
		w, _ := request(t, s, tc.method, tc.path, input("put"))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://wrong/v1/kv/put", strings.NewReader(input("put")))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	r.Host = s.cfg.Local().Client
	r.Header.Set("Content-Encoding", "gzip")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 64; i++ {
		s.slots <- struct{}{}
	}
	w, o := request(t, s, "POST", "/v1/kv/put", input("put"))
	if w.Code != 429 || o.Outcome != raft.NotAdmitted {
		t.Fatal(w.Code, o)
	}
	for i := 0; i < 64; i++ {
		<-s.slots
	}
	w, _ = request(t, s, "GET", "/v1/status", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	w, o = request(t, s, "POST", "/v1/kv/put", input("put"))
	if w.Code != 503 || o.Outcome != raft.NotAdmitted {
		t.Fatal(w.Code, o)
	}
}
func TestClientRejectsForgedHintsAndResultShapes(t *testing.T) {
	c, _ := NewClient(testConfig(t))
	defer c.Close()
	for _, raw := range []string{
		`{"code":"NOT_LEADER","request_id":"r","outcome":"not_admitted","leader_id":"n2","leader_url":"http://evil/"}`,
		`{"code":"OK","request_id":"r","outcome":"completed","index":"1","applied":false}`,
		`{"code":"OK","request_id":"r","outcome":"completed","index":"01","applied":true}`,
		`{"code":"TIMEOUT","request_id":"r","outcome":"not_admitted"}`,
		`{"code":"TIMEOUT","request_id":"r","outcome":"unknown","index":"1"}`,
	} {
		var o map[string]any
		_ = json.Unmarshal([]byte(raw), &o)
		status := 504
		if o["code"] == "OK" {
			status = 200
		}
		if o["code"] == "NOT_LEADER" {
			status = 503
		}
		if _, e := c.decodeResponse("put", status, []byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	for _, kind := range []string{"put", "delete"} {
		extra := `"applied":true`
		if kind == "delete" {
			extra = `"deleted":false`
		}
		if _, e := c.decodeResponse(kind, 200, []byte(fmt.Sprintf(`{"code":"OK","request_id":"r","outcome":"completed","index":"1",%s}`, extra))); e != nil {
			t.Fatal(e)
		}
	}
}

func TestSingleAttemptLostResponseAndDisconnectAfterRecording(t *testing.T) {
	// A real socket closes AFTER application and BEFORE any response bytes. The
	// test client must report uncertainty and must issue exactly one request.
	var api *Server
	dropped := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/kv/put" {
			recorded := httptest.NewRecorder()
			api.ServeHTTP(recorded, r)
			if recorded.Code != 200 {
				t.Errorf("write did not complete before loss: %d", recorded.Code)
			}
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			dropped <- struct{}{}
			return
		}
		api.ServeHTTP(w, r)
	}))
	defer srv.Close()
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration map[string]any }
	_ = json.Unmarshal(raw, &f)
	addr := strings.TrimPrefix(srv.URL, "http://")
	f.Configuration["client_addr"] = addr
	f.Configuration["members"].([]any)[0].(map[string]any)["client"] = addr
	encoded, _ := json.Marshal(f.Configuration)
	cfg, e := config.Parse(encoded)
	if e != nil {
		t.Fatal(e)
	}
	b := healthy()
	machine := statemachine.New()
	b.fn = func(_ context.Context, kind string, _ json.RawMessage) (raft.WriteOutcome, error) {
		op := statemachine.Put
		if kind == "delete" {
			op = statemachine.Delete
		}
		result := machine.Apply(statemachine.Command{Operation: op, Key: "x"})
		return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: result}}, nil
	}
	api, e = New(cfg, b)
	if e != nil {
		t.Fatal(e)
	}
	defer api.Shutdown(context.Background())
	client, _ := NewClient(cfg)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	o, _, e := client.Write(ctx, "n1", "put", []byte(input("put")))
	if e == nil || o.Outcome != raft.Unknown {
		t.Fatal(o, e)
	}
	select {
	case <-dropped:
	case <-ctx.Done():
		t.Fatal("no lost-response boundary")
	}
	if b.calls.Load() != 1 {
		t.Fatal("uncertain request was retried", b.calls.Load())
	}
	o, status, e := client.Write(ctx, "n1", "delete", []byte(input("delete")))
	if e != nil || status != 200 || o.Deleted == nil || !*o.Deleted {
		t.Fatal("lost PUT did not take effect", o, e)
	}
	o, status, e = client.Write(ctx, "n1", "delete", []byte(input("delete")))
	if e != nil || status != 200 || o.Deleted == nil || *o.Deleted {
		t.Fatal("duplicate was not an independent attempt", o, e)
	}
}

func TestHTTPShutdownCancelsRecordedWaitAndJoinsHandler(t *testing.T) {
	b := healthy()
	entered := make(chan struct{})
	joined := make(chan struct{})
	b.fn = func(ctx context.Context, _ string, _ json.RawMessage) (raft.WriteOutcome, error) {
		close(entered)
		<-ctx.Done()
		return raft.WriteOutcome{Outcome: raft.Unknown}, ctx.Err()
	}
	s, e := New(testConfig(t), b)
	if e != nil {
		t.Fatal(e)
	}
	go func() {
		w, o := request(t, s, "POST", "/v1/kv/put", input("put"))
		if w.Code != 503 || o.Outcome != raft.Unknown {
			t.Error(w.Code, o)
		}
		close(joined)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("handler not admitted")
	}
	if e = s.Shutdown(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-joined:
	case <-ctx.Done():
		t.Fatal("handler remained blocked")
	}
	// Repeated shutdown waits share one bounded completion signal.
	if e = s.Shutdown(ctx); e != nil {
		t.Fatal(e)
	}
}
