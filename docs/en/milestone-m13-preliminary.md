# M13 Phase A — Codec and in-memory capture

[Index](README.md) · [Italiano](../it/milestone-m13-preliminary.md)

Date: 22 November 2026. Git baseline: `cce883d`; no commit performed. **Phase: PRELIMINARY PHASE COMPLETED. Overall M13: PARTIALLY COMPLETED. M14: NOT STARTED.**

## Assignment and initial gates

The official [M13 — Snapshots and compaction](implementation-roadmap.md) requires atomic generations, installation, crash fixtures and T-015/T-016/T-026 within the roadmap subset. This phase implements only codec, atomic capture and restore into a fresh in-memory instance, as authorized by the [pre-M13 review](pre-m13-review.md). Approved contracts take precedence; no criterion is weakened to claim milestone completion.

C-01/OQ-05 was RESOLVED in design: isolated codec conformance is now verified. C-02/OQ-01 retains earlier primitive validation but remains CONDITIONALLY READY for new generations. C-03 remains CONDITIONALLY READY for runtime handoff and safety; isolated capture adds evidence without closing the gate.

## Components and inventory

New sources: `internal/storageformat/snapshot.go` (EncodeSnapshot, DecodeSnapshot, ValidateSnapshot); `internal/statemachine/state.go` (CopyState, FromState); `internal/committed/session_state.go` (CopySessions, SessionsFromState); `internal/applier/barrier.go` (Park, Inspect, Release); `internal/snapshot/memory.go` (NewMachine, Capture, Restore). Modified `internal/applier/applier.go` to coordinate the barrier in the existing worker. Each new source has its corresponding `_test.go` file.

Added the [v2 snapshot fixture](../fixtures/snapshot-v2.json). Updated both languages: architecture, canonical-formats, data-model, storage-and-recovery, testing-strategy, acceptance-criteria, traceability, failure-model, requirements, development-guide, implementation-roadmap, indexes and ADR-005/ADR-010; added this report and updated the root README. No node/Raft/WAL/server runtime, filesystem adapter or historical fixture changes.

## Format and compatibility

The approved container remains QSN1 version 1: 4-byte magic, uint64 big-endian payload length, canonical ASCII JSON payload, SHA-256 trailer covering header and payload. Total size is 44 bytes plus payload, at most 8 MiB. Whole-file hash differs from the trailer. Persistent profile v2 means configuration/applicationVersion/IDENTITY v2 and shared hash; it does not introduce snapshot container version 2.

Canonical fields and objects have deterministic order; uint64 indexes, terms, quotas and counts are decimal strings, while sequence is a safe integer JSON number, KV and sessions are sorted by key and client_id; values use canonical base64. Payload preserves index/term boundary, cluster/hash, logical quota, QCMDv1 fingerprint, latest sequence and original result with index, including failed CAS. An original result cannot follow the boundary.

Total length, overflow, magic, digest and counts are checked before content-dependent allocations. Incremental array parsing avoids disproportionate JSON object graphs; per-row limits derive from existing legal maxima. Unknown fields, noncanonical escapes/whitespace, duplicates, disorder, incompatible versions, invalid result unions and sums are rejected. The decoder re-encodes and compares all bytes; errors never return partial state.

Original v1 vectors remain byte-exact. The new fixture is built independently using Python's standard library and contains empty/stateful cases, payload, hexadecimal bytes, trailer digest and file hash. Synthetic command fingerprints do not claim an executed cluster history. The codec can validate historical vectors; Restore accepts only v2 configuration and exact identity/hash. No v1→v2 migration.

## Atomic capture and ownership

Park freezes new admissions at B = ScheduledApplyIndex and lets the accepted prefix finish in the single M02 worker. After the serial coordinator consumes completions, Capture requires A = W = L = B <= C: LastApplied, actual worker index, scheduled index and barrier coincide and do not exceed commitIndex. An advanced scheduled index is never captured prematurely: it is drained before copying.

Proof must come from the log-owning coordinator; TermAt reads the same synchronized view, never HTTP status. The zero boundary requires term zero; other boundaries require a consistent positive term. The machine checks actual applied count, mutation envelope index, nonregressing known term and restored boundary. Reads/noops advance the index without changing KV. Proof does not itself establish Raft quorum or commit; future integration must supply that evidence. ApplyEpoch is limited to isolated instance 1, not a runtime epoch-change protocol.

Inspect copies KV and sessions within the same owner execution, without a second writer or holding the applier metadata lock while copying. Trusted callbacks must terminate, never call Goexit and never wait on the same applier. The caller releases the barrier before serialization. One lease and one pending control bound requests; frozen admission does not mean copy readiness.

Canceling a wait does not discard accepted commands or an executing callback. Release receives worker acknowledgement; canceling its wait does not prematurely reopen admissions. Shutdown releases the worker and rejects new controls; application/inspection panic fails closed. Value, record and digest copies do not share mutable buffers: later writes and caller alterations cannot contaminate the machine or capture.

