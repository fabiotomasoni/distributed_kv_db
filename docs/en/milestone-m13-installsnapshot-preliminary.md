# M13 Phase E — Isolated InstallSnapshot and follower recovery

[Index](README.md) · [Italiano](../it/milestone-m13-installsnapshot-preliminary.md)

**23 November 2026 — ISOLATED INSTALLSNAPSHOT PHASE COMPLETED. Overall M13: PARTIALLY COMPLETED. M14 NOT STARTED.** Clean entry revision `b9ebde7`; no Git commit performed in this assignment.

## Authorization and official scope

The [roadmap](implementation-roadmap.md) assigns snapshots, compaction, generations, installation and the relevant T-015/T-016/T-026 subsets to M13. M14 retains the general checker/property work. The [pre-M13 review](pre-m13-review.md), [A](milestone-m13-preliminary.md), [B](milestone-m13-persistence.md), [C](milestone-m13-runtime-recovery.md), [D](milestone-m13-compaction-preliminary.md), ADR-005/ADR-010, canonical formats, M04 wire, M05 terms, M06 matching, M07 apply and M11 sessions were examined before implementation, together with both roadmaps and the M12 simulator.

Initial C-01/OQ-05 confirmed; overall C-02/OQ-01 and C-03 CONDITIONALLY READY; full T-015/T-016/T-026 open. The assignment and approved Phase D continuation permit dormant isolated implementation and verification using existing formats and publication primitives. No gate prohibits this bounded scope. **Pre-code decision: authorized in isolation**, with no authorization to activate the ordinary cluster path. No new wire field, format version, acceptance criterion or waiver is introduced.

## Components and changed files

| File | Responsibility |
|---|---|
| internal/snapshotinstall/receiver.go | Dormant receiver; one WAL owner, real Raft core, bounded M02 worker, staging and serialized handoff. |
| internal/snapshotinstall/transfer.go | Finite immutable leader transfer/controller; correlated chunks and explicit suffix AppendEntries. |
| internal/snapshot/install.go | Pure PlanInstallation and PublishReceived using the sole Phase B writer. |
| internal/wal/snapshot_transition.go | Explicit continuous-LOCK owner transition and refreshed generation tokens. |
| internal/snapshot/persistence.go | Factor existing publication into publishViewOwned; public B publication and D orchestration retain their semantics. |
| internal/wal/open.go; internal/wal/store.go | Retain immutable identity for controlled reopen under the same lock. |
| internal/raft/commit.go | Dormant startup-only EnableRecoveredCommit, accepting already-applied cursors and an explicit epoch. |

Seven new test files: snapshotinstall/receiver_test.go, safety_test.go, lifecycle_test.go, term_test.go; snapshot/install_test.go; wal/snapshot_transition_test.go; raft/snapshot_dormancy_test.go. They contain 29 top-level functions including one subprocess helper, plus bounded table cases. Documentation updates cover the two reports, indexes, protocol, formats, Raft, storage, architecture, failure model, testing/acceptance/traceability, development, roadmap and ADR-005/ADR-010.

No ordinary node, service, server, transport or election/replication source imports snapshotinstall. StartAPIWithSnapshotRecovery remains the Phase C capability; ordinary startup/CLI and client API guarantees are unchanged. The ordinary core still rejects Snapshot application effects with ErrUnsupported after any required real M05 term persistence; a dedicated regression proves this. No automatic fallback, listeners, transfer goroutines, snapshot trigger, live compaction, GC, production prefix deletion, migration or M14 work.

## Existing wire contract and receipt states

Uses M04 Snapshot on `/raft/v1/install-snapshot`, protocol_version 1. Envelope: cluster_id, config_hash, from, to, rpc_id, term. Request fields: leader_id, snapshot_id, last_included_index, last_included_term, offset, data_b64, done, total_bytes, sha256. Integers follow existing decimal-string rules. snapshot_id=sha256=SHA256 of the complete QSN1 file including trailer, distinct from its internal payload digest. Source generation binds the local leader controller and is **not an invented wire field**.

