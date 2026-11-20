# Future development guide

[Index](README.md) · [Italiano](../it/development-guide.md)

**Updated: 20 November 2026 — M01–M06 local components/elections implemented and checked; distributed commit remains design work.**

## Free prerequisites

MacBook Pro M5/macOS ARM64, terminal, Git, an existing editor and Go darwin/arm64. M01 verified baseline: **Go 1.27.4 darwin/arm64**; go.mod requires at least Go 1.27.4 and the [M01 report](milestone-m01.md) records the exact tested toolchain. The [official release history](https://go.dev/doc/devel/release) remains the upgrade reference. No beta versions or automatic downloads during offline tests; `GOTOOLCHAIN=local` prevents implicit fetching. Apply security patches after review and the suite; plan minor upgrades rather than changing mid-experiment.

Go and the standard library suffice for runtime; the toolchain has a [BSD license](https://go.dev/LICENSE). Git is a development tool, not a runtime dependency. No third-party Go packages are initially proposed: no generators, CGO, external DBs, packaged Raft or telemetry services. GitHub Free is optional. A Mermaid renderer is optional for viewing diagrams; source remains available offline. QuorumKV's license must be chosen by its owner before publication.

Every new dependency requires an ADR: concrete need, standard-library alternative, pinned version, license, maintainer activity, known vulnerabilities, ARM64 compatibility without extra toolchains, RAM/disk costs and offline acquisition/cache procedure. Never add a package merely because production projects commonly use it.

## Validated configuration schema

No node binary is included. This n1 example conforms to the implemented schema; n2/n3 change node_id, data_dir and bind addresses according to [networking](network-protocol.md). Shared fields remain identical. config_hash derives from cluster_id, the member list sorted by ID and replicated quotas; it excludes local data_dir and binds. M04 fixes the exact canonical encoding in the shared fixture: [contract](configuration.md).

```json
{"config_version":1,"cluster_id":"11111111111111111111111111111111","node_id":"n1","client_addr":"127.0.0.1:7401","peer_addr":"127.0.0.1:7501","data_dir":".local/n1","members":[{"id":"n1","client":"127.0.0.1:7401","peer":"127.0.0.1:7501"},{"id":"n2","client":"127.0.0.1:7402","peer":"127.0.0.1:7502"},{"id":"n3","client":"127.0.0.1:7403","peer":"127.0.0.1:7503"}],"max_logical_bytes":4194304,"max_keys":10000,"max_sessions":1024,"heartbeat_ms":100,"election_min_ms":600,"election_max_ms":1000}
```

Distinct free ports, immutable IDs, same protocol version. Future first startup requires explicit `--bootstrap` on an empty directory and a common manifest; ordinary startup never accidentally reinitializes empty directories. Bootstrap does not appoint a special leader: everyone starts as follower and follows Raft.

## Planned CLI interface, not executable today

```text
quorumkv node --config examples/n1.json --bootstrap
quorumkv node --config examples/n2.json --bootstrap
quorumkv node --config examples/n3.json --bootstrap
quorumkvctl --endpoints 127.0.0.1:7401,127.0.0.1:7402,127.0.0.1:7403 put config/theme dark
```

These names and paths are design examples, not existing files or commands. Omit bootstrap on restart. The CLI must persist its session and pending payload privately before requesting; never regenerate IDs on retry. To stop, send SIGTERM to the run-owned PID; deadline 5 s, after which the harness records forced termination and retains data for recovery.

## Future workflow

Follow the [roadmap](implementation-roadmap.md), keep main simple and packages aligned with [architecture](architecture.md). Properties and model first, then adapters and measurements. Every semantic change updates both languages, requirement, ADR, fixture and criterion. The original design delivery authorized no automatic implementation; later explicit assignments authorized M01/M02. Further milestones require their own scope and applicable gates.

For the existing M01/M02 module: `gofmt`, `go vet ./...` and `go test ./...` are checks performed on both local packages. Targeted applier race tests have passed for M02; integration tests remain proposed for later milestones within established limits. Pre-acquire toolchain/module caches; verify offline operation by disabling external networking without blocking loopback. No container is needed.

## Data, cleanup and troubleshooting

Keep `.local/` out of future version control; use synthetic data only. Manual backups do not replace replication or recovery; copying an active generation without coordination does not automatically produce a consistent backup. Clean only after stopping and verifying the path, never delete a directory to “fix” CRC or lock errors without preserving evidence.

| Symptom | Check and action |
|---|---|
| Port occupied | Identify owning PID; choose coherent ports across all configs or stop only your own process. |
| No leader | Check peer/config_hash and election logs; await a stable majority, never assign the role manually. |
| Timeout | Inspect outcome and retry the same ID; check Sync and peers, never assume rollback. |
| Lock occupied | Check for an active process; never bypass exclusion. |
| Invalid CRC/manifest | Quarantine and diagnose; no automatic reset. |
| Storage cap | Inspect compaction and inactive temporaries; never remove active WAL. |
| Mac under pressure | Stop run-owned tests/processes; reduce dataset/concurrency and retain measurements. |

## Checks now available for M01/M02

go.mod, internal/statemachine and internal/applier now exist; Go 1.27.4 darwin/arm64 was actually used. With that toolchain installed, go test -timeout=30s ./... and go vet ./... run from the root without services or external dependencies. This session used a temporary /tmp copy with a verified official checksum, not a global installation: [details and reproduction](milestone-m01.md). Node CLI/configuration examples remain proposals. M02 is completed; run go test -race -p 1 -count=1 -timeout=30s ./internal/applier with CGO_ENABLED=1 and the existing C compiler. See [M02 commands and results](milestone-m02.md).

## M04 configuration and local verification

The configuration schema is now implemented in [config.Parse](configuration.md), with canonical fixtures and bounded loopback tests. Node Start is a library, not the future qkv-node CLI; client listeners/commands above remain proposals. Reproduce M04 with the [report commands](milestone-m04.md), offline caches and temporary OS-assigned loopback ports. Loopback access must be allowed by the local execution policy; external access is unnecessary. M01–M04 are completed locally, M05 not started.

## M05 library integration and reproduction

node.StartElection(config,bootstrap) validates storage, binds the peer endpoint and runs election workers; ElectionStatus returns detached metadata. Call Shutdown with a bounded context and retain ownership until it actually completes; a canceled wait can be repeated. No qkv-node/CLI or client commands are implemented. [M05 report](milestone-m05.md) records offline test/build/race commands and [saved 20-seed traces](../fixtures/election-v1-results.json); ordinary tests verify the fixture without regenerating it. Custom clocks start from their supplied current milliseconds and transfer lifetime to Service; simulator restart supplies its current virtual instant, production uses a monotonic local clock. M06 remains not started.

## M06 library entry points

Use node.StartReplication(config,bootstrap) for M06. Existing StartElection remains the M05-only harness; there is no feature negotiation. node.AppendLocal(ctx,kind,command) gives only local Sync metadata, with an unknown local outcome if the caller cancels after admission; no dedup/client commit guarantee. Service Options.BatchSize accepts 1..16 before events. Inspect copied ElectionStatus.Progress or recovered log metadata; never apply directly. [M06 commands and evidence](milestone-m06.md) include replayable fixtures, local restart and targeted race. No node CLI/client API yet.
