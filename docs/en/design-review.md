# Design review and open questions

[Index](README.md) · [Italiano](../it/design-review.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Design review outcome

Documentation baseline 0.1 is ready for owner review. This phase remains documentation only: no Go code, CLI, server, deployment or benchmark. ADRs are Proposed. This review does not automatically approve implementation.

Cross-review checked: uniform 2/3 quorum; commit only through a current-term index; positive voting and replication after Sync; committed-only apply; GET through a new read entry sampled at its exact slot; retries with replicated sessions included in snapshots; compaction after durable publication; physical caps including temporary peak space. API examples use string indices like the peer protocol. The demo follower is restarted before crashing the leader so a majority remains.

## Open questions and gates

| ID | Question / current baseline | Required evidence / owner | Gate |
|---|---|---|---|
| OQ-01 | Directory sync, exclusive lock and rename on macOS ARM64 | Developer: platform documentation, recovery fixtures and process cutpoints; revise ADR-005 if primitives are insufficient. | M03, before durability claims |
| OQ-02 | APFS/SSD power-loss resilience unverified | Retain model-limited guarantee; no destructive tests on the personal Mac. Further guarantees require separate research and evidence. | No v1 power-loss claim |
| OQ-03 | 100/600–1000 ms timing and resource use are hypotheses | Developer: local baseline including Sync, RSS/CPU and peak disk; change configuration/documents uniformly if needed. | M05/M15 |
| OQ-04 | QuorumKV license unselected | Owner: explicit choice before public distribution. No invented LICENSE file in this delivery. | Before publication |
| OQ-05 | OpenAPI schema and binary/canonical fixtures remain to be produced | Developer: derive already-described contracts, validate examples and format through byte-for-byte fixtures. | M03/M04/M08 |
| OQ-06 | 1,024-session cap and latest response only | Intentional baseline, no eviction. Revisit only if planned demo/clients cannot comply; never introduce silent TTL. | M11 |
| OQ-07 | Uncertain 24-week plan at 6–8 hours/week | Owner/developer: reassess capacity after the first three milestones; drop optional work or extend the schedule. | M03 and each gate |

## Documentation checks performed

Local automated checks covered tree completeness/parity, relative Markdown links, balanced fenced blocks, JSON examples, identifiers and requirement/AC/test correspondence in the matrix. Nine Mermaid diagram pairs were checked for structure and sequence; the root README has the summary diagram. The [delivery report](delivery-report.md) contains the complete inventory.

Manual semantic review covered both languages, edge cases and relationships between API, Raft, storage and testing. Primary sources were accessed and dated in [references](references.md). Commands are explicitly future examples, measurements Not measured and criteria Not run.

## Verification limits

Automated checks prove neither semantic translation equivalence nor future algorithm correctness. Mermaid was reviewed as source and structure, **without graphic rendering or a dedicated Mermaid parser**: these tools were unavailable and no additional infrastructure was installed. No database tests, linearizability checker, race detector or benchmarks were executed. External links were checked for accessibility when accessed, without guaranteeing future availability.

## Conventions to preserve

Change both languages together; retired requirements retain IDs and rationale, never reuse them. API changes update tests, errors, ADRs and traces. Distinguish checked documentation, designed properties and tested properties. The recommended next step after approval is M01: sequential KV model, absence/empty/CAS fixtures and quotas, without networking or optimization.

## Historical record of the first 0.1 delivery

Local check on 18 November 2026: **96 Markdown files**, **47 language pairs**, **895 resolved relative links**, **14 valid JSON blocks**, **13 identical pairs of non-Mermaid technical examples**, **nine diagram pairs with matching structure**. Catalogs: 32 requirements, 32 ACs and 32 planned tests, all covered by the matrix. Structural checks: **zero errors detected**. This result concerns documents, not application tests.

## Subsequent 0.2 audit outcome

The initial structural check did not exclude semantic problems: the [architecture audit](architecture-review.md) records 11 findings and corrections, updating the judgment to **APPROVED WITH CONDITIONS**. Earlier check figures are historical, not a measurement of the current repository. OQ-01/OQ-05 gate durable storage/protocols; complete consensus remains subject to testing. Only M01–M02 are technically eligible to start now; this assignment starts no implementation.

## Current pre-M03 decisions

19 November 2026: [review and exact readiness boundary](pre-m03-review.md). OQ-05/C-01 storage schemas and byte fixtures resolved; M04 configuration/protocol and M08 OpenAPI still pending. OQ-01/C-02 platform sources and provisional adapter choices documented; actual directory/lock/rename/error validation remains open, first in isolated M03 tests. Only that limited conformance/capability work is conditionally eligible; storage integration waits for evidence, M03/M09 durability gates remain. No change to OQ-02/03/04/06/07 or authorization to implement M03 here.
