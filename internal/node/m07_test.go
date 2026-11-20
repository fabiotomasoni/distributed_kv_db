package node

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
)

func TestCommittedClusterCASFailoverAndFreshFollowerReplay(t *testing.T) {
	cs, ls := configs(t)
	var nodes [3]*Node
	for i := range nodes {
		n, err := startRuntime(cs[i], true, nil, func(string, string) (net.Listener, error) { return ls[i], nil }, true, 2, true)
		if err != nil {
			t.Fatal(err)
		}
		nodes[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	}
	waitLeader := func(excluded int) int {
		t.Helper()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		deadline := ctx(t)
		for {
			for i, n := range nodes {
				if i == excluded {
					continue
				}
				s, _ := n.ElectionStatus()
				if s.Failure != nil {
					t.Fatal(s.Failure)
				}
				if s.LastApplied > s.CommitIndex || s.ScheduledApplyIndex > s.CommitIndex {
					t.Fatal(s)
				}
				if s.Role == raft.Leader {
					return i
				}
			}
			select {
			case <-ticker.C:
			case <-deadline.Done():
				t.Fatal("no committed-mode leader")
			}
		}
	}
	command := func(sequence int, kind, value, expected string) json.RawMessage {
		m := map[string]any{"key": "x", "client_id": strings.Repeat("a", 32), "sequence": sequence}
		if kind == "put" {
			m["value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
		} else {
			m["expected"] = map[string]any{"exists": true, "value_b64": base64.StdEncoding.EncodeToString([]byte(expected))}
			m["new_value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
		}
		b, _ := json.Marshal(m)
		return b
	}
	leader := waitLeader(-1)
	put, err := nodes[leader].AppendLocal(ctx(t), "put", command(1, "put", "base", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		r, err := n.WaitApplied(ctx(t), put)
		if err != nil || r.Result.Code != statemachine.OK {
			t.Fatal(r, err)
		}
	}
	type attempt struct {
		receipt raft.LocalAppend
		value   string
		err     error
	}
	done := make(chan attempt, 2)
	for i, v := range []string{"a", "b"} {
		go func(seq int, value string) {
			r, err := nodes[leader].AppendLocal(ctx(t), "cas", command(seq, "cas", value, "base"))
			done <- attempt{r, value, err}
		}(i+2, v)
	}
	winner := ""
	success, failed := 0, 0
	for i := 0; i < 2; i++ {
		a := <-done
		if a.err != nil {
			t.Fatal(a.err)
		}
		r, err := nodes[leader].WaitApplied(ctx(t), a.receipt)
		if err != nil {
			t.Fatal(err)
		}
		switch r.Result.Code {
		case statemachine.OK:
			success++
			winner = a.value
		case statemachine.CompareFailed:
			failed++
		default:
			t.Fatal(r)
		}
		for _, n := range nodes {
			other, err := n.WaitApplied(ctx(t), a.receipt)
			if err != nil || other.Result.Code != r.Result.Code {
				t.Fatal("replicas applied different result", other, err)
			}
		}
	}
	if success != 1 || failed != 1 {
		t.Fatal("CAS order violated", success, failed)
	}
	if err = nodes[leader].Shutdown(ctx(t)); err != nil {
		t.Fatal(err)
	}
	replacement := waitLeader(leader)
	receipt, err := nodes[replacement].AppendLocal(ctx(t), "cas", command(4, "cas", "after-failover", winner))
	if err != nil {
		t.Fatal(err)
	}
	r, err := nodes[replacement].WaitApplied(ctx(t), receipt)
	if err != nil || r.Result.Code != statemachine.OK {
		t.Fatal("committed state lost across election", r, err)
	}
	restarted, err := StartCommitted(cs[leader], false)
	if err != nil {
		t.Fatal(err)
	}
	nodes[leader] = restarted
	t.Cleanup(func() { _ = restarted.Shutdown(ctx(t)) })
	r, err = restarted.WaitApplied(ctx(t), receipt)
	if err != nil || r.Result.Code != statemachine.OK {
		t.Fatal("fresh replay or catch-up changed CAS", r, err)
	}
	for _, n := range nodes {
		if err = n.Shutdown(ctx(t)); err != nil {
			t.Fatal(err)
		}
		s, _ := n.ElectionStatus()
		if s.LastApplied != s.CommitIndex {
			t.Fatal("shutdown lost committed application", s)
		}
	}
}
