# Consistency and linearizability

[Index](README.md) · [Italiano](../it/consistency-model.md)

**22 November 2026 — logged GET and PUT/DELETE baseline implemented; full future session/CAS/checker contract remains gated.**

## Observable contract

The required property is linearizability over one map: completed operations must admit a legal sequential ordering that respects real-time order. If A finishes before B is invoked, A precedes B; overlapping operations may appear in either order. “Strong consistency” alone is ambiguous: here it means only this contract. The formal reference is [Herlihy and Wing](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf).

PUT, DELETE and CAS logically linearize when their committed entry is applied. The log fixes order and the applier does not interleave commands. Failed CAS is a definitive result in that order and consumes the sequence. A deduplicated retry represents the same logical operation and original point; it is not another write. GET linearizes when the map is sampled while applying its barrier.

## Selected GET protocol: logged read

1. Validate the request and pass it to the local core only if currently leader; a follower returns NOT_LEADER. Never read directly from its local copy.
2. Create a new `read(key)` entry in the current term after invocation. The command does not change the map and uses no client session.
3. Persist and replicate exactly as for a write. Commit requires a durable majority and the current-term rule: leadership is therefore checked by a quorum for this entry, without using previous heartbeats or leases.
4. Wait for `lastApplied` to reach the exact entry. The applier evaluates the key in that slot and saves the result for the waiter before applying later mutations. Do not merely wait for a minimum index and then read a map being changed without synchronization.
5. Reply only with the captured result and a still-valid leader generation when delivering to the server. If a higher term has meanwhile been observed, return unknown or expire; this is conservative. An unobserved leadership loss does not invalidate a legally committed barrier.

This protocol prevents success for a GET **invoked after complete isolation**, when no message useful to its new round can reach a majority: there is no quorum for the new entry. It includes the requirement to commit a current-term entry, often the election noop; the read entry itself can also satisfy this requirement and carry the earlier prefix. It does not depend on clock accuracy.

## Effects and costs

Read-after-write holds when a write has a completed response and a later GET succeeds: it cannot see state earlier than that write, except for a subsequent legal modification. Leader changes do not weaken the guarantee if a recoverable majority remains. Missing-key observation is also ordered; 404 is a definitive result, not a network error.

Reads require networking, WAL and apply, increase space and SSD writes, and are unavailable on the minority. This is a deliberate trade-off for the bounded educational workload. ReadIndex may later reduce I/O, but requires an updated ADR, current-term commit, a correlated quorum round starting after the request and waiting for apply through the read index. It is outside the initial path and will not be enabled merely to improve benchmarks.

## Uncertain outcomes and availability

Timeout does not mean rollback. Pending operations may be completed or excluded in a legal extension of the history. Clients cannot change the ID of an uncertain mutation without risking another logical effect. No multi-key transactions, explicit stale reads or minority availability are promised. Correctness depends on retaining sessions as well as on Raft.

## Verification

[Testing](testing-strategy.md) defines histories with invocations, responses, identities and uncertain outcomes, an independent sequential model and a search over admissible orders. Targeted tests include an isolated old leader, acknowledged write before failover, delayed apply, concurrent CAS, and lost response with retry after snapshot. A finite test finding a legal order is evidence for that history, not a proof for every possible execution.

## Operations in flight when partition begins

A read committed before the partition may complete after isolation if its result and protocol checks remain valid; likewise an already-committed write may receive a delayed response. This is linearizable because its interval began before the fault. Never require revoking a past quorum. T-019 must drop relevant in-flight messages, establish isolation and only then invoke the GET that must fail; a second case permits a response from an already-committed barrier.

Linearization points belong to the ordered logical service, not a simultaneous physical point on every replica. Local apply may be delayed or repeated during recovery; a retry remains associated with its original effect. Failover may make a pending command observable through a replica applying it before the old leader, always in committed order.

## Reassessed comparison: log versus ReadIndex

| Aspect | A: logged GET, v1 baseline | B: ReadIndex, future extension |
|---|---|---|
| Correctness | Committed current-term read entry, sampled at its exact apply slot. | Known current-term entry commit, fresh correlated quorum round and apply through read index. |
| Leader change | Same commit rule as writes; no local read without a barrier. | Contexts bound to term/generation; abort on observed leadership loss before completion. |
| Quorum | Required for every new read entry. | Required for a read round; batching only for requests already invoked before the round starts. |
| Latency | Replication with persistence, commit checkpoint and apply; no measured figure. | Quorum round and apply wait, without per-GET WAL; not necessarily faster when apply lags. |
| Entries | One per admitted GET attempt. | Zero per GET, initial term noop amortized. |
| SSD | Entry and checkpoints on replicas, then compaction; measure actual write amplification. | No extra GET persistence in a stable term; normal Raft obligations remain. |
| Complexity | One shared ordered path; correct scheduling still required. | Separate readIndex/context state, acknowledgment correlation, apply wait and waiter/leadership handling. |
| Testing | Reuses commit/apply; adds exact-slot result and partition tests. | Adds obsolete acknowledgments, premature rounds, apply lag, batching and term changes. |
| MacBook | Plausible for bounded datasets/runs, not a wear or RAM measurement. | Less read-only I/O, with more implementation/review work. |

