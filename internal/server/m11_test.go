package server

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func processSessionConfigs(t *testing.T) ([3]config.Config, [3]string) {
	t.Helper()
	_, paths := fixtureConfigs(t)
	var cs [3]config.Config
	for i, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		m["config_version"] = 2
		m["application_version"] = 2
		b, _ = json.Marshal(m)
		cs[i], e = config.Parse(b)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return cs, paths
}
func TestSessionCASRetriesAfterFollowerAndAllProcessCrash(t *testing.T) {
	cs, paths := processSessionConfigs(t)
	var ps [3]*process
	for i := range ps {
		ps[i] = launch(t, paths[i], true)
	}
	current := leader(t, cs, ps)
	client, _ := clientapi.NewClient(cs[0])
	defer client.Close()
	body := func(id string, seq int, kind, value string, expected any) []byte {
		m := map[string]any{"client_id": strings.Repeat(id, 32), "sequence": seq, "key": "x"}
		if kind == "cas" {
			m["new_value_b64"] = value
			m["expected"] = expected
		} else if kind == "put" {
			m["value_b64"] = value
		}
		b, _ := json.Marshal(m)
		return b
	}
	call := func(kind string, raw []byte, status int, code string) clientapi.Response {
		t.Helper()
		r, s, e := client.Write(testContext(t), cs[current].NodeID(), kind, raw)
		if e != nil || s != status || r.Code != code || r.Outcome != raft.Completed {
			t.Fatal(s, r, e)
		}
		return r
	}
	get := func(value string) clientapi.Response {
		t.Helper()
		r, s, e := client.Get(testContext(t), cs[current].NodeID(), []byte(`{"key":"x"}`))
		if e != nil || s != 200 || r.ValueB64 == nil || *r.ValueB64 != value {
			t.Fatal("recovered value", r, e)
		}
		return r
	}
	first := body("a", 1, "cas", "Qg==", map[string]any{"exists": false})
	original := call("cas", first, 200, "OK")
	failed := body("b", 1, "cas", "Qw==", map[string]any{"exists": true, "value_b64": "QQ=="})
	negative := call("cas", failed, 409, "COMPARE_FAILED")
	call("put", body("c", 1, "put", "QQ==", nil), 200, "OK")
	follower := (current + 1) % 3
	ps[follower].stop(t, false)
	r := call("cas", first, 200, "OK")
	if r.Index != original.Index {
		t.Fatal("follower crash retry index", r)
	}
	get("QQ==")
	ps[follower] = launch(t, paths[follower], false)
	barrier := get("QQ==")
	index, _ := strconv.ParseUint(barrier.Index, 10, 64)
	for i := range ps {
		poll(t, func() bool {
			s, ok := status(cs[i])
			applied, _ := strconv.ParseUint(s.Applied, 10, 64)
			return ok && applied >= index
		})
	}

	oldLeader := current
	ps[oldLeader].stop(t, false)
	current = leader(t, cs, ps)
	if retried := call("cas", first, 200, "OK"); retried.Index != original.Index {
		t.Fatal("leader crash retry", retried)
	}
	if retried := call("cas", failed, 409, "COMPARE_FAILED"); retried.Index != negative.Index {
		t.Fatal("leader crash failed result", retried)
	}
	ps[oldLeader] = launch(t, paths[oldLeader], false)
	barrier = get("QQ==")
	index, _ = strconv.ParseUint(barrier.Index, 10, 64)
	for i := range ps {
		poll(t, func() bool {
			s, ok := status(cs[i])
			applied, _ := strconv.ParseUint(s.Applied, 10, 64)
			return ok && applied >= index
		})
	}
	for _, p := range ps {
		if e := p.cmd.Process.Kill(); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range ps {
		select {
		case <-p.done:
		case <-testContext(t).Done():
			t.Fatal("child not reaped")
		}
	}
	for i := range ps {
		ps[i] = launch(t, paths[i], false)
	}
	current = leader(t, cs, ps)
	r = call("cas", first, 200, "OK")
	if r.Index != original.Index || r.Applied == nil || !*r.Applied {
		t.Fatal("all-process restart lost original result", r)
	}
	r = call("cas", failed, 409, "COMPARE_FAILED")
	if r.Index != negative.Index || r.Applied == nil || *r.Applied {
		t.Fatal("failed result reexecuted after restart", r)
	}
	get("QQ==")
	second := body("a", 2, "cas", "RA==", map[string]any{"exists": true, "value_b64": "QQ=="})
	next := call("cas", second, 200, "OK")
	get("RA==")
	call("cas", first, 409, "STALE_SEQUENCE")
	r = call("cas", second, 200, "OK")
	if r.Index != next.Index {
		t.Fatal("latest sequence retention", r)
	}
	for _, p := range ps {
		p.stop(t, true)
	}
	// The persisted application profile is not silently opened by an old reader.
	for _, c := range cs {
		w, e := wal.Open(c.DataDir(), id(c))
		if e == nil {
			_ = w.Close()
			t.Fatal("legacy open accepted sessions")
		}
		w, e = wal.OpenSessions(c.DataDir(), id(c))
		if e != nil {
			t.Fatal(e)
		}
		_ = w.Close()
	}
}
