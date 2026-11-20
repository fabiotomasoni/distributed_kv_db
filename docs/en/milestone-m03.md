# M03 — Local WAL and crash recovery

[Index](README.md) · [Italiano](../it/milestone-m03.md)

**20 November 2026 — COMPLETED within the official local-storage M03 scope. M04 NOT STARTED.**

## Objective and actual boundary

The [roadmap](implementation-roadmap.md) requires frames, persistent hard state and exclusive ownership, binary specification and cutpoints, with local T-013 evidence. C-01 fixed the schemas; C-02 verified the local APFS primitives. This assignment completes that local storage layer using the existing codecs/adapter. Git baseline dd4a3f67 was clean. M01/M02 and approved fixtures/go.mod are unchanged; no external dependency, Git commit or later-milestone implementation.

Implemented: explicit empty-directory bootstrap, an ordered synchronous WAL owner, bounded streaming reader, atomic cross-frame hard-state/log/commit validation, selected-generation verification, final-tail recovery, fixed physical limits and deterministic fault/process tests. Recovery stops at the log level. It never calls Machine.Apply or the applier, establishes Raft commit, creates sessions or presents a client result. Full server restart as follower, end-to-end durability, snapshot capture/installation/rotation and compaction remain M09/M13 or later work, as already placed in the roadmap.

## Architecture and public contract

[internal/wal](../../internal/wal/doc.go) consumes storageformat and storagefs, never statemachine/applier. Bootstrap(root,Identity) requires a pre-existing private empty directory and explicit valid cluster_id/node_id/config_hash. Open(root,Identity) requires initialized metadata and acquires the same exclusive interprocess lock. A Store serializes Append, AppendProposal and Close with one mutex; it owns one descriptor and has no background goroutine or unbounded request queue. Future consensus integration must schedule this blocking owner outside the Raft core lock; that adapter is not implemented here.

Append receives a canonical transaction payload, validates and encodes it with the existing QWAL codec, computes a tentative post-state, reserves physical space, writes the complete frame and calls File.Sync. Only after successful Sync does it install the in-memory post-state and return Token{Generation,Sequence}. The token identifies a local generation/physical effect, not a Raft entry index, quorum or commit certificate. Inputs must remain immutable throughout the call. State returns a detached copy; it includes both committed and uncommitted entries. CommittedEntries copies only the checkpoint-covered suffix, and still is not an automatic apply path.

Invalid schemas/transitions and admission quotas fail before I/O without poisoning the owner or advancing sequence. Partial writes continue only when the underlying write succeeds with a short count. Write/Sync/inspection errors fail the writer closed: zero token, unchanged acknowledged in-memory state, preserved underlying error, no blind retry or rollback. Further Append returns ErrFailed until explicit close/reopen. A complete failed/unacknowledged frame may survive and appear after recovery; a prior error never proves absence of its effect.

Close waits for serialized Append, closes the WAL before releasing LOCK, retains earlier failure/close errors and is repeatable. All successfully accepted writes were already synchronized; Close does not convert an uncertain failed append into a successful acknowledgment. OS syscalls cannot be forcibly canceled safely; full server shutdown deadlines remain separate future work.

## Format, metadata and bootstrap ordering

No format/fixture changed: 24-byte big-endian QWAL v1 kind-1 transaction header, bounded canonical JSON, physical sequence starting at 1 and CRC32C over header bytes 0..19 plus payload. Existing entry/command validation and typed digests remain authoritative. Decoded Transaction/HardState/Suffix/LogEntry are typed views of those same bytes, not another persisted encoding. FrameSize checks available magic/version/kind/length before payload allocation; contradicted partial-header fields are corruption/unsupported format, not a plausible torn tail.

State validation is atomic: first frame contains exactly hard_state and commit equal to the selected base, with no replacement. Thereafter sequence is contiguous without wrap; term cannot regress; an existing same-term vote cannot change or clear. Higher term may reset the vote. Suffix replacement begins strictly above old commit and within the existing boundary, preserves the retained prefix, requires contiguous indices and nondecreasing terms bounded by post-state currentTerm. Commit cannot regress or exceed the post-replacement log. No transaction installs only some of its effects. A legally established commit remains the future caller's responsibility; these structural checks do not run Raft.

Bootstrap rechecks emptiness under LOCK to reject races/partial attempts. It writes checksummed IDENTITY and synchronizes it/root; creates gen-1.tmp; writes/synchronizes the exact approved empty QSN1 checkpoint and initial WAL transaction; synchronizes the staging directory; renames to gen-1 and synchronizes root; seals and publishes CURRENT through the existing PublishCurrent protocol. Only then opens the selected WAL and returns an owner. The checkpoint encoder supports only the mandatory empty bootstrap image, not general snapshot functionality. CURRENT does not need rewriting for ordinary appends because its referenced WAL is append-only. No new persistent cursor or applied-index field was added.

