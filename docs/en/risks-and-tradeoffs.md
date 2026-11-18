# Risks and trade-offs

[Index](README.md) · [Italiano](../it/risks-and-tradeoffs.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Choices and alternatives

| Comparison | Initial choice and reason | Accepted cost / review condition |
|---|---|---|
| Go / Rust / C++ | Go: standard-library concurrency/networking and a sustainable learning cycle. | GC and variable RSS; no performance superiority claim. |
| HTTP/JSON / binary RPC | HTTP/JSON for inspection without generators. | Parsing/Base64 and larger payloads; change only with measurements and a versioned schema. |
| RAM / WAL | In-memory map plus synchronous WAL and snapshots. | More storage complexity, but RAM-only cannot meet crash recovery. |
| Simplicity / performance | Event loop and one RPC per peer. | Limited throughput; no pipelining before safety verification. |
| Consistency / availability | Quorum for every operation, GET in the log. | Minority unavailable and more I/O; no implicit stale fallback. |
| Sync / async | Sync before acknowledgment. | SSD latency; async would change the durability contract. |
| Full replication / sharding | Three full copies, small dataset. | Triple space cost, compensated by one group. |
| Local processes / physical nodes | Three processes on the already-owned Mac. | Correlated computer failure; no physical HA claim. |
| Deterministic / random tests | Deterministic simulation plus bounded seeds and real processes. | Simulation may model reality incorrectly; integration remains necessary. |

## Risk register

Probability and impact are qualitative design estimates. The initial owner is the individual developer, who reviews the register at the indicated gate.

| ID | Risk | Probability / impact | Mitigation | Milestones |
|---|---|---|---|---|
| R-01 | Incorrect Raft implementation | High / critical | Small core, INV-01–06 after every event, review old-term cases. | M05–M07/M14 |
| R-02 | Acknowledgment before Sync | Medium / critical | Explicit persistence effects and cutpoints; fatal node on error. | M03/M09 |
| R-03 | Stale read from isolated leader | High / critical | GET in the log, partition and delayed-apply tests. | M10/M14 |
| R-04 | Lost or prematurely cleaned dedup | Medium / high | No TTL/eviction, sessions in snapshots, monotonic sequences. | M11/M13 |
| R-05 | Crash during snapshot | High / critical | Generations, atomic CURRENT, reserved peak space, cutpoints. | M13 |
| R-06 | Excessive load/SSD writes | Medium / medium | Quotas, short benchmarks, rotated logs, no stress loops. | M12/M15 |
| R-07 | Schedule delay | High / high | Small gates, remove optional work, extend 24 weeks if needed. | All |
| R-08 | Premature complexity | High / high | No dynamic membership, containers or unmeasured optimization. | M01–M16 |
| R-09 | Divergent translations | Medium / high | Paired files and identical IDs, review in the same change. | All |
| R-10 | Real filesystem differs from model | Medium / critical | macOS adapter tests, open power-loss guarantee, no unsupported proof claims. | M03/M13 |
| R-11 | Incorrect checker or combinatorial explosion | Medium / high | Independent model, known-impossible fixtures, <=12-op histories, explicit inconclusive. | M14 |
| R-12 | Session cap reached | Medium / medium | Correct client-ID reuse, documented cap, reset only through an explicit new cluster. | M11 |

## Managing uncertainty

[ADRs](adr/README.md) remain Proposed until their gates provide enough evidence. Accepting a risk does not turn a target into a guarantee. Changing a replicated limit requires uniform configuration and a compatibility plan; changing one node breaks determinism. If mitigation requires spending, choose local mitigation or reduce scope: the budget remains €0.
