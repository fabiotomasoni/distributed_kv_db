# Testing strategy and catalog

[Index](README.md) · [Italiano](../it/testing-strategy.md)

**Updated: 20 November 2026 — local M01/M02/M03 implemented and checked; distributed specifications remain design work.**

## Layers and reference model

State-machine unit tests: full table of pre/postconditions and quotas. Core unit tests: fake clock, seeded PRNG, event-driven networking and storage. Integration: three real processes and isolated temporary directories. Concurrency: handlers, shutdown and apply under the race detector. Property-based tests: `testing/quick` and bounded standard fuzzing for parsers, frames and command sequences; no Raft library or external service.

Use an independent small sequential model deliberately different from the implementation structure, avoiding tests that repeat the same bug. Compare canonical state only at equal applied indices. Check invariants INV-01–06 after each simulated transition, not just at the end.

## Planned test catalog

| Test | Requirement | Procedure | Evidence |
|---|---|---|---|
| T-001 | FR-001 | Start three distinct configurations and attempt a second opening of n1. | Manifest and exit codes. |
| T-002 | FR-002 | Vary membership, cluster ID and hash in configuration and RPCs. | Valid/invalid fixtures and before/after state. |
| T-003 | FR-003 | Inject invalid JSON, incorrect envelope and oversized body. | Responses and unchanged state hash. |
| T-004 | FR-004 | Simulate split votes and crash immediately after vote Sync; replay for 20 seeds. | Vote/term/leader trace and seed. |
| T-005 | FR-005 | Advance virtual time through 100 fault-free heartbeats. | Election count and timers. |
| T-006 | FR-006 | Construct divergent logs, send heartbeats and obsolete responses, then heal. | Prefixes, matchIndex and transitions. |
| T-007 | FR-007 | Reproduce an old-term entry on a majority, then a current noop. | Unchanged commit before, advanced commit after. |
| T-008 | FR-008 | Apply the same sequence to three models, delaying one applier. | Canonical digest and per-index results. |
| T-009 | FR-009 | Enumerate absent key, empty value, invalid input and repeated DELETE. | API fixture table and model. |
| T-010 | FR-010 | Run two CAS from different sessions on the same value with different new values. | History with exactly one successful change. |
| T-011 | FR-011 | Duplicate latest sequence, vary payload, use an old sequence and a gap. | Results and session table after replay. |
| T-012 | FR-012 | Drop response after apply, change leader, retry identically. | One mutation and original result. |
| T-013 | FR-013 | Cut frames at every boundary; simulate short write/Sync errors and incorrect CRC. | Disk images and recovery decision. |
| T-014 | FR-014 | Restart all with an uncommitted suffix and a lower commit checkpoint. | Reconstructed state before and after a new noop. |
| T-015 | FR-015 | Install snapshot on a far-behind follower with chunk retries and term change. | Image hash, sessions and final indices. |
| T-016 | FR-016 | Stop at every generation publication/cleanup cutpoint. | CURRENT and recovered generation for each case. |
| T-017 | FR-017 | Run election and timeout, inspect status and logs without user values. | Redacted log sample and metric snapshot. |
| T-018 | FR-018 | Send shutdown during RPC and persistence; check workers and lock. | Duration, goroutines and subsequent recovery. |
| T-019 | FR-019 | Establish complete isolation and drop pending messages; then write on the majority and invoke a new old-leader GET. | History excluding successful stale reads. |
| T-020 | FR-020 | Replay a scenario twice using the same seed and virtual clock. | Byte-identical event traces. |
| T-021 | NFR-001 | Enumerate small schedules with 3 nodes, 3 proposals and up to 2 faults. | Invariant report and explored bound. |
| T-022 | NFR-002 | Check 20 histories of at most 12 operations with CAS, retry and failover. | Witness order or reduced counterexample. |
| T-023 | NFR-003 | Acknowledge data, crash one then all processes, recover intact directories. | Acknowledged-success list compared with later GETs. |
| T-024 | NFR-004 | Stop a follower, restart it and await catch-up; then stop the leader leaving two healthy nodes. | Election timeline and resumption within local target. |
| T-025 | NFR-005 | Run bounded benchmark under the resource profile. | Raw samples and methodology. |
| T-026 | NFR-006 | Reach dataset, queue and simulated-disk quotas without massive load. | Errors, RSS, CPU and peak space. |
| T-027 | NFR-007 | Compare language trees, IDs, technical blocks and links; semantic review. | Review report with stated limitations. |
| T-028 | NFR-008 | Inject fake clock, PRNG, transport and storage into the same core. | Regression replay without real networking. |
| T-029 | NFR-009 | Record go version/GOARCH and build on the ARM64 Mac. | Build output and local suite. |
| T-030 | NFR-010 | Try binding 0.0.0.0, oversized input and broad permissions. | Rejections and actual permissions. |
| T-031 | NFR-011 | Disable Internet while retaining loopback and run independent offline integration; present the demo at M16. | Dependency manifest and offline trace. |
| T-032 | NFR-012 | Review dependencies and procedure on already-owned hardware. | License, cost and prerequisite inventory. |

