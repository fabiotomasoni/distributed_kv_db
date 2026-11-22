package node

import (
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
)

type readHistoryOp struct {
	kind, value, result string
	exists, deleted     bool
	start, end          uint64
}

// Independent one-key exhaustive check, bounded to this tiny complete history.
// It respects response-before-invocation edges and branches only on overlaps.
func legalReadHistory(ops []readHistoryOp, initial string, present bool) bool {
	var search func(uint64, string, bool) bool
	search = func(done uint64, value string, exists bool) bool {
		if done == (uint64(1)<<len(ops))-1 {
			return true
		}
		for i, o := range ops {
			bit := uint64(1) << i
			if done&bit != 0 {
				continue
			}
			ready := true
			for j, p := range ops {
				if p.end < o.start && done&(uint64(1)<<j) == 0 {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			nextValue, nextExists := value, exists
			switch o.kind {
			case "put":
				nextValue, nextExists = o.value, true
			case "delete":
				if o.deleted != exists {
					continue
				}
				nextValue, nextExists = "", false
			case "get":
				if o.exists != exists || (exists && o.result != value) {
					continue
				}
			}
			if search(done|bit, nextValue, nextExists) {
				return true
			}
		}
		return false
	}
	return search(0, initial, present)
}
func TestLoggedGETHTTPConcurrentHistoryFailoverAndMinority(t *testing.T) {
	cs, listeners := apiConfigs(t)
	var ns [3]*Node
	for i := range ns {
		n, e := startNode(cs[i], true, nil, func(_, addr string) (net.Listener, error) { return listeners[addr], nil }, true, 16, true, true)
		if e != nil {
			t.Fatal(e)
		}
		ns[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	}
	client, _ := clientapi.NewClient(cs[0])
	defer client.Close()
	leader := func(excluded int) int {
		t.Helper()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		c := ctx(t)
		for {
			for i, n := range ns {
				if i == excluded {
					continue
				}
				s, _ := n.ElectionStatus()
				if s.Role == raft.Leader && !s.Closed {
					return i
				}
			}
			select {
			case <-ticker.C:
			case <-c.Done():
				t.Fatal("no GET leader")
			}
		}
	}
	current := leader(-1)
	command := func(kind, key, value string) []byte {
		m := map[string]any{"key": key}
		if kind != "get" {
			m["client_id"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			m["sequence"] = 1
		}
		if kind == "put" {
			m["value_b64"] = value
		}
		b, _ := json.Marshal(m)
		return b
	}
	put := func(key, value string) {
		t.Helper()
		r, status, e := client.Write(ctx(t), cs[current].NodeID(), "put", command("put", key, value))
		if e != nil || status != 200 || r.Outcome != raft.Completed {
			t.Fatal(status, r, e)
		}
	}
	put("x", "YmFzZQ==")
	put("persist", "dg==")
	put("empty", "")
	follower := (current + 1) % 3
	r, status, e := client.Get(ctx(t), cs[follower].NodeID(), command("get", "x", ""))
	if e != nil || status != 503 || r.Outcome != raft.NotAdmitted {
		t.Fatal("follower served GET", status, r, e)
	}
	r, status, e = client.Get(ctx(t), cs[current].NodeID(), command("get", "empty", ""))
	if e != nil || status != 200 || r.ValueB64 == nil || *r.ValueB64 != "" || r.Exists == nil || !*r.Exists {
		t.Fatal("empty became absent", r, e)
	}
	var clock atomic.Uint64
	invoke := func(kind, value string) (readHistoryOp, error) {
		o := readHistoryOp{kind: kind, value: value, start: clock.Add(1)}
		var r clientapi.Response
		var status int
		var e error
		if kind == "get" {
			r, status, e = client.Get(ctx(t), cs[current].NodeID(), command("get", "x", ""))
		} else {
			r, status, e = client.Write(ctx(t), cs[current].NodeID(), kind, command(kind, "x", value))
		}
		o.end = clock.Add(1)
		if e != nil {
			return o, e
		}
		if r.Outcome != raft.Completed || (status != 200 && !(kind == "get" && status == 404)) {
			return o, raft.ErrInput
		}
		if r.Exists != nil {
			o.exists = *r.Exists
		}
		if r.ValueB64 != nil {
			o.result = *r.ValueB64
		}
		if r.Deleted != nil {
			o.deleted = *r.Deleted
		}
		return o, nil
	}
	type result struct {
		o readHistoryOp
		e error
	}
	done := make(chan result, 3)
	for _, kind := range []string{"get", "put", "get"} {
		go func(k string) { o, e := invoke(k, "bmV3"); done <- result{o, e} }(kind)
	}
	var history []readHistoryOp
	for i := 0; i < 3; i++ {
		r := <-done
		if r.e != nil {
			t.Fatal(r.e)
		}
		history = append(history, r.o)
	}
	for _, kind := range []string{"delete", "get"} {
		o, e := invoke(kind, "")
		if e != nil {
			t.Fatal(e)
		}
		history = append(history, o)
	}
	if !legalReadHistory(history, "YmFzZQ==", true) {
		t.Fatal("nonlinearizable small history", history)
	}
	if legalReadHistory([]readHistoryOp{{kind: "put", value: "new", start: 1, end: 2}, {kind: "get", exists: true, result: "old", start: 3, end: 4}}, "old", true) {
		t.Fatal("history checker accepted stale counterexample")
	}
	old := current
	if e = ns[old].Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
	current = leader(old)
	r, status, e = client.Get(ctx(t), cs[current].NodeID(), command("get", "persist", ""))
	if e != nil || status != 200 || r.ValueB64 == nil || *r.ValueB64 != "dg==" {
		t.Fatal("GET lost completed write after election", r, e)
	}
	// The only remaining peer stops: a new GET cannot use a cached value/quorum.
	other := 3 - old - current
	if e = ns[other].Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
	r, status, e = client.Get(ctx(t), cs[current].NodeID(), []byte(`{"key":"persist","timeout_ms":50}`))
	if e != nil || status != 504 || r.Outcome != raft.Unknown || r.ValueB64 != nil || r.Exists != nil {
		t.Fatal("minority GET completed", status, r, e)
	}
}
