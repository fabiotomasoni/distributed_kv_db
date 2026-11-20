# M07 — Distributed commit and state machine application

[Index](README.md) · [Italiano](../it/milestone-m07.md)

**20 November 2026 — COMPLETED within official M07. M08 NOT STARTED.**

## Scope, prerequisites and M06 review

Both [roadmaps](implementation-roadmap.md) assign M07 to the current-term commit rule and apply, FR-007/FR-008, AC-007/AC-008, T-007/T-008. Explicit follow-up scope requires the minimal fresh-machine reconstruction needed by this application path; full server/process recovery/MVP remains M09, logged reads M10, sessions/deduplication M11 and snapshots M13. No client HTTP/API or client success contract is added.

Entry Git baseline 937ec79 was clean. Reviewed both roadmaps, actual M06 core/service/node, M02 owner/future/backpressure contract, M03 checkpoint/recovery, consistency/failure/data contracts, applicable ADRs and audit gates. The full M01–M06 regression passed before changes. Reviewed exact Sent/Failed/request-byte/term correlation, monotone match, new-generation reset, durable local match and follower suffix protection. No blocking M06 defect was found; no prior codec/WAL/applier/state-machine fix was needed. C-01 previous formats/fixtures unchanged, C-02 local Darwin/APFS limits retained. C-03's M07 backlog/committed-prefix obligations have evidence below; its snapshot/dedup/stale-read/checker portions remain at their later gates.

## Commit algorithm and persistence

EnableCommit is an explicit startup capability above M06 replication. For a leader, select the majority boundary from the three durable match indices, including its own synchronized log. Advance only when the boundary exceeds the old checkpoint and that entry's term equals currentTerm. Older entries become covered only through that later current-term boundary. The existing new-leader noop supplies the first possible current-term boundary. This follows [Raft section 5.4.2 and Figure 2](https://raft.github.io/raft.pdf); replica count for an older term alone is deliberately insufficient.

Core maybeCommit runs after a successfully correlated current-leadership replica response. Invalid/obsolete requests, stale generations, transmission alone, lower-term responses or missing majority do not create commit proof. A valid higher-term reply demotes before commit evaluation. CurrentTerm/role changes preserve the already committed prefix and application progress.

persistCommit emits the existing canonical commit_index transaction, after all covered log frames are durable. CommitIndex changes only after its exact generation/physical-sequence Sync completion. Checkpoint failure is fatal/uncertain, with no new application or positive dependent follower reply. No second checkpoint or persisted applied cursor is introduced. A complete surviving checkpoint may recover even when its old caller never observed acknowledgment; its required durable prefix is verified by M03.

A follower learns commit only on a valid nonobsolete AppendEntries with matching prev and synchronized incoming suffix. Its candidate boundary is min(leaderCommit, prev+len(entries)); retain max(oldCommit,candidate). Thus a short heartbeat cannot commit an unrelated extra local tail. Hard-state/suffix persistence precedes the checkpoint; the successful RPC reply follows all required Sync operations. Duplicate or lower leaderCommit values do not rewrite/regress a checkpoint; obsolete/prefix-rejected messages never advance it. Future heartbeats/batches propagate the synchronized leader checkpoint.

## M02 adapter and ownership

[internal/committed](../../internal/committed/project.go) validates and projects existing noop/put/delete/cas entries. A private bridge intercepts noop inside the M02 worker, returning OK without invoking M01, so every global index advances without sending an invalid opcode to the KV machine. Mutations use the unchanged M01 implementation through unchanged M02. Client identity/sequence remain validated/persisted metadata; M07 implements no replicated session table or cross-index client deduplication.

Core owns LastApplied, ScheduledApplyIndex and ApplyEpoch=1 for this instance. The invariant is LastApplied <= ScheduledApplyIndex <= CommitIndex. Only successful TrySubmit advances Scheduled; only an ordered completed M02 Future advances LastApplied. A full queue leaves the unscheduled committed log in place. Repeated commit events retry from Scheduled+1, never LastApplied+1. Leadership change resets neither application cursor. Old-epoch completions are ignored; snapshots/epoch replacement are not implemented here.

