# Synchronous WAL and snapshot generations

[Index](../README.md) · [Italiano](../../it/adr/ADR-005-persistence-strategy.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-005 — Synchronous WAL and snapshot generations**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

Process crashes must not erase successes within the declared model.

## Problem

Coordinate log, vote, commit checkpoint, snapshots and disk caps.

## Alternatives considered

RAM-only; async WAL; sync WAL and snapshots; external database engine.

## Decision

Transactional CRC32C WAL frames, Sync before acknowledgment, generation snapshots with CURRENT.

## Rationale

Separates consensus decisions from local durability and enables reproducible cutpoints.

## Consequences

Snapshots include sessions; storage errors stop the node; local commit checkpoint.

## Drawbacks and limitations

Sync latency, temporary peak space and unproved APFS power-loss semantics.

## Linked requirements

FR-013, FR-014, FR-015, FR-016, FR-018, NFR-003, NFR-006. See the [matrix](../traceability.md).

## Validation and open questions

M03/M13: directory sync, lock, final format and cutpoints; power loss remains a separate guarantee. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../storage-and-recovery.md); [sources REF-08/09](../references.md); [roadmap](../implementation-roadmap.md).

## Audit 0.2 reassessment

Synchronous WAL confirmed, not physical write atomicity: recovery accepts only verified complete transactions. Specified final barrier, updated hard state, generation_id/write_sequence tokens and installation coordinated with the applier. The 32 MiB cap includes control reserve. Separate process-, OS- and power-loss contracts; only the first is the v1 target verifiable through process tests. No required Sync is removed. OQ-01/OQ-05 remain gates before durable storage.

## Pre-M03 decision — 19 November 2026

Retain this strategy and Proposed implementation status; no new ADR is needed. [Canonical formats](../canonical-formats.md) resolve the C-01 design prerequisite with checked bytes. [macOS contract](../macos-storage-contract.md) is the formally selected candidate: File.Sync for regular files on the inspected Go baseline, explicit checked directory fsync, stable LOCK inode with nonblocking exclusive flock, serialized same-filesystem rename/publication, fail-closed errors. Sources/choices reviewed; capability tests NOT RUN.

C-02 is CONDITIONALLY RESOLVED for documented choices and OPEN for implementation validation, consistent with the original audit’s future-adapter test stage. Only isolated conformance/capability tests are eligible first; writer/publication integration waits for primitive/lock/error/visibility evidence. Unsupported directory barriers require revision here, not an ignored error or generic Sync guarantee. T-013/T-023 and T-015/T-016 remain implementation acceptance gates. [Readiness report](../pre-m03-review.md). No M03 code or OS/power-loss claim.

## Preliminary M03 validation

[Evidence](../milestone-m03-preliminary.md): GATE C-02 VERIFIED for the tested local Darwin/APFS primitive choices. Adopt stable flock ownership, checked File.Sync/directory fsync and same-parent CURRENT replacement with explicit uncertain failures. The overall generation/WAL strategy remains Proposed until its full phase tests; this does not accept unimplemented recovery, snapshots or OS/power-loss guarantees. M03 remains PARTIALLY COMPLETED, pending a separate assignment for its remaining work.

## Local M03 implementation decision — 20 November 2026

[M03 local WAL](../milestone-m03.md) adopts this ADR for initial-generation framed append, hard-state/log/commit recovery, lock and explicit bootstrap. Local T-013 evidence passes; generation/sequence tokens follow the existing contract. The same File.Sync/directory barriers and PublishCurrent are reused. The complete snapshot/rotation portion remains Proposed until M13, end-to-end durability until M09; no OS/power-loss guarantee is accepted. No new persistence format or redundant applied cursor added.

## M05 election implementation — 20 November 2026

[M05](../milestone-m05.md) adopts this decision for deterministic roles/timers, one persistent vote per term, exact generation/sequence completion and the local election noop. One serial synchronous storage adapter suspends core advancement without holding a global mutex; no positive dependent output before Sync. Shutdown preserves I/O failures and ownership until drain. The 20-seed election oracle, 100 healthy heartbeat rounds and targeted race pass. Full replication/commit/apply/snapshot and physical-fault claims remain at their original gates; this does not mark all Raft or generation rotation Accepted.

## M06 implementation decision

