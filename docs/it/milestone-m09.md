# M09 — Recovery processi e MVP interno

[Indice](README.md) · [English](../en/milestone-m09.md)

**22 novembre 2026 — COMPLETED nel perimetro ufficiale M09. M10 NON INIZIATA.**

## Scope ufficiale e prerequisiti

Entrambe le [roadmap](implementation-roadmap.md) complete definiscono M09 come crash recovery/MVP interno: tre nodi locali, PUT/DELETE, replica durevole, replay prefisso committed e restart. Gate assegnati FR-014/AC-014/T-014, FR-018/AC-018/T-018 e sottoinsieme NFR-003/AC-023/T-023 che permette ispezione controllata modello/applier senza GET HTTP. AC-023 completa basata su GET rinviata dopo M10, snapshot/rotazione a M13. GET/barriere nel log sono M10; CAS HTTP, sessioni replicate e deduplicazione M11. Nessuna discrepanza scope o riassegnazione roadmap necessaria; sezioni GET/CAS dell’incarico condizionali e non applicabili.

Revisione Git iniziale 6e30675 pulita. Esaminati sorgenti/relazione/OpenAPI/fixture M08, entrambe roadmap, contratti API, consistenza, Raft, guasti/storage/configurazione, interfacce pertinenti M01–M07, accettazione e ADR pertinenti. C-01 conserva sottoinsieme verificato M08, C-02 modello locale Darwin/APFS testato; obblighi C-03 successivi letture/sessioni/snapshot/checker aperti. Tutte regressioni M01–M08 superate prima delle modifiche.

## Revisione M08 e safety degli esiti

Esaminati validazione/proiezione rigorose, ammissione limitata, confine registrazione ticket, append locale sincronizzata, commit maggioritario durevole del termine corrente, applicazione M02 esatta, correlazione risultato indice/termine e autorizzazione completamento serializzata owner. Snapshot HTTP finale è supplemento conservativo, non prova quorum indipendente. Perdita leadership successiva non osservata non revoca commit passato legale e cambi ruolo non possono essere atomici con consegna TCP. Nessun successo basato su sola ammissione applier o ruolo locale.

Prima di possibile registrazione, cancellazione esclude permanentemente ticket accodato sotto lock; not_admitted giustificato solo per quel tentativo. Dopo possibile append, timeout/disconnessione/step-down/errore è unknown; applicazione successiva resta possibile. RESOURCE_LIMIT dominio è risultato completed deterministico, distinto da errore infrastruttura. Perdita trasporto client è unknown e mai ritentata automaticamente. Stessa identità significa ancora tentativi indipendenti fino a M11. Test esistenti disco/apply bloccati, perdita leadership, socket perso, quorum/minoranza, cancellazione/shutdown e fixture schema superati invariati; race mirato nuovamente superato. Nessun difetto bloccante M08 o correzione produzione precedente identificati. Nessun comportamento HTTP/OpenAPI M08 alterato.

## Componenti implementati e lifecycle

[cmd/quorumkv](../../cmd/quorumkv/main.go) è piccolo adattatore segnali produzione. [internal/server](../../internal/server/run.go) possiede validazione argomenti/configurazione, node.StartAPI, attesa processo, diagnostica fissa redatta e shutdown limitato. Invocazione canonica quorumkv node -config FILE [-bootstrap]; accetta anche flag diretti senza node. -h/help esce 0; argomenti/config invalidi exit 2; errore avvio/runtime/shutdown exit 1. Lettura config limitata a 16 KiB e usa config.Parse invariato. Nessun bootstrap implicito, reset, riconfigurazione, retry automatico, CLI sessioni client o nuovo endpoint.

Tre [configurazioni esempio](../../examples/n1.json) condividono membership/hash statici esistenti e hanno directory dati private distinte. [Guida sviluppo](development-guide.md) fornisce istruzioni build, creazione directory private, primo bootstrap, restart senza bootstrap e stop. Percorsi relativi alla directory lavoro processo salvo assoluti. .gitignore esclude .local e binario build radice. Nessuna directory dati creata silenziosamente dall’eseguibile.

Node.Done espone ora teardown nodo completato, incluso errore automatico listener/servizio. Chiude solo dopo stop listener/worker/applicazione e rilascio WAL/LOCK. Eseguibile osserva cancellazione segnale o evento terminale, attende massimo 5 s Shutdown esistente e segnala fallimento/recovery-required su errore/timeout. Errore scrivendo node_started chiude anch’esso nodo appena aperto. node_started significa storage verificato e listener prenotati, non prontezza quorum/elezione. Diagnostica omette config grezza, valori utente e contenuto errori filesystem.