## Histories and linearizability checker

One coordinator records using a single monotonic clock: history_version, event_id, client_id, sequence, attempt_id, operation, key, synthetic input, invoke_time, return_time, code, outcome, result, server_term/index and scenario seed. Value inputs/outputs belong only in synthetic fixtures, never operational logs. History retains each attempt; the checker groups mutation retries into one logical operation, spanning first invocation to the **earliest** observed definitive response; later retries never extend that interval. Retried GET remains a separate operation.

The checker constructs real-time precedences (`return(A) < invoke(B)`), explores topological orderings and applies the model to check results. Operations returning unknown or no response may be excluded or included with a compatible effect, never automatically treated as failed. Separately verify that retries return the original result and never change state twice. Save a witness; preserve a reduced history for counterexamples. Checker timeout means **inconclusive**, not success.

Initial limits: 20 histories, at most 12 logical operations per history, two clients and two keys, checker budget 5 s/history. Never naively partition by key when quotas and sessions share global state. First validate the checker with a known legal history, an impossible history, timeout and pending operations.

## Reproducibility, limits and gates

Manifest: future Git revision, Go/macOS version, architecture, configuration and hash, PRNG and version, initial seed (1..20 for the ordinary suite), ordered events, fault plan, history and cutpoint. Virtual time and event order determine simulation. For real processes, identical seeds do not determine OS scheduling: save traces and reduce the case in simulation.

Unit-test deadline 10 s/case, integration 30 s/case, ordinary suite maximum 120 s; maximum concurrency 8 clients, 1,000 operations. Optional fuzzing 10 s per target, two workers, outside the ordinary suite. Stop on resource-limit violations; no overnight soak tests. Unique run directories; delete only successful-run artifacts, retaining failures under a 10 MiB evidence quota outside active data directories.

Local M01/M02 commands now available: `go test ./...` and `go vet ./...`; `go test -race -timeout=30s ./internal/applier` has passed for M02. The race detector finds data races in observed executions; it proves neither quorum nor linearizability nor absence of deadlock in every schedule. Tests and vet for both local packages and targeted M02 race checks ran as recorded in local evidence; distributed checks remain future work.

The M14 gate requires all mandatory tests and associated ACs, reproducible results and zero known violations. Physical power-loss tests remain excluded: process-kill results do not close that gap.

## Mandatory counterexamples and independent oracle

T-022/checker: synthetic history PUT(x,1) completes, then GET(x)=0, then an identical retry of the already-completed PUT. The checker must reject it: retry cannot move the first PUT after GET. If the first attempt only times out with no definitive response, its interval remains pending until the first completed response or history end; never assume no effect.

