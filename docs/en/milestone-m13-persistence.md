# M13 Phase B — Persistence and isolated recovery

[Index](README.md) · [Italiano](../it/milestone-m13-persistence.md)

**23 November 2026 — PERSISTENCE PHASE COMPLETED. Overall M13: PARTIALLY COMPLETED. M14: NOT STARTED.** Clean Git baseline `60e2a2c` (Phase A already recorded in history before this assignment). No commit performed in Phase B.

## Initial state and assignment

The [official roadmap](implementation-roadmap.md) defines M13 as “Snapshots and compaction”: atomic generations, installation and crash fixtures; T-015/T-016/T-026 within the phase subset. M14 retains the independent checker and general property checks; it is not started. The [pre-M13 review](pre-m13-review.md) and [Phase A](milestone-m13-preliminary.md) establish format, capture and limits. Initially the preliminary phase was PRELIMINARY PHASE COMPLETED, C-01/OQ-05 design and codec conformance verified, C-02/OQ-01 new generations and C-03 CONDITIONALLY READY.

This assignment authorizes persistent generations, publication and exclusively isolated recovery, with fault injection in test-owned directories. It does not authorize snapshot startup in nodes, replacing M03/M09 recovery, compaction, WAL deletion, InstallSnapshot or inter-node transfer. No official requirement is changed to claim M13 completion.

## Components and files

Created Darwin-specific `internal/snapshot/persistence.go` and `internal/snapshot/persistence_test.go`. APIs: OpenPersistence, Persistence.Publish, Persistence.Recover, Persistence.Close and DiskRecovery. Reused Phase A QSN1 codec, Restore, M03 WAL Replay and storagefs primitives for LOCK, private files, WriteAll, File.Sync and directory fsync. No second codec, append worker or session database.

OpenPersistence requires an existing explicitly M03-initialized v2 directory, with the previous owner closed. It never bootstraps. Its stable exclusive LOCK prevents concurrent cooperating opens; a mutex serializes publication, recovery and Close. Configuration must match IDENTITY v2 cluster, node and hash. No new CLI/HTTP option or runtime caller.

Updated root README and, in both languages, indexes, canonical-formats, storage-and-recovery, macos-storage-contract, architecture, failure-model, testing-strategy, acceptance-criteria, traceability, implementation-roadmap, development-guide and ADR-005/ADR-010; added both milestone-m13-persistence reports. No M01–M12/Phase A source, dependency, format or previous fixture changed.

## Approved identity and manifests

The canonical contract specifies exactly `snapshot` and `wal` inside `gen-N`: **CURRENT is the selection manifest**, with no invented third per-generation manifest. IDENTITY is the immutable identity manifest. Canonical checksummed CURRENT v1, at most 4096 bytes, contains generation, snapshot_sha256, base_index and base_term; its hash covers the entire snapshot file including trailer, distinct from the internal QSN1 digest. Image/payload remain version 1; application identity and configuration remain v2. No v1→v2 migration.

Under LOCK a fresh N is chosen above all recognized generations, including `.tmp`; malformed names and MaxUint64 overflow are rejected. Scanning allocates names, never selects recovery. Files are exclusive-created 0600, directories 0700; symlinks, hard links and unsafe paths are rejected. The selected generation must contain only the two approved files.

## Writing and publication

Publish consumes a detached capture; the caller keeps its buffers immutable until return. Encoding occurs outside the applier. Before writing, the current generation is validated under LOCK and its authoritative WAL read: the snapshot must satisfy previous base <= S <= current commit and match the log term at S. Historical application correctness relies on trusted Phase A capture; a digest cannot prove an arbitrary image corresponds to a Raft history.

The new WAL derives from the current locked view, never a caller-supplied stale vote: first frame sequence 1 contains current hard state and commit=S, followed by the entire S+1..N suffix in transactions of at most 16 entries/256 KiB, then current commit C checkpoint. It preserves speculative C+1..N too. S=MaxUint64 never computes S+1. Tests use snapshot term 2 and final hard state term 4/vote n2: the updated vote survives.

The protocol is: reserve old+new+staging+control accounting; create gen-N.tmp; full snapshot write and Sync/Close; reread and compare/validate persisted bytes; full WAL write and Sync/Close plus reread/Replay validation; staging fsync; gen-N.tmp→gen-N rename; root fsync; exclusive CURRENT.tmp creation; write/Sync/Close and reread validation; CURRENT.tmp→CURRENT rename; root fsync. The generation name is returned only after every barrier.

WriteAll continues the remainder after a successful short write, but never retries a partial write with error; zero progress is an error. Create, write, Sync, Close, rename, fsync and reread errors never become success. I/O failure after construction starts poisons the owner, requiring close and explicit recovery. Pre-construction input/quota rejections publish nothing. Cancellation is checked before admission; it never interrupts accepted syscalls. Close waits for active I/O before releasing LOCK; previous failures remain visible.

