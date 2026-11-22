# M13 Fase B — Persistenza e recovery isolato

[Indice](README.md) · [English](../en/milestone-m13-persistence.md)

**23 novembre 2026 — PERSISTENCE PHASE COMPLETED. M13 complessiva: PARTIALLY COMPLETED. M14: NON INIZIATA.** Baseline Git pulita `60e2a2c` (Fase A già registrata nella cronologia prima di questo incarico). Nessun commit effettuato in Fase B.

## Stato iniziale e mandato

La [roadmap ufficiale](implementation-roadmap.md) definisce M13 «Snapshot e compaction»: generazioni atomiche, installazione e fixture crash; gate T-015/T-016/T-026 nel sottoinsieme di fase. M14 conserva il checker indipendente e le prove generali delle proprietà; non viene iniziata. La [revisione pre-M13](pre-m13-review.md) e la [Fase A](milestone-m13-preliminary.md) stabiliscono formato, cattura e limiti. All'ingresso la fase preliminare era PRELIMINARY PHASE COMPLETED, C-01/OQ-05 design e conformità codec verificati, C-02/OQ-01 nuove generazioni e C-03 CONDITIONALLY READY.

Questo incarico autorizza generazioni persistenti, pubblicazione e recovery esclusivamente isolati, con fault injection in directory dei test. Non autorizza avvio snapshot nel nodo, sostituzione recovery M03/M09, compaction, cancellazione WAL, InstallSnapshot o trasferimenti fra nodi. Nessun requisito ufficiale viene modificato per dichiarare conclusa M13.

## Componenti e file

Creati `internal/snapshot/persistence.go` e `internal/snapshot/persistence_test.go`, specifici Darwin. API: OpenPersistence, Persistence.Publish, Persistence.Recover, Persistence.Close e DiskRecovery. Riutilizzati codec QSN1 Fase A, Restore, Replay WAL M03 e primitive storagefs per LOCK, file privati, WriteAll, File.Sync e directory fsync. Nessun secondo codec, append worker o database sessioni.

OpenPersistence richiede una directory v2 già inizializzata esplicitamente con M03, con precedente proprietario chiuso. Non effettua bootstrap. Il suo LOCK esclusivo stabile impedisce aperture cooperanti concorrenti; un mutex serializza pubblicazione, recovery e Close. La configurazione deve corrispondere a IDENTITY v2 per cluster, nodo e hash. Nessuna nuova opzione CLI/HTTP o chiamata dal runtime.

Aggiornati README principale e, in entrambe le lingue, indici, canonical-formats, storage-and-recovery, macos-storage-contract, architecture, failure-model, testing-strategy, acceptance-criteria, traceability, implementation-roadmap, development-guide e ADR-005/ADR-010; aggiunte le due relazioni milestone-m13-persistence. Nessun sorgente M01–M12/Fase A, dipendenza, formato o fixture precedente modificato.

## Identità e manifest approvati

Il contratto canonico prevede esattamente `snapshot` e `wal` nella directory `gen-N`: **CURRENT è il manifest di selezione**, non viene inventato un terzo file manifest per generazione. IDENTITY è il manifest immutabile di identità. CURRENT v1 canonico/checksummato, massimo 4096 byte, contiene generation, snapshot_sha256, base_index e base_term; hash dell'intero file snapshot incluso trailer, distinto dal digest interno QSN1. Immagine/payload restano versione 1; identità e configurazione applicativa restano v2. Nessuna migrazione v1→v2.

Sotto LOCK si sceglie un numero N nuovo, maggiore di tutte le generazioni riconosciute, anche `.tmp`; rifiutati nomi malformati e overflow MaxUint64. La scansione alloca nomi, non seleziona il recovery. File creati esclusivi 0600, directory 0700; symlink, hard link e percorsi insicuri sono rifiutati. La generazione selezionata deve contenere soltanto i due file approvati.

## Scrittura e pubblicazione

Publish consuma una cattura già distaccata; il caller ne mantiene immutabili i buffer fino al ritorno. Encoding fuori dall'applier. Prima di scrivere si valida la generazione corrente sotto LOCK e si legge il WAL autorevole: lo snapshot deve avere base precedente <= S <= commit corrente e termine coerente con il log a S. La correttezza storica dei dati applicativi dipende dalla cattura fidata Fase A; un digest non dimostra che un'immagine arbitraria corrisponda a una storia Raft.

