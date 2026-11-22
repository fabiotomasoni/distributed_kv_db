package snapshot

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

func cfg(t *testing.T) config.Config {
	t.Helper()
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v2-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration json.RawMessage }
	_ = json.Unmarshal(raw, &f)
	c, e := config.Parse(f.Configuration)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(c)
	return ctx
}
func worker(t *testing.T, m applier.Machine, base uint64) *applier.Applier {
	t.Helper()
	a, e := applier.New(m, applier.Config{Capacity: 4, InitialIndex: base})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := a.Shutdown(deadline(t)); e != nil && !errors.Is(e, applier.ErrApplyFailed) {
			t.Error(e)
		}
	})
	return a
}
func entry(index uint64, kind string, client int, seq uint64, value string, expected map[string]any) storageformat.LogEntry {
	var command any
	if kind == "read" {
		command = map[string]any{"key": "x"}
	} else if kind != "noop" {
		m := map[string]any{"key": "x", "client_id": fmt.Sprintf("%032x", client), "sequence": seq}
		if kind == "put" {
			m["value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
		}
		if kind == "cas" {
			m["expected"] = expected
			m["new_value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
		}
		command = m
	}
	term := uint64(1)
	if index >= 3 {
		term = 2
	}
	o := map[string]any{"index": fmt.Sprint(index), "term": fmt.Sprint(term), "kind": kind}
	if command != nil {
		o["command"] = command
	}
	raw, _ := json.Marshal(o)
	return storageformat.LogEntry{Index: index, Term: term, Raw: raw}
}
func submit(t *testing.T, a *applier.Applier, e storageformat.LogEntry) *applier.Future {
	t.Helper()
	command, err := committed.ProjectWithSessions(e)
	if err != nil {
		t.Fatal(err)
	}
	f, err := a.Submit(deadline(t), applier.Entry{Index: e.Index, Command: command})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func applied(t *testing.T, f *applier.Future) (statemachine.Result, uint64) {
	t.Helper()
	o, e := f.Wait(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	r, index, e := committed.DecodeSessionResult(o.Result, o.Index)
	if e != nil {
		t.Fatal(e)
	}
	return r, index
}
func proof(index, term, commit uint64) Proof {
	return Proof{LastApplied: index, ScheduledApplyIndex: index, CommitIndex: commit, ApplyEpoch: 1, TermAt: func(i uint64) (uint64, error) {
		if i != index {
			return 0, ErrBoundary
		}
		return term, nil
	}}
}
func capture(t *testing.T, a *applier.Applier, index, term, commit uint64) storageformat.Snapshot {
	t.Helper()
	b, e := a.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	defer b.Release(deadline(t))
	s, e := Capture(deadline(t), b, proof(index, term, commit))
	if e != nil {
		t.Fatal(e)
	}
	return s
}

type afterKV struct {
	inner            applier.Machine
	entered, release chan struct{}
	once             sync.Once
}

func (g *afterKV) Apply(c statemachine.Command) statemachine.Result {
	r := g.inner.Apply(c)
	if c.Operation == statemachine.CAS {
		g.once.Do(func() { close(g.entered); <-g.release })
	}
	return r
}
func TestCaptureDrainsAtomicCASSessionAndExactReadBoundary(t *testing.T) {
	m, e := NewMachine(cfg(t))
	if e != nil {
		t.Fatal(e)
	}
	gate := &afterKV{inner: m.kv, entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(gate.release) }) })
	m.sessions = committed.NewSessions(gate)
	m.delegate = committed.Wrap(m.sessions)
	a := worker(t, m, 0)
	t.Cleanup(func() { once.Do(func() { close(gate.release) }) })
	applied(t, submit(t, a, entry(1, "put", 1, 1, "A", nil)))
	cas := submit(t, a, entry(2, "cas", 2, 1, "B", map[string]any{"exists": true, "value_b64": "QQ=="}))
	select {
	case <-gate.entered:
	case <-deadline(t).Done():
		t.Fatal("CAS gate")
	}
	read := submit(t, a, entry(3, "read", 0, 0, "", nil))
	leases := make(chan *applier.Barrier, 1)
	failed := make(chan error, 1)
	go func() {
		b, e := a.Park(deadline(t))
		if e != nil {
			failed <- e
		} else {
			leases <- b
		}
	}()
	// Observe freeze through the public bounded admission interface. No Sleep.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
wait:
	for {
		if a.AdmissionFrozen() {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline(t).Done():
			t.Fatal("freeze not observed")
			break wait
		}
	}

	if p := a.Progress(); p.LastApplied != 1 || p.Scheduled != 3 {
		t.Fatal(p)
	}
	select {
	case <-leases:
		t.Fatal("capture crossed incomplete CAS/session update")
	default:
	}
	once.Do(func() { close(gate.release) })
	r, index := applied(t, cas)
	if r.Code != statemachine.OK || index != 2 {
		t.Fatal(r, index)
	}
	r, _ = applied(t, read)
	if string(r.Value) != "B" {
		t.Fatal(r)
	}
	var b *applier.Barrier
	select {
	case b = <-leases:
	case e := <-failed:
		t.Fatal(e)
	case <-deadline(t).Done():
		t.Fatal("capture did not park")
	}
	if _, e = Capture(deadline(t), b, proof(1, 1, 7)); !errors.Is(e, ErrBoundary) {
		t.Fatal("scheduled/old proof accepted", e)
	}
	image, e := Capture(deadline(t), b, proof(3, 2, 7))
	if e != nil || image.Index != 3 || image.Term != 2 || string(image.KV[0].Value) != "B" || len(image.Sessions) != 2 || image.Sessions[1].Result.Index != 2 {
		t.Fatal(image, e)
	}
	if e = b.Release(deadline(t)); e != nil {
		t.Fatal(e)
	}
	before, e := storageformat.EncodeSnapshot(image)
	if e != nil {
		t.Fatal(e)
	}
	applied(t, submit(t, a, entry(4, "noop", 0, 0, "", nil)))
	applied(t, submit(t, a, entry(5, "put", 3, 1, "C", nil)))
	after, e := storageformat.EncodeSnapshot(image)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("later apply changed capture")
	}
	image.KV[0].Value[0] = '!'
	image.Sessions[0].Result.Code = "BROKEN"
	independent := capture(t, a, 5, 2, 7)
	if string(independent.KV[0].Value) != "C" || independent.Sessions[0].Result.Code != "OK" {
		t.Fatal("caller altered machine", independent)
	}
}

