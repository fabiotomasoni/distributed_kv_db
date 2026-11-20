# M05 — Leader election

[Index](README.md) · [Italiano](../it/milestone-m05.md)

**20 November 2026 — COMPLETED for official M05. M06 NOT STARTED.**

## Official scope and prerequisite review

Both approved [roadmaps](implementation-roadmap.md) define M05 as **Leader election**, after M03/M04: timers and persistent votes, election simulator, T-004/T-005. FR-004/AC-004 requires majority election and one durable vote per term, including split vote and crash immediately after vote Sync over 20 seeds. FR-005/AC-005 requires empty heartbeats and 100 fault-free virtual rounds without spurious elections. INV-01, election-related INV-05/INV-06, NFR-001/NFR-008/NFR-009 and local resource/security constraints apply within this boundary; full Raft safety remains later work.

Git entry baseline 05898d7 was clean. Read both roadmaps, Raft/failure/consistency contracts, architecture and acceptance/testing requirements, relevant audit conditions and ADR-001–005, storage interfaces and M04 report/transport. The M01–M04 baseline suite passed, including TestIncompleteTailWithWrongSequenceFailsClosed. C-01 storage/config/protocol obligations remain satisfied; OpenAPI is due at M08. C-02 retains only the verified local Darwin/APFS profile. C-03 concerns M07/M10/M11/M13/M14, not a blocking M05 prerequisite. No known prerequisite blocker was found.

## Implemented architecture and interfaces

[internal/raft/core.go](../../internal/raft/core.go) is a single-owner event machine. New(config, recovered WAL state, generation, process epoch, injected Draw, current milliseconds) starts follower with recovered term/vote and copies the log term view. Tick, Request, Response and Durable return Effects: a single canonical persistence batch, bounded outgoing messages or a correlated reply. Stop closes the machine. Status is a detached value. No filesystem, sockets, real clock, goroutines, KV access or applier calls belong to Core; randomness is injected.

[service.go](../../internal/raft/service.go) adapts the same core to M03/M04. NewService borrows Storage (State/Status/Append), Options supplies optional Clock/Draw/epoch; defaults use a monotonic ticker, crypto/rand uniform inclusive timeouts and a fresh 128-bit process epoch. Start borrows Sender and starts one event/disk owner plus exactly two peer workers. Handle supplies the transport Processor; Status is published through atomic snapshots; Done and Shutdown cover complete worker termination. No extra persistence format or dependency.

[node.StartElection](../../internal/node/node.go) is the production library integration. It verifies/acquires WAL before binding, connects Service.Handle to existing transport, then starts elections. Existing node.Start and its injectable M04 processor retain their contract. ElectionStatus exposes detached status. Fatal service or listener termination initiates node shutdown. Admission/network cancellation and owner/handler draining precede WAL close and LOCK release. Recovered remains a detached log view, not an apply path. There is still no node CLI/client listener.

Dependency direction: node → raft/transport/wal; raft → config/peerprotocol/storageformat and WAL value types/interfaces. No reverse dependency from storage/transport and no statemachine/applier dependency. The service owner performs synchronous Append outside any core/global mutex; advancement is suspended for the one outstanding effect while peer admission remains bounded. This is the simple initial architecture explicitly allowed by the approved specification.

## State, transitions and persistence

Startup always follower, with recovered vote; no recovered leadership. Election deadline draws uniformly from configured inclusive bounds (default 600–1000 ms), including fresh draws after reset and restart at the current clock instant. Heartbeats default to 100 ms. The real adapter ticks at min(25 ms, heartbeat/2), minimum 1 ms; the core reads only explicit monotonic event timestamps. No callback timer or busy loop; coalesced ticks do not produce catch-up bursts.

Timeout increments term, enters candidate, stages self-vote and requests one hard-state write. Self is counted and RequestVote is emitted only after the exact generation/physical sequence token. One distinct peer plus self makes quorum 2. Before publishing leadership, append/synchronize the required local new-term noop. That noop is not replicated, committed or applied in M05. No nextIndex/matchIndex, conflict repair or proposal path is implemented.

RequestVote compares (lastLogTerm,lastLogIndex) lexicographically, including a retained snapshot boundary when the suffix is empty. Grant only if nonobsolete, up-to-date and no incompatible same-term vote. Higher term clears old vote and demotes; the new term and any grant share one synchronized hard-state transaction. Repeating the same granted vote needs no rewrite, but resets the election timer. A denied vote or stale RPC never resets it. Pending Status.Term/Vote are tentative while Pending=true; neither status nor a local vote is a durable acknowledgment. Failure makes the node terminal even if the underlying outcome is uncertain.