**Decision retained:** A for the first version because load is bounded and the initial objective is to verify one ordering path. Address the SSD constraint through short tests, no unnecessary rewrites and measurement, not invented performance. No local reads without quorum.

Future B path, only after M14/M15 and an ADR-006 update: (1) commit a current-term noop/entry; (2) on GET receipt capture R=commitIndex and create a unique context bound to term/generation; (3) start a new AppendEntries/heartbeat round carrying that context; (4) collect a correlated positive response from one distinct peer plus the leader in the current term; (5) revalidate role/generation and wait for lastApplied>=R; (6) have the applier capture the value under a coherent read barrier and complete the waiter. Never reuse acknowledgments received before the request or expose the map to handlers.

Timeout or higher term invalidates the context; late responses cannot complete newer rounds. The extension needs explicit RPC schema evolution for context, not silently accepting unknown v1 fields. Start with a new round per GET before adding batching. Test old leaders, delayed acknowledgments, uncommitted noop and lagging apply; require the same independent checker and error outcomes as the baseline. Then compare bytes written, Sync and latency under identical conditions without enabling an undocumented hybrid strategy.

Theoretical reference: [Raft, section 8](https://raft.github.io/raft.pdf). The operational sequence and gates above are the QuorumKV proposal, not a claim of existing implementation.

## Implemented M02 boundary

The [local applier](milestone-m02.md) serializes caller-indexed commands and returns results at their exact application slot. Concurrent creation/arrival does not define order: only the next contiguous index is admitted. This supplies an ordered local component, not proof of commit, quorum or distributed linearizability. Rejected admission consumes no index; canceled waits after admission cannot revoke an operation.

## M07 internal ordering evidence

[M07](milestone-m07.md) now implements legal current-term commit and exact ordered mutation/noop application. A failed CAS remains committed/applied with COMPARE_FAILED. LocalAppend is still only a local Sync receipt; WaitApplied is internal indexed/termed delivery with bounded retention, cancellation and supersession errors. This is not a public linearizability/exactly-once API: external HTTP/error/session behavior and logged GET barriers remain later phases. M07 rejects future read entries instead of enabling an early read path. The broad client contract above remains design work.

## M08 external write boundary

[M08](milestone-m08.md) now exposes PUT/DELETE results after durable legal commit, exact application and an owner-serialized role/term completion fence. The handler rechecks published progress before encoding. This does not make network delivery atomic with step-down, or revoke a legally committed operation on an unobserved loss. Cancellation can prove exclusion only before recording; thereafter unknown does not imply rollback. Completed writes respect committed log order; overlapping HTTP arrivals have no fixed order. A repeated identity is a new attempt until M11, so logical retry collapse/session claims above remain future design. There is no KV GET, CAS HTTP, ReadIndex or complete history checker. Tests cover small concurrent writes and DELETE after completed PUT across election, without claiming universal linearizability proof.

## M09 replay and uncertain suffix

[M09](milestone-m09.md) verifies fresh committed-prefix reconstruction before serving. No speculative suffix is exposed merely because its bytes survived. After election, a current-term noop may legally commit an inherited uncertain suffix; its previous unknown response is not evidence of rollback. Completed acknowledged entries survive tested process crashes with intact directories and a recoverable majority. Tests compare original entry bytes and the actual reconstructed machine against a small external acknowledged-history oracle. This does not add HTTP GET, safe retries, session identity collapse or a full linearizability checker.

## M10 implemented ordering and evidence

[M10](milestone-m10.md) implements the selected logged-read protocol. The fresh current-term entry is created after invocation, so a successful durable majority acknowledgment must cover that entry or a later legal current-term boundary. Pre-invocation heartbeats cannot establish that new prefix. Exact indexed/termed M02 output is retained and copied before later mutations; merely waiting for LastApplied and sampling a live map is never used. Missing-key and empty-value observations are distinguished.

The owner completion fence rejects observed role/term loss or insufficient commit/apply; a final HTTP snapshot is only conservative revalidation, not quorum proof. A pre-isolation committed read can complete after isolation, but a new isolated read cannot. Seeded three-node histories explicitly discard in-flight traffic before the old-leader invocation, retain a permanent committed ghost and compare read slots with an independent list/string model. A five-operation real HTTP history is searched over legal overlap orders and its checker rejects a known stale counterexample. These finite checks satisfy the M10 read gate; the general session/CAS/snapshot history checker remains M14. Repeated GET is a new logical read, without session collapse.
