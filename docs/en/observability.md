# Logs, metrics and diagnostics

[Index](README.md) · [Italiano](../it/observability.md)

**22 November 2026 — M12 profile implemented and checked; historical design sections are qualified by the M12 contract below.**

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

## Implemented M12 diagnostics and actual limits

[M12](milestone-m12.md) exposes GET /v1/metrics on the existing loopback client listener, Cache-Control: no-store, JSON metrics_version=1/local_only=true. No quorum required; M10 status unchanged. [M12 OpenAPI](../openapi/m12.json) adds only this operation, retaining all M11 operations identically. V1/v2 configuration/identity, PUT/DELETE/GET/CAS schemas and v2 headers remain unchanged; no automatic migration.

Synchronized bounded per-instance recorder: fixed-name aggregate counters for sent/failed RPCs, elections, leadership acquisitions, completed/unknown/rejected responses, dedup_hits, apply, recovery and storage errors. No arbitrary label, key or client_id. Local gauges: role/term/leader, commit/apply/log/snapshot (snapshot zero), match_index, active proposals, apply_lag, Go goroutines/heap and WAL bytes observed by the synchronized driver. Heap is not RSS. Counter/duration snapshots and gauges may describe nearby different instants; they are not a linearizable read.

Independent windows of at most 2048 recent microsecond samples: request, RPC, Append+Sync call, domain apply (excluding noop), local election from candidacy to first current-term commit, and committed-prefix reconstruction inside the service. Count is lifetime total; samples_us is only the recent window, without percentiles or benchmarks. Recovery does not measure full process startup/WAL-open; election does not measure time from external leader loss. No RSS/CPU/directory_bytes measurements or premature snapshot metrics; RPC breakdowns by peer/type and advanced temporal aggregation remain extensions, not promises of this version.

Structured JSON logs contain UTC timestamp, monotonic elapsed_us, level, event, node/cluster, term/role/leader, indices, server request_id and code/outcome/duration where applicable. Never keys, values, CAS payloads, full client_id or raw paths/errors. Role, recovery, startup/shutdown, apply and HTTP-result events; no INFO line for each healthy heartbeat. Ring of 256 records, <=1 KiB each; overwritten records count in dropped_logs. The instance's single fatal storage failure has a reserved copy even after overwrite. The core performs no log I/O and never waits for external consumers. Process coordination drains every 2 s and on shutdown: default stderr, or -log-dir in a separate private folder not nested with the data directory. Rotation uses four fixed files of at most 256 KiB, total 1 MiB; no per-line fsync. Fatal failure also attempts stderr. Diagnostic output errors are reported without changing consensus; arbitrary filesystem/stderr stalls have no guaranteed bound, like other local I/O.

Poll manually or every 2 s for at most 30 s in ordinary sessions; no dashboard/cloud/permanent process. Sampleable operational logs and complete invariant traces have separate authority: dropped_logs does not fabricate observations, while incomplete simulation traces stop the test as inconclusive. The [metrics fixture](../fixtures/metrics-m12.json), real HTTP election/timeout/no-quorum checks, race, redaction and rotation tests validate this profile.
