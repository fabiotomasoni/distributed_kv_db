# Letture con barriera nel log

[Indice](../README.md) · [English](../../en/adr/ADR-006-linearizable-reads.md)

**22 novembre 2026 — baseline letture nel log M10 implementata e verificata; verifiche generali M14 ancora aperte.**

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

## Decisione implementazione M10 — 22 novembre 2026

[M10](../milestone-m10.md) adotta GET nel log scelta per baseline implementata. Ogni tentativo persiste read fresca del termine corrente, attende commit/checkpoint maggioranza durevole legale e acquisisce risultato nell’applicazione M02 esatta prima di autorizzazione leadership serializzata owner. Superati HTTP/fixture rigorosi, apply bloccato slot esatto, isolamento vecchio leader con seed e piccole storie indipendenti; passano anche letture HTTP dopo crash. Accetta meccanismo M10 entro evidenze finite, non prova universale. ReadIndex/lease restano assenti e richiedono gate futuri già definiti. Codifica read canonica esistente invariata; capacità startup omogenea conserva test commit-only legacy senza negoziazione rolling.
