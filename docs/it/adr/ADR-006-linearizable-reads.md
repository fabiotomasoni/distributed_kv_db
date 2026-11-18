# Letture con barriera nel log

[Indice](../README.md) · [English](../../en/adr/ADR-006-linearizable-reads.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-006 — Letture con barriera nel log**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Un vecchio leader può restare isolato e credersi valido.

## Problema

Evitare letture stale senza assumere lease o clock sincronizzati.

## Alternative valutate

Lettura locale; lease; ReadIndex; entry read nel log.

## Decisione

GET propone read di termine corrente e campiona stato nel relativo slot apply committed.

## Motivazione

Usa un solo percorso quorum/persist/apply, più semplice da verificare inizialmente.

## Conseguenze

Niente GET sulla minoranza; quorum e applicazione sono verificati per ogni read.

## Svantaggi e limiti

Ogni lettura scrive WAL; ReadIndex solo dopo baseline e nuova review.

## Requisiti collegati

FR-010, FR-019, NFR-002. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M10/M14: test leader isolato e storie concorrenti; M15 misura costo I/O. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../consistency-model.md); [fonti REF-01/05](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

Confermata GET nel log per v1 dopo confronto esplicito di correttezza, latenza, entry, Sync e rischi. La futura ReadIndex richiede commit corrente, round nuovo dopo la richiesta, contesto termine/generazione e apply>=R; schema RPC e test vanno aggiornati prima di attivarla. La minoranza non completa GET iniziate dopo isolamento totale; risposte di operazioni già committed possono arrivare dopo il guasto. [Confronto completo e percorso](../consistency-model.md).