Service adds one M02 worker and one ordered future collector, without per-entry goroutines. Pending M02 capacity is 1..64 (default 64); at most 64 tickets are outstanding, ticket/completion channels cap at 64, recent-result storage at 64. The owner uses nonblocking admission while processing networking/ticks/completions; completion notifications resume scheduling without spin or loss. Blocking disk remains outside a core/global mutex. Inputs/results are copied at their ownership boundaries.

Service.WaitApplied(ctx,LocalAppend{Index,Term}) returns an indexed/termed domain result only after application. Caller cancellation abandons the wait, never an accepted command. A changed term at that index gives ErrSuperseded; evicted historical results give ErrResultExpired; infrastructure failure remains an error. Retained results are copied and remain inspectable after normal shutdown. This is internal result delivery, not external exactly-once/client linearizability. CAS COMPARE_FAILED or other deterministic domain rejection consumes its committed application slot and is never retried/rolled back as infrastructure failure.

## Recovery and shutdown

node.StartCommitted enables the M07 path; legacy StartElection/StartReplication preserve M05/M06 capability and original trace fixtures. NewService Options.Commit selects M07; ApplyCapacity is local bounded tuning, MachineFactory is an optional trusted factory that must return a fresh exclusively owned finite machine. Production always creates a new M01 machine.

Before listener/timer startup, reconstruct only indices 1..recovered CommitIndex on that new machine through M02. Rebuild Scheduled/LastApplied from completed futures; ignore the uncommitted suffix. A second start never replays over the old mutated instance. An application before a crash can be executed during reconstruction on a fresh map, without duplicating effects on the same live map. No external exactly-once claim or durable applied cursor is needed for this volatile machine.

The current M03 runtime has an empty base-zero checkpoint; nonzero snapshots remain unsupported. M07 deliberately rejects logs/batches containing future read entries instead of implementing an ordered-read feature early. Homogeneous startup capability is required; rolling feature negotiation/migration is not implemented. An unsupported legacy read log fails closed without modifying it. PUT/DELETE/CAS/noop encodings are unchanged.

Normal shutdown rejects admission/cancels networking, finishes a started write, schedules/drains the durable committed backlog, joins M02 and collector, then node closes WAL/releases LOCK. A canceled 5 s waiter reports incomplete waiting while ownership remains held. On fatal storage/apply error, admission closes immediately even if old accepted application is still draining. Accepted worker work finishes/aborts according to M02; no unsynchronized checkpoint is applied, no failed/uncertain application is retried on the same map. An arbitrary injected Apply that never returns cannot be forcibly stopped; the owner retains resources until actual drain. Failed runtime progress is conservative and unusable as a healthy completion guarantee.

## Tests and independent evidence

14 new test functions: 12 raft, one committed projection, one node integration. Core tests explicitly establish an old-term majority with unchanged commit, then acknowledge/synchronize a current-term noop and commit the prefix; no-majority and one-offline cases, monotonicity, obsolete response, higher-term demotion, follower verified-prefix cap, duplicate checkpoint and Sync failure are checked. Suffix frames must precede the checkpoint; scheduled/completed indices and old epochs are separately validated.

The C-03 backlog test commits 5 then 7 while the first mutation is gated and queue capacity is 1. A durable ghost prefix is compared byte-for-byte; no duplicate scheduling, gap or lost work is allowed. Releasing the worker yields exactly six mutations plus one noop in global order, including a successful CAS and an applied COMPARE_FAILED. Canceled result/shutdown waits cannot remove the obligation. Infrastructure panic fails closed. Bounded result expiry/supersession and M10 read exclusion have tests.

Checkpoint-failure tests verify zero application/replica acknowledgment and immediate admission rejection while an older safe application remains gated. Real tiny WAL images cut a checkpoint at 0/20/24/payload-final/full boundaries, reopen through M03 and replay either none or exactly the committed prefix on a new machine. Repeated full-image restart repeats a CAS-from-absent successfully on a different machine; a speculative secret key never appears. This is modeled interruption/live-OS byte recovery, not an OS/power-loss experiment.

