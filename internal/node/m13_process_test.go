package node

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
)

func TestSnapshotRecoveryChild(t *testing.T) {
	raw := os.Getenv("QUORUMKV_M13C_CONFIG")
	if raw == "" {
		return
	}
	c, e := config.Parse([]byte(raw))
	if e != nil {
		t.Fatal(e)
	}
	n, e := StartAPIWithSnapshotRecovery(c)
	if e != nil {
		t.Fatal(e)
	}
	emit := func() {
		s, _ := n.ElectionStatus()
		b, e := json.Marshal(s)
		if e != nil {
			t.Fatal(e)
		}
		fmt.Println(string(b))
	}
	emit()
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		if input.Text() != "status" {
			t.Fatal("invalid private test command")
		}
		emit()
	}
	if e = input.Err(); e != nil {
		t.Fatal(e)
	}
	if e = n.Shutdown(ctx(t)); e != nil {
		t.Fatal(e)
	}
}

type snapshotProcess struct {
	cmd      *exec.Cmd
	in       io.WriteCloser
	messages chan raft.Status
	done     chan struct{}
	err      error
}

func launchSnapshot(t *testing.T, c config.Config) *snapshotProcess {
	t.Helper()
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
	timeout, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	p := &snapshotProcess{messages: make(chan raft.Status, 1), done: make(chan struct{})}
	p.cmd = exec.CommandContext(timeout, os.Args[0], "-test.run=^TestSnapshotRecoveryChild$")
	p.cmd.Env = append(os.Environ(), "QUORUMKV_M13C_CONFIG="+string(encode(m)), "GOMAXPROCS=2", "GORACE=atexit_sleep_ms=0")
	p.cmd.Stderr = io.Discard
	out, e := p.cmd.StdoutPipe()
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	p.in, e = p.cmd.StdinPipe()
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	if e = p.cmd.Start(); e != nil {
		cancel()
		t.Fatal(e)
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			var s raft.Status
			if json.Unmarshal(scanner.Bytes(), &s) != nil {
				continue
			}
			select {
			case p.messages <- s:
			case <-p.done:
				return
			}
		}
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
		_ = p.in.Close()
		cancel()
	})
	boot := p.next(t)
	if boot.CommitIndex < 5 || boot.LastApplied < 5 || boot.LastApplied > boot.CommitIndex || boot.ScheduledApplyIndex > boot.CommitIndex {
		t.Fatal("partial boot", boot)
	}
	return p
}
func (p *snapshotProcess) next(t *testing.T) raft.Status {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case s := <-p.messages:
		if s.Failure != nil {
			t.Fatal(s.Failure)
		}
		return s
	case <-p.done:
		t.Fatal("snapshot child exited", p.err)
	case <-timer.C:
		t.Fatal("snapshot child status timed out")
	}
	return raft.Status{}
}
func (p *snapshotProcess) status(t *testing.T) raft.Status {
	t.Helper()
	if _, e := io.WriteString(p.in, "status\n"); e != nil {
		t.Fatal(e)
	}
	return p.next(t)
}
func (p *snapshotProcess) kill(t *testing.T) {
	t.Helper()
	if e := p.cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-p.done:
		if p.err == nil {
			t.Fatal("kill returned success")
		}
	case <-ctx(t).Done():
		t.Fatal("child did not join")
	}
}
func (p *snapshotProcess) stop(t *testing.T) {
	t.Helper()
	_ = p.in.Close()
	select {
	case <-p.done:
		if p.err != nil {
			t.Fatal(p.err)
		}
	case <-ctx(t).Done():
		t.Fatal("child shutdown timed out")
	}
}

