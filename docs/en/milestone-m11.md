# M11 — CAS HTTP and request deduplication

[Index](README.md) · [Italiano](../it/milestone-m11.md)

**22 November 2026 — COMPLETED in session profile v2. M12 NOT STARTED.**

## Official scope and M10 review

Both [roadmaps](implementation-roadmap.md) assign M11 CAS HTTP and persisted sessions/deduplication, T-010/T-011/T-012: FR-010/AC-010/T-010, FR-011/AC-011/T-011 and FR-012/AC-012/T-012. M12 retains general fault injection, reproducible manifests and observability; snapshots/compaction M13, the general checker M14 and measurements M15. No scope reassignment. C-01 gains new versioned fixtures; C-02 retains only the tested local macOS/APFS contract; C-03 satisfies its M11 obligation without closing later snapshot/checker gates.

Clean entry revision 3dd071a. Reviewed contracts, ADRs, M08–M10 reports and GET admission, current-term append, persistence, durable quorum, checkpoint, M02, exact-slot result and owner completion fence. PUT A / GET / PUT B retains the GET-slot result even with delayed delivery. No substitute local read; an isolated old leader or absent quorum cannot authorize new definitive reads. Cancellation, term change and replay behavior are preserved. The M01–M10 suite passed before changes and again at completion. No blocking M10 defect or earlier bug correction was identified.

## Compatibility and architectural decision

[ADR-010](adr/ADR-010-session-profile.md) records a verifiable necessity: M08–M10 v1 logs may contain repeated identities with different already acknowledged effects. Retroactive deduplication would lose logical data. M11 requires config_version=2/application_version=2, a shared hash including the profile and sealed IDENTITY format_version=2. Explicit bootstrap into new directories; no automatic migration, reset or in-place conversion. V1 keeps M10 semantics. Cross-profile opening fails before replay or WAL-tail repair. Partial v2 startup without API/sessions is rejected.

Entries, QCMDv1 digests, WAL frames/transactions, CURRENT and empty snapshots keep original v1 codecs. No earlier fixture changes. Mixed-profile peers have incompatible hashes. V2 mutations require exactly one X-QuorumKV-Application-Version: 2 header; the server acknowledges the profile in its response. The client checks the acknowledgment before trusting the result: historical binaries may ignore new headers. Missing/wrong/multiple headers are rejected before admission; v1 rejects the new header. GET keeps its M10 body and contract. The [profile fixture](../fixtures/configuration-v2-sessions.json) and examples/m11 also separate cluster ID, ports and directories.

## CAS and HTTP contract

POST /v1/kv/cas orders the existing canonical command. Comparison and update are one M01 transition through M02, without handler GET/compare/PUT. Expected distinguishes absence from present-empty. Success: 200 OK/completed/applied=true. Mismatch: 409 COMPARE_FAILED/completed/applied=false, a normally committed entry. RESOURCE_LIMIT is a cached negative application result. Distinct clients with different replacements and the same expected cannot both successfully change the value; CAS operations writing the same value may both succeed in legal order. No ABA solution.

Strict pre-proposal validation: ASCII key up to 256 bytes, values up to 16 KiB, canonical Base64, discriminated expected, exact fields, canonical identity/sequence, timeout_ms 50..5000 (default 2000), body limit 49152 bytes. Tested maximum canonical CAS request 44116 bytes; maximum entry 44184, within unchanged M02 limits. Followers/candidates do not forward requests. not_admitted proves exclusion before recording; after possible append, errors, timeout, cancellation, shutdown and leadership loss remain unknown. Unknown contains no definitive application result.

[OpenAPI 3.1 M11](../openapi/m11.json) and [18 HTTP fixtures](../fixtures/http-m11.json) define CAS, session errors, headers and responses. PUT/DELETE request schemas and GET/status operations are compared with M10; v2 explicitly adds the header requirement and session outcomes without rewriting historical artifacts. Finite offline validator with negative controls and handler/client tests; no external OpenAPI certification.

## Identity, algorithm and retention

