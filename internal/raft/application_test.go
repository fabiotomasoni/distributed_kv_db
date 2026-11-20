package raft

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

func mutation(index, term uint64, kind, key, value string, expected map[string]any) storageformat.LogEntry {
	command := map[string]any{"key": key, "client_id": strings.Repeat("a", 32), "sequence": index}
	if kind == "put" {
		command["value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	if kind == "cas" {
		command["expected"] = expected
		command["new_value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
	}
	raw, _ := json.Marshal(map[string]any{"index": fmt.Sprint(index), "term": fmt.Sprint(term), "kind": kind, "command": command})
	return storageformat.LogEntry{Index: index, Term: term, Raw: raw}
}
func appRequest(t *testing.T, s *Service, prev, commit uint64, entries []storageformat.LogEntry) peerprotocol.Message {
	t.Helper()
	a := []json.RawMessage{}
	for _, e := range entries {
		a = append(a, e.Raw)
	}
	pt := uint64(0)
	if prev > 0 {
		pt = 6
	}
	return wire(t, s.cfg, peerprotocol.Append, "n2", 6, map[string]any{"leader_id": "n2", "prev_log_index": fmt.Sprint(prev), "prev_log_term": fmt.Sprint(pt), "entries": a, "leader_commit": fmt.Sprint(commit)})
}

type observedMachine struct {
	machine    *statemachine.Machine
	entered    chan struct{}
	release    chan struct{}
	once       sync.Once
	mu         sync.Mutex
	seen       []string
	panicApply bool
}

func (m *observedMachine) Apply(c statemachine.Command) statemachine.Result {
	if m.entered != nil {
		m.once.Do(func() { close(m.entered); <-m.release })
	}
	if m.panicApply {
		panic("injected application failure")
	}
	r := m.machine.Apply(c)
	m.mu.Lock()
	m.seen = append(m.seen, fmt.Sprintf("%s/%s/%s/%s", c.Operation, c.Key, string(c.Value), r.Code))
	m.mu.Unlock()
	return r
}
func newAppService(t *testing.T, d Storage, m applier.Machine, capacity int) *Service {
	t.Helper()
	s, err := NewService(configuration(t, "n1"), d, Options{Epoch: strings.Repeat("a", 32), Clock: newClock(), Draw: minimum, Commit: true, ApplyCapacity: capacity, MachineFactory: func() applier.Machine { return m }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestApplyBacklogCommitFiveThenSevenNoDuplicateScheduling(t *testing.T) {
	m := &observedMachine{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	d := &memoryStorage{s: initial()}
	s := newAppService(t, d, m, 1)
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	expected := map[string]any{"exists": true, "value_b64": base64.StdEncoding.EncodeToString([]byte("one"))}
	entries := []storageformat.LogEntry{logEntry(1, 6), mutation(2, 6, "put", "x", "one", nil), mutation(3, 6, "cas", "x", "two", expected), mutation(4, 6, "cas", "x", "three", expected), mutation(5, 6, "delete", "absent", "", nil), mutation(6, 6, "put", "y", "kept", nil), mutation(7, 6, "delete", "x", "", nil)}
	if _, err := s.Handle(deadline(t), appRequest(t, s, 0, 5, entries[:5])); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("applier did not enter gate")
	}
	// The durable ghost prefix is the oracle even while application is blocked.
	if _, err := s.Handle(deadline(t), appRequest(t, s, 5, 7, entries[5:])); err != nil {
		t.Fatal(err)
	}
	st := s.Status()
	if st.CommitIndex != 7 || st.LastApplied > 1 || st.ScheduledApplyIndex < 2 || st.ScheduledApplyIndex > 3 {
		t.Fatal("backlog invariant", st)
	}
	ghost := d.State()
	if ghost.CommitIndex != 7 {
		t.Fatal("checkpoint did not precede schedule")
	}
	for i, e := range entries {
		if string(ghost.Entries[i].Raw) != string(e.Raw) {
			t.Fatal("ghost committed prefix diverged")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.WaitApplied(canceled, LocalAppend{7, 6}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.Shutdown(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("shutdown ignored active work", err)
	}
	close(m.release)
	if err := s.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	st = s.Status()
	if st.LastApplied != 7 || st.ScheduledApplyIndex != 7 || st.CommitIndex != 7 {
		t.Fatal(st)
	}
	for i := uint64(1); i <= 7; i++ {
		result, err := s.WaitApplied(deadline(t), LocalAppend{i, 6})
		if err != nil || result.Index != i {
			t.Fatal(i, err)
		}
		want := statemachine.OK
		if i == 4 {
			want = statemachine.CompareFailed
		}
		if result.Result.Code != want {
			t.Fatal("domain failure treated as infra error", i, result)
		}
	}
	m.mu.Lock()
	got := append([]string(nil), m.seen...)
	m.mu.Unlock()
	want := []string{"put/x/one/OK", "cas/x/two/OK", "cas/x/three/COMPARE_FAILED", "delete/absent//OK", "put/y/kept/OK", "delete/x//OK"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("double/missing/reordered application", got)
	}
	if r := m.machine.Apply(statemachine.Command{Operation: statemachine.Get, Key: "y"}); string(r.Value) != "kept" {
		t.Fatal(r)
	}
}
func TestApplicationInfrastructureFailureIsTerminal(t *testing.T) {
	m := &observedMachine{machine: statemachine.New(), panicApply: true}
	s := newAppService(t, &memoryStorage{s: initial()}, m, 1)
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	_, _ = s.Handle(deadline(t), appRequest(t, s, 0, 2, []storageformat.LogEntry{logEntry(1, 6), mutation(2, 6, "put", "x", "one", nil)}))
	_, err := s.WaitApplied(deadline(t), LocalAppend{2, 6})
	if !errors.Is(err, applier.ErrApplyFailed) {
		t.Fatal(err)
	}
	if err = s.Shutdown(deadline(t)); !errors.Is(err, applier.ErrApplyFailed) {
		t.Fatal(err)
	}
	if s.Status().LastApplied != 1 || s.Status().CommitIndex != 2 {
		t.Fatal(s.Status())
	}
}
func TestOnlyCommittedPrefixReplaysFreshAndResultsAreBounded(t *testing.T) {
	state := initial()
	state.HardState.Term = 5
	for i := uint64(1); i <= 70; i++ {
		state.Entries = append(state.Entries, logEntry(i, 5))
	}
	state.CommitIndex = 69
	s := newAppService(t, &memoryStorage{s: state}, statemachine.New(), 1)
	if s.Status().LastApplied != 69 || s.Status().ScheduledApplyIndex != 69 {
		t.Fatal("applied speculative suffix")
	}
	if _, err := s.WaitApplied(deadline(t), LocalAppend{1, 5}); !errors.Is(err, ErrResultExpired) {
		t.Fatal(err)
	}
	if _, err := s.WaitApplied(deadline(t), LocalAppend{69, 4}); !errors.Is(err, ErrSuperseded) {
		t.Fatal(err)
	}
	if err := s.Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WaitApplied(deadline(t), LocalAppend{70, 5}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := s.WaitApplied(nil, LocalAppend{0, 0}); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
}
func TestM07RejectsLoggedReadAndRuntimeReconfiguration(t *testing.T) {
	c := replicated(t, "n1", nil, 0)
	if err := c.EnableCommit(); err != nil {
		t.Fatal(err)
	}
	if err := c.EnableCommit(); err == nil {
		t.Fatal("runtime enable accepted")
	}
	e, _ := c.Tick(600)
	e = complete(t, c, e)
	r := e.Send[0]
	e, _ = c.Response(601, r, answer(t, c.cfg, r, 6, map[string]any{"vote_granted": true}))
	complete(t, c, e)
	if _, err := c.AppendLocal(602, "read", json.RawMessage(`{"key":"k"}`)); !errors.Is(err, ErrUnsupported) {
		t.Fatal("M10 leaked into M07", err)
	}
}
