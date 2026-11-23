# M13 Phase C — Runtime snapshot and WAL suffix recovery

[Index](README.md) · [Italiano](../it/milestone-m13-runtime-recovery.md)

**23 November 2026 — RUNTIME RECOVERY PHASE COMPLETED. Overall M13: PARTIALLY COMPLETED. M14 NOT STARTED.** Clean Git baseline `c0e00d0`, with Phases A and B already recorded in history before the assignment. No commit performed in this phase.

## Assignment and initial review

The [roadmap](implementation-roadmap.md) assigns M13 snapshots and compaction: atomic generations, installation, crash fixtures, T-015/T-016/T-026 within their phase subset. M14 retains the independent checker and general property verification. The [pre-M13 decisions](pre-m13-review.md), [Phase A](milestone-m13-preliminary.md) and [Phase B](milestone-m13-persistence.md), in both languages, establish format, ownership, publication and recovery. ADR-005/ADR-010 and storage/commit/session contracts retained. Initially C-01/OQ-05 was RESOLVED with verified conformance; isolated C-02/OQ-01 VERIFIED but integration conditional; C-03 CONDITIONALLY READY.

Phase C authorizes isolated combined recovery first, followed by controlled startup and bounded cluster tests. It does not authorize automatic snapshot creation, live rotation, compaction, WAL prefix deletion, GC or InstallSnapshot. No format, criterion or election/replication algorithm changes are needed. No blocking contradictions were found.

## Components and files

Created `internal/snapshot/recovery.go`: RecoverCommitted and CombinedRecovery. New tests: `internal/snapshot/recovery_test.go`, `internal/raft/snapshot_recovery_test.go`, `internal/node/m13_test.go`, `internal/node/m13_process_test.go`.

Modified `internal/wal/open.go` and `store.go` for explicit v2 snapshot-base opening, validated image copying and continued append under the same WAL/LOCK; `internal/raft/commit.go`, `service.go`, `application.go` for startup capability and application handoff; `internal/node/node.go` for the explicit entry point. Updated only the package comment in `internal/snapshot/memory.go` to describe explicit recovery use. No Phase B writer/publication, canonical codec/fixture, filesystem adapter, HTTP API, public configuration or CLI changes.

Updated both languages' storage-and-recovery, architecture, raft-consensus, data-model, failure-model, testing-strategy, acceptance-criteria, traceability, development-guide, implementation-roadmap, ADR-005/ADR-010, indexes and root README; added these reports. Historical reports remain scoped to their respective phases.

## Isolated combined recovery algorithm

RecoverCommitted receives v2 configuration, canonical image and State from validated WAL Replay. It checks S/termS, base, hard state, vote, sequence, commit, last index, entry order/continuity/terms and correspondence to their canonical bytes. It checks entry byte quota before accumulating lengths. Requires base<=S<=C<=N, matching boundary term and no overflow; corrupt snapshot or missing suffix returns an error without partial state.

Restore creates a fresh KV/session pair at prefix S. An M02 worker initialized at S applies only S+1..C, in order, through ProjectWithSessions. Every future must complete and its result decode before advancement. Entries C+1..N are validated/retained in the log, never applied by recovery. PUT/DELETE/CAS/read/noop retain their slots. The loop uses offsets rather than a potentially wrapping S+1 at MaxUint64.

Finally the recovery worker is drained and closed; only after termination does CombinedRecovery transfer exclusive ownership of the fresh Machine with Snapshot, LastApplied=C and ScheduledApplyIndex=C. A subsequent applier may start at C; never two simultaneous workers on the same machine. Cancellation/errors drain accepted work and return no partial machine. No historical GET result, waiter or result ring is restored.

## Retained prefix and WAL compatibility

The isolated component also accepts a complete log-level view with an earlier base than S: it validates the prefix and term at S without applying 0..S. A test uses CAS already included in the snapshot and suffix retries to prove it is not executed twice.

This does not introduce a second persistent format: a published generation still contains snapshot and base-S WAL as specified by Phase B, first frame with current hard state/commit=S, suffix and final checkpoint. An old base-zero WAL cannot simply replace the WAL in a base-S generation: Replay rejects it. Older generations retaining the full prefix remain preserved and are not selected as fallback. “WAL-only” means the historical empty-snapshot/base-zero path; its contract still requires CURRENT and generation files. Directories without CURRENT or snapshot are not a newly supported format.

## Deduplication and logical state

The same Machine restores and updates KV and sessions: client_id, latest sequence, digest, kind, result and original index; no parallel cache or TTL. A successful CAS retry already in the prefix retains original result/index; a failed CAS remains failed even when another operation makes current state match its expected comparison. Stale/conflicting sequences and the 1024 cap retain M11 semantics.

