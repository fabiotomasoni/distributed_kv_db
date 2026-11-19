# M02 review and pre-M03 gates

[Index](README.md) · [Italiano](../it/pre-m03-review.md)

**19 November 2026 — CONDITIONALLY READY. M03 NOT STARTED.**

## Scope, baseline and decision

Reviewed actual repository content at Git baseline 9a73963 (clean at entry), not just prior reports. Scope: M02 concurrency review, targeted missing-branch test, canonical-format design and macOS storage source review. No production Go file or M01 source changed; no WAL/storage/consensus/networking component was implemented. No automatic commit.

M02 remains COMPLETED within the approved local subset. No demonstrable production defect was found in examined paths. C-01 storage-format design is RESOLVED with fixed schemas and checked byte fixtures. C-02's source/choice portion is documented; actual adapter validation is OPEN. Therefore **CONDITIONALLY READY**, not READY FOR M03: only format conformance and isolated platform adapter tests may be proposed first. Storage-writer/publication integration must wait for their specified evidence. Future work still needs its own assignment; this review starts none of M03.

## M02 review and interleavings

| Examined property | Reasoned result / evidence |
|---|---|
| Single Apply owner | New starts one run worker; only it invokes Machine.Apply, outside the metadata mutex. Exclusive-machine ownership remains a caller precondition. |
| Ordered indices / duplicates / holes | Admission under one mutex accepts only Scheduled+1; refused indices do not advance the cursor. Concurrent arrival is not log order; producer retains gaps for ordered retry. |
| Queue bound / backpressure | Fixed ring 1..1024, default 64, plus one active command; raw copied inputs <=48 KiB. Submit suspends on changed/context; TrySubmit refuses full without silent loss. |
| Correlated results / ownership | Each accepted entry owns one Future/index; command copies and per-Wait result copies preserve ownership. No receiver is required for finish. |
| Cancel versus admission | Both context checks precede enqueue under the mutex. Before acceptance, cancellation consumes no index; after acceptance, context only limits waiting. Cancellation racing enqueue may legitimately return either outcome. |
| Full queue versus closing | A waiting producer releases the mutex and captures changed under lock; Shutdown changes state and notifies under that same lock. Woken producers recheck state before admission. No lost wakeup across that boundary. |
| Apply versus shutdown | Shutdown serializes admission closure, not Apply interruption. The active command and pending queue drain; repeated/concurrent waiters observe Done or their own context error. |
| Channel closure | changed is closed/replaced only under the mutex; one worker closes Done, one completion writes/closes each Future. No send races a work-channel close. |
| Progress versus completion | LastApplied advances after returned domain results, while Scheduled advances on admission. A progress snapshot can precede Future publication briefly; only Done is the full drain boundary. |
| Panic / termination | Panic fails closed: current effect uncertain, pending explicitly aborted; no retries. A never-returning/reentrant/Goexit dependency violates the documented contract and cannot be forcibly repaired by cancellation. |
| Leak/deadlock boundaries | Existing tests observe worker Done and producer joins/channels, including idle and active shutdown; no per-request worker or result-send blocking. This is finite path evidence, not exhaustive scheduling proof. |
| M01 invariants | Core unchanged; validation, quota accounting, sequential CAS, absence/empty semantics and defensive-copy regression suite passed. No direct concurrent use is authorized. |

Do not generalize M02 index rejection into replicated client deduplication or infer commit from admission. Future noop/session/epoch integration is explicitly outside the current KV applier, detailed in [formats](canonical-formats.md). Tiny fake transport/storage are harness boundaries, not proof of networking/persistence integration.

## Missing branch, changes and checks performed

Baseline coverage profile reproduced **99.3%** for applier. Its sole zero-count statement was applier.go's cancellation return after cloneCommand and before ring insertion. This is significant: cancellation may occur during copying even with immediate queue capacity.

