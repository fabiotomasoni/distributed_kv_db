# Traceability matrix

[Index](README.md) · [Italiano](../it/traceability.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Normative matrix

Each row has one primary criterion and test. Cross-cutting tests in [testing](testing-strategy.md) extend coverage. All rows are open.

| Requirement | Module | ADR | Milestone | Criterion | Test |
|---|---|---|---|---|---|
| [FR-001](requirements.md) | `config` | [ADR-001](adr/README.md) | [M04](implementation-roadmap.md) | [AC-001](acceptance-criteria.md) | [T-001](testing-strategy.md) |
| [FR-002](requirements.md) | `config` | [ADR-002](adr/README.md) | [M04](implementation-roadmap.md) | [AC-002](acceptance-criteria.md) | [T-002](testing-strategy.md) |
| [FR-003](requirements.md) | `transport` | [ADR-004](adr/README.md) | [M04](implementation-roadmap.md) | [AC-003](acceptance-criteria.md) | [T-003](testing-strategy.md) |
| [FR-004](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M05](implementation-roadmap.md) | [AC-004](acceptance-criteria.md) | [T-004](testing-strategy.md) |
| [FR-005](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M05](implementation-roadmap.md) | [AC-005](acceptance-criteria.md) | [T-005](testing-strategy.md) |
| [FR-006](requirements.md) | `raft-log` | [ADR-003](adr/README.md) | [M06](implementation-roadmap.md) | [AC-006](acceptance-criteria.md) | [T-006](testing-strategy.md) |
| [FR-007](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M07](implementation-roadmap.md) | [AC-007](acceptance-criteria.md) | [T-007](testing-strategy.md) |
| [FR-008](requirements.md) | `state-machine` | [ADR-003](adr/README.md) | [M07](implementation-roadmap.md) | [AC-008](acceptance-criteria.md) | [T-008](testing-strategy.md) |
| [FR-009](requirements.md) | `api` | [ADR-004](adr/README.md) | [M10](implementation-roadmap.md) | [AC-009](acceptance-criteria.md) | [T-009](testing-strategy.md) |
| [FR-010](requirements.md) | `state-machine` | [ADR-006](adr/README.md) | [M11](implementation-roadmap.md) | [AC-010](acceptance-criteria.md) | [T-010](testing-strategy.md) |
| [FR-011](requirements.md) | `sessions` | [ADR-007](adr/README.md) | [M11](implementation-roadmap.md) | [AC-011](acceptance-criteria.md) | [T-011](testing-strategy.md) |
| [FR-012](requirements.md) | `api` | [ADR-007](adr/README.md) | [M11](implementation-roadmap.md) | [AC-012](acceptance-criteria.md) | [T-012](testing-strategy.md) |
| [FR-013](requirements.md) | `storage` | [ADR-005](adr/README.md) | [M03](implementation-roadmap.md) | [AC-013](acceptance-criteria.md) | [T-013](testing-strategy.md) |
| [FR-014](requirements.md) | `recovery` | [ADR-005](adr/README.md) | [M09](implementation-roadmap.md) | [AC-014](acceptance-criteria.md) | [T-014](testing-strategy.md) |
| [FR-015](requirements.md) | `snapshot` | [ADR-005](adr/README.md) | [M13](implementation-roadmap.md) | [AC-015](acceptance-criteria.md) | [T-015](testing-strategy.md) |
| [FR-016](requirements.md) | `snapshot` | [ADR-005](adr/README.md) | [M13](implementation-roadmap.md) | [AC-016](acceptance-criteria.md) | [T-016](testing-strategy.md) |
| [FR-017](requirements.md) | `metrics` | [ADR-008](adr/README.md) | [M12](implementation-roadmap.md) | [AC-017](acceptance-criteria.md) | [T-017](testing-strategy.md) |
| [FR-018](requirements.md) | `node` | [ADR-005](adr/README.md) | [M09](implementation-roadmap.md) | [AC-018](acceptance-criteria.md) | [T-018](testing-strategy.md) |
| [FR-019](requirements.md) | `raft/api` | [ADR-006](adr/README.md) | [M10](implementation-roadmap.md) | [AC-019](acceptance-criteria.md) | [T-019](testing-strategy.md) |
| [FR-020](requirements.md) | `faults` | [ADR-008](adr/README.md) | [M12](implementation-roadmap.md) | [AC-020](acceptance-criteria.md) | [T-020](testing-strategy.md) |
| [NFR-001](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M14](implementation-roadmap.md) | [AC-021](acceptance-criteria.md) | [T-021](testing-strategy.md) |
| [NFR-002](requirements.md) | `api` | [ADR-006](adr/README.md) | [M14](implementation-roadmap.md) | [AC-022](acceptance-criteria.md) | [T-022](testing-strategy.md) |
| [NFR-003](requirements.md) | `storage` | [ADR-005](adr/README.md) | [M14](implementation-roadmap.md) | [AC-023](acceptance-criteria.md) | [T-023](testing-strategy.md) |
| [NFR-004](requirements.md) | `raft` | [ADR-002](adr/README.md) | [M14](implementation-roadmap.md) | [AC-024](acceptance-criteria.md) | [T-024](testing-strategy.md) |
| [NFR-005](requirements.md) | `metrics` | [ADR-004](adr/README.md) | [M15](implementation-roadmap.md) | [AC-025](acceptance-criteria.md) | [T-025](testing-strategy.md) |
| [NFR-006](requirements.md) | `node/storage` | [ADR-005](adr/README.md) | [M15](implementation-roadmap.md) | [AC-026](acceptance-criteria.md) | [T-026](testing-strategy.md) |
| [NFR-007](requirements.md) | `docs` | [ADR-009](adr/README.md) | [M16](implementation-roadmap.md) | [AC-027](acceptance-criteria.md) | [T-027](testing-strategy.md) |
| [NFR-008](requirements.md) | `faults` | [ADR-008](adr/README.md) | [M12](implementation-roadmap.md) | [AC-028](acceptance-criteria.md) | [T-028](testing-strategy.md) |
| [NFR-009](requirements.md) | `build` | [ADR-001](adr/README.md) | [M04](implementation-roadmap.md) | [AC-029](acceptance-criteria.md) | [T-029](testing-strategy.md) |
| [NFR-010](requirements.md) | `config/api` | [ADR-004](adr/README.md) | [M14](implementation-roadmap.md) | [AC-030](acceptance-criteria.md) | [T-030](testing-strategy.md) |
| [NFR-011](requirements.md) | `build` | [ADR-009](adr/README.md) | [M14](implementation-roadmap.md) | [AC-031](acceptance-criteria.md) | [T-031](testing-strategy.md) |
| [NFR-012](requirements.md) | `build` | [ADR-009](adr/README.md) | [M16](implementation-roadmap.md) | [AC-032](acceptance-criteria.md) | [T-032](testing-strategy.md) |

## Incremental coverage and audit

The matrix milestone is the final requirement gate; NFR-007 is checked at every change and closes with the M16 package. Mandatory [acceptance](acceptance-criteria.md) subcases belong to existing primary tests without artificially increasing the requirement count. The [audit](architecture-review.md) links findings, fixes and gates; an unexecuted test remains open.
