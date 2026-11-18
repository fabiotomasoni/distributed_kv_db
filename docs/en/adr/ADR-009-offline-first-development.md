# Offline development and zero additional cost

[Index](../README.md) · [Italiano](../../it/adr/ADR-009-offline-first-development.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-009 — Offline development and zero additional cost**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

€0 budget, one already-owned computer.

## Problem

Remove operational dependencies on accounts, cloud or metered billing.

## Alternatives considered

Cloud free tiers/trials; local containers; three native processes with free tools.

## Decision

Go standard library, filesystem and loopback; acquire dependencies once, run/test offline.

## Rationale

Meets objectives while avoiding charges and unnecessary infrastructure use.

## Consequences

Optional GitHub Free, resource caps and jointly maintained bilingual documents.

## Drawbacks and limitations

Initial bootstrap requires tool acquisition; no physical host independence.

## Linked requirements

NFR-007, NFR-011, NFR-012. See the [matrix](../traceability.md).

## Validation and open questions

M14/M16: demo without external networking and license/cost inventory. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../development-guide.md); [sources REF-04/10/11](../references.md); [roadmap](../implementation-roadmap.md).
