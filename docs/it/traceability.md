# Matrice di tracciabilità

[Indice](README.md) · [English](../en/traceability.md)

**Aggiornamento: 20 novembre 2026 — M01/M02/M03 locali implementate e verificate; le specifiche distribuite restano progettuali.**

## Matrice normativa

Ogni riga ha un criterio e un test primario. I test trasversali in [testing](testing-strategy.md) ampliano la copertura. Tutte le righe sono aperte.

| Requisito | Modulo | ADR | Milestone | Criterio | Test |
|---|---|---|---|---|---|
| [FR-001](requirements.md) | `config` | [ADR-001](adr/README.md) | [M04](implementation-roadmap.md) | [AC-001](acceptance-criteria.md) | [T-001](testing-strategy.md) |
| [FR-002](requirements.md) | `config` | [ADR-002](adr/README.md) | [M04](implementation-roadmap.md) | [AC-002](acceptance-criteria.md) | [T-002](testing-strategy.md) |
| [FR-003](requirements.md) | `transport` | [ADR-004](adr/README.md) | [M04](implementation-roadmap.md) | [AC-003](acceptance-criteria.md) | [T-003](testing-strategy.md) |
| [FR-004](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M05](implementation-roadmap.md) | [AC-004](acceptance-criteria.md) | [T-004](testing-strategy.md) |
| [FR-005](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M05](implementation-roadmap.md) | [AC-005](acceptance-criteria.md) | [T-005](testing-strategy.md) |
| [FR-006](requirements.md) | `raft-log` | [ADR-003](adr/README.md) | [M06](implementation-roadmap.md) | [AC-006](acceptance-criteria.md) | [T-006](testing-strategy.md) |
| [FR-007](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M07](implementation-roadmap.md) | [AC-007](acceptance-criteria.md) | [T-007](testing-strategy.md) |
| [FR-008](requirements.md) | `state-machine` | [ADR-003](adr/README.md) | [M07](implementation-roadmap.md) | [AC-008](acceptance-criteria.md) | [T-008](testing-strategy.md) |
| [FR-009](requirements.md) | `api` | [ADR-004](adr/README.md) | [M10](implementation-roadmap.md) | [AC-009](acceptance-criteria.md) | [T-009](testing-strategy.md) |
| [FR-010](requirements.md) | `state-machine` | [ADR-006](adr/README.md) | [M11](implementation-roadmap.md) | [AC-010](acceptance-criteria.md) | [T-010](testing-strategy.md) |
| [FR-011](requirements.md) | `sessions` | [ADR-007](adr/README.md) | [M11](implementation-roadmap.md) | [AC-011](acceptance-criteria.md) | [T-011](testing-strategy.md) |
| [FR-012](requirements.md) | `api` | [ADR-007](adr/README.md) | [M11](implementation-roadmap.md) | [AC-012](acceptance-criteria.md) | [T-012](testing-strategy.md) |
| [FR-013](requirements.md) | `storage` | [ADR-005](adr/README.md) | [M03](implementation-roadmap.md) | [AC-013](acceptance-criteria.md) | [T-013](testing-strategy.md) |
| [FR-014](requirements.md) | `recovery` | [ADR-005](adr/README.md) | [M09](implementation-roadmap.md) | [AC-014](acceptance-criteria.md) | [T-014](testing-strategy.md) |
| [FR-015](requirements.md) | `snapshot` | [ADR-005](adr/README.md) | [M13](implementation-roadmap.md) | [AC-015](acceptance-criteria.md) | [T-015](testing-strategy.md) |
| [FR-016](requirements.md) | `snapshot` | [ADR-005](adr/README.md) | [M13](implementation-roadmap.md) | [AC-016](acceptance-criteria.md) | [T-016](testing-strategy.md) |
| [FR-017](requirements.md) | `metrics` | [ADR-008](adr/README.md) | [M12](implementation-roadmap.md) | [AC-017](acceptance-criteria.md) | [T-017](testing-strategy.md) |
| [FR-018](requirements.md) | `node` | [ADR-005](adr/README.md) | [M09](implementation-roadmap.md) | [AC-018](acceptance-criteria.md) | [T-018](testing-strategy.md) |
| [FR-019](requirements.md) | `raft/api` | [ADR-006](adr/README.md) | [M10](implementation-roadmap.md) | [AC-019](acceptance-criteria.md) | [T-019](testing-strategy.md) |
| [FR-020](requirements.md) | `faults` | [ADR-008](adr/README.md) | [M12](implementation-roadmap.md) | [AC-020](acceptance-criteria.md) | [T-020](testing-strategy.md) |
| [NFR-001](requirements.md) | `raft` | [ADR-003](adr/README.md) | [M14](implementation-roadmap.md) | [AC-021](acceptance-criteria.md) | [T-021](testing-strategy.md) |
| [NFR-002](requirements.md) | `api` | [ADR-006](adr/README.md) | [M14](implementation-roadmap.md) | [AC-022](acceptance-criteria.md) | [T-022](testing-strategy.md) |
| [NFR-003](requirements.md) | `storage` | [ADR-005](adr/README.md) | [M14](implementation-roadmap.md) | [AC-023](acceptance-criteria.md) | [T-023](testing-strategy.md) |
| [NFR-004](requirements.md) | `raft` | [ADR-002](adr/README.md) | [M14](implementation-roadmap.md) | [AC-024](acceptance-criteria.md) | [T-024](testing-strategy.md) |
| [NFR-005](requirements.md) | `metrics` | [ADR-004](adr/README.md) | [M15](implementation-roadmap.md) | [AC-025](acceptance-criteria.md) | [T-025](testing-strategy.md) |
| [NFR-006](requirements.md) | `node/storage` | [ADR-005](adr/README.md) | [M15](implementation-roadmap.md) | [AC-026](acceptance-criteria.md) | [T-026](testing-strategy.md) |
| [NFR-007](requirements.md) | `docs` | [ADR-009](adr/README.md) | [M16](implementation-roadmap.md) | [AC-027](acceptance-criteria.md) | [T-027](testing-strategy.md) |
| [NFR-008](requirements.md) | `faults` | [ADR-008](adr/README.md) | [M12](implementation-roadmap.md) | [AC-028](acceptance-criteria.md) | [T-028](testing-strategy.md) |
| [NFR-009](requirements.md) | `build` | [ADR-001](adr/README.md) | [M04](implementation-roadmap.md) | [AC-029](acceptance-criteria.md) | [T-029](testing-strategy.md) |
| [NFR-010](requirements.md) | `config/api` | [ADR-004](adr/README.md) | [M14](implementation-roadmap.md) | [AC-030](acceptance-criteria.md) | [T-030](testing-strategy.md) |
| [NFR-011](requirements.md) | `build` | [ADR-009](adr/README.md) | [M14](implementation-roadmap.md) | [AC-031](acceptance-criteria.md) | [T-031](testing-strategy.md) |
| [NFR-012](requirements.md) | `build` | [ADR-009](adr/README.md) | [M16](implementation-roadmap.md) | [AC-032](acceptance-criteria.md) | [T-032](testing-strategy.md) |