Il WAL nuovo deriva dalla vista corrente bloccata, non da un voto fornito dal caller: primo frame sequence 1 con hard state corrente e commit=S, tutto il suffisso S+1..N in transazioni di massimo 16 entry/256 KiB, poi checkpoint del commit C corrente. Conserva anche C+1..N speculativo. Il caso S=MaxUint64 non calcola S+1. La prova usa snapshot con termine 2 e hard state finale termine 4/voto n2: il voto aggiornato sopravvive.

Il protocollo è: riserva contabile old+new+staging+controlli; creazione gen-N.tmp; scrittura completa e Sync/Close snapshot; rilettura e confronto/validazione dei byte persistiti; scrittura completa e Sync/Close WAL e rilettura/Replay di validazione; fsync staging; rename gen-N.tmp→gen-N; fsync radice; creazione esclusiva CURRENT.tmp; write/Sync/Close e validazione dei byte riletti; rename CURRENT.tmp→CURRENT; fsync radice. Il nome generazione viene restituito soltanto dopo tutte le barriere.

WriteAll continua dal resto dopo una write corta senza errore, ma non ripete una write parziale con errore; zero progresso è errore. Errori create, write, Sync, Close, rename, fsync o rilettura non diventano successo. Un errore I/O dopo l'inizio della costruzione avvelena il proprietario, che richiede chiusura e recovery espliciti. Rifiuti di input/quota prima della costruzione non pubblicano nulla. Cancellazione è controllata prima dell'ammissione; non interrompe syscall già accettate. Close attende I/O in corso prima di rilasciare LOCK; gli errori precedenti restano visibili.

## Punto di selezione e generazioni precedenti

«Costruita» indica file presenti nello staging; «validata» significa riletti/decodificati; «pronta» richiede entrambe le barriere file/directory e gen-N nella radice sincronizzata. La nuova selezione diventa osservabile al rename di CURRENT. «Pubblicazione completata» richiede anche fsync radice riuscito; «recuperabile dopo crash processo» viene verificato riaprendo e validando la generazione effettivamente referenziata.

Prima del rename CURRENT resta autorevole la vecchia generazione, anche se esistono staging o gen-N più grandi. Errore durante/toccato il rename CURRENT o dopo di esso produce esito incerto: nessun successo, rollback o eliminazione dei candidati. Con OS vivo, dopo rename completato i test recuperano la nuova selezione anche se la barriera successiva fallisce; ciò non certifica reboot o power loss.

Nessuna generazione precedente viene eliminata, nessun prefisso WAL è cancellato. Staging e CURRENT.tmp falliti restano evidenze e sono conteggiati; CURRENT.tmp già presente impedisce nuova pubblicazione, senza essere troncato. Cleanup/GC non sono implementati: i fault di rimozione e fsync dopo cleanup restano non esercitabili in Fase B. La conservazione può impedire nuove pubblicazioni al raggiungimento della quota; non viene aggirata eliminando file.

## Recovery isolato e coerenza logica

OpenPersistence/Recover validano identità, quota e sicurezza dell'albero, CURRENT e soltanto la generazione referenziata; verificano QSN1, hash whole-file, base indice/termine, configurazione, WAL strutturale e transizioni hard-state/log/commit con Replay. CURRENT mancante o corrotto, file mancanti, checksum/versioni/profili discordi e incoerenze sono errori, senza fallback alla generazione più alta o precedente.

Solo una coda WAL finale strutturalmente incompleta dopo un prefisso valido può essere troncata all'offset verificato e sincronizzata, secondo M03. La validazione snapshot/profilo precede la riparazione; corruzione completa resta fatale. File completi sopravvissuti sono risincronizzati. Errori di recovery non espongono macchina, confine o log parziali; il proprietario aperto diventa failed. I file conservati non vengono cancellati.

DiskRecovery restituisce una nuova Machine a Point=S/termS e il log-level State validato con hard state, C e suffisso. **Nessuna entry WAL è applicata.** Un commit superiore a S non viene presentato come già applicato: le prove osservano la mappa al solo S e conservano il suffisso separato. Non è recovery integrato snapshot+suffisso né avvio operativo del nodo; nessun listener, quorum o ack client viene autorizzato dal risultato.

