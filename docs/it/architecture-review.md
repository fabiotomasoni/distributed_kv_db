# Audit architetturale e validazione pre-implementazione

[Indice](README.md) · [English](../en/architecture-review.md)

**Audit 0.2 — 18 novembre 2026. Solo documentazione; nessun codice applicativo o risultato del database.**

## A. Executive summary

**Esito: APPROVED WITH CONDITIONS.** Le specifiche corrette consentono tecnicamente M01 (macchina a stati deterministica) e M02 (ownership, scheduling e harness isolato). Non autorizzano l'avvio indiscriminato di storage durevole, cluster o ottimizzazioni. Le condizioni prima delle fasi successive sono in E e G; questa consegna non inizia alcuna milestone.

Esaminati i **96 documenti Markdown preesistenti**: due indici comuni e 47 coppie linguistiche, incluse specifiche, tutti gli ADR e i diagrammi. Analisi semantica dei protocolli e delle dipendenze, confronto delle versioni e controllo strutturale dell'intero inventario. I due rapporti di audit sono nuovi: totale attuale **98 file**, 48 per lingua più due comuni. L'esame non si limita ai controlli dichiarati nella consegna precedente.

Individuati **11 problemi**: **0 CRITICAL, 6 HIGH, 4 MEDIUM, 1 LOW**. Tutti hanno una correzione documentale applicata. “Corretto” significa contratto chiarito o difetto rimosso dai documenti; non significa bug software risolto o test applicativo passato. Non ho trovato una regola base di voto/commit che prescriva direttamente la violazione di Raft; questo non prova l'assenza di ogni possibile errore.

Il disegno statico 2/3, il filtro elettorale per ultimo termine/indice, la persistenza prima di ack, il commit tramite termine corrente e la lettura nel log sono difendibili nel modello crash non bizantino. Le lacune più pericolose erano ai confini fra componenti asincroni, snapshot e test. La qualità editoriale non era sufficiente a renderle visibili.

## B. Problemi individuati

Gravità riferita alla baseline 0.1. Tutti gli stati sottostanti sono **Corrected in documentation**; verifica implementativa **Not run**. I documenti indicati esistono in entrambe le lingue; i collegamenti aprono la lingua corrente.

### AU-001 — HIGH

**Documento e sezione:** [raft-consensus.md](raft-consensus.md), “Commit e apply”.

**Problema, scenario e conseguenze:** Il cursore lastApplied era usato anche per accodare lavori asincroni. Commit 5 seguito da commit 7 prima dei completamenti accodava 1..5 e poi 1..7. Possibili assert falliti o riapplicazioni; il solo ordine della coda non basta.

**Correzione applicata:** Introdotti scheduledApplyIndex, capacità disponibile ed epoch apply, con inizializzazione dopo recovery e barriera su snapshot. Aggiornati architettura, pseudocodice, diagramma replica, ADR-003 e T-008/apply-backlog.

**Tracciabilità e riferimenti:** FR-008 / AC-008 / T-008; [REF-01 figura 2 / Figure 2](references.md).

### AU-002 — HIGH

**Documento e sezione:** [api-specification.md](api-specification.md), “Contratto HTTP proposto”.

**Problema, scenario e conseguenze:** CAS ammetteva expected e newValue raw da 16 KiB ciascuno, ma il corpo HTTP era limitato a 32 KiB. La sola coppia Base64 richiede 43 696 byte; la richiesta compatta completa verificata richiede 44 116 byte. Un CAS valido nel modello era sempre rifiutato dal trasporto a dimensione massima.

**Correzione applicata:** Cap client portato a 48 KiB (49 152 byte), mantenendo valore 16 KiB e entry 48 KiB; aggiornati sicurezza, ADR-004, test e criteri. Whitespace eccessivo resta soggetto al cap.

**Tracciabilità e riferimenti:** FR-009 / AC-009 / T-009; [Calcolo su specifica locale / local-spec calculation](references.md).

### AU-003 — HIGH

**Documento e sezione:** [storage-and-recovery.md](storage-and-recovery.md), “Snapshot e pubblicazione atomica”.

**Problema, scenario e conseguenze:** La rotazione finale indicava serializzazione e conservazione del voto, senza definire il confine operativo. Un implementatore poteva copiare hard state catturato a S, perdere un voto sincronizzato successivo o accettare un completamento con sequence riusata nella nuova generazione. InstallSnapshot non fissava la rivalidazione dopo il drain di apply.

