# M12 review and M13 snapshot prerequisites

[Index](README.md) · [Italiano](../it/pre-m13-review.md)

## Assignment, scope and decision

**22 November 2026 — CONDITIONALLY READY. M13 and M14 NOT STARTED.** Clean entry revision dbafd18. This assignment authorizes M12 review, demonstrated fixes and design contracts; not production snapshot writers, capture/export, compaction, InstallSnapshot or stateful recovery. None of those functions is implemented here.

Both roadmaps assign M13 snapshots and compaction: atomic generations, installation and crash fixtures, T-015/T-016/T-026. Requirements FR-015/AC-015/T-015, FR-016/AC-016/T-016 and the M13 NFR-006/AC-026/T-026 subset; WAL/recovery FR-013/FR-014 and sessions FR-011 are dependencies. M14 retains T-021/T-022/T-024/T-030/T-031 and the general checker; RAM/CPU measurements and benchmarks remain M15. The matrix below uses only C-01/OQ-05, C-02/OQ-01 and C-03 from the original audit. Design closure is not executable snapshot validation.

## M12 review and reproduced defects

The applyCompleted fix is consistent: M02 worker produces the result, Core.Applied advances, progress is published, then the result ring notifies waiters. The existing test gates notification and observes already-published progress; exact-slot reads, drain and cancellation regressions complement it. No machine lock held over I/O, no atomic metrics/WAL ordering imposed. This fix is unchanged. Diagnostic snapshots remain local non-linearizable copies.

Two narrow defects reproduced before correction: storage_error=true for an applier-only failure, and replay accepting a re-sealed manifest whose final outcome contradicted the trace. An atomic flag now records only fatal storage I/O failures, excluding proven quota refusal; node_error exposes every fatal failure. Both are observational. Replay also compares final outcome/failure, validates the passed/failed/inconclusive union and preserves inconclusive on budget exhaustion rather than returning success. New tests fail on the previous behavior. No consensus, domain application, persistent-format or original-fixture changes.

The simulator remains a separate binary: actual core/M02/WAL codecs, saved PRNG choices, virtual ordering, committed ghost and per-index results. Altered traces detect the first divergence; lost responses mean unknown, not rollback. ProcessCrash retains completed I/O even without Sync; SystemCrashModel all/synced is abstract exploration only. It is not a universal linearizability checker. Bounds: 5 s virtual time, 10000 events, 128 actions, 10 MiB evidence, real deadline and caller-independent cleanup.

Metrics use atomics/copies and a synchronized Recorder, with no new listener; 256-log ring and 2048-sample windows. Logs omit KV, CAS payloads, full client IDs and paths; optional four-file/1 MiB rotation in a private single-instance folder outside data. The core performs no log output. The coordinator drains every 2 s: the already-declared M12 limitation of arbitrarily blocking filesystem/stderr I/O remains, potentially delaying coordination/shutdown and not cancelable through context. It is not falsely claimed eliminated or covered by ordinary tests. It never permits ignoring errors or releasing LOCK before drain.

## Original gate inventory

| Gate / original obligation | Decision and available evidence | Missing evidence and mandatory tests | Status and residual risk |
|---|---|---|---|
| C-01 / OQ-05: byte-exact canonical schema, bootstrap, checksums and fixtures before fixing snapshot/WAL. | Existing QSN1, v1 payload/results/sessions, CURRENT and vectors; v2 compatibility/validation clarified below. Arithmetic checker and existing WAL/manifest codecs. | General positive/negative snapshot codec, v2-profile vectors, limits/corruption T-015/T-016; current snapshots are empty-only. | RESOLVED as a design prerequisite. Stateful conformance unverified; no new format/migration. |
| C-02 / OQ-01: select and locally verify macOS lock/Sync/rename, never infer durability from the Sync name. | Previously verified M03 Darwin/APFS primitives; WAL/bootstrap, PublishCurrent and errors reviewed; final protocol reuses these barriers. | New generation switch, close/directory-Sync errors, recovery and crash T-016/T-023 require M13 code. | RESOLVED for existing choices/tested primitives; CONDITIONALLY READY for the new snapshot path. No OS/power extension. |
| C-03: backlog, handoff, dedup, stale readers and checker at M07/M10/M11/M13/M14 gates. | M07/M10/M11/M12 evidence retained; drained capture, epochs and atomic switching defined below. | T-015 stale epoch/apply advancement during transfer, T-016 updated vote/publication, T-026 quotas; general T-021/T-022 remain M14. | CONDITIONALLY READY for M13. Snapshot implementation gate NOT closed; KV/session or hard-state mismatch remains a risk until tested. |

