# Objective acceptance criteria

[Index](README.md) · [Italiano](../it/acceptance-criteria.md)

**Updated: 19 November 2026 — local M01 implemented and checked; distributed specifications remain design work.**

## Acceptance rule

Full criteria remain open; the local M01 subsets of T-008/T-009 and M02 subsets of T-008/T-018 are **Passed**, as documented below. Closing a row requires a repeatable procedure, evidence tied to a revision/configuration and a satisfied condition. “No panic” or an error-free log is insufficient. See the [test catalog](testing-strategy.md) for scheduling and fixture details.

| Criterion | Requirement / procedure | Required evidence | Passing condition |
|---|---|---|---|
| AC-001 | FR-001 / T-001. Start three distinct configurations and attempt a second opening of n1. | Manifest and exit codes. | Three distinct directories and identities; second instance rejected. |
| AC-002 | FR-002 / T-002. Vary membership, cluster ID and hash in configuration and RPCs. | Valid/invalid fixtures and before/after state. | Divergent manifest or duplicate member prevents startup. |
| AC-003 | FR-003 / T-003. Inject invalid JSON, incorrect envelope and oversized body. | Responses and unchanged state hash. | Malformed payloads rejected without state mutation. |
| AC-004 | FR-004 / T-004. Simulate split votes and crash immediately after vote Sync; replay for 20 seeds. | Vote/term/leader trace and seed. | No term has two leaders in trace, including crash after voting. |
| AC-005 | FR-005 / T-005. Advance virtual time through 100 fault-free heartbeats. | Election count and timers. | Stable leader produces no spurious elections in a fault-free simulator. |
| AC-006 | FR-006 / T-006. Construct divergent logs, send heartbeats and obsolete responses, then heal. | Prefixes, matchIndex and transitions. | Divergent logs converge without overwriting committed entries. |
| AC-007 | FR-007 / T-007. Reproduce an old-term entry on a majority, then a current noop. | Unchanged commit before, advanced commit after. | Old entry is not committed merely by counting replicas. |
| AC-008 | FR-008 / T-008. Apply the same sequence to three models, delaying one applier. | Canonical digest and per-index results. | Same sequence produces identical state and results on all three nodes. |
| AC-009 | FR-009 / T-009. Enumerate absent key, empty value, invalid input and repeated DELETE. | API fixture table and model. | Absence, empty value and repeated DELETE match the API table. |
| AC-010 | FR-010 / T-010. Run two CAS from different sessions on the same value with different new values. | History with exactly one successful change. | Two concurrent CAS with the same expected: at most one changes the value. |
| AC-011 | FR-011 / T-011. Duplicate latest sequence, vary payload, use an old sequence and a gap. | Results and session table after replay. | Identical retry never reapplies; changed payload is an error. |
| AC-012 | FR-012 / T-012. Drop response after apply, change leader, retry identically. | One mutation and original result. | Response lost after commit resolved by retry with the same identifier. |
| AC-013 | FR-013 / T-013. Cut frames at every boundary; simulate short write/Sync errors and incorrect CRC. | Disk images and recovery decision. | Cutpoints before/after Sync preserve votes and acknowledged prefix. |
| AC-014 | FR-014 / T-014. Restart all with an uncommitted suffix and a lower commit checkpoint. | Reconstructed state before and after a new noop. | Crash and restart never expose an uncommitted suffix. |
| AC-015 | FR-015 / T-015. Install snapshot on a far-behind follower with chunk retries and term change. | Image hash, sessions and final indices. | InstallSnapshot restores identical state and deduplication. |
| AC-016 | FR-016 / T-016. Stop at every generation publication/cleanup cutpoint. | CURRENT and recovered generation for each case. | A crash at every phase leaves at least one recoverable generation. |
| AC-017 | FR-017 / T-017. Run election and timeout, inspect status and logs without user values. | Redacted log sample and metric snapshot. | Status and logs correlate an election and request without values. |
| AC-018 | FR-018 / T-018. Send shutdown during RPC and persistence; check workers and lock. | Duration, goroutines and subsequent recovery. | Shutdown within 5 s or reported termination followed by recovery. |
| AC-019 | FR-019 / T-019. Establish complete isolation and drop pending messages; then write on the majority and invoke a new old-leader GET. | History excluding successful stale reads. | Isolated leader never returns a local read as success. |
| AC-020 | FR-020 / T-020. Replay a scenario twice using the same seed and virtual clock. | Byte-identical event traces. | Two simulated replays have the same transition sequence. |
| AC-021 | NFR-001 / T-021. Enumerate small schedules with 3 nodes, 3 proposals and up to 2 faults. | Invariant report and explored bound. | Zero violations in the finite deterministic suite; no claim of exhaustive proof. |
| AC-022 | NFR-002 / T-022. Check 20 histories of at most 12 operations with CAS, retry and failover. | Witness order or reduced counterexample. | Checker finds a legal order for all specified small histories. |
| AC-023 | NFR-003 / T-023. Acknowledge data, crash one then all processes, recover intact directories. | Acknowledged-success list compared with later GETs. | No success lost in process-crash tests within the model. |
| AC-024 | NFR-004 / T-024. Stop a follower, restart it and await catch-up; then stop the leader leaving two healthy nodes. | Election timeline and resumption within local target. | After stabilization, with storage/CPU available and quotas unsaturated, resume within the 3 s local target. |
| AC-025 | NFR-005 / T-025. Run bounded benchmark under the resource profile. | Raw samples and methodology. | Report includes samples, errors, hardware and measurement interval. |
| AC-026 | NFR-006 / T-026. Reach dataset, queue and simulated-disk quotas without massive load. | Errors, RSS, CPU and peak space. | Quota yields a deterministic error; resource profile and peak disk recorded. |
| AC-027 | NFR-007 / T-027. Compare language trees, IDs, technical blocks and links; semantic review. | Review report with stated limitations. | File, identifier and link parity checked on every revision. |
| AC-028 | NFR-008 / T-028. Inject fake clock, PRNG, transport and storage into the same core. | Regression replay without real networking. | A regression reproduces from the saved manifest and seed. |
| AC-029 | NFR-009 / T-029. Record go version/GOARCH and build on the ARM64 Mac. | Build output and local suite. | Local build and tests record toolchain and architecture. |
| AC-030 | NFR-010 / T-030. Try binding 0.0.0.0, oversized input and broad permissions. | Rejections and actual permissions. | Public bind rejected, files private, oversized payloads rejected. |
| AC-031 | NFR-011 / T-031. Disable Internet while retaining loopback and run independent offline integration; present the demo at M16. | Dependency manifest and offline trace. | With external networking disabled, offline integration of planned operations succeeds; final presentation is separate. |
| AC-032 | NFR-012 / T-032. Review dependencies and procedure on already-owned hardware. | License, cost and prerequisite inventory. | Dependency inventory and procedure require neither payments nor mandatory accounts. |

