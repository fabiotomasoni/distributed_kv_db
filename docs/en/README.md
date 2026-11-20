# Engineering documentation

[Index](README.md) · [Italiano](../it/README.md)

**Updated: 20 November 2026 — local M01/M02/M03/M04/M05/M06 implemented and checked; distributed specifications remain design work.**

## How to read the design

M01 is implemented and checked: [local state machine and results](milestone-m01.md). M02 is completed: [ordered applier, lifecycle and tests](milestone-m02.md). M03 is completed at the local WAL/log-recovery boundary: [results](milestone-m03.md). M04 is completed: [RPC/configuration evidence](milestone-m04.md). M05 is completed: [leader election evidence](milestone-m05.md). M06 is completed: [replication evidence](milestone-m06.md). M07 and later remain unimplemented. Distributed specifications and cluster startup commands are still proposals; reports dated 18 November document the previous phase.

Quick path: overview → requirements → architecture → API. Safety review: Raft → consistency → storage → failures → testing. Planning: roadmap → criteria → matrix. M01/M02/M03/M04/M05/M06 are completed locally; M07 needs a new assignment and its own gates.

## Complete specifications

- [M06 completed: replication, conflicts and resolved scope](milestone-m06.md)

- [M05 completed: persistent election and simulator](milestone-m05.md)

- [M04 completed: RPC/configuration and evidence](milestone-m04.md)
- [Static configuration v1 and canonical hash](configuration.md)

- [M03 completed: local WAL, recovery and evidence](milestone-m03.md)

- [Preliminary M03: C-02 VERIFIED, milestone partial](milestone-m03-preliminary.md)

- [Pre-M03 review: CONDITIONALLY READY](pre-m03-review.md)
- [Canonical storage formats v1 and fixtures](canonical-formats.md)
- [macOS storage contract and pending C-02 tests](macos-storage-contract.md)

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
