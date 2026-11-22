# M09 — Process recovery and internal MVP

[Index](README.md) · [Italiano](../it/milestone-m09.md)

**22 November 2026 — COMPLETED within official M09. M10 NOT STARTED.**

## Official scope and prerequisites

Both full [roadmaps](implementation-roadmap.md) define M09 as crash recovery/internal MVP: three local nodes, PUT/DELETE, durable replication, committed-prefix replay and restart. The assigned gates are FR-014/AC-014/T-014, FR-018/AC-018/T-018 and the NFR-003/AC-023/T-023 subset allowing controlled model/applier inspection without HTTP GET. Full GET-based AC-023 is deferred until after M10, snapshots/rotation to M13. GET/logged barriers are M10; CAS HTTP, replicated sessions and deduplication M11. No scope discrepancy or roadmap reassignment was needed; the assignment's GET/CAS sections are conditional and do not apply.

Entry Git revision 6e30675 was clean. Reviewed M08 source/report/OpenAPI/fixtures, both roadmaps, API, consistency, Raft, failure/storage/configuration contracts, M01–M07 relevant interfaces, acceptance and pertinent ADRs. C-01 retains its checked M08 subset, C-02 the tested local Darwin/APFS model; later C-03 read/session/snapshot/checker obligations remain open. All M01–M08 regression tests passed before changes.

## M08 review and outcome safety

Reviewed strict validation/projection, bounded admission, the ticket's recording boundary, synchronized local append, current-term durable majority commit, exact M02 application, indexed/termed result correlation and owner-serialized completion authorization. The final HTTP snapshot is a conservative supplement, not an independent quorum proof. A later unobserved leadership loss cannot revoke a past legal commit, and role changes cannot be atomic with TCP delivery. No success is based on mere applier admission or local role alone.

Before possible recording, cancellation permanently excludes a queued ticket under its lock; not_admitted is justified only for that attempt. After possible append, timeout/disconnect/step-down/error is unknown; later application remains possible. Domain RESOURCE_LIMIT is a completed deterministic result, distinct from infrastructure error. Client transport loss is unknown and never automatically retried. Same identity still means independent attempts until M11. Existing gated disk/apply, leadership-loss, lost-socket, quorum/minority, cancellation/shutdown and schema fixtures passed unchanged; targeted race passed again. No blocking M08 defect or earlier production fix was identified. No M08 HTTP/OpenAPI behavior was altered.

## Implemented components and lifecycle

[cmd/quorumkv](../../cmd/quorumkv/main.go) is the small production signal adapter. [internal/server](../../internal/server/run.go) owns argument/config validation, node.StartAPI, process waiting, redacted fixed diagnostics and bounded shutdown. Canonical invocation is quorumkv node -config FILE [-bootstrap]; direct flags without node are accepted too. -h/help exits 0; invalid arguments/config exit 2; startup/runtime/shutdown failure exits 1. Configuration reading is bounded to 16 KiB and uses unchanged config.Parse. There is no implicit bootstrap, reset, reconfiguration, automatic retry, client-session CLI or new endpoint.

Three [example configurations](../../examples/n1.json) share the existing static membership/hash and have distinct private data directories. The [development guide](development-guide.md) supplies build, private-directory creation, first bootstrap, restart without bootstrap and stop instructions. Paths are relative to the process working directory unless absolute. .gitignore excludes .local and the root build binary. No data directories are created silently by the executable.

Node.Done now exposes completed node teardown, including automatic listener/service failure. It closes only after listeners/workers/application stop and WAL/LOCK release. The executable observes signal cancellation or that terminal event, waits at most 5 s for existing Shutdown and reports failure/recovery-required on error/timeout. Failure writing node_started also shuts down the newly opened node. node_started means storage verified and listeners reserved, not quorum/election readiness. Diagnostics omit raw configuration, user values and filesystem error contents.

## Recovery architecture and consistency

The production recovery algorithm is reused, not duplicated: acquire stable LOCK; validate identity/config and authoritative CURRENT; stream/validate WAL and monotone hard state/log/commit; reconstruct a fresh M01 machine through M02 over exactly 1..recovered CommitIndex; publish follower state with rebuilt ScheduledApplyIndex/LastApplied; then serve configured listeners and start normal election/replication. No persisted applied cursor, snapshot feature or alternate HTTP map access was added. M03 complete-corruption fail-closed and eligible torn-tail rules are unchanged.

A surviving log suffix is not proof of commit. It is excluded from replay even if every node contains it. Following a new election, a current-term quorum noop can legitimately commit the inherited suffix; this is compatible with the original unknown outcome. Before that new legal boundary, speculative commands must not appear in the rebuilt state. Completed acknowledged writes are preserved under the tested non-Byzantine process-crash model with intact directories and a recoverable durable majority.

SIGINT/SIGTERM rejects client admission and cancels HTTP waiters, while existing Raft/application ownership finishes started persistence and drains the durable committed obligation before releasing resources. A client can lose its response during shutdown: that never licenses rollback or a definitive failure. A canceled shutdown waiter bounds waiting only. If the process eventually exits after its 5 s limit, the next start recovers the surviving valid prefix. A blocked OS filesystem syscall during startup is not forcibly interruptible through a Go context; no arbitrary I/O latency bound is promised.

## Tests added and observed results

Eight new active top-level tests, plus one skipped-by-default child-process entry point. Five are command/lifecycle tests, two are actual process scenarios and one is a controlled three-node recovery test. Each uses explicit bounded waits, temporary private paths, small synthetic data and at most three node subprocesses; packages run serially. The process helper invokes the same production server.Run and signal adapter semantics as main, using the existing test binary rather than rebuilding a binary per node. The actual standalone executable was separately built and its help/invalid-argument exits checked.

