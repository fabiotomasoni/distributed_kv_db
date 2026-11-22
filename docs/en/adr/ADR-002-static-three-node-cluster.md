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

## M04 implementation evidence — 20 November 2026

[M04](../milestone-m04.md) implements this decision through immutable [configuration/hash](../configuration.md), strict protocol fixtures and bounded standard-library loopback HTTP. Storage identity/LOCK precede production bind; shutdown drains before release. No DNS/proxy/redirect/compression or external dependency. One outgoing RPC per peer, bounded sockets/deadlines. Callback decisions and future Raft timing/replication remain outside M04. The original design rationale and trusted-loopback limitations remain unchanged.

## M09 executable three-process MVP

[M09](../milestone-m09.md) runs the existing static n1/n2/n3 configurations as separate local processes, using unchanged quorum 2 and distinct ports/private directories. Kill/restart/failover tests preserve acknowledged state under the stated storage assumptions. No membership change, special bootstrap leader, physical independence or shared-host-failure availability is added.