Il percorso cattura→codec→file→CURRENT→lettura→Restore conserva byte KV, presenza-vuota, sequence, fingerprint, tipo, risultato e indice originale. Test con PUT/DELETE, read/noop, CAS riuscita/fallita, retry e sequenza obsoleta confrontano immagini esatte e ripetono il recovery. Il file al prefisso 4 conserva B anche dopo che la macchina originale ha raggiunto D al prefisso 10. Retry dopo restore conserva indice 6 per PUT e 3 per CAS fallita; conflitti e sequenze obsolete restano rifiutati. Un caso sintetico persistito a 1024 sessioni conferma cap senza eviction e avanzamento ammesso di un client esistente.

## Matrice fault injection

| Punti verificati | Selezione recuperabile attesa |
|---|---|
| generation.before-create; snapshot.before-create | Vecchio CURRENT; nessuna nuova selezione. |
| snapshot.written; snapshot.synced | Vecchia generazione; staging conservato. |
| wal.before-create; wal.written; wal.synced | Vecchia generazione; nessun WAL incompleto pubblicato. |
| generation.before-rename; generation.renamed | Vecchia generazione, anche con candidato finale presente. |
| current.before-publication; current.before-create | Vecchio CURRENT autorevole. |
| current.written; current.synced; current.before-rename | Vecchio CURRENT; CURRENT.tmp non è autorità. |
| current.renamed; current.directory-synced | Nuovo CURRENT referenziato; errore iniettato senza successo/rollback. |
| Create permission, partial ENOSPC, zero write, Sync EIO, Close EBADF e byte corrotti su snapshot/WAL/CURRENT.tmp | Errore esplicito prima del cambio di selezione; vecchia generazione recuperabile. |
| Write corte riuscite su ciascun file | Nessun byte duplicato, nuova generazione valida. |
| Rename generazione/CURRENT e fsync staging/radice prima/dopo CURRENT falliti | Nessun successo; nuova selezione solo se rename CURRENT è completato. |
| Uscita di due processi figli del test prima/dopo rename CURRENT | Recovery vecchio/nuovo esatto e rilascio LOCK. |
| Cleanup, switch writer/RAM, epoche/installazione runtime | Non implementati; evidenze ancora mancanti. |

16 cutpoint deterministici, 21 combinazioni file/errore incluse tre write corte riuscite, cinque errori rename/directory Sync e 26 varianti recovery invalido. Uscita processo 77 limitata al binario figlio del test; nessun kill di processi esterni. L'iniezione usa primitive reali con wrapper privati del test, non estende il simulatore M12 né crea hook pubblici del runtime.

## Verifiche eseguite e risorse

Superati suite completa M01–M12 e Fase A/Fase B con copertura e timeout 75 s/package, poi nuovamente l’intero package snapshot dopo l’ultimo irrigidimento dei controlli dei file. Race mirato superato su snapshot, storagefs, wal, applier, committed e storageformat; rieseguito snapshot dopo l’ultimo controllo della generazione. Gofmt senza file pendenti, go vet e build GOOS=darwin/GOARCH=arm64 superati. Verificatore fixture canoniche superato (aritmetica dei vettori, distinto dai test Go di recovery), nessuna fixture modificata. go list -m all restituisce soltanto il modulo del progetto, nessun go.sum. Verifica bilingue: 140 Markdown, 69 coppie linguistiche, collegamenti/tabelle/ID/blocchi tecnici senza errori. Controllo diff e perimetro: soltanto due nuovi file Go isolati, nessun import runtime di internal/snapshot e HEAD invariato.

11 nuove funzioni test principali, di cui una helper processo, nel nuovo file; regressioni Fase A incluse. Coperti anche metadati/cancellazione/ammissione, no fallback, recovery all-or-nothing, errore persistente del proprietario, esclusione concorrente/Close, quota e nomi/orfani/overflow, riparazione coda ammessa e confine MaxUint64.

Copertura statement del package snapshot sull’ultima suite isolata: 88,9%; nuovo persistence.go 87,5% (272/311 istruzioni), memory.go Fase A 93,5% (87/93). Profili temporanei `/tmp/quorumkv-m13b.cover` per regressione completa e `/tmp/quorumkv-m13b-snapshot-final.cover` per il package aggiornato. La copertura non è una prova di consistenza. Nel caso sequenziale a tre generazioni il picco di contenuti apparenti old+new+staging/controlli osservato ai cutpoint è 5799 byte, oltre alla riserva controllo di 65536 byte; la fixture persistita a 1024 sessioni misura 227653 byte. Queste sono misure dei soli casi sintetici, non picchi generali del profilo o occupazione fisica SSD.

