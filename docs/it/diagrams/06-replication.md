# Replica e commit

[Indice](../README.md) · [English](../../en/diagrams/06-replication.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
flowchart TD
  P[Proposta nel termine corrente] --> D[WAL locale durevole]
  D --> R[AppendEntries con indice e termine prev]
  R --> V{Prefisso follower coincide?}
  V -->|No| H[Hint conflitto e riduzione limitata nextIndex]
  H --> R
  V -->|Si| F[Persistenza suffisso modificato e Sync]
  F --> M[Aggiornamento matchIndex correlato]
  M --> Q{Match maggioranza e termine N corrente?}
  Q -->|No| Wait[Attesa o deadline con esito incerto]
  Q -->|Si| C[Avanzamento e persistenza commit]
  C --> A[Scheduling prefisso committed non accodato]
  A --> AP[Completamento apply ordinato]
  AP --> Reply[Completamento waiter client]
```

Entry vecchie possono diventare committed transitivamente tramite un indice corrente, mai contando soltanto repliche di vecchio termine. Heartbeat non tronca suffissi.

[Specifica normativa](../raft-consensus.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