## Gates and waivers

M09 closes only the roadmap MVP subset; M14 closes all correctness conditions; M15 closes the resource/performance report; M16 closes demo, budget and delivery. An inconclusive test stays open. Each waiver records the requirement, reason and risk in both languages; it never authorizes advertising an unverified guarantee. No waiver may introduce expenses or implementation in this phase.

## Mandatory subcases added by the audit

ACs remain Not run. AC-003/AC-009 require strict parsing and maximum CAS within a 48 KiB body; AC-008 includes commit backlog and full queues without duplicate apply; AC-013/AC-016 include surviving non-Sync writes and updated hard state at generation switch; AC-015 includes applyEpoch and a snapshot becoming obsolete during transfer; AC-019 distinguishes a new GET after isolation from a response for an already-committed read; AC-022 requires the retry-after-completion counterexample; AC-026 includes control-file reservation.

Early [roadmap](implementation-roadmap.md) subsets do not close these whole criteria. AC-031 uses independent offline integration at M14; demo presentation at M16 is not a circular prerequisite. Every subcase must appear in its primary test evidence; these are not new untraced requirements.

## Limited M01 closure

M01 is COMPLETED only within its official subset: local semantics, sequential CAS, validation, quotas, byte ownership and determinism verified by actual tests. [M01 report](milestone-m01.md). Distributed AC-008, HTTP/Raft-GET AC-009, concurrent AC-010, dedup AC-011 and cluster-resource AC-026 are not closed. This M01 record closes no later full criterion; M02 local evidence follows.