Recognized nonobsolete empty AppendEntries demotes a same-term candidate without clearing its vote and resets the election timer even on prev mismatch. A positive heartbeat checks the existing prefix and reports only its verified boundary. It never truncates a suffix, advances commit or mutates KV. Lower-term heartbeats reject without reset. Conflicting same-term leader identities fail closed in the core/simulator. Schematically valid nonempty AppendEntries and InstallSnapshot can still convey a higher term, which is synchronized, but return unavailable for their unimplemented effects. Snapshot staging/installation remains M13.

Response correlation validates both original request and reply against local configuration. Stale votes never elect a new generation; each voter counts once. A valid higher term still demotes and synchronizes even when its original request belongs to an old election. An isolated old leader may retain its lower-term belief until communication resumes; this creates no same-term dual leader and grants no client operation.

ErrBusy requires the outstanding Durable completion; ErrInput denotes an invalid view/event/clock, ErrUnsupported denotes later-milestone behavior, ErrFull bounded admission, ErrClosed terminal admission. Storage, entropy, overflow or bad completion tokens fail closed with ErrFatal. Raw network/storage failures are not converted to negative votes. Sequence, term, index, deadline and RPC counter cannot wrap into accepted progress.

## Bounds, cancellation and shutdown

Peer-event admission holds 256 events, plus the active event. Exactly one persistence effect; one active RPC per peer and one fixed worker/job slot per peer, result channel capacity 2. Outgoing attempts use M04's 300 ms deadline/100 ms dial, bounded sockets and bodies; no hidden transport retries. Subsequent core ticks use current state, with per-peer connection-error backoff 50/100/200/400 ms capped at 400, no parallel retry worker. Process epoch plus bounded monotonic counter prevents cross-restart RPC ID reuse.

An unstarted/canceled/full service rejects promptly. Cancellation before processing skips an event; after a vote write begins, cancellation abandons the waiter and cannot erase that durable vote. Replies use a buffered one-result channel and cannot hold the owner hostage. Shutdown rejects admission, cancels outbound workers, waits for the active synchronous write and joins workers. A canceled Shutdown caller may wait again; storage remains owned until actual drain. Write failures during shutdown remain observable. A stopped clock channel fails closed. Injected Clock/Draw/Sender/Storage must honor their finite/monotonic contracts; an OS write cannot safely be forcibly interrupted. Callers use a 5 s shutdown wait and report an incomplete wait rather than releasing LOCK early.

## Tests and saved evidence

17 new top-level tests: 16 in raft, one node integration. The core tests cover follower startup, timeout, self-vote/noop Sync barriers, exact tokens, log/snapshot election restrictions, same-term second vote rejection, timer reset rules, duplicate/old/higher-term replies, step-down, foreign RPC contexts, unsupported operations, corruption/error/overflow and Stop. No client/apply state can be changed by these tests or interfaces.

The finite simulator forces three synchronized self-votes before delivery, then injects seeded message loss, delay, duplication and reordering. For each seed 0–19 it crashes one node immediately after the simulated vote Sync and before the durable callback/reply, restarts it with its persistent view and fresh timeout/epoch, and explicitly rejects an incompatible same-term vote. An independent durable-vote/quorum/leader oracle checks transitions, not only final role. Every seed runs twice and must reproduce exactly. [election-v1-results.json](../fixtures/election-v1-results.json) saves all 20 seeds, vote/term/leader histories and SHA-256 fingerprints of complete scheduler traces: 20,527 events, 369 retained history records and 20 distinct seed/term leader elections in one replay; each seed includes its crash. Fingerprints detect drift but do not replace the oracle or prove exhaustive safety.

A separate healthy simulator advances 100 heartbeat rounds, emits 200 peer heartbeats and retains one leader/term with both followers stable. Another isolates the old leader, elects on the majority, heals and verifies higher-term demotion. Service tests gate disk completion to verify zero early vote, fill the 256-slot queue without hundreds of goroutines, cancel waiters/shutdown, inject Sync failure including during stop, check retry bounds and join both blocked network workers. Real WAL close/Open preserves the vote and rejects a second one. The node integration elects with three real loopback endpoints/WAL directories, verifies the synced local noop and commit=0, then closes/reopens all stores and verifies lock release. It uses bounded status polling; protocol timing/oracles are deterministic in the simulator.

The simulator crash is a model crash; the M05 real-WAL test is controlled close/Open, not an OS crash/power interruption. Existing M03 child-process and I/O fault tests ran again. No new claim of process-wide recovery/apply or distributed durability is made.

