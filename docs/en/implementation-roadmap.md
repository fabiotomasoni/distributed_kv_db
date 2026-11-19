# Implementation roadmap

[Index](README.md) · [Italiano](../it/implementation-roadmap.md)

**Updated: 19 November 2026 — local M01/M02 implemented and checked; distributed specifications remain design work.**

## Capacity and dependencies

Indicative 24-week (~6-month) plan, 6–8 hours/week for one part-time student, providing 144–192 available hours, which is not a validated estimate of required effort; this is an uncertain estimate, not a commitment. Four months requires more hours or cutting optional goals, never removing correctness gates. Recover delays by dropping optimizations first, then extending the calendar. No completion date is guaranteed.

M01 is **COMPLETED** as of 19 November 2026 after explicit authorization: [report and evidence](milestone-m01.md). M02 is **COMPLETED**: [report and evidence](milestone-m02.md). M03–M16 are **Not started**. Design activities in the table mean reviewing the corresponding contract before writing its module; deliverable means a future artifact.

| ID / objective | Prerequisites | Weeks | Design, implementation and deliverable | Tests / completion | Risk | Complexity |
|---|---|---|---|---|---|---|
| M01 — KV state machine (**COMPLETED**) | — | 1–2 | Map, semantics and fixtures; operation/quota table. | T-008/T-009 within the phase subset defined below; evidence saved. | ambiguous semantics | Low |
| M02 — Ownership and concurrency (**COMPLETED**) | M01 | 3 | Events, applier and backpressure; ownership contract. | T-008/T-018 within the phase subset defined below; evidence saved. | data races | Medium |
| M03 — Local persistence | M01 | 4–5 | Frames, hard state and lock; binary specification and cutpoints. | T-013 within the phase subset defined below; evidence saved. | macOS Sync | High |
| M04 — RPC and configuration | M02,M03 | 6 | Envelope and validation; canonical configuration fixtures. | T-001/T-002/T-003/T-029 within the phase subset defined below; evidence saved. | parsing and identity | Medium |
| M05 — Leader election | M03,M04 | 7–8 | Timers and persistent votes; election simulator. | T-004/T-005 within the phase subset defined below; evidence saved. | incorrect timer resets | High |
| M06 — Log replication | M05 | 9 | AppendEntries and conflicts; divergent-log traces. | T-006 within the phase subset defined below; evidence saved. | incorrect truncation | High |
| M07 — Distributed commit | M06 | 10 | Current-term rule and apply; commit traces. | T-007/T-008 within the phase subset defined below; evidence saved. | old-term commit | High |
| M08 — API and test client | M07 | 11 | PUT/DELETE, deadlines, derive OpenAPI; HTTP fixtures. | T-009/T-012 within the phase subset defined below; evidence saved. | unstable contract | Medium |
| M09 — Crash recovery / internal MVP | M08,M03 | 12–13 | Committed replay and restart; limited MVP report. | T-014/T-018/T-023 within the phase subset defined below; evidence saved. | speculative replay | High |
| M10 — Linearizable GET | M09 | 14 | Logged read barrier; stale-leader history. | T-019 within the phase subset defined below; evidence saved. | reading outside its slot | High |
| M11 — CAS and deduplication | M10 | 15–16 | CAS and persisted sessions; retry matrix. | T-010/T-011/T-012 within the phase subset defined below; evidence saved. | ABA and sessions | High |
| M12 — Fault injection and observability | M11 | 17–18 | Scenarios and replay, logs/metrics; reproducible manifest. | T-017/T-020/T-028 within the phase subset defined below; evidence saved. | nondeterminism | High |
| M13 — Snapshots and compaction | M12 | 19–20 | Atomic generations and install; snapshot crash fixtures. | T-015/T-016/T-026 within the phase subset defined below; evidence saved. | peak space and lost dedup | High |
| M14 — Property verification | M13 | 21–22 | Independent checker and safety suite; AC report. | T-021/T-022/T-024/T-030/T-031 within the phase subset defined below; evidence saved. | incomplete checker | High |
| M15 — Bounded measurements | M14 | 23 | Latency/resource baseline; sample and limitations report. | T-025/T-026 within the phase subset defined below; evidence saved. | premature optimization | Medium |
| M16 — Demo and delivery | M15 | 24 | Experiments, bilingual review and portfolio; evidence package. | T-027/T-032 within the phase subset defined below; evidence saved. | claims beyond evidence | Medium |