Group only attempts with the same identity **and the same canonical command**. REQUEST_CONFLICT, STALE_SEQUENCE and SEQUENCE_GAP are session-protocol observations, not successful duplicates; the full model includes sessions, original results, consumed sequences and quotas. Compare every definitive duplicate response against the original result without introducing effects. For completed non-mutating results (failed CAS, quota), model sequence advancement where required.

T-008/apply-backlog: commit 5 with lastApplied 0; before any completion, commit 7. The scheduled sequence must be exactly 1..7, once; also saturate the queue and verify resumption. T-015/snapshot-epoch and T-016/hard-state-handoff check the storage barriers. T-021 retains a global ghost register of entries that became committed and checks every future leader against that prefix, including after compaction; checking only equality of current logs does not establish Leader Completeness.

Each criterion retains its primary matrix test; these subcases are mandatory within that identifier. All remain **Not run**. The 20-history and 120 s bounds are maximum budgets: if the global budget expires, missing cases are inconclusive, never implicitly passed.

## Local M01 evidence

T-008/M01 and T-009/M01 ran and passed within the roadmap subset: 12 top-level tests, 44 subcases, three additional repetitions and 100% statement coverage of the local package. Independent list/string oracle rather than the production map, fixed seed 19 and per-step comparison. See [commands and limits](milestone-m01.md). At M01 delivery, distributed tests, apply backlog, linearizability checker, race detection and fault injection were Not run. M02 evidence follows.

## Local M02 evidence

T-008/M02 and T-018/M02 Passed: 20 new tests; full suite repeated three times, targeted race detector passed once with no reported race. Coverage: applier 99.3%, statemachine 100%. TestT008ApplyBacklogExactlyOnce exercises scheduled/completed cursor separation; TestT008QueueFullAndBlockingBackpressure exercises saturation and resumption. TestT018ShutdownDuringApplyAndConcurrentWaiters and TestT018ShutdownWakesBlockedProducers exercise lifecycle. TestT028FakeBoundariesAndControlledDelay supplies test-only clock/transport/storage foundations. See [all scenarios, commands and limits](milestone-m02.md). Full AC-008, AC-018 and AC-028 remain open at their final gates; no distributed or full-replay result is claimed.

## Pre-M03 review evidence

19 November 2026: [M02 re-review](pre-m03-review.md) reproduced the sole uncovered cancellation-after-copy return and added a deterministic test. 21 applier tests now pass with 100.0% statement coverage; M01 remains 12 tests/44 subcases, 100.0%. One targeted race run passed. Static format vectors pass arithmetic/checksum checks, not recovery execution. [C-02 finite plan](macos-storage-contract.md) defines future isolated platform tests; no M03 test or OS/power-loss experiment ran.

## Subsequent preliminary M03 evidence

[Phase report](milestone-m03-preliminary.md): GATE C-02 VERIFIED for local primitives, M03 PARTIALLY COMPLETED. 7 storageformat tests/18 subcases and 13 storagefs test functions/16 subcases pass; targeted race and unchanged M01/M02 regressions pass. Codec scope is stateless frames/entries/digests/manifests; no WAL/replay engine or complete AC-013/AC-014/AC-023 closure. Full snapshot and later gates remain open. Earlier figures/statuses describe their historical phase.

## Local M03 closure

[Final evidence](milestone-m03.md): M03 COMPLETED; local T-013/storage AC-013 portion Passed. WAL has 17 top-level functions/25 subcases, 82.9% local statement coverage; M01/M02, codec/adapter regressions and targeted WAL/adapter/applier race checks pass. All 18 canonical recovery expectations covered at the appropriate byte/namespace boundary. Full FR-013 Raft-response integration, AC-014 server restart and AC-023 end-to-end remain dependent on later milestones; AC-015/AC-016 snapshot acceptance is M13. Coverage/race/process tests do not certify physical-fault resilience. No primary requirement/AC ID or final distributed gate removed.

## M04 finite evidence

