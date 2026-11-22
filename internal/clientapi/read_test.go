package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type readBackendFake struct {
	*backend
	read func(context.Context, json.RawMessage) (raft.WriteOutcome, error)
}

func (b *readBackendFake) Read(ctx context.Context, raw json.RawMessage) (raft.WriteOutcome, error) {
	b.calls.Add(1)
	return b.read(ctx, raw)
}
func healthyRead() *readBackendFake {
	b := &readBackendFake{backend: healthy()}
	b.read = func(context.Context, json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: statemachine.OK, Exists: true, Value: []byte{0, 255}}}}, nil
	}
	return b
}
func TestHTTPLoggedGETContractAndStrictValidation(t *testing.T) {
	b := healthyRead()
	s, e := New(testConfig(t), b)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown(context.Background())
	w, o := request(t, s, "POST", "/v1/kv/get", `{"key":"x"}`)
	if w.Code != 200 || o.Outcome != raft.Completed || o.Exists == nil || !*o.Exists || o.ValueB64 == nil || *o.ValueB64 != "AP8=" || o.Index != "8" || o.Applied != nil {
		t.Fatal(w.Code, o)
	}
	for _, raw := range []string{`{}`, `{"key":"x","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, `{"key":"x","sequence":1}`, `{"key":"x","key":"y"}`, `{"Key":"x"}`, `{"key":"\ud800"}`, `{"key":""}`, `{"key":"x","value_b64":""}`, `{"key":"x","timeout_ms":49}`, `{"key":"x","timeout_ms":5001}`, `{"key":"x","timeout_ms":50.0}`} {
		calls := b.calls.Load()
		w, o = request(t, s, "POST", "/v1/kv/get", raw)
		if w.Code != 400 || o.Outcome != raft.NotAdmitted || b.calls.Load() != calls {
			t.Fatal(raw, w.Code, o)
		}
	}
	for _, n := range []int{MaxBody, MaxBody + 1} {
		raw := `{"key":"x"}` + strings.Repeat(" ", n-len(`{"key":"x"}`))
		w, _ = request(t, s, "POST", "/v1/kv/get", raw)
		want := 200
		if n > MaxBody {
			want = 413
		}
		if w.Code != want {
			t.Fatal(n, w.Code)
		}
	}
	b.read = func(context.Context, json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: statemachine.KeyNotFound}}}, nil
	}
	w, o = request(t, s, "POST", "/v1/kv/get", `{"key":"missing"}`)
	if w.Code != 404 || o.Outcome != raft.Completed || o.Exists == nil || *o.Exists || o.ValueB64 != nil {
		t.Fatal(w.Code, o)
	}
	b.read = func(context.Context, json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.Unknown}, context.DeadlineExceeded
	}
	w, o = request(t, s, "POST", "/v1/kv/get", `{"key":"x","timeout_ms":50}`)
	if w.Code != 504 || o.Outcome != raft.Unknown || o.Exists != nil || o.ValueB64 != nil {
		t.Fatal(w.Code, o)
	}
	w, _ = request(t, s, "GET", "/v1/kv/get", "")
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}
func TestGETClientResponseShapeAndNoSession(t *testing.T) {
	c, _ := NewClient(testConfig(t))
	defer c.Close()
	for _, tc := range []struct {
		status int
		raw    string
		valid  bool
	}{
		{200, `{"code":"OK","request_id":"r","outcome":"completed","index":"8","exists":true,"value_b64":""}`, true},
		{404, `{"code":"KEY_NOT_FOUND","request_id":"r","outcome":"completed","index":"8","exists":false}`, true},
		{200, `{"code":"OK","request_id":"r","outcome":"completed","index":"8","exists":true}`, false},
		{200, `{"code":"OK","request_id":"r","outcome":"completed","index":"8","exists":null,"value_b64":""}`, false},
		{200, `{"code":"OK","request_id":"r","outcome":"completed","index":"8","exists":false,"value_b64":""}`, false},
		{404, `{"code":"KEY_NOT_FOUND","request_id":"r","outcome":"completed","index":"8","exists":false,"value_b64":null}`, false},
		{200, `{"code":"OK","request_id":"r","outcome":"completed","index":"8","exists":true,"value_b64":"AB=="}`, false},
		{504, `{"code":"TIMEOUT","request_id":"r","outcome":"unknown","exists":false}`, false},
		{409, `{"code":"RESOURCE_LIMIT","request_id":"r","outcome":"completed","index":"8"}`, false},
	} {
		_, e := c.decodeResponse("get", tc.status, []byte(tc.raw))
		if (e == nil) != tc.valid {
			t.Fatal(tc, e)
		}
	}
	command, _, code := Decode("get", []byte(`{"key":"x","timeout_ms":50}`))
	if code != "" || string(command) != `{"key":"x"}` {
		t.Fatal(string(command), code)
	}
	if _, _, e := c.Write(context.Background(), "n1", "get", []byte(`{"key":"x"}`)); e == nil {
		t.Fatal("mutation API accepted GET")
	}
}

func TestM10OpenAPIReadFixturesAndM08Compatibility(t *testing.T) {
	var spec, old map[string]any
	readJSON(t, "../../docs/openapi/m10.json", &spec)
	readJSON(t, "../../docs/openapi/m08.json", &old)
	paths := spec["paths"].(map[string]any)
	if len(paths) != 4 {
		t.Fatal("scope")
	}
	for p, v := range old["paths"].(map[string]any) {
		if !reflect.DeepEqual(v, paths[p]) {
			t.Fatal("M08 schema changed", p)
		}
	}
	op := paths["/v1/kv/get"].(map[string]any)["post"].(map[string]any)
	reqSchema := op["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	var fixtures []struct {
		Request  map[string]any
		Status   int
		Response map[string]any
	}
	readJSON(t, "../../docs/fixtures/http-m10.json", &fixtures)
	client, _ := NewClient(testConfig(t))
	defer client.Close()
	for _, f := range fixtures {
		if e := schemaCheck(spec, reqSchema, f.Request); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(f.Request)
		if _, _, code := Decode("get", raw); code != "" {
			t.Fatal(code)
		}
		resSchema := op["responses"].(map[string]any)[strconv.Itoa(f.Status)].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if e := schemaCheck(spec, resSchema, f.Response); e != nil {
			t.Fatal(f, e)
		}
		encoded, _ := json.Marshal(f.Response)
		if _, e := client.decodeResponse("get", f.Status, encoded); e != nil {
			t.Fatal(e)
		}
		b := healthyRead()
		if f.Response["code"] == "NOT_LEADER" {
			b.s.Leader = "n2"
		}
		b.read = func(context.Context, json.RawMessage) (raft.WriteOutcome, error) {
			code := f.Response["code"].(string)
			r := raft.WriteOutcome{Outcome: f.Response["outcome"].(string)}
			if r.Outcome == raft.Completed {
				r.Applied = raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: statemachine.Code(code)}}
				r.Applied.Result.Exists, _ = f.Response["exists"].(bool)
				if v, ok := f.Response["value_b64"]; ok {
					r.Applied.Result.Value, _ = strictjson.Base64(v, 16384)
				}
			}
			errs := map[string]error{"TIMEOUT": context.DeadlineExceeded, "NO_LEADER": raft.ErrNotLeader, "NOT_LEADER": raft.ErrNotLeader, "UNAVAILABLE": raft.ErrClosed, "OVERLOADED": raft.ErrFull, "STORAGE_UNAVAILABLE": wal.ErrFailed}
			return r, errs[code]
		}
		server, _ := New(client.cfg, b)
		w, _ := request(t, server, "POST", "/v1/kv/get", string(raw))
		_ = server.Shutdown(context.Background())
		var actual map[string]any
		d := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
		d.UseNumber()
		_ = d.Decode(&actual)
		if w.Code != f.Status {
			t.Fatal(w.Code, f.Status)
		}
		if e := schemaCheck(spec, resSchema, actual); e != nil {
			t.Fatal(e)
		}
		actual["request_id"] = f.Response["request_id"]
		if !reflect.DeepEqual(actual, f.Response) {
			t.Fatal(actual, f.Response)
		}
		actual["value_b64"] = nil
		if schemaCheck(spec, resSchema, actual) == nil {
			t.Fatal("schema accepted null/forbidden value")
		}
	}
}
