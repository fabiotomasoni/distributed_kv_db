# Documentazione ingegneristica

[Indice](README.md) · [English](../en/README.md)

**Aggiornamento: 22 novembre 2026 — M01/M02/M03/M04/M05/M06/M07/M08/M09/M10/M11 locali implementate e verificate; le specifiche distribuite restano progettuali.**

## Come leggere il progetto

M01 è implementata e verificata: [macchina a stati locale e risultati](milestone-m01.md). M02 è completata: [applier ordinato, lifecycle e test](milestone-m02.md). M03 è completata al confine WAL/recovery log locale: [risultati](milestone-m03.md). M04 è completata: [evidenze RPC/configurazione](milestone-m04.md). M05 è completata: [evidenze leader election](milestone-m05.md). M06 è completata: [evidenze replica](milestone-m06.md). M07 è completata: [evidenze commit/applicazione](milestone-m07.md). M08 è completata: [API PUT/DELETE e test client](milestone-m08.md). M09 è completata: [recovery processi/MVP interno](milestone-m09.md). M10 è completata: [evidenze GET nel log](milestone-m10.md). M11 è completata nel profilo v2 esplicito: [evidenze CAS/dedup](milestone-m11.md). M12 e successive restano da implementare. Specifiche client distribuite complete restano proposte; avvio MVP eseguibile nella guida sviluppo; i rapporti datati 18 novembre documentano la fase precedente.

Percorso rapido: panoramica → requisiti → architettura → API. Revisione safety: Raft → consistenza → storage → guasti → test. Pianificazione: roadmap → criteri → matrice. M01/M02/M03/M04/M05/M06/M07/M08/M09/M10/M11 completate localmente; M12 richiede un nuovo incarico e i propri gate.

## Specifiche complete

- [M11 completata: CAS HTTP e sessioni replicate limitate](milestone-m11.md)
- [M10 completata: GET nel log ed evidenze vecchio leader](milestone-m10.md)
- [M09 completata: recovery processi e MVP interno](milestone-m09.md)
- [M08 completata: PUT/DELETE, incertezza e OpenAPI](milestone-m08.md)
- [M07 completata: commit termine corrente, applicazione e replay](milestone-m07.md)

- [M06 completata: replica, conflitti e perimetro risolto](milestone-m06.md)

- [M05 completata: elezione persistente e simulatore](milestone-m05.md)

- [M04 completata: RPC/configurazione ed evidenze](milestone-m04.md)
- [Configurazione statica v1 e hash canonico](configuration.md)

- [M03 completata: WAL locale, recovery ed evidenze](milestone-m03.md)

- [M03 preliminare: C-02 VERIFIED, milestone parziale](milestone-m03-preliminary.md)

- [Revisione pre-M03: CONDITIONALLY READY](pre-m03-review.md)
- [Formati storage canonici v1 e fixture](canonical-formats.md)
- [Contratto storage macOS e test C-02 pendenti](macos-storage-contract.md)

- [M01 completata: interfacce, test ed evidenze](milestone-m01.md)
- [M02 completata: applier ordinato, code e shutdown](milestone-m02.md)
- [Audit architetturale: APPROVED WITH CONDITIONS](architecture-review.md)
- [Panoramica di QuorumKV](overview.md)
- [Ambito, assunzioni e limiti](scope.md)
- [Requisiti verificabili](requirements.md)
- [Matrice di tracciabilità](traceability.md)
- [Architettura e concorrenza](architecture.md)
- [Modello dei dati e transizioni](data-model.md)
- [Specifica API client v1](api-specification.md)
- [Protocollo peer e trasporto](network-protocol.md)
- [Specifica del core Raft](raft-consensus.md)
- [Consistenza e linearizzabilità](consistency-model.md)
- [Persistenza, snapshot e crash recovery](storage-and-recovery.md)
- [Modello e scenari di guasto](failure-model.md)
- [Strategia e catalogo dei test](testing-strategy.md)
- [Criteri oggettivi di accettazione](acceptance-criteria.md)
- [Fault injection riproducibile](fault-injection.md)
- [Log, metriche e diagnosi](observability.md)
- [Prestazioni e protezione delle risorse](performance-and-resources.md)
- [Modello di sicurezza locale](security.md)
- [Guida al futuro sviluppo](development-guide.md)
- [Roadmap di implementazione](implementation-roadmap.md)
- [Rischi e trade-off](risks-and-tradeoffs.md)
- [Demo tecnica e valutazione](demo-and-evaluation.md)
- [Glossario bilingue](glossary.md)
- [Fonti primarie e stato della verifica](references.md)
- [Revisione progettuale e questioni aperte](design-review.md)
- [Relazione di consegna e inventario](delivery-report.md)
- [Decisioni architetturali: dieci ADR](adr/README.md)
- [Nove diagrammi Mermaid](diagrams/README.md)

## Manutenzione

Modificare entrambe le lingue nello stesso cambiamento. Stessi nomi file, ID, formule ed esempi tecnici; non riusare ID ritirati. Requisiti e criteri sono normativi; diagrammi ed esempi sono illustrativi. Una decisione modificata aggiorna ADR, API, test e matrice prima di cambiare le garanzie pubblicizzate.