Logical identity is (client_id, sequence), distinct from transport request_id: client_id is 32 lowercase hexadecimal digits; sequence is a canonical JSON integer 1..2^53-1. SHA-256 covers typed QCMDv1 operation, key, expected discriminator/value and replacement; identity, sequence, index, term, deadline and JSON representation are excluded. Reusing an identity with different payload yields REQUEST_CONFLICT.

The session wrapper is owned exclusively by M02 around unchanged M01. New client: capacity first, then sequence 1. Known client: last+1 applies once and caches OK/COMPARE_FAILED/RESOURCE_LIMIT; last with equal digest returns original result/index; last with different digest REQUEST_CONFLICT; lower STALE_SEQUENCE; gap SEQUENCE_GAP. These protocol errors neither mutate KV nor replace the valid record. Read/noop do not register sessions. No preliminary HTTP check decides deduplication.

At most 1024 sessions, no TTL/eviction: a new client at capacity gets SESSION_LIMIT; existing clients continue. Only the latest compact result, at most 512 bytes, without old values, plus highwater/digest/kind/index. Advancing loses the historical result but prevents lower sequences from reapplying. IDs are not recycled through expiry; exhausted limits require a future explicit decision rather than silent deletion. No perpetual retention of every result or general exactly-once.

## Quorum, retry and recovery

Every attempt, including duplicates, crosses a new current-term append, durable replication/quorum, checkpoint, M02 application and owner leadership/term/commit/apply fence. The logical result may refer to its original index; authorization uses the current attempt index/term. An isolated old leader with populated cache cannot complete a new retry without quorum. Results in the 64 temporary slots may expire: conservative unknown, without another effect.

Sessions are reconstructed only from the committed WAL prefix on a fresh machine; no additional file or database. An uncommitted suffix does not register deduplication, but may apply if later legally committed. Failover, replay and all-process crashes also reconstruct failed CAS results. Retry preserves original client_id/sequence/payload; a new operation advances only after resolving its pending outcome. Client.CAS/Write makes one bounded attempt; no infinite retry/backoff or automatic client identity persistence. Retaining pending identity and command across client crashes is the caller's responsibility. Repeated GET remains a fresh read.

## Tests and observed invariants

20 new top-level tests across nine files: three committed, one config, one storageformat, one wal, six raft service plus one raft simulation, five clientapi, one node and one process test. They cover absent/present-empty/successful/failed CAS, digests, maximum command, sequences/conflicts/distinct clients, quota and 1024 sessions without eviction. Gated apply demonstrates no confirmation before completion, one transition for concurrent duplicates, waiter-only cancellation, shared backpressure and draining shutdown. Proposal/checkpoint failures produce unknown without premature application; restart and retry recover one effect. The new I/O scenario passed ten times under race detection.

Explicit three-node simulation with seeds 42/117/203, each run twice with identical traces, a permanent committed ghost and an independent normalized-argument/highwater/value oracle. It checks result sequences and original indices, partition with message loss, isolated leader, new leader, successful/failed retry after state changes, healing and fresh replay. Existing regressions retain duplicated messages, stale responses, conflicts and current-term commit rules. This is not the M12 framework or M14 general checker.

Real HTTP cluster: CAS 200 response lost after authorization/application, client unknown, leader shutdown and new-leader retry with original index; competing distinct-client CAS, duplicates on distinct connections, payload conflict, gap/stale, absent quorum. Three real processes: follower crash, leader crash, all-process crash, intact-directory restart and retries of both successful and failed CAS after state changes, followed by verification GET. Advancing and an old retry do not reapply. Explicit timeouts and cleanup; at most three temporary nodes.

## Actual verification and coverage

Go 1.27.4 darwin/arm64 on the local macOS/APFS profile. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; normal CGO_ENABLED=0, race 1 with the existing compiler. Build uses GOOS=darwin/GOARCH=arm64. Equivalent checks actually executed:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m11.cover -timeout=75s ./...
go test -race -p 1 -count=1 -timeout=75s ./internal/committed ./internal/raft ./internal/clientapi ./internal/node ./internal/server ./internal/wal ./internal/config ./internal/storageformat
go test -race -p 1 -count=10 -timeout=40s ./internal/raft -run TestSessionCASIOFailureAndUncertainRecovery
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Full M01–M11 suite passed; targeted race passed without reports in exercised paths. gofmt found no files to correct, vet/build passed, single module without new dependencies/go.sum. Original canonical fixture arithmetic verification passed. External networking was not used; only temporary localhost was authorized for socket tests.