Isolated tests compare final snapshot bytes with an original machine continued through the history: suffix PUT/DELETE, successful/failed CAS, read/noop, retry and stale; no speculative contaminating entry. The full-prefix case changes x back to A before retrying a failed CAS: x remains A and the result keeps index 3. A 1024-session image rejects a new client during replay without eviction.

## Controlled activation and bootstrap

Activation is an explicit library choice: **node.StartAPIWithSnapshotRecovery(v2 config)**, without implicit bootstrap. Internally it uses wal.OpenSnapshotSessions and raft.Options.SnapshotRecovery. Ordinary StartAPI and the quorumkv command keep previous recovery and reject nonzero bases. No public flag/configuration, automatic migration or activation on existing clusters. Explicit mode rejects legacy v1, simultaneous bootstrap and combinations lacking API/commit/sessions/reads. MachineFactory is not allowed with snapshot recovery.

Actual order: configuration; CURRENT existence; LOCK; IDENTITY v2 and hash/node; canonical CURRENT; selected generation and private image; digest/config/base; WAL and hard state/commit; only permitted M03 incomplete-final-tail repair if needed, with Sync; inactive follower log-level core; isolated restore/replay; cursor reconstruction; fresh applier at C; only then listener binding, service/timer/network-worker Start and Serve. State is complete before exposure; constructing the core earlier produces no elections or RPCs.

CURRENT remains the v1 authoritative manifest; no third manifest or new protocol. LOCK and WAL descriptor stay within the same Store across reading and continued runtime: no close/reacquire window. SnapshotImage returns copied validated bytes. The writer continues appending only to the selected WAL with exact generation/sequence tokens; it never publishes new generations or deletes files.

## Cursors, handoff and failures

At restore A=L=S; after actual replay completion A=L=C, while N remains the last persisted index and may exceed C. Core reconstructs Scheduled/Applied in order for actually completed entries; never initializes cursors to N. Term and vote come from the final WAL, not snapshot. Startup role is follower, applyEpoch=1 for the fresh instance; no epoch change or live installation implemented.

The new runtime applier starts at C after isolated-worker termination. Result ring remains empty with cursor C: an old receipt is outside retention, never receives an invented GET response. New requests use normal M07/M10/M11 quorum and fences. nextIndex starts at last log+1; remote match is never increased merely because a local snapshot exists. termS remains the AppendEntries boundary sentinel.

Storage/profile/manifest/snapshot/WAL errors fail before listening; no WAL-only fallback. A bind failure after recovery drains service and closes Store/LOCK. Immediate shutdown follows existing lifecycle; tests reacquire locks and ports. No partial map is exposed or reverted through rollback.

## Tests and fault injection

13 new top-level test functions: seven isolated recovery, one service handoff, three bootstrap/compatibility and two process functions (one is a child helper). The first isolated group passed before bootstrap edits, as required.

| Check | Verified result |
|---|---|
| Empty/stateful snapshot, full prefix, repeated replay | Fresh equivalent state; only S+1..C, no duplicate application. |
| Commit before S/above N, gap, inconsistent term/vote/base/sequence, corrupt/mismatched raw entry, missing suffix, corrupt snapshot | Error with no partial machine/cursor. |
| Cancellation, worker cap, 1024 sessions, MaxUint64 | Controlled refusal or correct boundary without overflow/eviction. |
| Seven publication interruptions: before creation, after snapshot write/Sync, after generation rename, before publication/CURRENT rename and after rename | Old/new CURRENT selects the correct log; identical final applied state. |
| Missing/corrupt snapshot/WAL/CURRENT/IDENTITY, mismatched base, default mode and bind failure | No partial startup; listen callback unreachable except intentional bind; ports/LOCK released. |
| Image read blocked by explicit gate | NewService neither returns nor activates timers/network/collector before recovery; afterward A=L=C and follower. |
| Explicit/ordinary WAL-only base zero; legacy v1; missing CURRENT | Positive base-zero compatibility, explicit legacy/missing rejection without fallback. |
| Three processes with already published snapshots | Post-snapshot writes/GET, follower/leader crash/restart, two complete crash/restarts with new suffix between them, original successful/failed CAS retries retained. |

The process test uses at most three run-owned children, PID-targeted kill and joined exits; private status commands over pipes, localhost ports, deadlines and cleanup. No public diagnostic API added. HTTP successes are also awaited as applied progress on live replicas before interruption.

A speculative suffix is excluded **during recovery**; after startup Raft may legally commit it through a new-term noop and quorum. The test never promises an unknown operation can never apply. Every test node has a compatible base: a follower missing a required prefix needs InstallSnapshot and remains unsupported. No new runtime or M12 simulator fault switches.

