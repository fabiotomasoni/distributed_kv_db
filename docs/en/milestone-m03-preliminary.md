# M03 — Preliminary storage validation

[Index](README.md) · [Italiano](../it/milestone-m03-preliminary.md)

**19 November 2026 — GATE C-02 VERIFIED for the tested local platform contract. M03 overall: PARTIALLY COMPLETED.**

## Official scope and authorization

The approved [roadmap](implementation-roadmap.md) defines M03 as local persistence: frames, hard state and lock, binary specification and cutpoints, with T-013 in the phase subset. This assignment authorizes only the [pre-M03](pre-m03-review.md) first stage: canonical conformance and isolated filesystem/lock/barrier/publication tests. It does not authorize a full WAL writer, recovery engine, snapshots or subsequent milestones.

Examined Git baseline f16f5bb, actual formats/fixtures, both gate contracts, ADR-005/ADR-007, storage requirements/criteria and M01/M02 interfaces. Existing .DS_Store changes and internal/.DS_Store were present at entry and left untouched. M01/M02, go.mod and approved fixtures remain unchanged. No Git commit.

## Components and contracts

[internal/storageformat](../../internal/storageformat/doc.go) is stateless and has no filesystem dependency. It validates canonical ASCII JSON, exact fields/nullability, scalar spelling/bounds, key/value domains and entry unions. EncodeFrame/DecodeFrame implement the approved 24-byte big-endian QWAL header, version/kind, bounded payload, physical sequence field and CRC32C. Decode returns a private payload copy and consumed size for one frame; incomplete input returns ErrIncomplete with no effects. That error never authorizes truncation by itself. CommandDigest matches QCMD typed preimages and golden SHA-256 values; SealManifest/ValidateManifest match checksummed canonical CURRENT/IDENTITY bytes.

EncodeFrame requires an already canonical typed-valid JSON transaction rather than silently normalizing ambiguous input. It validates intra-frame contiguous indices, nondecreasing terms, batch/byte caps and current-term bounds when supplied. It cannot validate earlier votes, previously committed suffixes, bootstrap sequence continuity, referenced identity/configuration or legal Raft commit without a storage owner. No such stateful replay/repair is implemented. Snapshot/result/session codecs remain future work; existing image vectors are checked by the original arithmetic checker and small test harness, not a production snapshot engine.

[internal/storagefs](../../internal/storagefs/doc.go) separates low-level Darwin/APFS operations from formats. Acquire uses a stable private LOCK inode, O_NOFOLLOW/O_CLOEXEC, nonblocking exclusive flock and descriptor lifetime ownership. It rejects unsafe permissions, symlinks/special files and non-APFS roots; contention differs from other errors. Close is serialized with publication, idempotent and never unlinks LOCK. External mutation/noncooperating writers remain outside the contract. Detecting a cloud-sync folder is a caller precondition, not a capability this adapter claims.

WriteAll continues successful short writes, rejects no progress/invalid counts and stops on a partial error without retrying the whole append. File.Sync precedes checked Close. SyncDirectory uses Darwin directory fsync, retrying only EINTR and retaining close errors. PublishCurrent validates <=4096 immutable bytes before I/O, creates CURRENT.tmp exclusively, writes/synchronizes/closes, renames and synchronizes the parent. Invalid data touches no staging file. I/O failure poisons the owner until explicit close/restart, preserving evidence. Once rename is attempted, an error is conservatively ErrUncertain; no rollback, automatic cleanup or false success. Caller validation must be finite, non-reentrant and not mutate the input.

PublishCurrent is only a control-file primitive. The caller must first validate/synchronize its referenced generation. Tests pre-stage tiny copies of the approved empty image/bootstrap frame and verify CURRENT's selected generation; there is no production generation builder, snapshot capture, compaction, listener or storage worker attached to M01/M02.

## Tests implemented and actual platform results

| Area | Evidence |
|---|---|
| Exact formats | Two golden frames encode/decode/round-trip byte for byte; six entry digests and two manifest seals match unchanged fixtures. |
| Invalid input | Every cut of a small frame, bad CRC/payload/magic, unknown version/kind, zero/oversize length, noncanonical/duplicate/unknown fields, null/type/numeric/Base64 errors and malformed entry/batch boundaries rejected. Maximum CAS remains exactly 44184 bytes. |
| Lock ownership | Separate opens and independent child process reject a second owner; stable inode retained. Normal child close and parent-directed kill release the lock; child readiness and exit are synchronized, never by arbitrary sleeps. |
| Platform primitives | Regular File.Sync, one explicit F_FULLFSYNC capability probe and directory fsync succeed. File modes 0600 and root mode 0700 checked; close-on-exec checked. |
| Publication | Complete old/new manifests; no successful staging left behind; two real child cutpoints before/after rename. The surviving CURRENT references a complete checksummed fixture image and valid bootstrap frame. No fallback or early success. |
| Error paths | Injected permission refusal, ENOSPC, EIO, cross-device rename and unsupported-directory error; file/directory close errors propagated. Full operation order asserted; error after rename is uncertain even when new bytes are visible. |
| Cleanup and concurrency | Validation failure, retained staging, poisoned/closed owner, symlink/special-file refusal, missing paths and concurrent Close/Publish. Every child is joined; only run-owned PIDs/temporary directories affected. |