## Architettura recovery e consistenza

Algoritmo recovery produzione riusato, non duplicato: acquisire LOCK stabile; validare identità/config e CURRENT autorevole; leggere/validare streaming WAL e hard-state/log/commit monotoni; ricostruire macchina M01 nuova tramite M02 esattamente su 1..CommitIndex recuperato; pubblicare stato follower con ScheduledApplyIndex/LastApplied ricostruiti; poi servire listener configurati e avviare elezione/replica normale. Nessun cursore applied persistito, feature snapshot o accesso mappa HTTP alternativo aggiunto. Regole M03 fail-closed corruzione completa e code incomplete ammissibili invariate.

Suffisso log sopravvissuto non prova commit. Escluso dal replay anche se ogni nodo lo contiene. Dopo nuova elezione, noop quorum termine corrente può legittimamente committare suffisso ereditato; compatibile con esito unknown originale. Prima di tale nuovo confine legale, comandi speculativi non devono apparire nello stato ricostruito. Scritture completed confermate conservate nel modello testato crash processi non bizantino con directory integre e maggioranza durevole recuperabile.

SIGINT/SIGTERM rifiuta ammissione client e cancella waiter HTTP, mentre ownership Raft/applicazione esistente finisce persistenza iniziata e drena obbligo committed durevole prima di rilasciare risorse. Client può perdere risposta durante shutdown: mai autorizza rollback o fallimento definitivo. Waiter shutdown cancellato limita solo attesa. Se processo esce infine dopo limite 5 s, avvio successivo recupera prefisso valido sopravvissuto. Syscall filesystem OS bloccata durante startup non interrompibile forzatamente tramite contesto Go; nessun limite arbitrario latenza I/O promesso.

## Test aggiunti e risultati osservati

Otto nuovi test principali attivi, più entry point processo figlio saltato normalmente. Cinque test comandi/lifecycle, due scenari processi reali e uno recovery controllato a tre nodi. Ognuno usa attese esplicite limitate, percorsi privati temporanei, dati sintetici piccoli e massimo tre sottoprocessi nodo; package seriali. Helper processo invoca stesso server.Run produzione e semantica adattatore segnali del main, usando binario test esistente anziché ricompilare per nodo. Eseguibile standalone reale compilato separatamente e verificate uscite help/argomento invalido.

TestProcessCrashOneThenAllPreservesAcknowledgedHistory completa PUT (inclusi valori vuoti) e DELETE, uccide bruscamente follower, continua con maggioranza, riavvia/recupera follower, uccide leader e scrive tramite successore. Dopo che tutte repliche applicano confine confermato, uccide tutti e tre prima di attendere qualunque uscita. Riapertura offline confronta byte entry originali di ogni conferma salvata tra repliche e richiede checkpoint che coprano confine. Replay reale raft.NewService nuovo tramite M02 ispezionato solo dopo stop worker e confrontato con oracolo esterno mappa attesa/cancellazioni. Restart di tutte directory integre completa poi altre DELETE/PUT. Stop ordinato permette riacquisizione di ogni porta e lock WAL.

TestProcessSignalWithPendingWritePreservesUncertaintyAndRecovery ferma follower, invia scrittura deadline lunga al leader rimasto e attende crescita WAL, non sleep arbitrario, prima di SIGTERM. Risultato client unknown, suffisso speculativo sopravvive e replay effettivo conserva chiave sicura confermata escludendo speculativa. Processo solo riavviato pubblica soltanto confine checkpoint/apply recuperato; lunghezza log grezza non crea commit.

TestRestartAllExcludesSuffixUntilNewTermNoopCommits prepara tre immagini canoniche valide con termine 5, CommitIndex=1 e LastIndex=2. Ogni startup è follower/termine 5 con ScheduledApplyIndex=LastApplied=1. Solo dopo elezione/noop successivo a indice 3 può essere applicato indice ereditato 2. Verifica sia esclusione pre-elezione sia commit successivo legale, senza GET anticipata.

