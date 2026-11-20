# Tre membri statici

[Indice](../README.md) · [English](../../en/adr/ADR-002-static-three-node-cluster.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-002 — Tre membri statici**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Un solo Mac deve rappresentare un sistema distribuito osservabile.

## Problema

Trovare equilibrio tra quorum non banale e risorse contenute.

## Alternative valutate

Un nodo; tre processi; cinque processi; tre macchine fisiche.

## Decisione

Un gruppo n1/n2/n3 configurato staticamente su loopback, quorum 2.

## Motivazione

Tre è il minimo per tollerare un crash con consenso maggioritario.

## Conseguenze

Identità e config immutabili; nessuna membership dinamica.

## Svantaggi e limiti

Host, alimentazione e SSD sono un singolo dominio di guasto.

## Requisiti collegati

FR-001, FR-002, NFR-004. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M04/M14: configurazioni e failover locale verificati. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../scope.md); [fonti REF-01/02](../references.md); [roadmap](../implementation-roadmap.md).

## Evidenze implementazione M04 — 20 novembre 2026

[M04](../milestone-m04.md) implementa la decisione tramite [configurazione/hash](../configuration.md) immutabili, fixture protocollo rigorose e HTTP loopback limitato con libreria standard. Identità storage/LOCK precedono bind di produzione; shutdown drena prima del rilascio. Nessun DNS/proxy/redirect/compressione o dipendenza esterna. Una RPC in uscita per peer, socket/deadline limitati. Decisioni callback e futuri timing/replica Raft restano fuori M04. Motivazioni originali e limiti loopback fidato invariati.