Added TestT018CancellationAtAdmissionRecheck in [lifecycle_test.go](../../internal/applier/lifecycle_test.go). A test wrapper cancels a real context on the second Err check, without sleeps or production hooks. It verifies no Future, no queue/progress advance and no Apply call for the rejected copy, then successfully reuses index 1. This closes a test gap, not a discovered production bug; the implementation already behaved correctly. Production code remains unchanged.

Executed with the verified Go 1.27.4 darwin/arm64 toolchain in /tmp, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; ordinary tests CGO_ENABLED=0, race CGO_ENABLED=1 with existing Apple clang. Installed-toolchain equivalents from the root:

```sh
gofmt -l internal
go vet ./...
go test -p 1 -count=1 -coverprofile=/tmp/quorumkv-pre-m03-after.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
```

| Actual check | Result |
|---|---|
| Baseline full M01/M02 suite with coverage | Passed; M02 20 tests, coverage 99.3%; M01 12 tests/44 subcases, 100%. |
| gofmt | New test formatted; final gofmt -l internal empty. |
| go vet ./... | Passed. |
| Final full M01/M02 suite | Passed, one run; M02 now 21 tests, M01 unchanged 12/44. Both packages 100.0% statement coverage; no uncovered statement in final profile. |
| Targeted race, one run, timeout 30 s | Passed; no race reported in exercised M02 paths. |
| Static fixture arithmetic | Passed canonical bytes, six digests/results, two CRC32C frames, two image trailers/file hashes, manifests and max CAS 44184 bytes. |
| Independent cross-check | Temporary Go standard-library checker independently matched both frame CRC32C values and both snapshot trailer/file SHA-256 values. No storage API exercised. |
| Dependencies / scope | go.mod and M01/production Go unchanged, no external module or install. |

No heavy benchmark, load test, RAM/CPU/SSD measurement, storage adapter test, real process crash, OS crash or power interruption. Coverage and race results are finite evidence. The fixture checker checks arithmetic and selected vector mutations; the 18 recovery and six invalid-JSON expectations are future parser/reader specifications, not executed recovery tests. Bilingual structural checks cover links/tables/fences/ID/example parity; manual translation review is not external certification.

## C-01 original requirement and resolution

The [original audit, section E](architecture-review.md) says: before fixing M03 WAL/snapshot implementation, define byte-for-byte canonical schema, kind/nullable fields, bootstrap, checksums and recovery fixtures; finish configuration/protocol fixtures at M04 and OpenAPI before M08 stability. It does not require a complete M03 codec before M03.

Previously open: JSON canonicalization and numeric spelling; exact command/result/session and snapshot objects; optional versus null; typed digest bytes; complete header/checksum scope; bootstrap's initial transaction; generation/control-file schemas; admissible recovery/truncation versus corruption. [Canonical formats v1](canonical-formats.md) fixes these decisions without changing M01 semantics or conflating HTTP and persistence. Existing CRC32C transactional WAL and SHA-256 snapshot framing are retained. Fixed maximum sizes stay compatible with Raft entries and future sessions; incompatible changes require versioning/migration review.

C-01 closure checks: schema/domain/version rules complete; kind/nullable rules explicit; bootstrap/current/generation semantics fixed; positive/negative static byte expectations supplied; arithmetic and independent digest checks passed; mandatory future conformance linked to T-013/T-015/T-016. **RESOLVED for the storage design prerequisite.** Codec/recovery acceptance remains open at the implementation milestones. Configuration digest construction/protocol fixture completeness remains M04 and OpenAPI remains M08, as the original gate specifies; no full protocol/AC is closed early.

## C-02 original requirement and staged status

The [original audit, section E](architecture-review.md) says: before selecting the M03 adapter contract and before closing M03/M09 durability, verify documentation and lock/Sync/rename choices on macOS ARM64; lightweight adapter validation during future implementation, no destructive tests, no durability promise based on the Sync name alone.