Test comandi/errori coprono flag sconosciuti redatti, help, config obbligatoria, JSON eccessivo/malformato, startup precancellato, restart ordinario senza inizializzazione, bootstrap ripetuto preservando hash byte, ownership attiva duplicata, corruzione WAL completa senza fallback/riparazione, hash esempi uguali/directory distinte, stop contesto durante richiesta peer incompleta ed errore stream output. Riacquisiscono porte/lock dopo terminazione. Test esistenti persistenza durante shutdown/waiter cancellati conservano evidenze M02/M03/M05–M08; non forzati I/O kernel realmente bloccato o superamento reale shutdown 5 s. Fixture invalide solo test corrette a ordine campi canonico/voto null; nessuna modifica codec produzione o rilassamento fixture nascosto.

Fixture M01–M08, errori HTTP rigorosi, risposte perse e certezza risultati restano invariati e superati. Scenari finiti verificano conservazione prefisso committed, restart follower, nessun replay speculativo, avanzamento legale termine corrente, apply ordinato e ownership risorse. Non sono prova universale Raft/linearizzabilità, test guasto host indipendenti o evidenza deduplicazione.

## Verifiche effettive e copertura

Go 1.27.4 darwin/arm64 sul profilo locale macOS/APFS esistente testato. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente e 1 con compilatore esistente per race. Controlli equivalenti effettivi:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go build -p 1 -o /tmp/quorumkv-m09 ./cmd/quorumkv
/tmp/quorumkv-m09 node -h
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m09.cover -timeout=60s ./...
go test -race -p 1 -count=1 -timeout=60s ./internal/server ./internal/node ./internal/raft ./internal/clientapi
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Tutti superati; race senza segnalazioni nei percorsi esercitati, inclusi test server processi figli. Test finale aggiuntivo lifecycle embedded eseguito anch’esso con race. cmd/quorumkv 0.0%; internal/applier 100.0%; internal/clientapi 83.6%; internal/committed 92.3%; internal/config 89.2%; internal/node 81.7%; internal/peerprotocol 91.4%; internal/raft 88.1%; internal/server 90.2%; internal/statemachine 100.0%; internal/storageformat 92.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. Copertura aggregata tra binari test parent; main standalone/esecuzioni solo figli non dichiarati statement coperti. Race finito non prova ogni scheduling o consistenza distribuita. Nessun benchmark, misura CPU/RSS/usura SSD, test crash/reboot OS o perdita alimentazione. go.mod originale invariato, nessun go.sum o nuovo modulo; runtime solo standard library. Struttura/link/esempi bilingui verificati localmente; nessuna certificazione esterna.

## File, accettazione e limitazioni

Creati cmd/quorumkv/main.go; internal/server/run.go, run_test.go, process_test.go; internal/node/m09_test.go; examples/n1.json, n2.json, n3.json; .gitignore; queste relazioni bilingui. Unica modifica produzione precedente Node.Done, canale completamento lifecycle sola lettura. Algoritmi produzione M01–M08, formati storage, byte fixture, OpenAPI M08 e schemi HTTP invariati. Aggiornati pertinenti architettura bilingue, configurazione/guida sviluppo, compatibilità API, consistenza, Raft, contratti guasti/storage, test, accettazione, tracciabilità, ADR-002/ADR-005, indici e roadmap. Rapporti storici mantengono confini originali.

FR-014/AC-014/T-014, FR-018/AC-018/T-018 e sottoinsieme MVP NFR-003/AC-023/T-023 superati. AC-023 completa basata su GET resta dopo M10; recovery snapshot M13 e verifiche proprietà finali M14. C-01/C-02 conservati, gate successivi C-03 invariati. Nessun bug bloccante noto nel perimetro implementato. Guasto fisico host condiviso, perdita/corruzione maggioranza durevole, stalli kernel arbitrari, perdita OS/alimentazione, rolling upgrade, compaction, sessioni/retry sicuri, GET/CAS HTTP e deployment produzione restano esclusi.

**M09 COMPLETED; M10 NON INIZIATA.** Base ordinamento/recovery committed pronta per revisione/incarico M10 separati; nessuna barriera lettura, ReadIndex o contratto GET implementato. Costo aggiuntivo **0 €**; esecuzione offline, soprattutto test piccoli deterministici/modello più brevi test reali processi loopback, massimo tre processi nodo temporanei e nessun cluster lasciato attivo. Nessun servizio a pagamento, download, GPU, requisito VM/container, stress prolungato o scritture SSD massive. Test puliscono dati temporanei/attendono figli; scritti solo file repository richiesti e artefatti temporanei limitati. Nessun commit Git. Stop a M09.
