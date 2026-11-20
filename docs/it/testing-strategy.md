# Strategia e catalogo dei test

[Indice](README.md) · [English](../en/testing-strategy.md)

**Aggiornamento: 20 novembre 2026 — M01/M02/M03 locali implementate e verificate; le specifiche distribuite restano progettuali.**

## Livelli e modello di riferimento

Unit test della macchina a stati: tabella completa di pre/postcondizioni e quote. Unit test core: fake clock, PRNG con seed, rete e storage a eventi. Integration: tre processi reali e directory temporanee isolate. Concurrency: handler, shutdown e apply con race detector. Property-based: `testing/quick` e fuzzing standard limitato per parser, frame e sequenze di comandi; nessuna libreria Raft o servizio esterno.

Modello sequenziale indipendente, piccolo e deliberatamente diverso dalla struttura implementativa, per evitare test che ripetano lo stesso errore. Stato canonico confrontato solo allo stesso indice applicato. Invarianti INV-01–06 controllate dopo ogni transizione simulata, non soltanto a fine esecuzione.

## Catalogo test pianificati

| Test | Requisito | Procedura | Evidenza |
|---|---|---|---|
| T-001 | FR-001 | Avviare tre config distinte e tentare seconda apertura di n1. | Manifest e exit code. |
| T-002 | FR-002 | Variare membri, cluster ID e hash in config e RPC. | Fixture valide/invalide e stato prima/dopo. |
| T-003 | FR-003 | Iniettare JSON invalido, envelope errato e corpo oltre cap. | Risposte e hash stato invariato. |
| T-004 | FR-004 | Simulare split vote e crash subito dopo voto Sync; rieseguire per 20 seed. | Traccia voti/termini/leader e seed. |
| T-005 | FR-005 | Avanzare clock virtuale per 100 heartbeat senza guasti. | Conteggio elezioni e timer. |
| T-006 | FR-006 | Costruire log divergenti, inviare heartbeat e risposte obsolete, poi heal. | Prefissi, matchIndex e transizioni. |
| T-007 | FR-007 | Riprodurre caso entry vecchia su maggioranza, poi noop corrente. | Commit invariato prima e avanzato dopo. |
| T-008 | FR-008 | Applicare stessa sequenza su tre modelli, ritardando un applier. | Digest canonico e risultati per indice. |
| T-009 | FR-009 | Enumerare chiave assente, valore vuoto, input invalido e DELETE ripetuto. | Tabella fixture API e modello. |
| T-010 | FR-010 | Lanciare due CAS da sessioni diverse sullo stesso valore, new diversi. | Storia con un solo cambiamento riuscito. |
| T-011 | FR-011 | Duplicare ultima sequenza, variare payload, usare sequenza vecchia e gap. | Risultati e tabella sessioni dopo replay. |
| T-012 | FR-012 | Scartare risposta dopo apply, cambiare leader, ritentare identico. | Una sola mutazione e risultato originale. |
| T-013 | FR-013 | Tagliare frame a ogni confine; simulare short write/Sync error e CRC errato. | Immagini disco e decisione recovery. |
| T-014 | FR-014 | Riavviare tutti con suffisso non committed e checkpoint commit inferiore. | Stato ricostruito prima e dopo nuovo noop. |
| T-015 | FR-015 | Installare snapshot a follower molto arretrato con retry chunk e cambio termine. | Hash immagine, sessioni e indici finali. |
| T-016 | FR-016 | Arrestare a ogni cutpoint di pubblicazione/cleanup generazione. | CURRENT e generazione recuperata per caso. |
| T-017 | FR-017 | Eseguire elezione e timeout, ispezionare status e log senza valori utente. | Campione log redatto e snapshot metriche. |
| T-018 | FR-018 | Inviare shutdown durante RPC e persistenza; controllare worker e lock. | Durata, goroutine e recovery successivo. |
| T-019 | FR-019 | Stabilire isolamento completo e scartare messaggi pendenti; poi scrivere sulla maggioranza e invocare nuova GET sul vecchio leader. | Storia che esclude lettura stale di successo. |
| T-020 | FR-020 | Riprodurre due volte scenario con stesso seed e clock virtuale. | Tracce eventi identiche byte per byte. |
| T-021 | NFR-001 | Enumerare piccoli schedule con 3 nodi, 3 proposte e fino a 2 guasti. | Report invarianti e bound esplorato. |
| T-022 | NFR-002 | Verificare 20 storie fino a 12 operazioni con CAS, retry e failover. | Ordine testimone o controesempio ridotto. |
| T-023 | NFR-003 | Confermare dati, crash di processo su uno e poi tutti, recuperare directory intatte. | Lista successi confrontata con GET successivi. |
| T-024 | NFR-004 | Fermare follower, riavviarlo e attendere catch-up; poi fermare leader lasciando due nodi sani. | Timeline elezione e ripresa entro target locale. |
| T-025 | NFR-005 | Eseguire benchmark limitato secondo profilo risorse. | Campioni grezzi e metodologia. |
| T-026 | NFR-006 | Raggiungere quota dataset, code e disco simulato senza carico massivo. | Errori, RSS, CPU e spazio massimo. |
| T-027 | NFR-007 | Confrontare alberi linguistici, ID, blocchi tecnici e link; revisione semantica. | Rapporto revisione con limiti dichiarati. |
| T-028 | NFR-008 | Iniettare fake clock, PRNG, transport e storage nello stesso core. | Replay regressione senza rete reale. |
| T-029 | NFR-009 | Registrare go version/GOARCH e compilare sul Mac ARM64. | Output build e suite locale. |
| T-030 | NFR-010 | Provare bind 0.0.0.0, input gigante e permessi larghi. | Rifiuti e permessi effettivi. |
| T-031 | NFR-011 | Disabilitare Internet mantenendo loopback ed eseguire integrazione offline autonoma; demo presentata a M16. | Manifest dipendenze e traccia offline. |
| T-032 | NFR-012 | Revisionare dipendenze e procedura su macchina già posseduta. | Inventario licenze, costi e prerequisiti. |

