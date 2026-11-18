# Ciclo di una scrittura

[Indice](../README.md) · [English](../../en/diagrams/03-write.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
sequenceDiagram
  participant C as Client
  participant L as Leader
  participant W as WAL locale
  participant F as Follower
  participant A as Applier
  C->>L: PUT con client_id e sequence
  L->>W: Append transazione e Sync
  W-->>L: Durevole
  L->>F: AppendEntries
  F->>F: Persistenza e Sync
  F-->>L: Successo RPC correlata
  L->>L: Maggioranza e regola commit termine corrente
  L->>W: Persistenza checkpoint commit e Sync
  W-->>L: Commit durevole
  L->>A: Apply comando committed
  A-->>L: Risultato e sessione aggiornata
  L-->>C: Risultato completato
```

Il leader conta la propria copia solo dopo Sync. Un follower durevole più il leader costituiscono maggioranza. Se risposta client persa, ritentare identica operazione; non inferire rollback.

[Specifica normativa](../api-specification.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
