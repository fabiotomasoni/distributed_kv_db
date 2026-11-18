# WAL sincrono e generazioni snapshot

[Indice](../README.md) · [English](../../en/adr/ADR-005-persistence-strategy.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-005 — WAL sincrono e generazioni snapshot**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Crash di processo non deve cancellare successi nel modello dichiarato.

## Problema

Coordinare log, voto, commit checkpoint, snapshot e limiti disco.

## Alternative valutate

RAM-only; WAL async; WAL sync e snapshot; motore database esterno.

## Decisione

WAL frame CRC32C transazionali, Sync prima di ack, snapshot generazionali con CURRENT.

## Motivazione

Separa decisioni di consenso e durabilità locale, permette cutpoint riproducibili.

## Conseguenze

Snapshot include sessioni; errore storage ferma il nodo; checkpoint commit locale.

## Svantaggi e limiti

Latenza Sync, picco temporaneo e semantica power loss APFS non provata.

## Requisiti collegati

FR-013, FR-014, FR-015, FR-016, FR-018, NFR-003, NFR-006. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M03/M13: sync directory, lock, formato finale e cutpoint; power loss resta garanzia separata. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../storage-and-recovery.md); [fonti REF-08/09](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

WAL sincrono confermato, non atomicità fisica della write: recovery accetta soltanto transazioni complete verificate. Specificata barriera finale, hard state aggiornato, token generation_id/write_sequence e installazione coordinata con applier. Il cap 32 MiB include riserva controllo. Contratti separati per crash processo, sistema operativo e power loss; solo il primo è obiettivo verificabile v1 con test di processo. Non vengono rimosse Sync necessarie. OQ-01/OQ-05 restano gate prima dello storage durevole.
