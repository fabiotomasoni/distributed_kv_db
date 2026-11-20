# M04 — RPC and configuration

[Index](README.md) · [Italiano](../it/milestone-m04.md)

**20 November 2026 — COMPLETED within the official M04 scope. M05 NOT STARTED.**

## Scope, prerequisites and decision

The Italian and English [approved roadmaps](implementation-roadmap.md) define M04 as **RPC and configuration**: envelope/validation and canonical configuration fixtures, after M02/M03, with T-001/T-002/T-003/T-029. Linked requirements: FR-001/FR-002/FR-003, NFR-009 and the loopback/security limits. This is not election or replication. Git entry baseline 79d24f0 was clean. Both roadmaps, acceptance criteria, audit/pre-M03 reviews, M03 report, applicable ADR-002/ADR-004 and existing M01–M03 interfaces were examined. C-01 storage formats remained RESOLVED; its M04 configuration/protocol fixture obligation is fulfilled here. C-02 remains VERIFIED only for the tested local macOS/APFS profile. C-03 and later end-to-end gates are retained.

M03 was authorized as a prerequisite only after a bounded regression/coverage review and the correction below. Existing M01/M02 contracts, approved storage bytes and go.mod remain unchanged. No automatic application of recovered data, client API, timers, vote decision, log replication, snapshot staging/installation, CLI or M05 code was introduced.

## Preliminary M03 regression and correction

The baseline full suite passed: WAL 82.9%, storageformat 81.7%, storagefs 80.1%, applier/statemachine 100% package-local statement coverage. Examined streaming tail classification, error/fatal paths, commit/vote transitions, recovery, lock/close and uncovered branches. Existing tests cover synchronized acknowledgment and injected write/Sync/repair failures; no need to change those interfaces.

A focused new test reproduced a fail-closed gap: after a valid initial frame, an incomplete final frame with a readable but duplicate/gapped physical sequence was classified repairable. Before the fix TestIncompleteTailWithWrongSequenceFailsClosed failed on sequence 1 with a nil error. reader.go now validates the physical sequence as soon as the complete sequence field is available (20 bytes), before permitting tail truncation; MaxUint64 cannot wrap. The regression covers sequences 1/3 instead of 2, cuts at 20/25 bytes, Replay rejection and actual Open rejection with file bytes unchanged. The WAL suite then passed. This is a minimal integrity correction, not a format or acknowledged-durability change. WAL now has 18 top-level tests; prior 25 subcases remain. Non-arbitrary process-crash tail limitations remain unchanged.

## Components, interfaces and ownership

| Component | Actual interface and responsibility | Error and lifetime contract |
|---|---|---|
| strictjson | Decode/Fields/Uint/Integer/Hex/Base64; bounded lexical/schema foundation | ErrInvalid; no I/O/state; duplicate unescaped names, Unicode, depth and EOF checks |
| config | Parse; immutable Config accessors, Members and SharedBytes copies | ErrConfig; explicit loopback/static membership/quotas/timing; no listener or storage |
| peerprotocol | DecodeRequest/DecodeResponse; owned Message with Kind/Envelope/Bytes copies | ErrIdentity versus ErrMessage; validates structure/correlation, owns no term/log; only a validated request can correlate a response |
| transport | New/Serve/Send/Shutdown; Processor callback | ErrHTTP/ErrClosed or contextual/network errors; bounded HTTP/1.1 resources, cancellation and draining |
| node | Start/Peer/Recovered/Shutdown | Storage validation/LOCK before bind; drain before WAL close, return listener/close errors |

Dependency direction: node → config/transport/wal; transport → peerprotocol/config; peerprotocol → config/strictjson/storageformat; config → strictjson. No reverse dependency or call to the state machine/applier. M02 is preserved as the future ordered application boundary, not connected to uncommitted network messages.

The [configuration contract](configuration.md) fixes exact fields, immutable access, six unique loopback endpoints, canonical hash bytes and startup order. [configuration-v1.json](../fixtures/configuration-v1.json) is the checked hash preimage/reference; [peer-v1.json](../fixtures/peer-v1.json) contains exact request/response bytes for vote, append and snapshot, including obsolete response. They do not alter canonical-v1.json. Vote grants in real operation remain unavailable: a nil processor returns 503. Simulated processors exist only in tests.