## Gates

M09 is the first internal MVP: three nodes, PUT/DELETE, durable replication and recovery; do not yet advertise linearizable GET, CAS, safe retries or compactable storage. Unit tests may already design these semantics without exposing them. M10–M11 close the final client contract. M13 is necessary for the complete first version with bounded storage; M14 is the correctness gate, M15–M16 the evaluation gate.

For every milestone: (1) review the specification and open questions, (2) implement only ready dependencies, (3) run associated tests and relevant regressions, (4) record evidence and residual risks, (5) update both languages. T-027 applies at every milestone; T-021 grows from the first core tests rather than starting only at the end. Do not defer all verification to month six.

If M03 does not validate sync/locking, block every durability claim; incomplete M05 blocks replication; incomplete M07 blocks successful API responses; incomplete M13 blocks snapshot claims. No benchmark precedes the safety gate. Advanced features require new estimates rather than silently entering the schedule.

## Test subsets and gates without circular dependencies

| Milestone | Allowed subset completion | Full criteria still open |
|---|---|---|
| M01 | T-008/T-009 on the local sequential model and fixtures, including quotas and CAS semantics; no networking. | Distributed AC-008 at M07; complete API AC-009 at M10. |
| M02 | No duplicate scheduling, saturated queue, workers closed in the harness; define fake clock/transport/storage here. | Server shutdown AC-018 at M09; full replay AC-028 at M12. |
| M08 | PUT/DELETE HTTP, unknown/not_admitted classification, subset OpenAPI. | GET AC-009 at M10; dedup/failover AC-012 at M11. |
| M09 | Recovery/durability through controlled model/applier inspection; no client GET needed by the test. | End-to-end AC-023 with GET after M10; snapshots at M13. |
| M13 | Deterministic quotas, storage peak, installation and compaction. | RAM/CPU and report part of AC-026 at M15. |
| M14 | Safety/linearizability/security suite and offline integration, before the final presentation. | Performance report at M15, presentable demo and final inventory at M16. |

Other gates require their full tests once all dependencies exist. Passing a subset neither closes the full AC nor authorizes an unavailable feature. M14 closes correctness ACs; M15–M16 close measurement/delivery ACs. T-031 uses an independent offline integration procedure at M14 without depending on completion of the M16 demo. The matrix identifies the final requirement gate; the roadmap may introduce fixtures/subsets earlier.

M12 completes fault injection rather than starting its foundations: introduce fake clock/scheduler at M02, storage cutpoints at M03 and election oracles at M05. Reforecast after M03; 4–6 months is a desired horizon, prioritizing the internal MVP. Snapshots and the checker may need more time without introducing expenses or weakening correctness.

## Pre-M03 readiness after M02 review

[Review dated 19 November 2026](pre-m03-review.md): CONDITIONALLY READY. C-01 storage schema/fixtures resolved; C-02 platform-source review complete, implementation validation still open. A future M03 assignment may start only format conformance and isolated local adapter tests under the [finite resource plan](macos-storage-contract.md). Require primitive/lock/error/visibility evidence before integrating a storage writer/publication path. Failed or unsupported barriers block that path and require ADR revision; no silent downgrade. T-013 remains the M03 acceptance gate; T-023 M09 durability and M13 snapshot tests remain open. M03 is Not started; no benchmark, cluster or durable-success claim is authorized by this design review.
