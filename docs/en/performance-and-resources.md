# Performance and resource protection

[Index](README.md) · [Italiano](../it/performance-and-resources.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Initial profile, not results

MacBook Pro M5, 32 GB, macOS ARM64; three processes, no GPU, AI training, heavy infrastructure or continuous service. The figures below are design limits or targets to measure. No consumption or throughput has been measured. MB is decimal, MiB = 1,048,576 bytes: “roughly 100 MB” is an approximate operational target, not equality with 100 MiB.

| Resource | Initial limit / target | Future enforcement / verification |
|---|---|---|
| Logical dataset | 4 MiB and 10,000 keys per copy; 12 MiB across copies | Deterministic apply quota, T-026. |
| Value / key | 16 KiB / 256 bytes | Parser and model, T-009. |
| Sessions | 1,024; latest result <=512 bytes | No eviction, included in snapshots. |
| Physical WAL | 8 MiB/node, internal 512 KiB reserve | Admission/compaction, conservative storage stop. |
| Snapshot | 8 MiB/file | Length validated before allocation. |
| Data directory | 32 MiB/node including temporary/generation peak | Reserve before snapshot/install. |
| Ordinary cluster data | Target <=70 MiB (~73 MB); peak cap 96 MiB (~101 MB) | Sum all three directories, not one copy. |
| Operational logs | 1 MiB/node; 3 MiB cluster | Inclusive rotation, no continuous debug. |
| Active data+logs | Cap 99 MiB (~104 MB) | Stop before exceeding; ordinary target roughly 100 MB. |
| Test evidence | 10 MiB separate from active data | Retain selected failures only; manual retention. |
| Cluster RAM | Total RSS target <1,000 MB | Measure through macOS, not a runtime guarantee. |
| Go heap | Soft target 192 MiB/node | Future GOMEMLIMIT, neither an RSS cap nor OOM protection. |
| CPU | Mostly idle at rest; initial GOMAXPROCS=2 per node | Blocking timers, no busy polling. |
| Ordinary clients | At most 8, 1,000 total operations/run | Harness stops admission at the cap. |
| Benchmark | 2 s warm-up +10 s measurement, one initial repetition | 15 s deadline; optional, never continuous. |
| Ordinary suite | Maximum 120 s, integration <=30 s/case | Mandatory timeouts and cleanup. |

The 32 MiB data cap includes **all** temporary and control files. Four 8 MiB files would already exhaust the cap, leaving no room for IDENTITY, CURRENT, lock and manifests: that is not an admissible peak. Never install a snapshot concurrently. Limits favor rejecting work over exhausting disk, but filesystem overhead and other programs remain outside application accounting. Missing physical space causes fail-closed behavior.

## Controlled measurement

Use a synthetic dataset of 1,000 keys with 128-byte values, preloaded within quotas; separate GET-only, PUT-only and 80% GET/20% PUT workloads. GET incurs WAL in the initial design: state this in the report. Concurrency 1, then 4, then 8 only if earlier runs respect limits. Never automatically search for saturation; stop on memory pressure, growing swap, thermal pressure or excess space.

Throughput = completed operations inside the window / monotonic seconds; distinguish HTTP attempts from deduplicated logical operations. End-to-end latency spans first invocation to response, including retries; separately report errors, unknown outcomes and success-only latency to avoid hiding timeouts. Mean uses observed samples; p95/p99 use nearest-rank on sorted samples, stating n and the limited reliability of p99 with small samples. Never interpolate missing numbers or promise SLOs before a baseline.

Measure election as injected loss→committed noop; initial local target <=3 s after stabilization. Measure process recovery as start→completed replay; report catch-up and first client response separately. Record WAL/snapshot sizes, entry count and cache conditions. Never flush system caches or force reboot to make results “clean.”

## Monitoring on macOS

Activity Monitor: select all three processes, inspect CPU, Memory, Energy and bytes written in Disk; compare with a stopped-cluster baseline. `top -l 2 -pid <PID>` and `ps -o pid,rss,%cpu,etime -p <PID>` are observation examples to use with real run-owned PIDs, not commands already applied. macOS ps RSS is in KiB; sum the three processes without treating shared-memory accounting as perfect.

`du -sk .local/n1 .local/n2 .local/n3` is a future allocated-space example; compare it with application logical file counts because APFS allocation may differ. Avoid privileged tools and high-frequency sampling. Use local runtime metrics for goroutines/heap; optional profiling on loopback only during a short session, normally disabled.

## Report and cleanup

Each future report includes date, revision, Go/macOS, hardware, power state, configuration, dataset, warm-up, duration, seed, clients, n, errors, throughput, mean/p95/p99, peak/idle RSS/CPU, before/peak/after disk, election/recovery and limitations. Current measurement table: **Not measured** for every metric.

At session end stop processes, verify PIDs and locks, retain essential manifests/evidence; remove only explicitly recognized run directories. Never automatically clean active data or files outside the project. Prefer threshold compaction and short workloads: do not rerun benchmarks without a new engineering question.

## Peak reservation and quota pressure

Before rotation or installation check `current_file_bytes + expected_temporary_growth + control_reserve <= 32 MiB`. Initial control reserve is **64 KiB**, inside the cap; use a larger estimate if control files need more space. Never allocate a second full staging copy if the received file can be promoted into the new generation. Reservation includes appends admitted before the barrier and every still-referenced old generation.

If it does not fit, defer compaction and block new proposals before the cap without deleting a required prefix. Commit/catch-up may be needed to reduce the live suffix: there is no liveness guarantee under exhausted physical quota. The <=3 s resumption target also assumes available resources and no saturated quota. Measure APFS allocated space separately from logical bytes; application counters alone do not promise a perfect physical cap. T-026 simulates reservation refusal near the cap with control files present.
