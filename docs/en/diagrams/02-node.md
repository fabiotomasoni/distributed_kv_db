# Internal node architecture

[Index](../README.md) · [Italiano](../../it/diagrams/02-node.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
flowchart TD
  HTTP[HTTP handlers] --> Q[Bounded queues]
  Q --> R[Raft event loop]
  R --> D[Storage worker]
  D --> W[WAL and Sync]
  W --> DC[Durable completion]
  DC --> R
  R --> T[Peer transport]
  T --> Q
  R --> A[Ordered applier]
  A --> M[Map and sessions]
  A --> Q
  A --> S[Immutable snapshot capture]
  S --> D
  R --> O[Logs and metrics]
```

The core owns Raft state; the applier owns map and sessions. Persistence-dependent effects wait for durable completion. No handler reads the map directly.

[Normative specification](../architecture.md). Labels are localized; identifiers, order and conditions match the Italian version.