## Storie e checker di linearizzabilità

Un coordinatore registra con clock monotono unico: history_version, event_id, client_id, sequence, attempt_id, operation, key, input sintetico, invoke_time, return_time, code, outcome, result, server_term/index e scenario seed. Input/output di valore solo nelle fixture sintetiche, mai log operativi. Lo storico mantiene ogni tentativo; il checker raggruppa retry della stessa mutazione in una sola operazione logica, con intervallo dalla prima invocazione alla **prima** risposta definitiva osservata; retry successivi non estendono tale intervallo. Retry GET rimane operazione distinta.

Il checker costruisce precedenze reali (`return(A) < invoke(B)`), esplora ordinamenti topologici e applica il modello per verificare risultati. Operazioni con risposta unknown o senza risposta possono essere escluse o inserite con effetto compatibile, mai automaticamente considerate fallite. Controlla separatamente che retry restituiscano risultato originale e non cambino stato due volte. Un witness viene salvato; un controesempio conserva la storia ridotta. Timeout del checker è **inconclusive**, non successo.

Limiti iniziali: 20 storie, massimo 12 operazioni logiche per storia, due client e due chiavi, budget checker 5 s/storia. Non partizionare ingenuamente per chiave se quote e sessioni condividono stato globale. Eseguire prima casi di validazione del checker: storia nota legale, storia impossibile, timeout e pendenti.

## Riproducibilità, limiti e gate

Manifest: revisione Git futura, versione Go/macOS, architettura, configurazione e hash, PRNG e sua versione, seed iniziale (1..20 per suite ordinaria), eventi ordinati, fault plan, history e cutpoint. Clock virtuale e ordine degli eventi determinano la simulazione. Nei processi reali seed uguale non determina scheduling OS: salvare tracce e ridurre il caso in simulazione.

Unit test deadline 10 s/caso, integration 30 s/caso, suite ordinaria massimo 120 s; concurrency massima 8 client, 1 000 operazioni. Fuzzing opzionale 10 s per target, due worker, non nella suite ordinaria. Fermare su superamento risorse; no soak test notturni. Directory uniche per run; rimuovere soltanto artefatti del run riuscito, conservare fallimenti entro quota evidenze 10 MiB fuori directory dati attive.

Comandi locali ora disponibili per M01/M02: `go test ./...` e `go vet ./...`; `go test -race -timeout=30s ./internal/applier` è superato per M02. Race detector trova data race nelle esecuzioni osservate; non dimostra quorum, linearizzabilità o assenza di deadlock in ogni schedule. Test e vet di entrambi i package locali e race mirato M02 sono eseguiti, come riportato nelle evidenze locali; le verifiche distribuite restano future.

Il gate M14 richiede tutti i test obbligatori e AC associati, risultati riproducibili e zero violazioni note. I test di power loss fisico restano esclusi: non colmare la lacuna con risultati di kill del processo.

## Controesempi obbligatori e oracle indipendente

T-022/checker: storia sintetica PUT(x,1) completata, poi GET(x)=0, poi retry identico della PUT già completata. Il checker deve rifiutarla: il retry non può spostare la prima PUT dopo la GET. Se il primo tentativo ha solo timeout e nessuna risposta definitiva, l'intervallo resta pendente fino al primo completed o alla fine della storia; non assumere l'assenza dell'effetto.

