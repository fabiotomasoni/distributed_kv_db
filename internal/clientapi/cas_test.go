package clientapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

type sessionBackendFake struct{ *readBackendFake }

func (*sessionBackendFake) SessionWrites() bool { return true }
func sessionConfig(t *testing.T) config.Config {
	t.Helper()
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v2-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration json.RawMessage }
	_ = json.Unmarshal(raw, &f)
	c, e := config.Parse(f.Configuration)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func casInput() string {
	return `{"key":"x","expected":{"exists":false},"new_value_b64":"","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1}`
}
func TestCASHTTPContractCanonicalBoundsAndSessionErrors(t *testing.T) {
	b := &sessionBackendFake{healthyRead()}
	b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: statemachine.OK, Applied: true}}, ResultIndex: 2}, nil
	}
	s, e := New(sessionConfig(t), b)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown(context.Background())
	w, o := sessionRequest(t, s, "POST", "/v1/kv/cas", casInput())
	if w.Code != 200 || o.Index != "2" || o.Applied == nil || !*o.Applied {
		t.Fatal(w.Code, o)
	}
	for _, code := range []statemachine.Code{statemachine.CompareFailed, statemachine.ResourceLimit, committed.SessionLimit, committed.RequestConflict, committed.StaleSequence, committed.SequenceGap} {
		b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
			return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: code}}, ResultIndex: 3}, nil
		}
		w, o = sessionRequest(t, s, "POST", "/v1/kv/cas", casInput())
		if w.Code != 409 || o.Code != string(code) || o.Outcome != raft.Completed || o.Index != "3" {
			t.Fatal(w.Code, o)
		}
		if code == statemachine.CompareFailed || code == statemachine.ResourceLimit {
			if o.Applied == nil || *o.Applied {
				t.Fatal(o)
			}
		} else if o.Applied != nil || o.Deleted != nil {
			t.Fatal("session error contained operation fields", o)
		}
	}
	for _, raw := range []string{
		strings.Replace(casInput(), `{"exists":false}`, `{"exists":false,"value_b64":""}`, 1), strings.Replace(casInput(), `{"exists":false}`, `{"exists":true}`, 1), strings.Replace(casInput(), `{"exists":false}`, `{"exists":null}`, 1), strings.Replace(casInput(), `"new_value_b64":""`, `"new_value_b64":null`, 1), strings.Replace(casInput(), `"new_value_b64":""`, `"new_value_b64":"AB=="`, 1), strings.Replace(casInput(), `"sequence":1`, `"sequence":0`, 1), strings.Replace(casInput(), `"key":"x"`, `"key":"x","key":"y"`, 1),
	} {
		calls := b.calls.Load()
		w, o = sessionRequest(t, s, "POST", "/v1/kv/cas", raw)
		if w.Code != 400 || o.Outcome != raft.NotAdmitted || b.calls.Load() != calls {
			t.Fatal(raw, w.Code, o)
		}
	}
	value := base64.StdEncoding.EncodeToString(make([]byte, 16384))
	max := map[string]any{"key": strings.Repeat("k", 256), "expected": map[string]any{"exists": true, "value_b64": value}, "new_value_b64": value, "client_id": strings.Repeat("a", 32), "sequence": 9007199254740991, "timeout_ms": 5000}
	raw, _ := json.Marshal(max)
	if len(raw) != 44116 {
		t.Fatal("maximum CAS size changed", len(raw))
	}
	if _, _, code := Decode("cas", raw); code != "" {
		t.Fatal(code)
	}
	for _, n := range []int{MaxBody, MaxBody + 1} {
		body := string(raw) + strings.Repeat(" ", n-len(raw))
		w, _ = sessionRequest(t, s, "POST", "/v1/kv/cas", body)
		if n == MaxBody && w.Code == 413 || n > MaxBody && w.Code != 413 {
			t.Fatal(n, w.Code)
		}
	}
}
func TestCASClientResultValidationAndLegacyProfile(t *testing.T) {
	c, _ := NewClient(sessionConfig(t))
	defer c.Close()
	for _, tc := range []struct {
		code, extra string
		valid       bool
	}{{"OK", `,"applied":true`, true}, {"COMPARE_FAILED", `,"applied":false`, true}, {"RESOURCE_LIMIT", `,"applied":false`, true}, {"REQUEST_CONFLICT", "", true}, {"STALE_SEQUENCE", "", true}, {"SEQUENCE_GAP", "", true}, {"SESSION_LIMIT", "", true}, {"COMPARE_FAILED", `,"applied":true`, false}, {"SESSION_LIMIT", `,"applied":false`, false}} {
		status := 409
		if tc.code == "OK" {
			status = 200
		}
		raw := `{"code":"` + tc.code + `","request_id":"r","outcome":"completed","index":"2"` + tc.extra + `}`
		_, e := c.decodeResponse("cas", status, []byte(raw))
		if (e == nil) != tc.valid {
			t.Fatal(raw, e)
		}
	}
	legacy, _ := NewClient(testConfig(t))
	defer legacy.Close()
	r, _, e := legacy.CAS(context.Background(), "n1", []byte(casInput()))
	if e == nil || r.Outcome != raft.NotAdmitted {
		t.Fatal("CAS sent to known legacy profile", r, e)
	}
}