func TestIsolatedRestorePreservesSessionsAndSuffixWithoutReapplication(t *testing.T) {
	c := cfg(t)
	m, e := NewMachine(c)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, m, 0)
	expectedA := map[string]any{"exists": true, "value_b64": "QQ=="}
	initial := []storageformat.LogEntry{
		entry(1, "put", 1, 1, "A", nil), entry(2, "cas", 2, 1, "B", expectedA), entry(3, "cas", 3, 1, "C", expectedA), entry(4, "delete", 4, 1, "", nil), entry(5, "delete", 5, 1, "", nil), entry(6, "put", 1, 2, "A", nil), entry(7, "read", 0, 0, "", nil), entry(8, "noop", 0, 0, "", nil), entry(9, "cas", 2, 1, "B", expectedA), entry(10, "cas", 3, 1, "C", expectedA),
	}
	for _, e := range initial {
		applied(t, submit(t, a, e))
	}
	image := capture(t, a, 10, 2, 10)
	raw, e := storageformat.EncodeSnapshot(image)
	if e != nil {
		t.Fatal(e)
	}
	saved := bytes.Clone(raw)
	restored, point, e := Restore(c, raw, 4)
	if e != nil || point.Index != 10 || point.Term != 2 {
		t.Fatal(point, e)
	}
	b := worker(t, restored, point.Index)
	for i := range raw {
		raw[i] = 0
	}
	image.KV[0].Value[0] = '!'
	image.Sessions[0].Result.Code = "BROKEN"
	copyImage := capture(t, b, 10, 2, 10)
	again, e := storageformat.EncodeSnapshot(copyImage)
	if e != nil || !bytes.Equal(again, saved) {
		t.Fatal("restore alias or reapplied state", e)
	}
	suffix := []storageformat.LogEntry{entry(11, "cas", 2, 1, "B", expectedA), entry(12, "cas", 3, 1, "C", expectedA), entry(13, "put", 1, 1, "A", nil), entry(14, "put", 1, 4, "gap", nil), entry(15, "put", 1, 2, "conflict", nil), entry(16, "delete", 6, 1, "", nil), entry(17, "cas", 1, 3, "", map[string]any{"exists": false}), entry(18, "read", 0, 0, "", nil), entry(19, "noop", 0, 0, "", nil)}
	codes := []statemachine.Code{statemachine.OK, statemachine.CompareFailed, committed.StaleSequence, committed.SequenceGap, committed.RequestConflict, statemachine.OK, statemachine.OK, statemachine.OK, statemachine.OK}
	for i, e := range suffix {
		original, oi := applied(t, submit(t, a, e))
		replay, ri := applied(t, submit(t, b, e))
		if !reflect.DeepEqual(original, replay) || oi != ri || replay.Code != codes[i] {
			t.Fatal(e.Index, original, replay, oi, ri)
		}
		if e.Index == 11 && ri != 2 || e.Index == 12 && ri != 3 {
			t.Fatal("original result lost", ri)
		}
		if e.Index == 18 && (!replay.Exists || len(replay.Value) != 0) {
			t.Fatal("present empty read changed", replay)
		}
	}
	left, e := storageformat.EncodeSnapshot(capture(t, a, 19, 2, 19))
	if e != nil {
		t.Fatal(e)
	}
	right, e := storageformat.EncodeSnapshot(capture(t, b, 19, 2, 19))
	if e != nil || !bytes.Equal(left, right) {
		t.Fatal("suffix changed restored logical state", e)
	}
}
func TestRestoreSessionCapacityAndCachedQuotaResult(t *testing.T) {
	c := cfg(t)
	s := storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: 2048, Term: 2}
	command := entry(1, "put", 1, 1, "", nil)
	digest, e := storageformat.CommandDigest(command.Raw)
	if e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 1024; i++ {
		s.Sessions = append(s.Sessions, storageformat.SnapshotSession{ClientID: fmt.Sprintf("%032x", i), Sequence: 1, CommandSHA256: hex.EncodeToString(digest[:]), Kind: "put", Result: storageformat.SnapshotResult{Code: "RESOURCE_LIMIT", Index: uint64(i)}})
	}
	raw, e := storageformat.EncodeSnapshot(s)
	if e != nil {
		t.Fatal(e)
	}
	m, p, e := Restore(c, raw, 2)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, m, p.Index)
	r, index := applied(t, submit(t, a, entry(2049, "put", 1, 1, "", nil)))
	if r.Code != statemachine.ResourceLimit || index != 1 {
		t.Fatal("negative quota result was reapplied", r, index)
	}
	r, _ = applied(t, submit(t, a, entry(2050, "put", 1025, 1, "new", nil)))
	if r.Code != committed.SessionLimit {
		t.Fatal(r)
	}
	r, _ = applied(t, submit(t, a, entry(2051, "put", 1, 2, "next", nil)))
	if r.Code != statemachine.OK {
		t.Fatal(r)
	}
	r, _ = applied(t, submit(t, a, entry(2052, "put", 1, 1, "", nil)))
	if r.Code != committed.StaleSequence {
		t.Fatal(r)
	}
	captured := capture(t, a, 2052, 2, 2052)
	if len(captured.Sessions) != 1024 || len(captured.KV) != 1 || string(captured.KV[0].Value) != "next" {
		t.Fatal("capacity/retention", len(captured.Sessions))
	}
}
func TestCaptureMetadataProfilesAndShutdownErrors(t *testing.T) {
	c := cfg(t)
	m, _ := NewMachine(c)
	a := worker(t, m, 0)
	b, e := a.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Capture(nil, b, Proof{}); !errors.Is(e, applier.ErrContext) {
		t.Fatal(e)
	}
	if _, e = Capture(deadline(t), nil, Proof{}); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
	for _, p := range []Proof{{}, {LastApplied: 1, ScheduledApplyIndex: 1, CommitIndex: 1, ApplyEpoch: 1}, {ApplyEpoch: 2}, {ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return 1, nil }}} {
		if _, e = Capture(deadline(t), b, p); !errors.Is(e, ErrBoundary) {
			t.Fatal("bad proof", e)
		}
	}
	empty, e := Capture(deadline(t), b, proof(0, 0, 0))
	if e != nil || empty.Index != 0 {
		t.Fatal(e)
	}
	if e = b.Release(deadline(t)); e != nil {
		t.Fatal(e)
	}
	applied(t, submit(t, a, entry(1, "noop", 0, 0, "", nil)))
	b, e = a.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []Proof{{LastApplied: 1, ScheduledApplyIndex: 1, CommitIndex: 0, ApplyEpoch: 1}, {LastApplied: 1, ScheduledApplyIndex: 2, CommitIndex: 2, ApplyEpoch: 1}, {LastApplied: 1, ScheduledApplyIndex: 1, CommitIndex: 1, ApplyEpoch: 1}, {LastApplied: 1, ScheduledApplyIndex: 1, CommitIndex: 1, ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return 0, ErrBoundary }}} {
		if _, e = Capture(deadline(t), b, p); !errors.Is(e, ErrBoundary) {
			t.Fatal("bad live boundary", e)
		}
	}
	if e = b.Release(deadline(t)); e != nil {
		t.Fatal(e)
	}
	raw, e := storageformat.EncodeSnapshot(capture(t, a, 1, 1, 1))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Restore(c, raw, 0); !errors.Is(e, ErrBoundary) {
		t.Fatal(e)
	}
	var o map[string]any
	input, _ := os.ReadFile("../../examples/m11/n1.json")
	_ = json.Unmarshal(input, &o)
	o["cluster_id"] = strings.Repeat("3", 32)
	input, _ = json.Marshal(o)
	other, e := config.Parse(input)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = Restore(other, raw, 1); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	delete(o, "application_version")
	o["config_version"] = 1
	input, _ = json.Marshal(o)
	legacy, e := config.Parse(input)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = NewMachine(legacy); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, _, e = Restore(legacy, raw, 1); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, _, e = Restore(c, []byte("broken"), 1); e == nil {
		t.Fatal("corrupt restore")
	}
	if e = a.Shutdown(deadline(t)); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Park(deadline(t)); !errors.Is(e, applier.ErrClosed) {
		t.Fatal(e)
	}
	isolated, p, e := Restore(c, raw, 1)
	if e != nil {
		t.Fatal(e)
	}
	wrong := worker(t, isolated, 0)
	lease, e := wrong.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Capture(deadline(t), lease, proof(0, 0, 0)); !errors.Is(e, ErrBoundary) {
		t.Fatal("incorrect InitialIndex accepted", e)
	}
	_ = lease.Release(deadline(t))
	_ = p
}

