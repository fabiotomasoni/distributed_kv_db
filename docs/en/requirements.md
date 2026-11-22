# Verifiable requirements

[Index](README.md) · [Italiano](../it/requirements.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Normative rules

“Must” identifies a verifiable requirement; “target” denotes a value still to be measured. All requirements are **Must** for the complete first version except NFR-005 (Should: report mandatory, thresholds set after a baseline). Assignment to a milestone is not a result. The tests and criteria below are planned, not executed.

The problem in the second column explains the motivation: identity prevents voting aliases; ordering and quorums prevent divergence; storage supports the crash model; limits and offline execution make the project feasible on available hardware. Operational detail: [API](api-specification.md), [Raft](raft-consensus.md), [storage](storage-and-recovery.md).

| ID | Requirement and motivation | Priority | Objective acceptance | Verification |
|---|---|---|---|---|
| FR-001 | **Startup and identity**. Validate unique node_id and exclusive directory before opening listeners. **Reason:** Prevent aliases and concurrent access to the same storage. | Must | Three distinct directories and identities; second instance rejected. | AC-001 / T-001 |
| FR-002 | **Static configuration**. Accept exactly three members with matching cluster_id and config_hash. **Reason:** Prevent incompatible clusters and different majorities. | Must | Divergent manifest or duplicate member prevents startup. | AC-002 / T-002 |
| FR-003 | **Peer RPC**. Exchange versioned RPCs with validated identity and limits. **Reason:** Contain incorrect inputs before consensus. | Must | Malformed payloads rejected without state mutation. | AC-003 / T-003 |
| FR-004 | **Election**. Elect through a majority and one persistent vote per term. **Reason:** Prevent competing leaders in the same term. | Must | No term has two leaders in trace, including crash after voting. | AC-004 / T-004 |
| FR-005 | **Heartbeat**. Send empty AppendEntries and use timers without busy loops. **Reason:** Maintain stable leadership at rest. | Must | Stable leader produces no spurious elections in a fault-free simulator. | AC-005 / T-005 |
| FR-006 | **Replication and conflicts**. Reconcile uncommitted suffixes while preserving prefixes. **Reason:** Repair divergence without losing decisions. | Must | Divergent logs converge without overwriting committed entries. | AC-006 / T-006 |
| FR-007 | **Commit**. Commit using majority matchIndex and current term. **Reason:** Prevent unsafe commit after election. | Must | Old entry is not committed merely by counting replicas. | AC-007 / T-007 |
| FR-008 | **Ordered apply**. Apply the committed prefix once per index in each state-machine reconstruction; replay after restart is not a new logical effect. **Reason:** Obtain deterministic replicas. | Must | Same sequence produces identical state and results on all three nodes. | AC-008 / T-008 |
| FR-009 | **PUT/GET/DELETE**. Expose complete operations on valid keys and values. **Reason:** Provide an unambiguous client contract. | Must | Absence, empty value and repeated DELETE match the API table. | AC-009 / T-009 |
| FR-010 | **Atomic CAS**. Evaluate comparison and update as a single applied command. **Reason:** Prevent lost updates on one key. | Must | Two concurrent CAS with the same expected: at most one changes the value. | AC-010 / T-010 |
| FR-011 | **Client sessions**. Serialize client sequences and retain the replicated latest result. **Reason:** Prevent duplicate effects. | Must | Identical retry never reapplies; changed payload is an error. | AC-011 / T-011 |
| FR-012 | **Deadlines and retry**. Classify timeout after admission as an uncertain outcome. **Reason:** Avoid confusing missing responses with rollback. | Must | Response lost after commit resolved by retry with the same identifier. | AC-012 / T-012 |
| FR-013 | **Durable WAL**. Sync hard state and log before positive Raft responses. **Reason:** Make acknowledged votes and replicas reliable. | Must | Cutpoints before/after Sync preserve votes and acknowledged prefix. | AC-013 / T-013 |
| FR-014 | **Restart**. Restart as follower, rebuilding only proven committed state. **Reason:** Never expose speculative commands after crashes. | Must | Crash and restart never expose an uncommitted suffix. | AC-014 / T-014 |
| FR-015 | **Snapshots**. Include map, sessions, index, term and configuration. **Reason:** Recover peers lacking the old log. | Must | InstallSnapshot restores identical state and deduplication. | AC-015 / T-015 |
| FR-016 | **Compaction**. Delete old WAL only after durable snapshot publication. **Reason:** Bound growth without removing necessary data. | Must | A crash at every phase leaves at least one recoverable generation. | AC-016 / T-016 |
| FR-017 | **Diagnostics**. Expose role, indices and logs without user data. **Reason:** Explain faults without disclosing values. | Must | Status and logs correlate an election and request without values. | AC-017 / T-017 |
| FR-018 | **Shutdown**. Stop admission, close workers and synchronize storage. **Reason:** Prevent orphaned resources and inconsistent stops. | Must | Shutdown within 5 s or reported termination followed by recovery. | AC-018 / T-018 |
| FR-019 | **Linearizable reads**. Complete GET only after a committed and applied current-term barrier. **Reason:** Block stale reads from the minority. | Must | Isolated leader never returns a local read as success. | AC-019 / T-019 |
| FR-020 | **Fault injection**. Reproduce network, storage and process events with seed and trace. **Reason:** Reproduce rare faults. | Must | Two simulated replays have the same transition sequence. | AC-020 / T-020 |
| NFR-001 | **Safety**. Preserve Election Safety, Log Matching, Leader Completeness and State Machine Safety. **Reason:** Preserve the meaning of consensus. | Must | Zero violations in the finite deterministic suite; no claim of exhaustive proof. | AC-021 / T-021 |
| NFR-002 | **Linearizability**. Respect real-time order of completed operations. **Reason:** Respect client-observed real-time order. | Must | Checker finds a legal order for all specified small histories. | AC-022 / T-022 |
| NFR-003 | **Conditional durability**. Retain successes with a recoverable persistent majority. **Reason:** Retain successes within the stated model. | Must | No success lost in process-crash tests within the model. | AC-023 / T-023 |
| NFR-004 | **Single-crash tolerance**. Progress with two healthy nodes and timely networking. **Reason:** Demonstrate availability with one failed replica. | Must | After stabilization, with storage/CPU available and quotas unsaturated, resume within the 3 s local target. | AC-024 / T-024 |
| NFR-005 | **Measurable performance**. Measure throughput, mean, p95/p99 without invented thresholds. **Reason:** Compare changes using evidence. | Should | Report includes samples, errors, hardware and measurement interval. | AC-025 / T-025 |
| NFR-006 | **Bounded resources**. Enforce logical, queue and disk caps; measure RAM/CPU targets. **Reason:** Protect the already-owned Mac. | Must | Quota yields a deterministic error; resource profile and peak disk recorded. | AC-026 / T-026 |
| NFR-007 | **Bilingual maintainability**. Maintain parallel documents and traceable decisions. **Reason:** Enable maintenance without language divergence. | Must | File, identifier and link parity checked on every revision. | AC-027 / T-027 |
| NFR-008 | **Testability**. Inject time, randomness, networking and storage without external services. **Reason:** Separate the core from its real environment. | Must | A regression reproduces from the saved manifest and seed. | AC-028 / T-028 |
| NFR-009 | **ARM64 portability**. Use Go and its standard library on darwin/arm64. **Reason:** Avoid extra toolchains or hardware. | Must | Local build and tests record toolchain and architecture. | AC-029 / T-029 |
| NFR-010 | **Local security**. Restrict binding to loopback, inputs and file permissions. **Reason:** Limit accidental exposure and hostile input. | Must | Public bind rejected, files private, oversized payloads rejected. | AC-030 / T-030 |
| NFR-011 | **Offline**. Run cluster and suite without Internet after bootstrap. **Reason:** Avoid external services for operation. | Must | With external networking disabled, offline integration of planned operations succeeds; final presentation is separate. | AC-031 / T-031 |
| NFR-012 | **Zero additional cost**. Require no paid services, purchases or infrastructure. **Reason:** Respect the absolute financial constraint. | Must | Dependency inventory and procedure require neither payments nor mandatory accounts. | AC-032 / T-032 |

The [matrix](traceability.md) maps every requirement to a module, ADR, milestone and test. No requirement can be closed by inspecting a diagram alone. For NFR-001–003, tests provide finite evidence rather than universal proofs.

## Pre-M03 contract refinement

[Review](pre-m03-review.md) fixes the [canonical storage schema](canonical-formats.md) and [conditional macOS adapter contract](macos-storage-contract.md) for FR-013/FR-014/FR-015/FR-016, NFR-003/NFR-006. No requirement/ID is removed or weakened: process-crash durability remains the target; OS crash and power-loss guarantees are not added. Format-fixture arithmetic is design evidence, not satisfaction of AC-013/AC-023.

## M05 requirement evidence

FR-004/FR-005 have implemented election/heartbeat evidence in [M05](milestone-m05.md): AC-004/T-004 over 20 reproducible split-vote/crash-after-Sync seeds, AC-005/T-005 over 100 healthy heartbeat rounds. NFR-001 and NFR-008 gain finite election oracles/injected clocks/storage/network, not closure of all Raft safety or future replay requirements. NFR-009 native ARM64 checks pass again. No requirement is removed or weakened; no distributed API/durability claim follows from election alone.

## M10 read-gate evidence

[M10](milestone-m10.md): FR-019/AC-019/T-019 and the FR-009/AC-009/T-009 GET portion pass. T-019 runs seeds 42/117/203 twice with explicit event ordering, isolation after dropping in-flight messages, a committed ghost and an independent per-slot read oracle. Gated disk/apply/checkpoint tests, strict HTTP/OpenAPI GET fixtures, a five-operation HTTP history with independent ordering search/counterexample and real follower/failover/minority checks pass. Three-process crash/restart additionally verifies GET-based acknowledged-data recovery, complementing AC-023/T-023 without snapshots. Full M01–M09 regression and targeted race are recorded in the report. M11 CAS/dedup/AC-012 and M13 snapshots remain open; the general AC-022/T-022 checker remains M14. C-03 gains the M10 stale-reader/slot evidence only, not closure of every condition.

## M11 CAS/session gate evidence

[M11](milestone-m11.md) satisfies FR-010/AC-010/T-010, FR-011/AC-011/T-011 and FR-012/AC-012/T-012 for the explicit v2 session profile. Evidence includes independent ordered result/highwater models, 1024-session cap without eviction, canonical maximum CAS, gated committed duplicate/cancel/shutdown, speculative suffix exclusion, three replayed seeds run twice, real lost-response/new-leader retry, concurrent CAS/duplicates and follower/leader/all-process crash recovery. New configuration/identity/header vectors test the compatibility boundary without changing old fixtures. GET slot/partition/replay and all M01–M10 tests are retained. C-03 M11 dedup evidence is satisfied; M13 snapshots and M14 general checker remain open. M12 fault-observability infrastructure/metrics are not implemented. No finite suite or race run is a formal proof.

## M12 evidence and M11 review

[M12](milestone-m12.md) satisfies FR-017/AC-017/T-017, FR-020/AC-020/T-020 and NFR-008/AC-028/T-028: redacted logs correlate election and HTTP timeout, local metrics remain available without quorum, saved-manifest replay is identical and detects the first divergence. Seeds 7/42/117, partition/heal, delay/drop/duplicate, crash/restart, short write/ENOSPC/Sync failure, separate models and inconclusive budgets are tested. M11 review checks sequences 12/11/13, gap/conflict, 1024 cap and fresh-machine replay. V1/v2/failover/recovery/GET/CAS regressions retained; M13 snapshots and M14 general checker remain open.

## Pre-M13 traceability

[Review](pre-m13-review.md): FR-015/AC-015/T-015, FR-016/AC-016/T-016 and NFR-006/AC-026/T-026 subset remain UNIMPLEMENTED M13 requirements; future matrix defined with FR-011/FR-013/FR-014 dependencies. C-01/OQ-05 closed in design only; new C-02/OQ-01 path and C-03 snapshot CONDITIONALLY READY. M14 T-021/T-022/T-024/T-030/T-031 unchanged.

## M13 requirements after Phase A

FR-015/AC-015/T-015 and FR-016/AC-016/T-016 remain partial; NFR-006/AC-026/T-026 verified only for this phase's isolated quota subset; no requirement removed. Copy/restore preserve FR-008/FR-011 without proving consensus or end-to-end recovery. [Report and limits](milestone-m13-preliminary.md). M13 PARTIALLY COMPLETED; M14 not started.
