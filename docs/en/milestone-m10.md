# M10 — Linearizable GET through logged read entries

[Index](README.md) · [Italiano](../it/milestone-m10.md)

**22 November 2026 — COMPLETED within official M10. M11 NOT STARTED.**

## Official scope, requirements and gates

Both full [roadmaps](implementation-roadmap.md) define M10 as linearizable GET via a logged read barrier and stale-leader history, T-019. Applicable requirements are FR-019/AC-019/T-019 and the GET portion of FR-009/AC-009/T-009; GET-based process-recovery evidence complements NFR-003/AC-023/T-023. M11 owns CAS HTTP, replicated sessions and deduplication. Snapshots/compaction remain M13, the general history/property checker M14 and measurements M15. No ReadIndex, leases, local stale fallback, automatic retry or exactly-once. No roadmap reassignment or new blocking scope contradiction.

C-01 retains existing canonical/config/peer and immutable M08 HTTP contracts, extended by an M10 OpenAPI artifact. C-02 remains the tested local Darwin/APFS process-crash contract, not OS/power-loss assurance. C-03 gains the M10 exact-slot/stale-reader/counterexample evidence; its later session/snapshot/general-checker obligations remain open. [ADR-006](adr/ADR-006-linearizable-reads.md) adopts the already selected logged-read strategy for this baseline.

## M09 prerequisite review

Entry Git revision 4062142 was clean. Reviewed actual executable/server/node lifecycle, configuration/identity/LOCK, recovery before serving, cleanup on bind/start failure, signal handling, bounded process waits and child cleanup. Reviewed M07–M09 reports, API/consistency/Raft/storage/failure contracts, architecture audit and pertinent ADRs. The full M01–M09 suite passed before code changes.

M09 reacquires exclusive ownership, validates authoritative CURRENT and WAL, reconstructs only the checkpoint-covered prefix on a fresh M01 machine through M02, and starts follower. The surviving suffix is not inferred committed. Existing tests verify term/vote/log/checkpoint recovery, follower/leader/all-process termination, catch-up, acknowledged entry bytes/actual replay, invalid bootstrap/corruption, canceled waits, ports and locks. Shutdown retains storage ownership until actual drain and reports failure if a process wait expires. Existing Raft vote/election, log-matching, stale-response, current-term commit, backlog and HTTP certainty tests pass again. No blocking M09 defect or earlier production bug correction was needed. No OS/power-loss claim is added.

## Implementation and ownership

Core.EnableLoggedReads and Options.Reads explicitly select M10 before EnableCommit/replay/events; Reads requires Commit. Production node.StartAPI and the executable enable it for every member. Legacy StartCommitted/commit-only modes keep their original read rejection and fixtures. This is homogeneous startup capability, not rolling negotiation; older runtimes cannot reopen read-bearing logs. Configuration hash and wire/storage v1 bytes remain unchanged because read(key) was already an approved schema.

Service.Read uses the existing bounded proposal ticket, sole Raft owner and completion fence. It appends a fresh current-term read(key) after invocation. Local proposal Sync alone is not completion; normal correlated durable majority replication, current-term commit rule and synchronized commit checkpoint are required. The read entry itself may satisfy current-term commit and cover an older prefix. An acknowledgment predating invocation cannot cover its new index. No second read quorum algorithm or direct handler map access.

committed.ProjectWithReads projects the canonical read into M01 Get inside the unchanged M02 worker. The result is captured at that exact index before later mutations, retained with index/term and copied on delivery. LastApplied advances only after actual future completion; ScheduledApplyIndex is separate. Service.Read waits for that result and passes the existing owner-serialized leadership/term/commit/apply fence. The HTTP layer conservatively checks published status/deadline again; it does not substitute local role for quorum evidence. A step-down observed before authorization yields unknown. TCP delivery cannot be atomic with role changes, and a legal past quorum is never revoked.

