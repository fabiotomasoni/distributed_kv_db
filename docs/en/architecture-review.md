# Architecture audit and pre-implementation validation

[Index](README.md) · [Italiano](../it/architecture-review.md)

**Audit 0.2 — 18 November 2026. Documentation only; no application code or database results.**

## A. Executive summary

**Outcome: APPROVED WITH CONDITIONS.** Corrected specifications technically permit M01 (deterministic state machine) and M02 (ownership, scheduling and isolated harness). They do not authorize unrestricted durable-storage, cluster or optimization work. Conditions before later phases appear in E and G; this delivery starts no milestone.

Examined all **96 pre-existing Markdown documents**: two shared indexes and 47 language pairs, including specifications, all ADRs and diagrams. Performed semantic protocol/dependency review, language comparison and whole-inventory structural checks. The two audit reports are new: **98 files** now, 48 per language plus two shared files. Review was not limited to checks claimed by the previous delivery.

Found **11 issues**: **0 CRITICAL, 6 HIGH, 4 MEDIUM, 1 LOW**. Each has an applied documentation correction. “Corrected” means a clarified contract or removed document defect, not a fixed software bug or passed application test. I found no basic voting/commit rule directly prescribing a Raft violation; this does not prove absence of every possible error.

The static 2/3 design, last-term/index election restriction, persistence before acknowledgment, current-term commit and logged reads are defensible within the non-Byzantine crash model. The most dangerous gaps were asynchronous component boundaries, snapshots and tests. Editorial quality alone did not expose them.

## B. Findings

Severity refers to baseline 0.1. All entries below are **Corrected in documentation**; implementation verification is **Not run**. Referenced documents exist in both languages; links open the current language.

### AU-001 — HIGH

**Document and section:** [raft-consensus.md](raft-consensus.md), “Commit and apply”.

**Issue, scenario and consequences:** lastApplied was also used to enqueue asynchronous work. Commit 5 followed by commit 7 before completions enqueued 1..5 and then 1..7. Possible assertion failures or reapplication; queue ordering alone is insufficient.

**Applied correction:** Introduced scheduledApplyIndex, available capacity and apply epochs, initialized after recovery with a snapshot barrier. Updated architecture, pseudocode, replication diagram, ADR-003 and T-008/apply-backlog.

**Traceability and references:** FR-008 / AC-008 / T-008; [REF-01 figura 2 / Figure 2](references.md).

### AU-002 — HIGH

**Document and section:** [api-specification.md](api-specification.md), “Proposed HTTP contract”.

**Issue, scenario and consequences:** CAS allowed raw 16 KiB expected and newValue values, but HTTP bodies were capped at 32 KiB. The Base64 pair alone needs 43,696 bytes; the checked complete compact request needs 44,116 bytes. A model-valid maximum-size CAS was always rejected by transport.

**Applied correction:** Raised client cap to 48 KiB (49,152 bytes), retaining 16 KiB values and 48 KiB entries; updated security, ADR-004, tests and criteria. Excess whitespace remains subject to the cap.

**Traceability and references:** FR-009 / AC-009 / T-009; [Calcolo su specifica locale / local-spec calculation](references.md).

### AU-003 — HIGH

**Document and section:** [storage-and-recovery.md](storage-and-recovery.md), “Snapshots and atomic publication”.

**Issue, scenario and consequences:** Final rotation mentioned serialization and vote preservation without defining the operational boundary. An implementation could copy hard state captured at S, lose a later synchronized vote or accept a completion whose sequence was reused in a new generation. InstallSnapshot did not specify revalidation after draining apply.

**Applied correction:** Defined final barrier, persistence draining, current hard state, generation_id/sequence tokens, applier/storage barriers and publication before RAM switching. Final snapshot acknowledgment differs from chunk acknowledgment; updated T-015/T-016 and ADR-005. Preserving an old vote is not an allowed choice.

**Traceability and references:** FR-013 / FR-015 / FR-016 / T-013 / T-015 / T-016; [REF-01 sez. 7 / section 7; REF-08/09](references.md).

### AU-004 — HIGH