Aggregate statement coverage, taking the union of identical blocks across suite profiles: cmd/quorumkv 0.0%; internal/applier 100.0%; internal/clientapi 81.7%; internal/committed 82.9%; internal/config 89.7%; internal/node 81.8%; internal/peerprotocol 91.4%; internal/raft 88.3%; internal/server 90.2%; internal/statemachine 100.0%; internal/storageformat 93.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. This does not cover all schedules or credit main for code exercised only in child processes. It is not a formal proof of consensus, linearizability or deduplication.

## Inventory and resolved issues

Created internal/committed/sessions.go and nine test files: internal/committed/sessions_test.go, internal/config/session_profile_test.go, internal/storageformat/session_identity_test.go, internal/wal/session_profile_test.go, internal/raft/sessions_test.go, internal/raft/session_simulation_test.go, internal/clientapi/cas_test.go, internal/node/m11_test.go, internal/server/m11_test.go. Modified internal/config/config.go, internal/storageformat/manifest.go, internal/wal/open.go, bootstrap.go and store.go, internal/raft/service.go, application.go and write.go, internal/node/node.go, internal/clientapi/api.go and client.go. Storage/config changes provide the necessary version boundary; Raft core, M01/M02, filesystem, transport and parser remain unchanged.

Created M11 OpenAPI/fixtures, v2 configuration fixture, examples/m11/n1.json–n3.json, ADR-010 and these reports. Updated both languages' API, data/consistency model, architecture, canonical formats, configuration, recovery, faults, requirements/traceability, testing, acceptance, development guide, ADR-007, indexes and roadmap; updated main READMEs. Earlier milestone files and fixtures were not rewritten.

During integration the new golden test caught a hash built through the public accessor before validation: fixed with a shared private builder while retaining identical v1 bytes. Fixed the HTTP mock header name to follow Go normalization. The new I/O scenario required explicit manual-clock progress during retry after an in-flight heartbeat: corrected only the harness and verified ten times with race detection. Strengthened rejection of v2 configuration without session capability in the service constructor. No original fixture weakened; no retroactive M01–M10 bug correction. No remaining known blocking issue.

## Limits, acceptance and handoff

Verified CAS atomicity, original results, no second effect for recognized duplicates, no premature apply/response, deterministic replay, failover and uncertain-outcome classification in the described scenarios. Assigned M11 criteria FR-010/AC-010/T-010, FR-011/AC-011/T-011 and FR-012/AC-012/T-012 are satisfied in profile v2. Guarantees require stable identity/payload, an available durable prefix and homogeneous cluster; only the latest per-session result is recoverable. Retry consumes WAL and requires space/quorum; storage quotas can prevent it. No v1 migration, snapshots/compaction, session recycling, client identity persistence or general exactly-once.

No formal proof or exhaustive checker, prolonged stress, benchmark, RSS/CPU/SSD-wear measurement, durable-majority loss, OS crash or power-loss test. Process crashes do not demonstrate absolute OS/power-loss durability. Three processes on one Mac/SSD do not provide physical independence; existing local macOS contract limits remain. Documentation verification: 132 Markdown files, 65 language pairs, relative links/structure/technical examples aligned without errors; semantic review performed, no external certification.

**M11 COMPLETED. M12 NOT STARTED.** Preliminary M12 readiness: apply/session boundaries, v2 fixtures and reproducible scenarios are available; the next assignment must review gates and design manifests/metrics/fault framework without anticipating them here. Additional spending **€0**: standard library/offline, no service/purchase/download or new dependency. Two processors and serial packages, tiny datasets, at most three nodes, isolated temporary directories and cleanup; no cluster left running. No GPU, mandatory VM/Docker, mass writes or real power-loss simulation. No Git commit. Stop at M11.
