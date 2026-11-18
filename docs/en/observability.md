# Logs, metrics and diagnostics

[Index](README.md) · [Italiano](../it/observability.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Structured logs

Proposal: `log/slog`, line-oriented JSON. Stable fields: UTC timestamp for humans, monotonic elapsed time for measurements, level, event, node_id, cluster_id, term, role, leader_id, rpc_id, request_id, abbreviated client_id, sequence, log_index, commit_index, applied_index, duration_us, error_code, scenario_id. Never include values, CAS payloads, personal paths or arbitrary keys. Never use client_id as a metric label.

INFO: startup, shutdown, term/role change, election won, published snapshot and completed recovery. WARN: aggregated timeouts, quota rejection and excessive retries. ERROR: fatal storage, corruption and invariant violations. DEBUG: sampled replication progress; normally disabled. Successful heartbeats do not each produce an INFO line.

Correlation: request_id changes per HTTP attempt; `(client_id,sequence)` identifies the mutation; rpc_id identifies a peer attempt. Apply logs retain index and code without values. A retry may have a new request_id and original index: do not infer mutation count by counting HTTP responses.

## Local metrics

GET `/v1/status` is a potentially stale local copy, HTTP 200 even without quorum: `node_id`, `role`, `term`, `leader_id`, `commit_index`, `applied_index`, `last_log_index`, `snapshot_index`, `ready`, `storage_error`. `ready` means recovery and listeners are ready, not that a majority has been checked for a new operation.

Future GET `/v1/metrics` returns local JSON: RPC sent/failed counters by type and peer (fixed domain), election_started, leader_changes, client_completed/unknown/rejected by code, dedup_hits, snapshot_count; gauges pending_proposals, match_index per peer, apply_lag, goroutines, heap_bytes, wal_bytes, snapshot_bytes, directory_bytes; in-memory distributions of request/RPC/Sync/apply/election/recovery duration. Samples are capped at 2,048 observations per distribution; reports state the window and sample count. Metrics are not a source of consensus state.

RSS and CPU require process observation through macOS; Go heap is not RSS. Election duration: from coordinator-injected loss to the new leader's first committed noop. Recovery: from process start to completed replay, separate from catch-up and first successful operation.

## Overhead and retention

Poll manually or every 2 s, at most 30 s per ordinary session. No mandatory Prometheus/Grafana/cloud. Log buffer 256 records, maximum 1 KiB/record; if saturated, drop sampleable DEBUG/INFO and increment dropped_logs, never drop fatal errors without attempting stderr. Aggregate repeated errors once per second. Log writes do not affect quorum.

Proposed rotation 256 KiB, four total files = 1 MiB/node, including current and rotated files; orderly close, no fsync for every debug line. Test evidence is separate under a 10 MiB quota. Invariant-checking test traces cannot be sampled: running out of space or buffers makes the test inconclusive and stops it.

## Symptom diagnosis

No leader: check term/vote, peer addresses, config_hash and timers. Stalled commit: inspect matchIndex, RPC errors and Sync; measure before increasing timeouts. Apply lag: inspect applier and queues. Disk growth: inspect compactable prefix and leftover staging. High RAM: inspect heap/goroutines and queues; distinguish OS cache from process memory. The [guide](development-guide.md) and [resources](performance-and-resources.md) give the local procedure.