Each raw chunk is <=64 KiB; total image is 44 bytes..8 MiB; one in-memory transfer per receiver. Bytes grow only with actual validated chunks, never an unchecked declared allocation. Exact next offset appends. A completely received identical range is a retry; a gap returns the expected offset, partial overlap/conflicting bytes invalidate staging, inconsistent metadata invalidate it, another ID is rejected while busy. Explicit monotonic time expires staging after 5000 ms idle. Close/restart discard it and restart at offset zero. This is bounded memory receipt, not durable partial progress.

Responses retain accepted and next_offset. A partial accepted response changes only sender offset, never matchIndex. Only correlated accepted/done/full-length installed_index=S proves installation. An obsolete response is accepted=false,next_offset=0,reason=obsolete,resume_index,resume_term; resume=max(S,local base), with actual local term. The leader verifies that pair against its own frozen log; nextIndex may move to resume+1 but **matchIndex stays unchanged until a valid subsequent AppendEntries success**. A lost final ACK therefore yields either safe retry or obsolete+verified suffix exchange, without duplicate application.

## Terms, identity and transfer ownership

Every request is revalidated by peerprotocol before effects. The receiver drives the **real M05 core**, synchronizes its exact persistent effects through the WAL and feeds real Durable tokens back. A higher term updates local hard state first, with existing vote rules; a same-term valid leader preserves local vote. Lower terms cannot install. A vote/AppendEntries from a new term/leader invalidates prior staging; delayed old chunks cannot overwrite new state. Same-term conflicting leaders fail closed under existing M05 rules.

The leader controller binds term, generation and a process nonce/RPC correlation. Delayed responses from a superseded local term/generation never advance progress; foreign correlation is rejected. Higher response terms produce HigherTerm, with no progress: the driver must deliver the response to the real M05 core and persist step-down. A dedicated test actually elects a leader, then performs this durable transition. The model is single-owner, finite and explicitly driven; it does not implement a new election engine or automatic network retry/backtracking algorithm.

## Compacted indexes and suffix reconciliation

Let b be local base, S incoming boundary, A actual applied, C commit, N last. base_index/base_term represent the sentinel; entries are b+1..N. Indexes <=b are never application entries; an empty suffix has LastLogIndex=b and LastLogTerm=base_term. Existing RequestVote compares last term/index after restart; AppendEntries accepts prev at the sentinel only with its correct term, rejects earlier indexes and reconciles the available suffix using M06. No fictitious entry zero or rebased slice indexes.

Installation first parks/drains accepted work and real completions, proving A=ScheduledApplyIndex=worker cursor. Recheck current leader, term and apply epoch; S<=A is obsolete, including when it became obsolete during that drain. This is not a comparison against a guessed commit cursor.

| Boundary relation | New generation |
|---|---|
| Local term[S]=incoming termS | Retain **every** S+1..N entry, including speculative entries; C becomes max(old C,S). |
| No matching boundary and S<=old C | Reject: already-known committed data cannot be discarded. |
| No matching boundary and S>old C, including a shorter log | Snapshot establishes base/C/N=S; discard only the incompatible speculative suffix in the **new** generation. |

Local current term/vote remain authoritative, never a sender's vote. KV and sessions represent the same prefix S; committed retained suffix S+1..C is then applied, speculative C+1..N is available but unapplied. Sequence/generation tokens are storage coordinates, not Raft indexes. Arithmetic checks avoid wrapping. The format can encode MaxUint64; activation refuses N=MaxUint64 before publication because the existing core cannot form nextIndex=N+1. Epoch overflow likewise refuses before publication.

## Validation, persistence and atomic handoff

Decode/canonical checks validate QSN1 container/payload v1, complete KV/session rows and cap 1024, persistent configuration/identity v2, cluster/config hash, image length, complete-file hash, boundary and current local hard state. A valid file alone is insufficient authorization. PlanInstallation validates a final immutable local view and reuses generationWAL/Replay. PublishReceived rereads CURRENT without tail repair and verifies its sequence/base/term/hard-state/commit/last metadata under the store's exclusive lock. Invalid selection never falls back to an older generation.