AU-001 (cursors/epochs), AU-003 (hard-state handoff), AU-005 (crash models) and AU-007 (peak space) are audit findings, not new gates. OQ-02 excludes power loss, OQ-03 retains later measurements, OQ-06 retains the deliberate session cap. No new identifier assigned.

## Applied boundary and future capture protocol

Let b be the prior snapshot base, A core LastApplied, W the worker's last actually completed command, L ScheduledApplyIndex, C synchronized commitIndex and N the last log index. During normal backlog: b<=A<=W<=L<=C<=N. W may advance before the owner consumes completion; reading A while the worker proceeds does not identify the current map. Never capture at C or L merely because they are committed/scheduled.

Minimum future M13 choice: the owner stops new scheduling and fixes B=L; it continues consuming completions and permitted events without waiting under a core lock. Drain all previously accepted work through B; submit nothing later. An explicit handshake parks the worker with no active/queued Apply, and the owner has consumed every completion: A=W=L=B. Only then do S=B and termS=term[S] identify the copy. C and N may exceed S. M02's metadata mutex or Progress() alone is not a machine barrier.

KV+sessions are copied in one serial owner control operation with the same S/termS/config/applyEpoch; immutable output and defensive copies of keys/bytes/records. This control is not a new Raft entry, consumes no index and does not put megabyte images through M02's 48 KiB command limit. M13 must introduce/test a specific control/barrier interface with one capture pending; none exists today. Admissible alternatives: copying in the parked worker, or explicit temporary exclusive ownership transfer with acknowledgment before returning ownership. The first is selected for simplicity; never concurrent map reads from handler/core.

Release the barrier after copying, serialize outside the applier and explicitly sort ASCII arrays. Failed/canceled copying publishes nothing and resumes scheduling only after safe acknowledgment; accepted Apply work is not canceled. An applyEpoch change/installation invalidates pending capture; leadership change alone does not invalidate the committed prefix. Empty state after noop/read may have S>0 and termS>0. No capture API is implemented in this review.

## KV, sessions and reads at one prefix

KV is the exact image after all and only transitions 1..S; present-empty differs from absent, with no alias to the active map. Recompute key_count and logical_bytes (sum of key/decoded-value lengths), compare captured counters and 10000/4194304 quotas. Validate bounds/overflow before allocation; one copy/serialization active, no growth per pending request. RAM peak includes live map, copy and encoded buffer; bounded by the profile but not measured here and not an RSS claim.

Session snapshot from the same transition S: at most 1024, sorted by client_id, no TTL/eviction. Exact record client_id, sequence, command_sha256, kind, result; latest admitted sequence, original QCMDv1 digest, compact result <=512 bytes with original index inside result. Retain OK, COMPARE_FAILED and RESOURCE_LIMIT as allowed by kind; never cache SESSION_LIMIT/REQUEST_CONFLICT/STALE_SEQUENCE/SEQUENCE_GAP, worker or transport errors as latest result. result.index must be positive and <=S, never replaced by S or retry index. Historical digest cannot be recomputed from current KV: later operations may have changed it. Image validation checks shape/integrity; historical correctness comes from atomic capture/tests rather than checksum.

After restore/replay S+1..C, seq=last and matching digest returns original result/index after fresh quorum/apply; changed payload conflicts, lower sequence is stale, gaps rejected, last+1 consumes sequence even for failed CAS/quota. New clients beyond capacity are deterministically rejected. KV/sessions cannot be installed in two visible phases. No general exactly-once or historical results beyond the latest.

Read and noop advance the global boundary without KV/session effects. Snapshots may end on either; do not omit their indices. They contain no GET outputs, waiter result ring, role/leader, deadline, request_id or runtime epochs. Subsequent replay applies read/noop in order without reattaching waiters: old results never become new responses. Post-restart GET requires a fresh entry/quorum; mutation retry uses restored sessions, never a definitive local response from the snapshot alone.

