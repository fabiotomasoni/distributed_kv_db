package faultsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func scenario(t *testing.T) Scenario {
	t.Helper()
	b, e := os.ReadFile("../../examples/m12/session-retry.json")
	if e != nil {
		t.Fatal(e)
	}
	s, e := Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(c)
	return ctx
}
func runOK(t *testing.T, s Scenario) Manifest {
	t.Helper()
	m, e := Run(bounded(t), s)
	if e != nil {
		t.Fatal(e, m.Failure)
	}
	if m.Outcome != "passed" {
		t.Fatal(m.Outcome)
	}
	return m
}
func leaderBeforeFault(t *testing.T, s Scenario) string {
	t.Helper()
	s.Deadline = 1100
	s.Events = s.Events[:2]
	m := runOK(t, s)
	if len(m.History) != 1 || m.History[0].Node == "" {
		t.Fatal("election precondition", m.History)
	}
	return m.History[0].Node
}
func checkReplay(t *testing.T, m Manifest) {
	t.Helper()
	raw, e := m.Bytes()
	if e != nil || len(raw) > MaxEvidence {
		t.Fatal(e)
	}
	saved, e := ReadManifest(raw)
	if e != nil {
		t.Fatal(e)
	}
	again, e := Replay(bounded(t), saved)
	if e != nil {
		t.Fatal(e)
	}
	b, e := again.Bytes()
	if e != nil || !bytes.Equal(raw, b) {
		t.Fatal("manifest/trace changed", e)
	}
	// Re-seal a syntactically valid counterexample: replay must stop at its first
	// altered state digest, without reaching subsequent events.
	saved.Trace[5].StateSHA = "altered"
	saved.SHA = ""
	saved.SHA = digest(saved)
	bad, e := Replay(bounded(t), saved)
	if !errors.Is(e, ErrDivergence) || len(bad.Trace) != 5 {
		t.Fatal("first divergence not detected", len(bad.Trace), e)
	}
	raw[len(raw)/2] ^= 1
	if _, e = ReadManifest(raw); e == nil {
		t.Fatal("tampered evidence accepted")
	}
}
func TestT020T028SavedReplayAcrossLeaderCrash(t *testing.T) {
	for _, seed := range []uint64{7, 42, 117} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			s := scenario(t)
			s.Seed = seed
			old := leaderBeforeFault(t, s)
			s.Events[2].Node = old
			s.Events[4].Node = old
			m := runOK(t, s)
			if len(m.History) != 3 || m.History[0].Outcome != "unknown" || m.History[1].Outcome != "completed" || m.History[1].Code != "OK" || m.History[1].Node == old || m.History[1].OriginalIndex != m.History[0].Index || !m.History[1].Applied || string(m.History[2].Value) != "A" {
				t.Fatal(m.History)
			}
			checkReplay(t, m)
			fresh := runOK(t, s)
			a, _ := m.Bytes()
			b, _ := fresh.Bytes()
			if !bytes.Equal(a, b) {
				t.Fatal("same seed changed trace")
			}
		})
	}
}
func TestT020PartitionDelayDuplicateHealAndOldLeaderUnknown(t *testing.T) {
	s := scenario(t)
	old := leaderBeforeFault(t, s)
	retry := s.Events[3]
	retry.Node = old
	retry.At = 1150
	retry.Timeout = 200
	s.Events = s.Events[:2]
	for _, other := range []string{"n1", "n2", "n3"} {
		if other != old {
			s.Events = append(s.Events, Action{At: 1100, Action: "partition", From: old, To: other, Both: true})
		}
	}
	s.Events = append(s.Events, retry)
	for _, other := range []string{"n1", "n2", "n3"} {
		if other != old {
			s.Events = append(s.Events, Action{At: 2400, Action: "heal", From: old, To: other, Both: true})
		}
	}
	retry.Node = "leader"
	retry.At = 2800
	s.Events = append(s.Events, retry)
	m := runOK(t, s)
	if len(m.History) != 3 || m.History[1].Outcome != "unknown" || m.History[2].Outcome != "completed" || m.History[2].OriginalIndex != m.History[0].Index {
		t.Fatal(m.History)
	}
	counts := map[string]int{}
	for _, e := range m.Trace {
		counts[e.Event]++
	}
	if counts["duplicate"] == 0 || counts["drop"] == 0 || counts["send"] == 0 {
		t.Fatal(counts)
	}
	checkReplay(t, m)
}
func TestT028StorageCutsAndExplicitCrashModels(t *testing.T) {
	for _, fault := range []string{"short_write", "enospc", "sync_fail"} {
		for _, model := range []string{"ProcessCrash", "SystemCrashModel"} {
			t.Run(fault+model, func(t *testing.T) {
				s := scenario(t)
				old := leaderBeforeFault(t, s)
				s.Model = model
				cut := "wal.sync"
				if fault == "short_write" {
					cut = "wal.payload"
				}
				if fault == "enospc" {
					cut = "wal.header"
				}
				arm := Action{At: 800, Action: "storage", Node: old, Fault: fault, Cutpoint: cut}
				s.Events = append(s.Events[:1], append([]Action{arm}, s.Events[1:]...)...)
				s.Events[3].Node = old
				s.Events[5].Node = old
				if model == "SystemCrashModel" {
					s.Events[3].Survival = "synced"
				}
				m := runOK(t, s)
				faultSeen := false
				for _, e := range m.Trace {
					if e.Event == "io_fault" {
						faultSeen = true
					}
				}
				if !faultSeen {
					t.Fatal("fault was not exercised")
				}
				if m.History[0].Outcome == "completed" {
					t.Fatal("premature success")
				}
				if m.History[1].Outcome != "completed" || m.History[1].Code != "OK" {
					t.Fatal(m.History)
				}
				checkReplay(t, m)
			})
		}
	}
}
func TestScenarioValidationBudgetsCancellationAndReduction(t *testing.T) {
	s := scenario(t)
	raw, _ := json.Marshal(s)
	for _, bad := range [][]byte{[]byte(`{"scenario_version":1,"scenario_version":1}`), append(raw, []byte(`{}`)...), []byte(`{"extra":1}`), []byte(`{"scenario_version":1.0}`), bytes.Repeat([]byte("x"), MaxEvidence+1)} {
		if _, e := Parse(bad); e == nil {
			t.Fatal("invalid scenario")
		}
	}
	for _, mutate := range []func(*Scenario){func(s *Scenario) { s.Deadline = 5001 }, func(s *Scenario) { s.MaxEvents = 10001 }, func(s *Scenario) { s.Seed = 0 }, func(s *Scenario) { s.Events[0].Action = "install_snapshot" }} {
		b, _ := json.Marshal(s)
		copy, _ := Parse(b)
		mutate(&copy)
		if _, e := Run(bounded(t), copy); !errors.Is(e, ErrInput) {
			t.Fatal(e)
		}
	}
	s.MaxEvents = 5
	m, e := Run(bounded(t), s)
	if !errors.Is(e, ErrLimit) || m.Outcome != "inconclusive" {
		t.Fatal(m.Outcome, e)
	}
	ctx, c := context.WithCancel(context.Background())
	c()
	if _, e = Run(ctx, scenario(t)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	s = scenario(t)
	reduced := Minimize(s, func(candidate Scenario) bool {
		for _, a := range candidate.Events {
			if a.Action == "propose" {
				return true
			}
		}
		return false
	})
	if len(reduced.Events) != 1 || reduced.Events[0].Action != "propose" {
		t.Fatal(reduced.Events)
	}
}

func TestWALHeaderAndCommitSyncNamedCutpoints(t *testing.T) {
	for _, cut := range []string{"wal.header", "commit.sync"} {
		t.Run(cut, func(t *testing.T) {
			s := scenario(t)
			old := leaderBeforeFault(t, s)
			fault := "short_write"
			if cut == "commit.sync" {
				fault = "sync_fail"
			}
			s.Events = append(s.Events[:1], append([]Action{{At: 800, Action: "storage", Node: old, Fault: fault, Cutpoint: cut}}, s.Events[1:]...)...)
			s.Events[3].Node = old
			s.Events[5].Node = old
			m := runOK(t, s)
			found := false
			for _, v := range m.Trace {
				if v.Event == "io_fault" && v.Detail == fault+":"+cut {
					found = true
				}
			}
			if !found || m.History[0].Outcome == "completed" || m.History[1].Outcome != "completed" {
				t.Fatal(m.History, found)
			}
		})
	}
}
func TestSavedManifestFixtureReplay(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/m12-session-retry.json")
	if e != nil {
		t.Fatal(e)
	}
	m, e := ReadManifest(raw)
	if e != nil {
		t.Fatal(e)
	}
	again, e := Replay(bounded(t), m)
	if e != nil {
		t.Fatal(e)
	}
	a, _ := m.Bytes()
	b, _ := again.Bytes()
	if !bytes.Equal(a, b) {
		t.Fatal("saved fixture divergence")
	}
}
