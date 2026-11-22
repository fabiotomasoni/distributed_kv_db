# M12 — M11 review, fault injection and observability

[Index](README.md) · [Italiano](../it/milestone-m12.md)

**22 November 2026 — COMPLETED. M13 NOT STARTED.**

## Official scope and gates

Both [roadmaps](implementation-roadmap.md), read fully before changes, assign M12 fault injection and observability: scenarios/replay, logs/metrics and reproducible manifests. Requirements FR-017/AC-017/T-017, FR-020/AC-020/T-020, NFR-008/AC-028/T-028. M11 retains CAS/v2 sessions; M13 owns snapshots, installation and compaction, M14 the independent general checker, M15 measurements. No deliverable reassignment or new M13 feature. C-01 retains canonical codecs/fixtures and existing APIs; C-02 remains the tested local Darwin/APFS contract; C-03 snapshot/checker remains open at its original gates. ADR-008 accepted within the bounded profile described here. No new blocking gate emerged.

## M11 review and compatibility

Clean entry revision 18a2932, with the M11 commit already present. Reviewed bilingual reports, OpenAPI/fixtures, ADRs, consistency, API, storage/recovery and actual interfaces. M01–M11 suite passed before changes. CAS crosses append, replication, current-term quorum, checkpoint and M02; comparison/update remain one M01 transition with an exact result. No alternate GET+PUT or definitive cached response from an isolated leader. Negative CAS results retained; in-flight duplicates are decided by the M02 wrapper. not_admitted/unknown and term/role/commit/apply authorization remain unchanged.

A new focused test advances sequences 1..12, retries 12 with the same command/index, rejects 11 as STALE_SEQUENCE, 14 as SEQUENCE_GAP and changed-payload 12 as REQUEST_CONFLICT. Sequence 13 executes failed CAS and retains its result. It fills 1024 clients, rejects the 1025th without eviction and still checks the first client's result; it repeats the entire history on a fresh wrapper/machine comparing every result/index. Regressions retain distinct clients, concurrent duplicates, failover, speculative-suffix exclusion, lost response, follower/leader/all-process crashes and failed-CAS retry after state changes.

Persistent profile v2 unchanged: required configuration/hash and IDENTITY v2, new directories and explicit bootstrap, cross-profile opening rejected before WAL repair/replay. Temporary fixtures check v1 without touching personal data. Client acknowledgment header verification retained; v1 keeps M10 and no automatic migration. Session capacity and latest-result retention are approved limitations, not bugs to remove through TTL. No CAS/dedup safety bug reproduced. The final race run exposed the pre-existing progress-publication window described below, corrected without algorithm or format changes.

## Simulator and responsibilities

internal/faultsim and the separate cmd/quorumkv-sim binary provide a synthetic offline profile without sockets or data directories. The coordinator owns three unchanged Raft cores, xorshift64-v1 PRNG, ordered (virtual time,sequence) queue, WAL images and attempt history. Actual M02 owns M01/session transitions; every restart recreates these states from the checkpointed prefix. Synchronous persistence retains the current driver's responsibility with controlled faults and no token before success. Requirements, schemas, ownership, errors and bounds are specified in [faults](fault-injection.md).

Bounded actions: delay/drop/duplicate/reorder, directional/bidirectional partition/heal, RPC timeout, crash/restart, header/payload short write, pre-write ENOSPC, failed WAL/checkpoint Sync. No term/role override. Delayed requests/responses retain correlation; already-networked responses may cross restart. The two abstract SystemCrashModel all/synced choices are not actual OS/power-crash tests; ProcessCrash retains completed unsynchronized writes. Rules, cutpoints and survival choices are in the manifest; M13 snapshot/rename actions rejected.

Permanent committed ghost, vote/leader registers per term, Log Matching and per-index results survive restart. Commit/apply and logs are checked after every scheduler event; every image passes the actual WAL validator and every M02 result is compared with previously observed results at that index. The M11 suite additionally retains an independent logical model. The new runner is not the general M14 linearizability checker or a formal proof.

## Manifest, replay and outcomes

Versioned manifest with scenario/config/hash, core/harness/Go/platform versions, build revision when available, PRNG/draw tape, faults/cutpoints, ordered trace and synthetic invocation/response history. Replay reconstructs the scheduler without fresh random choices, checks each event/digest and stops at the first divergence; bad checksums/incompatible versions rejected. The [saved fixture](../fixtures/m12-session-retry.json) contains 669 events, seed 7, a lost CAS response after application, leader n3 crash, retry on n1 with original index 2 and subsequent GET of A. File replay compared byte for byte; a fresh same-seed run compared separately.

