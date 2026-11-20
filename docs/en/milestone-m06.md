# M06 — Log replication, AppendEntries and conflicts

[Index](README.md) · [Italiano](../it/milestone-m06.md)

**20 November 2026 — COMPLETED for official M06. M07 NOT STARTED.**

## Scope resolution and prerequisites

The follow-up assignment definitively confirms the [roadmap](implementation-roadmap.md): M06 is replication/conflict resolution/progress tracking, FR-006/AC-006/T-006. Distributed commit/current-term quorum rule and apply remain M07/T-007/T-008. The earlier scope block is **RESOLVED**, without reassigning roadmap milestones. The full previous preliminary report is retained at the end, including its historical blocked status and M05 evidence.

Git baseline 19703fd; entry retained the uncommitted preliminary documentation and two resource-release tests. The preliminary M05 correctness review found no blocker; its complete regression/race evidence was followed by further M01–M05 regressions here. C-01 original codecs/configuration/protocol fixtures are unchanged; C-02 retains only its tested local Darwin/APFS profile; C-03 remains at the original later gates. No prior production correctness fix was needed.

## Interfaces, ownership and limits

[replication.go](../../internal/raft/replication.go) extends the same single-owner core. EnableReplication(batch) accepts 1..16 only before events; the explicit zero/default M05 capability preserves the existing election harness and exact saved election fixtures. StartReplication(config,bootstrap) enables M06 with batch=16; StartElection and M04 Start retain their contracts. Service Options.BatchSize permits smaller local batches. This is startup selection, not runtime negotiation or rolling-upgrade support.

Core.AppendLocal(now,kind,command) assigns its own contiguous index/current term and validates canonical entry/command bytes. Service.AppendLocal and node.AppendLocal return LocalAppend{Index,Term} **only after local Sync**. They expose no client HTTP operation or quorum/commit result. A canceled caller can have an already-persisted local entry; retries have no deduplication guarantee. Nonleader/disabled/invalid/full/closed admission returns an explicit error. Commands are size-checked before queue copies and defensively copied. No state machine/applier dependency.

Core retains owned canonical entry bytes and term lookup; LogEntries returns deep copies. Status.Progress is a fixed copied three-member array. Each follower tracks nextIndex/matchIndex and at most one actual request. Sent registers dispatch after adapter admission; skipped offers do not invalidate a live request. Failed releases only the corresponding request; Response validates original request/reply identities, exact stored request bytes, RPC ID, current term and leadership before progress. Higher terms are observed first even for obsolete requests. State remains owned by one event loop, with 256 peer/local events, one active disk effect, two fixed peer workers and bounded job/result channels. Existing loopback/socket/body/deadline limits and tick-based backoff remain.

## AppendEntries and reconciliation

Follower processing fully validates the wire batch before effects, rejects obsolete terms without resetting the timer, recognizes current/higher-term leaders under the election rules, then verifies prev index/term including the retained boundary. Missing suffix returns lastIndex+1; a term mismatch returns its first local index and conflict_term. A higher term is synchronized before any dependent rejection/success. Empty heartbeats use the same prefix checks and never delete a suffix merely because the request ends early.

Matching entries are preserved byte-for-byte. At the first actual term conflict or missing entry, retain the prefix and persist the replacement suffix. Known committed indices recovered from M03 cannot be replaced; attempted conflicts fail closed without a write/positive reply. Different bytes at the same coherent index/term also cause an integrity failure. Duplicate/short batches preserve any consistent additional suffix and require no unnecessary log rewrite. Successful match_index is exactly prev+len(entries), not the follower's entire log length.

On leadership, the existing synchronized new-term noop initializes local match to the durable last index, remote match=0 and remote next=last+1. Correlated success advances match monotonically and next=match+1. Rejection applies the approved conflict-term/index hint, bounded by the leader's available log and match+1, reducing the attempted next position where possible. No transmission or uncorrelated/duplicate/old-generation response counts as replication. A new leadership generation resets remote knowledge; former-leader suffix changes occur only after it becomes follower. Snapshot catch-up for removed prefixes is not implemented before M13; M03 runtime still uses the available base-zero generation.

## WAL ordering and interruption

