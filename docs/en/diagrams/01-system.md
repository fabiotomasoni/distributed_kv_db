# System architecture

[Index](../README.md) · [Italiano](../../it/diagrams/01-system.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
flowchart LR
  C[Client] --> N1[n1 API + Raft + WAL]
  C --> N2[n2 API + Raft + WAL]
  C --> N3[n3 API + Raft + WAL]
  N1 <--> N2
  N2 <--> N3
  N1 <--> N3
```

Three processes on one Mac, quorum 2, each with its own directory. Client arrows do not imply every node accepts proposals: a follower returns NOT_LEADER.

[Normative specification](../architecture.md). Labels are localized; identifiers, order and conditions match the Italian version.