Cap immutati: snapshot 8 MiB, WAL 8 MiB, directory 32 MiB e riserva controllo 64 KiB; preservata nel profilo WAL la riserva consenso 512 KiB, senza nuova API proposal. Publish riserva l'intera immagine, WAL e CURRENT.tmp oltre ai file già presenti prima di costruire; nessun append concorrente ammesso dal proprietario isolato. I test iniettano contatori di quota e usano file sparse per dimensioni apparenti fuori limite, senza riempire l'SSD. Il picco logico conservato viene contato, non il consumo fisico APFS.

Budget aggiuntivo 0 €; solo libreria standard, offline, nessun download/cloud/GPU/VM/Docker. Nuovi casi normalmente pochi KiB; una fixture sintetica con 1024 sessioni è più grande ma limitata, senza ripetizione massiva; file sparse non sono serializzazioni da 8/32 MiB. Timeout 40 s package isolato e 75 s regressioni, GOMAXPROCS=2, package seriali, directory temporanee e processi del run con cleanup. Nessun cluster permanente, stress prolungato, riavvio OS o interruzione alimentazione. RSS/CPU e spazio fisico SSD non misurati.

## Gate e criteri ufficiali

| Gate / criterio | Esito Fase B | Evidenze ancora necessarie |
|---|---|---|
| C-01 / OQ-05 | RESOLVED; conformità QSN1, IDENTITY v2 e CURRENT v1 conservata/verificata. | Nessuna nuova versione o migrazione. |
| C-02 / OQ-01 | Pubblicazione/recovery isolati VERIFIED nel profilo locale; gate complessivo per integrazione runtime CONDITIONALLY READY. | Switch writer, handoff finale vivo, cleanup e prove runtime. |
| C-03 | CONDITIONALLY READY; coppia KV/sessioni e hard state finale conservati in isolamento. | Epoche, completamenti obsoleti, installazione atomica e safety nel cluster. |
| T-015 / FR-015 / AC-015 | Sottoinsieme codec/persistenza/restore e dedup verificato. | Follower arretrato, chunk/retry, cambio termine/leader e installazione; criterio aperto. |
| T-016 / FR-016 / AC-016 | Cutpoint pubblicazione e recovery processo isolato verificati. | Cleanup, compaction, switch live e crash cluster dopo successo; criterio aperto. |
| T-026 / NFR-006 / AC-026 | Quote/riserve/contabilità vecchi+nuovi+staging e limiti sessioni verificati. | Ammissione/code/riserva nel runtime nuovo, cleanup bloccato e report RSS/CPU della roadmap; criterio aperto. |

Le definizioni ufficiali restano quelle di [test](testing-strategy.md) e [accettazione](acceptance-criteria.md). Una generazione isolata recuperabile non certifica compaction, InstallSnapshot, recovery combinato o successi client dopo crash del cluster.

## Problemi, limiti e decisione

Nessun difetto M01–M12/Fase A dimostrato richiede modifiche. In sviluppo è stato corretto un riferimento del test alle costanti degli errori sessioni, che appartengono a committed, non a statemachine. La revisione ha inoltre reso espliciti il rifiuto di hard link/file aggiuntivi e l'avvelenamento del proprietario dopo recovery fallito. Nessun requisito o fixture storica adattato per far passare i test.

Le syscall filesystem possono bloccare indipendentemente dal context; nessuna promessa di cancellazione I/O. Il LOCK è cooperativo e richiede directory privata senza modifiche esterne. File.Sync e directory fsync riusciti verificano il protocollo nel modello processo con OS vivo, non atomicità fisica multi-file, resilienza OS/alimentazione o perdita SSD. Le copie e gli hash non sono una prova formale di storia applicativa/consenso.

**Fase B: PERSISTENCE PHASE COMPLETED. M13: PARTIALLY COMPLETED.** C-01/OQ-05 RESOLVED; nuove evidenze isolate C-02 verificate, integrazione C-02 e C-03 CONDITIONALLY READY; T-015/T-016/T-026 aperti. Un successivo incarico può autorizzare recovery snapshot+suffisso e handoff runtime con token/epoche/barriere, dopo revisione delle rispettive prove. Attivazione automatica, compaction/GC e InstallSnapshot restano non implementati e richiedono autorizzazione ed evidenze dedicate. M14 non iniziata; nessun commit Git.