M03 already supports canonical replace_suffix transactions, so **no WAL codec, format or storage production change** was required. Core builds the existing hard_state+replace_suffix batch; the service serializes Append outside any core/global mutex. Completion must match the exact generation/physical sequence. Install the in-memory suffix and acknowledge only after successful Sync. No raw persistent-file manipulation occurs in production.

Local proposals use existing AppendProposal to preserve the 512 KiB consensus reserve; replication/control uses Append. A known pre-write proposal ErrLimit leaves the leader/log/index usable and consumes no sequence. An uncertain/failed write, replication/control persistence failure or bad completion token is fatal, never a negative vote disguised as success. Service construction in M06 requires the proposal-capable storage interface; the M05 interface remains compatible.

Outgoing batches obey both the configured entry count and a conservative 256 KiB canonical entry-byte cap, which also respects the RPC command cap. A legal incoming RPC can contain <=256 KiB commands yet exceed the WAL's <=256 KiB total entry bytes. The whole batch is validated first; such suffixes are persisted in bounded consecutive transactions. The first replaces the divergent suffix, subsequent frames append the remainder; one positive reply follows the last Sync. Interruption can leave a validated durable prefix, never a partial transaction or positive acknowledgment for missing frames. Recovery replays that prefix under M03 and duplicate retries append the missing remainder safely. Known committed entries are preserved at every cut.

## Tests and finite invariant evidence

11 new final-implementation test functions: 10 raft, one node integration, plus the two preserved preliminary node tests. Existing assertions/election fixtures remain; the old memory-storage fake gained suffix replacement/AppendProposal support and its expanded result literal uses named fields. Production M01/M02/storage/transport/parser/config code remains unchanged.

Core tests cover empty/short/divergent/identical logs, single/multiple entries, duplicates, short heartbeats, committed-prefix conflict, inconsistent same-term bytes, obsolete/higher terms, conflict hints, next/match changes, actual dispatch versus offered/skipped messages, canceled attempts, stale/duplicate replies, generation reset, startup/batch bounds and local Sync versus remote knowledge. Injected Sync/uncertain-quota failures produce no replica acknowledgment; known proposal admission quota is retriable. A maximum legal wire batch exercises two bounded WAL effects with no reply before the final completion.

The real-WAL test seeds a controlled consistent commit checkpoint, cuts the reconciliation frame at 0/20/24/payload-final/full boundaries, reopens through wal.Open, verifies whole old/new transaction state and the identical committed prefix, then retries catch-up. The complete surviving write is explicitly not proof of a prior acknowledgment. No OS/power interruption is claimed.

A three-node memory simulation starts with a shared known committed prefix and divergent uncommitted tails, elects an up-to-date leader, then injects seeded loss, delay, duplication and reordering. It crashes/restarts n2 after suffix Sync and before acknowledgment. An independent disk-view oracle checks Log Matching across equal index/term prefixes, leader append-only, unchanged known commit and eventual tracker/log convergence after healing. Five seeds run twice with identical scheduler traces; [replication-v1-results.json](../fixtures/replication-v1-results.json) stores fingerprints/results (469 recorded events per replay across the five seeds). All reach last index 6 with the controlled commit fixed at 1.

The loopback integration runs at most three ordinary nodes, batch size 2, appends three entries, waits for identical durable logs and correlated match indices, stops a follower, appends again, restarts it through production StartReplication and verifies catch-up and shutdown. Commit remains 0 on every node. Channel/context synchronization, virtual clocks and bounded status tickers replace sleep-based coordination. Existing 20-seed/100-heartbeat M05 evidence and the invalid-physical-sequence WAL regression pass unchanged.

These are finite observations of Log Matching, leader append-only, persist-before-ack, monotonic terms/trackers and known committed-prefix preservation. Leader Completeness and state-machine safety retain their later consensus/apply gates; no universal/formal safety proof is claimed.

## Executed checks and results

Go 1.27.4 darwin/arm64, existing local macOS/APFS profile; GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2. CGO_ENABLED=0 normally, 1 with existing compiler for race. Actual equivalent commands:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m06-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node ./internal/transport ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

