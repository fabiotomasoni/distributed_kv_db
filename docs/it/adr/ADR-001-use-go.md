# Usare Go e libreria standard

[Indice](../README.md) · [English](../../en/adr/ADR-001-use-go.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-001 — Usare Go e libreria standard**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Uno studente deve gestire concorrenza, rete e test con budget nullo.

## Problema

Ridurre toolchain e dipendenze senza nascondere Raft.

## Alternative valutate

Go; Rust per ownership statica; C++ per controllo memoria.

## Decisione

Go darwin/arm64, baseline 1.27.4, nessuna libreria Raft o runtime esterno.

## Motivazione

HTTP, JSON, testing, log e checksum sono disponibili nella standard library.

## Conseguenze

Pacchetti piccoli, core a eventi e test locali senza container.

## Svantaggi e limiti

GC e scheduling richiedono misure; un mutex non prova correttezza distribuita.

## Requisiti collegati

NFR-009, NFR-012. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M01/M04: verificare toolchain e race detector ARM64. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../development-guide.md); [fonti REF-04/06/07/10/11](../references.md); [roadmap](../implementation-roadmap.md).
