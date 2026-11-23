# M13 Phase D — Isolated compaction and equivalence verification

[Index](README.md) · [Italiano](../it/milestone-m13-compaction-preliminary.md)

**23 November 2026 — ISOLATED COMPACTION PHASE COMPLETED. Overall M13: PARTIALLY COMPLETED. M14 NOT STARTED.** Clean Git baseline `243a48a`; Phases A–B–C already recorded in history before the assignment. No commit performed in this phase.

## Prior authorization check

The [roadmap](implementation-roadmap.md), read in full in both languages, assigns M13 snapshots/compaction, atomic generations, installation and T-015/T-016/T-026 within their subset. M14 retains the general checker and property checks. The [pre-M13 review](pre-m13-review.md), [Phase A](milestone-m13-preliminary.md), [Phase B](milestone-m13-persistence.md), [Phase C](milestone-m13-runtime-recovery.md), ADR-005/ADR-010 and canonical formats already define the base-S representation and publication protocol.

The Phase C conclusion permits a new assignment to authorize compaction design/verification under existing gates. This assignment authorizes non-destructive isolated construction only: the base-S format is already supported by WAL/readers, writer B and recovery C are verified. No ADR requires a new format or decision to reconstruct this representation in test directories. **Pre-code decision: isolated implementation allowed**, without converting CONDITIONALLY READY C-02/C-03 into runtime authorization. No blocking discrepancy or invented gate.

## Scope and files

Created `internal/snapshot/compaction.go`: PrepareCompaction, equivalence checking and private compactIsolated orchestration. Created `internal/snapshot/compaction_test.go` and `internal/raft/compaction_boundary_test.go`. Only modified `internal/snapshot/persistence.go`: extracting Publish into publishOwned under the same mutex and sharing a reader mode without repair for non-destructive proofs. Public Publish and B/C recovery retain earlier behavior.

PrepareCompaction is an in-memory function over immutable synthetic inputs; it changes no files or core. Publication orchestration is package-private and called only by tests on test-created directories. No node/service/server caller, option, trigger or client integration added. StartAPIWithSnapshotRecovery and ordinary startup unchanged; no production Raft source modified.

Updated both languages' storage-and-recovery, canonical-formats, raft-consensus, architecture, failure-model, testing-strategy, acceptance-criteria, traceability, implementation-roadmap, development-guide, ADR-005/ADR-010, indexes and root README; added both milestone-m13-compaction-preliminary reports. Fixtures, dependencies, codecs and historical reports not rewritten.

## Compact representation and invariants

No new format: application identity/configuration v2, QSN1 container/payload v1 and CURRENT v1. CURRENT is the authoritative manifest, not a third per-generation file. gen-N contains canonical snapshot and wal; base_index/base_term and whole-file hash must match the image. A generation name is never reused, including staging names, with no overflow.

Given a validated log with base b, commit C and last index N, S must satisfy b<=S<=C<=N and its term must equal term[S]. Snapshot/sessions represent only the prefix **actually applied on a fresh isolated instance**: PrepareCompaction never equates a live node's commit/scheduled cursor with apply. It reconstructs/applies through S, starts an exclusive owner at cursor S, parks the worker and uses Phase A Capture with A=W=L=S and the isolated view's synchronized C. Release precedes encoding; previous workers terminate before ownership transfer.

The WAL reuses B's generationWAL: initial sequence 1 with current hard state and commit=S, all S+1..N in bounded transactions, then final C checkpoint. Indexes <=S are represented by the snapshot; S+1..C will be applied by recovery; C+1..N remain byte-exact and available but unapplied. Term/vote come from the final view, not capture. termS remains the Log Matching sentinel. Physical sequence and generation_id may change; they are not Raft indexes. MaxUint64 never computes a wrapping S+1.

## Construction, publication and preservation

Before writing, PrepareCompaction checks the candidate by reconstructing both paths through C. compactIsolated holds the isolated owner's mutex across reading, preparation, proof, publication and final verification, with no live core/map lock. It reuses B's protocol rather than a second writer: accounting reservation; gen-N.tmp; complete snapshot/WAL write/Sync/Close and reread; staging fsync; gen-N rename/root fsync; canonical CURRENT.tmp write/Sync/Close and reread; CURRENT rename/root fsync.

The generation name is returned only after recovery verification from the published files too. Before CURRENT rename the previous generation remains authoritative; rename or later errors can be uncertain. No success on failure, rollback or highest-gen-N recovery selection. A post-publication proof failure poisons the owner and preserves the actual CURRENT and every candidate.

Previous generations and original WALs are never changed or deleted. The only authorized replacement is atomic CURRENT in the test directory. Failed staging/CURRENT.tmp are retained and charged; an existing CURRENT.tmp blocks publication. No cleanup, GC, prefix deletion, claimed physical saving or runtime compaction. Test files are removed only by temporary-directory cleanup.

## Recovery without source mutation

