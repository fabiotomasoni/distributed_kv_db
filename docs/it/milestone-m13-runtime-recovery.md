# M13 Fase C — Recovery snapshot e suffisso WAL nel runtime

[Indice](README.md) · [English](../en/milestone-m13-runtime-recovery.md)

**23 novembre 2026 — RUNTIME RECOVERY PHASE COMPLETED. M13 complessiva: PARTIALLY COMPLETED. M14 NON INIZIATA.** Baseline Git pulita `c0e00d0`, con Fasi A e B già registrate nella cronologia prima dell'incarico. Nessun commit effettuato in questa fase.

## Mandato e verifica iniziale

La [roadmap](implementation-roadmap.md) assegna M13 a snapshot e compaction: generazioni atomiche, installazione, fixture crash, T-015/T-016/T-026 nel relativo sottoinsieme. M14 mantiene checker indipendente e verifica generale delle proprietà. Le [decisioni pre-M13](pre-m13-review.md), [Fase A](milestone-m13-preliminary.md) e [Fase B](milestone-m13-persistence.md), nelle due lingue, fissano formato, ownership, pubblicazione e recovery. ADR-005/ADR-010 e contratti storage/commit/sessioni conservati. All'ingresso C-01/OQ-05 era RESOLVED con conformità verificata; C-02/OQ-01 isolato VERIFIED ma integrazione condizionata; C-03 CONDITIONALLY READY.

La Fase C autorizza prima recovery combinato isolato, poi avvio controllato e test limitati del cluster. Non autorizza creazione automatica snapshot, rotazione live, compaction, cancellazione prefissi WAL, GC o InstallSnapshot. Non serve modificare formati, criteri o algoritmi di election/replica. Non sono emerse contraddizioni bloccanti.

## Componenti e file

Creato `internal/snapshot/recovery.go`: RecoverCommitted e CombinedRecovery. Nuovi test: `internal/snapshot/recovery_test.go`, `internal/raft/snapshot_recovery_test.go`, `internal/node/m13_test.go`, `internal/node/m13_process_test.go`.

Modificati `internal/wal/open.go` e `store.go` per apertura v2 esplicita a base snapshot, copia dell'immagine validata e prosecuzione append sullo stesso WAL/LOCK; `internal/raft/commit.go`, `service.go`, `application.go` per capability di avvio e handoff applicativo; `internal/node/node.go` per l'entry point esplicito. Aggiornato soltanto il commento package di `internal/snapshot/memory.go` per il nuovo uso esplicito. Nessuna modifica a writer/pubblicazione Fase B, codec/fixture canoniche, adattatore filesystem, API HTTP, configurazione pubblica o CLI.

Aggiornati nelle due lingue storage-and-recovery, architecture, raft-consensus, data-model, failure-model, testing-strategy, acceptance-criteria, traceability, development-guide, implementation-roadmap, ADR-005/ADR-010, indici e README principale; aggiunte queste relazioni. Le relazioni storiche rimangono riferite alle rispettive fasi.

## Algoritmo del recovery combinato isolato

RecoverCommitted riceve configurazione v2, immagine canonica e State proveniente da Replay WAL validato. Verifica S/termS, base, hard state, voto, sequence, commit, ultimo indice, ordine/continuità/termini delle entry e corrispondenza con i rispettivi byte canonici. Controlla quota delle entry prima di accumularne le dimensioni. Richiede base<=S<=C<=N, termine al confine coerente e nessun overflow; snapshot corrotto o suffisso mancante produce errore senza stato parziale.

Restore crea una nuova coppia KV/sessioni al prefisso S. Un worker M02 inizializzato a S applica in ordine esclusivamente S+1..C attraverso ProjectWithSessions. Ogni future deve completarsi e il risultato deve essere decodificabile prima dell'avanzamento. Le entry C+1..N sono validate/conservate nel log, mai applicate dal recovery. PUT/DELETE/CAS/read/noop mantengono i propri slot. Il ciclo usa offset, non S+1 con possibile wraparound a MaxUint64.

Al termine il worker di recovery viene drenato e chiuso; soltanto dopo la sua chiusura CombinedRecovery trasferisce la proprietà esclusiva della nuova Machine con Snapshot, LastApplied=C e ScheduledApplyIndex=C. Un successivo applier può partire a C; mai due worker contemporaneamente sulla stessa macchina. Cancellazione o errore drena il lavoro già accettato e non restituisce una macchina parziale. Nessun risultato GET storico, waiter o result ring viene restaurato.

