# Versioned application profile for replicated sessions

[Index](../README.md) · [Italiano](../../it/adr/ADR-010-session-profile.md)

**ADR-010 — Accepted M11 design, 22 November 2026; implementation evidence is recorded in the M11 report.**

## Problem

M08–M10 persisted client_id/sequence as metadata without deduplication. Replaying those same committed bytes with session rules can remove previously acknowledged effects. Silently enabling sessions over a v1 directory is unsafe. A leader-only cache cannot fix this.

## Decision and compatibility

Add an explicit configuration v2 requiring application_version=2, binding that field into the shared config_hash. A v2 node uses checksummed IDENTITY format_version=2 with the same identity fields; its version denotes session semantics from the empty origin. Existing configuration v1 and IDENTITY v1 retain their exact bytes and M10 semantics. The existing node executable selects the profile from configuration; no rolling negotiation or automatic conversion.

M11 requires a new cluster/private empty directories with explicit bootstrap and all three v2 members. Opening either identity profile with the other fails before WAL repair/replay/listeners; shared hashes also prevent mixed-profile peer RPC. Preserve existing v1 directories and operate them only as the legacy M10 profile. No in-place migration, copying of active data or silent reset. An offline export/import migration would need a separate design and is not implemented here.

WAL frames/transactions, commands/digests, CURRENT and empty snapshot remain approved v1 encodings. No new replicated field is necessary: identity and sequence already exist. Mutations in profile v2 additionally require HTTP header `X-QuorumKV-Application-Version: 2`; v1 rejects that header, and v2 rejects an absent/other version before admission. The test client derives it from configuration. V2 mutation responses acknowledge the same version header and the M11 client rejects a missing/mismatched acknowledgment as unknown. Historical binaries may ignore unknown request headers, so no cross-version completion is trusted. GET remains unchanged. This prevents an old client from silently treating a repeated sequence as a new operation on a reused endpoint. The new identity version changes the application contract explicitly, rather than changing old byte meanings silently. A v2 empty snapshot has no sessions; full snapshots/compaction remain M13.

## Ownership and results

A private committed-command envelope carries bounded canonical entry metadata through the unchanged M02 worker to a session wrapper around unchanged M01. The wrapper atomically checks identity/digest, applies at most once and stores sequence/digest/latest compact result/original index. Only committed work reaches it. Fresh replay rebuilds map and sessions together; there is no second persistent session database. Duplicate entries still advance actual apply indices. Completion/leadership fences use the new committed attempt index/term; HTTP reports the original cached result index.

## Limits and validation

At most 1024 sessions, no TTL/eviction, one latest result <=512 bytes each; stale sequences never reapply. The compact result contains no old values. Unknown outcomes remain possible and every retry goes through a fresh leader/quorum/apply attempt. Test old/new rejection without file changes, canonical v2 identity/configuration vectors, concurrent duplicate CAS, payload conflicts, ordered errors, failover and all-process recovery. Preserve all original fixtures. No universal exactly-once, OS/power-loss or M12/M13 claim.

## Pre-M13 design decision — 22 November 2026

The [pre-M13 review](../pre-m13-review.md) retains QSN1/payload v1 with complete sessions and v2 identity/config for future stateful runtime; no v1 migration or vector changes. Select capture after worker drain/parking and atomic KV/session copy, a separate final barrier with updated hard state, authoritative CURRENT without fallback, peak reservation and InstallSnapshot within M13. Design contract clarified; general writer/codec/install/snapshot recovery unimplemented and T-015/T-016/T-026 unvalidated. C-01 design RESOLVED, C-02 extension and C-03 snapshot CONDITIONALLY READY; no OS/power-durability acceptance. M13 NOT STARTED.

## M13 Phase A isolated conformance — 22 November 2026

[Phase A](../milestone-m13-preliminary.md) implements QSN1/payload v1 and owner-only KV/session copies with v2 binding and fresh/isolated-only restore. No added version/migration; earlier vectors unchanged. Confirm separate worker control rather than indexed commands, B drain/coherent proof before copy, encoding after Release. Isolated C-01 conformance verified; no acceptance of runtime generations, InstallSnapshot, compaction or crash consistency. Complete C-02/C-03 and T-015/T-016/T-026 remain open.

## M13 Phase B isolated persistence — 23 November 2026

[Phase B](../milestone-m13-persistence.md) verifies isolated QSN1/CURRENT build/publication/recovery, updated final vote and complete v2 sessions. Existing format/protocol retained without third manifest or migration. New isolated C-02 evidence VERIFIED; integration C-02/C-03 CONDITIONALLY READY. Cleanup, runtime switch, suffix replay, compaction and InstallSnapshot unimplemented; full T-015/T-016/T-026 open, no OS/power claim.

## M13 Phase C runtime recovery — 23 November 2026

[Phase C](../milestone-m13-runtime-recovery.md) adopts base-S recovery and committed-suffix-only replay on fresh KV/sessions before listeners, through explicit v2 capability and continuous LOCK. Final-WAL term/vote, A=L=C cursors, fresh initial epoch, no historical result ring. Default base-zero path unchanged; no new format/configuration or migration. C-02 runtime recovery verified only within its subset; C-02/C-03 remain CONDITIONALLY READY for live cycle, compaction/installation and epochs. Full T-015/T-016/T-026 open, no OS/power guarantee.

## M13 Phase D isolated compaction — 23 November 2026

[Phase D](../milestone-m13-compaction-preliminary.md) adopts the existing representation without new fields: QSN1 snapshot/base-S WAL, final hard state, complete suffix and CURRENT v1 with v2 identity. The new assignment permits non-destructive isolated verification; no further format decision needed. Capture derives from actual apply; KV/sessions/indexes/suffix compared before/after B/C publication. Non-repairing reader only for D proof; ordinary behavior unchanged. Overall C-02/C-03 CONDITIONALLY READY and T-015/T-016/T-026 open. Equivalence alone authorizes no live activation, GC, deletion or InstallSnapshot.

## M13 Phase E isolated installation — 23 November 2026

[Phase E](../milestone-m13-installsnapshot-preliminary.md) is **ISOLATED INSTALLSNAPSHOT PHASE COMPLETED**: dormant receiver/controller, existing M04 chunks, real M05 term/vote persistence, coherent local suffix, B publication under continuous LOCK, fresh KV/sessions and apply epoch, correlated final ACK and deterministic suffix catch-up verified in isolation. No ordinary node/service/server/transport integration. C-01/OQ-05 confirmed; isolated C-02/OQ-01 and C-03 evidence added, overall both CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016 and T-026/AC-026/NFR-006 remain open. Runtime InstallSnapshot, live compaction, GC and prefix deletion remain unactivated; M13 PARTIALLY COMPLETED, M14 not started.

Existing decisions apply unchanged: one canonical publication protocol, updated **local** term/vote, matched-boundary suffix preservation, atomic KV/session restore after CURRENT, no partial ACK as replication proof. Stable LOCK is lent without release; old apply completes before replacement, fresh epoch rejects stale completions. No runtime compaction/GC authorization and no v1 migration.
