# Raft core specification

[Index](README.md) · [Italiano](../it/raft-consensus.md)

**20 November 2026 — M05 elections and M06 replication implemented and checked; commit and apply below remain design work.**

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