## M02 review and pre-M03 gate evidence

M02 local T-008/T-018 subsets Passed, reviewed from source with 21 tests and targeted race checks; full server/distributed ACs remain open. [Pre-M03 report](pre-m03-review.md): C-01 design resolved, C-02 implementation validation open, CONDITIONALLY READY only for initial conformance/adapter tests. AC-013 requires the future codec to consume positive/negative [static fixtures](canonical-formats.md), plus partial-write/Sync/error/cutpoint tests. AC-016 requires publication/handoff tests at its phase; AC-023 still requires actual process-restart evidence. None is closed by this documentation.

## Subsequent preliminary M03 evidence

[Phase report](milestone-m03-preliminary.md): GATE C-02 VERIFIED for local primitives, M03 PARTIALLY COMPLETED. 7 storageformat tests/18 subcases and 13 storagefs test functions/16 subcases pass; targeted race and unchanged M01/M02 regressions pass. Codec scope is stateless frames/entries/digests/manifests; no WAL/replay engine or complete AC-013/AC-014/AC-023 closure. Full snapshot and later gates remain open. Earlier figures/statuses describe their historical phase.

## Local M03 closure

[Final evidence](milestone-m03.md): M03 COMPLETED; local T-013/storage AC-013 portion Passed. WAL has 17 top-level functions/25 subcases, 82.9% local statement coverage; M01/M02, codec/adapter regressions and targeted WAL/adapter/applier race checks pass. All 18 canonical recovery expectations covered at the appropriate byte/namespace boundary. Full FR-013 Raft-response integration, AC-014 server restart and AC-023 end-to-end remain dependent on later milestones; AC-015/AC-016 snapshot acceptance is M13. Coverage/race/process tests do not certify physical-fault resilience. No primary requirement/AC ID or final distributed gate removed.

## M04 acceptance evidence

[M04 COMPLETED](milestone-m04.md): AC-001/AC-002/AC-003/AC-029 pass for FR-001/FR-002/FR-003/NFR-009 within the official phase subset. Evidence: three isolated library startups, lock/manifest failure before bind, exact configuration hash/protocol fixtures, malformed/oversized messages without processor delivery, bounded local exchange and ARM64 build/tests. Runtime CLI exit-code UX and later consensus/client/durability criteria remain unclaimed. M03 tail corruption regression is fixed and prior-component tests pass.

## M05 acceptance evidence

[M05 COMPLETED](milestone-m05.md): AC-004/T-004/FR-004 passed with all 20 saved seed histories and twice-replayed trace fingerprints, forced split vote and model crash immediately after vote Sync; no duplicate durable votes or leaders per term. AC-005/T-005/FR-005 passed through 100 healthy virtual rounds, 200 peer heartbeats and unchanged leader/term. Real loopback/WAL integration, term/vote reopening, error/shutdown and targeted race pass. This does not close AC-006/AC-007/AC-008 or distributed safety/client/durability criteria.

## M06 acceptance result

[M06 COMPLETED](milestone-m06.md): FR-006/AC-006/T-006 passed with ordinary/lagging/divergent logs, immutable known committed prefix, correlated nextIndex/matchIndex, retries/old responses, storage failure and restart. Five seeded replication traces replay twice against a Log Matching/append-only oracle; real loopback follower restart catches up. Checkpoint stays fixed; AC-007/AC-008 and the current-term quorum commit rule remain M07. All prior tests/fixtures retained.

## M07 acceptance evidence