**Correzione applicata:** Definiti barriera finale, drain persistenze, hard state corrente, token generation_id/sequence, doppia barriera applier/storage e pubblicazione prima dello switch RAM. Ack finale snapshot distinto da ack chunk; T-015/T-016 e ADR-005 aggiornati. Conservare un voto vecchio non è una scelta ammessa.

**Tracciabilità e riferimenti:** FR-013 / FR-015 / FR-016 / T-013 / T-015 / T-016; [REF-01 sez. 7 / section 7; REF-08/09](references.md).

### AU-004 — HIGH

**Documento e sezione:** [testing-strategy.md](testing-strategy.md), “Storie e checker di linearizzabilità”.

**Problema, scenario e conseguenze:** “Prima invocazione alla risposta definitiva” non precisava quale risposta dopo retry multipli, né il trattamento di payload diversi con stessa identità. Allungare una PUT già completata fino a un retry successivo può far accettare una GET stale che sarebbe impossibile nell’ordine reale.

**Correzione applicata:** Intervallo chiuso alla prima risposta definitiva osservata; retry successivi non lo estendono. Raggruppamento solo a comando canonico identico; errori sessione verificati a parte. Aggiunto controesempio noto da rifiutare, senza dichiarare un checker già esistente.

**Tracciabilità e riferimenti:** NFR-002 / AC-022 / T-022; FR-011 / T-011; [REF-05; REF-01 sez. 8 / section 8](references.md).

### AU-005 — HIGH

**Documento e sezione:** [fault-injection.md](fault-injection.md), “Storage e crash controllati”.

**Problema, scenario e conseguenze:** Conservare soltanto l’ultimo Sync non rappresenta un crash del solo processo: write e rename completati restano visibili nel kernel. Il simulatore avrebbe ignorato frame interi non Sync e CURRENT già rinominato, con copertura insufficiente del recovery reale.

**Correzione applicata:** Separati ProcessCrash e SystemCrashModel; sopravvivenza di I/O incompleti/non sincronizzati esplicita e registrata. T-013/T-016 ampliano cutpoint; T-023 resta verifica di processo. Nessun risultato del modello equivale a power loss APFS.

**Tracciabilità e riferimenti:** FR-013 / FR-016 / FR-020 / T-013 / T-016 / T-020; [REF-08/09](references.md).

### AU-006 — HIGH

**Documento e sezione:** [implementation-roadmap.md](implementation-roadmap.md), “Capacità, tabella milestone e gate”.

**Problema, scenario e conseguenze:** M01/M02 chiedevano AC completi con cluster/shutdown non ancora presenti; M08 chiedeva retry deduplicati di M11 e GET di M10. I test durabilità M09 e risorse M13 includevano parti disponibili solo dopo. Era impossibile soddisfare letteralmente tutti i gate nell’ordine previsto.

**Correzione applicata:** Definiti sottoinsiemi precoci, mantenendo AC completi aperti fino alle dipendenze. Anticipate fondamenta del simulatore a M02–M05, non tutto a M12; integrazione offline separata dalla demo. NFR-007 ha gate finale M16. Le 144–192 ore indicano capacità, non sforzo dimostrato.

**Tracciabilità e riferimenti:** NFR-007 / NFR-008 / NFR-011 / T-027 / T-028 / T-031; [Dipendenze interne della roadmap / internal roadmap dependencies](references.md).

### AU-007 — MEDIUM

**Documento e sezione:** [performance-and-resources.md](performance-and-resources.md), “Profilo iniziale e picco compaction”.

**Problema, scenario e conseguenze:** L’esempio 4×8 MiB saturava già il cap nodo 32 MiB, omettendo file di controllo. Il target di failover <=3 s non esplicitava risorse e quote disponibili; una coda/disco saturo può impedire progresso senza violare Raft.

**Correzione applicata:** Prenotazione su dimensioni reali e crescita, riserva controllo almeno 64 KiB dentro il cap; rifiuto prima di allocare, nessuna cancellazione di dati attivi. Separati byte logici e allocati APFS; ipotesi del target ripresa esplicitate nei requisiti e AC.

**Tracciabilità e riferimenti:** NFR-004 / NFR-006 / AC-024 / AC-026 / T-026; [Aritmetica dei limiti locali / local-limit arithmetic](references.md).

### AU-008 — MEDIUM

