# Persistenza e snapshot atomici

[Indice](../README.md) · [English](../../en/diagrams/09-snapshot.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Vista progettuale

```mermaid
flowchart TD
  A[Indice committed S applicato] --> C[Cattura mappa e sessioni a S]
  C --> R[Prenotazione spazio temporaneo di picco]
  R --> B[Drain write precedenti e blocco metadati finali]
  B --> W[Scrittura nuovo snapshot e suffisso WAL conservato]
  W --> S[Sync file e directory temporanea]
  S --> G[Rename generazione e Sync directory padre]
  G --> P[Scrittura CURRENT temporaneo e Sync]
  P --> N[Rename CURRENT e Sync directory padre]
  N --> Ack[Nuova generazione pubblicata durevolmente]
  Ack --> SW[Switch writer e generazione poi rilascio barriera]
  SW --> D[Eliminazione generazione precedente e Sync directory padre]
```

Append concorrenti sono serializzati dal worker durante rotazione finale. Nessuna cancellazione precede la pubblicazione durevole; crash intermedi scelgono la generazione referenziata da CURRENT.

[Specifica normativa](../storage-and-recovery.md). Le etichette sono tradotte; identificatori, ordine e condizioni corrispondono alla versione inglese.