[M07 COMPLETED](milestone-m07.md): FR-007/AC-007/T-007 pass old-term-majority/no-current-noop versus legal current-term indirect commit, no-quorum/stale response and checkpoint failure/recovery. FR-008/AC-008/T-008 pass identical three-model state/per-index results, 5→7 blocked capacity-1 backlog, no speculative/duplicate apply and unchanged M02 worker. C-03 M07 backlog/ghost obligation satisfied; snapshot handoff, session/read/history gates remain at their phases. No public client/complete durability acceptance inferred.

## M08 phase evidence

[M08 report](milestone-m08.md): FR-009/FR-012, AC-009/AC-012 and T-009/T-012 pass only for the roadmap's PUT/DELETE, strict HTTP validation, deadlines, unknown/not_admitted and OpenAPI subset. Tests include gated disk/apply cancellation, observed higher-term loss, bounded admission with separate peer capacity, real three-node concurrent writes/election/minority, lost response without automatic retry and repeated DELETE with a new result. The schema/HTTP fixture checker includes negative controls and runtime canonical extensions. M01–M07 regressions and targeted race pass. Full GET AC-009 remains M10; dedup/failover retry AC-012 remains M11, with snapshots/history checks at later gates. C-01's M08 subset artifact/fixture obligation is satisfied; full v1 stability is not claimed. C-02 and later C-03 obligations retain their original bounds.

## M09 internal MVP evidence

[M09](milestone-m09.md) satisfies FR-014/AC-014/T-014 and FR-018/AC-018/T-018, plus the NFR-003/AC-023/T-023 roadmap subset. New controlled startup test checks follower-only replay at commit 1 with suffix 2, then legal new-term noop application. Real child-process tests kill one follower, kill the leader, kill all, restart intact storage, compare acknowledged bytes/actual replayed values and resume PUT/DELETE. Pending-write SIGTERM checks unknown, suffix exclusion and resource release. CLI/error tests cover bounded/redacted input, missing initialization, duplicate lock, repeated bootstrap, corruption, pre-start cancellation and example configuration coherence. Previous M01–M08 regression and race evidence is rerun. The tests use one temporary three-node cluster at a time, bounded waits and small data; no benchmark or physical fault. AC-023's HTTP GET variant stays at M10, snapshots at M13, complete checker at M14. Existing M08 OpenAPI/fixtures and C-01/C-02/C-03 later gates are unchanged.

## M10 read-gate evidence

[M10](milestone-m10.md): FR-019/AC-019/T-019 and the FR-009/AC-009/T-009 GET portion pass. T-019 runs seeds 42/117/203 twice with explicit event ordering, isolation after dropping in-flight messages, a committed ghost and an independent per-slot read oracle. Gated disk/apply/checkpoint tests, strict HTTP/OpenAPI GET fixtures, a five-operation HTTP history with independent ordering search/counterexample and real follower/failover/minority checks pass. Three-process crash/restart additionally verifies GET-based acknowledged-data recovery, complementing AC-023/T-023 without snapshots. Full M01–M09 regression and targeted race are recorded in the report. M11 CAS/dedup/AC-012 and M13 snapshots remain open; the general AC-022/T-022 checker remains M14. C-03 gains the M10 stale-reader/slot evidence only, not closure of every condition.

## M11 CAS/session gate evidence

[M11](milestone-m11.md) satisfies FR-010/AC-010/T-010, FR-011/AC-011/T-011 and FR-012/AC-012/T-012 for the explicit v2 session profile. Evidence includes independent ordered result/highwater models, 1024-session cap without eviction, canonical maximum CAS, gated committed duplicate/cancel/shutdown, speculative suffix exclusion, three replayed seeds run twice, real lost-response/new-leader retry, concurrent CAS/duplicates and follower/leader/all-process crash recovery. New configuration/identity/header vectors test the compatibility boundary without changing old fixtures. GET slot/partition/replay and all M01–M10 tests are retained. C-03 M11 dedup evidence is satisfied; M13 snapshots and M14 general checker remain open. M12 fault-observability infrastructure/metrics are not implemented. No finite suite or race run is a formal proof.
