// Package faultsim is an explicit offline test profile. It drives the unchanged
// Raft core, real M02 and v2 session machine with bounded virtual events. No
// normal node or HTTP handler enables faults.
package faultsim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

const MaxEvidence = 10 * 1024 * 1024
const Version = "quorumkv-m12-v1"
const PRNG = "xorshift64-v1"

var ErrInput = errors.New("faultsim: invalid bounded scenario or manifest")
var ErrLimit = errors.New("faultsim: resource budget exhausted; inconclusive")
var ErrDivergence = errors.New("faultsim: replay diverged")

type Action struct {
	At             uint64          `json:"at_ms"`
	Action         string          `json:"action"`
	Node           string          `json:"node,omitempty"`
	From           string          `json:"from,omitempty"`
	To             string          `json:"to,omitempty"`
	Both           bool            `json:"both,omitempty"`
	Kind           string          `json:"kind,omitempty"`
	Command        json.RawMessage `json:"command,omitempty"`
	Timeout        uint64          `json:"timeout_ms,omitempty"`
	LoseResponse   bool            `json:"lose_response,omitempty"`
	Delay          uint64          `json:"delay_ms,omitempty"`
	DropEvery      uint64          `json:"drop_every,omitempty"`
	DuplicateEvery uint64          `json:"duplicate_every,omitempty"`
	Fault          string          `json:"fault,omitempty"`
	Cutpoint       string          `json:"cutpoint,omitempty"`
	Survival       string          `json:"survival,omitempty"`
}
type Scenario struct {
	Version   int                `json:"scenario_version"`
	ID        string             `json:"scenario_id"`
	Seed      uint64             `json:"seed"`
	Deadline  uint64             `json:"virtual_deadline_ms"`
	MaxEvents int                `json:"max_events"`
	Model     string             `json:"model_kind"`
	Configs   [3]json.RawMessage `json:"configurations"`
	Events    []Action           `json:"events"`
}
type Draw struct{ Min, Max, Value uint64 }
type Trace struct {
	At, Sequence                  uint64
	Event, Node, Detail, StateSHA string
}
type Attempt struct {
	ID                         int             `json:"attempt_id"`
	Node                       string          `json:"node_id"`
	Kind                       string          `json:"operation"`
	Command                    json.RawMessage `json:"synthetic_command"`
	Invoke, Return             uint64
	Deadline                   uint64
	Index, Term, OriginalIndex uint64
	Outcome, Code              string
	Applied, Deleted, Exists   bool
	Value                      []byte `json:"synthetic_result,omitempty"`
	LoseResponse               bool   `json:"lose_response"`
	Incarnation                int
}
type Manifest struct {
	CoreVersion    string    `json:"core_version"`
	SourceRevision string    `json:"source_revision"`
	SourceModified string    `json:"source_modified"`
	Failure        string    `json:"failure,omitempty"`
	Version        int       `json:"manifest_version"`
	Harness        string    `json:"harness_version"`
	Runtime        string    `json:"go_version"`
	Platform       string    `json:"platform"`
	PRNG           string    `json:"prng"`
	ConfigHash     string    `json:"config_hash"`
	Scenario       Scenario  `json:"scenario"`
	Draws          []Draw    `json:"draws"`
	Trace          []Trace   `json:"trace"`
	History        []Attempt `json:"history"`
	Outcome        string    `json:"outcome"`
	SHA            string    `json:"sha256"`
}