## Executed checks and coverage

Full M01–M12 and M13 A/B/C suites passed with coverage and 75 s/package timeout; node including process tests completes in about 17.1 s, snapshot in about 4.3 s. Targeted race passed on snapshot/wal/applier/committed/storageformat and new Snapshot cases in raft/node (node about 6.9 s under race). Gofmt, go vet, macOS ARM64 build, canonical fixture verifier, dependencies and diff checks passed. The Python verifier checks vector arithmetic and never replaces Go recovery tests. go list -m all returns only the project module, no go.sum. No previous fixture changed. Structural validation: 142 Markdown files, 70 language pairs, links/tables/IDs/technical blocks without errors. HEAD remains c0e00d0; no commit.

Statement coverage from the complete suite, deduplicating blocks shared by packages: snapshot 87.8%, new recovery.go 81.0% (51/63), node 82.3%, raft 88.7%, wal 83.8%, applier 95.1%, committed 85.1%. Temporary profile `/tmp/quorumkv-m13c.cover`. Coverage and race checks are not formal safety/crash-consistency proofs.

## Gates, acceptance and remaining readiness

| Official gate / criterion | Phase C evidence | Final status and missing checks |
|---|---|---|
| C-01 / OQ-05 | Unchanged v1 formats/v2 profile, reused codecs/fixtures. | RESOLVED/conformance verified. |
| C-02 / OQ-01 | Base-S reading, later appends and same-generation process restart, continuous lock and explicit recovery verified. | Runtime-recovery subset VERIFIED; overall M13 gate CONDITIONALLY READY for live rotation/cleanup and remaining cycle. |
| C-03 | Correct cursors, pre-listener handoff, KV/sessions, original results and speculative exclusion at recovery verified. | CONDITIONALLY READY: live installation, epochs/stale completions and compaction safety remain. |
| T-015 / FR-015 / AC-015 | Restore+suffix and post-restart retry verified. | Open: far-behind follower, chunk transfer/retries, term changes and InstallSnapshot. |
| T-016 / FR-016 / AC-016 | Selection cutpoints, recovery and follower/leader/all-process crashes after acknowledged writes. | Open: compaction, cleanup/GC and generation switch in a live runtime. |
| T-026 / NFR-006 / AC-026 | Worker cap 1..64, 1024 sessions and bounded WAL bytes; tested storage quotas retained by M03 writer. | Open: compaction/cleanup cycle and reserve under all runtime interactions, roadmap RSS/CPU measurements. |

Official definitions remain in [testing](testing-strategy.md) and [acceptance](acceptance-criteria.md); subsets never replace them. Compaction and InstallSnapshot **are not READY for activation** based only on Phase C: implementation and corresponding tests/authorization are required.

## Issues, guarantees and limitations

No demonstrated M01–M12/Phase A–B defect requires out-of-scope repair. Development corrected two harness errors: comparing an optional GET value directly as a string and using a mutation helper to construct a canonical read. The latter caused a test wait timeout before the gate; the record was fixed and explicit waits bounded. Final checks pass after correction; no error is hidden by changing fixtures or criteria.

Verified within the test model: committed state reconstructed before listening, no duplicate 0..S application, consistent dedup, WAL-derived term/vote, successes retained after process crashes with intact storage and recoverable majority. SSD loss, OS crash/reboot or power loss, complete compaction/installation safety, M14 general checker and production readiness are not proved. A hash checks integrity, not an arbitrary Raft history.

## Budget, hardware and decision

Additional budget €0; existing Go 1.27.4 darwin/arm64 tools/caches under `/tmp`, GOPROXY/GOSUMDB off, standard library only. No cloud/download/paid service, GPU, VM or Docker. GOMAXPROCS=2 and serial packages; few-record datasets, small snapshots (synthetic in-memory session-cap case), retained generations, temporary directories. Targeted 40 s/full-regression 75 s timeouts and 25 s per child; at most three ordinary nodes, no cluster left running. No stress, prolonged benchmark, bulk writes or physical crash; RSS/CPU unmeasured.

**Phase C: RUNTIME RECOVERY PHASE COMPLETED. M13: PARTIALLY COMPLETED.** No known blocker within completed scope; overall C-02/C-03 CONDITIONALLY READY and T-015/T-016/T-026 open. A new assignment may authorize live-cycle, compaction and InstallSnapshot design/verification under the approved gates without inferring permanent activation. Automatic publication, prefix deletion/GC and peer transfer remain unimplemented; M14 not started. No Git commit.
