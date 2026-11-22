# Traceability matrix

[Index](README.md) · [Italiano](../it/traceability.md)

**Updated: 20 November 2026 — local M01/M02/M03 implemented and checked; distributed specifications remain design work.**

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

## M01 subset progress

T-008/M01 and T-009/M01 are Passed for the local sequential core; see [tests, files and evidence](milestone-m01.md). FR-008/FR-009/FR-010 and NFR-006 have local-component evidence only: the matrix retains original final gates for consensus, API, concurrency and complete resources. M02 local evidence follows.

## M02 subset progress

M02 COMPLETED: T-008/M02 (ordered single ownership, no duplicate scheduling, bounded queue/backpressure) and T-018/M02 (worker shutdown/cancellation) Passed; T-028 fake foundations present. [Evidence and limitations](milestone-m02.md). FR-008/FR-018 and NFR-008 have local subset evidence only. The primary matrix still points to full AC-008 at M07, AC-018 at M09 and AC-028 at M12; none is closed early.

## Storage gate evidence before M03

C-01/OQ-05: [fixed schemas and static vectors](canonical-formats.md), design prerequisite resolved; future T-013/T-015/T-016 conformance still required. C-02/OQ-01: [documented candidate and missing evidence](macos-storage-contract.md), conditional choice, implementation validation open. [Review](pre-m03-review.md) links FR-013–FR-016 and NFR-003/NFR-006 to these decisions; original final matrix gates and all full ACs are retained.

## Subsequent preliminary M03 evidence

[Phase report](milestone-m03-preliminary.md): GATE C-02 VERIFIED for local primitives, M03 PARTIALLY COMPLETED. 7 storageformat tests/18 subcases and 13 storagefs test functions/16 subcases pass; targeted race and unchanged M01/M02 regressions pass. Codec scope is stateless frames/entries/digests/manifests; no WAL/replay engine or complete AC-013/AC-014/AC-023 closure. Full snapshot and later gates remain open. Earlier figures/statuses describe their historical phase.

## Local M03 closure

[Final evidence](milestone-m03.md): M03 COMPLETED; local T-013/storage AC-013 portion Passed. WAL has 17 top-level functions/25 subcases, 82.9% local statement coverage; M01/M02, codec/adapter regressions and targeted WAL/adapter/applier race checks pass. All 18 canonical recovery expectations covered at the appropriate byte/namespace boundary. Full FR-013 Raft-response integration, AC-014 server restart and AC-023 end-to-end remain dependent on later milestones; AC-015/AC-016 snapshot acceptance is M13. Coverage/race/process tests do not certify physical-fault resilience. No primary requirement/AC ID or final distributed gate removed.

## M04 implementation mapping

FR-001 → internal/node + wal identity/LOCK → AC-001/T-001. FR-002 → internal/config + configuration-v1.json + peer envelope → AC-002/T-002. FR-003 → internal/strictjson + peerprotocol + transport + peer-v1.json → AC-003/T-003. NFR-009 → native darwin/arm64 build and bounded suite → AC-029/T-029. [M04 evidence](milestone-m04.md) records results/limits; no later requirement is closed by these tests.

## M05 implementation mapping

FR-004 → internal/raft Core + M03 Append/Durable barrier → AC-004/T-004. FR-005 → injected Tick/Draw and empty AppendEntries + Service/transport → AC-005/T-005. INV-01/INV-05/INV-06 election evidence → independent persistent vote/quorum/leader oracle; NFR-008 finite replay → election-v1-results.json. [M05 report](milestone-m05.md) preserves NFR-001 later full safety gate and all M06+ obligations; no app/client requirement is closed.

## M06 implementation mapping

FR-006 → internal/raft/replication.go + existing WAL replace_suffix + transport/service → AC-006/T-006. Sent/Failed/Response and copied progress establish actual-request correlation; WAL frame cuts and seeded disk-view oracle cover known prefix preservation/recovery. [M06 evidence](milestone-m06.md) preserves all later FR-007/FR-008 gates: no quorum commit, checkpoint advancement or apply.

## M07 implementation mapping

FR-007 → internal/raft/commit.go + M06 proof correlation + M03 checkpoint → AC-007/T-007. FR-008 → core cursors + application.go + internal/committed + unchanged M02/M01 → AC-008/T-008. C-03 M07 → gated backlog and permanent ghost oracle. [M07 evidence](milestone-m07.md) retains M08 API/OpenAPI, M09 complete server recovery, M10 read, M11 session/dedup, M13 snapshot and final NFR-001/history gates.

## M08 phase evidence

[M08 report](milestone-m08.md): FR-009/FR-012, AC-009/AC-012 and T-009/T-012 pass only for the roadmap's PUT/DELETE, strict HTTP validation, deadlines, unknown/not_admitted and OpenAPI subset. Tests include gated disk/apply cancellation, observed higher-term loss, bounded admission with separate peer capacity, real three-node concurrent writes/election/minority, lost response without automatic retry and repeated DELETE with a new result. The schema/HTTP fixture checker includes negative controls and runtime canonical extensions. M01–M07 regressions and targeted race pass. Full GET AC-009 remains M10; dedup/failover retry AC-012 remains M11, with snapshots/history checks at later gates. C-01's M08 subset artifact/fixture obligation is satisfied; full v1 stability is not claimed. C-02 and later C-03 obligations retain their original bounds.

## M09 internal MVP evidence

[M09](milestone-m09.md) satisfies FR-014/AC-014/T-014 and FR-018/AC-018/T-018, plus the NFR-003/AC-023/T-023 roadmap subset. New controlled startup test checks follower-only replay at commit 1 with suffix 2, then legal new-term noop application. Real child-process tests kill one follower, kill the leader, kill all, restart intact storage, compare acknowledged bytes/actual replayed values and resume PUT/DELETE. Pending-write SIGTERM checks unknown, suffix exclusion and resource release. CLI/error tests cover bounded/redacted input, missing initialization, duplicate lock, repeated bootstrap, corruption, pre-start cancellation and example configuration coherence. Previous M01–M08 regression and race evidence is rerun. The tests use one temporary three-node cluster at a time, bounded waits and small data; no benchmark or physical fault. AC-023's HTTP GET variant stays at M10, snapshots at M13, complete checker at M14. Existing M08 OpenAPI/fixtures and C-01/C-02/C-03 later gates are unchanged.
