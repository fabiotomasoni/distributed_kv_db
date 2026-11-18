# Implementare direttamente Raft

[Indice](../README.md) · [English](../../en/adr/ADR-003-raft-consensus.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-003 — Implementare direttamente Raft**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Le copie devono ordinare comandi senza assumere rete affidabile.

## Problema

Preservare safety anche durante crash, partizioni ed elezioni concorrenti.

## Alternative valutate

Raft proprietario; libreria Raft esistente; primary-backup senza consenso; Paxos.

## Decisione

Core Raft a eventi, voto persistente, commit corrente e gruppo statico.

## Motivazione

Rende esplicite le proprietà oggetto del portfolio; libreria già pronta violerebbe lo scopo.

## Conseguenze

Specificare INV-01–06 e testare ogni transizione.

## Svantaggi e limiti

Alto rischio implementativo; niente claim derivato dalla sola scelta del nome Raft.

## Requisiti collegati

FR-004, FR-005, FR-006, FR-007, FR-008, NFR-001. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M05–M07/M14: review e test safety obbligatori. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../raft-consensus.md); [fonti REF-01/02](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

Confermata la regola di commit del termine corrente e il voto persistente. Corretto lo scheduling apply: scheduledApplyIndex distinto da lastApplied, code limitate e applyEpoch per installazione snapshot. Il core asincrono non può usare un cursore di completamento come cursore di accodamento. Validazione richiesta: T-008/apply-backlog e T-021 con registro ghost committed. Stato Proposed invariato.