Recorded platform: **macOS 27.2.1, build 26B131, darwin/arm64, Go 1.27.4**. Statfs on test roots and read-only project inspection reports **apfs**, mount **/System/Volumes/Data**, device **/dev/disk3s5**. Real writes used only private t.TempDir directories below the system temporary root, never node/user data. No assertion about other mounts or macOS versions.

First development run rejected the initial temporary-directory permissions; the harness now explicitly establishes 0700. The contract was not relaxed. Final runs pass. File.Sync's Go implementation was already source-reviewed to request F_FULLFSYNC with ENOTSUP fallback; the separate direct probe verifies availability on this mount, not physical cache behavior or power-loss survival.

## Executed checks and limits

Verified temporary toolchain/cache, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2. Ordinary tests CGO_ENABLED=0; targeted race CGO_ENABLED=1 with the existing Apple compiler. Installed-toolchain equivalents:

```sh
gofmt -l internal
go vet ./...
go test -json -p 1 -count=1 -cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/applier ./internal/storagefs
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Final verification | Result |
|---|---|
| gofmt and go vet | Passed. |
| storageformat | 7 top-level tests, 18 subcases passed; 90.8% statement coverage. |
| storagefs | 13 top-level functions including the child harness, 16 subcases passed; 89.0% statement coverage. |
| M01/M02 regressions | Passed: statemachine 12 tests/44 subcases, applier 21 tests; both 100.0% coverage. Sources unchanged. |
| Targeted race | Passed for applier/storagefs; no race reported in exercised paths. |
| Existing fixture arithmetic/dependencies | Passed; approved fixtures/go.mod unchanged, no external module, install or service. |

The final suite uses two real publication cutpoints; bounded development/final repetitions remained within the 16-cutpoint phase budget. Each real-file case writes only tiny fixtures/manifests, well below 16 KiB. A conservative content-size/invocation bound for this phase is below 128 KiB of deliberately supplied synthetic file contents, below the 1 MiB budget; this is workload arithmetic, not measured SSD/internal write amplification. Child deadlines 5 s, package deadlines 30 s, at most parent+one child at a time, ordinary suite below 120 s. No stress, benchmark, mass writes, filled disk, cache flush, privileged operation, VM/container, GPU or physical fault test. Temporary contents are removed only by their owning test cleanup.

Race/coverage do not prove all schedules or all I/O failures. Subprocess termination tests a live kernel and visible filesystem namespace, not OS crash or power loss. The 18 stored recovery scenarios are still specifications: stateful vote/commit/suffix replay and final-tail truncation await the actual WAL phase. Full AC-013/AC-014/AC-023 and snapshot AC-015/AC-016 are not closed.

## Gate decision and remaining M03

**GATE C-02 VERIFIED** for choosing this local Darwin/APFS primitive contract: mount/permissions, cooperative interprocess lock and release, regular/directory barriers, synchronized replacement, finite cutpoints and explicit I/O error handling have concrete evidence. This resolves the audit's pre-integration platform gate. The pre-M03 table's reader-conformance obligation has evidence for the authorized stateless frame/manifest/bootstrap-byte subset; stateful recovery acceptance remains T-013 during the rest of M03, as the roadmap requires. No requirement is waived or converted into a power-loss guarantee. C-01 format design remains resolved; only its tested codec subset is implemented.

**M03 PARTIALLY COMPLETED**: future, separately authorized work may implement the bounded WAL owner/writer, hard-state/log/commit transition validation, generation-aware persistence tokens, bootstrap/restart handling, quota enforcement and complete T-013 recovery/cutpoint tests. Do not infer commit from log length or wire M01/M02 to partially validated storage. Snapshot capture/installation/compaction, durable-majority client claims, Raft, RPC, HTTP and later milestones remain blocked at their own gates. Full M03 completion requires that remaining evidence, not merely successful fsync.

Created storageformat/doc.go, canonical.go, entry.go, frame.go, manifest.go and format_test.go; storagefs doc.go, write.go, fs_darwin.go, write_test.go and fs_darwin_test.go. Updated this bilingual phase report, current indexes/roadmap, canonical/storage/macOS specifications, ADR-005, audit/gate/question addenda, testing/acceptance/traceability and historical review pointers. The two languages share formats, APIs, limits, evidence and remaining gates. Additional cost **€0**, existing Mac only. No automatic commit or continuation beyond this phase; await a new assignment.

Modified documentation in each language: README.md, acceptance-criteria.md, adr/ADR-005-persistence-strategy.md, architecture-review.md, canonical-formats.md, design-review.md, implementation-roadmap.md, macos-storage-contract.md, pre-m03-review.md, storage-and-recovery.md, testing-strategy.md and traceability.md, plus shared README.md/docs/README.md and this new phase report. Structural checks passed: 110 Markdown files, 54 language pairs, 1269 relative links, 96 tables and 18 identical non-Mermaid example pairs; no structural errors. No Mermaid rendering or external translation certification.

## Subsequent M03 completion

20 November 2026: [M03 COMPLETED locally](milestone-m03.md), with synchronous WAL, persistent transition validation, bounded reader/log recovery, explicit bootstrap and fault/process evidence. C-01/C-02 retain their resolved/verified local scope. No machine apply, consensus, full snapshot/rotation or M04 code. Prior partial/proposed statements describe their historical phase; end-to-end/physical durability remains unverified at later gates.
