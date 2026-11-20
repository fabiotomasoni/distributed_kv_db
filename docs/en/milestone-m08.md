# M08 — PUT/DELETE HTTP and test client

[Index](README.md) · [Italiano](../it/milestone-m08.md)

**21 November 2026 — COMPLETED within official M08. M09 NOT STARTED.**

## Official scope and M07 blocking review

Both complete [roadmaps](implementation-roadmap.md) assign M08 to PUT/DELETE HTTP, deadlines, unknown/not_admitted classification, derived OpenAPI and HTTP fixtures/test client, T-009/T-012 subsets. FR-009/FR-012 and AC-009/AC-012 are satisfied only within that subset. M09 owns full crash recovery/server MVP; GET is M10, CAS/sessions/deduplication M11, advanced observability M12, snapshots M13 and final checker M14. No roadmap reassignment or later implementation.

Entry revision be097f4 was clean. Reviewed M06/M07 code/reports, API/consistency/Raft/storage specifications, M02 futures and ownership, M04 transport/configuration, applicable ADRs and C-01/C-02/C-03 gates. Ran all M01–M07 tests before implementation: passed. Current-term majority selection includes only correlated durable matches, preserves old prefixes through current-term commit and synchronizes the checkpoint before apply. Follower commit is capped by the verified prefix. Application tracks Scheduled separately from LastApplied, uses exact indexed/termed results and reconstructs only recovered committed entries on a fresh machine. Existing old-term, stale-response, partition, checkpoint-failure, 5→7 apply backlog, replay and failover tests passed again. No blocking M07 defect or prior production fix was necessary.

## Components, ownership and lifecycle

New internal/clientapi provides strict wire validation/projection, bounded HTTP server, per-attempt IDs and a single-attempt test client. internal/raft/write.go adds explicit admission tickets and completion fences to the existing service owner; commit/application algorithms and storage bytes remain unchanged. Client requests use a separate 64-slot queue, preserving the existing 256 peer-event capacity. At most 64 active service write calls and 64 completion-fence messages; result retention remains the M07 ring of 64. Returned results include the receipt index and term. Expired or superseded results fail uncertainly rather than borrowing another operation's result.

The ticket lock makes pre-record cancellation a permanent exclusion. The owner validates the pure transition, marks recording BEFORE executing possible disk I/O, releases the ticket lock and uses the existing synchronous WAL effect. Cancellation thereafter abandons the wait only. Buffered single replies prevent abandoned clients from blocking the owner. Explicit WAL reserve rejection before writing is the narrow safe storage exclusion. No per-entry result-delivery goroutine or unbounded queue. Queued canceled tickets may temporarily occupy bounded slots until drained.

node.StartAPI reserves both configured listeners only after identity/LOCK/recovery and fresh committed replay, then starts the immutable components. Two HTTP servers are needed because M04 already specifies distinct client and peer ports. They share the M04 accepted-connection registry/limiter; peer transport/protocol is not reimplemented. Client sockets cap at 32 including idle, handler write slots at 64; headers/body/write/idle are bounded by 500 ms/5 s/6 s/1 s. Start failure closes acquired listeners and releases storage. Shutdown closes admission, cancels/joins HTTP waiters, drains the existing committed application obligation, joins peer transport and only then closes WAL/LOCK. A canceled shutdown caller bounds its wait, not storage ownership.

## Exposed API and result semantics

POST /v1/kv/put requires key/value_b64/client_id/sequence; POST /v1/kv/delete requires key/client_id/sequence. Both allow canonical integer timeout_ms 50..5000 (default 2000). Deadline begins at handler admission, including body parsing. Local GET /v1/status returns stale node_id/role/term/commit_index/last_applied only. Unsupported KV GET/CAS are 404 INVALID_ARGUMENT/not_admitted; wrong method/media 405/415. No forwarding or redirects. A follower/candidate returns NO_LEADER or NOT_LEADER with hints only from configured membership.

Strict JSON rejects duplicate decoded names, unknown/case-mismatched fields, invalid UTF-8/surrogates, excess depth, trailing documents and noncanonical integer/Base64 forms. Keys, client IDs, sequence and decoded values use existing canonical validation. The 49,152-byte body cap is tested at exact/cap+1, key at 256/257 and value at 16,384/16,385 bytes; empty values remain valid. Transport request_id/deadline never enter the unchanged persistent command schema. IDs are generated per attempt with a random prefix and counter. Values are absent from errors/logs.

Local Sync is insufficient for completed. Service.Write waits for actual M02 application of the exact receipt, then a serialized owner fence verifies healthy current leadership/term and sufficient durable commit/LastApplied. The HTTP layer conservatively rechecks published state and deadline before encoding. This authorizes completion at that event; role changes and TCP delivery cannot be physically atomic. Unobserved loss after a legal quorum does not revoke the past commit. PUT returns 200/OK/applied=true; DELETE 200/OK/deleted=true or false for an absent key. RESOURCE_LIMIT is deterministic 409/completed with index and no mutation. There is no public map access or read shortcut.

## Deadlines, uncertainty and retry boundary

Before recording, a canceled/expired queued ticket is permanently excluded: UNAVAILABLE/not_admitted. Input/body errors, follower/candidate and overload are also not_admitted. Pre-write WAL reserve rejection is STORAGE_UNAVAILABLE/not_admitted. This certainty is about this attempt, never an earlier attempt sharing identity. After possible recording, deadline is 504 TIMEOUT/unknown; observed leadership loss, cancellation or infrastructure failure is 503 UNAVAILABLE/unknown, storage failure 507 STORAGE_UNAVAILABLE/unknown. Unknown omits index/operation success fields and never implies rollback. Slow body reading is capped at 5 s; a requested deadline already expired after parsing cannot append.