## Prefisso conservato e compatibilità del WAL

Il componente isolato accetta anche una vista log-level completa con base precedente a S: valida il prefisso e il termine a S ma non applica 0..S. Un test usa CAS già nello snapshot e retry nel suffisso per verificare che non venga eseguita due volte.

Questo non introduce un secondo formato persistente: una generazione pubblicata continua a contenere snapshot e WAL base-S secondo Fase B, primo frame hard state corrente/commit=S, suffisso e checkpoint finale. Un vecchio WAL base-zero non può essere semplicemente sostituito nella generazione base-S: Replay lo rifiuta. Le vecchie generazioni con prefisso completo rimangono conservate e non sono selezionate come fallback. «WAL-only» indica il percorso storico con snapshot vuoto/base zero; il contratto richiede comunque CURRENT e i file della generazione. Directory senza CURRENT o senza snapshot non sono un nuovo formato ammesso.

## Deduplicazione e stato logico

La stessa Machine ripristina e aggiorna KV e sessioni: client_id, ultima sequence, digest, tipo, risultato e indice originale; niente cache parallela o TTL. Retry di CAS riuscita già nel prefisso conserva l'esito e indice originale; CAS fallita resta fallita anche quando un'altra operazione rende lo stato attuale compatibile con il confronto. Sequenze obsolete/conflitti e cap 1024 restano M11.

I test isolati confrontano byte snapshot finali con una macchina originale continuata: suffisso PUT/DELETE, CAS riuscita/fallita, read/noop, retry e stale; nessuna entry speculativa contaminante. Il caso full-prefix porta x di nuovo ad A prima del retry di una CAS fallita: il valore resta A e il risultato conserva indice 3. Un'immagine con 1024 sessioni rifiuta un nuovo client durante il replay senza eviction.

## Attivazione controllata e bootstrap

L'attivazione è una scelta esplicita della libreria: **node.StartAPIWithSnapshotRecovery(config v2)**, senza bootstrap implicito. Internamente usa wal.OpenSnapshotSessions e raft.Options.SnapshotRecovery. Il percorso ordinario StartAPI e il comando quorumkv continuano a usare il recovery precedente, rifiutando base nonzero. Nessun flag/configurazione pubblica aggiunto, migrazione automatica o attivazione sui cluster esistenti. La modalità esplicita rifiuta profilo legacy v1, bootstrap simultaneo e combinazioni senza API/commit/sessioni/read. MachineFactory non è ammessa con snapshot recovery.

Ordine effettivo: configurazione; CURRENT presente; LOCK; IDENTITY v2 e hash/nodo; CURRENT canonico; generazione selezionata e immagine privata; digest/config/base; WAL e hard state/commit; eventuale sola riparazione della coda finale incompleta ammessa M03, con Sync; core log-level follower senza attività; restore e replay isolati; ricostruzione cursori; nuovo applier a C; solo dopo bind listener, Start del servizio/timer/worker rete e Serve. L'ordine realizza lo stato completo prima di esporlo; la costruzione anticipata del core non produce elezioni o RPC.

CURRENT resta il manifest autorevole v1; nessun terzo manifest o nuovo protocollo. LOCK e descrittore WAL restano nello stesso Store durante lettura e prosecuzione runtime: nessuna finestra close/reacquire. SnapshotImage restituisce una copia dei byte validati. Il nuovo writer continua ad appendere esclusivamente al WAL selezionato con token generazione/sequence esatti; non pubblica nuove generazioni e non cancella file.

## Cursori, handoff e fallimenti

Al ripristino base A=L=S; dopo completamento reale del replay A=L=C, N resta l'ultimo indice persistito, anche maggiore di C. Il core ricostruisce Scheduled/Applied in ordine per le entry effettivamente completate; non pone i cursori a N. Termine e voto provengono dal WAL finale, non dallo snapshot. Il nodo parte follower, applyEpoch=1 della nuova istanza; nessun cambio epoca o installazione live è implementato.

