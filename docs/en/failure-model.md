# Failure model and scenarios

[Index](README.md) · [Italiano](../it/failure-model.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Model

The model covers non-Byzantine crash-stop and crash-recovery. Networking is asynchronous for safety and partially synchronous for liveness. Byzantine faults, false Sync acknowledgments, malicious peers, corrupted RAM and complete loss of the shared SSD are excluded. Detectable corruption causes conservative unavailability, not guaranteed recovery.

In the table, “GET/write” means new linearizable client operations. An already-completed retry never authorizes a follower to serve cached data. Each scenario initially uses three nodes with committed data unless a different initial state is shown. Invariant identifiers are defined in [Raft](raft-consensus.md).

| Scenario / initial state and fault | Expected reaction | Availability | Invariants | Recovery | Test |
|---|---|---|---|---|---|
| F-01 — Healthy cluster; follower crash | Leader continues with the other peer. | GET/write on majority. | INV-02/04/06 | Restart and log or snapshot catch-up. | T-024 |
| F-02 — Healthy cluster; leader crash | Timeout, election on two nodes; pending outcomes uncertain. | Pause then majority service. | INV-01/03 | Old leader returns as follower. | T-004/T-024 |
| F-03 — Healthy cluster; two crashes | No quorum; no new commit. | No new GET/write; local status. | INV-03/04 | Restart at least two intact directories. | T-019/T-023 |
| F-04 — Active replication; slow network | Correlated timeout and retry; elections possible. | Progress only with timely delivery. | INV-01/05 | Remove delay; reelect if needed. | T-020 |
| F-05 — RPC in flight; message loss | Retransmission does not change logical effect. | Reduced; no guarantee without delivery. | INV-02/06 | Restore network and catch up. | T-003/T-020 |
| F-06 — RPC in flight; duplication | Votes, appends and chunks are idempotent. | Normal if majority communicates. | INV-01/02 | Ignore duplicates using correlation and indices. | T-006/T-015 |
| F-07 — Leader change; reordered response | Ignore obsolete progress, observe a higher term. | Normal on the new majority. | INV-05 | New attempt with current generation. | T-006 |
| F-08 — Three nodes; 1 versus 2 partition | Side 2 elects/continues; isolated node cannot complete new barriers started after isolation. | Majority only. | INV-01/03/04 | Heal and replace uncommitted suffix. | T-019/T-021 |
| F-09 — Stopped node; restart | Follower recovery; no local client reads. | Other two available. | INV-04/06 | Replay committed then sync from leader. | T-014 |
| F-10 — Isolated leader; returns after election | Step down on higher term; no committed overwrite. | New majority. | INV-02/03 | Reconcile log. | T-006/T-019 |
| F-11 — Persistence active; write/Sync fails | Node enters fatal-storage; no positive ack. | Only if other two operate. | INV-06 | Fix cause and restart with recovery. | T-013 |
| F-12 — WAL near cap; ENOSPC | Local physical quota; stop without deleting committed data. | Majority if two directories are healthy and host space remains available. | INV-04/06 | Free only verified inactive artifacts. | T-026 |
| F-13 — Restart; incorrect CRC | Fail closed; quarantine directory. | Two healthy peers if available. | INV-04/06 | Verified manual restore, never skip records. | T-013/T-016 |
| F-14 — Admitted request; client disconnects | Remove waiter, never cancel command. | Cluster continues. | INV-04 | Retry same identity and payload. | T-012 |
| F-15 — Commit/apply; response lost | Uncertain client outcome, retained session result. | Cluster available. | INV-04/06 | Retry recovers latest result without another effect. | T-011/T-012 |
| F-16 — Three processes on one Mac; host failure | All unavailable; no physical independence. | None. | INV-06 (conditional) | Recovery only with intact persistent data; power loss unverified. | T-023 |

## Separate safety and progress

A node may believe it is leader during a partition but cannot acknowledge new operations without a quorum. Two leaders in the same term are forbidden; leaders of different terms may temporarily coexist as local beliefs. Test the first fact through complete traces and the second through failed barriers on the isolated side. No deadline authorizes applying an uncommitted write.

## Implemented local storage fault evidence

[M03](milestone-m03.md) verifies partial writes, Sync/read/close/inspection errors, torn final frames, complete corruption, failed bootstrap publication and process termination around write/Sync. Unknown append outcomes remain unknown; complete surviving bytes can be recovered without proving prior acknowledgment. No local recovery applies speculative entries or falls back on corruption. These are process-crash/live-OS observations, not OS crash or power-loss tests; full distributed scenarios stay open.

## Implemented M05 election faults

[M05 evidence](milestone-m05.md) exercises split votes, message loss/delay/duplicates/reordering, partition/heal with an isolated old leader, model crash after vote Sync/before reply, restart and incompatible second-vote rejection. Storage failures stop voting, including when discovered during shutdown; workers/LOCK are not abandoned to hide a failed wait. Higher terms from old valid replies are still persisted. Healthy heartbeat progress and same-term Election Safety are finite observations. The broad GET/write/replication scenarios above remain later work; no OS/power fault or physical availability guarantee is added.

## M06 replication failure evidence

[M06 tests](milestone-m06.md) cover loss/delay/duplication/reordering, divergent tails, restart after suffix Sync before acknowledgment and real WAL frame cuts. The oracle checks durable disk prefixes, not merely messages sent. Old-generation progress is ignored but valid higher terms persist. Sync/uncertain storage failure is fatal without a replica ack; known pre-write proposal quota rejection is retriable. Complete unacknowledged surviving frames can recover, with no inferred commit or application. OS/power faults remain outside this evidence.

## M07 commit/apply failure evidence

[M07](milestone-m07.md) tests isolated old leader without new quorum commit, majority election with retained ghost prefix, checkpoint Sync errors, blocked apply with newer commits, canceled delivery, application panic and fresh restart. Domain-negative CAS advances LastApplied; infrastructure failure does not authorize retries on an uncertain live map. A complete unacknowledged checkpoint may recover; only its covered prefix is rebuilt. Normal stop drains known committed work, fatal stop rejects admission before waiting for accepted effects. No external exactly-once/OS/power-fault guarantee.

## M08 HTTP uncertainty

[M08](milestone-m08.md) distinguishes proven pre-record exclusion from possible append. Timeout/disconnect after that boundary cannot cancel replication or committed apply, nor prove failure. Observed leadership loss before completion authorization is unknown. Isolated leaders cannot confirm new writes without quorum. A response can be lost after apply; the single-attempt test client reports unknown and does not retry. Repeated DELETE is a new effect/result until M11; later sessions remain required. HTTP shutdown cancels bounded waiters before the existing committed drain and lock release. No new process-crash, OS-crash or power-loss guarantee.

## M09 process-crash boundary

[M09](milestone-m09.md) exercises abrupt follower kill, continued majority writes, follower restart/catch-up, abrupt leader kill/new election, then killing all three and restarting intact directories. A separate SIGTERM interrupts a pending write after WAL growth without quorum: its response is unknown, safe committed state replays and the speculative key is absent before a later legal commit. Startup rejects duplicate ownership, repeated bootstrap and complete corruption without reset or fallback. These are non-Byzantine process crashes with the OS/filesystem alive, not independent machines, OS crash or power loss. Missing a durable majority or losing the shared SSD invalidates preservation assumptions. No unknown request is automatically retried.
