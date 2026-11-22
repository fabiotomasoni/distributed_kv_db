# Architecture and concurrency

[Index](README.md) · [Italiano](../it/architecture.md)

**Updated: 20 November 2026 — M01–M07 local components/elections implemented and checked; internal commit/application implemented; client API remains design work.**

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

## Actual M01/M02 boundary

The implemented packages are internal/statemachine (unchanged sequential M01 core) and internal/applier (M02 scheduler). The latter accepts Entry{Index,Command}, returns a Future with Outcome{Index,Result}, and owns the only Apply caller. Concurrent producers supply contiguous indices; no reorder buffer. Scheduled advances on admission, LastApplied on completion; the caller must establish commit. The queue holds 64 pending entries by default, configurable 1..1024, plus one active call. Submit waits for space; TrySubmit returns ErrFull without advancing the cursor.

Shutdown closes admission and drains; cancellation only ends a caller’s wait after acceptance. Futures publish without requiring a receiver. Panics fail closed with explicit uncertain/current and aborted/pending outcomes. Full ownership, memory bounds and lifecycle contracts: [M02 report](milestone-m02.md). Snapshot epochs, sessions, commit decisions and all other components remain design work. The future 64-completion adapter is not implemented as a channel here; futures avoid blocking the worker on recipient availability.

## Implemented M03 storage boundary

[internal/wal and evidence](milestone-m03.md) add a single synchronized local storage owner above storageformat/storagefs. Append returns a generation/physical-sequence token only after Sync; Open reconstructs validated log metadata and repairs only an eligible incomplete final tail. No dependency on statemachine/applier and no alternate apply path. A future core must establish commit and dispatch blocking storage outside its event lock. Snapshot rotation, sessions, networking and consensus remain planned.

## M04 component boundary

Implemented [interfaces and ownership](milestone-m04.md): node → config/transport/wal, transport → peerprotocol/config, peerprotocol → config/strictjson/storageformat. Immutable configuration and owned message bytes; storage identity and LOCK precede bind, canceled/drained peer work precedes WAL close. No dependency on statemachine/applier from peer handlers; no recovered entry is automatically applied. Future Raft core owns term/vote/log decisions and schedules blocking storage outside its critical section.

## M05 core and adapter ownership

[M05 interfaces](milestone-m05.md) add internal/raft: pure single-owner event core, explicit persistence completion, copied Status. Service serializes events and performs the one synchronous WAL operation as the disk adapter outside a core/global mutex; core advancement pauses with bounded peer queues. One owner plus two fixed network workers, 256 queued peer events, two results, one job/active call per peer. Atomic snapshots allow status access during disk waits. Shutdown cancels admission/network, waits for disk and workers/handlers, then node releases storage. StartElection adds this integration while Start retains M04 injection. No applier or client path is connected.

## M06 replication ownership

[M06](milestone-m06.md) extends the same core/service without another worker pool. Owned canonical entries, copied fixed progress array, one registered request per peer; queue bounds remain 256 events and two network workers/results. Node.StartReplication enables batch=16, service options allow 1..16 at construction only. AppendLocal returns a local durable position, not a client commit. WAL AppendProposal preserves reserve for local admission, Append serves replication/control. No applier or commit advancement is connected.

## M07 application ownership and bounds

[M07](milestone-m07.md) adds the committed projection bridge, one unchanged M02 worker and one FIFO future collector to the existing owner/two peer workers. No worker per entry. Pending M02 queue 1..64, tickets/completions/outstanding futures <=64, copied recent results 64. The owner performs TrySubmit, preserves unscheduled committed work on ErrFull and drains completion events while networking/ticks continue. Normal shutdown drains durable committed work before releasing storage; fatal admission closes before waiting for an active application. Fresh replay precedes binding. No direct M01 calls from core/HTTP or new M02 production interface.

## M08 client adapter