Any bootstrap failure preserves evidence and releases ownership. Missing CURRENT with other files, including LOCK-only bootstrap debris, never triggers implicit initialization/resume. If a failed call had already fully published CURRENT, a subsequent explicit Open can validate it; Bootstrap still refuses to overwrite the directory. Production generation allocation/rotation is not implemented beyond the initial empty bootstrap.

## Reader and recovery algorithm

1. Reject uninitialized storage; acquire LOCK; validate canonical/checksummed IDENTITY against the supplied identity, then CURRENT. CURRENT alone selects the generation; no highest-directory selection or fallback to old valid data.
2. Validate the selected private generation and the exact empty checkpoint identity/base/whole-file hash. A nonzero snapshot base is explicitly unsupported until M13, never treated as empty or partially loaded.
3. Account directory bytes, open the existing private WAL without truncation and reject an already oversize file. Stream one 24-byte header and at most one bounded payload at a time; never allocate from an unchecked length or read the whole WAL into memory.
4. Validate complete frames/checksums/schema and each tentative cross-frame post-state. Retain only the live log suffix in memory; do not apply KV commands. Any complete integrity/semantic/read error returns no usable partial result and causes Open to fail without changing the file.
5. Only structural EOF incompleteness after an entirely valid initial prefix produces a repairable final tail. Truncate to the verified offset and synchronize the file; failure of Truncate or Sync fails Open. Empty/incomplete initial WAL, bad magic/version/length, complete bad checksum, sequence/vote/term/commit violation or interior corruption is fatal. Never search for another magic.
6. Re-synchronize complete surviving frames before exposing recovered local storage as ready, including complete writes that survived without a prior Sync/ack. Seek to the verified end and resume at the next physical sequence. Repeated recovery neither duplicates nor rewrites valid frames.

A corrupted length can mimic a torn final payload. The automatic tail rule is only for the approved process-crash/non-arbitrary-corruption model; it is not general bit-rot repair. No policy can infer a missing acknowledgment or physical-fault history from these bytes alone. Present, synchronized, committed, applied and client-acknowledged remain distinct. Store exposes log metadata only; application/dedup/noop/epoch coordination must later use the applier architecture.

## Physical bounds and error policy

WAL absolute cap: 8 MiB. AppendProposal admits new proposals only below that cap minus a 512 KiB consensus reserve; Append is the replication/control path and can use the reserve. Future consensus callers must classify admission correctly. The inclusive node directory cap is 32 MiB, counting old/staging/control files, with at least 64 KiB control margin reserved before growth. No needed prefix is deleted to create space. Without M13 compaction, reaching the cap deliberately prevents further growth; availability under exhausted quota is not promised.

The directory counter uses logical file lengths, not a guarantee about APFS physical allocation, other applications or SSD writes. A failed directory inspection is fatal; a known admission-limit rejection remains retriable without file mutation. Paths/files are private and no-follow checks reuse the verified adapter. Noncooperating external mutation, OS/SSD corruption and physical loss are outside the process-crash contract.

## Tests, evidence and limits

Tests cover the requested A–O categories: real append/reopen/order; exact bootstrap/frame/image/manifest fixture bytes; every cut of a small final frame; complete/interior bad checksums; unsupported formats; partial-write and Sync failure with zero token; repeated repair/recovery; normal and forced child restart; exclusive ownership; authoritative CURRENT and no corrupt-generation fallback; bootstrap publication cuts; prior-component regressions.

All 18 approved recovery scenarios now have local evidence: 13 byte-image cases run through Replay, five namespace expectations through bootstrap/manifest tests. Seven named bootstrap cutpoints preserve either an explicitly incomplete directory or a fully verifiable publication. The existing filesystem publication tests cover failures before/after CURRENT rename. The new child harness covers complete write before Sync (no inferred commit), after a synchronized checkpoint, and ordinary close. Readiness is pipe-synchronized, child deadline 5 s, only run-owned PIDs are killed/joined. Recovered votes are explicitly checked and a second same-term vote is rejected after restart. No random stress or time.Sleep synchronization.

Tiny dependency wrappers inject partial write/ENOSPC, failed Sync/EIO, close error, failed Truncate/repair Sync, read errors and directory inspection/quotas. Quota tests simulate cursor/usage values rather than allocating an 8/32 MiB file or filling the SSD. Detached state copies are mutated in tests; concurrent Append/Close is joined and checked under race detection.