All passed. Targeted race reported none in exercised paths; raft/node rerun after final startup-guard change. Aggregate statement coverage across instrumented test binaries: raft 90.3%, node 81.3%; statemachine/applier 100%, config 87.3%, peerprotocol 91.4%, strictjson 92.1%, transport 90.3%, storageformat 92.0%, storagefs 87.9%, WAL 82.7%. Different production denominators explain changes from M05; coverage does not certify every fault/schedule. Original fixtures remain byte-identical, arithmetic verifier passes, root module only, no go.sum/new dependency. No benchmark/physical-fault/resource measurement was performed.

## Files, acceptance and next boundary

Created internal/raft/replication.go, replication_test.go, replication_recovery_test.go, replication_simulation_test.go; internal/node/m06_test.go; docs/fixtures/replication-v1-results.json. Extended internal/raft/core.go and service.go, internal/node/node.go; minimally adapted the existing test fake in service_test.go. Preserved pre_m06_test.go and all preliminary evidence. Updated this bilingual report plus relevant indexes, roadmap, Raft/network/architecture/storage/recovery/failure/testing/acceptance/traceability and ADR-003/ADR-005 follow-ups. The roadmap allocation itself is unchanged.

**FR-006 / AC-006 / T-006: Passed. M06 COMPLETED**, no known blocking bug. M07 can begin its own review with durable per-peer replication progress. It must still implement legal quorum/current-term commit, persisted commit advancement/propagation and ordered applier integration. In M06 leaderCommit is carried unchanged from recovered state and deliberately never advances the follower checkpoint; no runtime commit_index transaction is emitted. There is no client commit acknowledgment, read barrier, deduplication, full client API or snapshot installation.

Additional budget **€0**, local standard-library work with existing tools. Two processors, serial packages, <=30 s package deadlines, <=5 s bounded waits, at most three real nodes, otherwise tiny in-memory scenarios. No paid/cloud/API service, download, GPU, VM/container, prolonged stress, massive SSD writes, personal/system edits or automatic Git commit. Tests needed loopback execution access only, no external networking. No M07 implementation. Stop at M06.

## Historical preliminary review (superseded)

<details>
<summary>Original preliminary report, before scope confirmation</summary>

**20 November 2026 — M06 BLOCKED pending required scope clarification; implementation NOT STARTED. M05 preliminary review passed within the limits below.**

## Official scope and discrepancy

Both approved [roadmaps](implementation-roadmap.md) assign M06 to **log replication**, AppendEntries/conflicts and divergent-log traces, FR-006/AC-006/T-006. M07 separately owns **distributed commit**, the current-term rule and apply, T-007/T-008. The new assignment is titled “Log Replication & Distributed Commit” and includes mandatory commit computation/propagation and old-term commit scenarios. These cross the M06/M07 boundary.

Section 3 of the assignment explicitly requires: if official M06 differs substantially from the described replication/commit scope, do not begin implementation, report the discrepancy and clarify the scope before following the roadmap. Consequently no M06 production code, log-replication tests, new commit logic or roadmap reassignment has been introduced. A clarification request proposes M06 limited to AppendEntries, log reconciliation and nextIndex/matchIndex, with distributed commit/apply retained at M07. Silence is not approval. This is a scope gate, not a discovered Election Safety failure.

Git entry baseline 19703fd was clean. The existing roadmap definition is preserved. No milestone is marked completed by this preliminary review.

## M05 correctness review

Reviewed actual internal/raft core/service and node lifecycle against the M05 report, election specification, persistence and relevant ADR/audit contracts. The core is single-owner, handlers enqueue bounded events, a single synchronized persistence effect suspends advancement, and network workers cannot mutate consensus state. Self-vote and vote replies wait for exact generation/physical-sequence completion. Higher terms demote and clear the old vote; stale responses cannot count in a new election but valid higher terms remain observed. Vote grants enforce persistent same-term exclusion and term-first last-log comparison. Timer resets are limited to election start, granted vote and recognized nonobsolete leader RPCs. Recovery starts follower; no uncommitted data is applied.

