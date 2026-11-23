# QuorumKV — Documentation / Documentazione

**M01–M12 COMPLETED — 23 November / novembre 2026. M13 PARTIALLY COMPLETED: A–E verified within their scope / A–E verificate nel rispettivo perimetro. Runtime InstallSnapshot, compaction and GC remain unactivated / non attivati.**

[Italiano: documentazione completa](it/README.md) · [English: complete documentation](en/README.md)

Due percorsi equivalenti, con specifiche, requisiti, ADR, diagrammi, test previsti e roadmap. Sono implementati la macchina a stati locale, l’applier ordinato, WAL/recovery log, RPC/configurazione, elezione leader, replica log, commit e applicazione ordinata; sono disponibili PUT/DELETE e GET linearizzabile; CAS HTTP/deduplicazione richiedono il nuovo profilo sessioni v2.

Two equivalent paths with specifications, requirements, ADRs, diagrams, planned tests and a roadmap. The local state machine, ordered applier, WAL/log recovery, RPC/configuration, leader election, log replication, commit and ordered application are implemented; PUT/DELETE and linearizable GET are available; CAS HTTP/deduplication require the new v2 session profile.

[README del progetto / Project README](../README.md)

[M01: relazione italiana](it/milestone-m01.md) · [M01: English report](en/milestone-m01.md)

[M02: relazione italiana](it/milestone-m02.md) · [M02: English report](en/milestone-m02.md)

[Revisione pre-M03: CONDITIONALLY READY](it/pre-m03-review.md) · [Pre-M03 review: CONDITIONALLY READY](en/pre-m03-review.md)

[M03 preliminare: C-02 VERIFIED, milestone parziale](it/milestone-m03-preliminary.md) · [Preliminary M03: C-02 VERIFIED, milestone partial](en/milestone-m03-preliminary.md)

[M03 completata: WAL/recovery](it/milestone-m03.md) · [M03 completed: WAL/recovery](en/milestone-m03.md)

[M04 completata: RPC/configurazione](it/milestone-m04.md) · [M04 completed: RPC/configuration](en/milestone-m04.md)

[M05 completata: leader election](it/milestone-m05.md) · [M05 completed: leader election](en/milestone-m05.md)

[M06 completata: replica, conflitti e revisione preliminare](it/milestone-m06.md) · [M06 completed: replication, conflicts and preliminary review](en/milestone-m06.md)

[M07 completata: commit e applicazione](it/milestone-m07.md) · [M07 completed: commit and application](en/milestone-m07.md)

[M11 completata: CAS e sessioni](it/milestone-m11.md) · [M11 completed: CAS and sessions](en/milestone-m11.md)

[M12 completata: fault/replay e diagnostica](it/milestone-m12.md) · [M12 completed: fault/replay and diagnostics](en/milestone-m12.md)

[Revisione pre-M13: CONDITIONALLY READY](it/pre-m13-review.md) · [Pre-M13 review: CONDITIONALLY READY](en/pre-m13-review.md)

[M13 Fase E: InstallSnapshot isolato](it/milestone-m13-installsnapshot-preliminary.md) · [M13 Phase E: isolated InstallSnapshot](en/milestone-m13-installsnapshot-preliminary.md)