Il nuovo applier runtime parte a C dopo che quello isolato è terminato. Il result ring resta vuoto con cursore C: una vecchia receipt risulta fuori retention, non riceve una risposta GET inventata. Nuove richieste attraversano i normali quorum e fence di M07/M10/M11. nextIndex parte dall'ultimo log+1, match remoto non viene aumentato in base al solo snapshot. Il confine termS resta sentinella per AppendEntries.

Errori storage/profilo/manifest/snapshot/WAL fanno fallire l'avvio prima di listen; nessun fallback WAL-only. Un bind fallito dopo recovery drena il servizio e chiude Store/LOCK. Shutdown immediato segue il lifecycle esistente; le prove riacquisiscono lock e porte. Nessuna mappa parziale viene pubblicata o annullata tramite rollback.

## Test e fault injection

13 nuove funzioni test principali: sette recovery isolato, una handoff servizio, tre bootstrap/compatibilità e due processi (una è helper figlio). Il primo gruppo isolato è passato prima delle modifiche al bootstrap, come richiesto.

| Prova | Risultato verificato |
|---|---|
| Snapshot vuoto/stateful, prefisso completo, replay ripetuto | Nuovo stato equivalente; solo S+1..C, nessuna doppia applicazione. |
| Commit prima di S/oltre N, gap, termine/voto/base/sequence incoerenti, raw entry corrotta/disallineata, suffisso mancante, snapshot corrotto | Errore e nessuna macchina/cursore parziale. |
| Cancellazione, cap worker, 1024 sessioni, MaxUint64 | Rifiuto controllato o confine corretto senza overflow/eviction. |
| Sette interruzioni pubblicazione: prima creazione, dopo snapshot write/Sync, dopo rename generazione, prima pubblicazione/CURRENT rename e dopo rename | CURRENT vecchio/nuovo seleziona il log corretto; stato applicato finale identico. |
| Snapshot/WAL/CURRENT/IDENTITY mancanti o corrotti, base discordante, modalità default e bind fallito | Nessun avvio parziale; callback listen non raggiunta salvo bind intenzionale; porte/LOCK liberati. |
| Lettura immagine bloccata con gate esplicito | NewService non ritorna né attiva timer/rete/collector prima del recovery; poi A=L=C e follower. |
| WAL-only base zero esplicito e ordinario; legacy v1; CURRENT mancante | Compatibilità positiva base zero, rifiuto legacy/mancanza senza fallback. |
| Tre processi con snapshot già pubblicato | Scritture/GET dopo snapshot, crash/restart follower e leader, due crash/restart completi, nuovo suffisso fra i due, retry CAS originario e fallito preservati. |

Il test processi usa al massimo tre figli appartenenti al run, con kill indirizzato al loro PID e attese di join; comandi privati status su pipe, porte localhost, deadline e cleanup. Nessuna API diagnostica pubblica aggiunta. I successi HTTP vengono attesi anche come progresso applicato sulle repliche vive prima delle interruzioni.

Un suffisso speculativo è escluso **durante il recovery**; dopo l'avvio Raft può legittimamente commetterlo tramite noop del termine nuovo e quorum. Il test non promette che un'operazione unknown non verrà mai applicata. Tutti i nodi del test condividono una base compatibile: il caso follower privo di un prefisso necessario richiede InstallSnapshot e rimane non supportato. Non vengono introdotti nuovi fault switch nel runtime o nel simulatore M12.

## Controlli eseguiti e copertura

Superati suite completa M01–M12 e M13 A/B/C con copertura e timeout 75 s/package; il package node, inclusi i test processo, completa in circa 17,1 s, snapshot in circa 4,3 s. Race mirato superato su snapshot/wal/applier/committed/storageformat e sui nuovi casi Snapshot di raft/node (node circa 6,9 s con race). Superati gofmt, go vet, build macOS ARM64, verificatore fixture canoniche, dipendenze e controllo del diff. Il verificatore Python controlla l’aritmetica dei vettori, non sostituisce i test Go di recovery. go list -m all restituisce solo il modulo del progetto, nessun go.sum. Nessuna fixture precedente modificata. Verifica strutturale: 142 Markdown, 70 coppie linguistiche, link/tabelle/ID/blocchi tecnici senza errori. HEAD rimane c0e00d0; nessun commit.

Copertura statement dalla suite completa con blocchi deduplicati fra package: snapshot 87,8%, nuovo recovery.go 81,0% (51/63), node 82,3%, raft 88,7%, wal 83,8%, applier 95,1%, committed 85,1%. Profilo temporaneo `/tmp/quorumkv-m13c.cover`. Coverage e race non dimostrano formalmente safety o crash consistency.

