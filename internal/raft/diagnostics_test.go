package raft

import (
	"context"
	"encoding/json"
	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/diagnostics"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiagnosticsObserveElectionDedupAndUnknownWithoutPayload(t *testing.T) {
	d := &diagnosticProposalQuota{sessionStorage: &sessionStorage{&memoryStorage{s: initial()}}}
	s := newSessionService(t, d, statemachine.New())
	electWriteService(t, s)
	cmd := casAbsent(strings.Repeat("a", 32), 1)
	r, e := s.Write(deadline(t), "cas", cmd)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.Write(deadline(t), "cas", cmd)
	if e != nil || r.ResultIndex != retry.ResultIndex {
		t.Fatal(retry, e)
	}
	snap := s.Diagnostics().Snapshot()
	if snap.Counters["election_started"] < 1 || snap.Counters["leader_changes"] < 1 || snap.Counters["dedup_hits"] != 1 || snap.Counters["rpc_sent"] < 1 {
		t.Fatal(snap)
	}
	m := s.Metrics()
	if m["local_only"] != true || m["apply_lag"] != uint64(0) {
		t.Fatal(m)
	}
	raw, _ := json.Marshal(s.Diagnostics().Drain())
	if strings.Contains(string(raw), "Qg==") || strings.Contains(string(raw), strings.Repeat("a", 32)) || strings.Contains(string(raw), `"key"`) {
		t.Fatal("payload leaked", string(raw))
	}
	d.full.Store(true)
	refused, e := s.Write(deadline(t), "cas", cmd)
	if e == nil || refused.Outcome != NotAdmitted || s.Status().Failure != nil {
		t.Fatal("quota classification", refused, e)
	}
	for _, record := range s.Diagnostics().Drain() {
		if record.Event == diagnostics.StorageError {
			t.Fatal("proven quota refusal labeled fatal")
		}
	}
	ctx, c := context.WithCancel(context.Background())
	c()
	if _, e = s.Write(ctx, "cas", cmd); e == nil {
		t.Fatal("cancellation lost")
	}
	if e = s.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
}

type diagnosticProposalQuota struct {
	*sessionStorage
	full atomic.Bool
}

func (d *diagnosticProposalQuota) AppendProposal(raw []byte) (wal.Token, error) {
	if d.full.Load() {
		return wal.Token{}, wal.ErrLimit
	}
	return d.sessionStorage.AppendProposal(raw)
}

func TestAppliedStatusPublishedBeforeResultNotification(t *testing.T) {
	// A valid durable suffix whose first noop has just become committed. Gate
	// result delivery so status publication order is observable without sleeps.
	disk := &memoryStorage{s: wal.State{Sequence: 1, HardState: storageformat.HardState{Term: 1}, Entries: []storageformat.LogEntry{logEntry(1, 1)}}}
	s := newAppService(t, disk, statemachine.New(), 1)
	s.core.state.CommitIndex = 1
	if e := s.core.Scheduled(1); e != nil {
		t.Fatal(e)
	}
	s.application.inflight = 1
	s.application.mu.Lock()
	done := make(chan struct{})
	go func() {
		s.applyCompleted(applyCompletion{outcome: applier.Outcome{Index: 1, Result: statemachine.Result{Code: statemachine.OK}}, epoch: 1, term: 1})
		close(done)
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	published := false
waiting:
	for {
		if s.Status().LastApplied == 1 {
			published = true
			break
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			break waiting
		}
	}
	s.application.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion stuck")
	}
	if e := s.Shutdown(context.Background()); e != nil {
		t.Fatal(e)
	}
	if !published {
		t.Fatal("result notification preceded published applied cursor")
	}
}