[M08](milestone-m08.md) adds internal/clientapi and Service.Write. Two standard-library HTTP servers are necessary for the distinct client/peer ports already fixed by M04 configuration; they share M04's accepted-connection limiter. No duplicate Raft transport/protocol is introduced. Typed PUT/DELETE projection enters a bounded queue separate from peer RPC events; the existing sole Raft owner performs storage effects. Caller goroutines wait on buffered receipts, actual M02 completion and a serialized final role/term fence; no per-write delivery worker is created. StartAPI constructs both immutable endpoints before starting background work. Shutdown rejects/cancels HTTP waits, joins handlers, drains committed Raft/application work, closes peer I/O and only then releases WAL/LOCK. Bind failure releases already acquired listeners and storage. CLI/process recovery MVP remains M09.

## M09 process owner and recovery MVP

[M09](milestone-m09.md) adds only cmd/quorumkv → internal/server → node.StartAPI. Signals cancel a bounded process wait; Node.Done reports completed automatic teardown. The unchanged WAL validates authoritative CURRENT, identity, frames and commit; the unchanged service reconstructs a fresh M01 machine through M02 over exactly the checkpoint prefix before listeners. No persisted applied cursor, alternate application path, second recovery engine or new RPC/API. A suffix remains speculative until legal new-term quorum commit. The process layer reports fatal/timeout exits instead of claiming a healthy running node. Test-only inspection accesses a replayed map only after its worker has terminated.

## M10 logged-read capability

[M10](milestone-m10.md) extends the existing client adapter and shared bounded service admission/completion path. Service.Read creates read entries; committed.ProjectWithReads maps them to M01 Get inside the unchanged M02 worker. The exact captured value is copied into the existing 64-result ring and copied on delivery. At most 64 retained values of at most 16 KiB add a bounded payload budget of 1 MiB, excluding ordinary object/queue overhead. No unbounded waiter table or per-read worker.

Options.Reads requires Commit and enables Core.EnableLoggedReads before EnableCommit/replay/events. Production StartAPI/CLI enables it; legacy StartCommitted/commit-only tests retain read rejection. It is a homogeneous startup capability, not rolling feature negotiation. All members must run the same M10 runtime; older modes fail closed on read-bearing recovery. Read acceptance in leader/follower and fresh replay uses the existing v1 schema; no WAL/RPC format change. Status, write endpoints and shutdown resource ownership are unchanged.

## M11 M02-owned replicated session wrapper

[M11](milestone-m11.md) adds committed.Sessions around the fresh M01 machine, inside the existing sole M02 worker. A private bounded command envelope carries the approved canonical entry bytes, including identity/sequence/index, through M02's unchanged 48 KiB aggregate copy boundary; the maximum legal CAS fits. The wrapper alone owns map/session mutation and returns the approved compact result/original index through a private bounded result envelope. M01/M02 production interfaces remain unchanged; HTTP never accesses the session map.

The service unwraps the result into its existing current-index/current-term result ring, with a parallel 64-element original-index array. LastApplied and completion fences use the actual newly committed attempt; HTTP uses the cached logical result index. No per-request goroutine/cache, second persistent session store or Raft commit change. The 1024 latest records are reconstructed with the map by fresh committed-prefix replay. Versioned configuration/identity and HTTP profile acknowledgment prevent silently reinterpreting legacy operation semantics. General snapshots/session export, metrics and reusable fault infrastructure remain later milestones.

## M12 observation boundary

[M12](milestone-m12.md) adds local diagnostics and an explicit simulator separate from the server. Observations/logs never authorize commit, apply or client responses and do not change the algorithms. V2 sessions, exact-slot GET, atomic CAS and fresh-quorum retry remain unchanged; no v1 migration. Simulation uses actual core/M02/WAL codecs, a permanent committed ghost and per-index result comparison; replay checks every event against saved choices. ProcessCrash retains completed unsynchronized writes; SystemCrashModel explores all/synced survival only in memory. Incomplete traces are inconclusive. No actual OS/power crash, snapshots or compaction; M13/M14 gates retained. Operational specifications are in [faults](fault-injection.md) and [diagnostics](observability.md).