Reran all existing M01–M05 tests, including 20 saved election seeds each replayed twice, crash-after-model-vote-Sync and second-vote rejection, 100 healthy heartbeat rounds, stale/higher replies, real WAL vote reopen, service I/O failures and three-node loopback election. The M04 regression TestIncompleteTailWithWrongSequenceFailsClosed remains unchanged and passed. No known blocking M05 correctness defect was found in this bounded review; finite tests do not prove all Election Safety executions.

C-01 storage/configuration/protocol obligations remain satisfied, OpenAPI remains M08. C-02 retains only the previously verified local Darwin/APFS profile, without OS-crash/power-loss proof. C-03 remains at its original M07/M10/M11/M13/M14 gates. None authorizes advancing the M07 commit implementation into M06.

## Targeted integration coverage review

The initial suite reproduced aggregate coverage **raft 92.3%, node 74.3%** using -coverpkg=./... with block union across test binaries. Examined uncovered node branches instead of targeting 100%. Important remaining paths were cleanup after allocating the election service but failing listener startup, mismatched listener address and independent service termination initiating node cleanup. Thin wrappers/accessors and unreachable valid-configuration/fresh-service error branches did not justify artificial injection mechanisms.

Added only [internal/node/pre_m06_test.go](../../internal/node/pre_m06_test.go):

- TestElectionStartupFailureReleasesStorage: injected bind failure and mismatched listener address, no usable node, preserved bind error and successful WAL reopen proving ownership release.
- TestElectionServiceTerminationClosesNode: stop the election service independently, wait for the node Done watcher to complete peer/storage cleanup, repeat shutdown and reopen WAL to verify LOCK release.

Both passed without changing production behavior. The latter exercises the shared service-termination cleanup path; it is not a newly injected real disk failure. Existing raft service tests cover terminal storage failures and failure retention during shutdown. Aggregate node coverage became **81.4%**, raft remained **92.3%**. No production M03–M05 fix or interface change was needed.

## Executed checks and constraints

Go 1.27.4 darwin/arm64, existing local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; normal CGO_ENABLED=0, targeted race CGO_ENABLED=1 with existing compiler.

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m06-preliminary.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node
```

All passed. Node race was rerun after adding the two tests, without reported races in exercised paths. The coverage command instruments all packages; individual test-binary percentages are not the aggregate per-package coverage stated above. Previous fixtures, production source and go.mod remain unchanged. No new dependency or persisted encoding, no Git commit. Documentation gets structural/link/language-pair checks; no formal certification.

Budget **€0 additional**, local/offline tests only, temporary private directories and dynamic loopback sockets. Two processors, serial packages, <=30 s package deadlines, <=5 s waits, tiny synthetic data; no cloud/API service, download, benchmark, stress, GPU, VM/container, large writes or personal/system file change. Local listener tests required the existing execution-policy allowance for loopback, not an external connection.

## M06 deliverables and next readiness

AppendEntries nonempty replication, divergent-suffix reconciliation, nextIndex/matchIndex, leader append admission, replication failure/restart scenarios and M06 Log Matching evidence are **not implemented by this delivery**. Commit calculation/current-term rule/propagation and applier integration remain **not started at M07**. No new replication interfaces, WAL extension or successful replication acknowledgment is claimed. Existing M05 heartbeat handling is unchanged.

The technical M05 prerequisite review is passed within its finite evidence. **M06 remains BLOCKED solely pending the scope clarification required by the user's assignment**, and its implementation remains NOT STARTED. Once confirmed, the authorized M06 roadmap scope can be implemented without changing the roadmap. M07 readiness cannot be established before M06 acceptance. No M07 work or automatic commit.

</details>

Documentation verification: 120 Markdown files, 59 language pairs, 1,503 relative links, 106 tables and 25 paired non-Mermaid examples; zero structural errors. Reciprocal language links, section/ID parity, JSON examples and fences checked, semantic equivalence reviewed locally. No external certification or Mermaid rendering.

## M07 handoff update

The report above records the M06 replication-only handoff. [M07 is now completed](milestone-m07.md) with explicit startup commit/application capability, preserving legacy M06 behavior/fixtures, existing WAL bytes and M02 production. Correlation review found no blocking M06 defect. M08 remains not started; previous statements deferring commit/apply describe their historical phase.