## Selection point and previous generations

“Built” means files exist in staging; “validated” means reread/decoded; “ready” requires both file/directory barriers and gen-N within the synchronized root. New selection becomes observable at CURRENT rename. “Publication completed” also requires successful root fsync; “recoverable after process crash” is checked by reopening and validating the actually referenced generation.

Before CURRENT rename, the old generation remains authoritative even when staging or higher gen-N directories exist. Error while attempting CURRENT rename or afterward yields uncertainty: no success, rollback or candidate deletion. With the OS alive, tests recover the new selection after completed rename even if the next barrier fails; this does not certify reboot or power loss.

No previous generation or WAL prefix is deleted. Failed staging and CURRENT.tmp remain evidence and are counted; an existing CURRENT.tmp blocks further publication without truncation. Cleanup/GC are unimplemented: removal and post-cleanup fsync faults cannot be exercised in Phase B. Retention may block further publication at quota; deleting files never circumvents it.

## Isolated recovery and logical consistency

OpenPersistence/Recover validate identity, tree safety/quota, CURRENT and only its referenced generation; they verify QSN1, whole-file hash, index/term base, configuration, structural WAL and hard-state/log/commit transitions through Replay. Missing/corrupted CURRENT, missing files, mismatched checksums/versions/profiles and inconsistent metadata are errors, never fallback to the highest or previous generation.

Only a structurally incomplete final WAL tail after a valid prefix may be truncated to its verified offset and synchronized, as in M03. Snapshot/profile validation precedes repair; complete corruption remains fatal. Complete surviving files are resynchronized. Recovery errors expose no partial machine, boundary or log; an open owner becomes failed. Retained files are never deleted.

DiskRecovery returns a fresh Machine at Point=S/termS and validated log-level State with hard state, C and suffix. **No WAL entry is applied.** Commit above S is never presented as already applied: tests observe the map at S only and keep the suffix separate. This is neither integrated snapshot+suffix recovery nor operational node startup; no listener, quorum or client acknowledgment is authorized by the result.

The capture→codec→file→CURRENT→read→Restore path preserves KV bytes, present-empty values, sequence, fingerprint, kind, result and original index. Tests containing PUT/DELETE, read/noop, successful/failed CAS, retry and stale sequence compare exact images and repeat recovery. The prefix-4 file retains B even after the original machine reaches D at prefix 10. Post-restore retry preserves PUT index 6 and failed CAS index 3; conflicts/stale sequences remain rejected. One persisted synthetic 1024-session case confirms the cap without eviction and permits an existing client to advance.

## Fault injection matrix

| Checked points | Expected recoverable selection |
|---|---|
| generation.before-create; snapshot.before-create | Old CURRENT; no new selection. |
| snapshot.written; snapshot.synced | Old generation; staging retained. |
| wal.before-create; wal.written; wal.synced | Old generation; no incomplete WAL published. |
| generation.before-rename; generation.renamed | Old generation, even with a final candidate present. |
| current.before-publication; current.before-create | Old CURRENT remains authoritative. |
| current.written; current.synced; current.before-rename | Old CURRENT; CURRENT.tmp is not authoritative. |
| current.renamed; current.directory-synced | Newly referenced CURRENT; injected error without success/rollback. |
| Create permission, partial ENOSPC, zero write, Sync EIO, Close EBADF and corrupt bytes on snapshot/WAL/CURRENT.tmp | Explicit failure before selection changes; old generation recoverable. |
| Successful short writes on each file | No duplicated bytes, valid new generation. |
| Generation/CURRENT rename and staging/root fsync failures before/after CURRENT | No success; new selection only if CURRENT rename completed. |
| Two test-child process exits before/after CURRENT rename | Exact old/new recovery and LOCK release. |
| Cleanup, writer/RAM switch, runtime epochs/installation | Unimplemented; missing evidence. |

16 deterministic cutpoints, 21 file/error combinations including three successful short writes, five rename/directory Sync failures and 26 invalid-recovery variants. Process exit 77 is limited to the test-child binary; no external process is killed. Injection uses real primitives with private test wrappers, without extending the M12 simulator or introducing public runtime hooks.

## Executed checks and resources

Full M01–M12 and Phase A/Phase B suites passed with coverage and 75 s/package timeout, followed by the entire snapshot package again after the final file-validation tightening. Targeted race passed on snapshot, storagefs, wal, applier, committed and storageformat; snapshot rerun after the final generation check. Gofmt found no pending files; go vet and GOOS=darwin/GOARCH=arm64 build passed. Canonical fixture verifier passed (vector arithmetic, distinct from Go recovery tests), no fixture changed. go list -m all returns only the project module, no go.sum. Bilingual validation: 140 Markdown files, 69 language pairs, links/tables/IDs/technical blocks without errors. Diff/scope check: only two new isolated Go files, no runtime import of internal/snapshot and unchanged HEAD.

