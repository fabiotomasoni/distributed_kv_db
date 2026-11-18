# Sviluppo offline e zero spese

[Indice](../README.md) · [English](../../en/adr/ADR-009-offline-first-development.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-009 — Sviluppo offline e zero spese**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Budget 0 €, un solo computer già posseduto.

## Problema

Eliminare dipendenze operative da account, cloud o consumo fatturabile.

## Alternative valutate

Servizi cloud free-tier/trial; container locali; tre processi nativi con tool gratuiti.

## Decisione

Go standard, filesystem e loopback; dipendenze acquisite una volta, runtime/test offline.

## Motivazione

È sufficiente per gli obiettivi ed evita addebiti e consumi infrastrutturali inutili.

## Conseguenze

GitHub Free opzionale, cap risorse e documenti bilingui mantenuti insieme.

## Svantaggi e limiti

Bootstrap iniziale richiede acquisizione tool; nessuna indipendenza fisica dell'host.

## Requisiti collegati

NFR-007, NFR-011, NFR-012. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M14/M16: demo senza rete esterna e inventario licenze/costi. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../development-guide.md); [fonti REF-04/10/11](../references.md); [roadmap](../implementation-roadmap.md).