[macOS storage contract](macos-storage-contract.md) records primary Apple sources and actual Go source, a stable advisory-lock inode, regular-file synchronization, a separately checked directory barrier, serialized same-filesystem publication and explicit fatal/uncertain error handling. It distinguishes normal stop, process crash, OS crash, power loss and corruption. No synchronization required for safety was removed. [ADR-005](adr/ADR-005-persistence-strategy.md) formally records this staged candidate; no new architectural strategy or new ADR ID is necessary.

**CONDITIONALLY RESOLVED for documented choices; OPEN for implementation validation.** Missing: actual mount/capability results, directory Sync, separate-process locking/release, replacement cutpoints, injected write/Sync/close errors and storage recovery conformance. The contract names owner (developer), milestone and pass conditions for each. In M03, allow only fixture conformance and isolated adapter tests first: <=16 KiB/case, <=16 cutpoints, <=2 owned processes, <=1 MiB intentionally submitted writes total, <=30 s/case and <=120 s suite. No devices or user files are altered. Before writer/publication integration require primitive/lock/error/visibility evidence, revising/blocking on failure. M03 closure needs T-013; M09 durability needs T-023; snapshot handoff/install still needs M13 T-015/T-016. OS/power-loss guarantees remain excluded. This follows the audit's staged test requirement rather than relabeling an unsatisfied gate as unconditional approval.

## Open questions, documents and final readiness

OQ-01 retains the pending implementation proof; OQ-05 is resolved for storage bytes only, with M04/M08 deliverables pending. C-03 consensus/dedup/snapshot/checker obligations remain open; only the local apply-backlog subset has evidence. Timing/resources, physical resilience and licensing remain at their existing gates. No evidence justifies full AC-013/AC-023 closure.

Created canonical-formats.md, macos-storage-contract.md and this report in both languages; shared fixtures canonical-v1.json and read-only verify_canonical_v1.py. Modified only the missing-branch test plus relevant indexes, data/API/peer/Raft format links, storage specification, ADR-005/ADR-007 addenda, gate/audit/question register, requirements/acceptance/traceability/roadmap/test evidence. M02's original report remains historical with a review pointer. Italian and English specify the same bytes, limits, statuses and evidence; technical examples share the same fixture source.

Modified shared files: README.md and docs/README.md. In each language: README.md, acceptance-criteria.md, adr/ADR-005-persistence-strategy.md, adr/ADR-007-client-retry-semantics.md, api-specification.md, architecture-review.md, data-model.md, design-review.md, implementation-roadmap.md, milestone-m02.md, network-protocol.md, raft-consensus.md, references.md, requirements.md, storage-and-recovery.md, testing-strategy.md and traceability.md. Only Go change: internal/applier/lifecycle_test.go. Structural validation: 108 Markdown files, 53 language pairs, 1227 relative links, 92 tables and 17 matching non-Mermaid technical example pairs, zero structural errors detected.

**Final outcome: CONDITIONALLY READY.** M02 is verified at its local scope; C-01 design is fixed; C-02 still needs real adapter evidence. A future explicit M03 assignment may begin only with the limited conformance/capability work above, then proceed past the internal gate only on recorded success. No unrestricted durability, snapshot, cluster or deployment authorization; no M03 implementation in this delivery. Additional cost €0, no services/purchases/new hardware, only small synthetic data and bounded checks on the existing Mac. Source consultation used the network for this review; local tests and fixture checks have no network/service dependency.

## Subsequent preliminary M03 evidence

[Phase report](milestone-m03-preliminary.md): GATE C-02 VERIFIED for local primitives, M03 PARTIALLY COMPLETED. 7 storageformat tests/18 subcases and 13 storagefs test functions/16 subcases pass; targeted race and unchanged M01/M02 regressions pass. Codec scope is stateless frames/entries/digests/manifests; no WAL/replay engine or complete AC-013/AC-014/AC-023 closure. Full snapshot and later gates remain open. Earlier figures/statuses describe their historical phase.
