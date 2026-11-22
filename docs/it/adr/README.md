# Indice ADR

[Indice](../README.md) · [English](../../en/adr/README.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Registro delle decisioni

Tutte le decisioni sono Proposed: forniscono una baseline concreta senza fingere che l'implementazione le abbia validate. Gli obblighi dell'incarico (zero spese, bilinguismo, niente codice in questa fase) restano vincoli, non opzioni da mettere ai voti. Accettazione futura richiede motivazione e gate; sostituzione usa stato Superseded e nuovo ADR, non riuso dell'identificatore.

| ID | Decisione | Stato |
|---|---|---|
| ADR-001 | [Usare Go e libreria standard](ADR-001-use-go.md) | Proposed |
| ADR-002 | [Tre membri statici](ADR-002-static-three-node-cluster.md) | Proposed |
| ADR-003 | [Implementare direttamente Raft](ADR-003-raft-consensus.md) | Proposed |
| ADR-004 | [Trasporto HTTP/JSON locale](ADR-004-network-transport.md) | Proposed |
| ADR-005 | [WAL sincrono e generazioni snapshot](ADR-005-persistence-strategy.md) | Proposed |
| ADR-006 | [Letture con barriera nel log](ADR-006-linearizable-reads.md) | Proposed |
| ADR-007 | [Sequenze client e deduplicazione limitata](ADR-007-client-retry-semantics.md) | Proposed |
| ADR-008 | [Fault injection locale deterministica](ADR-008-fault-injection.md) | Proposed |
| ADR-009 | [Sviluppo offline e zero spese](ADR-009-offline-first-development.md) | Proposed |

- [ADR-010: profilo applicativo sessioni versionato](ADR-010-session-profile.md)