**Document and section:** [testing-strategy.md](testing-strategy.md), “Histories and linearizability checker”.

**Issue, scenario and consequences:** “First invocation to definitive response” did not identify which response after multiple retries or handle different payloads sharing an identity. Extending an already-completed PUT until a later retry can make an otherwise impossible stale GET appear legal.

**Applied correction:** Close the interval at the earliest observed definitive response; later retries never extend it. Group only identical canonical commands; check session errors separately. Added a known counterexample that must be rejected, without claiming an existing checker.

**Traceability and references:** NFR-002 / AC-022 / T-022; FR-011 / T-011; [REF-05; REF-01 sez. 8 / section 8](references.md).

### AU-005 — HIGH

**Document and section:** [fault-injection.md](fault-injection.md), “Storage and controlled crashes”.

**Issue, scenario and consequences:** Retaining only the last Sync does not represent a process-only crash: completed writes and renames remain visible in the kernel. Simulation would miss complete non-Sync frames and already-renamed CURRENT, under-covering real recovery.

**Applied correction:** Separated ProcessCrash and SystemCrashModel; explicitly record incomplete/unsynchronized I/O survival. T-013/T-016 extend cutpoints; T-023 remains process verification. Model results are not APFS power-loss evidence.

**Traceability and references:** FR-013 / FR-016 / FR-020 / T-013 / T-016 / T-020; [REF-08/09](references.md).

### AU-006 — HIGH

**Document and section:** [implementation-roadmap.md](implementation-roadmap.md), “Capacity, milestone table and gates”.

**Issue, scenario and consequences:** M01/M02 required full cluster/shutdown ACs before those features existed; M08 required M11 deduplicated retry and M10 GET. M09 durability and M13 resource tests included later features. The gates could not all be satisfied literally in the planned order.

**Applied correction:** Defined early subsets while keeping whole ACs open until dependencies exist. Simulator foundations start at M02–M05, not entirely M12; offline integration is separate from demo presentation. Final NFR-007 gate is M16. The 144–192 hours describe capacity, not established effort.

**Traceability and references:** NFR-007 / NFR-008 / NFR-011 / T-027 / T-028 / T-031; [Dipendenze interne della roadmap / internal roadmap dependencies](references.md).

### AU-007 — MEDIUM

**Document and section:** [performance-and-resources.md](performance-and-resources.md), “Initial profile and compaction peak”.

**Issue, scenario and consequences:** The 4×8 MiB example already exhausted the 32 MiB node cap, excluding control files. The <=3 s failover target omitted available resources/quotas; saturated queues/disks may prevent progress without violating Raft.

**Applied correction:** Reserve actual sizes plus growth, with at least 64 KiB control reserve inside the cap; reject before allocation without deleting active data. Separate logical and APFS allocated bytes; state resumption-target assumptions in requirements and ACs.

**Traceability and references:** NFR-004 / NFR-006 / AC-024 / AC-026 / T-026; [Aritmetica dei limiti locali / local-limit arithmetic](references.md).

### AU-008 — MEDIUM

**Document and section:** [consistency-model.md](consistency-model.md), “GET protocol and partition”.

**Issue, scenario and consequences:** “An isolated leader cannot complete reads” was too broad for operations committed before the fault. Forbidding a legal delayed response would cause false test failures; new requests after isolation must be distinguished.

**Applied correction:** T-019/D-06 first establish isolation and discard relevant messages, then invoke GET. Another subcase permits an already-committed barrier response. Clarified logical linearization relative to local apply/failover; updated ADR-006.

**Traceability and references:** FR-019 / AC-019 / T-019; [REF-05; REF-01 sez. 8 / section 8](references.md).

### AU-009 — MEDIUM

**Document and section:** [storage-and-recovery.md](storage-and-recovery.md), “Contract and assumptions”.

**Issue, scenario and consequences:** Process crash and power loss were distinguished, but OS crash lacked a separate contract. A reader could incorrectly transfer process-kill results to reboot or hardware persistence.