The proof uses B's shared reader with repair disabled: an incomplete WAL tail is an error and never truncated. This protects source bytes, including known corruption injected by a test. M03/B/C policy is unchanged: ordinary recovery may still repair only the model's permitted structurally incomplete final tail, never complete corruption. Sync and Close remain checked.

CURRENT, identity, profile, image/hash/base and WAL must all validate. Recovery C restores KV/sessions on a fresh instance and applies only S+1..C; final A=L=C, N and speculative suffix remain separate. Errors/cancellation expose no partial state. Corrupt current selection never permits fallback to older generations even when still valid.

## Equivalence and deduplication proofs

Test path A: direct full committed-prefix replay through M02/M11 without PrepareCompaction or RecoverCommitted. Path B: candidate/published image, decoder and suffix-only recovery C. Comparison checks every key/value and presence, logical bytes, every client/sequence/fingerprint/kind/result/original index, commit, cursors, hard state, last index, new boundary and all retained entries, including speculative ones. Hashes/counts never replace diagnostic rows. Both paths intentionally share the project's domain machine; this is not M14's independent checker.

The small history contains PUT, DELETE, read, noop, successful/failed CAS, deduplicated retry, stale sequence and multiple clients. **All 11 boundaries S=0..10** are exercised, not just favorable cuts, with N=11 and C=10. Final hard state term 4/vote n2 survives even when termS is 1 or 2. Speculative value 11 never contaminates the final map. Repeated recovery and explicit reopening yield identical rows; successive nonzero-base compactions retain every generation.

A further actually executed 1024-client case stays in memory: capture S=512, replay through C=1024, reject a new client and advance an existing client without eviction. MaxUint64 checks boundary arithmetic only, without new entries. Eight alterations valid in shape but different in meaning (KV, session sequence/digest/result/index, vote, speculative bytes or missing speculative suffix) must fail comparison even when the final KV map agrees.

## Tests, fault injection and results

13 new top-level tests: 11 snapshot and two pure Raft scenarios; bounded table subcases. No new distributed process for Phase D; A–C regressions include existing local three-process tests.

| Group | Observable state and result |
|---|---|
| Boundaries 0..10, repeated nonzero base, invalid/canceled raw input | Detailed equivalence or rejection before staging creation; sources preserved. |
| 16 B cutpoints: before create, snapshot/WAL written/synced, before/after generation rename, CURRENT before create/written/synced/before/after rename and root Sync | Old CURRENT before its rename, new afterward; equivalent committed recovery and intact old files. |
| Nine file faults: partial ENOSPC, Sync and Close on snapshot/WAL/CURRENT.tmp | Explicit failure, no new selection, retained residue. |
| Five namespace faults: staging directory, generation/CURRENT rename, root Sync before/after CURRENT | No success; new selection only if CURRENT rename completed. |
| Insufficient quota, existing CURRENT.tmp | No deletion for space or automatic cleanup; rejection and retained evidence. |
| Ten corruptions: truncated/checksum/version snapshot, truncated/checksum/gap/boundary-term WAL, manifest, CURRENT and profile | Error without fallback or source repair in compaction procedure. |
| Corruption after publication and waiter cancellation after root Sync | Uncertain outcome; new CURRENT retained, no rollback; valid explicit recovery for cancellation alone. |
| Concurrent Close during blocked write | LOCK held until drain, no deadlock or early success. |
| Short/divergent follower, wrong boundary term and committed conflict | Sentinel retained; reject or reconcile only uncommitted suffix, no fabricated replication. |
| Runtime cleanup/prefix deletion | Unimplemented and unexecuted; missing evidence remains. |

Tests reuse B's cutpoints and I/O wrappers with real primitives on run-owned directories; they never extend M12 scenarios with unsupported actions. The matrix fixes inputs/order without stress or sleep dependence. Tests inspect selected files, application rows and original bytes. Corruption is deliberately introduced only by tests on their own files.

Targeted D tests and the complete M01–M12/M13 A–D suite passed with coverage, 75 s/package timeout, GOMAXPROCS=2 and serial packages. Snapshot takes about 7.4 s in regression, node about 17.2 s including prior three-process cases. Full targeted race passed on snapshot (about 14.6 s), raft (about 6.5 s), wal, storagefs, applier and committed. Gofmt, go vet, macOS ARM64 build, canonical fixtures, dependencies and git diff --check passed. The Python verifier checks vector arithmetic: separate Go tests verify recovery. go list -m all returns only the project module, no added go.sum/dependency. Bilingual check: 144 Markdown files, 71 pairs, links/tables/IDs/technical blocks without errors. Scope check confirms persistence.go is the only previously existing production source modified, no runtime compaction caller and HEAD unchanged at 243a48a.

Statement coverage measured over the complete suite, deduplicating blocks imported by several packages: snapshot 87.3%, new compaction.go 83.8% (88/105), raft 88.8%, wal 83.5%, applier 95.1%, committed 85.1%. Temporary profile `/tmp/quorumkv-m13d.cover`. This is not a correctness/safety proof for unimplemented runtime paths.

## Lagging followers and future handoff conditions