## Canonical format and v2 compatibility

Retain QSN1, uint64 big-endian payload length and 32-byte SHA-256 trailer over the 12-byte header plus payload; exact size 44+payload <=8388608. Whole-file digest, including trailer, identifies snapshot_id and CURRENT.snapshot_sha256; do not confuse it with the trailer. Existing exact payload: format_version=1, cluster_id, config_hash, last_included_index, last_included_term, logical_bytes, key_count, session_count, kv, sessions. KV {key,value_b64} and sessions as above; recursively ASCII-sorted object names, arrays sorted by key/client_id, canonical uint64 strings for indices/terms/counts, JSON sequence integer 1..9007199254740991, canonical Base64. No preference-driven fields.

ADR-005/ADR-010 integration choice: future operational stateful images use the session-aware v2 profile, identified by config/IDENTITY v2 and config_hash including application_version=2. Snapshot container/payload remain v1 as already reserved by ADR-010; profile version is not container version. Existing v1 vectors remain syntax/stateless tests and do not authorize opening legacy data with session semantics. Legacy v1 runtime retains empty bootstrap/base-zero operation; no automatic migration or reinterpretation of v1 logs. These already-approved fields need no new version; any incompatible change requires the existing versioning process.

The future reader validates lengths before allocation, exact EOF/no trailing data, magic/version, digest, canonical JSON/UTF-8, exact names/no nulls, sorting/uniqueness, counts/sums/quotas and kind-consistent result unions. S=0 requires termS=0 and empty state; S>0 requires termS>0 even for empty KV. termS<=recovered currentTerm. Truncation, bad checksum, unknown version, mismatched identity/config/boundary or session-result index outside [1,S] fail without effects. Approved conservative bound 6782978 bytes, below 8 MiB; not a RAM/APFS-allocation measurement. No new snapshot fixture or codec created here: existing arithmetic checks and future M13 checks remain distinct.

## Metadata, WAL suffix and final barrier

Snapshot stores application state and S/termS, not currentTerm/votedFor. IDENTITY binds node/cluster/config; CURRENT selects exactly one generation and repeats only base_index/base_term and whole-file hash with its own checksum. Generation WAL is the sole authority for current hard state and C checkpoint: first frame seq=1 contains updated hard_state and commit=S, no replace_suffix; then contiguous S+1..N entries in transactions <=16 entries/256 KiB and payload <=384 KiB, followed by checkpoint C. Validate S<=C<=N, monotonic terms and vote preservation. If S=MaxUint64, suffix is empty without overflowing S+1. No applied-index file or parallel session database.

After capture at S, application/log may advance while serializing the copy. Before final WAL construction, storage ownership suspends new proposals/persistent transitions and other rotations, drains accepted old-generation I/O, and fixes updated T=currentTerm, V=votedFor, C, N. Votes/terms processed after capture must be included; never use S-time hard state. Not-yet-processed events remain queued and persist to the new WAL after handoff. No map/core lock held while waiting for I/O; no new dependent acknowledgment during switching. A previously authorized acknowledgment is not revoked.

Local rotation does not replace the live map: it may already be applied beyond S, so never resets A/L to S or reapplies a prefix. Update log base/term and lookups, retain commit/proven progress; peer nextIndex<=S requires snapshot transfer rather than fabricated match advancement. Invalidate/realign active RPCs needing discarded prefixes. Check I/O (generation_id,write_sequence) tokens and capture/apply epochs: reused sequence in another generation authorizes nothing. Local switching without state replacement needs no new applyEpoch; installation does. No switch API is added now.

## Publication, errors and quotas

Ordering derives from the storage contract, not rename alone: (1) under stable LOCK allocate a never-used gen-N and reserve peak space; (2) create private same-filesystem gen-N.tmp, fully write snapshot/final WAL, validate contents/references, Sync files and check relevant Close errors; (3) fsync staging directory; (4) rename to gen-N and fsync root; (5) construct CURRENT v1 with S/termS and whole-file digest, write new 0600 CURRENT.tmp, validate/Sync; (6) rename over CURRENT and fsync root; (7) switch writer/core to new-generation tokens, close old writer after drain/check errors; (8) only then resume permitted participation/acknowledgments and clean recognized obsolete generations, then fsync root. CURRENT selects the generation; full protocol, not rename alone, establishes success.