11 new top-level test functions, including one process helper, in the new file; Phase A regressions included. Also covers metadata/cancellation/admission, no fallback, all-or-nothing recovery, persistent owner failure, concurrent exclusion/Close, quota and naming/orphans/overflow, permitted tail repair and MaxUint64 boundary.

Snapshot package statement coverage on the final isolated suite: 88.9%; new persistence.go 87.5% (272/311 statements), Phase A memory.go 93.5% (87/93). Temporary profiles `/tmp/quorumkv-m13b.cover` for the full regression and `/tmp/quorumkv-m13b-snapshot-final.cover` for the updated package. Coverage is not a consistency proof. The sequential three-generation case observed an old+new+staging/control apparent-content peak of 5799 bytes at cutpoints, plus the 65536-byte control reserve; the persisted 1024-session fixture measures 227653 bytes. These measurements describe only synthetic cases, not general profile maxima or physical SSD allocation.

Unchanged caps: snapshot 8 MiB, WAL 8 MiB, directory 32 MiB and control reserve 64 KiB; WAL profile retains its 512 KiB consensus reserve, without a new proposal API. Publish reserves the complete image, WAL and CURRENT.tmp beyond retained files before construction; the isolated owner admits no concurrent append. Tests inject quota counters and use sparse files for oversized apparent lengths without filling the SSD. Retained logical peak is counted, not physical APFS consumption.

Additional budget €0; standard library only, offline, no downloads/cloud/GPU/VM/Docker. New cases normally use a few KiB; one synthetic 1024-session fixture is larger but bounded, without bulk repetition; sparse files are not 8/32 MiB serializations. Timeouts 40 s isolated package and 75 s regressions, GOMAXPROCS=2, serial packages, temporary directories and run-owned processes with cleanup. No permanent cluster, prolonged stress, OS reboot or power interruption. RSS/CPU and physical SSD space unmeasured.

## Gates and official criteria

| Gate / criterion | Phase B outcome | Remaining evidence |
|---|---|---|
| C-01 / OQ-05 | RESOLVED; QSN1, IDENTITY v2 and CURRENT v1 conformance retained/verified. | No new version or migration. |
| C-02 / OQ-01 | Isolated publication/recovery VERIFIED on the local profile; overall runtime integration gate CONDITIONALLY READY. | Writer switch, live final handoff, cleanup and runtime checks. |
| C-03 | CONDITIONALLY READY; KV/sessions and final hard state preserved in isolation. | Epochs, stale completions, atomic installation and cluster safety. |
| T-015 / FR-015 / AC-015 | Codec/persistence/restore and dedup subset verified. | Lagging follower, chunks/retries, term/leader changes and installation; criterion open. |
| T-016 / FR-016 / AC-016 | Publication cutpoints and isolated process recovery verified. | Cleanup, compaction, live switch and post-success cluster crash; criterion open. |
| T-026 / NFR-006 / AC-026 | Quotas/reserves/old+new+staging accounting and session bounds verified. | New runtime admission/queues/reserve, blocked cleanup and roadmap RSS/CPU report; criterion open. |

Official definitions remain in [testing](testing-strategy.md) and [acceptance](acceptance-criteria.md). An isolated recoverable generation certifies neither compaction, InstallSnapshot, combined recovery nor client successes after cluster crashes.

## Issues, limitations and decision

No demonstrated M01–M12/Phase A defect requires changes. Development corrected a test reference to session error constants, which belong to committed rather than statemachine. Review also made hard-link/extra-file rejection and owner poisoning after failed recovery explicit. No requirement or historical fixture was adapted to make tests pass.

Filesystem syscalls can block independently of context; no I/O cancellation promise. LOCK is cooperative and requires a private directory without external changes. Successful File.Sync and directory fsync verify the protocol under process failure with the OS alive, not physical multi-file atomicity, OS/power resilience or SSD loss. Copies and hashes are not a formal application-history/consensus proof.

**Phase B: PERSISTENCE PHASE COMPLETED. M13: PARTIALLY COMPLETED.** C-01/OQ-05 RESOLVED; new isolated C-02 evidence verified, integration C-02 and C-03 CONDITIONALLY READY; T-015/T-016/T-026 open. A later assignment may authorize snapshot+suffix recovery and runtime handoff with tokens/epochs/barriers after reviewing their tests. Automatic activation, compaction/GC and InstallSnapshot remain unimplemented and require dedicated authorization and evidence. M14 not started; no Git commit.
