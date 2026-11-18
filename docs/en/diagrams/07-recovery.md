# Crash recovery

[Index](../README.md) · [Italiano](../../it/diagrams/07-recovery.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Design view

```mermaid
flowchart TD
  S[Start] --> L[Acquire exclusive directory lock]
  L --> I[Validate identity and CURRENT]
  I --> V{Generation valid?}
  V -->|No| X[Fail closed and preserve evidence]
  V -->|Yes| SN[Load verified snapshot and sessions]
  SN --> W[Replay complete WAL transactions]
  W --> T{Incomplete final frame only?}
  T -->|Yes| TR[Truncate final incomplete frame and Sync]
  T -->|No| CK[Check CRC and monotonic metadata]
  TR --> CK
  CK --> A[Apply only through persisted commit]
  A --> F[Start follower and catch up]
  F --> B[New quorum read barrier before GET]
```

A complete frame with bad checksum fails closed; the incomplete branch applies only in the process-crash model. Present log does not mean committed log.

[Normative specification](../storage-and-recovery.md). Labels are localized; identifiers, order and conditions match the Italian version.