## Gate, accettazione e readiness residua

| Gate / criterio ufficiale | Evidenza Fase C | Stato finale e prove mancanti |
|---|---|---|
| C-01 / OQ-05 | Formati v1 e profilo v2 invariati, codec/fixture riutilizzati. | RESOLVED/conformità verificata. |
| C-02 / OQ-01 | Lettura base-S, append successivi e restart processo sulla stessa generazione, lock continuo e recovery esplicito verificati. | Sottoinsieme recovery runtime VERIFIED; gate M13 complessivo CONDITIONALLY READY per rotazione live/cleanup e restante ciclo. |
| C-03 | Cursori corretti, handoff prima dei listener, KV/sessioni, risultato originale e speculative escluso al recovery verificati. | CONDITIONALLY READY: installazione live, epoche/completion obsolete e safety compaction ancora da provare. |
| T-015 / FR-015 / AC-015 | Restore+suffix e retry dopo restart verificati. | Aperto: follower molto arretrato, trasferimento chunk/retry, cambio termine e InstallSnapshot. |
| T-016 / FR-016 / AC-016 | Cutpoint selezione, recovery e crash follower/leader/tutti dopo scritture confermate. | Aperto: compaction, cleanup/GC e switch di generazione nel runtime vivo. |
| T-026 / NFR-006 / AC-026 | Cap worker 1..64, sessioni 1024 e byte WAL limitati; quote storage già verificate conservate nel writer M03. | Aperto: ciclo compaction/cleanup e riserva sotto tutte le interazioni runtime, misure RSS/CPU della roadmap. |

Le definizioni ufficiali rimangono in [test](testing-strategy.md) e [accettazione](acceptance-criteria.md); nessun sottoinsieme le sostituisce. Compaction e InstallSnapshot **non sono READY per attivazione** sulla base della sola Fase C: richiedono implementazione e relative prove/autorizzazioni.

## Problemi, garanzie e limiti

Nessun difetto M01–M12/Fasi A–B dimostrato richiede correzione fuori perimetro. In sviluppo sono stati corretti due errori di harness: confronto di un valore GET opzionale come stringa diretta e uso di un helper mutante per costruire una read canonica. Quest'ultimo causava un'attesa del test scaduta prima del gate; corretto il record e rese limitate le attese esplicite. Le verifiche finali passano dopo le correzioni; nessun errore viene nascosto modificando fixture o criterio.

Garanzie verificate nel modello di test: stato committed ricostruito prima dell'ascolto, nessuna doppia applicazione 0..S, dedup coerente, voto/termine dal WAL, successi conservati dopo crash di processo con storage integro e maggioranza recuperabile. Non sono dimostrate perdita SSD, crash/reboot OS o alimentazione, safety completa di compaction/installazione, checker generale M14 o prontezza alla produzione. Un hash verifica integrità, non prova una storia Raft arbitraria.

## Budget, hardware e decisione

Budget aggiuntivo 0 €; toolchain Go 1.27.4 darwin/arm64 e cache preesistenti sotto `/tmp`, GOPROXY/GOSUMDB off, sola libreria standard. Nessun cloud/download/servizio a pagamento, GPU, VM o Docker. GOMAXPROCS=2 e package seriali; dataset di pochi record, snapshot piccoli (cap sessioni sintetico in memoria), generazioni conservate, directory temporanee. Timeout 40 s mirati/75 s regressioni e 25 s per figlio; massimo tre nodi ordinari, nessun cluster lasciato attivo. Nessuno stress, benchmark prolungato, scrittura massiva o crash fisico; RSS/CPU non misurati.

**Fase C: RUNTIME RECOVERY PHASE COMPLETED. M13: PARTIALLY COMPLETED.** Nessun blocco conosciuto nel perimetro completato; C-02/C-03 complessivi CONDITIONALLY READY e T-015/T-016/T-026 aperti. Un nuovo incarico può autorizzare progettazione/verifica del ciclo live, compaction e InstallSnapshot secondo i gate già approvati, senza dedurne l'attivazione permanente. Pubblicazione automatica, cancellazione prefissi/GC e trasferimento peer restano non implementati; M14 non iniziata. Nessun commit Git.