**Applied correction:** Three-level table: process-crash v1 target, OS crash subject to an unvalidated platform contract, power loss outside the guarantee. Retained required Sync; reduce I/O by avoiding unnecessary rewrites/Sync, causal batching and future ReadIndex.

**Traceability and references:** NFR-003 / AC-023 / T-023; [REF-08/09](references.md).

### AU-010 — MEDIUM

**Document and section:** [network-protocol.md](network-protocol.md), “JSON input validation”.

**Issue, scenario and consequences:** The contract was strict but did not explain that encoding/json v1 accepts duplicate keys and replaces invalid UTF-8/surrogates; struct decoding may also accept case-insensitive names. A naive adapter would not satisfy the specification.

**Applied correction:** Require token/UTF-8 scanning, exact names, duplicate keys after unescaping, one document, bounded depth and canonical numbers, with T-003/T-009 fixtures. Retained standard library, no added dependency; added REF-13.

**Traceability and references:** FR-003 / FR-009 / NFR-010 / T-003 / T-009; [REF-13](references.md).

### AU-011 — LOW

**Document and section:** [failure-model.md](failure-model.md), “Scenario table, F-08 row”.

**Issue, scenario and consequences:** The unescaped bar in “1|2” created an extra Markdown column, shifting availability, invariants and tests. The earlier link check did not detect this.

**Applied correction:** Replaced with “1 contro 2” / “1 versus 2”; localized the conditional cell and clarified physically shared storage. Added table-width checking to documentation validation.

**Traceability and references:** NFR-007 / AC-027 / T-027; [Sintassi Markdown locale / local Markdown syntax](references.md).

## C. Reassessed decisions and property review

**Reads:** retain A, logged GET, for the first version; full comparison and B ReadIndex path are in [consistency](consistency-model.md) and ADR-006. A reuses replication/commit/apply but generates WAL and checkpoints; B avoids per-read entries but requires current-term commit, a fresh post-request round, term/generation correlation and apply waiting. Both need quorum for new reads; neither assumes a lease. Do not enable ReadIndex before safety gates and measurement of actual cost. It is not a necessary safety correction to baseline A.

**WAL and acknowledgments:** retain checksummed transactional framing. Writes are not assumed atomically persistent: recovery validates a complete frame before applying its effects. Positive votes/terms and replication require successful Sync; client success requires legal commit, a durable majority and apply. Synchronous commit checkpointing remains a conservative choice costing more than Raft's minimum. No required Sync was removed.

**Retries:** replicated sessions, monotonic sequences, latest result and digest in snapshots; corrected checker limits without adding follower caches or TTLs. Timeout remains unknown. Old sequences never reapply and historical results beyond the latest are not guaranteed. Clients must retain pending identity; changing IDs creates a different logical operation.

**Recovery:** follower after restart, replay only through proven commit checkpoint, never infer commit from log length. A complete uncommitted suffix may survive and later be removed. Installed generations preserve current hard state and sessions; complete corruption fails closed rather than skipping records.

**Deterministic testing:** retain an event core without direct I/O, but fake clock/storage/transport start before elections. A seed alone cannot reproduce OS processes. An independent oracle and committed ghost register are required; the race detector covers observed concurrent access, not consensus or linearizability. Check invariants through traces rather than final status alone.

| Property | Acceptable design argument | Future evidence required |
|---|---|---|
| Election Safety | Intersecting quorums and one persistent vote per term. | T-004; vote, Sync, crash and subsequent vote. |
| Log Matching | Verified prev index/term; truncate only the first uncommitted conflict. | T-006, short heartbeats, duplicates and late responses. |
| Leader Completeness | Up-to-date restriction and current-term commit, not majority alone. | T-007/T-021 with ghost prefix even after snapshots. |
| State Machine Safety | Committed prefix and ordered apply once per index/epoch. | T-008/T-015 with backlog and state replacement. |
| Linearizability | Shared log, committed-slot read, deterministic CAS and dedup. | T-019/T-022; validate checker against counterexamples too. |
| Process durability | Persist-before-ack and explicitly ordered generation publication. | T-013/T-016/T-023; no power-loss extrapolation. |

