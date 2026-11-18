# Recupero dopo crash

[Indice](../README.md) · [English](../../en/diagrams/07-recovery.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
flowchart TD
  S[Avvio] --> L[Lock esclusivo della directory]
  L --> I[Verifica identita e CURRENT]
  I --> V{Generazione valida?}
  V -->|No| X[Fail-closed e conservazione evidenze]
  V -->|Si| SN[Caricamento snapshot verificato e sessioni]
  SN --> W[Replay transazioni WAL complete]
  W --> T{Solo ultimo frame incompleto?}
  T -->|Si| TR[Troncamento ultimo frame incompleto e Sync]
  T -->|No| CK[Controllo CRC e metadati monotoni]
  TR --> CK
  CK --> A[Apply solo fino al commit persistito]
  A --> F[Avvio follower e catch-up]
  F --> B[Nuova barriera quorum prima di GET]
```

Checksum errato di frame completo porta a fail-closed; il ramo incompleto vale solo nel modello crash di processo. Log presente non equivale a log committed.

[Specifica normativa](../storage-and-recovery.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
