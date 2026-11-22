# Development and local MVP guide

[Index](README.md) · [Italiano](../it/development-guide.md)

**Updated: 22 November 2026 — M01–M10 implemented within their phase scopes; PUT/DELETE process MVP available.**

## Free prerequisites

MacBook Pro M5/macOS ARM64, terminal, Git, an existing editor and Go darwin/arm64. M01 verified baseline: **Go 1.27.4 darwin/arm64**; go.mod requires at least Go 1.27.4 and the [M01 report](milestone-m01.md) records the exact tested toolchain. The [official release history](https://go.dev/doc/devel/release) remains the upgrade reference. No beta versions or automatic downloads during offline tests; `GOTOOLCHAIN=local` prevents implicit fetching. Apply security patches after review and the suite; plan minor upgrades rather than changing mid-experiment.

Go and the standard library suffice for runtime; the toolchain has a [BSD license](https://go.dev/LICENSE). Git is a development tool, not a runtime dependency. No third-party Go packages are initially proposed: no generators, CGO, external DBs, packaged Raft or telemetry services. GitHub Free is optional. A Mermaid renderer is optional for viewing diagrams; source remains available offline. QuorumKV's license must be chosen by its owner before publication.

Every new dependency requires an ADR: concrete need, standard-library alternative, pinned version, license, maintainer activity, known vulnerabilities, ARM64 compatibility without extra toolchains, RAM/disk costs and offline acquisition/cache procedure. Never add a package merely because production projects commonly use it.

## Validated configuration schema

The M09 node binary is now available. This n1 example conforms to the implemented schema; n2/n3 change node_id, data_dir and bind addresses according to [networking](network-protocol.md). Shared fields remain identical. config_hash derives from cluster_id, the member list sorted by ID and replicated quotas; it excludes local data_dir and binds. M04 fixes the exact canonical encoding in the shared fixture: [contract](configuration.md).

```json
{"config_version":1,"cluster_id":"11111111111111111111111111111111","node_id":"n1","client_addr":"127.0.0.1:7401","peer_addr":"127.0.0.1:7501","data_dir":".local/n1","members":[{"id":"n1","client":"127.0.0.1:7401","peer":"127.0.0.1:7501"},{"id":"n2","client":"127.0.0.1:7402","peer":"127.0.0.1:7502"},{"id":"n3","client":"127.0.0.1:7403","peer":"127.0.0.1:7503"}],"max_logical_bytes":4194304,"max_keys":10000,"max_sessions":1024,"heartbeat_ms":100,"election_min_ms":600,"election_max_ms":1000}
```

Distinct free ports, immutable IDs, same protocol version. First startup requires explicit `--bootstrap` on an empty directory and a common manifest; ordinary startup never accidentally reinitializes empty directories. Bootstrap does not appoint a special leader: everyone starts as follower and follows Raft.

## M09 node executable and local operation

From the repository root, build once and create private empty directories for a new cluster. The three [example configurations](../../examples/n1.json) already agree on membership. Run one command per terminal; keep at most three node processes. node_started means initialized/listeners reserved, not leader or quorum proof.

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 go build -p 1 -o /tmp/quorumkv ./cmd/quorumkv
install -d -m 0700 .local .local/n1 .local/n2 .local/n3
/tmp/quorumkv node --config examples/n1.json --bootstrap
/tmp/quorumkv node --config examples/n2.json --bootstrap
/tmp/quorumkv node --config examples/n3.json --bootstrap
```

Bootstrap is only for the first empty-directory start. On restart run the same three commands WITHOUT --bootstrap; preserve node identities, ports and intact data directories. Do not delete data to resolve an error. SIGINT (Ctrl-C) or SIGTERM initiates shutdown; the process allows 5 s and returns failure on timeout, preserving files for recovery. Exit codes: 0 normal/help, 2 invalid arguments/config, 1 startup/runtime/shutdown failure. Config/data paths are relative to the working directory unless absolute. `.local/` and the root build binary are ignored by Git.

The M08 client is a single-attempt Go test client, not quorumkvctl or a persistent session CLI. Optional local inspection with an existing curl: query /v1/status on the three configured ports, choose the one currently reporting leader, then issue a single PUT/DELETE to that port. These requests assume n1 currently leads; adjust only to another configured member when necessary.

```sh
curl --max-time 3 http://127.0.0.1:7401/v1/status
curl --max-time 3 -H 'Content-Type: application/json' -d '{"key":"demo","value_b64":"","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1}' http://127.0.0.1:7401/v1/kv/put
curl --max-time 3 -H 'Content-Type: application/json' -d '{"key":"demo","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":2}' http://127.0.0.1:7401/v1/kv/delete
```

A follower returns not_admitted; never follow arbitrary hints. Timeout/lost response is unknown and may have taken effect. Do not automatically retry: deduplication/original-result recovery is M11. CAS HTTP, snapshots/compaction and production deployment are absent; logged GET is available below. Stop every node when finished. [M09 test/report](milestone-m09.md) distinguishes process-crash evidence from unverified OS/power loss. Historical M01–M07 library notes below describe their original delivery boundaries.

## Future workflow

Follow the [roadmap](implementation-roadmap.md), keep main simple and packages aligned with [architecture](architecture.md). Properties and model first, then adapters and measurements. Every semantic change updates both languages, requirement, ADR, fixture and criterion. The original design delivery authorized no automatic implementation; later explicit assignments authorized M01/M02. Further milestones require their own scope and applicable gates.

For the existing M01/M02 module: `gofmt`, `go vet ./...` and `go test ./...` are checks performed on both local packages. Targeted applier race tests have passed for M02; integration tests remain proposed for later milestones within established limits. Pre-acquire toolchain/module caches; verify offline operation by disabling external networking without blocking loopback. No container is needed.

## Data, cleanup and troubleshooting

Keep `.local/` out of future version control; use synthetic data only. Manual backups do not replace replication or recovery; copying an active generation without coordination does not automatically produce a consistent backup. Clean only after stopping and verifying the path, never delete a directory to “fix” CRC or lock errors without preserving evidence.

| Symptom | Check and action |
|---|---|
| Port occupied | Identify owning PID; choose coherent ports across all configs or stop only your own process. |
| No leader | Check peer/config_hash and election logs; await a stable majority, never assign the role manually. |
| Timeout | Preserve unknown; no automatic retry until M11. Check peers and storage, never assume rollback. |
| Lock occupied | Check for an active process; never bypass exclusion. |
| Invalid CRC/manifest | Quarantine and diagnose; no automatic reset. |
| Storage cap | Stop new writes; compaction is not implemented before M13. Preserve active WAL and diagnose. |
| Mac under pressure | Stop run-owned tests/processes; reduce dataset/concurrency and retain measurements. |

## Checks now available for M01/M02

go.mod, internal/statemachine and internal/applier now exist; Go 1.27.4 darwin/arm64 was actually used. With that toolchain installed, go test -timeout=30s ./... and go vet ./... run from the root without services or external dependencies. This session used a temporary /tmp copy with a verified official checksum, not a global installation: [details and reproduction](milestone-m01.md). Node CLI/configuration examples remain proposals. M02 is completed; run go test -race -p 1 -count=1 -timeout=30s ./internal/applier with CGO_ENABLED=1 and the existing C compiler. See [M02 commands and results](milestone-m02.md).

## M04 configuration and local verification

The configuration schema is now implemented in [config.Parse](configuration.md), with canonical fixtures and bounded loopback tests. Node Start is a library, not the future qkv-node CLI; client listeners/commands above remain proposals. Reproduce M04 with the [report commands](milestone-m04.md), offline caches and temporary OS-assigned loopback ports. Loopback access must be allowed by the local execution policy; external access is unnecessary. M01–M04 are completed locally, M05 not started.

## M05 library integration and reproduction

node.StartElection(config,bootstrap) validates storage, binds the peer endpoint and runs election workers; ElectionStatus returns detached metadata. Call Shutdown with a bounded context and retain ownership until it actually completes; a canceled wait can be repeated. No qkv-node/CLI or client commands are implemented. [M05 report](milestone-m05.md) records offline test/build/race commands and [saved 20-seed traces](../fixtures/election-v1-results.json); ordinary tests verify the fixture without regenerating it. Custom clocks start from their supplied current milliseconds and transfer lifetime to Service; simulator restart supplies its current virtual instant, production uses a monotonic local clock. M06 remains not started.

## M06 library entry points

Use node.StartReplication(config,bootstrap) for M06. Existing StartElection remains the M05-only harness; there is no feature negotiation. node.AppendLocal(ctx,kind,command) gives only local Sync metadata, with an unknown local outcome if the caller cancels after admission; no dedup/client commit guarantee. Service Options.BatchSize accepts 1..16 before events. Inspect copied ElectionStatus.Progress or recovered log metadata; never apply directly. [M06 commands and evidence](milestone-m06.md) include replayable fixtures, local restart and targeted race. No node CLI/client API yet.

## M07 library usage and limits

node.StartCommitted(config,bootstrap) enables commit/application and fresh prefix replay. AppendLocal still returns only local durability; WaitApplied(ctx,receipt) returns an internal copied indexed/termed domain result, potentially ErrSuperseded/ErrResultExpired or a canceled/failed wait. No public server CLI/API yet. M05/M06 startup helpers retain legacy behavior. Service Options.ApplyCapacity is 1..64; MachineFactory is a trusted fresh exclusive dependency, production uses M01. [M07 commands/results](milestone-m07.md) record bounded offline checks and three replayed commit traces. Client/OpenAPI is M08, read/dedup/snapshot gates later.

## M10 GET operation and limits

The same executable/examples now enable logged GET on all three nodes. Use the same M10 binary everywhere; there is no rolling capability negotiation and older commit-only runtimes reject read-bearing logs. Choose a configured member currently reporting leader; the following single attempt assumes n1. Missing key is 404/completed with exists=false; a present empty value is 200 with exists=true/value_b64="".

```sh
curl --max-time 3 -H 'Content-Type: application/json' -d '{"key":"demo","timeout_ms":2000}' http://127.0.0.1:7401/v1/kv/get
```

Use POST with key/optional timeout only, not client identity/sequence. Each call appends/replicates a new read entry and writes WAL/checkpoint; do not poll KV continuously. Status is only local diagnostics. Timeout/lost response is unknown; a manually repeated GET is a fresh later read and may observe another value. The Go test client now has Client.Get and never automatically retries. PUT/DELETE limits and uncertainty remain unchanged. CAS HTTP, sessions/dedup and compaction remain unavailable. Stop the three node processes when finished; [M10 report](milestone-m10.md).