func decode(raw []byte, v any) error {
	if len(raw) > MaxEvidence {
		return ErrLimit
	}
	if _, e := strictjson.Decode(raw, MaxEvidence); e != nil {
		return ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return ErrInput
	}
	return nil
}
func Parse(raw []byte) (Scenario, error) {
	var s Scenario
	if e := decode(raw, &s); e != nil {
		return s, e
	}
	return s, validate(s)
}
func member(s string) bool { return s == "n1" || s == "n2" || s == "n3" }
func validate(s Scenario) error {
	if s.Version != 1 || s.Seed == 0 || s.Deadline == 0 || s.Deadline > 5000 || s.MaxEvents < 1 || s.MaxEvents > 10000 || len(s.Events) > 128 || len(s.ID) < 1 || len(s.ID) > 64 || (s.Model != "ProcessCrash" && s.Model != "SystemCrashModel") {
		return ErrInput
	}
	for _, c := range s.ID {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return ErrInput
		}
	}
	hash := ""
	for i, raw := range s.Configs {
		c, e := config.Parse(raw)
		if e != nil || !c.Sessions() || c.NodeID() != fmt.Sprint("n", i+1) {
			return ErrInput
		}
		if i > 0 && c.Hash() != hash {
			return ErrInput
		}
		hash = c.Hash()
	}
	var previous uint64
	for i, a := range s.Events {
		if a.At > s.Deadline || i > 0 && a.At < previous {
			return ErrInput
		}
		previous = a.At
		switch a.Action {
		case "partition", "heal":
			if !member(a.From) || !member(a.To) || a.From == a.To {
				return ErrInput
			}
		case "network":
			if a.Delay > 200 || a.DropEvery > 32 || a.DuplicateEvery > 32 {
				return ErrInput
			}
		case "crash", "restart":
			if !member(a.Node) {
				return ErrInput
			}
			if a.Action == "crash" && s.Model == "SystemCrashModel" && a.Survival != "all" && a.Survival != "synced" {
				return ErrInput
			}
			if s.Model == "ProcessCrash" && a.Survival != "" && a.Survival != "all" {
				return ErrInput
			}
		case "storage":
			if !member(a.Node) || (a.Fault != "short_write" && a.Fault != "enospc" && a.Fault != "sync_fail") || (a.Cutpoint != "wal.header" && a.Cutpoint != "wal.payload" && a.Cutpoint != "wal.sync" && a.Cutpoint != "commit.sync") {
				return ErrInput
			}
			if (a.Fault == "short_write" && a.Cutpoint != "wal.header" && a.Cutpoint != "wal.payload") || (a.Fault == "sync_fail" && a.Cutpoint != "wal.sync" && a.Cutpoint != "commit.sync") || (a.Fault == "enospc" && a.Cutpoint != "wal.header") {
				return ErrInput
			}
		case "propose":
			if !member(a.Node) && a.Node != "leader" {
				return ErrInput
			}
			if a.Timeout < 50 || a.Timeout > 5000 || len(a.Command) > 48*1024 {
				return ErrInput
			}
			command, e := strictjson.Decode(a.Command, 48*1024)
			if e != nil {
				return ErrInput
			}
			raw, _ := json.Marshal(map[string]any{"index": "1", "term": "1", "kind": a.Kind, "command": command})
			if (a.Kind != "put" && a.Kind != "delete" && a.Kind != "cas" && a.Kind != "read") || storageformat.ValidateEntry(raw) != nil {
				return ErrInput
			}
		default:
			return ErrInput
		}
	}
	return nil
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (m Manifest) Bytes() ([]byte, error) {
	m.SHA = ""
	m.SHA = digest(m)
	b, e := json.Marshal(m)
	if len(b) > MaxEvidence {
		return nil, ErrLimit
	}
	return b, e
}
func ReadManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if e := decode(raw, &m); e != nil {
		return m, e
	}
	want := m.SHA
	m.SHA = ""
	if want != digest(m) || m.Version != 1 || m.Harness != Version || m.CoreVersion != "raft-m11" || m.PRNG != PRNG || m.Runtime != runtime.Version() || m.Platform != runtime.GOOS+"/"+runtime.GOARCH || len(m.Trace) > 10000 || len(m.Draws) > 20000 || len(m.History) > 128 || validate(m.Scenario) != nil {
		return m, ErrInput
	}
	m.SHA = want
	return m, nil
}

func sourceVersion() (string, string) {
	revision, modified := "unversioned-worktree", "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
			if s.Key == "vcs.modified" {
				modified = s.Value
			}
		}
	}
	return revision, modified
}
