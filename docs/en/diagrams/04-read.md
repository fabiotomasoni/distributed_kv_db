# Linearizable read lifecycle

[Index](../README.md) · [Italiano](../../it/diagrams/04-read.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
sequenceDiagram
  participant C as Client
  participant L as Leader
  participant Q as Durable majority
  participant A as Ordered applier
  C->>L: GET key
  L->>L: Append read in current term
  L->>Q: Persist and replicate read
  Q-->>L: Durable acknowledgments
  L->>L: Commit using current-term rule
  L->>L: Sync commit checkpoint
  L->>A: Apply prefix through read slot
  A->>A: Sample key in exact read slot
  A-->>L: Captured read result
  L->>L: Recheck observed generation
  L-->>C: Value or KEY_NOT_FOUND
```

The request does not use a past heartbeat as leadership proof. Without a majority the new barrier cannot commit and the read expires; an observed higher term conservatively prevents a response.

[Normative specification](../consistency-model.md). Labels are localized; identifiers, order and conditions match the Italian version.
