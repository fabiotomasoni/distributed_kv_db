# Relazione di consegna e inventario

[Indice](README.md) · [English](../en/delivery-report.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Risultato iniziale 0.1 e architettura

Creati **96 file Markdown**: README principale bilingue, indice docs bilingue e **47 file per lingua**. Le due versioni contengono specifiche complete, 32 requisiti, 32 criteri di accettazione, 32 test pianificati, 16 scenari di guasto, 16 milestone, nove esperimenti demo, nove ADR e nove diagrammi dettagliati per lingua. Gli indici e i rapporti fanno parte dei 47 file.

Baseline: un gruppo statico di tre processi Go su loopback, HTTP/JSON, core Raft a eventi, macchina a stati deterministica, WAL sincrono, checkpoint commit locale, snapshot generazionali e sessioni client senza eviction. Quorum 2/3, GET ordinata nel log, CAS atomico su valore completo. Budget aggiuntivo 0 €, runtime offline dopo bootstrap strumenti, cap risorse espliciti.

## Decisioni, rischi e questioni aperte

Le nove decisioni sono nel [registro ADR](adr/README.md), tutte Proposed. I rischi principali sono errori Raft, acknowledgment anticipati, snapshot incompleti, checker errato e divergenze linguistiche: [registro rischi](risks-and-tradeoffs.md). Le sette questioni aperte e i relativi gate sono nella [revisione](design-review.md); includono sync macOS, power loss, baseline risorse, licenza e fixture di formato.

## Verifiche e limiti

Verificati inventario e parità linguistica, link relativi, riferimenti agli identificatori, copertura della matrice, JSON illustrativi e delimitatori fenced. Revisionati manualmente semantica Raft, API, dedup, snapshot, fault, quote e diagrammi. Le fonti consultate sono in [references](references.md). Mermaid non renderizzato; nessun test del database eseguito e nessuna misura prestazionale inventata. Le traduzioni sono state controllate semanticamente, senza una certificazione esterna.

**Non è stato scritto codice applicativo.** Nessun file Go, modulo, CLI, servizio o dipendenza runtime è stato creato; nessun deploy, installazione pesante o benchmark. Il file .DS_Store preesistente non è stato modificato. Gli strumenti temporanei di authoring/verifica sono fuori dal repository e non fanno parte del database.

## Prima milestone consigliata

Dopo approvazione separata della documentazione: M01, macchina a stati KV e modello sequenziale con fixture di semantica e quote. Non iniziare automaticamente Raft, networking o infrastruttura. Vedere [roadmap](implementation-roadmap.md).

## Aggiornamento dopo audit 0.2

Inventario attuale **98 file Markdown**, 48 per lingua e due comuni; aggiunto [architecture-review.md](architecture-review.md). Il giudizio tecnico è **APPROVED WITH CONDITIONS**: solo M01–M02 ora, gate espliciti prima di storage e cluster. La relazione di audit elenca tutte le modifiche e i controlli attuali; i numeri della prima consegna descrivono la baseline storica.

## Inventario completo

File comuni: [README principale](../../README.md), [indice documentazione](../README.md). Ogni file elencato sotto esiste con stesso percorso nella lingua inglese, raggiungibile dal link in testa al documento.

- [README.md](README.md)
- [acceptance-criteria.md](acceptance-criteria.md)
- [adr/ADR-001-use-go.md](adr/ADR-001-use-go.md)
- [adr/ADR-002-static-three-node-cluster.md](adr/ADR-002-static-three-node-cluster.md)
- [adr/ADR-003-raft-consensus.md](adr/ADR-003-raft-consensus.md)
- [adr/ADR-004-network-transport.md](adr/ADR-004-network-transport.md)
- [adr/ADR-005-persistence-strategy.md](adr/ADR-005-persistence-strategy.md)
- [adr/ADR-006-linearizable-reads.md](adr/ADR-006-linearizable-reads.md)
- [adr/ADR-007-client-retry-semantics.md](adr/ADR-007-client-retry-semantics.md)
- [adr/ADR-008-fault-injection.md](adr/ADR-008-fault-injection.md)
- [adr/ADR-009-offline-first-development.md](adr/ADR-009-offline-first-development.md)
- [adr/README.md](adr/README.md)
- [api-specification.md](api-specification.md)
- [architecture.md](architecture.md)
- [architecture-review.md](architecture-review.md)
- [consistency-model.md](consistency-model.md)
- [data-model.md](data-model.md)
- [delivery-report.md](delivery-report.md)
- [demo-and-evaluation.md](demo-and-evaluation.md)
- [design-review.md](design-review.md)
- [development-guide.md](development-guide.md)
- [diagrams/01-system.md](diagrams/01-system.md)
- [diagrams/02-node.md](diagrams/02-node.md)
- [diagrams/03-write.md](diagrams/03-write.md)
- [diagrams/04-read.md](diagrams/04-read.md)
- [diagrams/05-election.md](diagrams/05-election.md)
- [diagrams/06-replication.md](diagrams/06-replication.md)
- [diagrams/07-recovery.md](diagrams/07-recovery.md)
- [diagrams/08-partition.md](diagrams/08-partition.md)
- [diagrams/09-snapshot.md](diagrams/09-snapshot.md)
- [diagrams/README.md](diagrams/README.md)
- [failure-model.md](failure-model.md)
- [fault-injection.md](fault-injection.md)
- [glossary.md](glossary.md)
- [implementation-roadmap.md](implementation-roadmap.md)
- [network-protocol.md](network-protocol.md)
- [observability.md](observability.md)
- [overview.md](overview.md)
- [performance-and-resources.md](performance-and-resources.md)
- [raft-consensus.md](raft-consensus.md)
- [references.md](references.md)
- [requirements.md](requirements.md)
- [risks-and-tradeoffs.md](risks-and-tradeoffs.md)
- [scope.md](scope.md)
- [security.md](security.md)
- [storage-and-recovery.md](storage-and-recovery.md)
- [testing-strategy.md](testing-strategy.md)
- [traceability.md](traceability.md)
