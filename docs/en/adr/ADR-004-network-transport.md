# Local HTTP/JSON transport

[Index](../README.md) · [Italiano](../../it/adr/ADR-004-network-transport.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-004 — Local HTTP/JSON transport**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

RPCs must be inspectable and work offline without generators.

## Problem

Define framing, errors and backpressure without excessive infrastructure.

## Alternatives considered

Standard HTTP/JSON; custom binary TCP; gRPC/protobuf.

## Decision

Loopback HTTP/1.1, strict JSON, v1 schema and one active RPC per peer.

## Rationale

Simplifies diagnosis and dependencies, sufficient for the small workload.

## Consequences

Explicit body/entry caps, decimal-string indices, deadlines and correlation.

## Drawbacks and limitations

Base64, parsing and lack of TLS do not meet public-use requirements.

## Linked requirements

FR-003, FR-009, NFR-005, NFR-010. See the [matrix](../traceability.md).

## Validation and open questions

M04/M08: interoperable fixtures and client OpenAPI before API stability. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../network-protocol.md); [sources REF-04/12](../references.md); [roadmap](../implementation-roadmap.md).

## Audit 0.2 reassessment

HTTP/JSON remains appropriate. Corrected client cap from 32 to 48 KiB for CAS with two maximum values; peer cap unchanged. Strict decoding requires duplicate-key scanning, UTF-8, exact field names and document-end checks beyond standard decoding. T-003/T-009 must exercise boundaries. No new external package.

## M04 implementation evidence — 20 November 2026

[M04](../milestone-m04.md) implements this decision through immutable [configuration/hash](../configuration.md), strict protocol fixtures and bounded standard-library loopback HTTP. Storage identity/LOCK precede production bind; shutdown drains before release. No DNS/proxy/redirect/compression or external dependency. One outgoing RPC per peer, bounded sockets/deadlines. Callback decisions and future Raft timing/replication remain outside M04. The original design rationale and trusted-loopback limitations remain unchanged.

## M08 implementation evidence — 21 November 2026

[M08](../milestone-m08.md) adds PUT/DELETE HTTP and an offline checked [OpenAPI subset](../../openapi/m08.json) with shared HTTP fixtures. Distinct configured peer/client ports require two standard-library servers sharing the M04 accepted-socket limiter; no independent RPC protocol. Client JSON cap remains 48 KiB. Configured-member hints only, no forwarding/redirect/proxy. C-01 is satisfied for the M08 subset; GET/CAS and final API stability remain later gates.
