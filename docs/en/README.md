# Engineering documentation

[Index](README.md) · [Italiano](../it/README.md)

**Updated: 19 November 2026 — local M01/M02 implemented and checked; distributed specifications remain design work.**

## How to read the design

M01 is implemented and checked: [local state machine and results](milestone-m01.md). M02 is completed: [ordered applier, lifecycle and tests](milestone-m02.md). M03 and later milestones remain unimplemented. Distributed specifications and cluster startup commands are still proposals; reports dated 18 November document the previous phase.

Quick path: overview → requirements → architecture → API. Safety review: Raft → consistency → storage → failures → testing. Planning: roadmap → criteria → matrix. M01/M02 are completed; M03 remains subject to review and storage gates.

## Complete specifications

- [M01 completed: interfaces, tests and evidence](milestone-m01.md)
- [M02 completed: ordered applier, queues and shutdown](milestone-m02.md)
- [Architecture audit: APPROVED WITH CONDITIONS](architecture-review.md)
- [QuorumKV overview](overview.md)
- [Scope, assumptions and limitations](scope.md)
- [Verifiable requirements](requirements.md)
- [Traceability matrix](traceability.md)
- [Architecture and concurrency](architecture.md)
- [Data model and transitions](data-model.md)
- [Client API v1 specification](api-specification.md)
- [Peer protocol and transport](network-protocol.md)
- [Raft core specification](raft-consensus.md)
- [Consistency and linearizability](consistency-model.md)
- [Persistence, snapshots and crash recovery](storage-and-recovery.md)
- [Failure model and scenarios](failure-model.md)
- [Testing strategy and catalog](testing-strategy.md)
- [Objective acceptance criteria](acceptance-criteria.md)
- [Reproducible fault injection](fault-injection.md)
- [Logs, metrics and diagnostics](observability.md)
- [Performance and resource protection](performance-and-resources.md)
- [Local security model](security.md)
- [Future development guide](development-guide.md)
- [Implementation roadmap](implementation-roadmap.md)
- [Risks and trade-offs](risks-and-tradeoffs.md)
- [Technical demo and evaluation](demo-and-evaluation.md)
- [Bilingual glossary](glossary.md)
- [Primary references and verification status](references.md)
- [Design review and open questions](design-review.md)
- [Delivery report and inventory](delivery-report.md)
- [Architecture decisions: nine ADRs](adr/README.md)
- [Nine Mermaid diagrams](diagrams/README.md)

## Maintenance

Update both languages in the same change. Keep filenames, IDs, formulas and technical examples aligned; never reuse retired IDs. Requirements and criteria are normative; diagrams and examples are illustrative. A changed decision updates its ADR, API, tests and matrix before changing advertised guarantees.
