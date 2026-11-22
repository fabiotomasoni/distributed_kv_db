package node

import (
	"encoding/json"
	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/diagnostics"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestT017LocalMetricsRemainAvailableWithoutQuorum(t *testing.T) {
	cs, ls := sessionConfigs(t)
	var ns [3]*Node
	for i := range ns {
		n, e := startNode(cs[i], true, nil, func(_, addr string) (net.Listener, error) { return ls[addr], nil }, true, 16, true, true)
		if e != nil {
			t.Fatal(e)
		}
		ns[i] = n
		t.Cleanup(func() { _ = n.Shutdown(ctx(t)) })
	}
	leader := -1
	timer := time.NewTicker(5 * time.Millisecond)
	defer timer.Stop()
	deadline := ctx(t)
wait:
	for {
		for i, n := range ns {
			s, _ := n.ElectionStatus()
			if s.Role == raft.Leader && s.LastApplied >= 1 {
				leader = i
				break wait
			}
		}
		select {
		case <-timer.C:
		case <-deadline.Done():
			t.Fatal("election timed out")
		}
	}
	c, e := clientapi.NewClient(cs[0])
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	command := []byte(`{"key":"synthetic-private-key","expected":{"exists":false},"new_value_b64":"cHJpdmF0ZS12YWx1ZQ==","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1,"timeout_ms":500}`)
	r, _, e := c.CAS(ctx(t), cs[leader].NodeID(), command)
	if e != nil || r.Outcome != raft.Completed {
		t.Fatal(r, e)
	}
	for i, n := range ns {
		if i != leader {
			if e = n.Shutdown(ctx(t)); e != nil {
				t.Fatal(e)
			}
		}
	}
	r, status, e := c.CAS(ctx(t), cs[leader].NodeID(), []byte(strings.Replace(string(command), `"timeout_ms":500`, `"timeout_ms":50`, 1)))
	if e != nil || status != 504 || r.Outcome != raft.Unknown {
		t.Fatal(r, status, e)
	}
	httpClient := &http.Client{Timeout: time.Second}
	defer httpClient.CloseIdleConnections()
	response, e := httpClient.Get("http://" + cs[leader].Local().Client + "/v1/metrics")
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	var metric struct {
		LocalOnly    bool                 `json:"local_only"`
		Observations diagnostics.Snapshot `json:"observations"`
	}
	if e = json.NewDecoder(response.Body).Decode(&metric); e != nil || response.StatusCode != 200 || !metric.LocalOnly || metric.Observations.Counters["client_completed"] < 1 || metric.Observations.Counters["client_unknown"] < 1 {
		t.Fatal(metric, e)
	}
	logs := ns[leader].Diagnostics().Drain()
	seenElection, seenTimeout := false, false
	for _, log := range logs {
		if log.Event == diagnostics.Transition && log.Role == "leader" {
			seenElection = true
		}
		if log.Code == "TIMEOUT" && log.Outcome == raft.Unknown && log.RequestID == r.RequestID {
			seenTimeout = true
		}
		raw := string(diagnostics.Encode(log))
		if strings.Contains(raw, "synthetic-private-key") || strings.Contains(raw, "cHJpdmF0ZS12YWx1ZQ==") || strings.Contains(raw, strings.Repeat("a", 32)) {
			t.Fatal("log disclosed payload")
		}
	}
	if !seenElection || !seenTimeout {
		t.Fatal("missing correlation", seenElection, seenTimeout)
	}
}
