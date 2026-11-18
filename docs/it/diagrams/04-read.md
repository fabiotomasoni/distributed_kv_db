# Ciclo di lettura linearizzabile

[Indice](../README.md) · [English](../../en/diagrams/04-read.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
sequenceDiagram
  participant C as Client
  participant L as Leader
  participant Q as Maggioranza durevole
  participant A as Applier ordinato
  C->>L: GET chiave
  L->>L: Append read nel termine corrente
  L->>Q: Persistenza e replica read
  Q-->>L: Acknowledgment durevoli
  L->>L: Commit con regola termine corrente
  L->>L: Sync checkpoint commit
  L->>A: Apply prefisso fino allo slot read
  A->>A: Lettura chiave nello slot read esatto
  A-->>L: Risultato lettura acquisito
  L->>L: Ricontrollo generazione osservata
  L-->>C: Valore oppure KEY_NOT_FOUND
```

La richiesta non usa un heartbeat passato come prova di leadership. Senza maggioranza la nuova barriera non viene committed e la lettura scade; un termine maggiore osservato impedisce una risposta conservativamente.

[Specifica normativa](../consistency-model.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
