# Write lifecycle

[Index](../README.md) · [Italiano](../../it/diagrams/03-write.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
sequenceDiagram
  participant C as Client
  participant L as Leader
  participant W as Local WAL
  participant F as Follower
  participant A as Applier
  C->>L: PUT with client_id and sequence
  L->>W: Append transaction and Sync
  W-->>L: Durable
  L->>F: AppendEntries
  F->>F: Persist and Sync
  F-->>L: Success for correlated RPC
  L->>L: Majority and current-term commit rule
  L->>W: Persist commit checkpoint and Sync
  W-->>L: Durable commit
  L->>A: Apply committed command
  A-->>L: Result and session updated
  L-->>C: Completed result
```

The leader counts itself only after Sync. One durable follower plus the leader form a majority. If the client response is lost, retry the identical operation; never infer rollback.

[Normative specification](../api-specification.md). Labels are localized; identifiers, order and conditions match the Italian version.