Disconnect after recording cannot cancel committed apply. A response lost after apply leaves the client uncertain. The test client has at most four active calls, no ambient proxy/compression/redirect, configured-member targets and validated exact configured hints, response cap and strict envelope checks. It performs one POST with a timeout margin of 250 ms, no automatic replay, hint following or retry. Transport errors are unknown; local pre-send validation/cancellation is not_admitted.

client_id/sequence are validated/persisted metadata only. There is no replicated session table, deduplication, exactly-once/at-most-once guarantee, stable original result or retry-after-restart promise. Repeating identical DELETE can return true then false at different indices; tests explicitly verify this. The broad final API retry examples remain future M11 design. GET barriers and ReadIndex are absent; logged GET remains the approved future strategy.

## Tests and finite evidence

14 new top-level tests across raft (5), clientapi (7 including contract) and node (2). Service tests gate actual M02 application and disk persistence, cancel recorded versus queued attempts, ensure a canceled queued command never appends, reserve peer capacity under client saturation, reject stale completion fences and exercise nonleader/closed/invalid/reserve errors. A higher-term AppendEntries while apply is blocked forces an unknown result after the committed work still finishes. Direct fence tests separately reject insufficient apply, changed role and changed term.

HTTP tests cover malformed/oversized input, canonical projection, empty/max values and key/body boundaries, timeout schema, fresh request IDs, result/error status matrix, configured hints, unsupported future routes, host/media/query validation, overload and shutdown joining a recorded wait. Client checks reject forged hints, invalid success indices/booleans and certainty/status mismatch. A real socket is dropped after handler application but before any response bytes: the client reports unknown, exactly one backend call occurred, and a subsequent DELETE proves the effect. Repeating DELETE proves the absence of deduplication.

The real three-node loopback test uses production WAL/transport/Raft/application/API/test-client adapters. It rejects a follower write, confirms PUT and its replica application, completes two overlapping writes at distinct indices, stops the leader, deletes the preserved value through its elected successor and repeats the same identity as an independent operation. Stopping the third member leaves a lone leader: a new locally logged write times out unknown without commit acknowledgment. All six listener addresses and three WAL locks are reacquired after shutdown. Client-bind failure separately verifies peer/storage cleanup. No cluster remains active.

[OpenAPI 3.1](../openapi/m08.json) covers exactly PUT/DELETE and local status. [20 shared fixtures](../fixtures/http-m08.json) are checked against request/response schemas, runtime decoder, server status/body and test-client validation. The offline finite validator covers every assertion keyword used in this artifact, rejects unsupported assertions and includes negative controls for missing/extra fields and range violations. Canonical uint64/Base64 and configured hints additionally use runtime checks/extensions. This is bounded contract evidence, not independent external OpenAPI certification or a universal linearizability proof. Old canonical storage/config/peer/election/replication/commit fixtures are unchanged.

## Actual checks and coverage

Go 1.27.4 darwin/arm64, existing local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normally, 1 with the existing compiler for race. Actual equivalent commands:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m08.cover -timeout=45s ./...
go test -race -p 1 -count=1 -timeout=45s ./internal/raft ./internal/clientapi ./internal/node ./internal/transport
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

All passed; race reported none in exercised paths. Aggregate statement coverage across binaries: applier 100.0%; clientapi 82.5%; committed 92.3%; config 88.2%; node 79.8%; peerprotocol 91.4%; raft 88.1%; statemachine 100.0%; storageformat 92.6%; storagefs 87.9%; strictjson 92.1%; transport 90.1%; wal 82.7%. Coverage does not prove every schedule, crash or consistency property. Original formats/go.mod unchanged, no go.sum or new dependency; the module inventory contains only this repository. No benchmark, physical-fault experiment or hardware/resource measurement. Documentation structure, reciprocal links, paired examples and bilingual parity were checked locally; no external certification.

## Files, gates, constraints and handoff

Created internal/raft/write.go and write_test.go; internal/clientapi/api.go, client.go, api_test.go and contract_test.go; internal/node/m08_test.go; docs/openapi/m08.json; docs/fixtures/http-m08.json; these bilingual reports. Modified only service dispatch, node startup/shutdown and transport limiter reuse in prior production code. M01/M02/core commit/WAL/codec/filesystem/config/strict parser production remains unchanged. Updated both language indexes, API, architecture, consistency, Raft, failure model, testing, acceptance, traceability, roadmap and ADR-004/ADR-007 plus root index. Historical M06/M07 evidence is retained.

C-01's M08 OpenAPI/fixture subset is satisfied; full API stability requires M10/M11. C-02 remains the tested Darwin/APFS local contract; no OS/power-loss assurance. C-03 M07 backlog evidence is preserved, later snapshot/session/read/history gates remain open. No known blocking defect remains within M08. Complete client linearizability checker, GET, CAS HTTP, dedup, snapshots, CLI/server process recovery and deployment remain excluded.

**M08 COMPLETED. M09 NOT STARTED.** Its prerequisite library interfaces are ready for a separate M09 review/assignment; full recovery/MVP acceptance has not been performed here. Additional spending **€0**, offline standard library, at most three temporary ordinary nodes, small synthetic datasets, two processors/serial packages and explicit short timeouts. No paid service/download, GPU, mandatory VM/container, prolonged stress, mass SSD writes or personal/system-file changes. No Git commit. Stop at M08.

Documentation check: 124 Markdown files, 61 language pairs, 1,607 relative links, 106 tables and 27 paired technical examples; zero structural errors.
