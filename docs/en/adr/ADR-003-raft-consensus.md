# Implement Raft directly

[Index](../README.md) · [Italiano](../../it/adr/ADR-003-raft-consensus.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-003 — Implement Raft directly**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

Replicas must order commands without assuming reliable networking.

## Problem

Preserve safety across crashes, partitions and competing elections.

## Alternatives considered

Custom Raft; existing Raft library; primary-backup without consensus; Paxos.

## Decision

Event-driven Raft core, persistent voting, current-term commit and static group.

## Rationale

Makes portfolio properties explicit; a ready-made library would violate the purpose.

## Consequences

Specify INV-01–06 and test every transition.

## Drawbacks and limitations

High implementation risk; naming Raft is not evidence of correctness.

## Linked requirements

FR-004, FR-005, FR-006, FR-007, FR-008, NFR-001. See the [matrix](../traceability.md).

## Validation and open questions

M05–M07/M14: mandatory review and safety tests. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../raft-consensus.md); [sources REF-01/02](../references.md); [roadmap](../implementation-roadmap.md).

## Audit 0.2 reassessment

Confirmed current-term commit and persistent voting. Corrected apply scheduling: scheduledApplyIndex distinct from lastApplied, bounded queues and applyEpoch for snapshot installation. An asynchronous core cannot use a completion cursor as its enqueue cursor. Required validation: T-008/apply-backlog and T-021 with a committed ghost register. Proposed status unchanged.

## M02 implementation evidence

19 November 2026: [M02](../milestone-m02.md) implements only the local owner/applier boundary: contiguous caller-supplied indices, distinct Scheduled/LastApplied, bounded admission and single-worker execution. T-008/M02 Passed, including apply backlog. Raft, commit proof, applyEpoch/snapshot replacement and T-021 remain future gates; this addendum does not mark Raft Accepted or erase the audit.