Reads share the existing 64 active service calls/queued proposals, 64 fence messages, 64 retained results and 32 accepted client connections; peer capacity remains separately bounded. At most 64 retained read values of 16 KiB give a payload bound of 1 MiB, excluding normal object/queue overhead and other copies. No unbounded waiter table, per-read worker or new dependency. Backpressure, cancellation and shutdown reuse the existing path. Disk reserve rejection before recording is not_admitted; possible disk/checkpoint failure is unknown and cannot expose a value.

## HTTP and Go interfaces

POST /v1/kv/get accepts only required key and optional canonical integer timeout_ms 50..5000, default 2000. No client_id/sequence, expected/value or caller request_id. Body cap is 49,152 bytes; keys use the existing 256-byte ASCII grammar. Strict duplicate-name/UTF-8/surrogate/case/unknown-field/trailing-document/depth/number checks precede proposal. Transport timeout/request correlation is never persisted in the read command. HTTP GET on the KV path is 405; local GET /v1/status is unchanged stale diagnostics.

A found key returns 200 OK/completed with canonical string index, exists=true and canonical value_b64; present-empty is an empty string. Absence returns 404 KEY_NOT_FOUND/completed with index and exists=false, omitting value_b64. Neither outcome mutates KV. A follower/candidate returns NOT_LEADER/NO_LEADER with only configured hints; no forwarding/redirect. Timeout after possible recording is 504 TIMEOUT/unknown, observed leadership loss/cancellation/infrastructure failure 503 UNAVAILABLE/unknown and storage failure 507 STORAGE_UNAVAILABLE/unknown. Unknown/not_admitted omit read-result fields. Proven pre-record exclusion retains M08 semantics.

Client.Get sends one bounded POST to a configured member and validates result shape, canonical index/Base64, status/outcome and configured hints. It never retries automatically. A manually repeated GET is a fresh later operation and can see another value. Cancellation/disconnection abandons its wait without canceling committed application. No stable original result, session or exactly-once claim. PUT/DELETE schemas and behavior remain unchanged; CAS is still unavailable.

## Tests and independent evidence

Ten new top-level tests: five raft, three client API, one node and one process scenario. The gated service test commits read index 3 while a later write at 4 commits but apply is blocked. No read response appears before actual M02 completion; even when the later write finishes first at the caller, the delayed read returns its own earlier value. Result-buffer mutation cannot change retained output. A subsequent read sees the new value and a missing read returns KEY_NOT_FOUND. Cancellation and observed higher-term demotion produce unknown while committed apply still finishes. Proposal reserve/uncertain storage failures and checkpoint failure produce no premature read/application/value.

T-019's explicit-event three-node simulation uses seeds 42/117/203, each run twice with identical traces. Real core transitions, synchronous fake disk and actual M02 work retain a permanent committed ghost; an independent list/string model checks read output at every exact slot. It completely isolates n1 and discards in-flight traffic before the new old-leader read, elects n2 on the majority and writes/reads a newer value there. The old leader still locally believes leader, but its new read cannot commit/apply or pass completion. A separate read committed before isolation may deliver its captured result. Healing demotes/supersedes the uncertain receipt; a fresh read-bearing committed-prefix replay preserves the correct indexed result. Seeds vary message order; partition loss is explicit. Older delayed/duplicated/stale-ack regressions remain unchanged, rather than claiming a new general network-fault framework.

HTTP tests cover found/binary/empty/missing results, exact body cap/cap+1, strict invalid fields/numbers, no session fields, unknown timeout shapes, method restrictions and adversarial response shapes. Nine [GET fixtures](../fixtures/http-m10.json) are checked against [OpenAPI 3.1](../openapi/m10.json), runtime projection, actual handler status/body and test-client decoding. The M10 artifact contains all three original M08 path schemas byte-equivalently at the JSON-object level; original [M08 artifact](../openapi/m08.json)/fixtures are unchanged. The existing finite offline schema validator includes negative controls and canonical runtime extensions; no external OpenAPI certification is claimed.

