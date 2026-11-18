# Replication and commit

[Index](../README.md) · [Italiano](../../it/diagrams/06-replication.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
flowchart TD
  P[Proposal in current term] --> D[Local WAL durable]
  D --> R[AppendEntries with prev index and term]
  R --> V{Follower prefix matches?}
  V -->|No| H[Conflict hint and bounded nextIndex reduction]
  H --> R
  V -->|Yes| F[Persist changed suffix and Sync]
  F --> M[Update correlated matchIndex]
  M --> Q{Majority match and term N current?}
  Q -->|No| Wait[Wait or deadline unknown]
  Q -->|Yes| C[Advance and persist commit]
  C --> A[Schedule unscheduled committed prefix]
  A --> AP[Ordered apply completion]
  AP --> Reply[Complete client waiter]
```

Old entries may become committed transitively through a current-term index, never merely by counting old-term replicas. Heartbeats do not truncate suffixes.

[Normative specification](../raft-consensus.md). Labels are localized; identifiers, order and conditions match the Italian version.
