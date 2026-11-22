package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/clientapi"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

// Only child processes enter this branch. They run the same server entry point
// and signal adapter as cmd/quorumkv, without a compiler or paid service.
func TestNodeProcess(t *testing.T) {
	if os.Getenv("QUORUMKV_M09_CHILD") != "1" {
		t.Skip("subprocess entry point")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := Run(ctx, args, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

type boundedLog struct {
	mu sync.Mutex
	b  []byte
}

func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	left := 16*1024 - len(b.b)
	if left > len(p) {
		left = len(p)
	}
	if left > 0 {
		b.b = append(b.b, p[:left]...)
	}
	return n, nil
}
func (b *boundedLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.b) }

type process struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	log  boundedLog
}

func launch(t *testing.T, path string, bootstrap bool) *process {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	args := []string{"-test.run=^TestNodeProcess$", "--", "-config", path}
	if bootstrap {
		args = append(args, "-bootstrap")
	}
	p := &process{done: make(chan struct{})}
	p.cmd = exec.Command(exe, args...)
	p.cmd.Env = append(os.Environ(), "QUORUMKV_M09_CHILD=1", "GOMAXPROCS=2", "GORACE=atexit_sleep_ms=0")
	p.cmd.Stdout = &p.log
	p.cmd.Stderr = &p.log
	if e = p.cmd.Start(); e != nil {
		t.Fatal(e)
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		select {
		case <-p.done:
			return
		default:
		}
		_ = p.cmd.Process.Kill()
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
			t.Error("child did not terminate")
		}
	})
	return p
}
func (p *process) stop(t *testing.T, graceful bool) {
	t.Helper()
	select {
	case <-p.done:
		return
	default:
	}
	var e error
	if graceful {
		e = p.cmd.Process.Signal(syscall.SIGTERM)
	} else {
		e = p.cmd.Process.Kill()
	}
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-p.done:
		if graceful && p.err != nil {
			t.Fatal(p.err, p.log.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process stop deadline", p.log.String())
	}
}
func fixtureConfigs(t *testing.T) ([3]config.Config, [3]string) {
	t.Helper()
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration map[string]any }
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	var reservations []net.Listener
	for _, v := range f.Configuration["members"].([]any) {
		m := v.(map[string]any)
		for _, k := range []string{"peer", "client"} {
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			reservations = append(reservations, l)
			m[k] = l.Addr().String()
		}
	}
	defer func() {
		for _, l := range reservations {
			_ = l.Close()
		}
	}()
	var cs [3]config.Config
	var paths [3]string
	for i, v := range f.Configuration["members"].([]any) {
		m := v.(map[string]any)
		d := t.TempDir()
		if e = os.Chmod(d, 0700); e != nil {
			t.Fatal(e)
		}
		f.Configuration["data_dir"] = d
		f.Configuration["node_id"] = m["id"]
		f.Configuration["client_addr"] = m["client"]
		f.Configuration["peer_addr"] = m["peer"]
		b, _ := json.Marshal(f.Configuration)
		cs[i], e = config.Parse(b)
		if e != nil {
			t.Fatal(e)
		}
		paths[i] = filepath.Join(t.TempDir(), "config.json")
		if e = os.WriteFile(paths[i], b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return cs, paths
}
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func poll(t *testing.T, condition func() bool) {
	t.Helper()
	ctx := testContext(t)
	timer := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()
	for !condition() {
		select {
		case <-timer.C:
		case <-ctx.Done():
			t.Fatal("condition deadline")
		}
	}
}

type localStatus struct {
	NodeID  string `json:"node_id"`
	Role    string `json:"role"`
	Term    string `json:"term"`
	Commit  string `json:"commit_index"`
	Applied string `json:"last_applied"`
}

func status(c config.Config) (localStatus, bool) {
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 50 * time.Millisecond}).DialContext, DisableCompression: true, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 200 * time.Millisecond}
	r, e := client.Get("http://" + c.Local().Client + "/v1/status")
	if e != nil {
		return localStatus{}, false
	}
	defer r.Body.Close()
	var s localStatus
	e = json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&s)
	return s, e == nil && r.StatusCode == 200 && s.NodeID == c.NodeID()
}
func leader(t *testing.T, cs [3]config.Config, ps [3]*process) int {
	t.Helper()
	found := -1
	poll(t, func() bool {
		for i, p := range ps {
			if p == nil {
				continue
			}
			select {
			case <-p.done:
				continue
			default:
			}
			s, ok := status(cs[i])
			if ok && s.Role == "leader" {
				found = i
				return true
			}
		}
		return false
	})
	return found
}
func id(c config.Config) wal.Identity {
	return wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()}
}
func readStore(t *testing.T, c config.Config) *wal.Store {
	t.Helper()
	s, e := wal.Open(c.DataDir(), id(c))
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func wireCommand(kind, key, value string, seq int) []byte {
	m := map[string]any{"key": key, "client_id": strings.Repeat("a", 32), "sequence": seq}
	if kind == "put" {
		m["value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	b, _ := json.Marshal(m)
	return b
}
func completed(t *testing.T, client *clientapi.Client, c config.Config, kind, key, value string, seq int) clientapi.Response {
	t.Helper()
	r, s, e := client.Write(testContext(t), c.NodeID(), kind, wireCommand(kind, key, value, seq))
	if e != nil || s != 200 || r.Outcome != raft.Completed {
		t.Fatal("write not completed", s, r, e)
	}
	return r
}

// Inspect a real fresh production replay only after its sole M02 owner stops.
// This is controlled test inspection, not an HTTP GET or another application path.
func inspectReplay(t *testing.T, c config.Config, w *wal.Store, want map[string]string, absent ...string) {
	t.Helper()
	var machine *statemachine.Machine
	s, e := raft.NewService(c, w, raft.Options{Commit: true, MachineFactory: func() applier.Machine { machine = statemachine.New(); return machine }})
	if e != nil {
		t.Fatal(e)
	}
	st := s.Status()
	if st.Role != raft.Follower || st.LastApplied != w.State().CommitIndex || st.ScheduledApplyIndex != st.LastApplied {
		t.Fatal("replay state", st)
	}
	if e = s.Shutdown(testContext(t)); e != nil {
		t.Fatal(e)
	}
	for k, v := range want {
		r := machine.Apply(statemachine.Command{Operation: statemachine.Get, Key: k})
		if r.Code != statemachine.OK || string(r.Value) != v {
			t.Fatal("acknowledged value lost", k, r.Code)
		}
	}
	for _, k := range absent {
		r := machine.Apply(statemachine.Command{Operation: statemachine.Get, Key: k})
		if r.Code != statemachine.KeyNotFound {
			t.Fatal("unexpected replayed key", k)
		}
	}
}
func TestProcessCrashOneThenAllPreservesAcknowledgedHistory(t *testing.T) {
	cs, paths := fixtureConfigs(t)
	var ps [3]*process
	for i := range ps {
		ps[i] = launch(t, paths[i], true)
	}
	for i := range ps {
		poll(t, func() bool { _, ok := status(cs[i]); return ok })
	}
	client, e := clientapi.NewClient(cs[0])
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	first := leader(t, cs, ps)
	expected := map[string]string{"keep": "M09_PRIVATE_VALUE", "empty": ""}
	var receipts []clientapi.Response
	receipts = append(receipts, completed(t, client, cs[first], "put", "keep", expected["keep"], 1), completed(t, client, cs[first], "put", "empty", "", 2), completed(t, client, cs[first], "put", "gone", "remove", 3))
	r := completed(t, client, cs[first], "delete", "gone", "", 4)
	if r.Deleted == nil || !*r.Deleted {
		t.Fatal(r)
	}
	receipts = append(receipts, r)
	follower := (first + 1) % 3
	ps[follower].stop(t, false)
	expected["after_follower"] = "f"
	r = completed(t, client, cs[first], "put", "after_follower", "f", 5)
	receipts = append(receipts, r)
	ps[follower] = launch(t, paths[follower], false)
	target, _ := strconv.ParseUint(r.Index, 10, 64)
	poll(t, func() bool {
		s, ok := status(cs[follower])
		n, _ := strconv.ParseUint(s.Applied, 10, 64)
		return ok && n >= target
	})
	ps[first].stop(t, false)
	next := leader(t, cs, ps)
	expected["after_leader"] = "l"
	r = completed(t, client, cs[next], "put", "after_leader", "l", 6)
	receipts = append(receipts, r)
	ps[first] = launch(t, paths[first], false)
	target, _ = strconv.ParseUint(r.Index, 10, 64)
	for i := range ps {
		poll(t, func() bool {
			s, ok := status(cs[i])
			n, _ := strconv.ParseUint(s.Applied, 10, 64)
			return ok && n >= target
		})
	}
	// Kill all before waiting: none gets an orderly shutdown/application drain.
	for _, p := range ps {
		if e = p.cmd.Process.Kill(); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range ps {
		select {
		case <-p.done:
		case <-testContext(t).Done():
			t.Fatal("kill did not reap")
		}
	}
	var ghost [][]byte
	for i, c := range cs {
		w := readStore(t, c)
		state := w.State()
		if state.CommitIndex < target {
			t.Fatal("checkpoint regressed")
		}
		for _, receipt := range receipts {
			index, _ := strconv.ParseUint(receipt.Index, 10, 64)
			raw := state.Entries[index-1].Raw
			if i == 0 {
				ghost = append(ghost, bytes.Clone(raw))
			} else {
				j := 0
				for receipts[j].Index != receipt.Index {
					j++
				}
				if !bytes.Equal(raw, ghost[j]) {
					t.Fatal("acknowledged prefix differs")
				}
			}
		}
		inspectReplay(t, c, w, expected, "gone")
		if e = w.Close(); e != nil {
			t.Fatal(e)
		}
	}
	// Restart intact directories, form a new quorum and prove continued writes.
	for i := range ps {
		ps[i] = launch(t, paths[i], false)
	}
	next = leader(t, cs, ps)
	r = completed(t, client, cs[next], "delete", "keep", "", 7)
	if r.Deleted == nil || !*r.Deleted {
		t.Fatal("completed PUT missing after all-process crash", r)
	}
	completed(t, client, cs[next], "put", "post_restart", "ok", 8)
	for _, p := range ps {
		p.stop(t, true)
		log := p.log.String()
		if strings.Contains(log, "M09_PRIVATE_VALUE") || strings.Contains(log, base64.StdEncoding.EncodeToString([]byte("M09_PRIVATE_VALUE"))) {
			t.Fatal("value leaked into logs")
		}
	}
	for _, c := range cs {
		w := readStore(t, c)
		_ = w.Close()
		for _, addr := range []string{c.Local().Peer, c.Local().Client} {
			l, e := net.Listen("tcp", addr)
			if e != nil {
				t.Fatal("listener leaked", e)
			}
			_ = l.Close()
		}
	}
}
func TestProcessSignalWithPendingWritePreservesUncertaintyAndRecovery(t *testing.T) {
	cs, paths := fixtureConfigs(t)
	var ps [3]*process
	for i := range ps {
		ps[i] = launch(t, paths[i], true)
	}
	current := leader(t, cs, ps)
	client, _ := clientapi.NewClient(cs[0])
	defer client.Close()
	completed(t, client, cs[current], "put", "safe", "safe", 1)
	for i, p := range ps {
		if i != current {
			p.stop(t, true)
		}
	}
	walPath := filepath.Join(cs[current].DataDir(), "gen-1", "wal")
	before, e := os.Stat(walPath)
	if e != nil {
		t.Fatal(e)
	}
	pending := wireCommand("put", "speculative", "secret", 2)
	var o map[string]any
	_ = json.Unmarshal(pending, &o)
	o["timeout_ms"] = 5000
	pending, _ = json.Marshal(o)
	type result struct {
		r   clientapi.Response
		err error
	}
	done := make(chan result, 1)
	go func() {
		r, _, e := client.Write(testContext(t), cs[current].NodeID(), "put", pending)
		done <- result{r, e}
	}()
	// No quorum and no concurrent log-producing workload. Observing growth gates
	// termination on a real attempted record, rather than an arbitrary sleep.
	poll(t, func() bool { now, e := os.Stat(walPath); return e == nil && now.Size() > before.Size() })
	ps[current].stop(t, true)
	select {
	case r := <-done:
		if r.r.Outcome != raft.Unknown {
			t.Fatal("pending write claimed exclusion/completion", r)
		}
	case <-testContext(t).Done():
		t.Fatal("client wait leaked")
	}
	w := readStore(t, cs[current])
	state := w.State()
	if state.LastIndex() <= state.CommitIndex {
		t.Fatal("no speculative suffix")
	}
	inspectReplay(t, cs[current], w, map[string]string{"safe": "safe"}, "speculative")
	_ = w.Close()
	// A lone restarted node replays only commit before binding. It cannot turn
	// mere log length into commit; even a later self-vote is not a majority.
	p := launch(t, paths[current], false)
	poll(t, func() bool {
		s, ok := status(cs[current])
		return ok && s.Applied == fmt.Sprint(state.CommitIndex) && s.Commit == s.Applied
	})
	p.stop(t, true)
}
