package node

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
)

func sessionConfigs(t *testing.T) ([3]config.Config, map[string]net.Listener) {
	t.Helper()
	legacy, ls := apiConfigs(t)
	var out [3]config.Config
	for i, c := range legacy {
		m, _, _ := fixture(t)
		h, min, max := c.Timing()
		m["config_version"] = 2
		m["application_version"] = 2
		m["cluster_id"] = c.ClusterID()
		m["node_id"] = c.NodeID()
		m["client_addr"] = c.Local().Client
		m["peer_addr"] = c.Local().Peer
		m["data_dir"] = c.DataDir()
		m["members"] = c.Members()
		m["heartbeat_ms"] = h
		m["election_min_ms"] = min
		m["election_max_ms"] = max
		out[i] = parse(t, m)
	}
	return out, ls
}

type lostCASListener struct {
	net.Listener
	armed atomic.Bool
	lost  chan struct{}
}
type lostCASConn struct {
	net.Conn
	owner *lostCASListener
}

func (l *lostCASListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e != nil {
		return nil, e
	}
	return &lostCASConn{c, l}, nil
}
func (c *lostCASConn) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(`"applied":true`)) && bytes.Contains(p, []byte(`"code":"OK"`)) && c.owner.armed.CompareAndSwap(true, false) {
		_ = c.Conn.Close()
		close(c.owner.lost)
		return 0, io.ErrClosedPipe
	}
	return c.Conn.Write(p)
}
func TestSessionCASLostResponseFailoverConcurrentRetriesAndMinority(t *testing.T) {
	cs, ls := sessionConfigs(t)
	var ns [3]*Node
	var losses [3]*lostCASListener
	for i, c := range cs {
		losses[i] = &lostCASListener{Listener: ls[c.Local().Client], lost: make(chan struct{})}
		ls[c.Local().Client] = losses[i]
	}
	for i := range ns {
		n, e := startNode(cs[i], true, nil, func(_, addr string) (net.Listener, error) { return ls[addr], nil }, true, 16, true, true)
		if e != nil {
			t.Fatal(e)
		}
		ns[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	}
	leader := func(excluded int) int {
		t.Helper()
		c := ctx(t)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
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
			case <-tick.C:
			case <-c.Done():
				t.Fatal("no session leader")
			}
		}
	}
	client, _ := clientapi.NewClient(cs[0])
	defer client.Close()
	current := leader(-1)
	body := func(id string, seq int, kind, value string, expected any) []byte {
		m := map[string]any{"client_id": strings.Repeat(id, 32), "sequence": seq, "key": "x"}
		if kind == "cas" {
			m["expected"] = expected
			m["new_value_b64"] = value
		} else if kind == "put" {
			m["value_b64"] = value
		}
		b, _ := json.Marshal(m)
		return b
	}
	send := func(kind string, raw []byte, want int, code string) clientapi.Response {
		t.Helper()
		r, status, e := client.Write(ctx(t), cs[current].NodeID(), kind, raw)
		if e != nil || status != want || r.Code != code || r.Outcome != raft.Completed {
			t.Fatal(status, r, e)
		}
		return r
	}
	absent := map[string]any{"exists": false}
	present := func(v string) any { return map[string]any{"exists": true, "value_b64": v} }
	original := body("a", 1, "cas", "Qg==", absent)
	losses[current].armed.Store(true)
	r, _, e := client.CAS(ctx(t), cs[current].NodeID(), original)
	if e == nil || r.Outcome != raft.Unknown {
		t.Fatal("lost response not uncertain", r, e)
	}
	select {
	case <-losses[current].lost:
	case <-ctx(t).Done():
		t.Fatal("did not lose post-apply reply")
	}
	var origin string
	for _, entry := range ns[current].Recovered().Entries {
		var o map[string]any
		_ = json.Unmarshal(entry.Raw, &o)
		if o["kind"] == "cas" {
			origin = strconv.FormatUint(entry.Index, 10)
			break
		}
	}
	if origin == "" {
		t.Fatal("no original CAS")
	}
	stopped := current
	if e = ns[stopped].Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
	current = leader(stopped)
	retry := send("cas", original, 200, "OK")
	if retry.Applied == nil || !*retry.Applied || retry.Index != origin {
		t.Fatal("new leader re-executed CAS", retry)
	}
	type answer struct {
		r      clientapi.Response
		status int
		e      error
	}
	done := make(chan answer, 2)
	for _, id := range []string{"b", "c"} {
		go func(id string) {
			raw := body(id, 1, "cas", map[string]string{"b": "Qw==", "c": "RA=="}[id], present("Qg=="))
			r, status, e := client.CAS(ctx(t), cs[current].NodeID(), raw)
			done <- answer{r, status, e}
		}(id)
	}
	success, failed := 0, 0
	for i := 0; i < 2; i++ {
		a := <-done
		if a.e != nil {
			t.Fatal(a.e)
		}
		if a.status == 200 && a.r.Code == "OK" {
			success++
		} else if a.status == 409 && a.r.Code == "COMPARE_FAILED" {
			failed++
		} else {
			t.Fatal(a)
		}
	}
	if success != 1 || failed != 1 {
		t.Fatal("non-atomic CAS", success, failed)
	}
	failedBody := body("d", 1, "cas", "Wg==", present("WA=="))
	failure := send("cas", failedBody, 409, "COMPARE_FAILED")
	send("put", body("e", 1, "put", "WA==", nil), 200, "OK")
	again := send("cas", failedBody, 409, "COMPARE_FAILED")
	if again.Index != failure.Index {
		t.Fatal("failed CAS reevaluated", again)
	}
	duplicate := body("f", 1, "cas", "WQ==", present("WA=="))
	for i := 0; i < 2; i++ {
		go func() {
			r, status, e := client.CAS(ctx(t), cs[current].NodeID(), duplicate)
			done <- answer{r, status, e}
		}()
	}
	var duplicateIndex string
	for i := 0; i < 2; i++ {
		a := <-done
		if a.e != nil || a.status != 200 || a.r.Applied == nil || !*a.r.Applied {
			t.Fatal(a)
		}
		if i == 0 {
			duplicateIndex = a.r.Index
		} else if duplicateIndex != a.r.Index {
			t.Fatal("duplicate got another result", a)
		}
	}
	send("cas", body("f", 1, "cas", "Wg==", present("WA==")), 409, "REQUEST_CONFLICT")
	send("delete", body("a", 2, "delete", "", nil), 200, "OK")
	send("cas", original, 409, "STALE_SEQUENCE")
	send("put", body("1", 2, "put", "", nil), 409, "SEQUENCE_GAP")
	// No quorum: even a cached retry requires a NEW committed attempt.
	remaining := 3 - stopped - current
	if e = ns[remaining].Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
	var object map[string]any
	_ = json.Unmarshal(duplicate, &object)
	object["timeout_ms"] = 50
	timed, _ := json.Marshal(object)
	r, status, e := client.CAS(ctx(t), cs[current].NodeID(), timed)
	if e != nil || status != 504 || r.Outcome != raft.Unknown || r.Index != "" {
		t.Fatal("cached result bypassed quorum", status, r, e)
	}
}