## Copertura incrementale e audit

La milestone della matrice è il gate finale del requisito; NFR-007 si controlla a ogni cambiamento e si chiude al pacchetto M16. I sottocasi obbligatori dell'[accettazione](acceptance-criteria.md) appartengono ai test primari esistenti, senza aumentare artificialmente il numero dei requisiti. [Audit](architecture-review.md) collega problemi, correzioni e gate; un test non eseguito resta aperto.

## Avanzamento del sottoinsieme M01

T-008/M01 e T-009/M01 sono Passed per il core sequenziale locale; vedere [test, file ed evidenze](milestone-m01.md). FR-008/FR-009/FR-010 e NFR-006 hanno soltanto evidenza del componente locale: la matrice mantiene i gate finali originali per consenso, API, concorrenza e risorse complete. Seguono le evidenze locali M02.

## Avanzamento del sottoinsieme M02

M02 COMPLETED: T-008/M02 (ownership singola ordinata, scheduling senza duplicati, coda limitata/backpressure) e T-018/M02 (shutdown/cancellazioni worker) Passed; presenti le fondamenta fake T-028. [Evidenze e limiti](milestone-m02.md). FR-008/FR-018 e NFR-008 hanno soltanto evidenza del sottoinsieme locale. La matrice primaria mantiene AC-008 completo a M07, AC-018 a M09 e AC-028 a M12; nessuno è chiuso anticipatamente.

## Evidenze gate storage prima di M03

C-01/OQ-05: [schemi fissati e vettori statici](canonical-formats.md), prerequisito progettuale risolto; conformità futura T-013/T-015/T-016 ancora richiesta. C-02/OQ-01: [candidato documentato ed evidenze mancanti](macos-storage-contract.md), scelta condizionata, validazione implementativa aperta. La [revisione](pre-m03-review.md) collega FR-013–FR-016 e NFR-003/NFR-006 alle decisioni; conservati gate finali originali della matrice e tutti gli AC completi.

## Evidenze successive M03 preliminare

[Relazione di fase](milestone-m03-preliminary.md): GATE C-02 VERIFIED per primitive locali, M03 PARTIALLY COMPLETED. Passano 7 test storageformat/18 sottocasi e 13 funzioni test storagefs/16 sottocasi; superati race mirato e regressioni M01/M02 invariate. Ambito codec: frame/entry/digest/manifest senza stato; nessun motore WAL/replay o chiusura completa AC-013/AC-014/AC-023. Gate snapshot e successivi aperti. Cifre/stati precedenti descrivono la fase storica pertinente.

## Chiusura M03 locale