Before CURRENT rename the old generation remains authoritative; new staging/gen-N is not selected. If CURRENT rename succeeds but directory Sync fails: fatal/uncertain, no acknowledgment or rollback, retain both generations and recover the CURRENT actually present. If runtime handoff fails after publication, never answer from old state; stop and recover the authoritative new generation. Post-publication cleanup failure does not invalidate the valid snapshot: retain/count residues and stop further rotations/admission according to quota, never delete active data for space. Never ignore Sync/Close errors. Cancellation bounds a waiter rather than interrupting/duplicating an uncertain syscall.

Unchanged quotas: 8 MiB snapshot, 8 MiB WAL including 512 KiB consensus reserve, 32 MiB directory including old/staging and 64 KiB control reserve. Accounting includes actual existing bytes, snapshot copy, final WAL and control files; two 8+8 MiB maxima do not automatically leave room for two generations plus controls. If peak does not fit, refuse before writing/deleting; serial storage ownership enforces reservation against concurrent appends during preparation. Filesystem ENOSPC remains possible: accounting does not reserve APFS blocks against other processes. External logs have a separate cap. Existing triggers: 6 MiB WAL or 2000 entries, applied progress and minimum 30 s interval; installation/compaction mutually exclusive. No SSD-filling tests.

## Future recovery and InstallSnapshot

Future recovery: acquire exclusive LOCK and validate IDENTITY/config/profile before file mutation; read/validate CURRENT <=4096 bytes and only its referenced generation. Validate full snapshot, hash/base/config and associated WAL, including termS/hard-state consistency; never choose the highest gen-N. Missing/corrupt CURRENT, incomplete referenced generation or bad checksum fail explicitly: no silent fallback, reset or rebootstrap. Staging/orphans may be cleaned only after authority validation and ownership/quota checks.

Reconstruct WAL before exposing state: final hard state, log S+1..N and valid C checkpoint. Restore S on a fresh KV/session pair and apply only S+1..C, in order through M02/adapters; C+1..N stays speculative. Initialize A=L=S and advance completions through C before listeners; recover term/vote from WAL, follower role, fresh runtime identity. Only structurally incomplete final WAL tails may be repaired/Sync'd; complete corruption is fatal. Current runtime rejects nonzero bases with ErrSnapshot: this path is unchanged.

InstallSnapshot is mandatory M13, not anticipation: transport is already specified in network-protocol. One transfer per node, raw chunks <=64 KiB, total image <=8 MiB, whole-file ID/digest, exact offsets or verified identical retries, 5 s staging timeout and quotas; conflicting bytes or invalid term/leader invalidate staging without touching active state. Chunk acknowledgment proves neither replication nor installation.

Before final installation: stop new scheduling, fully drain accepted work/completions, then revalidate term/leader, capture/apply epoch and S against current A. S<=A is obsolete without replacement; a previously started transfer may become obsolete during drain. For S>A require local (S,termS) matching to retain S+1..N; preserve checkpoint C if greater than S. Mismatch with S<=C is an integrity error, never committed-data loss. Mismatch with S>C permits discarding the uncommitted suffix and setting C=N=S while preserving updated local hard state. Never copy a leader vote from the snapshot.

After both barriers and durable publication, replace KV+sessions together, set A=L=S, increment applyEpoch with overflow checking, invalidate old completions/result ring/waiters (unknown or superseded, no invented result), then resume retained committed replay/scheduling and send installed_index=S. Crash between CURRENT and RAM recovers the new snapshot without serving the old map. Leader advances match/next only for a valid correlated final acknowledgment; lost acknowledgment uses obsolete/resume followed by verified AppendEntries. After compaction retain termS as the sentinel and a complete transferable image for lagging followers; local existence never proves follower possession.

## Mandatory future M13 matrix

Every row is **NOT RUN for M13 snapshots**. These are subcases of official tests, not new IDs. Current M12 simulation rejects snapshot/generation actions; an M13 test adapter will be needed, not a fault switch in the ordinary server. Complete trace; exhausted budgets mean inconclusive.