TransitionSnapshot serializes Append/Close and lends the **same stable LOCK** to one finite non-reentrant publisher; it never drops/reacquires ownership. PublishReceived invokes B's single publication sequence: reserve old+new+staging space; fresh gen-N.tmp; full snapshot/WAL write, Sync, Close, reread; staging-directory fsync; generation rename/root fsync; canonical CURRENT.tmp write/Sync/Close/reread; CURRENT rename/root fsync. gen-N contains only snapshot and wal, CURRENT is the authoritative manifest. No third manifest, alternate writer, selected-highest-generation recovery or rollback.

The WAL embeds the new snapshot base/hash, local updated hard state and coherent retained suffix/commit. Before CURRENT rename the old selection remains authoritative; afterward recovery selects the new one even if the caller fails. Callback/reopen/old-file Close failures poison the owner, retain candidates and never acknowledge. A successful transition must actually select a new generation, refresh file/state/token ownership, and match the received image hash/base. Old files are retained; legitimate M05 hard-state appends before the switch preserve the old WAL prefix rather than promising its whole file never changes.

The receiver mutex excludes RPC/lifecycle/new admissions throughout the isolated handoff. Accepted old M02 work is drained even if a request is canceled, real completions consumed, barrier released, and old worker shut down **before publication**. After publication and writer adoption, restore KV/sessions together on a fresh machine, increment the logical applyEpoch, construct a new core/applier at S, clear old pending/result state and apply retained committed suffix through C. Only then send installed_index=S. Old-epoch completions are ignored. Diagnostic View captures a fresh machine using its Phase A capture epoch 1; the core's separate logical apply epoch increments across handoff. No reuse of historical live waiters or result ring is implied.

## Deterministic catch-up and equivalence

The leader model owns base S=4, termS=2, term 5, C=10,N=11, with an immutable suffix. The follower starts behind S; nextIndex<=S cannot be served by ordinary AppendEntries from the active leader log. Explicitly send chunks, install, then resume existing M06 AppendEntries with prev=S/termS. Tests inspect the complete log and application rows against direct full committed-prefix application, not just a digest/count.

The small history includes PUT, DELETE, read, noop, successful and failed CAS, duplicate and stale requests, several clients and an unapplied speculative entry. Comparisons check bytes/presence, every session's client/sequence/fingerprint/kind/cached result/original index, C/A/scheduled, hard state, boundary and all suffix entries. After restart and replacement of the speculative entry, retries preserve the original successful and failed CAS results even when today's KV would give a different outcome. The same-KV/different-session crash case detects restoring the wrong generation. A real 1024-client capture/transfer verifies no eviction, deterministic refusal of client 1025 and continued progress by an existing client.

Two fixed-input catch-up runs with a fixed test nonce compare request/response trace bytes and final image/log, reporting the first divergence index. Existing M12 saved-manifest/seed scenarios remain regressions; their simulator does not gain unsupported snapshot actions. The new isolated finite driver uses the real core/storage/applier and private publication seams. No M14 independent general checker is claimed.

## Tests and fault evidence

| Group | Executed observations |
|---|---|
| Boundaries/elections | Empty suffix, matching/wrong/before/after prev boundary, RequestVote freshness before/after restart, MAX arithmetic/refused activation. |
| Receipt | Exact duplicate, gap, overlap, conflicting bytes, changed metadata/ID, expiry, clock regression, Close/partial restart. |
| Authorization/validation | Lower/same/higher term, new leader/vote, delayed old chunks/responses, mismatched identity/profile/version/hash/boundary, wire size limits, corrupted CURRENT. |
| Suffix/apply | Committed+speculative retention, uncommitted mismatch discard only in new generation, committed conflict refusal, blocked old worker and snapshot becoming obsolete while draining. |
| Idempotence | Lost final response, obsolete+verified AE, duplicate image, repeated recovery, cached failed/successful CAS, stale sequence, full session cap. |
| Publication | All 16 existing B cutpoints exercised through received-generation publication; before CURRENT rename select old, after select new; detailed replay and source preservation. |
| File faults | 12 combinations: snapshot/WAL/CURRENT.tmp with partial ENOSPC, Sync, Close or corrupt reread; no selected bad generation. |
| Handoff | Injected errors at image.validated, before-generation, generation.published and memory.installed; no final ACK. |
| Actual process exits | One child at a time exits at before-generation, generation.published or memory.installed; repeated restart selects recoverable old/new state and persisted term, retry installs once or returns obsolete. |
| Ownership | LOCK probes before/after publication/after memory always reject a second opener; callback/reopen/no-publication/old-Close errors poison; successful next append carries new generation tokens. |
| Resources | Existing dataset/queue bounds, transfer/chunk/session limits, reserved peak refusal before writes; no cleanup to make space. |
| Dormancy | Ordinary core Snapshot effects remain unsupported; no production integration imports or automatic transfer. |

