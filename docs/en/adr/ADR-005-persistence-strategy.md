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
