# ADR index

[Index](../README.md) · [Italiano](../../it/adr/README.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Decision register

All decisions are Proposed: a concrete baseline without pretending implementation has validated it. Assignment obligations (zero spending, bilingual documentation, no code in this phase) remain constraints rather than optional votes. Future acceptance requires rationale and a gate; replacement uses Superseded status and a new ADR rather than reusing the identifier.

| ID | Decision | Status |
|---|---|---|
| ADR-001 | [Use Go and the standard library](ADR-001-use-go.md) | Proposed |
| ADR-002 | [Three static members](ADR-002-static-three-node-cluster.md) | Proposed |
| ADR-003 | [Implement Raft directly](ADR-003-raft-consensus.md) | Proposed |
| ADR-004 | [Local HTTP/JSON transport](ADR-004-network-transport.md) | Proposed |
| ADR-005 | [Synchronous WAL and snapshot generations](ADR-005-persistence-strategy.md) | Proposed |
| ADR-006 | [Reads through logged barriers](ADR-006-linearizable-reads.md) | Proposed |
| ADR-007 | [Client sequences and bounded deduplication](ADR-007-client-retry-semantics.md) | Proposed |
| ADR-008 | [Local deterministic fault injection](ADR-008-fault-injection.md) | Proposed |
| ADR-009 | [Offline development and zero additional cost](ADR-009-offline-first-development.md) | Proposed |

- [ADR-010: versioned session application profile](ADR-010-session-profile.md)
