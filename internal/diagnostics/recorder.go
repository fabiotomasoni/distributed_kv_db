// Package diagnostics retains bounded, redacted local observations. It never
// participates in consensus and accepts no user keys, values or raw errors.
package diagnostics

import (
	"encoding/json"
	"sync"
	"time"
)

const LogCapacity = 256
const SampleCapacity = 2048

type Event string

const (
	Startup      Event = "startup"
	Recovery     Event = "recovery_completed"
	Transition   Event = "raft_transition"
	Applied      Event = "applied"
	Request      Event = "client_result"
	StorageError Event = "storage_failure"
	Shutdown     Event = "shutdown"
)

type Record struct {
	Level      string `json:"level"`
	ID         uint64 `json:"event_id"`
	Timestamp  string `json:"timestamp"`
	ElapsedUS  int64  `json:"elapsed_us"`
	Event      Event  `json:"event"`
	NodeID     string `json:"node_id"`
	ClusterID  string `json:"cluster_id"`
	Term       uint64 `json:"term"`
	Role       string `json:"role"`
	LeaderID   string `json:"leader_id,omitempty"`
	Index      uint64 `json:"log_index,omitempty"`
	Commit     uint64 `json:"commit_index"`
	Applied    uint64 `json:"applied_index"`
	RequestID  string `json:"request_id,omitempty"`
	Code       string `json:"error_code,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	DurationUS int64  `json:"duration_us,omitempty"`
}
type Distribution struct {
	Count   uint64  `json:"count"`
	Samples []int64 `json:"samples_us"`
}
type Snapshot struct {
	Counters    map[string]uint64       `json:"counters"`
	Durations   map[string]Distribution `json:"durations"`
	DroppedLogs uint64                  `json:"dropped_logs"`
}
type window struct {
	count  uint64
	values [SampleCapacity]int64
}
type Recorder struct {
	mu                     sync.Mutex
	start                  time.Time
	node, cluster          string
	counters               map[string]uint64
	durations              map[string]*window
	logs                   [LogCapacity]Record
	next, drained, dropped uint64
	fatal                  *Record
}

func New(node, cluster string) *Recorder {
	return &Recorder{start: time.Now(), node: node, cluster: cluster, counters: make(map[string]uint64), durations: make(map[string]*window)}
}

// Names have a fixed domain; caller-controlled labels cannot grow maps.
func counter(name string) bool {
	switch name {
	case "election_started", "leader_changes", "rpc_sent", "rpc_failed", "client_completed", "client_unknown", "client_rejected", "dedup_hits", "storage_errors", "applied", "recovered":
		return true
	}
	return false
}
func (r *Recorder) Inc(name string) {
	if r == nil || !counter(name) {
		return
	}
	r.mu.Lock()
	r.counters[name]++
	r.mu.Unlock()
}
func (r *Recorder) Observe(name string, us int64) {
	if r == nil || us < 0 {
		return
	}
	switch name {
	case "request", "rpc", "sync", "apply", "election", "recovery":
	default:
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.durations[name]
	if w == nil {
		w = &window{}
		r.durations[name] = w
	}
	w.values[w.count%SampleCapacity] = us
	w.count++
}

// Log takes typed metadata only. All strings except request IDs are allowlisted.
func (r *Recorder) Log(v Record) {
	if r == nil {
		return
	}
	switch v.Event {
	case Startup, Recovery, Transition, Applied, Request, StorageError, Shutdown:
	default:
		return
	}
	v.Level = "INFO"
	if v.Event == Applied || v.Event == Request {
		v.Level = "DEBUG"
	}
	if v.Outcome == "unknown" {
		v.Level = "WARN"
	}
	if v.Event == StorageError {
		v.Level = "ERROR"
	}
	switch v.Role {
	case "", "follower", "candidate", "leader":
	default:
		v.Role = ""
	}
	if v.LeaderID != "n1" && v.LeaderID != "n2" && v.LeaderID != "n3" {
		v.LeaderID = ""
	}
	switch v.Outcome {
	case "", "completed", "unknown", "not_admitted":
	default:
		v.Outcome = ""
	}
	switch v.Code {
	case "", "OK", "KEY_NOT_FOUND", "COMPARE_FAILED", "RESOURCE_LIMIT", "SESSION_LIMIT", "REQUEST_CONFLICT", "STALE_SEQUENCE", "SEQUENCE_GAP", "INVALID_ARGUMENT", "TIMEOUT", "UNAVAILABLE", "STORAGE_UNAVAILABLE", "NOT_LEADER", "NO_LEADER", "BUSY", "BODY_TOO_LARGE", "OVERLOADED", "REQUEST_TOO_LARGE":
	default:
		v.Code = "UNAVAILABLE"
	}
	if len(v.RequestID) > 80 {
		v.RequestID = ""
	}
	for _, c := range v.RequestID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			v.RequestID = ""
			break
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	v.ID = r.next
	v.NodeID = r.node
	v.ClusterID = r.cluster
	v.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	v.ElapsedUS = time.Since(r.start).Microseconds()
	if r.next-r.drained > LogCapacity {
		r.dropped++
		r.drained = r.next - LogCapacity
	}
	r.logs[(r.next-1)%LogCapacity] = v
	if v.Event == StorageError {
		copy := v
		r.fatal = &copy
	}
}
func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := Snapshot{Counters: make(map[string]uint64), Durations: make(map[string]Distribution), DroppedLogs: r.dropped}
	for k, v := range r.counters {
		s.Counters[k] = v
	}
	for k, w := range r.durations {
		n := w.count
		if n > SampleCapacity {
			n = SampleCapacity
		}
		d := Distribution{Count: w.count, Samples: make([]int64, 0, n)}
		for i := w.count - n; i < w.count; i++ {
			d.Samples = append(d.Samples, w.values[i%SampleCapacity])
		}
		s.Durations[k] = d
	}
	return s
}

// Drain is called by the process coordinator, never the Raft owner. Records
// overwritten before draining are counted; fatal metadata has a reserved copy.
func (r *Recorder) Drain() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, 0, LogCapacity+1)
	if r.fatal != nil && r.fatal.ID <= r.drained {
		out = append(out, *r.fatal)
	}
	for i := r.drained; i < r.next; i++ {
		out = append(out, r.logs[i%LogCapacity])
	}
	r.drained = r.next
	r.fatal = nil
	return out
}
func Encode(v Record) []byte { b, _ := json.Marshal(v); return append(b, '\n') }