## Protocol, limits and technical decisions

All three future RPC schemas are validated now; schema support does not implement their consensus effects. Exact fields, identity/version/config, canonical uint64 strings, candidate/leader identity, contiguous bounded entries, storage command schema, nondecreasing terms and response correlation are checked before delivery. Snapshot chunks are decoded/bounded without staging files or treating a digest as a verified image. Response match_index is correlated to the sent batch; obsolete resume_index cannot precede the sent snapshot boundary. Semantic vote/term/leadership decisions remain future core work.

Strict UTF-8 JSON rejects duplicate decoded keys, wrong-case/unknown names, unpaired surrogates, depth >16, concatenated documents, noncanonical integers/Base64. Equivalent legal field order/escapes normalize successfully; the strict persistent codec is reused only after canonical entry projection. Config cap 16 KiB; RPC cap 384 KiB; append <=16 entries, <=256 KiB commands, <=48 KiB each entry; snapshot raw chunk <=64 KiB, image <=8 MiB. rpc_id uses 1–128 ASCII letters/digits or -_.:, owned for uniqueness by the caller/process; stale correlated leadership effects still belong to the future core.

Transport uses the standard library only: no proxy, redirect follow, compression, TLS or external service. Explicit matching loopback listener, configured Host, POST/known paths and JSON media type; 400 schema, 409 envelope, 413 oversize, 415 media, 404 route, 405 method, 503 unavailable/invalid processor. Non-200 is a transport error, never a vote. Dial 100 ms, outgoing total 300 ms including same-peer admission, headers 500 ms, incoming handler/read/write and idle 1 s; pool <=2 per remote host and 16 accepted sockets including idle. One active outgoing call per peer, caller-owned cancellable waiters, one attempt without hidden retries. No worker/retry queue. Responses are bounded, closed and revalidated.

Shutdown permanently closes admission, cancels base context and I/O, closes sockets/pool, drains admitted work, then node closes WAL. Node keeps LOCK while a processor is still draining even if a caller abandons waiting. Unexpected serve failure triggers the same cleanup and remains observable through Shutdown. Processor is a trusted finite/context-compliant adapter; Go cannot force arbitrary callbacks to terminate. Direct transport is an injectable low-level library; storage-before-bind is enforced by node.Start. No client listener or production CLI yet.

## Tests and actual checks

12 new top-level M04 tests: strictjson 2, node/integration 7, transport 3; plus the M03 regression. TestConfigurationCanonicalFixtureAndRejections checks exact hash, local/permutation invariance, immutable copies, duplicate IDs/endpoints, IPv4-mapped alias, invalid ports/path/quota/timing/version. TestProtocolFixturesAndInvalidMessages checks all fixture unions, request/response role rejection, correlation, duplicate/case/numeric/identity errors and legal equivalent escapes. TestProtocolSizeAndSnapshotBoundaries covers command aggregate cap, maximum value, index wrap, snapshot limits/offset/done/Base64/digest and negative append union.

TestThreeNodeStartupIdentityAndLock starts three temporary isolated identities, rejects duplicate ownership/mismatched persistent identity before listener creation, injects bind failure and verifies lock release/data preservation. TestTransportRoundTripValidationCancellationAndShutdown verifies real loopback request/reply, malformed/oversize/media/Host/method errors before processor, admission cancellation, no retry and shutdown/repeated wait. TestNodeShutdownKeepsLockUntilProcessorDrains and TestListenerFailureDrainsAndReleasesStorage check lifecycle ordering/error visibility. Transport tests additionally inject a stalled peer, malformed response, redirect, 503, invalid processor and a finite accepted-socket source for the exact 16-connection cap/release.

All concurrency tests use channels/contexts and bounded joins, no time.Sleep synchronization. Real loopback ports are OS-assigned and held by test listeners; a test-only listener factory preserves startup ordering checks. Local listener access required a less restricted test invocation because the restricted environment denied loopback bind; there are no external connections. Existing WAL child-process/lock/publication/failure regressions ran again.

