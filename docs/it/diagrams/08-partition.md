# Partizione uno contro due

[Indice](../README.md) · [English](../../en/diagrams/08-partition.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
flowchart LR
  subgraph Minoranza
    O[Vecchio leader n1]
    U[Nuova GET o scrittura] --> O
    O --> TO[Nessun nuovo commit quorum e timeout incerto]
  end
  subgraph Maggioranza
    N[Nuovo leader n2] <--> F[n3]
    C[Client] --> N
    N --> OK[Commit termine corrente e apply]
  end
  O -. Heal e termine maggiore .-> N
```

Leader di termini diversi possono temporaneamente credersi leader; mai due eletti nello stesso termine. Al heal n1 diventa follower e perde solo suffisso non committed.

[Specifica normativa](../failure-model.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
