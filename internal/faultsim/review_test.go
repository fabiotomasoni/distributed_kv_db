package faultsim

import (
	"errors"
	"os"
	"testing"
)

func TestReplayRejectsChangedFinalOutcome(t *testing.T) {
	raw, err := os.ReadFile("../../docs/fixtures/m12-session-retry.json")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ReadManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	// A valid, re-sealed manifest whose final assertion conflicts with the trace.
	// Replay must compare that assertion as well as events and client history.
	saved.Outcome = "failed"
	saved.Failure = "synthetic expected failure"
	raw, err = saved.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	saved, err = ReadManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Replay(bounded(t), saved); !errors.Is(err, ErrDivergence) {
		t.Fatalf("changed final outcome accepted: %v", err)
	}
}

func TestReplayPreservesInconclusiveAndRejectsInvalidOutcome(t *testing.T) {
	s := scenario(t)
	s.MaxEvents = 5
	original, err := Run(bounded(t), s)
	if !errors.Is(err, ErrLimit) || original.Outcome != "inconclusive" {
		t.Fatal(original.Outcome, err)
	}
	replayed, err := Replay(bounded(t), original)
	if !errors.Is(err, ErrLimit) || errors.Is(err, ErrDivergence) || replayed.Outcome != "inconclusive" {
		t.Fatal(replayed.Outcome, err)
	}
	original.Outcome = "unsupported"
	raw, err := original.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ReadManifest(raw); !errors.Is(err, ErrInput) {
		t.Fatal("invalid outcome accepted", err)
	}
}