**Documento e sezione:** [consistency-model.md](consistency-model.md), “Protocollo GET e partizione”.

**Problema, scenario e conseguenze:** “Leader isolato non completa letture” era troppo generale per operazioni già committed prima del guasto. Vietare una risposta tardiva lecita causerebbe falsi fallimenti nei test; il caso di nuove richieste dopo isolamento deve essere distinto.

**Correzione applicata:** T-019/D-06 stabiliscono prima isolamento e scarto dei messaggi rilevanti, poi invocano GET. Un sottocaso consente risposta a barriera già committed. Precisati punti logici di linearizzazione rispetto ad apply locali e failover; ADR-006 aggiornato.

**Tracciabilità e riferimenti:** FR-019 / AC-019 / T-019; [REF-05; REF-01 sez. 8 / section 8](references.md).

### AU-009 — MEDIUM

**Documento e sezione:** [storage-and-recovery.md](storage-and-recovery.md), “Contratto e assunzioni”.

**Problema, scenario e conseguenze:** Crash processo e power loss erano distinti, ma il crash del sistema operativo non aveva un contratto autonomo. Un lettore poteva trasferire impropriamente risultati di kill del processo al reboot o alla persistenza hardware.

**Correzione applicata:** Tabella a tre livelli: obiettivo v1 process crash, OS crash subordinato al contratto piattaforma e non validato, power loss fuori garanzia. Conservate Sync necessarie; riduzione I/O mediante assenza di riscritture/Sync inutili, batching causale e futuro ReadIndex.

**Tracciabilità e riferimenti:** NFR-003 / AC-023 / T-023; [REF-08/09](references.md).

### AU-010 — MEDIUM

**Documento e sezione:** [network-protocol.md](network-protocol.md), “Validazione input JSON”.

**Problema, scenario e conseguenze:** Il contratto era rigoroso ma non chiariva che encoding/json v1 accetta duplicate key e sostituisce UTF-8/surrogate non validi; il decode in struct può anche accettare nomi con case diverso. Un adattatore ingenuo non avrebbe rispettato la specifica.

**Correzione applicata:** Richiesta scansione token/UTF-8, nomi esatti, duplicate key dopo unescape, singolo documento, profondità e numeri canonici, con fixture T-003/T-009. Standard library mantenuta, nessuna dipendenza introdotta; aggiunta REF-13.

**Tracciabilità e riferimenti:** FR-003 / FR-009 / NFR-010 / T-003 / T-009; [REF-13](references.md).

### AU-011 — LOW

**Documento e sezione:** [failure-model.md](failure-model.md), “Tabella scenari, riga F-08”.

**Problema, scenario e conseguenze:** La barra non escapata in “1|2” creava una colonna Markdown aggiuntiva, spostando disponibilità, invarianti e test. Il precedente controllo link non rilevava il difetto.

**Correzione applicata:** Sostituito con “1 contro 2” / “1 versus 2”; resa monolingue la cella condizionale e chiarito che i nodi condividono lo storage fisico. Aggiunto controllo di ampiezza tabelle alla verifica documentale.

**Tracciabilità e riferimenti:** NFR-007 / AC-027 / T-027; [Sintassi Markdown locale / local Markdown syntax](references.md).

## C. Decisioni rivalutate e controllo delle proprietà

**Letture:** confermata A, GET nel log, per la prima versione; tabella comparativa e percorso B ReadIndex in [consistenza](consistency-model.md) e ADR-006. A riusa replica/commit/apply ma genera WAL e checkpoint; B evita entry per lettura ma richiede commit corrente, round fresco iniziato dopo la richiesta, correlazione per termine/generazione e attesa apply. Entrambe richiedono quorum per nuove letture; nessuna lease è assunta. Non attivare ReadIndex prima di gate safety e misure del costo effettivo. Non è una correzione necessaria di safety alla baseline A.

**WAL e ack:** formato transazionale con checksum confermato. La write non è assunta atomicamente persistente: recovery verifica frame completo prima di applicarne gli effetti. Voto/termine e replica positiva richiedono Sync riuscita; successo client richiede commit legale, maggioranza durevole e apply. Il checkpoint commit sincrono resta una scelta prudenziale, più costosa del minimo Raft. Nessuna Sync necessaria è stata rimossa.