Temporary directories, deterministic cutpoints and I/O wrappers use actual existing primitives. Child exits simulate process loss, not OS reboot/power failure. CURRENT corruption rejects without fallback. Publication fault tests examine selected bytes/state as well as error values. Failed staging/control files are retained and charged, not silently cleaned; only test-owned temporary directories are later removed.

## Executed checks and measured coverage

All targeted new tests and the full M01–M12/M13 A–E regression suite passed, with coverage, serial packages, GOMAXPROCS=2, offline module settings and a 75 s/package timeout. Full-run times: snapshotinstall 2.052 s, snapshot 8.586 s, raft 0.614 s, wal 0.948 s, node 17.125 s (including prior three-process recovery), server 8.571 s. The complete command and targeted reproduction commands are in the [development guide](development-guide.md).

Targeted race detector passed: snapshotinstall 5.351 s, snapshot 16.653 s, wal 3.232 s, raft 6.480 s, applier 1.233 s, committed 2.259 s and storagefs 2.439 s. Gofmt reported no pending formatting; go vet and darwin/arm64 build passed. Canonical verifier passed; existing Go tests separately validate the codec/recovery and wire fixtures. go list -m all returns only the project module, go.mod unchanged and no go.sum/dependency added. git diff --check passed; HEAD remains b9ebde7. Source-call-site review confirms no ordinary integration, automatic deletion or runtime transfer.

Union statement coverage, deduplicating identical blocks imported by multiple test packages from `/tmp/quorumkv-m13e.cover`: snapshotinstall 84.2% (331/393), snapshot 87.0%, raft 88.6%, wal 84.4%, applier 95.1%, committed 85.1%. New receiver.go 84.2%, transfer.go 84.3%, snapshot/install.go 87.5%, wal/snapshot_transition.go 92.9%. Per-package cross-coverage lines in the raw test log are not these union percentages. Coverage does not establish absent runtime safety.

Bilingual mechanical check: 146 Markdown files, 72 language pairs, 2191 relative links, 132 tables and 42 matching non-Mermaid examples; no broken links, unclosed fences, column/ID/section-count or technical-block parity errors. Semantic review keeps scope/status/limits and all official criteria aligned. Inventory: 58 internal production Go files, 80 internal test files. Actual new transfer/session tests are small and finite. Existing logical caps remain 4 MiB dataset, 10000 keys, 16 KiB value and 1024 sessions; image <=8 MiB, WAL <=8 MiB including 512 KiB consensus reserve, directory <=32 MiB including 64 KiB control reserve. Reserved publication refuses before writes rather than deleting old files. No RSS/CPU or physical SSD peak measurement claimed.

## Official criteria and gate decisions

