# Documentazione ingegneristica

[Indice](README.md) · [English](../en/README.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Come leggere il progetto

QuorumKV è nella fase di progettazione. Tutti i server, comandi, test e risultati descritti sono futuri salvo i controlli documentali esplicitamente registrati. Non eseguire gli esempi di avvio come se il software fosse disponibile.

Percorso rapido: panoramica → requisiti → architettura → API. Revisione safety: Raft → consistenza → storage → guasti → test. Pianificazione: roadmap → criteri → matrice. La prima implementazione consigliata è M01; richiede approvazione separata della documentazione.

## Specifiche complete

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
- [Decisioni architetturali: nove ADR](adr/README.md)
- [Nove diagrammi Mermaid](diagrams/README.md)

## Manutenzione

Modificare entrambe le lingue nello stesso cambiamento. Stessi nomi file, ID, formule ed esempi tecnici; non riusare ID ritirati. Requisiti e criteri sono normativi; diagrammi ed esempi sono illustrativi. Una decisione modificata aggiorna ADR, API, test e matrice prima di cambiare le garanzie pubblicizzate.