[M06](../milestone-m06.md) retains this ADR: canonical suffix replacement above the known committed boundary, synchronized exact-token completion and bounded peer progress. Existing WAL bytes/primitives are reused; local proposals preserve reserve through AppendProposal. Legal large RPCs can span bounded WAL transactions with a single final ack and safe recoverable prefix on interruption. Commit/current-term quorum/apply and full snapshot semantics remain later gates. No complete-Raft or OS/power durability acceptance is inferred.

## M07 implementation evidence

[M07](../milestone-m07.md) retains this decision for current-term quorum proof, synchronized commit_index, fresh volatile-state replay through unchanged M02 and explicit completion cursors. Independent committed/result oracles, gated backlog, checkpoint cuts and complementary loopback tests pass. No extra checkpoint, new persistence bytes or per-entry goroutine. C-03 M07 evidence does not close later snapshots/sessions/read/checker gates or physical-fault guarantees.

## M09 conditional process durability — 22 November 2026

[M09](../milestone-m09.md) supplies actual single/all-process kill/restart evidence for the internal MVP, with acknowledged-history bytes and fresh applied-state inspection. Retain the existing synchronous WAL/checkpoint protocol; no new applied cursor or storage format. Tests require intact directories and a recoverable majority. This accepts only the roadmap's process-crash subset: HTTP GET evidence remains after M10 and general snapshots at M13. No OS/power-loss acceptance or complete fault proof.

## Pre-M13 design decision — 22 November 2026

The [pre-M13 review](../pre-m13-review.md) retains QSN1/payload v1 with complete sessions and v2 identity/config for future stateful runtime; no v1 migration or vector changes. Select capture after worker drain/parking and atomic KV/session copy, a separate final barrier with updated hard state, authoritative CURRENT without fallback, peak reservation and InstallSnapshot within M13. Design contract clarified; general writer/codec/install/snapshot recovery unimplemented and T-015/T-016/T-026 unvalidated. C-01 design RESOLVED, C-02 extension and C-03 snapshot CONDITIONALLY READY; no OS/power-durability acceptance. M13 NOT STARTED.

## M13 Phase A isolated conformance — 22 November 2026

[Phase A](../milestone-m13-preliminary.md) implements QSN1/payload v1 and owner-only KV/session copies with v2 binding and fresh/isolated-only restore. No added version/migration; earlier vectors unchanged. Confirm separate worker control rather than indexed commands, B drain/coherent proof before copy, encoding after Release. Isolated C-01 conformance verified; no acceptance of runtime generations, InstallSnapshot, compaction or crash consistency. Complete C-02/C-03 and T-015/T-016/T-026 remain open.

## M13 Phase B isolated persistence — 23 November 2026

[Phase B](../milestone-m13-persistence.md) verifies isolated QSN1/CURRENT build/publication/recovery, updated final vote and complete v2 sessions. Existing format/protocol retained without third manifest or migration. New isolated C-02 evidence VERIFIED; integration C-02/C-03 CONDITIONALLY READY. Cleanup, runtime switch, suffix replay, compaction and InstallSnapshot unimplemented; full T-015/T-016/T-026 open, no OS/power claim.

## M13 Phase C runtime recovery — 23 November 2026

[Phase C](../milestone-m13-runtime-recovery.md) adopts base-S recovery and committed-suffix-only replay on fresh KV/sessions before listeners, through explicit v2 capability and continuous LOCK. Final-WAL term/vote, A=L=C cursors, fresh initial epoch, no historical result ring. Default base-zero path unchanged; no new format/configuration or migration. C-02 runtime recovery verified only within its subset; C-02/C-03 remain CONDITIONALLY READY for live cycle, compaction/installation and epochs. Full T-015/T-016/T-026 open, no OS/power guarantee.

## M13 Phase D isolated compaction — 23 November 2026

[Phase D](../milestone-m13-compaction-preliminary.md) adopts the existing representation without new fields: QSN1 snapshot/base-S WAL, final hard state, complete suffix and CURRENT v1 with v2 identity. The new assignment permits non-destructive isolated verification; no further format decision needed. Capture derives from actual apply; KV/sessions/indexes/suffix compared before/after B/C publication. Non-repairing reader only for D proof; ordinary behavior unchanged. Overall C-02/C-03 CONDITIONALLY READY and T-015/T-016/T-026 open. Equivalence alone authorizes no live activation, GC, deletion or InstallSnapshot.
