# Architettura generale

[Indice](../README.md) · [English](../../en/diagrams/01-system.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
flowchart LR
  C[Client] --> N1[n1 API + Raft + WAL]
  C --> N2[n2 API + Raft + WAL]
  C --> N3[n3 API + Raft + WAL]
  N1 <--> N2
  N2 <--> N3
  N1 <--> N3
```

Tre processi sullo stesso Mac, quorum 2, ogni nodo con directory propria. Le frecce client non implicano che ogni nodo accetti proposte: follower risponde NOT_LEADER.

[Specifica normativa](../architecture.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