**Retry:** sessioni replicate, sequenza monotona, ultimo risultato e digest negli snapshot; corretti i limiti del checker, non introdotte cache follower o TTL. Timeout resta unknown. Vecchie sequenze non si riapplicano e il risultato storico oltre l'ultimo non è garantito. Il client deve conservare l'identità pendente; cambiare ID crea una diversa operazione logica.

**Recovery:** follower dopo restart, replay solo fino a checkpoint commit provato, nessun commit dedotto dalla lunghezza log. Un suffisso completo non committed può sopravvivere e poi essere rimosso. La generazione installata conserva hard state aggiornato e sessioni; corruzione completa fa fail-closed, non salta-record.

**Testing deterministico:** confermato core a eventi senza I/O diretto, ma fake clock/storage/transport nascono prima delle elezioni. Un seed da solo non riproduce processi OS. Oracolo indipendente e registro ghost committed necessari; race detector copre accessi concorrenti osservati, non consenso o linearizzabilità. Ogni invariabile va verificata con tracce, non con status finale soltanto.

| Proprietà | Argomento progettuale accettabile | Evidenza futura richiesta |
|---|---|---|
| Election Safety | Quorum intersecanti e voto persistente unico/termine. | T-004; voto, Sync, crash e voto successivo. |
| Log Matching | prev index/term verificato; truncate solo primo conflitto non committed. | T-006, heartbeat corto, duplicati e risposte tardive. |
| Leader Completeness | Filtro up-to-date e commit tramite termine corrente, non sola maggioranza. | T-007/T-021 con ghost prefix anche dopo snapshot. |
| State Machine Safety | Prefisso committed e apply ordinato una volta/indice/epoch. | T-008/T-015 con backlog e sostituzione stato. |
| Linearizzabilità | Log comune, read in slot committed, CAS e dedup deterministici. | T-019/T-022; checker validato anche su controesempi. |
| Durabilità processo | Persist-before-ack e generazioni pubblicate con ordine esplicito. | T-013/T-016/T-023; nessuna estrapolazione a power loss. |

### Scenari di guasto rivalutati

| Scenario | Safety e disponibilità attese / recupero | Test |
|---|---|---|
| Follower crash | Due nodi sani progrediscono, poi log/snapshot catch-up; host e risorse disponibili. | T-024 |
| Leader crash | Nessun successo speculativo; elezione e noop del nuovo termine; pending resta incerto. | T-004/T-012/T-024 |
| Due nodi persi | Nessun nuovo quorum; status locale soltanto; recuperare directory intatte. | T-019/T-023 |
| Partizione 2 contro 1 | Lato due può progredire dopo stabilizzazione; nuova GET isolata non riesce. | T-019/T-021 |
| Vecchio leader isolato | Nessuna nuova barriera; risposte già committed possono arrivare in ritardo. | T-019, AU-008 |
| Nodo obsoleto riavviato | Parte follower, filtro elettorale e riconciliazione senza cancellare committed. | T-006/T-014/T-015 |
| Risposta persa dopo commit | Retry identico legge risultato sessione replicato, senza nuovo effetto. | T-011/T-012 |
| WAL corrotto | Frame completo CRC errato blocca il nodo; nessun replay selettivo. | T-013 |
| Crash durante write | Frame intero/parziale/non Sync sopravvissuto; recovery rispetta checkpoint. | T-013/T-014 |
| Crash durante snapshot | Solo generazione pubblicata; old/new hard state coerenti. | T-015/T-016 |
| Crash durante compaction | Nessun delete prima della pubblicazione; writer/epoch coordinati. | T-016, AU-003 |
| Elezioni concorrenti | Split vote può impedire progresso; non concede due voti nello stesso termine. | T-004/T-021 |

Liveness richiede stabilizzazione della rete, scheduling e storage tempestivi, risorse non esaurite. I tre processi non tollerano lo spegnimento del Mac come servizio disponibile e non simulano tre dischi indipendenti.

## D. Modifiche effettuate

Aggiornati **57 documenti preesistenti**, con interventi mirati; aggiunti soltanto i due rapporti di audit. Nessun documento eliminato e nessuna ricostruzione indiscriminata dei 96 file. Revisionati sei ADR (003–008); Go, tre membri statici e vincolo offline/zero spese sono confermati senza nuove dipendenze.