Executed with Go 1.27.4 darwin/arm64, macOS 27.2.1 build 26B131, local APFS as logged by the filesystem regression suite. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; normal CGO_ENABLED=0, race CGO_ENABLED=1 with the existing compiler. Equivalent installed-toolchain commands:

```sh
gofmt -l internal
go vet ./...
go test -json -p 1 -count=1 -cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Actual final evidence | Result |
|---|---|
| gofmt / go vet | Passed. |
| WAL normal suite | 17 top-level functions including child harness, 25 subcases passed; 82.9% statement coverage. |
| Prior components | Passed: M01 12/44, M02 21/0, storageformat 7/18, storagefs 13/16 (top-level/subcases). |
| Measured package-local coverage | statemachine/applier 100.0%; storageformat 81.7%; storagefs 80.1%. New helpers exercised from WAL integration tests are not instrumented in those packages' own test binaries by plain -cover. |
| Race | WAL, storagefs and applier passed, including final WAL rerun after the last change; no reported race in exercised paths. |
| Fixtures / dependencies | Original fixture arithmetic passed; fixtures, go.mod and M01/M02 unchanged; module list contains only the root module. |

Tests use owned private temporary directories, tiny synthetic records, bounded datasets and <=30 s package deadlines, with the ordinary suite below 120 s. No giant file allocation, benchmark, prolonged stress, real ENOSPC, OS crash, power interruption, privileged disk operation, cloud/API service, paid software, new hardware or install. No RAM/CPU/SSD measurements or universal durability proof are invented. Coverage does not establish every I/O failure/schedule. Local durable-token/recovery evidence is not proof of distributed majority durability or client linearizability; T-023 end-to-end remains M09 and snapshot T-015/T-016 remains M13.

## Files, acceptance and M04 readiness

Created internal/wal/doc.go, state.go, reader.go, store.go, open.go, bootstrap.go; tests reader_test.go, store_test.go, namespace_test.go, process_test.go. Added storageformat/transaction.go (validated typed views), bootstrap.go (empty image only), storagefs/files_darwin.go (checked opens/bounded reads); minimally extended storageformat/frame.go for streaming header sizing and updated storageformat/doc.go’s ownership comment. Reused all original serialization, Sync/locking and CURRENT publication machinery. No M01/M02 change.

Updated bilingual M03 reports and relevant indexes, architecture/storage/format/macOS contracts, failure/testing strategy, ADR-005, roadmap, traceability/acceptance and historical audit/preliminary-report pointers. Historical figures remain labeled by phase. C-01 RESOLVED and C-02 VERIFIED retain their exact limitations. Local T-013/M03 and the storage portion of AC-013 are Passed; complete Raft response, server-recovery and physical-fault claims are not closed early.

**Final status: COMPLETED for M03.** The full official local deliverables are present: framed persistence, hard-state/log/commit recovery, exclusive ownership, bootstrap/manifest coordination, bounded growth and fault evidence. No unresolved M03 blocker remains. M04's storage prerequisite is ready for review; configuration/hash/protocol fixtures and its own gates must still be addressed in a new assignment. M04 is not started. Additional budget **€0**; no automatic commit. Stop at M03.

Documentation verification: 112 Markdown files, 55 language pairs, 1309 relative links, 98 tables and 19 identical non-Mermaid technical example pairs; zero structural errors. Reciprocal links, headings/IDs, JSON examples and code fences checked. Semantic translation reviewed locally; no external certification or Mermaid rendering. Shared README.md/docs/README.md and each language’s README.md, architecture.md, canonical-formats.md, storage-and-recovery.md, failure-model.md, adr/ADR-005-persistence-strategy.md, implementation-roadmap.md, testing-strategy.md, acceptance-criteria.md, traceability.md, architecture-review.md, macos-storage-contract.md and milestone-m03-preliminary.md updated alongside this report.

## M04 regression addendum — 20 November 2026

The report above records the M03 handoff. During [M04](milestone-m04.md), TestIncompleteTailWithWrongSequenceFailsClosed reproduced an incomplete final frame whose already-readable physical sequence was duplicate/gapped but classified repairable. reader.go now rejects it before any truncation, including 20-byte partial headers. Replay/Open tests verify rejection and unchanged WAL bytes for sequences 1/3 and cuts 20/25. WAL now has 18 top-level tests; full regression and targeted race checks pass. No format or durability-model change. M04 is now completed; M05 not started.
