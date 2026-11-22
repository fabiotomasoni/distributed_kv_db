package raft

import (
	"github.com/fabiotomasoni/distributed_kv_db/internal/diagnostics"
	"runtime"
	"time"
)

func (x *Service) Diagnostics() *diagnostics.Recorder { return x.diagnostic }

// publish is called only by the serial owner (or before/after its lifetime).
func (x *Service) publish() {
	s := x.core.Status()
	old := x.observed
	if old.Role != s.Role || old.Term != s.Term || old.Leader != s.Leader {
		x.diagnostic.Log(diagnostics.Record{Event: diagnostics.Transition, Term: s.Term, Role: string(s.Role), LeaderID: s.Leader, Commit: s.CommitIndex, Applied: s.LastApplied})
		if s.Role == Candidate && s.Term != old.Term {
			x.diagnostic.Inc("election_started")
			x.electionStart = time.Now()
		}
		if s.Role == Leader && old.Role != Leader {
			x.diagnostic.Inc("leader_changes")
		}
	}
	if !x.electionStart.IsZero() && s.Role == Leader && s.CommitIndex > 0 && x.core.entry(s.CommitIndex).Term == s.Term {
		x.diagnostic.Observe("election", time.Since(x.electionStart).Microseconds())
		x.electionStart = time.Time{}
	}
	x.observed = s
	x.published.Store(s)
}

// Metrics reads detached observations; it is never consulted by the core.
func (x *Service) Metrics() map[string]any {
	s := x.Status()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return map[string]any{"metrics_version": 1, "node_id": x.cfg.NodeID(), "cluster_id": x.cfg.ClusterID(), "local_only": true, "role": s.Role, "term": s.Term, "leader_id": s.Leader, "commit_index": s.CommitIndex, "applied_index": s.LastApplied, "last_log_index": s.LastIndex, "snapshot_index": 0, "storage_error": x.storageFailed.Load(), "node_error": s.Failure != nil, "pending_proposals": len(x.writeSlots), "apply_lag": s.CommitIndex - s.LastApplied, "match_index": s.Progress, "goroutines": runtime.NumGoroutine(), "heap_bytes": m.HeapAlloc, "wal_bytes": x.walBytes.Load(), "observations": x.diagnostic.Snapshot()}
}
