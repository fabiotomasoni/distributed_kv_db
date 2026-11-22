package server

import (
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
)

func TestLoggedGETAfterThreeProcessCrashAndReplay(t *testing.T) {
	cs, paths := fixtureConfigs(t)
	var ps [3]*process
	for i := range ps {
		ps[i] = launch(t, paths[i], true)
	}
	current := leader(t, cs, ps)
	client, _ := clientapi.NewClient(cs[0])
	defer client.Close()
	completed(t, client, cs[current], "put", "survivor", "M10-small", 1)
	get := func(key string, wantStatus int, want string) {
		t.Helper()
		r, s, e := client.Get(testContext(t), cs[current].NodeID(), []byte(`{"key":"`+key+`"}`))
		if e != nil || s != wantStatus || r.Outcome != raft.Completed {
			t.Fatal(s, r, e)
		}
		if wantStatus == 200 && (r.ValueB64 == nil || *r.ValueB64 != want) {
			t.Fatal(r)
		}
		if wantStatus == 404 && (r.Exists == nil || *r.Exists) {
			t.Fatal(r)
		}
	}
	get("survivor", 200, "TTEwLXNtYWxs")
	get("missing", 404, "")
	// Abruptly stop every server, preserving directories containing read entries.
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
	get("survivor", 200, "TTEwLXNtYWxs")
	get("missing", 404, "")
	completed(t, client, cs[current], "delete", "survivor", "", 2)
	get("survivor", 404, "")
	for _, p := range ps {
		p.stop(t, true)
	}
}
