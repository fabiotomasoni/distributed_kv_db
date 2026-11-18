# Local deterministic fault injection

[Index](../README.md) · [Italiano](../../it/adr/ADR-008-fault-injection.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Identifier and status

**ADR-008 — Local deterministic fault injection**. Status **Proposed**, 18 November 2026. Design decision to confirm at the stated gates, not implementation evidence.

## Context

Rare faults are not reliably reproduced through random stress.

## Problem

Explore edge cases while keeping tests short on the Mac.

## Alternatives considered

Random sleeps; external proxies/VMs; in-process simulator/adapters and local harness.

## Decision

Virtual clock/network, cutpoint storage, seed+trace; real processes for integration.

## Rationale

Allows replay and counterexample reduction without infrastructure or massive load.

## Consequences

Faults normally disabled, mandatory manifests and per-run cleanup.

## Drawbacks and limitations

The model may be wrong; seed does not determine OS scheduling.

## Linked requirements

FR-017, FR-020, NFR-008. See the [matrix](../traceability.md).

## Validation and open questions

M12/M14: identical simulation replay and complementary real cases. If gates fail, update or supersede this ADR in both languages; retain the ID and decision history.

## References

[Related specification](../fault-injection.md); [sources REF-01/07/09](../references.md); [roadmap](../implementation-roadmap.md).

## Audit 0.2 reassessment

Separated ProcessCrash and SystemCrashModel: unsynchronized writes may survive, and process kill does not clear the kernel cache. Cutpoints cover complete/partial frames and visible renames. Fake foundations at M02–M05, full framework at M12. No simulator proves real power-loss behavior.