Simulated outcomes distinguish pending, completed, unknown and not_admitted. Timeout, response/leadership loss, crash or storage failure do not invent rollback; definitive results require the attempt's applied/committed index and leadership/term. Retry preserves original result/index within M11. At most 5 s virtual time, 10000 events, 128 actions/attempts, 10 MiB input/evidence, 10 s real command deadline plus bounded cleanup. Timeout/budgets mean inconclusive. Unsampled invariant trace. Exclusive 0600 output creation; no evidence overwrite or directory deletion. Local reducer is limited to actions and the supplied predicate/preconditions.

## Observability and interactions

internal/diagnostics owns fixed counters, 2048-duration windows and a 256-log ring <=1 KiB/record; no growth per key/client. Service hooks record transitions, RPC, Append+Sync, apply, recovery and dedup; HTTP hooks correlate request_id/outcome/code. Copied data, synchronized access, no log I/O in the Raft owner or handler access to the session map. GET /v1/metrics is local, potentially stale and available without quorum; status and all M11 operations remain identical in [M12 OpenAPI](../openapi/m12.json). The [metrics fixture](../fixtures/metrics-m12.json), finite offline validator and negative controls check schemas/bounds.

Process coordination drains JSON every 2 s and at shutdown, default stderr; separate private -log-dir uses four files of at most 256 KiB, total 1 MiB, never WAL data. Reserved copy of the single fatal storage failure, stderr attempted even with rotation. Overwritten records counted; successful heartbeats generate no INFO. Diagnostic errors never become consensus evidence. Snapshot includes role/index/match, proposal/lag, heap/goroutine and WAL-byte gauges; no key, value, CAS payload, full client ID or raw path. Precisely documented durations: local candidacy→first current-term commit, service prefix reconstruction, not full startup or external leader loss. These are not benchmarks or RSS/CPU measurements. [Full contract](observability.md).

## Tests, issues and fixes

16 new top-level tests: six faultsim, two diagnostics, two clientapi, two raft, one node, one server, one CLI and a review in the existing committed sessions file. Seeds 7/42/117 check response loss, leader change, restart, original result/index, identical fresh-run trace and serialized-manifest replay. Altered trace and bad checksum are negative controls; partition/heal, delay/drop/duplicate and old-leader no-quorum are observable rather than nominal faults. Six combinations of three I/O failures with both crash models, plus header/commit-Sync cutpoints, check no premature acknowledgment and recovery. Budgets, malformed JSON, cancellation, reduction and incompatibility are tested.

Real three-node HTTP tests correlate election and CAS timeout, stop two followers and verify metrics 200 while retry remains unknown. Unit tests check redaction, nonaliasing snapshots, concurrency, sample/log caps, overflow and fatal-failure retention. Rotation writes roughly 1 MiB of synthetic data once to check four files, privacy and symlink rejection. The process/coordinator test checks startup/shutdown logs, data-directory exclusion and lock/WAL reopening. Existing real M11 follower/leader/all-process regressions pass without expectation changes.

Issues found only in the new work: initial scenario validation needed command JSON normalization matching the core; fixed without codec changes. Partition testing showed the new driver needed RPC timeout scheduling and Failed for the exact attempt, like production: fixed and replay checked. Cleanup uses a deadline independent of the canceled caller and classifies setup/cleanup failures as failed/inconclusive. The M09 race regression reproduced WaitApplied result availability before the updated LastApplied snapshot: applyCompleted notified the buffer before publishing progress. Added diagnostics widened this pre-existing window. Minimal fix: publish the cursor after Core.Applied and before notification. A new gated-delivery test fails without the fix and passes three times under race after it; original M09 expectations unchanged. No commit, CAS or deduplication change. Also corrected the new logger so proven pre-write ErrLimit refusal is not classified as fatal; covered in the diagnostics scenario. An initial HTTP invocation without localhost permission could not bind; authorized local runs passed. No protocol workaround.

## Actual verification and coverage