| Gruppo | Cambiamento |
|---|---|
| Raft/architettura/diagramma replica | Scheduling apply separato dal completamento, gestione epoch e backpressure. |
| API/rete/sicurezza | Corpo 48 KiB compatibile con CAS massimo; decoder rigoroso e ack snapshot espliciti. |
| Storage/diagramma snapshot | Barriera finale, voto aggiornato, handoff writer, checkpoint e durabilità a tre livelli. |
| Test/fault/AC | Controesempi obbligatori, sopravvivenza I/O e precondizioni dei test di isolamento. |
| Requisiti/risorse | Riserva controllo, condizioni del target failover e offline integration. |
| Roadmap/matrice | Gate per sottoinsiemi e dipendenze corrette, fondamenti test anticipati. |
| Indici e rapporti precedenti | Audit navigabile, inventario 98 file e controlli iniziali chiaramente storici. |

I seguenti percorsi sono aggiornati **in entrambe le lingue**; il link apre quella corrente. Il README comune è aggiornato una sola volta.

- [README.md](README.md)
- [acceptance-criteria.md](acceptance-criteria.md)
- [adr/ADR-003-raft-consensus.md](adr/ADR-003-raft-consensus.md)
- [adr/ADR-004-network-transport.md](adr/ADR-004-network-transport.md)
- [adr/ADR-005-persistence-strategy.md](adr/ADR-005-persistence-strategy.md)
- [adr/ADR-006-linearizable-reads.md](adr/ADR-006-linearizable-reads.md)
- [adr/ADR-007-client-retry-semantics.md](adr/ADR-007-client-retry-semantics.md)
- [adr/ADR-008-fault-injection.md](adr/ADR-008-fault-injection.md)
- [api-specification.md](api-specification.md)
- [architecture.md](architecture.md)
- [consistency-model.md](consistency-model.md)
- [delivery-report.md](delivery-report.md)
- [demo-and-evaluation.md](demo-and-evaluation.md)
- [design-review.md](design-review.md)
- [diagrams/06-replication.md](diagrams/06-replication.md)
- [diagrams/09-snapshot.md](diagrams/09-snapshot.md)
- [failure-model.md](failure-model.md)
- [fault-injection.md](fault-injection.md)
- [implementation-roadmap.md](implementation-roadmap.md)
- [network-protocol.md](network-protocol.md)
- [performance-and-resources.md](performance-and-resources.md)
- [raft-consensus.md](raft-consensus.md)
- [references.md](references.md)
- [requirements.md](requirements.md)
- [security.md](security.md)
- [storage-and-recovery.md](storage-and-recovery.md)
- [testing-strategy.md](testing-strategy.md)
- [traceability.md](traceability.md)
- [README principale](../../README.md)

Nuovi: [audit italiano](../it/architecture-review.md) e [audit inglese](../en/architecture-review.md).

### Validazione documentale effettivamente eseguita

Controllo locale finale: **98 file Markdown**, **48 coppie linguistiche**, **1027 collegamenti relativi**, **68 tabelle**, **14 blocchi JSON** e **16 messaggi HTTP con Content-Length verificato**. Identici i 13 esempi tecnici non Mermaid fra le due lingue; corrispondente la struttura delle nove coppie di diagrammi. Tutti i 32 requisiti mantengono AC e test primari nella matrice; ID e numero sezioni allineati. **Zero errori strutturali rilevati**, nessun documento rimosso, nessun nuovo file non documentale. I 57 file modificati sono stati identificati confrontando digest prima/dopo, poiché il repository contiene ancora documenti non tracciati da Git.