func TestSnapshotClusterProcessCrashesAndRepeatedSuffixRecovery(t *testing.T) {
	cs, ls := sessionConfigs(t)
	for _, c := range cs {
		snapshotSeed(t, c)
	}
	for _, l := range ls {
		_ = l.Close()
	}
	var ps [3]*snapshotProcess
	for i, c := range cs {
		ps[i] = launchSnapshot(t, c)
	}
	client, e := clientapi.NewClient(cs[0])
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	leader := func() int {
		t.Helper()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		deadline := ctx(t)
		for {
			for i, p := range ps {
				if p == nil {
					continue
				}
				s := p.status(t)
				if s.Role == raft.Leader && !s.Pending && s.LastTerm == s.Term && s.LastApplied >= s.LastIndex {
					return i
				}
			}
			select {
			case <-tick.C:
			case <-deadline.Done():
				t.Fatal("no recovered leader")
			}
		}
	}
	waitApplied := func(index uint64) {
		t.Helper()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		deadline := ctx(t)
		for {
			all := true
			for _, p := range ps {
				if p == nil {
					continue
				}
				s := p.status(t)
				if s.LastApplied < index {
					all = false
				}
				if s.LastApplied > s.CommitIndex || s.ScheduledApplyIndex > s.CommitIndex {
					t.Fatal(s)
				}
			}
			if all {
				return
			}
			select {
			case <-tick.C:
			case <-deadline.Done():
				t.Fatal("recovered cluster did not apply", index)
			}
		}
	}
	write := func(seq int, value string) uint64 {
		t.Helper()
		i := leader()
		raw := encode(map[string]any{"key": "after", "client_id": strings.Repeat("d", 32), "sequence": seq, "value_b64": value})
		r, status, e := client.Write(ctx(t), cs[i].NodeID(), "put", raw)
		if e != nil || status != 200 || r.Code != "OK" || r.Outcome != raft.Completed {
			t.Fatal(r, status, e)
		}
		var index uint64
		if _, e = fmt.Sscan(r.Index, &index); e != nil {
			t.Fatal(e)
		}
		waitApplied(index)
		return index
	}
	read := func(key, want string) {
		t.Helper()
		i := leader()
		r, status, e := client.Get(ctx(t), cs[i].NodeID(), encode(map[string]any{"key": key}))
		if e != nil || status != 200 || (r.ValueB64 == nil || *r.ValueB64 != want) {
			t.Fatal("recovered GET", r, status, e)
		}
	}
	retry := func() {
		t.Helper()
		i := leader()
		r, status, e := client.CAS(ctx(t), cs[i].NodeID(), encode(map[string]any{"key": "x", "client_id": strings.Repeat("a", 32), "sequence": 1, "expected": map[string]any{"exists": false}, "new_value_b64": "QQ=="}))
		if e != nil || status != 200 || r.Index != "1" || r.Applied == nil || !*r.Applied {
			t.Fatal("successful snapshot CAS repeated", r, status, e)
		}
		r, status, e = client.CAS(ctx(t), cs[i].NodeID(), encode(map[string]any{"key": "x", "client_id": strings.Repeat("b", 32), "sequence": 1, "expected": map[string]any{"exists": true, "value_b64": "Qg=="}, "new_value_b64": "Qw=="}))
		if e != nil || status != 409 || r.Code != "COMPARE_FAILED" || r.Index != "4" {
			t.Fatal("failed suffix CAS lost", r, status, e)
		}
	}
	read("x", "QQ==")
	retry()
	write(1, "Wg==")
	follower := (leader() + 1) % 3
	ps[follower].kill(t)
	ps[follower] = nil
	write(2, "WjI=")
	ps[follower] = launchSnapshot(t, cs[follower])
	waitApplied(write(3, "WjM="))
	oldLeader := leader()
	ps[oldLeader].kill(t)
	ps[oldLeader] = nil
	read("after", "WjM=")
	retry()
	ps[oldLeader] = launchSnapshot(t, cs[oldLeader])
	waitApplied(write(4, "WjQ="))
	// Two full crash/restart cycles, with a new committed suffix between them.
	for cycle := 0; cycle < 2; cycle++ {
		for i, p := range ps {
			p.kill(t)
			ps[i] = nil
		}
		for i, c := range cs {
			ps[i] = launchSnapshot(t, c)
		}
		if cycle == 0 {
			read("after", "WjQ=")
		} else {
			read("after", "WjU=")
		}
		read("x", "QQ==")
		retry()
		if cycle == 0 {
			write(5, "WjU=")
		}
	}
	for _, p := range ps {
		p.stop(t)
	}
}
