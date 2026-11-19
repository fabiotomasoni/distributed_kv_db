# macOS storage contract — C-02

[Index](README.md) · [Italiano](../it/macos-storage-contract.md)

**19 November 2026 — documented candidate contract; platform adapter tests NOT RUN.**

## Original gate and source limits

C-02 blocks selecting the M03 adapter contract without platform-documentation review, and closing M03/M09 durability without lightweight implementation validation. It explicitly places adapter validation during future implementation. This document completes the source/choice review; the experimental portion stays OPEN. [Decision and permitted future work](pre-m03-review.md). No storage adapter, process-crash experiment or APFS probe is implemented/executed here.

Primary sources accessed 19 November 2026: [Apple fsync](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html), [fcntl](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fcntl.2.html), [rename](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/rename.2.html), [flock](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html) and [About APFS](https://developer.apple.com/documentation/foundation/about-apple-file-system). The manual pages are archival; the old fcntl filesystem list predates APFS and does not establish current APFS support. Their API descriptions plus inspected Go source inform choices, not a certification of the installed OS, filesystem or SSD.

Apple distinguishes fsync from a request to flush drive caches through F_FULLFSYNC. Rename provides same-filesystem replacement semantics; that alone is not a QuorumKV durable publication protocol. Flock is advisory, so all cooperating writers must participate. APFS features do not make an application transaction spanning several files atomic. These are the premises; the protocol and restricted claims below are local engineering decisions/inferences, requiring implementation evidence.

## File, directory and locking choices

Inspected the actual Go 1.27.4 toolchain: os/file_posix.go delegates File.Sync to internal/poll; [fd_fsync_darwin.go](https://raw.githubusercontent.com/golang/go/go1.27.4/src/internal/poll/fd_fsync_darwin.go) requests F_FULLFSYNC and falls back to fsync only on ENOTSUP, retrying EINTR through the wrapper. Therefore File.Sync is not simply a plain fsync on this baseline. This is source verification, not a measurement of flush behavior. Review this path on toolchain changes.

Candidate file barrier: use File.Sync on written regular files, with all errors surfaced; no redundant second full flush by default. Its successful return establishes the project's process-crash barrier under a live OS, not proof that the strong request was honored end-to-end. The fallback is not grounds for claiming stronger failure resilience. Restrict the future v1 adapter to a private local APFS node directory; no SMB/NFS, cloud-synchronized folder, removable device or cross-filesystem generation movement in the supported profile. M03 must record actual OS/build, architecture, filesystem/mount and toolchain.

Candidate namespace barrier: open the directory, call Darwin syscall.Fsync on its descriptor, retry EINTR only, check result and close error. Directory fsync capability/order must be tested; do not assume regular-file File.Sync/F_FULLFSYNC semantics apply to directories. EINVAL/ENOTSUP or any other unhandled failure blocks publication; never silently omit the barrier. A supported alternative requires an ADR revision and new evidence before use. This uses only standard-library APIs and a Darwin-specific future file; no new dependency or CGO requirement.

Use syscall.Flock(LOCK_EX|LOCK_NB) on a stable root LOCK file, retain its descriptor through all work and shutdown, and set close-on-exec. Do not unlink, truncate/recreate or rotate the locked inode: a second inode could admit another owner. Contention rejects startup; non-contention errors are fatal. A PID file is diagnostic only, never a lock. No inherited descriptor to a child. Future tests must verify rejection by a separately opened process, release after normal close and release after run-owned child exit. The lock is cooperative, not protection from arbitrary third-party file modification.

Open only recognized regular files/directories under the private root; reject symlinks, wrong identity and malformed generation names, and refuse unsafe path traversal. Files 0600, directories 0700. KV keys stay inside the encoded image/log, never become paths. Keep diagnostic writes bounded and values out of logs.

## Ordering, errors and publication

One serialized storage worker owns append and publication. Write complete buffers with checked counts: a successful short count must continue from the unwritten remainder; a zero-progress write or any write error fails the effect, never acknowledging a prefix. Never retry an entire append after an uncertain partial write. A buffered user-space writer must flush before Sync. No positive vote, replication response, local-replica counting, apply checkpoint or client success precedes its required successful barrier from [storage ordering](storage-and-recovery.md).

ENOSPC, EIO, failed Sync, namespace or relevant close error closes storage participation; preserve evidence and require explicit recovery/restart. Context cancellation does not cancel an in-progress filesystem syscall or prove no bytes were written. A shutdown deadline reports incomplete shutdown; it must not close/reuse a descriptor concurrently with a still-running write or publish an uncertain effect. No response may be authorized by a completion from an obsolete generation.

Publish using only same-parent/same-filesystem staging: reserve peak bytes; create a new generation; finish snapshot and complete WAL with current vote/commit under the final barrier; synchronize both files and generation directory; rename generation and synchronize node root; write and synchronize CURRENT.tmp; rename it over CURRENT; synchronize root. Only then switch the writer and acknowledge publication. Root/IDENTITY creation must similarly synchronize the file and relevant parent directory before bootstrap is published. M03 implements only its phase subset; snapshot rotation/installation remains M13.

If rename succeeds and the following directory barrier fails, report fatal/uncertain, retain both generations and do not roll CURRENT back or delete either candidate. Recovery validates the CURRENT actually present. Cleanup occurs only after verified publication, removes only recognized unreferenced old/staging paths, and synchronizes the parent after removal. Cleanup failure stops further rotation/admission as needed to preserve the cap; it cannot justify deleting active data. Never replace CURRENT with copy-and-delete, select an older generation on corruption, or allow a stale snapshot vote to replace current hard state.

## Failure contracts and Mac protection

| Event | Contract / required evidence |
|---|---|
| Normal shutdown | Drain accepted storage, barrier pending changes, close files/lock, report errors. A timed-out stop is incomplete. Not tested yet. |
| Process crash, OS remains alive | v1 target: completed writes/renames may survive even without Sync; incomplete writes may leave prefixes. Preserve synchronized dependencies and replay only proven commit. Future T-013/T-023. |
| OS crash/reboot | No validated guarantee; simulate a declared abstract model only. No actual crash test authorized. |
| Sudden power loss | Excluded v1 guarantee; no physical experiment, no inference from Sync or APFS branding. |
| I/O error / corruption / SSD loss | Fail closed on verified complete corruption, inconsistent metadata or I/O errors. Checksums detect cases, do not repair arbitrary damage. Loss of the shared SSD or durable majority has no preservation guarantee. |

Retain all required synchronizations. Avoid identical persistent rewrites, allow causally safe batching without early acknowledgments, and compact only at established thresholds. Logical directory cap remains 32 MiB including staging/control reserve, WAL 8 MiB, snapshot 8 MiB; no application counter guarantees APFS physical allocation or available space reserved against other apps. APFS shared free space is not summed across volumes. No preallocation of large images or filling the SSD to test ENOSPC; inject errors instead.

## Pending validation and exact authorization boundary

| Evidence / owner | Milestone and finite procedure | Passing condition |
|---|---|---|
| Local mount and primitives / developer | First isolated M03 adapter tests; record OS/Go/arch/filesystem, tiny files and directory barriers. | Supported profile recorded; every required barrier succeeds and errors are propagated. |
| Cooperative lock / developer | M03: parent and run-owned child, separate opens, normal close and forced child exit. | Second owner rejected while locked; explicit fresh open succeeds after release. No global process-name kills. |
| Replacement visibility / developer | M03: old/new tiny manifests; at most 16 publication cutpoints with owned subprocesses and synchronized barriers. | Recovered CURRENT selects a complete verified generation; no early acknowledgment or fallback. |
| Partial write/error policy / developer | M03: injected short/zero writes, failed file/directory Sync, ENOSPC/EIO and relevant close errors. | No success on failed/uncertain effect; no spin or blind full-frame retry. |
| Reader conformance / developer | M03 T-013 consumes static frame/manifest/bootstrap fixtures; M13 adds full snapshot/switch scenarios. | Positive bytes accepted, negatives rejected, truncation only at permitted final boundary. |
| Durable success after restart / developer | M09 T-023; M13 T-015/T-016 cover rotation, vote handoff and install. | Acknowledged-prefix and dedup properties within process-crash model. No OS/power-loss extrapolation. |

Initial M03 real-file probe budget: <=16 KiB synthetic contents per case, <=16 cutpoints, <=2 owned processes, <=1 MiB deliberately submitted writes total, <=30 s/case and <=120 s suite. Count attempted bytes including repeats; stop at the budget, not an assertion about SSD-internal writes. Dedicated run directory, cleanup only run-owned files, no privileges/cache flushing/reboots/device tools. Model tests can use tiny in-memory copies. These limits are future test requirements; none of those storage tests ran in this review.

C-02 is **CONDITIONALLY RESOLVED for the documented choice, OPEN for implementation validation**. Only format conformance and isolated adapter/capability tests may be proposed as the first M03 activities. Before integrating a storage writer/publication path, require the primitive/lock/error/visibility evidence above or revise the ADR and block the affected path. M03/M09 durability acceptance, OS/power-loss claims, production deployment and M13 snapshot installation remain blocked at their respective gates. This is the staged validation already required by the original audit, not a waiver. This assignment starts none of M03.