[M04 tests and results](milestone-m04.md) implement the local subsets of T-001/T-002/T-003/T-029: three library startups/identity/lock-before-bind, canonical configuration and protocol fixtures, strict input/size rejection before processor, HTTP round-trip, cancellation/300 ms timeout, connection cap, failure/shutdown and native ARM64 build. 12 new M04 test functions plus one WAL tail-sequence regression; race checked node/transport/wal/storagefs/applier. Channels/contexts synchronize tests; held dynamic loopback listeners and tiny private temporary directories avoid port races/personal files. CLI exit-code evidence, Raft safety and end-to-end application/durability remain later gates.

## M05 deterministic election suite

[M05](milestone-m05.md) adds 17 test functions: 16 raft, one real node integration. T-004 uses seeds 0–19, forced split, message loss/delay/duplication/reordering, crash after model Sync before callback, follower restart and second-vote denial; independent vote/quorum/leader oracles and [saved traces](../fixtures/election-v1-results.json) must reproduce twice. T-005 separately advances 100 heartbeat rounds without spurious election. Partition/heal, old higher-term replies, timer rules, token/Sync/error/overflow, bounded admission, cancellation and joined shutdown are tested. Service disk gates/channels and virtual clocks avoid sleep synchronization. Actual WAL reopen and three loopback nodes test adapters; no new OS/power crash or client operation. Race checked raft/node/transport/wal/storagefs/applier; original WAL sequence-tail regression retained.

## M06 replication suite

[M06](milestone-m06.md) adds 11 final test functions (10 raft, one node), preserving the two preliminary lifecycle tests and every original election/WAL fixture. T-006 covers suffix reconciliation, duplicate/short heartbeat, committed conflict, actual-send correlation, hints/old generations, local quota/Sync failure, maximum wire/WAL split, frame cuts/reopen and real three-node follower restart. Five deterministic seeds replay twice against [saved results](../fixtures/replication-v1-results.json) and independent disk-view invariants. M05 20 seeds/100 rounds still pass. No commit/applier test is misreported as implemented M07 behavior.

## M07 finite commit/application suite

[M07](milestone-m07.md) adds 14 tests: current-term commit and old-term counterexample; follower prefix caps and failed checkpoint; separate scheduled/applied cursors; real M02 capacity-1 backlog 5→7 with a committed ghost; panic/domain-negative/cancellation/result-retention paths; real checkpoint cuts/fresh CAS replay; three twice-replayed [commit traces](../fixtures/commit-v1-results.json) with partition/election/restart and independent committed/result/state oracles; real loopback concurrent CAS/failover/replay. Previous fixtures/suites pass. The simulator controls core events, runtime backlog separately checks asynchronous scheduling; neither proves every OS schedule.

## M08 phase evidence

[M08 report](milestone-m08.md): FR-009/FR-012, AC-009/AC-012 and T-009/T-012 pass only for the roadmap's PUT/DELETE, strict HTTP validation, deadlines, unknown/not_admitted and OpenAPI subset. Tests include gated disk/apply cancellation, observed higher-term loss, bounded admission with separate peer capacity, real three-node concurrent writes/election/minority, lost response without automatic retry and repeated DELETE with a new result. The schema/HTTP fixture checker includes negative controls and runtime canonical extensions. M01–M07 regressions and targeted race pass. Full GET AC-009 remains M10; dedup/failover retry AC-012 remains M11, with snapshots/history checks at later gates. C-01's M08 subset artifact/fixture obligation is satisfied; full v1 stability is not claimed. C-02 and later C-03 obligations retain their original bounds.

## M09 internal MVP evidence

[M09](milestone-m09.md) satisfies FR-014/AC-014/T-014 and FR-018/AC-018/T-018, plus the NFR-003/AC-023/T-023 roadmap subset. New controlled startup test checks follower-only replay at commit 1 with suffix 2, then legal new-term noop application. Real child-process tests kill one follower, kill the leader, kill all, restart intact storage, compare acknowledged bytes/actual replayed values and resume PUT/DELETE. Pending-write SIGTERM checks unknown, suffix exclusion and resource release. CLI/error tests cover bounded/redacted input, missing initialization, duplicate lock, repeated bootstrap, corruption, pre-start cancellation and example configuration coherence. Previous M01–M08 regression and race evidence is rerun. The tests use one temporary three-node cluster at a time, bounded waits and small data; no benchmark or physical fault. AC-023's HTTP GET variant stays at M10, snapshots at M13, complete checker at M14. Existing M08 OpenAPI/fixtures and C-01/C-02/C-03 later gates are unchanged.

