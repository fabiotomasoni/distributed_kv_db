# Rischi e trade-off

[Indice](README.md) · [English](../en/risks-and-tradeoffs.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Scelte e alternative

| Confronto | Scelta iniziale e ragione | Costo accettato / condizione di revisione |
|---|---|---|
| Go / Rust / C++ | Go: concorrenza e rete nella standard library, ciclo di apprendimento sostenibile. | GC e RSS variabile; non si dimostra superiorità prestazionale. |
| HTTP/JSON / RPC binario | HTTP/JSON per ispezione e assenza di generatori. | Parsing/Base64 e payload maggiori; cambiare solo con misure e schema versionato. |
| RAM / WAL | Mappa RAM più WAL sincrono e snapshot. | Storage più complesso, ma RAM-only non soddisfa crash recovery. |
| Semplicità / prestazioni | Event loop e una RPC per peer. | Throughput limitato; no pipelining finché safety non è verificata. |
| Consistenza / disponibilità | Quorum per ogni operazione, GET nel log. | Minoranza indisponibile e più I/O; nessun fallback stale implicito. |
| Sync / async | Sync prima di ack. | Latenza SSD; async cambierebbe contratto di durabilità. |
| Replica completa / sharding | Tre copie complete, dataset piccolo. | Costo triplo di spazio, compensato da un solo gruppo. |
| Processi locali / nodi fisici | Tre processi sul Mac già disponibile. | Correlated failure del computer; nessun claim HA fisico. |
| Test deterministici / casuali | Simulatore deterministico più seed limitati e processi reali. | Simulatore può modellare male la realtà; integrazione necessaria. |

## Registro rischi

Probabilità e impatto sono stime qualitative di progetto. Il responsabile iniziale è lo sviluppatore individuale, che rivede il registro al gate indicato.

| ID | Rischio | Probabilità / impatto | Mitigazione | Milestone |
|---|---|---|---|---|
| R-01 | Implementazione errata Raft | Alta / critico | Core piccolo, INV-01–06 dopo ogni evento, revisione casi vecchi termini. | M05–M07/M14 |
| R-02 | Ack prima di Sync | Media / critico | Effetti persistenti espliciti e cutpoint; nodo fatal su errore. | M03/M09 |
| R-03 | Read stale da leader isolato | Alta / critico | GET nel log, test partizione e apply ritardato. | M10/M14 |
| R-04 | Dedup persa o ripulita troppo presto | Media / alto | Nessun TTL/eviction, snapshot include sessioni, sequenza monotona. | M11/M13 |
| R-05 | Crash durante snapshot | Alta / critico | Generazioni, CURRENT atomico, picco spazio prenotato, cutpoint. | M13 |
| R-06 | Carico/SSD eccessivi | Media / medio | Quote, benchmark brevi, log ruotati, no loop di stress. | M12/M15 |
| R-07 | Ritardo calendario | Alta / alto | Gate piccoli, rimuovere opzionali, estendere 24 settimane se necessario. | Tutte |
| R-08 | Complessità anticipata | Alta / alto | Nessuna membership dinamica, container o ottimizzazione non misurata. | M01–M16 |
| R-09 | Traduzioni divergenti | Media / alto | Coppie file e ID identici, revisione nello stesso cambiamento. | Tutte |
| R-10 | Filesystem reale diverso dal modello | Media / critico | Test adapter macOS, garanzia power loss aperta, non pubblicizzare prove assenti. | M03/M13 |
| R-11 | Checker sbagliato o esplosione combinatoria | Media / alto | Modello indipendente, fixture impossibili note, storie <=12 op, inconclusive esplicito. | M14 |
| R-12 | Cap sessioni raggiunto | Media / medio | Client ID riusati correttamente, cap documentato, reset solo nuovo cluster esplicito. | M11 |

## Gestione delle incertezze

Gli [ADR](adr/README.md) sono Proposed finché i rispettivi gate non portano evidenze sufficienti. Accettare un rischio non trasforma un obiettivo in garanzia. Cambiare limite replicato richiede configurazione uniforme e piano di compatibilità; cambiarlo su un solo nodo rompe determinismo. Se una mitigazione richiede spese, scegliere una mitigazione locale o ridurre l'ambito: il budget resta 0 €.
