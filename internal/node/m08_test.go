package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func apiConfigs(t *testing.T) ([3]config.Config, map[string]net.Listener) {
	t.Helper()
	m, _, _ := fixture(t)
	members := m["members"].([]any)
	listeners := map[string]net.Listener{}
	for _, v := range members {
		member := v.(map[string]any)
		for _, key := range []string{"peer", "client"} {
			l := listen(t)
			member[key] = l.Addr().String()
			listeners[l.Addr().String()] = l
		}
	}
	var cs [3]config.Config
	for i, v := range members {
		member := v.(map[string]any)
		m["node_id"] = member["id"]
		m["peer_addr"] = member["peer"]
		m["client_addr"] = member["client"]
		m["data_dir"] = private(t)
		cs[i] = parse(t, m)
	}
	return cs, listeners
}
func TestAPIThreeNodesWritesFailoverAndIndependentDuplicate(t *testing.T) {
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
	client, e := clientapi.NewClient(cs[0])
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	leader := func(exclude int) int {
		t.Helper()
		timer := time.NewTicker(5 * time.Millisecond)
		defer timer.Stop()
		ctx := ctx(t)
		for {
			for i, n := range ns {
				if i == exclude {
					continue
				}
				s, _ := n.ElectionStatus()
				if s.Failure != nil {
					t.Fatal(s.Failure)
				}
				if s.Role == raft.Leader && !s.Closed {
					return i
				}
			}
			select {
			case <-timer.C:
			case <-ctx.Done():
				t.Fatal("no API leader")
			}
		}
	}
	raw := func(kind string, seq int) []byte {
		m := map[string]any{"key": "x", "client_id": strings.Repeat("a", 32), "sequence": seq}
		if kind == "put" {
			m["value_b64"] = "eA=="
		}
		b, _ := json.Marshal(m)
		return b
	}
	write := func(i int, kind string, seq int) clientapi.Response {
		t.Helper()
		o, status, e := client.Write(ctx(t), cs[i].NodeID(), kind, raw(kind, seq))
		if e != nil || status != 200 || o.Outcome != raft.Completed {
			t.Fatal(status, o, e)
		}
		return o
	}
	first := leader(-1)
	follower := (first + 1) % 3
	o, status, e := client.Write(ctx(t), cs[follower].NodeID(), "put", raw("put", 1))
	if e != nil || status != 503 || o.Outcome != raft.NotAdmitted {
		t.Fatal(status, o, e)
	}
	put := write(first, "put", 1)
	for _, n := range ns {
		r, e := n.WaitApplied(ctx(t), receipt(t, put, n))
		if e != nil || !r.Result.Applied {
			t.Fatal(r, e)
		}
	}
	// Independent overlapping writes: exact slots are distinct and ordered, with
	// no HTTP-arrival order promise. All actual results belong to their receipts.
	type res struct {
		o      clientapi.Response
		e      error
		status int
	}
	done := make(chan res, 2)
	for seq := 2; seq < 4; seq++ {
		go func(seq int) {
			o, s, e := client.Write(ctx(t), cs[first].NodeID(), "put", raw("put", seq))
			done <- res{o, e, s}
		}(seq)
	}
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		r := <-done
		if r.e != nil || r.status != 200 || r.o.Outcome != raft.Completed || seen[r.o.Index] {
			t.Fatal(r)
		}
		seen[r.o.Index] = true
	}
	if e = ns[first].Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
	next := leader(first)
	deleted := write(next, "delete", 4)
	if deleted.Deleted == nil || !*deleted.Deleted {
		t.Fatal("failover lost completed put", deleted)
	}
	duplicate := write(next, "delete", 4)
	if duplicate.Deleted == nil || *duplicate.Deleted || duplicate.Index == deleted.Index {
		t.Fatal("M08 accidentally promises deduplication", duplicate)
	}
	// An isolated former majority member cannot commit a new entry alone.
	remaining := 3 - first - next
	if e = ns[remaining].Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
	pending := map[string]any{"key": "isolated", "value_b64": "", "client_id": strings.Repeat("b", 32), "sequence": 1, "timeout_ms": 50}
	b, _ := json.Marshal(pending)
	o, status, e = client.Write(ctx(t), cs[next].NodeID(), "put", b)
	if e != nil || status != 504 || o.Outcome != raft.Unknown || o.Index != "" {
		t.Fatal("minority acknowledged", status, o, e)
	}
	s, _ := ns[next].ElectionStatus()
	if s.LastIndex <= s.CommitIndex {
		t.Fatal("test did not append speculative write", s)
	}
	for _, n := range ns {
		if e = n.Shutdown(ctx(t)); e != nil {
			t.Fatal(e)
		}
	}
	// All six configured listeners and storage locks are released.
	for _, c := range cs {
		for _, addr := range []string{c.Local().Peer, c.Local().Client} {
			l, e := net.Listen("tcp", addr)
			if e != nil {
				t.Fatal(e)
			}
			_ = l.Close()
		}
		w, e := wal.Open(c.DataDir(), wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
		if e != nil {
			t.Fatal(e)
		}
		_ = w.Close()
	}
}
func receipt(t *testing.T, o clientapi.Response, n *Node) raft.LocalAppend {
	t.Helper()
	var index uint64
	if _, e := fmt.Sscan(o.Index, &index); e != nil {
		t.Fatal(e)
	}
	s, _ := n.ElectionStatus()
	return raft.LocalAppend{Index: index, Term: s.Term}
}

// Client-bind failure must release the already-acquired peer listener and lock.
func TestAPIStartupClientBindFailureReleasesStorage(t *testing.T) {
	cs, ls := apiConfigs(t)
	wanted := errors.New("client bind injected")
	_, e := startNode(cs[0], true, nil, func(_, addr string) (net.Listener, error) {
		if addr == cs[0].Local().Client {
			return nil, wanted
		}
		return ls[addr], nil
	}, true, 16, true, true)
	if !errors.Is(e, wanted) {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", cs[0].Local().Peer)
	if e != nil {
		t.Fatal(e)
	}
	_ = l.Close()
	w, e := wal.Open(cs[0].DataDir(), wal.Identity{ClusterID: cs[0].ClusterID(), NodeID: cs[0].NodeID(), ConfigHash: cs[0].Hash()})
	if e != nil {
		t.Fatal(e)
	}
	_ = w.Close()

}
