# Technical demo and evaluation

[Index](README.md) · [Italiano](../it/demo-and-evaluation.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Common preparation

Future demo, runnable only after M14 and results review. No experiment has been performed. Hardware and caps follow [resources](performance-and-resources.md); 3 loopback nodes follow [networking](network-protocol.md), at most 2 clients and 100 operations per experiment, maximum 30 s each. Total session <=10 minutes including explanations, without continuous load. Synthetic data only, external connectivity disabled and dependencies already available.

Save a run manifest with revision, configuration/hash, toolchain, seed, directories and registered PIDs. Every experiment retains role/term/commit/apply logs and request_id/client_id/sequence history. Measure latency, errors/unknown, election where present, recovery where relevant, RSS/CPU and peak space. Never attribute a measurement to another phase.

| Experiment | Preparation/configuration | Actions | Expected result and success | Invariants | Tests |
|---|---|---|---|---|---|
| D-01 — Three-node cluster | Empty directories with common manifest, seed 7. | Bootstrap three followers; await election and committed noop. | One leader in the term; status from three members. | INV-01 | T-001/T-004 |
| D-02 — Write and replication | D-01 completed, one new session. | PUT config/theme then GET; await equal follower apply. | Expected value and identical digest at equal index. | INV-02/04/06 | T-008/T-009 |
| D-03 — Follower crash | D-02 completed; identify a follower. | Stop follower, PUT on majority, then restart it and catch up. | Write succeeds and follower catches up. | INV-03/04 | T-024 |
| D-04 — Leader crash | D-03 healed; three live nodes again. | Stop leader; attempt a request and record timeout. | No false success from old leader; pending outcome uncertain. | INV-06 | T-012/T-024 |
| D-05 — New election | D-04, two healthy reachable followers. | Await new leader and noop, write, then restart old leader. | Term rises, writes resume within 3 s target after stabilization. | INV-01/03 | T-004/T-024 |
| D-06 — Network partition | Three live after D-05, fault adapter enabled. | Isolate leader from two peers and drop pending messages; then invoke GET on old leader, PUT+GET on pair, finally heal. | Isolated side cannot complete GET; majority continues and logs converge. | INV-02/03/04 | T-019/T-021 |
| D-07 — Concurrent CAS | Key stock/pen=12, two different client IDs. | Concurrent CAS expected=12 to 11 and 10; identical winner retry. | Exactly one changes the value; retry returns the same original result. | INV-04 | T-010/T-011 |
| D-08 — Restart and snapshot | Known successes, published snapshot and existing sessions. | Crash all three processes, restart same directories; GET and retry latest sequence. | Acknowledged data retained, no double apply; distinguish replay from catch-up. | INV-04/06 | T-014/T-015/T-023 |
| D-09 — Linearizable history | Checker validated against legal and impossible fixtures. | Record <=12 operations with CAS, GET, timeout and leader change; check. | Checker produces a legal witness, not timeout/inconclusive. | INV-04 | T-022 |

## Per-experiment logs, metrics and cleanup

D-01 retains votes/terms and time to first noop; D-02 indices and replica hashes, latency and Sync; D-03/D-04/D-05 PIDs/exits, quorum and resumption time; D-06 partition rules and heal timeline, outcomes per side; D-07 CAS comparison and dedup_hits, mutation count; D-08 CURRENT/generation and indices, replay/catch-up; D-09 full history and witness, checker duration.

Each experiment's cleanup: remove fault rules, restart only run-stopped nodes, await catch-up or mark the scenario failed and stop the run. D-03 must restart its follower before D-04 or quorum would be lost. D-06 removes obsolete staging after verified heal; D-07 closes client-side sessions without evicting replicated metadata; D-08 retains directories for inspection; D-09 closes the checker and saves its outcome. At session end stop all run-owned PIDs, close files, archive evidence within 10 MiB and delete only explicitly selected synthetic data.

## Interview presentation

Open with purpose, actual status and the limits of three processes on one host. Show a write, failover and a counterexample rejected by the checker, then a valid history. Discuss the current-term rule, voting before acknowledgment, read barrier, uncertain timeout, dedup in snapshots and generation publication. Present measurements only with their experimental profile. If a demo fails, use logs and seeds to explain it; never replace the outcome with invented screenshots or numbers.
