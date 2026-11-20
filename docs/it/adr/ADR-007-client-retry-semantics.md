# Sequenze client e deduplicazione limitata

[Indice](../README.md) · [English](../../en/adr/ADR-007-client-retry-semantics.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-007 — Sequenze client e deduplicazione limitata**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Risposta persa non rivela se una mutazione sia stata eseguita.

## Problema

Evitare doppio effetto senza tabella dedup illimitata o TTL basato su clock.

## Alternative valutate

Nessuna dedup; UUID con TTL; sessioni sequenziali con ultimo risultato.

## Decisione

Client ID persistente, una pendente, sequence monotona; 1 024 sessioni senza eviction.

## Motivazione

Stato limitato e deterministico; high-water mark impedisce riapplicazioni storiche.

## Conseguenze

Risultati sessione replicati e negli snapshot; quota e CAS fallito consumano sequenza.

## Svantaggi e limiti

Solo ultimo risultato recuperabile; cap sessioni può bloccare nuovi client; no exactly-once delivery.

## Requisiti collegati

FR-011, FR-012. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M11/M13: crash client/server, conflitti identità e retry dopo snapshot. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../data-model.md); [fonti REF-01/05](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

Confermate sessioni sequenziali senza TTL e risultato ultimo persistente. Il checker chiude l'intervallo logico alla prima risposta definitiva e non lo allunga per retry successivi; conflitti identità sono osservazioni di protocollo distinte. Nessun retry giustifica più di un effetto logico. Validare T-011/T-012/T-022 anche dopo snapshot.

## Chiarimento digest canonico — 19 novembre 2026

I [formati canonici](../canonical-formats.md) fissano ora preimmagine tipizzata/versionata del digest e schema risultato in cache/sessione snapshot. Identità/sequence confrontate separatamente; digest su opcode e argomenti operativi tipizzati, esclusi metadati trasporto. Expected assente e presente vuoto differiscono. Risolve ambiguità di formato storage, non implementazione sessioni o accettazione T-011/T-012.

## Confine M08 — sessioni ancora assenti

[M08](../milestone-m08.md) valida/persiste client_id e sequence ma deliberatamente non implementa deduplicazione prima di M11. Test client esegue un tentativo, mai retry automatico di unknown e valida hint leader configurati senza seguire redirect. DELETE identica ripetuta può restituire deleted=true poi false a nuovi indici; nessuna garanzia risultato originale o at-most-once. Decisione progettuale sopra resta richiesta per M11/M13, non promessa attuale. Timeout/disconnessione non annulla possibile effetto committed.
