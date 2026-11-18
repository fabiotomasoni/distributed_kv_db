# Engineering documentation

[Index](README.md) · [Italiano](../it/README.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## How to read the design

QuorumKV is in the design phase. All described servers, commands, tests and results are future work except explicitly recorded documentation checks. Do not execute startup examples as if software were available.

Quick path: overview → requirements → architecture → API. Safety review: Raft → consistency → storage → failures → testing. Planning: roadmap → criteria → matrix. The recommended first implementation is M01; it requires separate documentation approval.

## Complete specifications

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
