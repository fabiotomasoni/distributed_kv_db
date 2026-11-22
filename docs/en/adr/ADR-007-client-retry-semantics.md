# Client sequences and bounded deduplication

[Index](../README.md) · [Italiano](../../it/adr/ADR-007-client-retry-semantics.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-007 — Client sequences and bounded deduplication**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

A lost response does not reveal whether a mutation executed.

## Problem

Avoid duplicate effects without an unbounded table or clock-based TTL.

## Alternatives considered

No dedup; UUIDs with TTL; sequential sessions retaining the latest result.

## Decision

Persistent client ID, one pending request, monotonic sequence; 1,024 sessions without eviction.

## Rationale

Bounded deterministic state; high-water mark prevents historical reapplication.

## Consequences

Session results replicated and snapshotted; quota and failed CAS consume sequence.

## Drawbacks and limitations

Only latest result recoverable; session cap may block new clients; no exactly-once delivery.

## Linked requirements

FR-011, FR-012. See the [matrix](../traceability.md).

## Validation and open questions

M11/M13: client/server crashes, identity conflicts and retries after snapshot. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../data-model.md); [sources REF-01/05](../references.md); [roadmap](../implementation-roadmap.md).

## Audit 0.2 reassessment

Confirmed sequential sessions without TTL and persistent latest result. The checker closes the logical interval at the earliest definitive response and never extends it for later retries; identity conflicts are separate protocol observations. No retry justifies more than one logical effect. Validate T-011/T-012/T-022 including after snapshots.

## Canonical digest clarification — 19 November 2026

[Canonical formats](../canonical-formats.md) now fixes the versioned typed digest preimage and cached-result/session snapshot schema. Identity/sequence are compared separately; the digest covers opcode and typed operation arguments, excluding transport metadata. Absent and present-empty expectations differ. This resolves storage-format ambiguity, not session implementation or T-011/T-012 acceptance.

## M08 boundary — no sessions yet

[M08](../milestone-m08.md) validates/persists client_id and sequence but deliberately implements no deduplication before M11. The test client issues one attempt, never automatically retries unknown and validates configured leader hints without following redirects. Repeated identical DELETE can return deleted=true then false at new indices; no original-result or at-most-once guarantee. The design decision above remains required for M11/M13, not a current promise. Timeout/disconnect cannot undo a possible committed effect.

## M11 implementation — 22 November 2026

[M11](../milestone-m11.md) implements the selected sequence/digest/latest-result policy in the sole M02-owned wrapper, reconstructed from the committed WAL prefix on every replica/restart. Identical latest retries preserve result/index; conflicts/stale/gaps do not advance valid session state. Failed CAS/logical quota results are cached, max 1024 IDs without TTL/eviction, no historic-result guarantee after advancing. Each retry crosses a fresh quorum/application/leadership fence; no follower/isolated cache shortcut. [ADR-010](ADR-010-session-profile.md) explicitly versions this application contract so old metadata-only logs are never reinterpreted silently. CAS/retry/failover/all-process recovery and bounded retention pass finite tests. General snapshot/session handoff and final checker remain M13/M14; the test client remains single-attempt and caller-managed identities, not a persistent production client CLI.
