# Architecture and concurrency

[Index](README.md) · [Italiano](../it/architecture.md)

**Updated: 19 November 2026 — local M01 implemented and checked; distributed specifications remain design work.**

## Component boundaries

The architecture will use an event-driven Raft core with a single owner of consensus state. HTTP, disk and observability are adapters: the core does not read real time, open sockets or access the filesystem. Explicit inputs (ticks, RPCs, proposals, storage completions) produce testable transitions and effects.

| Future component | Responsibility and design interface | State and dependencies |
|---|---|---|
| CLI | Validate options, read configuration, invoke API; no consensus logic. | Addresses and locally persisted client session. |
| Client API | Deadlines, leader contact, serialization; retries with the same identity. | One in-flight request per session. |
| Node server | Admission, shutdown and lifecycle; translate HTTP into events. | Bounded queues, depends on core and adapters. |
| Raft engine | `Step(event) -> effects`, election, replication and commit. | One event loop owns role, term, vote and indices. |
| Raft log | `(index,term)` lookup, append, suffix replacement, snapshot view. | Core-exclusive access; no independent commit decision. |
| State machine | `Apply(index,command) -> result`, map and deduplication. | One ordered applier; no I/O, time or randomness. |
| Storage engine | `Persist(batch) -> durable(generation_id,sequence)`; frame recovery. | One serial worker, no core mutation. |
| Transport | Send RPCs and deliver responses with request generation. | One worker per peer, at most one active RPC per peer. |
| Snapshot manager | Immutable capture, publication, installation and compaction. | Coordinated with core and applier; serialized with storage. |
| Configuration manager | Validate manifest, quotas, addresses and identities. | Immutable after startup. |
| Logging / metrics | Structured events and copies of statistics. | Bounded buffers; debug output never blocks consensus. |
| Fault injection | Decorate clock, network and storage with recorded events. | Explicitly enabled test profile only. |

## Ordering, synchronization and ownership

The core must not wait for disk or networking while holding a global mutex. A transition requiring persistence waits for `durable(generation_id,sequence)` before publishing dependent messages: positive vote, successful AppendEntries, counting the local replica. The first implementation may suspend core advancement during this effect, leaving handlers with bounded queues only; no response precedes storage completion. Events and buffers transfer ownership or use immutable copies.

The applier receives only contiguous committed intervals and returns `(index,result)`. The core advances `lastApplied` from completions, never from enqueueing. HTTP handlers cannot read the map directly: GET is an ordered read command. Snapshots capture map, sessions and metadata at one index under an applier barrier; serialization uses a copy, avoiding concurrent reads of Go maps.

Initial limits: 64 pending client proposals per node, 256 peer events, 64 apply completions, one active persistence operation, one active RPC per peer. Reserve capacity for consensus events; client saturation returns `OVERLOADED` rather than creating unlimited goroutines. Client disconnection removes only its waiter; an admitted proposal continues.

## Main paths

Write: validation → admission → entry → local WAL Sync → durable replication → commit → apply/dedup → response. GET uses a **current-term logged barrier** and reads in the applier at its exact index; it costs more than ReadIndex but reduces initial complexity. Full details are in [API](api-specification.md) and [consistency](consistency-model.md).

Shutdown: stop admission, cancel outbound RPCs, finish already-safe storage and apply effects, sync and close. After 5 s report incomplete shutdown; the next start uses recovery. Do not promise success for unapplied requests or change configuration during shutdown.

## Future organization, not created

`cmd/quorumkv` and `cmd/quorumkvctl`; `internal/{node,raft,raftlog,statemachine,storage,transport,snapshot,config,observability,faults}`; `tests/{simulation,integration,history}`. Minimal interfaces belong to consumers, rather than a generic framework. State-machine tests do not import HTTP; core tests do not depend on real disk.

## Diagrams

The [diagram index](diagrams/README.md) contains nine views: system, node, write, read, election, replication/commit, recovery, partition and snapshot. Arrows describe a design, not constructed components.

## Apply scheduling indices and epochs

The core retains `scheduledApplyIndex`, initialized to lastApplied after recovery, with `lastApplied <= scheduledApplyIndex <= commitIndex`. Enqueue only the unscheduled interval even when commit advances while the applier is slow; a full queue leaves the index unchanged. One producer, contiguous ranges and `(applyEpoch,index)` completions prevent duplicate apply. The core keeps draining completions/storage without blocking while trying to fill the apply queue.

InstallSnapshot requires a barrier: stop new scheduling, drain or coordinately invalidate old work, wait until the applier is no longer mutating the old map, then replace state and set `lastApplied=scheduledApplyIndex=S`, incrementing applyEpoch. Old-epoch completions never change indices, waiters or sessions. Leadership change alone does not reset these indices: the committed prefix remains valid. INV-05 for lastApplied holds during normal operation; recovery reconstructs from the checkpoint before the node becomes ready.

## Actual M01 boundary

The only implemented package is internal/statemachine, exposing New() and Apply(Command) Result: [report](milestone-m01.md). The future applier Apply(index,command) contract will wrap this core; index, commit, sessions, epochs and scheduling were not implemented early. The core receives calls already ordered by its owner, without thread safety. All other components/arrows remain design work.