func TestM11OpenAPIHTTPFixturesAndEarlierRequestCompatibility(t *testing.T) {
	var spec, old map[string]any
	readJSON(t, "../../docs/openapi/m11.json", &spec)
	readJSON(t, "../../docs/openapi/m10.json", &old)
	paths := spec["paths"].(map[string]any)
	if len(paths) != 5 {
		t.Fatal("M12 scope leak")
	}
	for _, path := range []string{"/v1/kv/get", "/v1/status"} {
		if !reflect.DeepEqual(paths[path], old["paths"].(map[string]any)[path]) {
			t.Fatal("read contract changed")
		}
	}
	for _, kind := range []string{"put", "delete"} {
		if !reflect.DeepEqual(spec["components"].(map[string]any)["schemas"].(map[string]any)[kind+"Request"], old["components"].(map[string]any)["schemas"].(map[string]any)[kind+"Request"]) {
			t.Fatal("earlier mutation input changed")
		}
	}
	var fixtures []struct {
		Kind     string
		Headers  map[string]string
		Request  map[string]any
		Status   int
		Response map[string]any
	}
	readJSON(t, "../../docs/fixtures/http-m11.json", &fixtures)
	c, _ := NewClient(sessionConfig(t))
	defer c.Close()
	for _, f := range fixtures {
		op := paths["/v1/kv/"+f.Kind].(map[string]any)["post"].(map[string]any)
		parameter := op["parameters"].([]any)[0].(map[string]any)
		if f.Headers[ApplicationHeader] != "2" || parameter["required"] != true {
			t.Fatal("profile header fixture")
		}
		if e := schemaCheck(spec, parameter["schema"].(map[string]any), f.Headers[ApplicationHeader]); e != nil {
			t.Fatal(e)
		}
		requestSchema := op["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if e := schemaCheck(spec, requestSchema, f.Request); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(f.Request)
		if _, _, code := Decode(f.Kind, raw); code != "" {
			t.Fatal(code)
		}
		schema := op["responses"].(map[string]any)[strconv.Itoa(f.Status)].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if e := schemaCheck(spec, schema, f.Response); e != nil {
			t.Fatal(f, e)
		}
		b := &sessionBackendFake{healthyRead()}
		b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
			code := statemachine.Code(f.Response["code"].(string))
			index, _ := strconv.ParseUint(f.Response["index"].(string), 10, 64)
			return raft.WriteOutcome{Outcome: raft.Completed, Applied: raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: code, Applied: code == statemachine.OK && f.Kind != "delete", Deleted: true}}, ResultIndex: index}, nil
		}
		server, _ := New(c.cfg, b)
		w, _ := sessionRequest(t, server, "POST", "/v1/kv/"+f.Kind, string(raw))
		_ = server.Shutdown(context.Background())
		if w.Header().Get(ApplicationHeader) != "2" {
			t.Fatal("missing response profile")
		}
		if w.Code != f.Status {
			t.Fatal(w.Code, f)
		}
		var actual map[string]any
		d := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
		d.UseNumber()
		_ = d.Decode(&actual)
		if e := schemaCheck(spec, schema, actual); e != nil {
			t.Fatal(actual, e)
		}
		actual["request_id"] = f.Response["request_id"]
		if !reflect.DeepEqual(actual, f.Response) {
			t.Fatal(actual, f.Response)
		}
		encoded, _ := json.Marshal(actual)
		if _, e := c.decodeResponse(f.Kind, w.Code, encoded); e != nil {
			t.Fatal(e)
		}
		actual["unexpected"] = true
		if schemaCheck(spec, schema, actual) == nil {
			t.Fatal("fixture validator ignored extra field")
		}
	}
}

func sessionRequest(t *testing.T, s *Server, method, path, raw string) (*httptest.ResponseRecorder, Response) {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+s.cfg.Local().Client+path, strings.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(ApplicationHeader, "2")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	var o Response
	if e := json.Unmarshal(w.Body.Bytes(), &o); e != nil {
		t.Fatal(e)
	}
	return w, o
}
func TestApplicationHeaderPreventsCrossProfileMutation(t *testing.T) {
	b := &sessionBackendFake{healthyRead()}
	s, e := New(sessionConfig(t), b)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown(context.Background())
	for _, version := range []string{"", "1", "3"} {
		r := httptest.NewRequest("POST", "http://"+s.cfg.Local().Client+"/v1/kv/put", strings.NewReader(input("put")))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set(ApplicationHeader, version)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 400 || b.calls.Load() != 0 {
			t.Fatal("legacy client reached M11 mutation", w.Code)
		}
	}
	legacy, _ := New(testConfig(t), healthy())
	defer legacy.Shutdown(context.Background())
	w, _ := sessionRequest(t, legacy, "POST", "/v1/kv/put", input("put"))
	if w.Code != 400 {
		t.Fatal("M11 request reached legacy mutation", w.Code)
	}
}

type sessionRoundTrip func(*http.Request) (*http.Response, error)

func (f sessionRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSessionClientRequiresVersionAcknowledgment(t *testing.T) {
	c, _ := NewClient(sessionConfig(t))
	defer c.Close()
	for _, versions := range [][]string{nil, {"1"}, {"2", "2"}, {"2"}} {
		c.http = &http.Client{Transport: sessionRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get(ApplicationHeader) != "2" {
				t.Fatal("client omitted profile")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}, http.CanonicalHeaderKey(ApplicationHeader): versions}, Body: io.NopCloser(strings.NewReader(`{"code":"OK","request_id":"r","outcome":"completed","index":"2","applied":true}`))}, nil
		})}
		r, _, e := c.CAS(context.Background(), "n1", []byte(casInput()))
		valid := len(versions) == 1 && versions[0] == "2"
		if valid && (e != nil || r.Outcome != raft.Completed) || !valid && (e == nil || r.Outcome != raft.Unknown) {
			t.Fatal(versions, r, e)
		}
	}
}
