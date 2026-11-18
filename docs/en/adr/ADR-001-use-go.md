# Use Go and the standard library

[Index](../README.md) · [Italiano](../../it/adr/ADR-001-use-go.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-001 — Use Go and the standard library**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

One student must handle concurrency, networking and tests with no budget.

## Problem

Reduce toolchains and dependencies without hiding Raft.

## Alternatives considered

Go; Rust for static ownership; C++ for memory control.

## Decision

Go darwin/arm64, baseline 1.27.4, no Raft library or external runtime.

## Rationale

HTTP, JSON, testing, logging and checksums are available in the standard library.

## Consequences

Small packages, event-driven core and local testing without containers.

## Drawbacks and limitations

GC and scheduling need measurement; a mutex does not prove distributed correctness.

## Linked requirements

NFR-009, NFR-012. See the [matrix](../traceability.md).

## Validation and open questions

M01/M04: verify toolchain and ARM64 race detector. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../development-guide.md); [sources REF-04/06/07/10/11](../references.md); [roadmap](../implementation-roadmap.md).