Calcoli leggeri effettuati: richiesta CAS compatta massima 44 116 byte, oltre il vecchio cap 32 768 e sotto il nuovo 49 152; quattro file da 8 MiB esauriscono 33 554 432 byte prima dei file di controllo. Sono verifiche dei limiti della specifica, non benchmark o test del database. Rilette le regole Raft rispetto a figura 2 e sezioni 5, 7, 8 del [paper originale](https://raft.github.io/raft.pdf); controllati [etcd API guarantees](https://etcd.io/docs/v3.6/learning/api_guarantees/), [Go File.Sync](https://pkg.go.dev/os#File.Sync), [Go encoding/json](https://pkg.go.dev/encoding/json), [modello memoria Go](https://go.dev/ref/mem), [linearizzabilità](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf) e [crash consistency](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/pillai). Fonti consultate il 18 novembre 2026; argomenti e correzioni locali non sono risultati sperimentali delle fonti.

Limiti: nessun parser/rendering Mermaid dedicato, solo sorgenti e struttura; nessun build Go, race detector, simulatore, checker eseguibile, benchmark, crash del processo database o crash OS. Il software non esiste. Non verificati dir Sync, lock, APFS e power loss sulla macchina reale. Parità meccanica non certifica equivalenza semantica; la revisione delle traduzioni è manuale e non esterna.

## E. Questioni ancora aperte

I rilievi AU-001–AU-011 sono corretti **nel progetto**; non restano difetti documentali di quella lista lasciati senza soluzione. Restano gate di progetto, distinti dai test futuri necessari per chiudere le proprietà.

| Condizione | Blocco e ambito | Chiusura richiesta |
|---|---|---|
| C-01 / OQ-05 | Prima di fissare l'implementazione WAL/snapshot in M03 | Definire schema canonico byte-per-byte, kind/nullable, bootstrap, checksum e fixture di recovery. Per config/protocollo completare fixture a M04, OpenAPI prima di stabilità a M08. |
| C-02 / OQ-01 | Prima di scegliere il contratto dell'adattatore storage M03; prima di chiudere durabilità M03/M09 | Verificare documentazione e scelta di lock/Sync/rename su macOS ARM64; validazione leggera dell'adattatore durante la futura implementazione, non test distruttivo. Nessuna promessa di durabilità basata sul solo nome Sync. |
| C-03 | Prima di chiudere M07/M10/M11/M13/M14 | Test previsti per apply backlog, handoff, dedup, stale leader e checker con controesempi. Sono gate di accettazione del codice futuro, non test già passati richiesti impossibilmente prima di M01. |

**Non bloccanti per M01–M02:** OQ-02 power loss resta escluso; OQ-03 timing/RAM/SSD si misurano successivamente; OQ-04 licenza richiede scelta prima di pubblicazione; OQ-06 cap sessioni è baseline deliberata; OQ-07 calendario da rivedere dopo primi risultati. ReadIndex non è prerequisito. Nessuna questione aperta autorizza nuove spese o modifica unilaterale delle quote replicate.

## F. Verifica dei vincoli

| Vincolo | Giudizio progettuale e limite |
|---|---|
| macOS ARM64, M5/32 GB | Go e libreria standard, nessun CGO o servizio esterno proposto: architettura compatibile, build reale non eseguita. Adattatore storage resta gate specifico. |
| 0 € aggiuntivi | Nessun cloud, hosting, API a consumo, trial, dominio, hardware o abbonamento richiesti. Nessuna risorsa provisionata in questo audit. |
| Offline | Runtime via loopback/filesystem, toolchain acquisita prima e fetch automatici disattivati. Conferma empirica in T-031 quando esisterà codice. GitHub non è obbligatorio. |
| RAM/CPU | Obiettivo RSS cluster <circa 1 GB, heap morbido 192 MiB/nodo, timer bloccanti, tre processi; plausibile con dataset piccolo, non misurato e non cap hardware garantito. |
| SSD/spazio | Dataset logico 4 MiB/copia, WAL 8 MiB/nodo, directory 32 MiB incluso picco e riserva, log 1 MiB/nodo; cap attivo aggregato 99 MiB (~104 MB), target ordinario circa 100 MB. I/O reale e allocazione APFS da misurare. |
| Carico e cleanup | Benchmark 2+10 s entro deadline 15 s; integration <=30 s/caso, suite <=120 s, massimo 8 client/1 000 operazioni. Cleanup dei run, niente 24/7, GPU, training AI, Docker o Kubernetes obbligatori. |

Tre repliche moltiplicano anche l'I/O delle letture nel log: la fattibilità non si deduce dalla sola RAM disponibile. I cap, l'ammissione e i run brevi rendono plausibile l'esecuzione sul computer già posseduto; i risultati vanno misurati. La prima versione completa è impegnativa per uno studente; il MVP interno e i gate incrementali contengono il rischio, senza promettere la consegna in 144–192 ore.

## G. Esito finale

**APPROVED WITH CONDITIONS.** È tecnicamente ragionevole iniziare **solo M01 e M02**, dopo l'eventuale istruzione esplicita del proprietario di passare allo sviluppo. M01 deve restare una macchina a stati key-value deterministica isolata da rete e consenso. M02 introduce ownership, scheduling e fake, senza dipendere da storage distribuito già corretto.

Prima delle fasi successive chiudere i gate pertinenti C-01/C-02 e completare le evidenze C-03 al rispettivo gate di accettazione. Non è un'approvazione generale di ogni dettaglio binario ancora Proposed, né una certificazione delle garanzie runtime. I problemi corretti non richiedono cambiare Raft o imporre ReadIndex: richiedono rispettare esattamente le nuove barriere e gli oracoli.

**Nessun codice applicativo è stato scritto; nessuna milestone è stata iniziata.** Nessun deploy, servizio, benchmark o infrastruttura a pagamento. Le due lingue sono aggiornate insieme; l'audit termina con la consegna documentale.

## H. Seguito del 19 novembre 2026

L’audit sopra è registro storico. La [revisione pre-M03](pre-m03-review.md) riesamina direttamente il codice M02 effettivo e registra lo stato attuale dei gate: specifica storage C-01 RESOLVED con fixture statiche verificate; scelta documentata C-02 CONDITIONALLY RESOLVED, validazione adattatore reale OPEN. Esito CONDITIONALLY READY soltanto per conformità formati iniziale e test isolati adattatore in un futuro incarico. Prima dell’integrazione writer/pubblicazione richiedere evidenze primitive/lock/errori/visibilità; prima dell’accettazione M03/M09 conservare T-013/T-023. C-03 resta ai gate originali. Nessuna implementazione M03 iniziata o promessa di durabilità validata.

## I. Evidenze piattaforma M03 preliminare

La [fase M03 limitata](milestone-m03-preliminary.md) registra GATE C-02 VERIFIED per le primitive adattatore locali provate e conserva l’ambito originale audit: niente claim durabilità completa da sola Sync. Schemi/fixture C-01 invariati; ora eseguita conformità byte senza stato contro di essi. T-013 completo e accettazione successiva processo/snapshot aperti; M03 PARTIALLY COMPLETED, nessuna milestone successiva iniziata. Stati audit/seguiti precedenti restano storici.

## Completamento M03 successivo

20 novembre 2026: [M03 COMPLETED localmente](milestone-m03.md), con WAL sincrono, validazione transizioni persistenti, reader/recovery log limitati, bootstrap esplicito ed evidenze errore/processo. C-01/C-02 conservano ambito locale risolto/verificato. Nessun apply macchina, consenso, snapshot/rotazione completi o codice M04. Precedenti stati parziali/proposti descrivono la fase storica pertinente; durabilità end-to-end/fisica ancora non verificata ai gate successivi.

## Follow-up M04 attuale — 20 novembre 2026

[M04 COMPLETED](milestone-m04.md) soddisfa l’obbligo residuo fixture configurazione/protocollo M04 di C-01 senza modificare formati storage. OpenAPI resta dovuta prima della stabilità M08; C-03 conserva i gate di accettazione originali. C-02 mantiene perimetro locale Darwin/APFS verificato e limiti guasti fisici. Una lacuna riprodotta nella sequenza coda incompleta M03 è stata corretta minimamente/testata prima dell’integrazione M04. Le precedenti dichiarazioni non iniziata sopra sono storiche; M05 resta non iniziata.

## Follow-up M05 attuale — 20 novembre 2026

[M05 COMPLETED](milestone-m05.md) verifica voto persistente, transizioni termine/ruolo/timer ed evidenze elettorali richieste 20 seed/100 heartbeat con adattatori limitati. Perimetro originale C-01/C-02 conservato; C-03 resta a M07/M10/M11/M13/M14. Nessuna modifica byte storage, ipotesi guasti fisici o gate accettazione successivi. L’esito attuale supera dichiarazioni storiche non iniziata su M05; M06 resta non iniziata.

## Follow-up M06 attuale

[M06 COMPLETED](milestone-m06.md), dopo risoluzione esplicita perimetro che conferma roadmap. Replica log/conflitti/tracking hanno ora evidenze T-006; byte storage e limiti C-01/C-02 invariati. Gate C-03 commit/apply e accettazioni successive aperti alle fasi originali. Nessuna implementazione M07 o nuovo modello durabilità.

## Follow-up gate M07 attuale

[M07 COMPLETED](milestone-m07.md): commit legale corrente e cursori applicazione schedulato/completato separati verificati con backlog C-03 5→7 e oracolo ghost committed/stato. M01/M02/byte-primitive storage invariati, C-01/OpenAPI resta M08, limiti locali C-02 conservati. C-03 handoff/epoch snapshot, dedup, stale-read e checker finale indipendente non chiusi anticipatamente. Nessuna linearizzabilità client completa o garanzia OS/alimentazione.