| Official test / point | Fault or interleaving to construct | Required result |
|---|---|---|
| T-015 / T-008 | Backlog C=7, A=0, L=5; capture requested before completions. | Capture only after draining B=5 and A=W=L=5; never snapshot at 7 or a map beyond S. |
| T-015 / T-011 | Successful/failed/quota CAS, retry and 1024 sessions; crash after lost response. | Same-S KV/sessions, original index/failure retained, deterministic stale/gap/conflict/cap after restore+suffix. |
| T-015 / T-019 | S ends on read/noop; prior pending GET and new post-restart GET. | No index gap, no old GET result delivered as new, fresh quorum barrier. |
| T-015 | Capture error/cancellation, mutation of returned buffers, different map iteration order. | No publication; ownership acknowledged/restored, deterministic bytes and nonaliasing copies. |
| T-015 / T-016 | Truncation, checksum, length/version, counts, duplicate arrays, profile/hash mismatch. | Reject without partial state/over-cap allocation; original v1 fixtures unchanged. |
| T-016 / snapshot.sync | Short/zero write, ENOSPC, failed snapshot/WAL/staging-directory Sync or Close. | No acknowledgment/publication; old valid CURRENT, uncertain/fatal error and counted residues. |
| T-016 / generation.rename | Crash before/after generation rename/root Sync, before CURRENT. | Old CURRENT selects old generation; staging/unreferenced generation never selected. |
| T-016 / current.rename | CURRENT.tmp written/Syncd, crash before/after rename; failed directory Sync. | Old selection before rename; afterward use surviving valid CURRENT, no rollback/ack on failed barrier. |
| T-016 / AU-003 | Vote/term updated after S capture but before final barrier; crash after switch. | New WAL retains final hard state rather than S-time state; no second same-term vote. |
| T-016 / cleanup | Removal/cleanup-fsync error; crash between publication and RAM/writer handoff. | New CURRENT stays authoritative; no old-state serving/committed loss; never delete active data for quota. |
| T-015 / AU-001 | Apply advances during transfer; old-epoch completion after install; old-generation token. | Recheck obsolete, drain before replacement, reject stale completions/tokens without changing cursors/sessions. |
| T-015 | Far-behind follower, identical/conflicting chunk retries, term/leader change, lost final acknowledgment. | Bounded staging; no match from chunks alone; correlated final install or obsolete+verified AppendEntries. |
| T-015 / T-014 | Restore with committed/speculative suffix; mismatch within commit; repeated recovery. | Apply only S+1..C, retain suffix on match, reject committed loss; identical KV/session pair on repeated restart. |
| T-016 / T-023 | Crash one/all three processes after success with published snapshot, CAS retry and fresh GET. | Successes/dedup preserved with recoverable majority; no OS/power-crash equivalence. |
| T-026 / AU-007 | Old+new+staging/control peak, insufficient quota, consensus reserve, blocked cleanup. | Refuse before side effects, no 32 MiB overflow or deletion needed for recovery; full RAM/CPU evidence remains M15. |

Proposed initial future-phase budget: in-memory 3-node models, 5 s virtual/10000 events; isolated file tests <=16 cutpoints, <=16 KiB synthetic contents/case and <=1 MiB deliberate writes total, <=30 s/case and <=120 s suite, <=2 storage processes or at most 3 distributed nodes. Simulate full quotas rather than filling disk; run-owned directories and safe cleanup only. These tests are NOT executed here. Larger real images require an explicit future M13 I/O budget; RSS/CPU measurement is outside this review.

## Checks actually executed

Go 1.27.4 darwin/arm64, existing tools: GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2, normal CGO_ENABLED=0 and race 1; build GOOS=darwin/GOARCH=arm64. No downloads or external networking. Equivalent commands actually executed:

```sh
go test -p 1 -count=1 -timeout=40s ./internal/diagnostics ./internal/faultsim ./internal/storageformat ./internal/storagefs ./internal/wal ./internal/applier
go test -p 1 -count=1 -timeout=40s ./internal/raft ./internal/clientapi
go test -p 1 -count=1 -timeout=30s ./internal/statemachine ./internal/committed ./internal/config ./internal/strictjson
go test -race -p 1 -count=1 -timeout=30s ./internal/raft -run 'Test(MetricsDistinguish|AppliedStatus|Diagnostics|LoggedReadCaptures)'
go test -race -p 1 -count=1 -timeout=30s ./internal/faultsim -run 'Test(ReplayRejects|ReplayPreserves|SavedManifest|ScenarioValidation)'
go test -race -p 1 -count=1 -timeout=30s ./internal/node ./internal/server -run 'Test(T017LocalMetricsRemainAvailableWithoutQuorum|DiagnosticRotationLifecycleAndDataIsolation)'
go vet ./...
go build -p 1 ./...
gofmt -l cmd internal
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

The complete bounded package suites above and targeted race checks passed: progress/result ordering, exact-slot read, storage/applier failure distinction, saved-fixture replay, first divergence/inconclusive budgets, log redaction/bounds, HTTP metrics without quorum and shutdown/data reopening. Both reproductions first failed: storage_error=true for application failure and altered-final-outcome replay accepted without error. They pass after correction, together with the added reproduced-inconclusive/invalid-outcome test. Three new top-level tests in two files; no historical expectation changes.

The entire M01–M12/process suite was not rerun: selected relevant regressions rather than stress. No new coverage/performance measurements; historical M12 percentages are not results of this assignment. gofmt found no files needing correction; vet/build and canonical checker passed; the latter checks vector arithmetic rather than snapshot recovery. No M13/InstallSnapshot/compaction or OS/power tests. At most three temporary localhost nodes in integration, serial packages and test-verified cleanup; no cluster left running.

## Decisions, files and bilingual conformance

No new ADR/gate. Clarified ADR-005/ADR-010 and existing canonical contract: same format, v2-profile binding, capture after drain/parking, updated hard state at final fence, authoritative generation/no fallback, peak quota and epoch-aware installation. Capture interfaces, general codec, switch and restore remain M13 work. M01 has no export, M02 no capture, WAL opens only empty/base-zero snapshots and Core.EnableCommit rejects nonzero base: existing guards unchanged.

Production changes only in internal/raft/service.go and diagnostics.go (observational storage flag/node_error), internal/faultsim/runner.go and types.go (replay-outcome validation/comparison). New internal/raft/diagnostics_review_test.go and internal/faultsim/review_test.go; docs/openapi/m12.json specifies the two optional booleans without endpoint/request-schema/required-field changes. No snapshot code, new writer/persistent file/migration, dependency or go.sum.

Created both pre-m13-review.md files. Updated both languages' canonical-formats, architecture, data-model, storage-and-recovery, failure-model, testing-strategy, acceptance-criteria, architecture-review, requirements, traceability, network-protocol, observability/faults, guide, roadmap, indexes, ADR-005/ADR-010 and snapshot diagram note. Main READMEs link the report; historical M11/M12 reports and original fixtures not rewritten. Structural checks across 136 Markdown files/67 language pairs, sections, requirement IDs, technical blocks, tables and relative links found no errors; semantic equivalence reviewed. No external certification or formal proof.

## Eligible work, limits and final decision

**CONDITIONALLY READY.** A future M13 assignment may initially authorize stateless-codec conformance with positive/negative vectors, in-memory KV/session capture and handshake/epoch models, and isolated publication/error checks using existing primitives. Before integrating/activating writer, compaction, InstallSnapshot and stateful recovery in the runtime, the corresponding T-015/T-016/T-026 matrix evidence is required, including updated vote, committed suffix, nonaliasing capture and quotas. This is not a new gate: it stages existing C-01/C-02/C-03. This assignment does not authorize even the initial M13 phase.

Listed implementation validations remain open, rather than fundamental format/ownership/authority decisions. No claim that snapshot crash consistency is already verified. Durability contract: process crashes with live OS, intact directories and recoverable majority, conditional on the future protocol/tests. Atomic rename is not data/reference durability; abstract models and Sync do not prove OS/power crashes, shared-SSD failure or durable-majority loss. Known blocking diagnostic-output limitations are explicit above, not a universal shutdown promise.

Additional spending **€0**, existing local Go/standard-library tools only, no infrastructure/cloud/purchase, stress, benchmarks, GPU, mandatory Docker/VM, SSD filling or destructive tests. Synthetic data, timeouts and run-owned temporary directories. No Git commit. **M13 and M14 NOT STARTED; stop at CONDITIONALLY READY.**
