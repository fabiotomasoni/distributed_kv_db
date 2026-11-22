# Reads through logged barriers

[Index](../README.md) · [Italiano](../../it/adr/ADR-006-linearizable-reads.md)

**22 November 2026 — M10 logged-read baseline implemented and checked; broader M14 verification remains open.**

## Identifier and status

**ADR-006 — Reads through logged barriers**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

An old leader may stay isolated and believe itself valid.

## Problem

Avoid stale reads without leases or synchronized clocks.

## Alternatives considered

Local read; lease; ReadIndex; logged read entry.

## Decision

GET proposes a current-term read and samples state at its committed apply slot.

## Rationale

Uses one quorum/persist/apply path, initially simpler to verify.

## Consequences

No minority GET; quorum and application are checked for each read.

## Drawbacks and limitations

Every read writes WAL; ReadIndex only after a baseline and new review.

## Linked requirements

FR-010, FR-019, NFR-002. See the [matrix](../traceability.md).

## Validation and open questions

M10/M14: isolated-leader and concurrent-history tests; M15 measures I/O cost. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../consistency-model.md); [sources REF-01/05](../references.md); [roadmap](../implementation-roadmap.md).

## Audit 0.2 reassessment

Retained logged GET for v1 after explicit comparison of correctness, latency, entries, Sync and risk. Future ReadIndex requires current-term commit, a new round after request, term/generation context and apply>=R; update RPC schema and tests before enabling it. The minority cannot complete GETs started after total isolation; responses for already-committed operations may arrive after the fault. [Full comparison and migration path](../consistency-model.md).

## M10 implementation decision — 22 November 2026

[M10](../milestone-m10.md) adopts the selected logged GET for the implemented baseline. Each attempt persists a fresh current-term read, waits for legal durable majority commit/checkpoint and captures the result at exact M02 application before owner-serialized leadership authorization. Strict HTTP/fixtures, exact-slot gated apply, seeded old-leader isolation and small independent histories pass; post-crash HTTP reads also pass. This accepts the M10 mechanism within finite evidence, not a universal proof. ReadIndex/leases remain absent and require the previously stated future gates. Existing canonical read encoding is unchanged; homogeneous startup capability preserves legacy commit-only tests without rolling negotiation.
