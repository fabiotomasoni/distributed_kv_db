package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/fabiotomasoni/distributed_kv_db/internal/diagnostics"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type observedBackend struct {
	*sessionBackendFake
	d *diagnostics.Recorder
}

func (b *observedBackend) Diagnostics() *diagnostics.Recorder { return b.d }
func (b *observedBackend) Metrics() map[string]any {
	return map[string]any{"metrics_version": 1, "local_only": true, "node_id": "n1", "role": "leader", "term": 6, "commit_index": 8, "applied_index": 8, "observations": b.d.Snapshot()}
}
func TestT017HTTPMetricsAndCorrelatedTimeoutRedaction(t *testing.T) {
	b := &observedBackend{&sessionBackendFake{healthyRead()}, diagnostics.New("n1", strings.Repeat("1", 32))}
	b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
		return raft.WriteOutcome{Outcome: raft.Unknown}, context.DeadlineExceeded
	}
	s, e := New(sessionConfig(t), b)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Shutdown(context.Background())
	w, r := sessionRequest(t, s, "POST", "/v1/kv/cas", casInput())
	if w.Code != 504 || r.Outcome != raft.Unknown {
		t.Fatal(w.Code, r)
	}
	observations := b.d.Snapshot()
	if observations.Counters["client_unknown"] != 1 || observations.Durations["request"].Count != 1 {
		t.Fatal(observations)
	}
	logs := b.d.Drain()
	if len(logs) != 1 || logs[0].RequestID != r.RequestID || logs[0].Code != "TIMEOUT" || logs[0].Outcome != "unknown" || logs[0].Level != "WARN" {
		t.Fatal(logs)
	}
	raw, _ := json.Marshal(logs)
	if strings.Contains(string(raw), "new_value") || strings.Contains(string(raw), strings.Repeat("a", 32)) {
		t.Fatal("mutation payload leaked")
	}
	before := b.calls.Load()
	req := httptest.NewRequest("GET", "http://"+s.cfg.Local().Client+"/v1/metrics", nil)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"local_only":true`) || b.calls.Load() != before {
		t.Fatal(w.Code, w.Body.String())
	}
	req = httptest.NewRequest("POST", "http://"+s.cfg.Local().Client+"/v1/metrics", nil)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 405 || w.Header().Get("Allow") != "GET" {
		t.Fatal(w.Code)
	}
}
func TestM12OpenAPIPreservesM11Operations(t *testing.T) {
	var spec, old map[string]any
	readJSON(t, "../../docs/openapi/m12.json", &spec)
	readJSON(t, "../../docs/openapi/m11.json", &old)
	paths := spec["paths"].(map[string]any)
	schema := paths["/v1/metrics"].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	var fixture map[string]any
	readJSON(t, "../../docs/fixtures/metrics-m12.json", &fixture)
	if e := metricSchema(spec, schema, fixture); e != nil {
		t.Fatal(e)
	}
	backend := &observedBackend{&sessionBackendFake{healthyRead()}, diagnostics.New("n1", strings.Repeat("1", 32))}
	backend.d.Observe("request", 3)
	raw, _ := json.Marshal(backend.Metrics())
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var live any
	_ = decoder.Decode(&live)
	if e := metricSchema(spec, schema, live); e != nil {
		t.Fatal(e)
	}
	fixture["local_only"] = false
	if e := metricSchema(spec, schema, fixture); e == nil {
		t.Fatal("invalid const accepted")
	}
	fixture["local_only"] = true
	fixture["observations"].(map[string]any)["durations"].(map[string]any)["request"].(map[string]any)["samples_us"] = make([]any, 2049)
	if e := metricSchema(spec, schema, fixture); e == nil {
		t.Fatal("unbounded samples accepted")
	}

	if len(paths) != 6 {
		t.Fatal("unexpected API scope")
	}
	for path, value := range old["paths"].(map[string]any) {
		a, _ := json.Marshal(value)
		b, _ := json.Marshal(paths[path])
		if string(a) != string(b) {
			t.Fatal("M11 operation changed", path)
		}
	}
}

// The additive diagnostics schema needs bounded arrays and fixed-domain maps.
// Keep historical contract tests unchanged; extend only this M12 validator.
func metricSchema(root, s map[string]any, v any) error {
	switch s["type"] {
	case "object":
		o, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("object")
		}
		props, _ := s["properties"].(map[string]any)
		required, _ := s["required"].([]any)
		for _, key := range required {
			if _, ok = o[key.(string)]; !ok {
				return fmt.Errorf("required")
			}
		}
		for key, value := range o {
			if p, ok := props[key]; ok {
				if e := metricSchema(root, p.(map[string]any), value); e != nil {
					return e
				}
			} else {
				switch additional := s["additionalProperties"].(type) {
				case bool:
					if !additional {
						return fmt.Errorf("additional field")
					}
				case map[string]any:
					if e := metricSchema(root, additional, value); e != nil {
						return e
					}
				default:
					return fmt.Errorf("additional schema")
				}
			}
		}
	case "array":
		values, ok := v.([]any)
		if !ok {
			return fmt.Errorf("array")
		}
		bound, _ := strconv.Atoi(s["maxItems"].(json.Number).String())
		if len(values) > bound {
			return fmt.Errorf("maxItems")
		}
		for _, value := range values {
			if e := metricSchema(root, s["items"].(map[string]any), value); e != nil {
				return e
			}
		}
	default:
		return schemaCheck(root, s, v)
	}
	return nil
}
