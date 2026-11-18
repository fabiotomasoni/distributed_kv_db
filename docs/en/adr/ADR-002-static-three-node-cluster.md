# Three static members

[Index](../README.md) · [Italiano](../../it/adr/ADR-002-static-three-node-cluster.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-002 — Three static members**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

One Mac must represent an observable distributed system.

## Problem

Balance a nontrivial quorum with bounded resources.

## Alternatives considered

One node; three processes; five processes; three physical machines.

## Decision

One statically configured n1/n2/n3 group on loopback, quorum 2.

## Rationale

Three is the minimum for majority consensus tolerating one crash.

## Consequences

Immutable identities/config; no dynamic membership.

## Drawbacks and limitations

Host, power and SSD share one failure domain.

## Linked requirements

FR-001, FR-002, NFR-004. See the [matrix](../traceability.md).

## Validation and open questions

M04/M14: verify configuration and local failover. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../scope.md); [sources REF-01/02](../references.md); [roadmap](../implementation-roadmap.md).