func TestCaptureCancellationAndProjectedIndexMismatch(t *testing.T) {
	m, _ := NewMachine(cfg(t))
	a := worker(t, m, 0)
	b, e := a.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(deadline(t))
	cancel()
	if _, e = Capture(ctx, b, proof(0, 0, 0)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = b.Release(deadline(t)); e != nil {
		t.Fatal(e)
	}
	// A projected mutation claiming index 2 cannot be applied at actual slot 1.
	projected, e := committed.ProjectWithSessions(entry(2, "put", 1, 1, "wrong", nil))
	if e != nil {
		t.Fatal(e)
	}
	f, e := a.Submit(deadline(t), applier.Entry{Index: 1, Command: projected})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Wait(deadline(t)); !errors.Is(e, applier.ErrApplyFailed) {
		t.Fatal("mismatched command index", e)
	}
	if _, e = a.Park(deadline(t)); !errors.Is(e, applier.ErrClosed) {
		t.Fatal(e)
	}
}

func TestRestoreApprovedV2ByteVector(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/snapshot-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	var vectors map[string]json.RawMessage
	if e = json.Unmarshal(raw, &vectors); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"empty", "stateful"} {
		t.Run(name, func(t *testing.T) {
			var vector struct {
				Hex string `json:"image_hex"`
			}
			if e := json.Unmarshal(vectors[name], &vector); e != nil {
				t.Fatal(e)
			}
			image, e := hex.DecodeString(vector.Hex)
			if e != nil {
				t.Fatal(e)
			}
			m, point, e := Restore(cfg(t), image, 3)
			if e != nil {
				t.Fatal(e)
			}
			a := worker(t, m, point.Index)
			captured := capture(t, a, point.Index, point.Term, point.Index)
			encoded, e := storageformat.EncodeSnapshot(captured)
			if e != nil || !bytes.Equal(image, encoded) {
				t.Fatal("restored v2 fixture changed", e)
			}
			if name == "stateful" {
				command, _ := json.Marshal(map[string]any{"index": "10", "term": "3", "kind": "read", "command": map[string]any{"key": "empty"}})
				r, _ := applied(t, submit(t, a, storageformat.LogEntry{Index: 10, Term: 3, Raw: command}))
				if r.Code != statemachine.OK || !r.Exists || len(r.Value) != 0 {
					t.Fatal("empty fixture value lost", r)
				}
			}
		})
	}
}

func TestRestoredBoundaryRejectsRegressingLogTerm(t *testing.T) {
	c := cfg(t)
	image, e := storageformat.EncodeSnapshot(storageformat.Snapshot{ClusterID: c.ClusterID(), ConfigHash: c.Hash(), Index: 9, Term: 3})
	if e != nil {
		t.Fatal(e)
	}
	m, p, e := Restore(c, image, 4)
	if e != nil {
		t.Fatal(e)
	}
	a := worker(t, m, p.Index)
	b, e := a.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Capture(deadline(t), b, proof(9, 2, 9)); !errors.Is(e, ErrBoundary) {
		t.Fatal("regressed base term", e)
	}
	_ = b.Release(deadline(t))
	f := submit(t, a, entry(10, "put", 1, 1, "A", nil))
	if _, e = f.Wait(deadline(t)); !errors.Is(e, applier.ErrApplyFailed) {
		t.Fatal("regressed entry term", e)
	}
}
