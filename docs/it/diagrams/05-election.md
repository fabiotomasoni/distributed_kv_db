# Elezione del leader

[Indice](../README.md) · [English](../../en/diagrams/05-election.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
sequenceDiagram
  participant A as n1 candidato
  participant B as n2 follower
  participant C as n3 follower
  A->>A: Timeout elettorale e incremento termine
  A->>A: Persistenza voto proprio e Sync
  A->>B: RequestVote con ultimo indice e termine
  A->>C: RequestVote con ultimo indice e termine
  B->>B: Verifica log aggiornato e voto
  B->>B: Persistenza termine e voto con Sync
  B-->>A: Voto concesso
  A->>A: Due voti distinti e nuovo leader
  A->>A: Append noop del termine corrente
  A->>B: AppendEntries
  A->>C: AppendEntries
```

Il diagramma mostra un percorso vincente, non un tempo massimo. Voto negato non resetta il timer. Split vote richiede nuove elezioni; crash del votante conserva il voto sincronizzato.

[Specifica normativa](../raft-consensus.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