## Checks actually executed

Go 1.27.4 darwin/arm64, existing macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 ordinarily, 1 with the existing compiler for race. Equivalent commands:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m05-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node ./internal/transport ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Check | Actual result |
|---|---|
| Format/vet/native build | Passed. |
| M05 and M01–M04 regression | All packages passed; original WAL tail-sequence regression retained/passed. |
| Election acceptance | All 20 saved seeds replayed twice; no oracle violation. 100 healthy heartbeat rounds, no spurious election. |
| Targeted race | raft/node/transport/wal/storagefs/applier passed; no reported race in exercised paths. raft/node rerun after final changes. |
| Coverage union across test binaries | raft 92.3%, node 74.3%; statemachine/applier 100%, config 87.3%, peerprotocol 91.4%, strictjson 92.1%, transport 90.3%, storageformat 91.7%, storagefs 87.9%, WAL 82.7%. |
| Dependency/fixture checks | Root module only, no go.sum/new package dependency. All previous fixtures and codecs unchanged; existing arithmetic verifier passed. |

Coverage uses -coverpkg=./... and merges exercised blocks across test binaries; standalone zero lines for config/peerprotocol do not describe integration coverage. It is not an exhaustive fault/schedule proof. No benchmark, throughput, latency, power-loss or physical availability measurement was performed. Docs receive structural/link/language parity checks plus local semantic review; no external certification.

## Files, decisions and prior components

Created internal/raft/core.go, service.go, core_test.go, simulation_test.go, service_test.go; internal/node/m05_test.go; shared election-v1-results.json; bilingual milestone-m05.md. Extended only internal/node/node.go in prior implementation, retaining the M04 API and startup/shutdown ordering. No M01/M02/WAL/codec/filesystem/transport/config/parser change or prior fixture modification; no prior-milestone correctness fix was necessary. During M05 review, tests fixed new runtime error retention during shutdown and explicit virtual restart-time initialization before delivery.

Updated relevant indexes, roadmap, Raft/architecture/network/development contracts, ADR-003/ADR-005, requirements/traceability/acceptance/testing/failure evidence and M04 historical handoff pointers in both languages. Chosen synchronous-effect suspension, finite workers, local election noop and validated token barrier preserve approved architecture without a general simulation framework or second persistence system.

## Acceptance, limits and M06 readiness

AC-004/T-004/FR-004 and AC-005/T-005/FR-005 pass in the official election scope. INV-01 and election portions of INV-05/INV-06 have the finite evidence above. No mandatory M05 criterion remains unsatisfied and no known blocking bug remains. Full log matching, leader completeness, commit/apply safety, client linearizability, persistent majority durability and snapshot semantics are not closed by these election tests.

Budget **€0 additional**; standard library and existing toolchain only. All tests local/offline after cache preparation; temporary loopback listener access required an execution-policy allowance, not external network. Two processors, serial package execution, <=30 s package deadlines, <=5 s waits/children, three small real nodes only in the short integration, otherwise in-memory simulation. No cloud, paid API, account, install, container/VM, GPU, stress benchmark, massive SSD writes or personal/system file edits. Hardware utilization was bounded by procedure, not invented measurements.

**Final state: M05 COMPLETED.** M06 can begin its own review with recovered term/log views, strict transport and tested election roles; it still needs log replacement, conflict handling and correlated replication progress. Commit/apply remains M07, client/server features later. No M06 implementation, no Git commit. Stop at M05.

Documentation verification: 118 Markdown files, 58 language pairs, 1,445 relative links, 106 tables and 23 matching non-Mermaid technical example pairs checked with zero structural errors. Reciprocal language links, headings/IDs, JSON and fences checked; translations reviewed locally.

## Pre-M06 regression addendum

[Preliminary review](milestone-m06.md) reran M01–M05 and targeted race checks, confirmed saved election traces and the 100 heartbeat rounds, and added two node resource-release tests (bind/mismatched listener and independent service termination). No production change or known M05 blocker; aggregate node coverage is now 81.4%, raft 92.3%. The 74.3% above records the M05 handoff. M06 scope clarification is pending; no replication or commit implemented.

## M06 final handoff update

The preliminary scope gate described above is now resolved by the follow-up assignment: [M06 completed](milestone-m06.md) with replication/conflict tracking, preserving original M05 behavior/fixtures and the preliminary tests. M07 commit/apply remains not started. Historical M05 coverage and preliminary evidence above remain phase-specific.
