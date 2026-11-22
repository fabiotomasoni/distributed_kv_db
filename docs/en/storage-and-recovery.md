# Persistence, snapshots and crash recovery

[Index](README.md) · [Italiano](../it/storage-and-recovery.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Contract and assumptions

Minimal custom engine: in-memory map, append-only WAL and filesystem snapshots; no external database. Primary target: process crashes, with completed write/Sync/rename operations honored by the operating system. Do not equate `kill -9` with power loss. Hardware guarantees against sudden power failure depend on APFS, macOS, controller and SSD caches and remain **Proposed**, without destructive testing on the personal computer.

[Go File.Sync](https://pkg.go.dev/os#File.Sync) exposes synchronization but is not end-to-end proof of physical-fault resilience. Filesystem operation ordering matters: see [Pillai et al., OSDI 2014](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/pillai). Verify directory synchronization support and behavior on darwin/arm64 in M03; never ignore errors. Any need for additional Darwin primitives requires ADR-005 review and a dependency inventory.

## Directory, identity and proposed WAL format

Private 0700 directory, 0600 files, one active owner through an exclusive process lock (future macOS adapter; crash tests verify lock release). Immutable identity manifest contains version, cluster_id, node_id and config_hash. Never copy an n1 directory to start n2. Future files: `IDENTITY`, `CURRENT`, and `gen-N/` generations containing snapshot and WAL; none are created in this phase.

WAL v1 frame: 24-byte big-endian header: magic `QWAL` (4), version uint16 (2), kind uint16 (2), payload length uint32 (4), frame sequence uint64 (8), CRC32C (4). Checksum covers the first 20 header bytes and payload; it is not authentication. Kind 1 = transaction; canonical UTF-8 JSON payload, maximum 384 KiB, decimal-string indices/terms. Physical sequence is contiguous from 1 per generation.

Transaction has optional `hard_state:{current_term,voted_for}`, `replace_suffix:{from_index,entries}`, `commit_index`. Fully validate a frame before installing all its effects during recovery, never half a transaction. Replacement is valid only beyond current commit, with contiguous entries; it may replace a suffix with nothing. The physical WAL retains logically invalidated old records: no in-place truncation for Raft conflicts. Compaction recreates a WAL containing only the live suffix.

Unknown version, inconsistent index, regressing hard state, bad checksum or commit beyond log are fatal errors. Structural incompleteness **only in the final frame** may be truncated to the last valid boundary and synchronized, within the process-crash model excluding arbitrary corruption. A complete frame with bad CRC is not “repaired” by discarding it. A corrupt header might resemble an incomplete write: thus automatic bit-rot recovery is not guaranteed and suspected corruption requires fail-closed behavior. Never skip records searching for the next magic.

## Mandatory persistence ordering

| Event | Before responding or publishing |
|---|---|
| Term increment / vote | Hard state written and successful Sync. |
| AppendEntries success | Relevant term/vote and log changes in synchronized WAL. |
| Leader counts local replica | Frame containing local entry already synchronized. |
| Commit/apply | commitIndex checkpoint synchronized after required entries; only legally learned commit. |
| Completed client response | Entry on durable majority, legal commit and completed apply; dedup reconstructible. |
| Snapshot installed | Verified image and durably published generation; never acknowledge from staging. |

Persisted `commitIndex` is a conservative monotonic checkpoint, not a way to infer commit from log.length. Hard state and log may share a frame. The checkpoint is not replicated as a command and does not replace Raft rules. Never succeed on short writes, ENOSPC, failed Sync or relevant close failure. Fatal storage: stop voting, proposing and acknowledging; diagnose and explicitly restart after resolution.

## Recovery

1. Acquire lock and validate identity/configuration; no client listener before verification.
2. Read CURRENT and validate generation, snapshot and WAL. Missing CURRENT in a non-empty directory is an error, not permission for a new cluster. No automatic fallback to an old snapshot if the current generation is corrupt.
3. Load snapshot: map, quota counters, session table with result and digest, lastIncludedIndex/Term, cluster/config and version. Verify SHA-256, counts and limits.
4. Replay valid frames to reconstruct hard state and log suffix, retaining the maximum legally checkpointed commit. Initialize lastApplied to the snapshot index and apply through persisted commit. Never apply the remainder.
5. Start as follower with a fresh timer; the leader will communicate further commit. A restarted cluster may recommit a durable prefix using a current noop. Answer GET only after a new barrier, never from replay alone.

Permanent loss of a durable majority removes any preservation or automatic recovery guarantee. Quarantine a corrupt single directory; manual restoration from a verified healthy-peer snapshot requires preserving evidence and correct identity first. Never change membership or independently form a quorum from empty nodes.

## Snapshots and atomic publication

Capture at S=lastApplied, necessarily committed. Under an applier barrier capture map+sessions+S+term[S]+config, then use an immutable copy. Image v1: header `QSN1`, big-endian uint64 length, canonical JSON payload (sorted keys, Base64 values), 32-byte SHA-256 trailer over header+payload. Maximum file 8 MiB. Noop and read leave no user data in the snapshot.

Generation procedure: (a) reserve space; (b) create gen-N.tmp, write snapshot and WAL containing suffix S+1..last, hard state and commit checkpoint; (c) synchronize files and temporary directory; (d) rename to gen-N and sync parent directory; (e) write CURRENT.tmp with format `{format_version:1,generation:"gen-N",snapshot_sha256,base_index,base_term}` and a SHA-256 checksum of the canonical encoding of the preceding fields, Sync, rename over CURRENT and sync parent; (f) only then delete the previous generation and sync parent. Published generation files are immutable except WAL append; during final brief rotation the storage worker serializes append and suffix copying, preserving entries arriving during snapshot serialization. Failure before publication leaves old CURRENT; unreferenced temporaries can be removed after validation.

InstallSnapshot uses the same mechanism. If `(S,termS)` matches the local log, retain the later suffix; otherwise discard it only if local commit is not destroyed. Reject obsolete snapshots `S <= lastApplied`; never regress commit. If S exceeds local commit, a valid leader snapshot establishes a committed prefix and commit at least S. Coordinate applier and core before replacing state; no parallel apply of the old suffix. Term/vote persistence must survive generation changes.

## Compaction and space

Trigger at 6 MiB physical WAL or 2,000 entries since the last snapshot, only if lastApplied advances; one active compaction, minimum 30 s interval. Hard WAL cap 8 MiB including a 512 KiB metadata/consensus reserve; when data space is exhausted reject new proposals and continue replication/commit within the reserve. If no prefix can be compacted and the reserve runs out, fail storage closed: never delete entries to create space.

Aggregate per-node directory cap 32 MiB **including** staging and old generations; reserve peak space before starting. Snapshot installation and compaction never run concurrently. Replicated logical caps differ from local physical caps: a full dataset produces deterministic RESOURCE_LIMIT; a full disk makes one node unavailable. Complete limits and the roughly 100 MB cluster target are in [resources](performance-and-resources.md).

## Required cutpoints

Crash before/after header write, payload, Sync, commit checkpoint, generation rename, CURRENT rename, directory sync and cleanup; include crash after client success and retry after recovery. Snapshot tests also cover incomplete transfer, bad checksum, duplicate chunks and changed leadership. Tests must inspect recovered files and acknowledged history; absence of panic is insufficient.

## Bootstrap and format validation

On a truly empty directory with explicit bootstrap: create durable IDENTITY, an initial generation with an empty `(0,0)` snapshot, hard state `(term=0,votedFor=empty)`, commit 0 and initial WAL sequence; publish CURRENT using the same procedure as later generations. A mid-bootstrap crash leaves an incomplete directory: ordinary startup rejects it, and a future explicit resume command may complete it only after validating files, never silently delete it.

CURRENT is limited to 4 KiB and has no self-referential checksum: digest the data fields only, then add the checksum field. base_index/base_term match the image; snapshot_sha256 digests the entire image file. Index/term numbers remain strings. The [canonical specification](canonical-formats.md) now fixes byte-for-byte fixtures for headers, JSON, digests and recovery expectations; M03 must test its reader against them. Serialization cannot rely on unordered map output.

## Generation-switch and installation barrier

Snapshot-copy serialization at S may run while the core progresses; the **final rotation** is a serialized transition. Before copying the final suffix: suspend proposals, persistent transitions and concurrent compaction; drain persistence already submitted to the old generation and capture updated `(currentTerm,votedFor,commitIndex,lastLogIndex)` from the core. Never copy hard state captured at snapshot start.

While holding this barrier, write the live suffix S+1..lastLogIndex, current hard state and commit checkpoint into the new generation, then synchronize and publish CURRENT as specified. No vote or append is acknowledged during the switch. Release the barrier only after publication and writer switching. Every storage request/completion uses `(generation_id,write_sequence)`; sequence alone, which restarts at 1, does not globally identify an effect. An old completion appearing after the switch cannot authorize messages or mutations; it signals a coordination bug.

For InstallSnapshot: revalidate term, epoch and index when **both** storage and applier barriers are quiescent, after draining or invalidating old work; if S is now <=lastApplied, do not install. Synchronize the applier as in [architecture](architecture.md), preserve current local hard state, retain a suffix only with a matching `(S,termS)` boundary and never discard commit. If lastApplied<S<=commitIndex and the boundary mismatches, fail closed. Publish persistent state first, then replace the map/epoch in RAM under the barrier, then acknowledge installed_index and reopen scheduling. A crash between publication and RAM replacement recovers the new generation: never answer using the old map.

T-016 must introduce a vote or term advancement between capture at S and the final barrier, then crash after switching: the updated vote must survive. T-015 must advance apply during transfer and delay an old applyEpoch completion.

## Durability levels and write reduction

| Failure | Design contract | Evidence still required |
|---|---|---|
| Process crash, OS alive | v1 target: preserve successes with a recoverable majority and coherent completed I/O; non-Sync writes may survive. | T-013/T-016 and controlled process kills T-023, not yet executed. |
| OS crash/reboot | No already-validated guarantee; the Sync/rename protocol targets this level subject to the macOS contract. | Platform sources and simulation are not tests of actual system crashes. |
| Power loss / SSD failure | Outside the v1 guarantee; depends on caches, controller and the device shared by all replicas. | No destructive tests required or performed. |

Retain necessary Sync for votes, hard state, replication and snapshot publication. Never sync identical heartbeats without persistent changes; never rewrite already-identical entries; checkpoint commit only when it advances. Combining hard state, append and learned commit in one frame is allowed when causal order permits; never acknowledge early while awaiting a batch. Synchronous commit checkpointing is an initial conservative choice, not a universal Raft requirement: retain it until an ADR and recovery tests justify an alternative.

Logged GET adds entries and normally checkpoints on each replica learning commit, so even a read-only workload has nonzero I/O. Measure bytes written and Sync count per logical operation at M15; first prefer safe batching within deadlines, sampled diagnostic logs and small workloads. ReadIndex is the future read-specific optimization under ADR-006; never remove required synchronization to improve a measurement.

Final image validation compares cluster/config, included index/term, length and digest against the RPC envelope; disagreement means rejection, never partial installation. The leader advances matchIndex through S and nextIndex to at least S+1 only for a correlated final `installed_index=S` acknowledgment, with term/generation still valid and durable publication completed; chunk acknowledgments alone do not establish replication. A lost acknowledgment can be resolved through the obsolete-snapshot path followed by verified AppendEntries.

## Normative format and provisional platform contract

The [canonical v1 specification and checked fixtures](canonical-formats.md) now fix the storage schema: recursive ASCII field ordering, kind/nullability, typed command digest, entry/result/session objects, snapshot arrays/counters and checksummed IDENTITY/CURRENT. That document supplies exact framing and bootstrap/recovery expectations; this page retains the ordering and generation protocol. No storage codec exists.

The [macOS contract](macos-storage-contract.md) records File.Sync’s actual Go 1.27.4 Darwin path, stable advisory locking and a directory-fsync requirement awaiting separate tests. If CURRENT rename succeeds but its directory barrier fails, publication is uncertain/fatal: preserve both generations, do not roll back or acknowledge. C-01 is resolved at design level; C-02 remains conditional pending actual adapter evidence. [Current review](pre-m03-review.md). All storage tests and durability acceptance remain future work.

## Preliminary implementation boundary

[Phase evidence](milestone-m03-preliminary.md) verifies the C-02 local primitive contract and stateless QWAL/control-file codecs. No full append/replay/generation engine is available. PublishCurrent synchronizes one validated control file; its caller must already have synchronized/validated the referenced generation. A frame checksum or successful fsync is neither proof of legal commit nor full recovery correctness. M03 remains PARTIALLY COMPLETED; retain all later storage/snapshot/durability gates.

## Current local WAL and recovery — M03 completed

[Full local implementation and tests](milestone-m03.md) now provide explicit empty bootstrap, checksummed identity/CURRENT coordination, serial Sync-before-token append and bounded streaming hard-state/log/commit recovery. Ordinary Open selects only CURRENT, checks identity/checkpoint and never falls back. An incomplete final frame after a fully verified initial prefix is truncated and synchronized; any complete corruption/state violation or read error fails closed without repair. Recovered complete non-Sync frames are re-synchronized before use.

Recovery does not apply KV commands; callers must later coordinate commit proof, sessions/noops and the applier. M03 supports only the mandatory empty checkpoint; nonzero snapshot bases fail explicitly pending M13. AppendProposal reserves 512 KiB inside the 8 MiB WAL cap; Append can use it for replication/control. All node files count toward 32 MiB with 64 KiB growth margin. No compaction/delete-for-space: reject at the cap. Full server crash recovery and durability remain M09, general snapshots/rotation M13. Earlier design/preliminary statuses in this document are historical.

## M06 use of existing suffix transactions

[M06](milestone-m06.md) reuses replace_suffix without changing storage code or bytes. Retain the committed prefix, replace from the first uncommitted conflict in a synchronized transaction and install RAM/ack only after its correlated token. Large legal RPC suffixes split at the existing per-transaction entry-byte cap; a crash can leave a complete prefix of these frames. No final RPC success precedes every required Sync; recovery validates each surviving whole frame and retry resumes idempotently. Local proposals use AppendProposal; replication/control use Append. Known commit_index is unchanged, no apply or new checkpoint transition.

## M07 checkpoint and fresh replay

[M07](milestone-m07.md) emits existing commit_index frames only after covered durable entries and legal leader/follower proof. Sync failure stops progress/application; complete surviving checkpoint recovery remains conditional on M03 validation. No applied-index file is added. StartCommitted reconstructs a fresh base-zero in-memory machine through M02 over exactly 1..recovered commit, ignoring speculative suffix. Replay rebuilds both application cursors before listening. Full process recovery/MVP, sessions and general snapshots remain M09/M11/M13; OS/power loss remains unverified.

## M09 process recovery evidence

[M09](milestone-m09.md) completes the internal-MVP T-014/T-018/T-023 subset with real process termination and fresh replay. Acknowledged entry bytes/checkpoints are recovered from intact directories; actual M07 reconstruction through M02 is compared with an external expected map only after stopping its owner. Graceful shutdown joins work before releasing ports/LOCK, and signaled pending writes remain uncertain. Missing initialization, duplicate locks, repeated bootstrap and complete corruption fail closed before serving. Existing M03 repair rules, reserves, canonical bytes and base-zero limitation are unchanged. Full AC-023 end-to-end GET remains after M10, snapshot publication/recovery M13. OS crash/reboot and power-loss durability are still unverified.

## M10 read replay and bounded cost

[M10](milestone-m10.md) reuses the already-approved v1 read command and digest bytes. Successful GET requires an ordinary synchronized proposal, durable quorum and commit checkpoint; failed/wait-abandoned reads may leave entries. Fresh recovery replays committed reads at their exact slots on the new machine, without modifying KV state or applying speculative suffixes. A real three-process crash/restart test reads an acknowledged value and missing key through new HTTP barriers after recovery. No read result is a persisted client/session guarantee. Reads consume the existing WAL cap/reserve; compaction remains M13 and I/O measurements M15. No removed Sync or new OS/power-loss claim.

## M11 versioned session recovery

[M11](milestone-m11.md) preserves WAL transaction/entry/digest bytes and introduces only explicit configuration v2/IDENTITY v2 application semantics, documented in [ADR-010](adr/ADR-010-session-profile.md). New v2 bootstrap starts with empty map/sessions; existing base-zero WAL recovery reconstructs log/commit before M02 replays the committed prefix and rebuilds both states. Duplicate mutation entries advance apply but reuse their original cached result. Uncommitted suffixes never register sessions during startup. A later legal new-term commit may register them exactly once. Real follower/leader/all-process crash/restart tests preserve successful and failed CAS results. No separate session file, applied cursor, implicit legacy conversion or snapshot rotation. The 8 MiB WAL/32 MiB directory cap remains; even a cached retry needs fresh log space and can be unavailable at the cap. OS/power-loss limitations are unchanged.

## M12 storage models without new formats

[M12](milestone-m12.md) adds local diagnostics and an explicit simulator separate from the server. Observations/logs never authorize commit, apply or client responses and do not change the algorithms. V2 sessions, exact-slot GET, atomic CAS and fresh-quorum retry remain unchanged; no v1 migration. Simulation uses actual core/M02/WAL codecs, a permanent committed ghost and per-index result comparison; replay checks every event against saved choices. ProcessCrash retains completed unsynchronized writes; SystemCrashModel explores all/synced survival only in memory. Incomplete traces are inconclusive. No actual OS/power crash, snapshots or compaction; M13/M14 gates retained. Operational specifications are in [faults](fault-injection.md) and [diagnostics](observability.md).

## Pre-M13 protocol and evidence status

The [pre-M13 review](pre-m13-review.md) specifies capture S, final-fence hard state, generation WAL with initial S commit/full suffix/final C checkpoint, Sync+rename+directory-Sync publication and token handoff. CURRENT remains sole authority; no corruption fallback. Rotation preserves live state beyond S; InstallSnapshot requires drain and obsolete/matching revalidation before post-publication replacement. Reserve old/new/staging/control peak within 32 MiB, never pre-delete for space. C-01 design RESOLVED, existing C-02 primitives reconfirmed but new path CONDITIONALLY READY, C-03 snapshot CONDITIONALLY READY. M13 not started: writer, installation, compaction and stateful recovery absent; no snapshot crash-consistency validation.

## Isolated restore, no runtime recovery

[M13 Phase A](milestone-m13-preliminary.md) adds only codecs and snapshot.Restore into a fresh in-memory instance. Restore requires v2 config, exact cluster/hash and termS<=currentTerm from the caller's validated hard-state view; returns Point for the new applier InitialIndex. No <=S entry replay, file opening or listener. Proof/Capture rejects inconsistent applier bases. Caller separately supplies only a proven committed suffix; the isolated library does not decide commit.

WAL.Open/OpenSessions still reject stateful/nonzero-base snapshots; bootstrap/M09 recovery unchanged. No new CURRENT, writer, generation switch, compaction or InstallSnapshot. Schemas/sessions/capture pass isolated tests, not crash recovery or C-02/C-03/T-016 closure. Publication protocol and [pre-M13 matrix](pre-m13-review.md) remain next-phase requirements rather than active code.