Raggruppare soltanto tentativi con stessa identità **e stesso comando canonico**. REQUEST_CONFLICT, STALE_SEQUENCE e SEQUENCE_GAP sono osservazioni del protocollo di sessione, non duplicati riusciti; il modello completo include sessioni, esito originale, sequenza consumata e quote. Confrontare tutte le risposte definitive duplicate con il risultato originale, senza introdurre nuovi effetti. Per i risultati completed senza modifica (CAS fallita, quota), modellare anche l'avanzamento di sequenza ove previsto.

T-008/apply-backlog: commit 5 con lastApplied 0; prima di qualunque completamento, commit 7. La sequenza schedulata deve essere esattamente 1..7, una volta; saturare inoltre la coda e verificarne ripresa. T-015/snapshot-epoch e T-016/hard-state-handoff controllano le barriere specificate nello storage. T-021 conserva un registro ghost globale delle entry diventate committed e confronta tutti i leader futuri con quel prefisso, anche dopo compaction; verificare soltanto l'uguaglianza dei log correnti non prova Leader Completeness.

Ogni criterio mantiene il test primario nella matrice; questi sottocasi sono obbligatori nello stesso identificatore. Tutti restano **Not run**. I limiti di 20 storie e 120 s sono budget massimi: se il budget globale termina, casi mancanti sono inconclusive, mai pass implicito.

## Evidenze locali M01

T-008/M01 e T-009/M01 sono eseguiti e passano nel sottoinsieme della roadmap: 12 test principali, 44 sottocasi, tre ripetizioni aggiuntive e 100% statement coverage del package locale. Modello oracle a lista/stringhe indipendente dalla mappa di produzione, seed fisso 19 e confronto dopo ogni passo. Vedere [comandi e limiti](milestone-m01.md). Alla consegna M01 i test distribuiti, il backlog apply, il checker di linearizzabilità, la race detection e la fault injection erano Not run. Seguono le evidenze M02.

## Evidenze locali M02

T-008/M02 e T-018/M02 Passed: 20 nuovi test; suite completa ripetuta tre volte, race detector mirato superato una volta senza race segnalate. Copertura: applier 99,3%, statemachine 100%. TestT008ApplyBacklogExactlyOnce esercita la separazione dei cursori schedulato/completato; TestT008QueueFullAndBlockingBackpressure esercita saturazione e ripresa. TestT018ShutdownDuringApplyAndConcurrentWaiters e TestT018ShutdownWakesBlockedProducers esercitano il lifecycle. TestT028FakeBoundariesAndControlledDelay fornisce fondamenta clock/transport/storage solo nei test. Vedere [scenari completi, comandi e limiti](milestone-m02.md). AC-008, AC-018 e AC-028 completi restano aperti ai gate finali; nessun risultato distribuito o di replay completo dichiarato.

## Evidenze revisione pre-M03

19 novembre 2026: [riesame M02](pre-m03-review.md) riprodotto l’unico return scoperto di cancellazione dopo copia e aggiunto test deterministico. Ora 21 test applier passano con 100,0% statement coverage; M01 resta 12 test/44 sottocasi, 100,0%. Un run race mirato superato. Vettori statici formato superano aritmetica/checksum, non esecuzione recovery. Il [piano finito C-02](macos-storage-contract.md) definisce futuri test isolati piattaforma; nessun test M03 o esperimento OS/power loss eseguito.

## Evidenze successive M03 preliminare

[Relazione di fase](milestone-m03-preliminary.md): GATE C-02 VERIFIED per primitive locali, M03 PARTIALLY COMPLETED. Passano 7 test storageformat/18 sottocasi e 13 funzioni test storagefs/16 sottocasi; superati race mirato e regressioni M01/M02 invariate. Ambito codec: frame/entry/digest/manifest senza stato; nessun motore WAL/replay o chiusura completa AC-013/AC-014/AC-023. Gate snapshot e successivi aperti. Cifre/stati precedenti descrivono la fase storica pertinente.

## Chiusura M03 locale

[Evidenze finali](milestone-m03.md): M03 COMPLETED; T-013/parte storage AC-013 locali Passed. WAL con 17 funzioni principali/25 sottocasi, 82,9% statement coverage locale; passano regressioni M01/M02, codec/adattatore e race mirato WAL/adattatore/applier. Tutte le 18 aspettative recovery canoniche coperte al confine byte/namespace pertinente. Integrazione risposte Raft FR-013 completa, restart server AC-014 e AC-023 end-to-end dipendono da milestone successive; accettazione snapshot AC-015/AC-016 a M13. Copertura/race/test processo non certificano resilienza a guasti fisici. Nessun ID requisito/AC primario o gate distribuito finale rimosso.