A deterministic three-node event simulation has an independent permanent committed ghost register and per-index domain-result oracle. Three network seeds run twice; loss/delay/duplication, partition, new election, isolated old leader without quorum and fresh restart are exercised. The old leader's speculative write never commits/applies; the new term retains prior committed data. Every node applies through actual M02 futures and has identical final state/results, including a failed CAS. [commit-v1-results.json](../fixtures/commit-v1-results.json) stores commit/apply trace fingerprints (30 events per seed), equal final state digests and CommitIndex=LastApplied=5. This simulator uses explicit core events with M02 work; asynchronous service backpressure is verified separately by the gated runtime test.

The real three-node loopback test uses production M07 adapters, ordered concurrent CAS (exactly one succeeds), leader stop after commit, majority election, a further CAS proving preserved state and production StartCommitted restart/catch-up. Each stopped node drains through its durable checkpoint. Tests inspect M01 values only after the M02 owner has terminated. Old M01–M06 tests, 20 election seeds/100 heartbeats, replication seeds, lifecycle and invalid-physical-sequence WAL regression remain unchanged and pass.

These finite tests check current-term quorum commit, committed-prefix preservation, no speculative apply, exact ordered application, monotone cursors and matching replica state. They do not constitute a universal Raft/linearizability proof, external client history checker or snapshot safety evidence.

## Actual verification results

Go 1.27.4 darwin/arm64, existing local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normally, 1 with existing compiler for race. Actual equivalent commands:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m07-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node ./internal/committed ./internal/applier ./internal/wal ./internal/transport
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

All passed. Race reported none in exercised paths; raft/node rerun after final fatal-admission hardening. Aggregate statement coverage across test binaries: raft 88.5%, committed 92.3%, node 81.2%; applier/statemachine 100%, config 87.3%, peerprotocol 91.4%, strictjson 92.1%, transport 90.3%, storageformat 92.3%, storagefs 87.9%, WAL 82.7%. Coverage is not the per-binary percentage printed by -coverpkg or proof of every schedule. No benchmark, physical-fault or measured resource claim. Original fixtures/go.mod byte-identical, no go.sum/dependency; existing arithmetic verifier passes.

## Files, acceptance and next readiness

Created internal/raft/commit.go, application.go and commit_test.go, application_test.go, commit_recovery_test.go, commit_simulation_test.go; internal/committed/project.go and project_test.go; internal/node/m07_test.go; docs/fixtures/commit-v1-results.json and these bilingual reports. Extended core status/replication hooks, service application lifecycle and node startup/result delivery. The existing memory-storage test fake now records commit transactions. Production M01/M02/WAL/codec/filesystem/transport/config/parser remain unchanged. No prior blocking bug was found; final review hardened the new fatal-admission/drain path with a targeted test.

FR-007/FR-008, AC-007/AC-008, T-007/T-008 and the M07 C-03 backlog obligation are satisfied within this phase. No known blocking bug remains. Full process/server restart/MVP at M09, snapshot handoff/epochs at M13, read barriers at M10, dedup at M11 and independent final safety/history gates remain open. API stability/OpenAPI C-01 remains due at M08.

**M07 COMPLETED.** M08 can review the internal durable local receipts and applied outcomes, but must add its own PUT/DELETE HTTP/error contract and OpenAPI evidence. No M08 code, public client/read API, session guarantee, ReadIndex, advanced snapshot or cloud deployment was introduced. Updated relevant bilingual contracts/indexes/ADR/status while preserving historical phase evidence. No Git commit.

Additional budget **€0**. Existing standard-library toolchain only; runtime/tests offline with temporary private files/dynamic loopback ports and at most three ordinary nodes. Two processors, serial packages, <=30 s package deadlines, <=5 s bounded waits; mostly small in-memory scenarios. No paid services, account, download, GPU, VM/container, massive writes, prolonged stress, personal/system modification or invented hardware measurements. The public Raft paper was consulted as the normative source; local execution needs no external service. Stop at M07.

Documentation verification: 122 Markdown files, 60 language pairs, 1,561 relative links, 106 tables and 26 matching non-Mermaid technical example pairs; zero structural errors. Reciprocal language links, heading/ID parity, JSON and fences checked; semantic equivalence reviewed locally. No external certification or rendered Mermaid verification.