## M10 read-gate evidence

[M10](milestone-m10.md): FR-019/AC-019/T-019 and the FR-009/AC-009/T-009 GET portion pass. T-019 runs seeds 42/117/203 twice with explicit event ordering, isolation after dropping in-flight messages, a committed ghost and an independent per-slot read oracle. Gated disk/apply/checkpoint tests, strict HTTP/OpenAPI GET fixtures, a five-operation HTTP history with independent ordering search/counterexample and real follower/failover/minority checks pass. Three-process crash/restart additionally verifies GET-based acknowledged-data recovery, complementing AC-023/T-023 without snapshots. Full M01–M09 regression and targeted race are recorded in the report. M11 CAS/dedup/AC-012 and M13 snapshots remain open; the general AC-022/T-022 checker remains M14. C-03 gains the M10 stale-reader/slot evidence only, not closure of every condition.

## M11 CAS/session gate evidence

[M11](milestone-m11.md) satisfies FR-010/AC-010/T-010, FR-011/AC-011/T-011 and FR-012/AC-012/T-012 for the explicit v2 session profile. Evidence includes independent ordered result/highwater models, 1024-session cap without eviction, canonical maximum CAS, gated committed duplicate/cancel/shutdown, speculative suffix exclusion, three replayed seeds run twice, real lost-response/new-leader retry, concurrent CAS/duplicates and follower/leader/all-process crash recovery. New configuration/identity/header vectors test the compatibility boundary without changing old fixtures. GET slot/partition/replay and all M01–M10 tests are retained. C-03 M11 dedup evidence is satisfied; M13 snapshots and M14 general checker remain open. M12 fault-observability infrastructure/metrics are not implemented. No finite suite or race run is a formal proof.

## M12 evidence and M11 review

[M12](milestone-m12.md) satisfies FR-017/AC-017/T-017, FR-020/AC-020/T-020 and NFR-008/AC-028/T-028: redacted logs correlate election and HTTP timeout, local metrics remain available without quorum, saved-manifest replay is identical and detects the first divergence. Seeds 7/42/117, partition/heal, delay/drop/duplicate, crash/restart, short write/ENOSPC/Sync failure, separate models and inconclusive budgets are tested. M11 review checks sequences 12/11/13, gap/conflict, 1024 cap and fresh-machine replay. V1/v2/failover/recovery/GET/CAS regressions retained; M13 snapshots and M14 general checker remain open.

## Verifiable pre-M13 plan

The [pre-M13 review](pre-m13-review.md) contains the future T-015/T-016/T-026 matrix and separates design from missing evidence. Snapshot/compaction/install tests are NOT RUN; current M12 simulation does not implement those cutpoints. First work eligible for a future assignment: stateless codec/vectors, in-memory capture handshake, oracles and isolated barrier checks before integrating active writer/recovery. Preserve the committed ghost across compaction, original session results and final vote; M14 checker gate unchanged. This review adds only regressions for the two documented M12 defects.

## M13 Phase A partial evidence

[M13 Phase A report](milestone-m13-preliminary.md): PRELIMINARY PHASE COMPLETED, overall M13 PARTIALLY COMPLETED. T-015/AC-015: codec/atomic copy/isolated restore/sessions verified, not follower transfer or term change during install. T-016/AC-016: image integrity/rejection verified; no new publication/generation/switch/crash recovery, criterion open. T-026/AC-026: in-memory 4 MiB/10000-key/16 KiB-value/1024-session limits, <=8 MiB image and single-capacity copy control verified; directory peak, physical reserves, RAM/CPU/SSD not measured. C-01 design RESOLVED with verified isolated codec conformance; C-02/OQ-01 extension and C-03 snapshot CONDITIONALLY READY. No complete gate closed from a subset; M14 not started.