### Reassessed failure scenarios

| Scenario | Expected safety/availability and recovery | Tests |
|---|---|---|
| Follower crash | Two healthy nodes progress, then log/snapshot catch-up; host/resources available. | T-024 |
| Leader crash | No speculative success; election/current-term noop; pending outcomes uncertain. | T-004/T-012/T-024 |
| Two nodes lost | No new quorum; local status only; recover intact directories. | T-019/T-023 |
| 2 versus 1 partition | Pair can progress after stabilization; new isolated GET cannot succeed. | T-019/T-021 |
| Isolated old leader | No new barrier; already-committed responses may arrive late. | T-019, AU-008 |
| Obsolete node restarts | Starts follower, election restriction and reconciliation preserve commit. | T-006/T-014/T-015 |
| Response lost after commit | Identical retry recovers replicated session result without another effect. | T-011/T-012 |
| Corrupt WAL | Complete bad-CRC frame blocks the node; no selective replay. | T-013 |
| Crash during write | Surviving complete/partial/non-Sync frame; recovery respects checkpoint. | T-013/T-014 |
| Crash during snapshot | Published generation only; coherent old/new hard state. | T-015/T-016 |
| Crash during compaction | No deletion before publication; coordinated writer/epoch. | T-016, AU-003 |
| Concurrent elections | Split vote may prevent progress; never grants two votes in a term. | T-004/T-021 |

Liveness requires stable networking, timely scheduling/storage and unexhausted resources. Three processes cannot keep the service available while the Mac is off and do not represent three independent disks.

## D. Changes made

Updated **57 existing documents** through targeted edits; added only the two audit reports. Removed no document and did not indiscriminately rebuild the 96 files. Revised six ADRs (003–008); Go, three static members and offline/zero-spending constraints remain without new dependencies.

| Group | Change |
|---|---|
| Raft/architecture/replication diagram | Separate apply scheduling/completion, epoch management and backpressure. |
| API/network/security | 48 KiB body supporting maximum CAS; strict decoding and explicit snapshot acknowledgment. |
| Storage/snapshot diagram | Final barrier, updated vote, writer handoff, checkpoints and three-level durability. |
| Tests/faults/ACs | Mandatory counterexamples, I/O survival and isolation-test preconditions. |
| Requirements/resources | Control reserve, failover-target conditions and offline integration. |
| Roadmap/matrix | Subset gates and corrected dependencies, early testing foundations. |
| Indexes/prior reports | Navigable audit, 98-file inventory and explicitly historical initial checks. |

The following paths are updated **in both languages**; links open the current language. The shared README is updated once.

- [README.md](README.md)
- [acceptance-criteria.md](acceptance-criteria.md)
- [adr/ADR-003-raft-consensus.md](adr/ADR-003-raft-consensus.md)
- [adr/ADR-004-network-transport.md](adr/ADR-004-network-transport.md)
- [adr/ADR-005-persistence-strategy.md](adr/ADR-005-persistence-strategy.md)
- [adr/ADR-006-linearizable-reads.md](adr/ADR-006-linearizable-reads.md)
- [adr/ADR-007-client-retry-semantics.md](adr/ADR-007-client-retry-semantics.md)
- [adr/ADR-008-fault-injection.md](adr/ADR-008-fault-injection.md)
- [api-specification.md](api-specification.md)
- [architecture.md](architecture.md)
- [consistency-model.md](consistency-model.md)
- [delivery-report.md](delivery-report.md)
- [demo-and-evaluation.md](demo-and-evaluation.md)
- [design-review.md](design-review.md)
- [diagrams/06-replication.md](diagrams/06-replication.md)
- [diagrams/09-snapshot.md](diagrams/09-snapshot.md)
- [failure-model.md](failure-model.md)
- [fault-injection.md](fault-injection.md)
- [implementation-roadmap.md](implementation-roadmap.md)
- [network-protocol.md](network-protocol.md)
- [performance-and-resources.md](performance-and-resources.md)
- [raft-consensus.md](raft-consensus.md)
- [references.md](references.md)
- [requirements.md](requirements.md)
- [security.md](security.md)
- [storage-and-recovery.md](storage-and-recovery.md)
- [testing-strategy.md](testing-strategy.md)
- [traceability.md](traceability.md)
- [Root README](../../README.md)