[Evidenze finali](milestone-m03.md): M03 COMPLETED; T-013/parte storage AC-013 locali Passed. WAL con 17 funzioni principali/25 sottocasi, 82,9% statement coverage locale; passano regressioni M01/M02, codec/adattatore e race mirato WAL/adattatore/applier. Tutte le 18 aspettative recovery canoniche coperte al confine byte/namespace pertinente. Integrazione risposte Raft FR-013 completa, restart server AC-014 e AC-023 end-to-end dipendono da milestone successive; accettazione snapshot AC-015/AC-016 a M13. Copertura/race/test processo non certificano resilienza a guasti fisici. Nessun ID requisito/AC primario o gate distribuito finale rimosso.

## Mappatura implementazione M04

FR-001 → internal/node + identità/LOCK wal → AC-001/T-001. FR-002 → internal/config + configuration-v1.json + envelope peer → AC-002/T-002. FR-003 → internal/strictjson + peerprotocol + transport + peer-v1.json → AC-003/T-003. NFR-009 → build darwin/arm64 nativa e suite limitata → AC-029/T-029. Le [evidenze M04](milestone-m04.md) registrano risultati/limiti; nessun requisito successivo chiuso da questi test.

## Mappatura implementazione M05

FR-004 → internal/raft Core + barriera M03 Append/Durable → AC-004/T-004. FR-005 → Tick/Draw iniettati e AppendEntries vuote + Service/trasporto → AC-005/T-005. Evidenze elettorali INV-01/INV-05/INV-06 → oracolo indipendente voto persistente/quorum/leader; replay finito NFR-008 → election-v1-results.json. La [relazione M05](milestone-m05.md) conserva gate safety completa successivo NFR-001 e tutti gli obblighi M06+; nessun requisito app/client chiuso.

## Mappatura implementazione M06

FR-006 → internal/raft/replication.go + replace_suffix WAL esistente + trasporto/servizio → AC-006/T-006. Sent/Failed/Response e progresso copiato stabiliscono correlazione richiesta effettiva; tagli frame WAL e oracolo vista disco con seed coprono conservazione prefisso noto/recovery. Le [evidenze M06](milestone-m06.md) conservano tutti i gate FR-007/FR-008 successivi: nessun commit quorum, avanzamento checkpoint o apply.

## Mappatura implementazione M07

FR-007 → internal/raft/commit.go + correlazione prove M06 + checkpoint M03 → AC-007/T-007. FR-008 → cursori core + application.go + internal/committed + M02/M01 invariati → AC-008/T-008. C-03 M07 → backlog bloccato e oracolo ghost permanente. Le [evidenze M07](milestone-m07.md) conservano gate M08 API/OpenAPI, M09 recovery server completo, M10 read, M11 sessioni/dedup, M13 snapshot e NFR-001/storie finali.

## Evidenze di fase M08

[Relazione M08](milestone-m08.md): FR-009/FR-012, AC-009/AC-012 e T-009/T-012 superati solo per sottoinsieme roadmap PUT/DELETE, validazione HTTP rigorosa, deadline, unknown/not_admitted e OpenAPI. Test includono cancellazione con disco/apply bloccati, perdita osservata a termine maggiore, ammissione limitata con capacità peer separata, tre nodi reali con scritture concorrenti/elezione/minoranza, risposta persa senza retry automatico e DELETE ripetuta con nuovo risultato. Checker schema/fixture HTTP include controlli negativi ed estensioni canoniche runtime. Regressioni M01–M07 e race mirato superati. GET AC-009 completa resta M10; retry dedup/failover AC-012 resta M11, con snapshot/checker storie ai gate successivi. Obbligo artefatto/fixture sottoinsieme M08 C-01 soddisfatto; nessuna stabilità v1 completa dichiarata. C-02 e obblighi successivi C-03 mantengono limiti originali.

## Evidenze MVP interno M09

[M09](milestone-m09.md) soddisfa FR-014/AC-014/T-014 e FR-018/AC-018/T-018, più sottoinsieme roadmap NFR-003/AC-023/T-023. Nuovo test startup controllato verifica replay solo follower a commit 1 con suffisso 2, poi applicazione legale noop nuovo termine. Test reali processi figli uccidono un follower, leader, tutti, riavviano storage integro, confrontano byte confermati/valori realmente ricostruiti e riprendono PUT/DELETE. SIGTERM scrittura pendente verifica unknown, esclusione suffisso e rilascio risorse. Test CLI/errori coprono input limitato/redatto, inizializzazione mancante, lock duplicato, bootstrap ripetuto, corruzione, cancellazione prima di avvio e coerenza configurazioni esempio. Rieseguite regressioni M01–M08 e race. Test usano un cluster temporaneo a tre nodi per volta, attese limitate e dati piccoli; nessun benchmark o guasto fisico. Variante GET HTTP AC-023 resta M10, snapshot M13, checker completo M14. OpenAPI/fixture M08 e gate successivi C-01/C-02/C-03 invariati.