TestProcessCrashOneThenAllPreservesAcknowledgedHistory completes PUTs (including empty values) and a DELETE, kills a follower abruptly, continues on the majority, restarts/catches it up, kills the leader and writes through its successor. After all replicas apply the acknowledged boundary, it kills all three before waiting for any exit. Offline reopen compares every saved acknowledgment's original entry bytes across replicas and requires checkpoints covering that boundary. Actual fresh raft.NewService replay through M02 is inspected only after stopping its worker and compared with an external expected map/deletion oracle. Restarting all intact directories then completes further DELETE/PUT operations. Graceful stop permits reacquiring every port and WAL lock.

TestProcessSignalWithPendingWritePreservesUncertaintyAndRecovery stops the followers, submits a long-deadline write to the remaining leader and waits for WAL growth, not an arbitrary sleep, before SIGTERM. The client result is unknown, a speculative suffix survives, and actual replay retains the safe acknowledged key while excluding the speculative key. A lone restarted process publishes only the recovered checkpoint/apply boundary; raw log length cannot create commit.

TestRestartAllExcludesSuffixUntilNewTermNoopCommits seeds three valid canonical images with term 5, CommitIndex=1 and LastIndex=2. Every startup is follower/term 5 with ScheduledApplyIndex=LastApplied=1. Only after a later election/noop at index 3 can the inherited index 2 be applied. This verifies both pre-election exclusion and legal later commitment, without an early GET implementation.

Command/error tests cover redacted unknown flags, help, required config, oversized/malformed JSON, pre-canceled startup, ordinary restart without initialization, repeated bootstrap preserving byte hashes, duplicate live ownership, complete WAL corruption without fallback/repair, matching example hashes/distinct directories, context stop during an incomplete peer request and output-stream failure. They reacquire ports/locks after termination. Existing persistence-during-shutdown/canceled waiter tests retain M02/M03/M05–M08 evidence; actual hung-kernel I/O or a real 5 s shutdown overrun was not forced. Test-only invalid fixtures were corrected to canonical field ordering/null vote; no production codec change or hidden fixture relaxation.

M01–M08 fixtures, strict HTTP errors, lost responses and result certainty remain unchanged and pass. The finite scenarios check committed-prefix preservation, follower restart, no speculative replay, legal current-term advancement, ordered apply and resource ownership. They are not a universal Raft/linearizability proof, independent host failure test or deduplication evidence.

## Actual verification and coverage

Go 1.27.4 darwin/arm64 on the existing tested local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normally and 1 with the existing compiler for race. Actual equivalent checks:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go build -p 1 -o /tmp/quorumkv-m09 ./cmd/quorumkv
/tmp/quorumkv-m09 node -h
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m09.cover -timeout=60s ./...
go test -race -p 1 -count=1 -timeout=60s ./internal/server ./internal/node ./internal/raft ./internal/clientapi
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

All passed; race reported none in exercised paths, including the child-process server tests. The final additional embedded lifecycle test was also run under race. cmd/quorumkv 0.0%; internal/applier 100.0%; internal/clientapi 83.6%; internal/committed 92.3%; internal/config 89.2%; internal/node 81.7%; internal/peerprotocol 91.4%; internal/raft 88.1%; internal/server 90.2%; internal/statemachine 100.0%; internal/storageformat 92.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. Coverage is aggregate across parent test binaries; standalone main/child-only executions are not advertised as covered statements. A finite race run is not a proof of every schedule or distributed consistency. No benchmark, CPU/RSS/SSD-wear measurement, OS crash/reboot or power-loss test. Original go.mod remains unchanged, no go.sum or new module; runtime is standard-library only. Bilingual structure/links/examples were checked locally; no external certification.

## Files, acceptance and limitations

Created cmd/quorumkv/main.go; internal/server/run.go, run_test.go, process_test.go; internal/node/m09_test.go; examples/n1.json, n2.json, n3.json; .gitignore; these bilingual reports. The only prior production change is Node.Done, a read-only lifecycle completion channel. M01–M08 production algorithms, storage formats, fixture bytes, M08 OpenAPI and HTTP schemas are unchanged. Updated relevant bilingual architecture, configuration/development guide, API compatibility, consistency, Raft, failure/storage contracts, testing, acceptance, traceability, ADR-002/ADR-005, indexes and roadmap. Historical milestone reports retain their original boundaries.

FR-014/AC-014/T-014, FR-018/AC-018/T-018 and the NFR-003/AC-023/T-023 MVP subset pass. Full GET-based AC-023 remains after M10; snapshot recovery M13 and final property checks M14. C-01/C-02 retained, later C-03 gates unchanged. No known blocking bug in the implemented scope. Physical shared-host failure, loss/corruption of a durable majority, arbitrary kernel stalls, OS/power loss, rolling upgrades, compaction, sessions/safe retries, GET/CAS HTTP and production deployment remain excluded.

**M09 COMPLETED; M10 NOT STARTED.** The committed ordering/recovery foundation is ready for a separate M10 review/assignment; no read barrier, ReadIndex or GET contract has been implemented. Additional cost **€0**; offline execution, mostly small deterministic/model tests plus short real-loopback process tests, no more than three temporary node processes and no cluster left active. No paid service, download, GPU, VM/container requirement, prolonged stress or mass SSD writes. Tests clean temporary data/reap their children; only requested repository files and bounded temporary artifacts were written. No Git commit. Stop at M09.