New: [Italian audit](../it/architecture-review.md) and [English audit](../en/architecture-review.md).

### Documentation validation actually performed

Final local check: **98 Markdown files**, **48 language pairs**, **1027 relative links**, **68 tables**, **14 JSON blocks** and **16 HTTP messages with verified Content-Length**. The 13 non-Mermaid technical examples match across languages; nine diagram pairs have corresponding structure. All 32 requirements retain primary ACs/tests in the matrix; IDs and section counts align. **Zero structural errors detected**, no removed document, no new non-document file. The 57 modified files were identified by comparing before/after digests because the repository still contains documents untracked by Git.

Lightweight calculations performed: maximum compact CAS request is 44,116 bytes, above the old 32,768 cap and below the new 49,152; four 8 MiB files exhaust 33,554,432 bytes before control files. These check specification limits, not database benchmarks or tests. Reviewed Raft rules against Figure 2 and sections 5, 7, 8 of the [original paper](https://raft.github.io/raft.pdf); checked [etcd API guarantees](https://etcd.io/docs/v3.6/learning/api_guarantees/), [Go File.Sync](https://pkg.go.dev/os#File.Sync), [Go encoding/json](https://pkg.go.dev/encoding/json), [Go memory model](https://go.dev/ref/mem), [linearizability](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf) and [crash consistency](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/pillai). Sources accessed on 18 November 2026; local arguments/corrections are not experimental findings from those sources.

Limitations: no dedicated Mermaid parser/rendering, only source and structure; no Go build, race detector, executable simulator/checker, benchmark, database-process crash or OS crash. Software does not exist. No actual-machine directory Sync, locking, APFS or power-loss validation. Mechanical parity does not certify semantic equivalence; translation review is manual, not external.

## E. Remaining open questions

Findings AU-001–AU-011 are corrected **in the design**; none of that list is left without a solution. Design gates remain, separate from future tests needed to close properties.

| Condition | Block and scope | Required closure |
|---|---|---|
| C-01 / OQ-05 | Before fixing the M03 WAL/snapshot implementation | Define byte-for-byte canonical schema, kind/nullable fields, bootstrap, checksums and recovery fixtures. Finish config/protocol fixtures at M04, OpenAPI before M08 API stability. |
| C-02 / OQ-01 | Before selecting the M03 storage adapter contract; before closing M03/M09 durability | Verify documentation and lock/Sync/rename choices on macOS ARM64; lightweight adapter validation during future implementation, not destructive testing. No durability promise based on the Sync name alone. |
| C-03 | Before closing M07/M10/M11/M13/M14 | Planned apply-backlog, handoff, dedup, stale-leader and counterexample-checker tests. These are future-code acceptance gates, not already-passed tests impossibly required before M01. |

**Non-blocking for M01–M02:** OQ-02 power loss remains excluded; OQ-03 timing/RAM/SSD are measured later; OQ-04 license choice is required before publication; OQ-06 session cap is deliberate baseline; OQ-07 schedule is reforecast after initial results. ReadIndex is not a prerequisite. No open question authorizes new spending or unilateral changes to replicated quotas.

## F. Constraint verification

| Constraint | Design judgment and limitation |
|---|---|
| macOS ARM64, M5/32 GB | Go and standard library, no proposed CGO/external service: compatible architecture, no actual build performed. Storage adapter remains a specific gate. |
| €0 additional spending | No required cloud, hosting, metered APIs, trials, domains, hardware or subscriptions. This audit provisioned no resources. |
| Offline | Loopback/filesystem runtime, pre-acquired toolchain and disabled automatic fetching. Empirical confirmation in T-031 once code exists. GitHub is optional. |
| RAM/CPU | Cluster RSS target below roughly 1 GB, soft 192 MiB/node heap, blocking timers, three processes; plausible for small data, unmeasured and not a guaranteed hardware cap. |
| SSD/space | Logical dataset 4 MiB/copy, WAL 8 MiB/node, directory 32 MiB including peak/reserve, logs 1 MiB/node; active aggregate cap 99 MiB (~104 MB), ordinary target roughly 100 MB. Measure real I/O and APFS allocation. |
| Load and cleanup | Benchmark 2+10 s within 15 s deadline; integration <=30 s/case, suite <=120 s, maximum 8 clients/1,000 operations. Run cleanup, no mandatory 24/7, GPU, AI training, Docker or Kubernetes. |

Three replicas also multiply logged-read I/O: feasibility is not inferred from available RAM alone. Caps, admission and short runs make execution plausible on the already-owned computer; actual results must be measured. The complete first version is demanding for a student; the internal MVP and incremental gates contain risk without promising delivery in 144–192 hours.

## G. Final outcome

**APPROVED WITH CONDITIONS.** It is technically reasonable to start **only M01 and M02**, after any explicit owner instruction to begin development. M01 must remain a deterministic key-value state machine isolated from networking and consensus. M02 introduces ownership, scheduling and fakes without depending on already-correct distributed storage.

Before later phases close applicable C-01/C-02 gates and complete C-03 evidence at its acceptance gate. This is not blanket approval of every still-Proposed binary detail or certification of runtime guarantees. Corrected findings require neither changing Raft nor imposing ReadIndex: they require following the new barriers and oracles precisely.

**No application code was written; no milestone was started.** No deployment, service, benchmark or paid infrastructure. Both languages were updated together; the audit ends with this documentation delivery.

## H. Follow-up on 19 November 2026

The audit above is a historical record. The [pre-M03 review](pre-m03-review.md) independently re-examines actual M02 code and records current gate status: C-01 storage specification RESOLVED with checked static fixtures; C-02 documented choice CONDITIONALLY RESOLVED, actual adapter validation OPEN. Outcome CONDITIONALLY READY only for initial format conformance and isolated adapter tests in a future assignment. Before storage writer/publication integration require primitive/lock/error/visibility evidence; before M03/M09 acceptance retain T-013/T-023. C-03 remains at its original gates. No M03 implementation started and no durability claim validated.

## I. Preliminary M03 platform evidence

The [limited M03 phase](milestone-m03-preliminary.md) records GATE C-02 VERIFIED for the tested local adapter primitives and preserves the original audit scope: no full durability claim from Sync alone. C-01 schemas/fixtures remain unchanged; stateless byte conformance now runs against them. Full T-013 and later process/snapshot acceptance remain open; M03 PARTIALLY COMPLETED, no later milestone started. Earlier audit/follow-up statuses remain historical.

## Subsequent M03 completion

20 November 2026: [M03 COMPLETED locally](milestone-m03.md), with synchronous WAL, persistent transition validation, bounded reader/log recovery, explicit bootstrap and fault/process evidence. C-01/C-02 retain their resolved/verified local scope. No machine apply, consensus, full snapshot/rotation or M04 code. Prior partial/proposed statements describe their historical phase; end-to-end/physical durability remains unverified at later gates.

## Current M04 follow-up — 20 November 2026

[M04 COMPLETED](milestone-m04.md) fulfills the remaining C-01 M04 configuration/protocol fixture obligation without changing storage formats. OpenAPI remains due before M08 stability; C-03 remains at its original acceptance gates. C-02 retains the verified local Darwin/APFS scope and physical-fault limitations. A reproduced M03 incomplete-tail sequence gap was minimally fixed/tested before M04 integration. Earlier not-started statements above are historical; M05 remains not started.

## Current M05 follow-up — 20 November 2026

[M05 COMPLETED](milestone-m05.md) verifies persistent voting, term/role/timer transitions and the required 20-seed/100-heartbeat election evidence with bounded adapters. Original C-01/C-02 scope retained; C-03 remains at M07/M10/M11/M13/M14. No change to storage bytes, physical-fault assumptions or later acceptance gates. The current result supersedes historical not-started statements about M05; M06 remains not started.