## Deduplication and isolated restore

The copy includes latest result per session, sequence, kind, digest and original index; no parallel cache, expiry or eviction. The limit remains 1024 sessions. CAS and session update are indivisible relative to capture, including negative outcomes. PUT, DELETE, GET, noop, retry, conflicts and stale sequences retain earlier semantics.

Restore validates codec, profile and term against currentTerm supplied by the caller's validated hard-state view; constructs fresh maps and session wrappers, returns Point and requires the corresponding InitialIndex in the isolated applier. It never reapplies 0..S, swaps live maps, opens storage or selects bootstrap paths. Suffix S+1 resumes in order and retries preserve original results and indexes, including failed CAS even if current state would now allow success.

## Added tests and exercised errors

17 top-level test functions across five files: four codec, four barrier, one KV state, one sessions, seven capture/restore. Byte-exact vectors, round-trip, insertion-independent order, corrupted/truncated input, wrong digests/lengths/counts/versions, duplicates and invalid unions. Also verified uint64 boundaries, 10000 keys, 256-byte keys, 16 KiB values, 4 MiB logical data and 1024 sessions; the combined maximum case used is checked against the conservative bound of 6782978 bytes, below 8 MiB.

Concurrency uses explicit channels and gates: CAS blocked after KV mutation before session registration, an already scheduled GET, drain and exact-prefix comparison. Cancellation, shutdown, panic, invalid callbacks, inconsistent metadata, term regression, wrong envelope index and defensive copies are checked. Restore is compared with the original machine continued through the same suffix, including retries, stale/gap/conflicting sequences, new sessions and full quota. The v2 fixture is restored and recaptured byte for byte.

Extending the M12 simulator is unnecessary for these in-memory faults; its regressions remain executed. No publication test becomes an active writer.

## Checks actually executed

Go 1.27.4 darwin/arm64, standard library only, GOPROXY/GOSUMDB disabled. Full M01–M12 suite and new tests with coverage, 75 s per-package timeout, GOMAXPROCS=2 and package parallelism 1. Targeted race on applier, snapshot, storageformat, statemachine and committed with 40 s timeout. Gofmt, go vet, macOS ARM64 build, original fixture verification, dependency and documentation/scope checks completed. Reproducible commands are in the [development guide](development-guide.md).

Statement coverage measured across the full suite, merging package profiles by block: applier 95.1%, snapshot 93.5%, storageformat 91.5%, statemachine 100%, committed 85.1%. The temporary profile is `/tmp/quorumkv-m13a.cover`; blocks imported by several packages are counted once. Coverage and race checks are not a formal correctness proof.

## Official criteria still open

| Criterion | Phase A evidence | Missing evidence |
|---|---|---|
| T-015 / AC-015 / FR-015 | Valid codec, coherent copy, isolated restore and dedup. | Installation to a far-behind follower, chunk retries, term changes, final cluster hash/sessions/indexes. |
| T-016 / AC-016 / FR-016 | Image corruption and isolated rejection. | Stops at every publication/cleanup cutpoint, authoritative CURRENT and at least one recoverable generation. |
| T-026 / AC-026 / NFR-006 | Logical/session quotas, codec and copy-request bounds. | Queues and simulated disk on the new path, directory peaks/reserves; RSS/CPU and resource report scheduled by the roadmap. |

All three remain open: passing a subset does not pass the full criterion. C-02 new generations and C-03 runtime handoff remain CONDITIONALLY READY.

## Issues, costs and limitations

Development corrected a fixture harness that tried to interpret root metadata as snapshot records and a context cancellation flagged by vet. Final checks include those corrections. No newly found defect in earlier milestones requires repair: M02 receives only the isolated barrier capability; ordinary ordering and semantics remain regression-tested.

Additional budget €0; offline execution, no dependencies, cloud, GPU, Docker or VM. Small datasets, one in-memory case at approved limits, few processes and temporary clusters of at most three nodes, timeouts and cleanup. No bulk snapshot writes, prolonged stress or physical crashes. RSS/CPU are unmeasured and power/SSD failures or snapshot crash consistency are not certified. Digest and isolated restore do not establish durable publication or safe compaction.

## Decision and next phase

**PRELIMINARY PHASE COMPLETED** within authorized scope. **M13: PARTIALLY COMPLETED**; C-01/OQ-05 RESOLVED in design with codec conformance verified; C-02/OQ-01 new generations and C-03 CONDITIONALLY READY; T-015/T-016/T-026 open; M14 NOT STARTED.

A new assignment may authorize a writer and generation protocol in temporary directories, cutpoints/errors/barriers/quotas and contract-compliant recovery selection checks. Runtime integration and activation, epoch handoff, CURRENT publication, compaction and InstallSnapshot remain subject to authorization and their respective evidence. No trigger, snapshot RPC, WAL deletion or snapshot bootstrap recovery is activated in this phase. No Git commit.