Toolchain: Go 1.27.4 darwin/arm64; macOS 27.2.1 build 26B131, local APFS. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normally, 1 for race with existing compiler. Equivalent installed-toolchain commands actually executed:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m04-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/node ./internal/transport ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Verification | Actual result |
|---|---|
| Formatting, vet, build | Passed, native darwin/arm64. |
| M04 plus M01–M03 regression | All packages passed; new M04 tests and WAL regression passed. |
| Race detector | node, transport, wal, storagefs, applier passed; no reported races in exercised paths. |
| Fixtures/dependencies | Existing fixture verifier passed, original fixtures/M01/M02/go.mod unchanged; root module only, no go.sum or new dependency. |
| Aggregate statement coverage | config 86.3%, peerprotocol 91.4%, strictjson 92.1%, transport 90.3%, node 80.0%; WAL 82.7%, storageformat 91.7%, storagefs 87.9%, statemachine/applier 100%. |

Coverage is the union of instrumented blocks across test binaries using -coverpkg=./..., not package-local coverage. Zero-package standalone lines for config/peerprotocol are superseded by their exercised integration coverage. This differs from the preliminary package-local M03 measure; changed denominators mean the WAL percentage is not evidence of a regression. Untested fault/schedule branches remain; no claim of exhaustive or formal proof.

## Files and documentation

Created internal/strictjson/json.go and json_test.go, internal/config/config.go, internal/peerprotocol/protocol.go, internal/transport/peer.go and peer_test.go, internal/node/node.go and m04_test.go. Modified only internal/wal/reader.go and reader_test.go in prior implementation. Added the two shared JSON fixtures and bilingual configuration.md/milestone-m04.md. Updated relevant shared/language indexes, roadmap, architecture, development guide, network/canonical-format contracts, ADR-002/ADR-004, testing/acceptance/traceability and historical audit/M03 follow-up pointers. Historical results remain phase-labeled. No prior contract was bypassed.

## Acceptance, limitations and next readiness

| Criterion | M04 result and boundary |
|---|---|
| T-001 / AC-001 / FR-001 | Passed for library startup, three identities/directories, exclusive owner and failure-before-bind; process CLI exit-code UX remains later. |
| T-002 / AC-002 / FR-002 | Passed for strict membership/canonical hash, divergent persistent identity and rejected incompatible RPC/configuration. |
| T-003 / AC-003 / FR-003 | Passed for validated exchange, malformed/oversized input and no processor effects; M04 owns no application state to mutate. |
| T-029 / AC-029 / NFR-009 | Native ARM64 build/tests passed with recorded toolchain. |
| Gates and scope | C-01 M04 fixtures fulfilled; C-02 local limitations preserved; C-03/OpenAPI M08 and later gates remain open. No mandatory M04 criterion left unsatisfied. |

No distributed database operation or Raft safety proof is claimed. Snapshot state machine/publication, timing/backoff, election/term transition handling, replication, client sessions/API, server process commands, full durability and observability remain at their respective milestones. No OS/power-loss/physical high-availability evidence. The trusted processor contract and lack of runtime CLI are explicit integration limits, not hidden implementations of later phases.

Budget **€0 additional**, local standard-library work only, no paid/cloud/API/installation/GPU/VM/container. Two processors, serial package execution, <=30 s package deadlines, <=5 s child/context bounds, tiny temporary synthetic files; no benchmark, disk filling, prolonged stress, personal/system file changes or destructive operation. No resource measurements invented.

**Final M04 status: COMPLETED.** M05 has usable typed/configuration/transport/storage boundaries for its own preliminary review, but election tests, persistent voting integration and its acceptance gates must still be implemented under a new assignment. M05 NOT STARTED. No Git commit performed.

Documentation checks: 116 Markdown files, 57 language pairs, relative links, table/fence structure, technical example parity and reciprocal language links checked without structural errors. Semantic equivalence reviewed locally; no external certification or visual Mermaid rendering.

## M05 handoff follow-up — 20 November 2026

The M04 report above records its handoff. [M05 is now completed](milestone-m05.md) using the existing configuration/protocol/transport/storage interfaces; only node lifecycle gains the optional election integration. M04 implementations/fixtures and the WAL sequence-tail regression are unchanged and pass again. M06 not started.
