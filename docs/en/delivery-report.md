# Delivery report and inventory

[Index](README.md) · [Italiano](../it/delivery-report.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Initial 0.1 outcome and architecture

Created **96 Markdown files**: bilingual root README, bilingual docs index and **47 files per language**. Both versions contain complete specifications, 32 requirements, 32 acceptance criteria, 32 planned tests, 16 failure scenarios, 16 milestones, nine demo experiments, nine ADRs and nine detailed diagrams per language. Indexes and reports are included in the 47 files.

Baseline: one static group of three Go processes on loopback, HTTP/JSON, event-driven Raft core, deterministic state machine, synchronous WAL, local commit checkpoint, generation snapshots and client sessions without eviction. Quorum 2/3, logged GET, atomic CAS on whole values. Additional budget €0, offline runtime after tool bootstrap, explicit resource caps.

## Decisions, risks and open questions

The nine decisions are in the [ADR register](adr/README.md), all Proposed. Main risks are Raft bugs, premature acknowledgments, incomplete snapshots, an incorrect checker and language divergence: [risk register](risks-and-tradeoffs.md). Seven open questions and their gates are in the [review](design-review.md); they include macOS sync, power loss, resource baseline, licensing and format fixtures.

## Checks and limitations

Checked inventory and language parity, relative links, identifier references, matrix coverage, illustrative JSON and fenced delimiters. Manually reviewed Raft semantics, API, dedup, snapshots, faults, quotas and diagrams. Accessed sources are in [references](references.md). Mermaid was not rendered; no database tests were run and no performance measurements were invented. Translations received semantic review without external certification.

**No application code was written.** No Go files, module, CLI, service or runtime dependency was created; no deployment, heavy installation or benchmark. The pre-existing .DS_Store file was not changed. Temporary authoring/checking tools are outside the repository and are not part of the database.

## Recommended first milestone

After separate documentation approval: M01, KV state machine and sequential model with semantics/quota fixtures. Do not automatically start Raft, networking or infrastructure. See the [roadmap](implementation-roadmap.md).

## Update after 0.2 audit

Current inventory: **98 Markdown files**, 48 per language plus two shared files; added [architecture-review.md](architecture-review.md). Technical outcome: **APPROVED WITH CONDITIONS**: only M01–M02 now, explicit gates before storage/cluster work. The audit lists all changes and current checks; initial delivery figures describe the historical baseline.

## Complete inventory

Shared files: [root README](../../README.md), [documentation index](../README.md). Every file below exists at the same relative path in Italian, accessible through the link at the top of each document.

- [README.md](README.md)
- [acceptance-criteria.md](acceptance-criteria.md)
- [adr/ADR-001-use-go.md](adr/ADR-001-use-go.md)
- [adr/ADR-002-static-three-node-cluster.md](adr/ADR-002-static-three-node-cluster.md)
- [adr/ADR-003-raft-consensus.md](adr/ADR-003-raft-consensus.md)
- [adr/ADR-004-network-transport.md](adr/ADR-004-network-transport.md)
- [adr/ADR-005-persistence-strategy.md](adr/ADR-005-persistence-strategy.md)
- [adr/ADR-006-linearizable-reads.md](adr/ADR-006-linearizable-reads.md)
- [adr/ADR-007-client-retry-semantics.md](adr/ADR-007-client-retry-semantics.md)
- [adr/ADR-008-fault-injection.md](adr/ADR-008-fault-injection.md)
- [adr/ADR-009-offline-first-development.md](adr/ADR-009-offline-first-development.md)
- [adr/README.md](adr/README.md)
- [api-specification.md](api-specification.md)
- [architecture.md](architecture.md)
- [architecture-review.md](architecture-review.md)
- [consistency-model.md](consistency-model.md)
- [data-model.md](data-model.md)
- [delivery-report.md](delivery-report.md)
- [demo-and-evaluation.md](demo-and-evaluation.md)
- [design-review.md](design-review.md)
- [development-guide.md](development-guide.md)
- [diagrams/01-system.md](diagrams/01-system.md)
- [diagrams/02-node.md](diagrams/02-node.md)
- [diagrams/03-write.md](diagrams/03-write.md)
- [diagrams/04-read.md](diagrams/04-read.md)
- [diagrams/05-election.md](diagrams/05-election.md)
- [diagrams/06-replication.md](diagrams/06-replication.md)
- [diagrams/07-recovery.md](diagrams/07-recovery.md)
- [diagrams/08-partition.md](diagrams/08-partition.md)
- [diagrams/09-snapshot.md](diagrams/09-snapshot.md)
- [diagrams/README.md](diagrams/README.md)
- [failure-model.md](failure-model.md)
- [fault-injection.md](fault-injection.md)
- [glossary.md](glossary.md)
- [implementation-roadmap.md](implementation-roadmap.md)
- [network-protocol.md](network-protocol.md)
- [observability.md](observability.md)
- [overview.md](overview.md)
- [performance-and-resources.md](performance-and-resources.md)
- [raft-consensus.md](raft-consensus.md)
- [references.md](references.md)
- [requirements.md](requirements.md)
- [risks-and-tradeoffs.md](risks-and-tradeoffs.md)
- [scope.md](scope.md)
- [security.md](security.md)
- [storage-and-recovery.md](storage-and-recovery.md)
- [testing-strategy.md](testing-strategy.md)
- [traceability.md](traceability.md)
