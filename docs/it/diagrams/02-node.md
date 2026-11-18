# Architettura interna di un nodo

[Indice](../README.md) · [English](../../en/diagrams/02-node.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
flowchart TD
  HTTP[Handler HTTP] --> Q[Code limitate]
  Q --> R[Ciclo eventi Raft]
  R --> D[Worker storage]
  D --> W[WAL e Sync]
  W --> DC[Completamento durevole]
  DC --> R
  R --> T[Trasporto peer]
  T --> Q
  R --> A[Applier ordinato]
  A --> M[Mappa e sessioni]
  A --> Q
  A --> S[Cattura immutabile snapshot]
  S --> D
  R --> O[Log e metriche]
```

Il core possiede stato Raft; l'applier possiede mappa e sessioni. Gli effetti dipendenti da persistenza attendono completamento durevole. Nessun handler legge la mappa direttamente.

[Specifica normativa](../architecture.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
