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

## Evidenze implementative M02

19 novembre 2026: [M02](../milestone-m02.md) implementa soltanto il confine owner/applier locale: indici contigui forniti dal chiamante, Scheduled/LastApplied distinti, ammissione limitata ed esecuzione a worker singolo. T-008/M02 Passed, incluso backlog apply. Raft, prova del commit, applyEpoch/sostituzione snapshot e T-021 restano gate futuri; questa appendice non rende Raft Accepted né cancella l’audit.

## Implementazione elettorale M05 — 20 novembre 2026

[M05](../milestone-m05.md) adotta la decisione per ruoli/timer deterministici, un voto persistente per termine, completamento esatto generazione/sequenza e noop elettorale locale. Un adattatore storage sincrono seriale sospende avanzamento core senza mutex globale; nessun output dipendente positivo prima di Sync. Shutdown conserva errori I/O e ownership fino al drain. Superati oracolo elettorale 20 seed, 100 round heartbeat sani e race mirate. Replica/commit/apply/snapshot completi e garanzie guasti fisici restano ai gate originali; non rende tutto Raft o rotazione generazioni Accepted.

## Decisione implementativa M06

[M06](../milestone-m06.md) conserva questo ADR: sostituzione suffisso canonica sopra confine committed noto, completamento sincronizzato con token esatto e progresso peer limitato. Byte/primitive WAL esistenti riusati; proposte locali conservano riserva con AppendProposal. RPC lecite grandi possono attraversare transazioni WAL limitate con un ack finale e prefisso recuperabile sicuro all’interruzione. Commit/quorum termine corrente/apply e semantica snapshot completa restano gate successivi. Nessuna accettazione Raft completo o durabilità OS/alimentazione inferita.

## Evidenze implementazione M07

[M07](../milestone-m07.md) conserva decisione per prova quorum termine corrente, commit_index sincronizzato, replay stato volatile nuovo tramite M02 invariato e cursori completamento espliciti. Superati oracoli committed/risultati indipendenti, backlog bloccato, tagli checkpoint e test loopback complementari. Nessun checkpoint aggiuntivo, byte persistente nuovo o goroutine per entry. Evidenze C-03 M07 non chiudono gate successivi snapshot/sessioni/read/checker o garanzie guasti fisici.
