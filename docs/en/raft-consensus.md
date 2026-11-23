# Raft core specification

[Index](README.md) · [Italiano](../it/raft-consensus.md)

**20 November 2026 — M05–M07 elections, replication and commit/application implemented and checked; full client/snapshot safety remains future work.**

## State, indices and properties

Baseline contract: the [Raft paper](https://raft.github.io/raft.pdf), Figure 2 and sections 5–7. This specification chooses a static group and does not implement joint consensus. No property has yet been proved for QuorumKV.

Persistent before dependent responses: `currentTerm`, `votedFor` (empty or one member), log and snapshot metadata. Volatile: role, leader hint, `commitIndex`, `lastApplied`; leader: `nextIndex[peer]`, `matchIndex[peer]`, active requests. `snapshotIndex <= lastApplied <= commitIndex <= lastLogIndex`; after recovery lastApplied starts at the snapshot and grows by replaying persisted commit. Persisted commit is a conservative local extension described in [storage](storage-and-recovery.md).

The log uses contiguous global uint64 indices starting at 1; `(0,0)` is the sentinel. Snapshots retain `(lastIncludedIndex,lastIncludedTerm)` as a comparable boundary. The last-entry term used for elections includes this boundary when the suffix is empty.

| Invariant | Design obligation and argument |
|---|---|
| INV-01 Election Safety | At most one leader per term: intersecting majorities and one persistent vote per term. |
| INV-02 Log Matching | Equal index and term imply identical prefixes: prev checks and replacement only at the first conflict. |
| INV-03 Leader Completeness | Every future leader contains committed entries: up-to-date election restriction and current-term commit rule; quorum alone is insufficient. |
| INV-04 State Machine Safety | Replicas never apply different commands at the same index: immutable committed prefix and ordered apply. |
| INV-05 Monotonicity | Term, commit and apply never regress; matchIndex never regresses within a leader generation. |
| INV-06 Persist-before-ack | Positive votes and replication represent already-synchronized state. |

## Transitions and timers

Always start as follower, with recovered vote and no assumption of previous leadership. Election timeout: increment term, become candidate, vote for self, persist, then send RequestVote. Count each member once, self only after Sync; two votes in the same term establish leadership. Initialize nextIndex=lastLogIndex+1, peer matchIndex=0 and local matchIndex=last durable index. Append a new-term noop; no client success relies on an old entry before legal commit.

Any valid RPC or response with a higher term: update term, clear votedFor, become follower and synchronize before responding. Lower term: reject with current term, without resetting the timer. Candidate receiving valid same-term AppendEntries steps down without clearing that term's vote. Two distinct leaders in one term violate INV-01: record and fail in simulation, rather than inventing a merging rule.

Reset the election timer only when starting an election, granting a vote, or receiving AppendEntries/InstallSnapshot from a recognized leader with term at least current (even if prev mismatches). Do not reset on denied RequestVote or arbitrary traffic. Split votes produce no winner; new randomized timeouts avoid systematic synchronization, with no theoretical time bound in an asynchronous network.

## RequestVote

Grant if the term is not obsolete, `(votedFor is empty or candidate_id)` and the candidate log is at least as up-to-date: compare `(lastLogTerm,lastLogIndex)` lexicographically, term first. Persist term and vote before `vote_granted=true`; repeated voting for the same candidate may be acknowledged. A crash immediately after voting must not permit a different vote in that term. Old responses are not counted in a new election; a higher term forces step-down.

## AppendEntries, heartbeats and conflicts

1. Validate envelope and batch without partial effects; handle term and role.
2. Verify prev exists with prevLogTerm, including the snapshot sentinel; if missing, reject with a useful index. If prev precedes the snapshot boundary, request boundary realignment/installation without accessing removed indices.
3. Find the first index with a different term. Truncate the suffix **only from that conflict** and append missing entries. Do not truncate because a batch ends before the local log; heartbeats never delete entries. If a conflict is <= commitIndex, stop with an integrity error: never overwrite committed data.
4. Atomically persist suffix replacement before success. Already-identical entries need no rewrite unless hard state is pending.
5. Update commit to `max(commitIndex, min(leaderCommit, prevLogIndex+len(entries)))`, never beyond the prefix verified by this request; persist advancement before apply.
6. Respond with success and match_index=prev+len(entries), not the entire local log length.

On rejection the leader reduces nextIndex using conflict_index; if conflict_term is locally known it may skip to the last index of that term +1. Hints are always bounded by the current log and correlated request. If nextIndex <= snapshotIndex, send a snapshot. On success update matchIndex monotonically and nextIndex=match+1. An obsolete failure response never undoes a newer success; one RPC per peer simplifies but does not remove correlation requirements.

## Commit and apply

The leader chooses the largest N with `N > commitIndex`, `log[N].term == currentTerm` and at least two `matchIndex >= N`, including its own durable log. Only then may it advance commitIndex to N and include the entire prefix transitively. Never commit an older-term entry merely because it is now present on two nodes. The current noop allows the inherited prefix to become committed indirectly.

Design pseudocode, not implementation:

```text
on durable_replicated_progress:
  N := largest index with majority durable match and term[N] = currentTerm
  if N > commitIndex:
    persist(commit=N)
    after durable: commitIndex := N; schedule_apply()
on schedule_apply or apply_capacity_available:
  end := min(commitIndex, scheduledApplyIndex + available_apply_slots)
  if end > scheduledApplyIndex:
    enqueue_apply(scheduledApplyIndex+1 .. end, applyEpoch)
    scheduledApplyIndex := end  // only after successful queue admission
on applied(epoch, i, result):
  if epoch != applyEpoch: discard obsolete completion; return
  require i = lastApplied+1
  require i <= scheduledApplyIndex <= commitIndex
  lastApplied := i
  resolve matching client waiter only if response conditions hold
  schedule_apply()
```

Apply contiguously and never apply speculative entries. Mutation results and sessions change in the same deterministic transition. During leadership change, waiters may receive uncertain outcomes; no uncommitted entry is presented as definitive.

## Edge cases and liveness

| Case | Required behavior |
|---|---|
| Leader loses quorum | It may still believe it is leader; it cannot commit or return a local GET. Deadline → unknown. |
| Old leader returns | Higher terms make it a follower; its uncommitted suffix may be replaced. |
| Divergent logs | Prev comparison and suffix repair; committed data is never removed. |
| Leader inherits old entries | Commit a current noop/entry before committing the prefix through counting. |
| Late responses | Ignore obsolete-generation progress; still observe valid higher terms. |
| Crash after vote/append | Recovery retains synchronized state; no acknowledgment before Sync. |
| New leader with incomplete log | Election restriction prevents victory if it would exclude the committed prefix; uncommitted entries may disappear. |

The system favors safety over availability. It uses no time leases, requires no synchronized clocks and promises no election when timeouts are continuously violated. [T-004–008 and T-021](testing-strategy.md) must verify transitions and invariants in simulation and real processes; design arguments do not replace evidence.

## Persistent schema boundary

[Canonical formats](canonical-formats.md) fixes log kinds, typed commands, transaction semantics and snapshot/control metadata. Decoding a valid commit checkpoint does not establish that a runtime Raft transition was legal; consensus must supply that proof. No Raft or noop/session adapter is implemented by this specification.

## Implemented M05 election boundary

[M05](milestone-m05.md) implements New/Tick/Request/Response/Durable/Stop with explicit current milliseconds, injected draws and one pending synchronized effect. Recover follower/term/vote and copied term boundaries; count self only after exact generation/sequence completion, quorum 2 then synchronize a local new-term noop before publishing leader. Empty heartbeats recognize leadership/reset timers and check prev without deleting log, advancing commit or applying. Nonempty append/snapshot effects remain unavailable, although valid higher terms are persisted. Late replies still convey higher terms; old votes cannot elect a new term. Status.Pending marks tentative fields, Failure makes them unusable as acknowledgment. The 20-seed saved vote/term/leader oracle and 100 healthy heartbeat rounds pass. Log replication, nextIndex/matchIndex, commit/apply, sessions and snapshot state machines remain future phases; the broad algorithm above is not all implemented.

## Implemented M06 replication boundary

[M06](milestone-m06.md) adds startup EnableReplication(1..16), canonical owned entries, AppendLocal with local-Sync-only receipt, first-conflict replace_suffix and exact durable-prefix acknowledgments. Status.Progress copies next/match values; Sent/Failed distinguish actual dispatch from skipped offers, and correlated current-generation responses alone update progress. Conflict hints are bounded and match never regresses. Recovered committed prefixes cannot be overwritten. The general commit/apply algorithm above remains M07: leaderCommit is carried but never advances a checkpoint here. Original M05 mode/fixtures remain supported; StartReplication enables the complete M06 layer.

## M07 implemented commit and application

[M07](milestone-m07.md) enables the current-term majority boundary only after correlated replica proof and persists commit_index before changing CommitIndex or scheduling. Followers learn max(old,min(leaderCommit,verified request end)) only after relevant log Sync; duplicate/obsolete/rejected-prefix traffic cannot advance it. The existing new-term noop enables indirect old-prefix commit. Core owns LastApplied<=ScheduledApplyIndex<=CommitIndex, advancing Scheduled only on M02 admission and LastApplied only on future completion. ApplyEpoch=1 is instance-local; snapshots/replacement epochs remain M13. Legacy M05/M06 modes remain available, StartCommitted selects M07. No public reads/client guarantee.

## M08 completion authorization

[Service.Write](../../internal/raft/write.go) preserves M07 commit/application algorithms. A ticket lock separates permanent pre-record exclusion from possible disk effects. Once recording may begin, cancellation abandons only the caller. Local Sync alone never yields completed: wait for the exact indexed/termed applied result, then send a fence through the owner. It rejects failure, shutdown, nonleader, changed term or insufficient commit/apply. Higher-term observation before that event prevents success even when the command is committed/applied. Result expiry or supersession yields uncertainty. No session table or new persistent format.

## M09 restart evidence

[M09](milestone-m09.md) leaves core transitions unchanged. A controlled three-node image has term 5, committed index 1 and an uncommitted index 2 on every node. Each startup publishes follower/term 5 with LastApplied=ScheduledApplyIndex=CommitIndex=1 and LastIndex=2. Only the later current-term quorum noop can commit/apply index 2. Real process kills additionally test leader/follower recovery, catch-up and acknowledged-prefix preservation. No term, vote or commit is inferred from previous leadership or raw log length.

## M10 current-term read entries

[M10](milestone-m10.md) enables approved read(key) proposals, follower replication and replay under explicit startup capability. Every request assigns a fresh current-term index. Existing local Sync, exact-correlated follower match, current-term majority boundary and synchronized commit checkpoint are unchanged. The read entry itself can satisfy the current-term commit condition; no separate heartbeat lease/ReadIndex round is substituted. ScheduledApplyIndex advances only on actual M02 admission, LastApplied on completion, and the read result belongs to that index/term. The existing owner fence rejects an observed higher term before response authorization. Old responses cannot commit the new read prefix; legacy stale-ack tests remain unchanged.

## M13 Phase C — 23 November 2026

[Phase C](milestone-m13-runtime-recovery.md) is RUNTIME RECOVERY PHASE COMPLETED: RecoverCommitted restores S and applies only S+1..C through M02, then drains/closes the isolated worker before a fresh applier at C. WAL and snapshot validated under continuous LOCK. Startup is explicit through node.StartAPIWithSnapshotRecovery for v2; ordinary StartAPI/CLI retain the base-zero guard. No fallback on corrupt CURRENT/snapshot. KV/sessions, hard state and A=L=C verified with follower/leader/all-process crash/restarts. C-01/OQ-05 confirmed; recovery C-02/OQ-01 subset verified, overall C-02/C-03 CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016 and T-026/AC-026/NFR-006 open: compaction, cleanup and InstallSnapshot unimplemented. M13 PARTIALLY COMPLETED, M14 not started.

## M13 Phase D — 23 November 2026

[Phase D](milestone-m13-compaction-preliminary.md) is ISOLATED COMPACTION PHASE COMPLETED: approved base-S representation, candidate from actual isolated apply and field-by-field proof against full replay. Publication/recovery reuse B/C in test directories; sources and previous generations retained, no WAL repair in D proofs. PrepareCompaction is memory-only; private orchestration has no runtime callers. StartAPIWithSnapshotRecovery unchanged. C-01/OQ-05 confirmed, isolated C-02/OQ-01 evidence added; overall C-02/C-03 CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016, T-026/AC-026/NFR-006 open. InstallSnapshot, live compaction, GC and prefix deletion unimplemented; M13 PARTIALLY COMPLETED, M14 not started.

## M13 Phase E — 23 November 2026

[Phase E](milestone-m13-installsnapshot-preliminary.md) is **ISOLATED INSTALLSNAPSHOT PHASE COMPLETED**: dormant receiver/controller, existing M04 chunks, real M05 term/vote persistence, coherent local suffix, B publication under continuous LOCK, fresh KV/sessions and apply epoch, correlated final ACK and deterministic suffix catch-up verified in isolation. No ordinary node/service/server/transport integration. C-01/OQ-05 confirmed; isolated C-02/OQ-01 and C-03 evidence added, overall both CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016 and T-026/AC-026/NFR-006 remain open. Runtime InstallSnapshot, live compaction, GC and prefix deletion remain unactivated; M13 PARTIALLY COMPLETED, M14 not started.

The base-S sentinel remains term-addressable but is never an application entry. Existing RequestVote last-term/index and AppendEntries prev matching are tested below/at/above a nonzero boundary and after restart. Dormant EnableRecoveredCommit is startup-only and consumes a caller's already-applied proof; ordinary capability paths do not invoke it. After real worker drain, incoming S<=A is obsolete. Matching term[S] retains all S+1..N and max(C,S); nonmatching S<=C is rejected, otherwise only speculative local suffix is excluded from the new generation. New epoch rejects old completions; N=MaxUint64 activation is refused before publication to preserve the core's nextIndex guard.
