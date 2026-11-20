package raft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func writeCommand() json.RawMessage {
	return json.RawMessage(`{"key":"x","value_b64":"eA==","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1}`)
}
func waitWrite(t *testing.T, condition func() bool) {
	t.Helper()
	timer := time.NewTicker(time.Millisecond)
	defer timer.Stop()
	ctx := deadline(t)
	for !condition() {
		select {
		case <-timer.C:
		case <-ctx.Done():
			t.Fatal("write test wait expired")
		}
	}
}
func electWriteService(t *testing.T, s *Service) {
	t.Helper()
	if err := s.Start(senderFunc(func(ctx context.Context, m peerprotocol.Message) (peerprotocol.Message, error) {
		s.clock.(*manualClock).now.Add(1)
		var o map[string]any
		_ = json.Unmarshal(m.Bytes(), &o)
		fields := map[string]any{"vote_granted": true}
		if _, ok := o["entries"]; ok {
			var end uint64
			_, _ = fmt.Sscan(o["prev_log_index"].(string), &end)
			end += uint64(len(o["entries"].([]any)))
			fields = map[string]any{"success": true, "match_index": fmt.Sprint(end)}
		}
		return answer(t, s.cfg, m, m.Envelope().Term, fields), nil
	})); err != nil {
		t.Fatal(err)
	}
	s.clock.(*manualClock).tick(600)
	waitWrite(t, func() bool { return s.Status().Role == Leader && s.Status().LastApplied >= 1 })
	t.Cleanup(func() { _ = s.Shutdown(deadline(t)) })
}
func TestWriteWaitsActualApplyAndCancellationDoesNotRollback(t *testing.T) {
	m := &observedMachine{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	s := newAppService(t, &memoryStorage{s: initial()}, m, 1)
	electWriteService(t, s)
	ctx, cancel := context.WithCancel(deadline(t))
	type result struct {
		o   WriteOutcome
		err error
	}
	done := make(chan result, 1)
	go func() { o, e := s.Write(ctx, "put", writeCommand()); done <- result{o, e} }()
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("no mutation")
	}
	select {
	case r := <-done:
		t.Fatal("success before application", r)
	default:
	}
	cancel()
	r := <-done
	if r.o.Outcome != Unknown || !errors.Is(r.err, context.Canceled) {
		t.Fatal(r)
	}
	close(m.release)
	waitWrite(t, func() bool { return s.Status().LastApplied == 2 })
	// The next independent PUT completes at a new index, even with same identity.
	o, err := s.Write(deadline(t), "put", writeCommand())
	if err != nil || o.Outcome != Completed || o.Applied.Index != 3 || !o.Applied.Result.Applied {
		t.Fatal(o, err)
	}
}
func TestWriteDiskBoundaryQueuedCancellationAndPeerReserve(t *testing.T) {
	d := &memoryStorage{s: initial()}
	s := newAppService(t, d, statemachine.New(), 1)
	electWriteService(t, s)
	// Storage controls are changed while the manual clock/network are quiescent;
	// the next owner event is the client proposal. No asynchronous disk activity.
	d.entered = make(chan struct{}, 1)
	d.release = make(chan struct{})
	firstCtx, firstCancel := context.WithCancel(deadline(t))
	first := make(chan WriteOutcome, 1)
	go func() { o, _ := s.Write(firstCtx, "put", writeCommand()); first <- o }()
	select {
	case <-d.entered:
	case <-deadline(t).Done():
		t.Fatal("disk did not block")
	}
	queuedCtx, queuedCancel := context.WithCancel(deadline(t))
	queued := make(chan WriteOutcome, 1)
	go func() { o, _ := s.Write(queuedCtx, "put", writeCommand()); queued <- o }()
	waitWrite(t, func() bool { return len(s.writes) == 1 })
	queuedCancel()
	if o := <-queued; o.Outcome != NotAdmitted {
		t.Fatal(o)
	}
	firstCancel()
	if o := <-first; o.Outcome != Unknown {
		t.Fatal(o)
	}
	// A canceled queued ticket is permanently excluded, and client saturation
	// cannot fill the separate 256-slot peer event queue.
	for i := 0; i < 64; i++ {
		s.writeSlots <- struct{}{}
	}
	o, err := s.Write(deadline(t), "put", writeCommand())
	if o.Outcome != NotAdmitted || !errors.Is(err, ErrFull) || len(s.events) != 0 {
		t.Fatal(o, err)
	}
	for i := 0; i < 64; i++ {
		<-s.writeSlots
	}
	// Disable the gate before releasing its current wait, while owner blocked.
	release := d.release
	d.entered = nil
	close(release)
	waitWrite(t, func() bool { return s.Status().LastApplied == 2 && len(s.writes) == 0 })
	if d.State().LastIndex() != 2 {
		t.Fatal("canceled queued request was appended")
	}
}
func TestWriteFenceRejectsObservedLeadershipLossAndInvalidProgress(t *testing.T) {
	s := newAppService(t, &memoryStorage{s: initial()}, statemachine.New(), 1)
	// Direct owner events, no goroutines: exercise the exact authorization fence.
	s.core.state.Role = Leader
	s.core.state.Term = 6
	s.core.state.CommitIndex = 2
	s.core.state.LastApplied = 1
	check := func(want error) {
		f := writeFence{LocalAppend{2, 6}, make(chan error, 1)}
		s.processFence(f)
		if e := <-f.reply; !errors.Is(e, want) {
			t.Fatal(e, want)
		}
	}
	check(ErrInput)
	s.core.state.LastApplied = 2
	check(nil)
	s.core.state.Role = Follower
	check(ErrNotLeader)
	s.core.state.Role = Leader
	s.core.state.Term = 7
	check(ErrNotLeader)
	_ = s.Shutdown(deadline(t))
}
func TestWriteAdmissionAndPrewriteStorageRejection(t *testing.T) {
	s := newAppService(t, &memoryStorage{s: initial()}, statemachine.New(), 1)
	if o, e := s.Write(nil, "put", writeCommand()); o.Outcome != NotAdmitted || !errors.Is(e, ErrInput) {
		t.Fatal(o, e)
	}
	if o, e := s.Write(deadline(t), "cas", writeCommand()); o.Outcome != NotAdmitted || !errors.Is(e, ErrInput) {
		t.Fatal(o, e)
	}
	if o, e := s.Write(deadline(t), "put", writeCommand()); o.Outcome != NotAdmitted || !errors.Is(e, ErrClosed) {
		t.Fatal(o, e)
	}
	if err := s.Start(senderFunc(noNetwork)); err != nil {
		t.Fatal(err)
	}
	if o, e := s.Write(deadline(t), "put", writeCommand()); o.Outcome != NotAdmitted || !errors.Is(e, ErrNotLeader) {
		t.Fatal(o, e)
	}
	_ = s.Shutdown(deadline(t))
	d := &memoryStorage{s: initial()}
	s = newAppService(t, d, statemachine.New(), 1)
	electWriteService(t, s)
	d.failure = wal.ErrLimit
	o, e := s.Write(deadline(t), "put", writeCommand())
	if o.Outcome != NotAdmitted || !errors.Is(e, wal.ErrLimit) {
		t.Fatal(o, e)
	}
	d.failure = nil
	o, e = s.Write(deadline(t), "put", json.RawMessage(strings.Repeat("x", 49*1024)))
	if o.Outcome != NotAdmitted || e == nil {
		t.Fatal(o, e)
	}
}

func TestWriteObservedHigherTermWhileApplyBlockedIsUnknown(t *testing.T) {
	m := &observedMachine{machine: statemachine.New(), entered: make(chan struct{}), release: make(chan struct{})}
	s := newAppService(t, &memoryStorage{s: initial()}, m, 1)
	electWriteService(t, s)
	type result struct {
		o   WriteOutcome
		err error
	}
	done := make(chan result, 1)
	go func() { o, e := s.Write(deadline(t), "put", writeCommand()); done <- result{o, e} }()
	select {
	case <-m.entered:
	case <-deadline(t).Done():
		t.Fatal("apply did not block")
	}
	st := s.Status()
	higher := heartbeat(t, s.cfg, "n2", st.Term+1, st.LastIndex, st.LastTerm, st.CommitIndex)
	if _, e := s.Handle(deadline(t), higher); e != nil {
		t.Fatal(e)
	}
	close(m.release)
	r := <-done
	if r.o.Outcome != Unknown || !errors.Is(r.err, ErrNotLeader) {
		t.Fatal("old leader completed client result", r)
	}
	if s.Status().LastApplied != 2 {
		t.Fatal("stepdown canceled committed apply")
	}
}
