package committed

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

func sessionEntry(index uint64, client string, seq uint64, kind, key, value string, expected map[string]any) storageformat.LogEntry {
	command := map[string]any{"client_id": client, "sequence": seq, "key": key}
	if kind == "put" {
		command["value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	if kind == "cas" {
		command["expected"] = expected
		command["new_value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	raw, _ := json.Marshal(map[string]any{"index": fmt.Sprint(index), "term": "1", "kind": kind, "command": command})
	return storageformat.LogEntry{Index: index, Term: 1, Raw: raw}
}
func TestSessionOrderedResultsConflictsAndOriginalIndices(t *testing.T) {
	m := statemachine.New()
	s := NewSessions(m)
	a, e := applier.New(Wrap(s), applier.Config{Capacity: 2})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Shutdown(context.Background())
	A, B, C, D := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32)
	expected := func(v string) map[string]any {
		return map[string]any{"exists": true, "value_b64": base64.StdEncoding.EncodeToString([]byte(v))}
	}
	cases := []struct {
		client      string
		seq         uint64
		kind, value string
		expected    map[string]any
		code        statemachine.Code
		origin      uint64
	}{
		{A, 1, "put", "A", nil, statemachine.OK, 1}, {A, 1, "put", "A", nil, statemachine.OK, 1}, {A, 1, "put", "B", nil, RequestConflict, 3}, {A, 3, "delete", "", nil, SequenceGap, 4},
		{A, 2, "cas", "B", expected("A"), statemachine.OK, 5}, {A, 2, "cas", "B", expected("A"), statemachine.OK, 5}, {A, 1, "put", "A", nil, StaleSequence, 7},
		{B, 1, "cas", "C", expected("A"), statemachine.CompareFailed, 8}, {A, 3, "delete", "", nil, statemachine.OK, 9}, {B, 1, "cas", "C", expected("A"), statemachine.CompareFailed, 8},
		{C, 1, "cas", "", map[string]any{"exists": false}, statemachine.OK, 11}, {D, 1, "cas", "Z", expected(""), statemachine.OK, 12}, {D, 1, "cas", "Z", map[string]any{"exists": false}, RequestConflict, 13},
	}
	for i, tc := range cases {
		entry := sessionEntry(uint64(i+1), tc.client, tc.seq, tc.kind, "x", tc.value, tc.expected)
		command, e := ProjectWithSessions(entry)
		if e != nil {
			t.Fatal(e)
		}
		f, e := a.TrySubmit(applier.Entry{Index: entry.Index, Command: command})
		if e != nil {
			t.Fatal(e)
		}
		o, e := f.Wait(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		r, origin, e := DecodeSessionResult(o.Result, entry.Index)
		if e != nil || r.Code != tc.code || origin != tc.origin {
			t.Fatal(i, r, origin, e)
		}
	}
	if e = a.Shutdown(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(s.clients) != 4 || s.clients[A].sequence != 3 || s.clients[B].sequence != 1 {
		t.Fatal("session highwater")
	}
	r := m.Apply(statemachine.Command{Operation: statemachine.Get, Key: "x"})
	if string(r.Value) != "Z" {
		t.Fatal(r)
	}
}

type quotaMachine struct{ calls int }

func (m *quotaMachine) Apply(statemachine.Command) statemachine.Result {
	m.calls++
	return statemachine.Result{Code: statemachine.ResourceLimit}
}
func TestSessionQuotaRetentionAndNoEviction(t *testing.T) {
	q := &quotaMachine{}
	s := NewSessions(q)
	for i := 0; i < MaxSessions; i++ {
		entry := sessionEntry(uint64(i+1), fmt.Sprintf("%032x", i+1), 1, "put", "x", "", nil)
		cmd, e := ProjectWithSessions(entry)
		if e != nil {
			t.Fatal(e)
		}
		r, origin, e := DecodeSessionResult(s.Apply(cmd), entry.Index)
		if e != nil || r.Code != statemachine.ResourceLimit || origin != entry.Index {
			t.Fatal(r, origin, e)
		}
	}
	if len(s.clients) != 1024 || q.calls != 1024 {
		t.Fatal("quota")
	}
	for _, tc := range []struct {
		client string
		seq    uint64
		code   statemachine.Code
	}{{fmt.Sprintf("%032x", 1), 1, statemachine.ResourceLimit}, {fmt.Sprintf("%032x", 1), 2, statemachine.ResourceLimit}, {fmt.Sprintf("%032x", 2048), 1, SessionLimit}, {fmt.Sprintf("%032x", 1), 1, StaleSequence}} {
		entry := sessionEntry(2000, tc.client, tc.seq, "put", "x", "", nil)
		cmd, _ := ProjectWithSessions(entry)
		r, _, e := DecodeSessionResult(s.Apply(cmd), entry.Index)
		if e != nil || r.Code != tc.code {
			t.Fatal(r, e)
		}
	}
	if len(s.clients) != 1024 || q.calls != 1025 {
		t.Fatal("retry reapplied or session evicted", q.calls)
	}
}
func TestSessionMaximumCASFitsUnchangedM02Bound(t *testing.T) {
	client := strings.Repeat("a", 32)
	value := strings.Repeat("x", 16384)
	entry := sessionEntry(^uint64(0), client, 9007199254740991, "cas", strings.Repeat("k", 256), value, map[string]any{"exists": true, "value_b64": base64.StdEncoding.EncodeToString([]byte(value))})
	command, e := ProjectWithSessions(entry)
	if e != nil {
		t.Fatal(e)
	}
	if len(command.Operation)+len(command.Value) > applier.MaxCommandBytes {
		t.Fatal("adapter exceeds M02")
	}
	digest, e := storageformat.CommandDigest(entry.Raw)
	if e != nil {
		t.Fatal(e)
	}
	entry2 := sessionEntry(1, strings.Repeat("b", 32), 1, "cas", strings.Repeat("k", 256), value, map[string]any{"exists": true, "value_b64": base64.StdEncoding.EncodeToString([]byte(value))})
	d, e := storageformat.CommandDigest(entry2.Raw)
	if e != nil || d != digest {
		t.Fatal("digest included identity/index")
	}
	if bytes.Contains(command.Value, []byte("timeout_ms")) {
		t.Fatal("transport leaked")
	}
}