A base-S leader cannot build AppendEntries when nextIndex<=S; a follower whose log does not reach sentinel S/termS cannot accept the first suffix either. Tests verify builder ErrUnsupported below base, short-follower rejection and unchanged remote matchIndex: a local snapshot does not prove installation on that follower. With a compatible base, an uncommitted divergent suffix can be reconciled by existing M06 rules; conflict inside known committed entries is fatal. No production core change.

Before runtime compaction or making a prefix deletable, the following are still required:

1. Capture actual A=W=L=S, then a distinct final storage barrier: suspend fresh persistent effects/rotations, drain I/O and freeze updated term, vote, C and N. Retain entries arriving after capture too.
2. Complete publication with old+new+staging reservation, writer/core switch using correct generation_id/write_sequence tokens, invalidate old completions. Live maps may be beyond S: never reset A/L or replay their prefix.
3. Retain S/termS, a complete image with whole-file digest/configuration, every needed suffix entry and local hard state. Never fabricate nextIndex/matchIndex advancement or delete the sole follower recovery source.
4. Verified InstallSnapshot: valid leader/term/identity, chunks <=64 KiB, image <=8 MiB, exact offset/retry, 5 s staging timeout, quotas and digest. Before final installation drain applier, recheck S against current A and leader/epochs; obsolete if S<=A, no committed loss on mismatch, retain suffix only under approved conditions. Preserve updated local vote.
5. Publish before atomically replacing KV/sessions, increment applyEpoch with overflow checks, invalidate obsolete waiters/result ring/completions. Only correlated final installed_index acknowledgment can authorize leader progress, never chunk ack. Handle lost acknowledgment under the contract.
6. Deletion/GC checks only after verified authority/recoverability, never delete active files for quota; post-CURRENT I/O failures stop participation without rollback. Shutdown must complete or report incomplete drain before LOCK release. Deadline never cancels uncertain syscalls.

These are already approved conditions to verify in a future assignment, not implementation or permanent authorization. InstallSnapshot and runtime deletion remain **not ready for activation**.

## Gates and official criteria

| Gate / criterion | A–C evidence and D addition | Final status / missing checks |
|---|---|---|
| C-01 / OQ-05 | Codec, v2 profile and canonical references reused without new bytes. | RESOLVED, conformance verified. |
| C-02 / OQ-01 | B primitives/publication and C runtime recovery retained; D proves isolated compact construction/publication/recovery. | Isolated subset VERIFIED; overall gate CONDITIONALLY READY for live switch/cleanup/remaining faults. |
| C-03 | Capture/dedup/C handoff retained; D equivalence and boundary/suffix verified in isolation. | CONDITIONALLY READY for installation, epochs/stale completions and runtime deletion safety. |
| T-015 / FR-015 / AC-015 | Codec, capture, restore+suffix, post-restart retries and compact equivalence. | Open: official criterion requires far-behind follower, chunk retries/term change and InstallSnapshot. |
| T-016 / FR-016 / AC-016 | B publication/cutpoints, C process crashes and D equivalence with retained sources. | Open: cleanup/GC, prefix deletion and live runtime compaction handoff. |
| T-026 / NFR-006 / AC-026 | A/B quotas, C queues/reader, D actual 1024 cap, retained suffix, reserve/pre-write refusal. | Open: runtime/cleanup interactions and roadmap's full RSS/CPU/space report. |

Original definitions in [testing](testing-strategy.md) and [acceptance](acceptance-criteria.md) are unchanged. No isolated check closes a criterion requiring absent functionality.

## Issues, guarantees and limitations

No discovered M01–M12/Phase A–C bug requires repair. The new non-repairing read mode is necessary for non-destructive proof, not migration or WAL reinterpretation. B's protocol was only factored to retain one publication sequence and serialize it with D's proof. Executed checks found no blocker.

Verified: a recoverable compact representation logically equivalent within synthetic scope, preserved dedup/committed state, available but unapplied speculative entries, explicit errors and unchanged previous-generation bytes. Unproved: live compaction, prefix-missing follower recovery, InstallSnapshot, safe GC, M14 general checker, RSS/CPU, OS reboot/power durability or SSD loss. Hashes and Sync are not a formal consensus or physical multi-file atomicity proof.

Additional budget €0, fully offline, standard library and existing Go 1.27.4 darwin/arm64 tools/caches only. GOMAXPROCS=2, serial packages, explicit timeouts, few generations and small datasets; 1024-client history entirely in memory. No cloud, GPU, VM/Docker, stress, intensive benchmark or bulk writes. Only test-owned directories and controlled cleanup; no actual cluster directory changed and no cluster left running.

## Decision and continuation

**Phase D: ISOLATED COMPACTION PHASE COMPLETED. M13: PARTIALLY COMPLETED.** C-01 confirmed, overall C-02/C-03 CONDITIONALLY READY, T-015/T-016/T-026 open. No known blocker within completed isolated scope. A new assignment may authorize InstallSnapshot and required handoff/quota/epoch checks or further isolated verification; permanent compaction, GC and prefix deletion are not authorized by Phase D alone. M14 not started. No Git commit.
