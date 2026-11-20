package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

// This finite offline validator covers EVERY assertion keyword used by this
// artifact. Unknown keywords fail the check rather than being silently ignored.
// It is deliberately not a general OpenAPI/JSON Schema certification tool.
func schemaCheck(root, s map[string]any, v any) error {
	for k := range s {
		switch k {
		case "$ref", "type", "properties", "required", "additionalProperties", "oneOf", "const", "enum", "pattern", "minLength", "maxLength", "minimum", "maximum", "default", "contentEncoding", "x-canonical-uint64", "x-canonical-base64-max-decoded-bytes", "x-configured-leader-url":
		default:
			return fmt.Errorf("unsupported schema assertion %s", k)
		}
	}
	if ref, ok := s["$ref"].(string); ok {
		const prefix = "#/components/schemas/"
		if len(ref) <= len(prefix) || ref[:len(prefix)] != prefix {
			return fmt.Errorf("ref")
		}
		return schemaCheck(root, root["components"].(map[string]any)["schemas"].(map[string]any)[ref[len(prefix):]].(map[string]any), v)
	}
	if variants, ok := s["oneOf"].([]any); ok {
		matched := 0
		for _, variant := range variants {
			if schemaCheck(root, variant.(map[string]any), v) == nil {
				matched++
			}
		}
		if matched != 1 {
			return fmt.Errorf("oneOf matches %d", matched)
		}
		return nil
	}
	if c, ok := s["const"]; ok && !reflect.DeepEqual(c, v) {
		return fmt.Errorf("const")
	}
	if list, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range list {
			found = found || reflect.DeepEqual(e, v)
		}
		if !found {
			return fmt.Errorf("enum")
		}
	}
	switch s["type"] {
	case "object":
		o, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("object")
		}
		props := s["properties"].(map[string]any)
		for _, k := range s["required"].([]any) {
			if _, ok := o[k.(string)]; !ok {
				return fmt.Errorf("required %s", k)
			}
		}
		for k, value := range o {
			p, ok := props[k]
			if !ok {
				return fmt.Errorf("additional field %s", k)
			}
			if err := schemaCheck(root, p.(map[string]any), value); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("string")
		}
		if p, ok := s["pattern"].(string); ok {
			if !regexp.MustCompile(p).MatchString(str) {
				return fmt.Errorf("pattern")
			}
		}
		for k, n := range map[string]bool{"minLength": true, "maxLength": false} {
			if b, ok := s[k].(json.Number); ok {
				bound, _ := strconv.Atoi(string(b))
				if (n && len(str) < bound) || (!n && len(str) > bound) {
					return fmt.Errorf("length")
				}
			}
		}
		if _, ok := s["x-canonical-uint64"]; ok {
			if _, e := strictjson.Uint(str); e != nil {
				return e
			}
		}
		if bound, ok := s["x-canonical-base64-max-decoded-bytes"].(json.Number); ok {
			n, _ := strconv.Atoi(string(bound))
			if _, e := strictjson.Base64(str, n); e != nil {
				return e
			}
		}
	case "integer":
		n, e := strictjson.Integer(v)
		if e != nil {
			return e
		}
		if b, ok := s["minimum"].(json.Number); ok {
			min, _ := strconv.ParseUint(string(b), 10, 64)
			if n < min {
				return fmt.Errorf("minimum")
			}
		}
		if b, ok := s["maximum"].(json.Number); ok {
			max, _ := strconv.ParseUint(string(b), 10, 64)
			if n > max {
				return fmt.Errorf("maximum")
			}
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("boolean")
		}
	case nil:
	default:
		return fmt.Errorf("type")
	}
	return nil
}
func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e = d.Decode(v); e != nil {
		t.Fatal(e)
	}
}
func TestOpenAPIAndHTTPFixtureConformance(t *testing.T) {
	var spec map[string]any
	readJSON(t, "../../docs/openapi/m08.json", &spec)
	if spec["openapi"] != "3.1.0" {
		t.Fatal("version")
	}
	paths := spec["paths"].(map[string]any)
	if len(paths) != 3 {
		t.Fatal("future API leaked")
	}
	var fixtures []struct {
		Kind     string
		Request  map[string]any
		Status   int
		Response map[string]any
	}
	readJSON(t, "../../docs/fixtures/http-m08.json", &fixtures)
	c, _ := NewClient(testConfig(t))
	defer c.Close()
	for _, f := range fixtures {
		operation := paths["/v1/kv/"+f.Kind].(map[string]any)["post"].(map[string]any)
		reqSchema := operation["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if e := schemaCheck(spec, reqSchema, f.Request); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(f.Request)
		if _, _, code := Decode(f.Kind, raw); code != "" {
			t.Fatal(code)
		}
		resSchema := operation["responses"].(map[string]any)[strconv.Itoa(f.Status)].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		if e := schemaCheck(spec, resSchema, f.Response); e != nil {
			t.Fatal(f, e)
		}
		encoded, _ := json.Marshal(f.Response)
		if _, e := c.decodeResponse(f.Kind, f.Status, encoded); e != nil {
			t.Fatal(f, e)
		}
		b := healthy()
		if f.Response["code"] == "NOT_LEADER" {
			b.s.Leader = "n2"
		}
		b.fn = func(context.Context, string, json.RawMessage) (raft.WriteOutcome, error) {
			o := raft.WriteOutcome{Outcome: f.Response["outcome"].(string)}
			code := f.Response["code"].(string)
			errs := map[string]error{"TIMEOUT": context.DeadlineExceeded, "NO_LEADER": raft.ErrNotLeader, "NOT_LEADER": raft.ErrNotLeader, "UNAVAILABLE": raft.ErrClosed, "OVERLOADED": raft.ErrFull, "STORAGE_UNAVAILABLE": wal.ErrFailed}
			if o.Outcome == raft.Completed {
				o.Applied = raft.AppliedResult{Index: 8, Term: 6, Result: statemachine.Result{Code: statemachine.Code(code), Applied: true, Deleted: true}}
			}
			return o, errs[code]
		}
		server, e := New(c.cfg, b)
		if e != nil {
			t.Fatal(e)
		}
		w, out := request(t, server, "POST", "/v1/kv/"+f.Kind, string(raw))
		_ = server.Shutdown(context.Background())
		if w.Code != f.Status {
			t.Fatal(w.Code, f)
		}
		actual := map[string]any{}
		d := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
		d.UseNumber()
		_ = d.Decode(&actual)
		if e := schemaCheck(spec, resSchema, actual); e != nil {
			t.Fatal(out, e)
		}
		actual["request_id"] = f.Response["request_id"]
		if !reflect.DeepEqual(actual, f.Response) {
			t.Fatal(actual, f.Response)
		}
		// Negative controls: neither an extra field nor missing outcome is valid.
		f.Response["unexpected"] = true
		if schemaCheck(spec, resSchema, f.Response) == nil {
			t.Fatal("validator ignored additional field")
		}
		delete(f.Response, "unexpected")
		delete(f.Response, "outcome")
		if schemaCheck(spec, resSchema, f.Response) == nil {
			t.Fatal("validator ignored missing outcome")
		}
	}
	for _, kind := range []string{"put", "delete"} {
		schema := spec["components"].(map[string]any)["schemas"].(map[string]any)[kind+"Request"].(map[string]any)
		var o map[string]any
		_ = json.Unmarshal([]byte(input(kind)), &o)
		o["sequence"] = json.Number("9007199254740992")
		if schemaCheck(spec, schema, o) == nil {
			t.Fatal("range ignored")
		}
	}
}
