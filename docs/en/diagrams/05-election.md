# Leader election

[Index](../README.md) · [Italiano](../../it/diagrams/05-election.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
sequenceDiagram
  participant A as n1 candidate
  participant B as n2 follower
  participant C as n3 follower
  A->>A: Election timeout; increment term
  A->>A: Persist self vote and Sync
  A->>B: RequestVote with last index and term
  A->>C: RequestVote with last index and term
  B->>B: Validate up-to-date log and vote
  B->>B: Persist term and vote; Sync
  B-->>A: Vote granted
  A->>A: Two distinct votes; become leader
  A->>A: Append current-term noop
  A->>B: AppendEntries
  A->>C: AppendEntries
```

The diagram shows a winning path, not a maximum duration. A denied vote does not reset the timer. Split votes require new elections; a voter crash retains its synchronized vote.

[Normative specification](../raft-consensus.md). Labels are localized; identifiers, order and conditions match the Italian version.