Go 1.27.4 darwin/arm64, local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; normal CGO_ENABLED=0, race 1 with existing compiler. Build GOOS=darwin/GOARCH=arm64. Equivalent commands actually executed:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m12.cover -timeout=75s ./...
go test -race -p 1 -count=1 -timeout=75s ./internal/faultsim ./internal/diagnostics ./internal/raft ./internal/clientapi ./internal/node ./internal/server ./cmd/quorumkv-sim ./internal/committed
go test -race -p 1 -count=1 -timeout=40s ./internal/faultsim -run 'Test(WALHeader|SavedManifest|ScenarioValidation)'
go test -race -p 1 -count=3 -timeout=30s ./internal/raft -run 'Test(AppliedStatus|Diagnostics)'
go test -race -p 1 -count=1 -timeout=75s ./internal/raft ./internal/node
go run ./cmd/quorumkv-sim -replay docs/fixtures/m12-session-retry.json
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Full M01–M12 suite passed on the final source. Initial full targeted race group passed; a later M09 run exposed the documented window, reproduced by the new test without the fix and corrected. Final affected components and new cleanup/cutpoint/fixture tests were rechecked with race; no data-race reports in passing runs. The initial faultsim race suite took about 63 s: tiny scenarios, finite execution, not a stress test. No simultaneous clusters across runs: serial packages and at most three nodes. Formatting, vet, build, original canonical fixture checker, single-module check and OpenAPI checks passed. No new module/go.sum; no external connection.

Aggregate statement coverage, union of identical blocks across test profiles: cmd/quorumkv 0.0%; cmd/quorumkv-sim 75.9%; internal/applier 100.0%; internal/clientapi 82.7%; internal/committed 82.9%; internal/config 89.7%; internal/diagnostics 90.5%; internal/faultsim 82.5%; internal/node 81.5%; internal/peerprotocol 91.4%; internal/raft 88.9%; internal/server 87.8%; internal/statemachine 100.0%; internal/storageformat 93.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. This does not cover every schedule or credit main for instructions executed only in children, and is not formal consensus/linearizability proof. Tests check intermediate results, indices and traces rather than only final values.

## Files and documentation

Created internal/faultsim/{types.go,runner.go,runner_test.go}, internal/diagnostics/{recorder.go,rotation.go,recorder_test.go}, internal/raft/diagnostics.go and diagnostics_test.go, internal/clientapi/metrics_test.go, internal/node/m12_test.go, internal/server/m12_test.go, cmd/quorumkv-sim/main.go and main_test.go. Modified internal/raft/service.go and application.go, internal/clientapi/api.go, internal/node/node.go, internal/server/run.go and internal/committed/sessions_test.go. Raft core, M01/M02, session wrapper, mutation/read client, configuration, codecs and production WAL writer/recovery remain unchanged; application.go contains the minimal observable-publication fix.

Created docs/openapi/m12.json, docs/fixtures/metrics-m12.json, docs/fixtures/m12-session-retry.json, examples/m12/session-retry.json and these reports. Updated both languages' fault injection, observability, ADR-008, architecture, consistency, storage/recovery, failures, API, requirements/traceability, testing, criteria, guide, roadmap and indexes. Original M01–M11 artifacts preserved; no canonical fixture or historical test-expectation changes. Final checks across 134 Markdown files/66 language pairs, relative links, tables, fences, requirement IDs and technical examples found no errors; semantic review performed, no external certification.

## Acceptance, limitations and M13 readiness

**M12 COMPLETED.** FR-017/AC-017/T-017, FR-020/AC-020/T-020 and NFR-008/AC-028/T-028 satisfied within the explicit profile; no outstanding mandatory M12 requirement or known blocker. Terms/votes, Log Matching, committed ghost, current-term commit, ordered apply, LastApplied<=commit, GET slots, CAS/sessions and uncertainty are preserved in exercised cases. M11 retains 1024 sessions without eviction and latest-result-only retention; no general exactly-once, v1 migration or hidden retention improvement.

The simulator is a finite model: synchronous storage, three nodes, bounded actions and synthetic programs; it does not reproduce kernel scheduling. Versioned replay does not prove universal model correctness. No exhaustive M14 checker, M15 benchmark/stress, RSS/CPU/SSD measurements or actual OS/power crash. Durable-majority loss and shared Mac/SSD failure remain outside guarantees. Local metrics may be stale; sampleable logs may lose records with explicit counts. Arbitrary filesystem/stderr stalls have no demonstrated bound. No external certification.

**M13 NOT STARTED.** Preliminary readiness only: scenarios/replay/cutpoints and diagnostics are available for future snapshot handoff verification. The next assignment must review applier/storage barriers, atomic map+session snapshots, epochs, updated hard state, generations/CURRENT, installation and peak quota; snapshot C-03 and later gates are not closed by M12. No snapshot/compaction component is implemented.

Additional spending **€0**, offline Go/standard library, no service, purchase or download. Tiny datasets, explicit timeouts, two processors and serial packages, at most three ordinary nodes, isolated temporary folders and cleanup. No cluster left running, GPU, mandatory VM/Docker, mass writes or destructive test. No Git commit. Stop at M12.