## M13 Phase B — 23 November 2026

The [Phase B](milestone-m13-persistence.md) adds isolated evidence for writing/rereading, CURRENT, retained generations, fresh recovery without suffix replay, 16 cutpoints and two process exits before/after rename. T-015/AC-015/FR-015: persistence and dedup, not transfer/installation. T-016/AC-016/FR-016: isolated publication, not cleanup/compaction/live switch. T-026/AC-026/NFR-006: old+new+staging reservation and simulated quotas, not RSS/CPU report or runtime admission. C-01/OQ-05 RESOLVED; isolated C-02/OQ-01 VERIFIED but integration and C-03 CONDITIONALLY READY. M13 PARTIALLY COMPLETED, full criteria open, M14 not started.

## M13 Phase C — 23 November 2026

[Phase C](milestone-m13-runtime-recovery.md) is RUNTIME RECOVERY PHASE COMPLETED: RecoverCommitted restores S and applies only S+1..C through M02, then drains/closes the isolated worker before a fresh applier at C. WAL and snapshot validated under continuous LOCK. Startup is explicit through node.StartAPIWithSnapshotRecovery for v2; ordinary StartAPI/CLI retain the base-zero guard. No fallback on corrupt CURRENT/snapshot. KV/sessions, hard state and A=L=C verified with follower/leader/all-process crash/restarts. C-01/OQ-05 confirmed; recovery C-02/OQ-01 subset verified, overall C-02/C-03 CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016 and T-026/AC-026/NFR-006 open: compaction, cleanup and InstallSnapshot unimplemented. M13 PARTIALLY COMPLETED, M14 not started.

## M13 Phase D — 23 November 2026

[Phase D](milestone-m13-compaction-preliminary.md) is ISOLATED COMPACTION PHASE COMPLETED: approved base-S representation, candidate from actual isolated apply and field-by-field proof against full replay. Publication/recovery reuse B/C in test directories; sources and previous generations retained, no WAL repair in D proofs. PrepareCompaction is memory-only; private orchestration has no runtime callers. StartAPIWithSnapshotRecovery unchanged. C-01/OQ-05 confirmed, isolated C-02/OQ-01 evidence added; overall C-02/C-03 CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016, T-026/AC-026/NFR-006 open. InstallSnapshot, live compaction, GC and prefix deletion unimplemented; M13 PARTIALLY COMPLETED, M14 not started.

## M13 Phase E — 23 November 2026

[Phase E](milestone-m13-installsnapshot-preliminary.md) is **ISOLATED INSTALLSNAPSHOT PHASE COMPLETED**: dormant receiver/controller, existing M04 chunks, real M05 term/vote persistence, coherent local suffix, B publication under continuous LOCK, fresh KV/sessions and apply epoch, correlated final ACK and deterministic suffix catch-up verified in isolation. No ordinary node/service/server/transport integration. C-01/OQ-05 confirmed; isolated C-02/OQ-01 and C-03 evidence added, overall both CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016 and T-026/AC-026/NFR-006 remain open. Runtime InstallSnapshot, live compaction, GC and prefix deletion remain unactivated; M13 PARTIALLY COMPLETED, M14 not started.

The full official definitions remain unchanged: T-015 requires far-behind follower installation with chunk retries/term change; T-016 publication **and cleanup** cutpoints; T-026 dataset/queue/disk quota errors with RSS, CPU and peak-space evidence. E adds isolated transfer/epoch/suffix/dedup and persistence/handoff faults to A–D evidence. Remaining: ordinary RPC/replication integration, live compaction/deletion/GC, runtime quota interactions and measured resource report. Compacted logical state enables transfer; GC is not needed for isolated receipt, but cleanup must exist before its safety criterion can pass. Detailed matrix and real results are in the E report.