## Evidenze finite M04

[Test e risultati M04](milestone-m04.md) implementano sottoinsiemi locali T-001/T-002/T-003/T-029: tre avvii libreria/identità/lock-prima-bind, fixture configurazione/protocollo canoniche, rifiuto input/dimensione prima del processor, scambio HTTP, cancellazione/timeout 300 ms, cap connessioni, errore/shutdown e build ARM64 nativa. 12 nuove funzioni test M04 più una regressione sequenza coda WAL; race verificata su node/transport/wal/storagefs/applier. Canali/contesti sincronizzano i test; listener loopback dinamici mantenuti e cartelle temporanee private piccole evitano gare sulle porte/file personali. Evidenze exit code CLI, safety Raft e applicazione/durabilità end-to-end restano gate successivi.

## Suite elettorale deterministica M05

[M05](milestone-m05.md) aggiunge 17 funzioni test: 16 raft, una integrazione nodo reale. T-004 usa seed 0–19, split forzato, perdita/ritardo/duplicazione/riordino messaggi, crash dopo Sync modello prima della callback, riavvio follower e rifiuto secondo voto; oracoli indipendenti voto/quorum/leader e [tracce salvate](../fixtures/election-v1-results.json) devono riprodursi due volte. T-005 avanza separatamente 100 round heartbeat senza elezione spuria. Testati partizione/heal, risposte vecchie termine maggiore, regole timer, token/Sync/errore/overflow, admission limitata, cancellazione e shutdown con join. Gate disco/canali servizio e clock virtuali evitano sincronizzazione sleep. Riapertura WAL effettiva e tre nodi loopback provano adattatori; nessun nuovo crash OS/alimentazione o operazione client. Race verificata raft/node/transport/wal/storagefs/applier; regressione originale sequenza-coda WAL conservata.

## Suite replica M06

[M06](milestone-m06.md) aggiunge 11 funzioni test finali (10 raft, una nodo), conservando due test preliminari lifecycle e ogni fixture elettorale/WAL originale. T-006 copre riconciliazione suffisso, duplicati/heartbeat corto, conflitto committed, correlazione invio effettivo, hint/vecchie generazioni, quota locale/errore Sync, split wire/WAL massimo, tagli frame/riapertura e riavvio follower tre nodi reali. Cinque seed deterministici riprodotti due volte contro [risultati salvati](../fixtures/replication-v1-results.json) e invarianti indipendenti vista disco. M05 20 seed/100 round ancora superati. Nessun test commit/applier riportato impropriamente come comportamento M07 implementato.

## Suite finita commit/applicazione M07

[M07](milestone-m07.md) aggiunge 14 test: commit corrente e controesempio vecchio termine; cap prefisso follower/checkpoint fallito; cursori schedulato/applicato distinti; backlog M02 reale capacità 1 5→7 con ghost committed; percorsi panic/negativo dominio/cancellazione/conservazione risultati; tagli checkpoint reali/replay CAS nuovo; tre [tracce commit](../fixtures/commit-v1-results.json) riprodotte due volte con partizione/elezione/riavvio e oracoli indipendenti committed/risultati/stato; CAS concorrenti/failover/replay loopback reali. Fixture/suite precedenti superate. Simulatore controlla eventi core, backlog runtime verifica separatamente scheduling asincrono; nessuno prova ogni schedule OS.

## Evidenze di fase M08

[Relazione M08](milestone-m08.md): FR-009/FR-012, AC-009/AC-012 e T-009/T-012 superati solo per sottoinsieme roadmap PUT/DELETE, validazione HTTP rigorosa, deadline, unknown/not_admitted e OpenAPI. Test includono cancellazione con disco/apply bloccati, perdita osservata a termine maggiore, ammissione limitata con capacità peer separata, tre nodi reali con scritture concorrenti/elezione/minoranza, risposta persa senza retry automatico e DELETE ripetuta con nuovo risultato. Checker schema/fixture HTTP include controlli negativi ed estensioni canoniche runtime. Regressioni M01–M07 e race mirato superati. GET AC-009 completa resta M10; retry dedup/failover AC-012 resta M11, con snapshot/checker storie ai gate successivi. Obbligo artefatto/fixture sottoinsieme M08 C-01 soddisfatto; nessuna stabilità v1 completa dichiarata. C-02 e obblighi successivi C-03 mantengono limiti originali.
