package committed

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

const MaxSessions = 1024
const (
	SessionLimit    statemachine.Code      = "SESSION_LIMIT"
	RequestConflict statemachine.Code      = "REQUEST_CONFLICT"
	StaleSequence   statemachine.Code      = "STALE_SEQUENCE"
	SequenceGap     statemachine.Code      = "SEQUENCE_GAP"
	sessionCommand  statemachine.Operation = "raft/session"
	sessionResult   statemachine.Code      = "raft/session-result"
)

type session struct {
	sequence, index uint64
	digest          [32]byte
	kind            string
	result          statemachine.Result
}

// Sessions is owned exclusively by M02, together with its fresh M01 machine.
// No wall clock, external I/O, leader cache, TTL or eviction participates.
type Sessions struct {
	machine applier.Machine
	clients map[string]session
}

func NewSessions(machine applier.Machine) *Sessions {
	return &Sessions{machine: machine, clients: make(map[string]session)}
}
func ProjectWithSessions(e storageformat.LogEntry) (statemachine.Command, error) {
	if err := storageformat.ValidateEntry(e.Raw); err != nil {
		return statemachine.Command{}, err
	}
	o, _ := strictjson.Decode(e.Raw, storageformat.MaxEntry)
	if o["kind"] == "read" || o["kind"] == "noop" {
		return ProjectWithReads(e)
	}
	// The maximum legal canonical CAS plus this small private opcode is below
	// M02's unchanged 48 KiB aggregate copy bound. No wire field is added.
	return statemachine.Command{Operation: sessionCommand, Value: bytes.Clone(e.Raw)}, nil
}
func (s *Sessions) Apply(c statemachine.Command) statemachine.Result {
	if c.Operation != sessionCommand {
		if c.Operation != statemachine.Get {
			panic("session adapter received an unprojected mutation")
		}
		return s.machine.Apply(c)
	}
	raw := c.Value
	if storageformat.ValidateEntry(raw) != nil {
		panic("invalid session projection")
	}
	o, _ := strictjson.Decode(raw, storageformat.MaxEntry)
	kind := o["kind"].(string)
	m := o["command"].(map[string]any)
	index, _ := strictjson.Uint(o["index"])
	seq, _ := strictjson.Integer(m["sequence"])
	client := m["client_id"].(string)
	digest, err := storageformat.CommandDigest(raw)
	if err != nil {
		panic("invalid command digest")
	}
	cached, known := s.clients[client]
	reject := func(code statemachine.Code) statemachine.Result {
		return encodeSessionResult(kind, index, statemachine.Result{Code: code})
	}
	if !known {
		if len(s.clients) >= MaxSessions {
			return reject(SessionLimit)
		}
		if seq != 1 {
			return reject(SequenceGap)
		}
	} else {
		if seq < cached.sequence {
			return reject(StaleSequence)
		}
		if seq == cached.sequence {
			if digest != cached.digest || kind != cached.kind {
				return reject(RequestConflict)
			}
			return encodeSessionResult(kind, cached.index, cached.result)
		}
		if seq != cached.sequence+1 {
			return reject(SequenceGap)
		}
	}
	command, err := ProjectWithReads(storageformat.LogEntry{Index: index, Raw: raw})
	if err != nil {
		panic("invalid mutation projection")
	}
	result := s.machine.Apply(command)
	if result.Code != statemachine.OK && result.Code != statemachine.CompareFailed && result.Code != statemachine.ResourceLimit {
		panic("invalid session domain result")
	}
	if result.Code == statemachine.CompareFailed && kind != "cas" {
		panic("invalid compare result")
	}
	if result.Code == statemachine.ResourceLimit && kind == "delete" {
		panic("invalid delete quota result")
	}
	if result.Code == statemachine.OK && kind != "delete" && !result.Applied {
		panic("invalid successful mutation")
	}
	result.Value = nil // cached mutation results contain no old/user values
	s.clients[client] = session{seq, index, digest, kind, result}
	return encodeSessionResult(kind, index, result)
}
func encodeSessionResult(kind string, index uint64, r statemachine.Result) statemachine.Result {
	o := map[string]any{"code": r.Code, "index": fmt.Sprint(index)}
	if r.Code == statemachine.OK {
		if kind == "delete" {
			o["deleted"] = r.Deleted
		} else {
			o["applied"] = true
		}
	} else if r.Code == statemachine.CompareFailed || r.Code == statemachine.ResourceLimit {
		o["applied"] = false
	}
	raw, err := json.Marshal(o)
	if err != nil || len(raw) > 512 {
		panic("invalid compact result")
	}
	return statemachine.Result{Code: sessionResult, Value: raw}
}

// DecodeSessionResult extracts the approved compact result from a private M02
// completion. Current attempt index/term remain separate from original index.
func DecodeSessionResult(r statemachine.Result, current uint64) (statemachine.Result, uint64, error) {
	if r.Code != sessionResult {
		return r, current, nil
	}
	o, err := strictjson.Decode(r.Value, 512)
	if err != nil {
		return statemachine.Result{}, 0, err
	}
	if _, err = strictjson.Fields(o, "code index", "applied deleted"); err != nil {
		return statemachine.Result{}, 0, err
	}
	index, err := strictjson.Uint(o["index"])
	if err != nil || index == 0 || index > current {
		return statemachine.Result{}, 0, ErrKind
	}
	code, ok := o["code"].(string)
	if !ok {
		return statemachine.Result{}, 0, ErrKind
	}
	out := statemachine.Result{Code: statemachine.Code(code)}
	switch out.Code {
	case statemachine.OK:
		if len(o) != 3 {
			return out, 0, ErrKind
		}
		if v, ok := o["applied"].(bool); ok && v {
			out.Applied = true
		} else if v, ok := o["deleted"].(bool); ok {
			out.Deleted = v
		} else {
			return out, 0, ErrKind
		}
	case statemachine.CompareFailed, statemachine.ResourceLimit:
		if len(o) != 3 || o["applied"] != false {
			return out, 0, ErrKind
		}
	case SessionLimit, RequestConflict, StaleSequence, SequenceGap:
		if len(o) != 2 {
			return out, 0, ErrKind
		}
	default:
		return out, 0, ErrKind
	}
	return out, index, nil
}