A real loopback cluster checks follower rejection, empty value, five overlapping/sequential PUT/GET/DELETE operations, failover read-after-completed-write and an isolated minority GET timing out unknown without value fields. An independent bounded one-key exhaustive search checks real-time precedence and observed results for that small history; a known stale counterexample must be rejected. This is a test-specific checker, not the full M14 session/CAS/snapshot checker or universal linearizability proof.

The new three-process scenario writes data, performs found/missing GETs, kills all servers abruptly, restarts intact directories containing read entries, performs fresh found/missing GETs, then DELETE followed by GET absence. No cached local read or replay-only HTTP reply bypasses a new quorum barrier. Existing M09 follower/leader/all-process crash, startup, shutdown and resource tests also pass. At most three temporary nodes, small data and explicit timeout/cleanup; no cluster remains running.

## Actual verification and coverage

Go 1.27.4 darwin/arm64, existing local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; normal CGO_ENABLED=0, race 1 with the existing compiler. Actual equivalent checks:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m10.cover -timeout=60s ./...
go test -race -p 1 -count=1 -timeout=60s ./internal/raft ./internal/clientapi ./internal/node ./internal/server ./internal/committed
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

All passed, including the full M01–M09 regression; race reported none in exercised paths. cmd/quorumkv 0.0%; internal/applier 100.0%; internal/clientapi 82.4%; internal/committed 93.3%; internal/config 89.2%; internal/node 81.7%; internal/peerprotocol 91.4%; internal/raft 88.1%; internal/server 90.2%; internal/statemachine 100.0%; internal/storageformat 92.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. Aggregate statement coverage across parent test binaries is not coverage of every schedule, standalone main/child-only execution or proof of consensus/linearizability. The initial restricted-loopback HTTP run could not bind; the authorized local-network run passed, with no code workaround. No benchmark, prolonged stress, CPU/RSS/SSD-wear measurement, OS crash or power-loss test. No new module/go.sum; original fixture arithmetic checker passes. Local bilingual structure/link/example checks and semantic review passed; no external certification.

## Files, acceptance and handoff

Modified internal/raft/core.go, commit.go, replication.go, service.go, application.go and write.go; internal/committed/project.go; internal/clientapi/api.go and client.go; internal/node/node.go. Created internal/raft/read_test.go and read_simulation_test.go, internal/clientapi/read_test.go, internal/node/m10_test.go and internal/server/m10_test.go; docs/openapi/m10.json and docs/fixtures/http-m10.json; these reports. Updated both language API/consistency/architecture/Raft/storage/failure/testing/acceptance/requirements/traceability/development docs, ADR-006, indexes and roadmap. M01/M02, WAL/filesystem/config/parser/transport production and earlier fixtures/tests remain unchanged. No prior bug or blocking issue found; the new layer reuses verified boundaries.

FR-019/AC-019/T-019 and GET FR-009/AC-009/T-009 pass within M10, with GET-based process recovery complementing AC-023/T-023. CAS/dedup/AC-012 remain M11, snapshots M13, final AC-022/T-022/general safety checks M14. C-03's M10 obligation is satisfied without closing later obligations. Read results may expire from bounded retention and yield unknown; storage caps can stop new reads because reads write WAL. All members require the same startup capability. OS/power loss, physical shared-host independence, corrupt/lost durable majority and arbitrary kernel stalls retain previous limitations.

**M10 COMPLETED. M11 NOT STARTED.** The ordered read/recovery foundation is ready for a separate M11 review/assignment; no CAS HTTP or replicated session/deduplication was implemented. Additional spending **€0**, offline standard library, serial packages/two processors, bounded tiny tests and temporary node processes only. No paid service/download, GPU, mandatory VM/container, mass SSD writes or permanent cluster. No Git commit. Stop at M10.

Documentation verification: 128 Markdown files, 63 language pairs, 1,723 relative links, 106 tables and 31 paired technical examples; zero structural errors.