| Criterion / official objective | A–D evidence, then E addition | Missing evidence and prerequisites |
|---|---|---|
| T-015 / FR-015 / AC-015: far-behind follower, chunk retries and term change; image hash, sessions, final indexes | A codec/capture/restore, B durable image, C suffix/restart, D equivalent compact view. E adds isolated chunks, real term persistence, epoch/obsolete drain, final ACK/retry and suffix catch-up. | **OPEN in full**: ordinary RPC/leader fallback, concurrent real replication, election/retry/shutdown integration. A logically compacted prefix is required; GC is not needed to test transfer itself. |
| T-016 / FR-016 / AC-016: stop at every generation publication/cleanup cutpoint; recoverable selection after each | A integrity, B publication, C process recovery, D retained-source equivalence. E adds received generation plus writer/memory transition, file faults and child exits around publication. | **OPEN in full**: live compaction switch, prefix deletion and cleanup/GC cutpoints. These require safe retention and deletion authority; isolated installation cannot prove cleanup. |
| T-026 / NFR-006 / AC-026: dataset, queue and simulated disk quotas; errors, RSS, CPU, peak space | A logical caps, B reservation, C queues/recovery, D 1024 cap/retained suffix. E adds bounded chunk/image/staging, cap transfer, real reserved publication refusal. | **OPEN in full**: complete runtime admission/transfer/cleanup interactions and measured RSS/CPU/peak-space report. Eventual GC affects sustained space; not required to validate bounded isolated refusal. |

Original criteria in [testing](testing-strategy.md) and [acceptance](acceptance-criteria.md) are unchanged. No full criterion is weakened or closed from an isolated subset.

C-01/OQ-05 **RESOLVED/confirmed**: unchanged codec/wire/v2 profile and canonical fixtures. C-02/OQ-01: isolated received-publication/owner-transition/restart subset verified, overall **CONDITIONALLY READY** for live cycle/cleanup/fault obligations. C-03: isolated term/barrier/epoch/dedup/suffix handoff verified, overall **CONDITIONALLY READY** for real RPC/replication lifecycle and deletion safety. Codec, generations, combined recovery, isolated compaction and isolated InstallSnapshot have evidence; runtime InstallSnapshot, runtime compaction and GC remain unactivated/unverified.

## Issues, corrections and limits

Review added a same-generation rejection to the borrowed-lock transition and post-adoption image hash/base verification, avoiding any false successful handoff after a publisher that did nothing or selected an unexpected view. Tests cover no-publication and reopen/Close failures. The sole publication protocol was factored to accept the reconciled local view; no second persistence implementation or repair of older formats. No known blocking defect remains within this isolated scope after the executed checks.

Verified guarantees are bounded by synthetic histories and fault models: no known committed/sessions regression, validated coherent publication, preserved old candidates, drained old worker, refreshed epochs/tokens and correlated final progress. Unproved: ordinary distributed InstallSnapshot, concurrency with service queues/waiters/timers, liveness under arbitrary failures, safe deletion/GC, physical APFS durability after power loss, SSD failure and formal consensus correctness. A green finite suite or race detector is not a proof.

Additional budget **€0**, fully offline using standard library and existing Go 1.27.4 darwin/arm64 caches/toolchain on the owned ARM64 Mac. GOMAXPROCS=2, serial packages, short explicit deadlines; tiny histories, at most three ordinary nodes in earlier regression cases, one crash child at a time. The 1024-session image is transferred in bounded <=64 KiB chunks; no near-8 MiB stress transfer is run. No cloud, paid API, new hardware, GPU, VM/Docker, intensive benchmark, OS crash or real production-directory mutation.

## Decision and requirements before activation

**Phase E: ISOLATED INSTALLSNAPSHOT PHASE COMPLETED. M13: PARTIALLY COMPLETED.** C-01 confirmed; overall C-02/C-03 CONDITIONALLY READY; full T-015/T-016/T-026 OPEN. M14 not started, no Git commit.

Runtime InstallSnapshot still needs an explicitly authorized assignment integrating one real service/RPC owner: serialize per-peer snapshot/suffix flights, real leader selection/step-down and retry/deadlines, apply/I/O/lifecycle barriers, completion epochs, new-generation tokens, waiter invalidation, quotas, failure/uncertain-result stop policy and real three-node lost-response/term-change/crash evidence. The isolated model does not register itself as a production handler.

Runtime compaction separately needs actual capture A=W=L=S followed by a distinct final I/O barrier preserving updated hard state, C/N and every newly arrived suffix entry, without resetting already-applied live maps. Safe GC/prefix deletion needs verified CURRENT authority, retained installable follower recovery source, active-generation exclusion, full deletion/fsync cutpoints, space accounting and shutdown/uncertain-I/O handling. These dangerous changes are deferred; this assignment authorizes none of them. Stop at Phase E completion.
