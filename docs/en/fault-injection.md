# Reproducible fault injection

[Index](README.md) · [Italiano](../it/fault-injection.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Adapters and authority

The future framework replaces clock, PRNG, transport and storage while keeping the core unchanged. Single-threaded simulation uses deterministic priority `(virtual_time,event_sequence)`; each send, delivery, disk completion and election becomes an event. No sleeps inside the core. A second harness orchestrates three real processes: it covers sockets and filesystem without claiming identical scheduling.

Supported faults: drop, delay, duplicate, reorder, directional or bidirectional pair isolation, process crash, restart with the same directory, short write, ENOSPC and failed Sync. “Force leadership” means isolating/crashing the leader or advancing timers, never changing role or term around the algorithm.

## Proposed scenario format

Illustrative example, no runner exists:

```json
{"scenario_version":1,"scenario_id":"partition-old-leader","seed":7,"virtual_deadline_ms":5000,"max_events":10000,"events":[{"at_ms":1200,"action":"isolate","node":"n1","direction":"both"},{"at_ms":3000,"action":"heal","node":"n1"}]}
```

The fixture establishes leadership in a deterministic preliminary phase; if n1 is not leader, setup fails rather than testing another case. Drop/delay selects RPC type, from/to, rpc_id or ordinal number; the trace records each random decision. Duplicates have distinct event_ids and the same payload; reordering uses explicit sequence. `heal` removes future rules; already-scheduled messages follow the scenario's declared policy, default delivery at their original due time. Crash discards RAM and messages addressed to the dead instance; restart increments incarnation and never reuses old waiters.

## Storage and controlled crashes

The adapter distinguishes bytes submitted to the kernel, synchronized bytes and visible namespace; it explicitly selects the following crash model. Named cutpoints: `wal.header`, `wal.payload`, `wal.sync`, `commit.sync`, `snapshot.sync`, `generation.rename`, `current.rename`, `directory.sync`, `cleanup`. Simulate ENOSPC with quotas rather than filling the real SSD. Never corrupt user or active directories: copy fixtures into test-owned temporary directories.

Simulation may explore adversarial filesystem reorderings as hypotheses; it does not prove actual APFS behavior. Real-process tests signal only PIDs registered by that run; never terminate by generic process name. Retain exit codes and lock state.

## Manifest and replay

Save scenario, seed, PRNG/version, configuration and hash, harness/core version, ordered event list, fault decisions, cutpoint, logs and client history. Simulator replay makes no new random choices: consume the saved trace and compare digests after every event. Stop at the first divergence. Minimize a failure by removing events while preserving preconditions and the violation.

Limits: 5 s virtual time, 10,000 events/scenario; 30 s real time per integration case, maximum 120 s ordinary suite. At completion cancel timers, close connections/files, terminate only run-owned processes and verify free ports. Retain failures within 10 MiB under manual retention; clean successful runs. Fault injection is not enabled through the normal HTTP endpoint: it requires an explicit test build/profile and configuration, all on loopback.

## Usefulness criterion

A useful fault causes an observable transition and checks a property: drop before acknowledgment for INV-06, lost response for dedup, isolation for GET, snapshot cutpoint for recoverable generations. [F-01–16](failure-model.md) and [T-020/T-028](testing-strategy.md) define coverage and replay; high load never replaces these experiments.

## Separate crash models and I/O survival

**ProcessCrash**: the kernel and filesystem cache stay alive. Retain completed writes and renames even without Sync; an in-progress write may leave no bytes, a prefix or all bytes. No bytes acknowledged as synchronized may disappear within the model. Test at least crash before write, after write/before Sync and after Sync, including complete unsynchronized frames, a partial final frame and renamed CURRENT whose directory has not yet been synchronized. Recovery accepts a surviving complete valid frame but never applies entries merely because they exist.

**SystemCrashModel**: separate abstract exploration, not a hardware promise; preserve synchronized dependencies under the modeled contract and vary survival of unsynchronized effects, including namespace operations. “Last Sync only” is one possible case, not the only one. Never attribute these outcomes to an actual macOS crash or power loss.

T-013 and T-016 cover both logical image models within finite cutpoints; T-023 checks real process crashes only. Record model_kind, survival choice and I/O order in the replay manifest. Installing a corrupt image remains separate from an incomplete crash. The network kernel is not an oracle: include responses delayed across restart and never use simulator incarnation to hide messages from the core that the real protocol could receive.
