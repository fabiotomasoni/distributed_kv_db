# M07 — Commit distribuito e applicazione macchina a stati

[Indice](README.md) · [English](../en/milestone-m07.md)

**20 novembre 2026 — COMPLETED nel perimetro ufficiale M07. M08 NOT STARTED.**

## Perimetro, prerequisiti e revisione M06

Entrambe le [roadmap](implementation-roadmap.md) assegnano M07 a regola commit del termine corrente e apply, FR-007/FR-008, AC-007/AC-008, T-007/T-008. Il nuovo incarico richiede esplicitamente la ricostruzione minima su macchina nuova necessaria a questo percorso; recovery completo server/processi/MVP resta M09, letture nel log M10, sessioni/deduplicazione M11 e snapshot M13. Nessuna HTTP/API client o contratto successo client aggiunti.

Baseline Git iniziale 937ec79 pulita. Rivisti entrambe le roadmap, core/servizio/nodo M06 effettivi, contratto owner/future/backpressure M02, checkpoint/recovery M03, contratti consistenza/guasti/dati, ADR applicabili e gate audit. Regressione completa M01–M06 superata prima delle modifiche. Esaminati correlazione Sent/Failed/byte richiesta/termine, match monotono, reset nuova generazione, match locale durevole e protezione suffisso follower. Nessun difetto M06 bloccante trovato; nessuna correzione codec/WAL/applier/macchina a stati precedente necessaria. Formati/fixture precedenti C-01 invariati, limiti locali Darwin/APFS C-02 conservati. Obblighi C-03 M07 backlog/prefisso committed verificati sotto; parti snapshot/dedup/stale-read/checker restano ai gate successivi.

## Algoritmo commit e persistenza

EnableCommit è capacità esplicita all’avvio sopra replica M06. Il leader sceglie il confine maggioritario dai tre match durevoli, incluso il proprio log sincronizzato. Avanza solo se il confine supera il checkpoint precedente e il termine di quell’entry coincide con currentTerm. Entry precedenti diventano coperte soltanto tramite quel confine successivo di termine corrente. Il noop del nuovo leader esistente fornisce il primo confine possibile del termine corrente. Si segue [Raft sezione 5.4.2 e figura 2](https://raft.github.io/raft.pdf); il solo conteggio di repliche di vecchio termine è deliberatamente insufficiente.

Core maybeCommit viene eseguito dopo risposta replica correlata correttamente alla leadership corrente. Richieste invalide/obsolete, vecchie generazioni, sola trasmissione, risposte di termine inferiore o maggioranza assente non creano prova commit. Risposta valida di termine maggiore declassa prima della valutazione commit. Cambi currentTerm/ruolo conservano prefisso già committed e progresso applicazione.

persistCommit emette transazione canonica commit_index esistente dopo durabilità di tutti i frame log coperti. CommitIndex cambia solo dopo completamento Sync esatto generazione/sequenza fisica. Errore checkpoint è fatale/incerto, senza nuova applicazione o risposta positiva follower dipendente. Nessun secondo checkpoint o cursore applied persistito introdotto. Checkpoint completo sopravvissuto può recuperare anche se il vecchio chiamante non ha osservato acknowledgment; M03 ne verifica il prefisso durevole necessario.

Il follower apprende commit solo da AppendEntries valida non obsoleta con prev corrispondente e suffisso entrante sincronizzato. Confine candidato min(leaderCommit, prev+len(entries)); conserva max(oldCommit,candidato). Così un heartbeat corto non committa una coda locale aggiuntiva estranea. Persistenza hard-state/suffisso precede checkpoint; risposta RPC positiva segue tutte le Sync necessarie. Valori leaderCommit duplicati o inferiori non riscrivono/regrediscono checkpoint; messaggi obsoleti/prefisso rifiutato non lo avanzano. Heartbeat/batch successivi propagano checkpoint leader sincronizzato.

## Adattatore M02 e ownership

[internal/committed](../../internal/committed/project.go) valida e proietta entry noop/put/delete/cas esistenti. Un bridge privato intercetta noop dentro il worker M02, restituendo OK senza invocare M01, così ogni indice globale avanza senza inviare opcode invalido alla macchina KV. Mutazioni usano implementazione M01 invariata tramite M02 invariato. Identità/sequence client restano metadati validati/persistiti; M07 non implementa tabella sessioni replicata o deduplicazione client tra indici.

Core possiede LastApplied, ScheduledApplyIndex e ApplyEpoch=1 per questa istanza. Invariante LastApplied <= ScheduledApplyIndex <= CommitIndex. Solo TrySubmit riuscita avanza Scheduled; solo Future M02 completata in ordine avanza LastApplied. Coda piena lascia log committed non schedulato al proprio posto. Eventi commit ripetuti riprendono da Scheduled+1, mai LastApplied+1. Cambio leadership non resetta cursori applicazione. Completamenti di epoch vecchie ignorati; snapshot/sostituzione epoch non implementati qui.

Service aggiunge un worker M02 e un collector ordinato future, senza goroutine per entry. Capacità pending M02 1..64 (default 64); massimo 64 ticket pendenti, canali ticket/completamenti limitati a 64, archivio risultati recenti a 64. Owner usa admission non bloccante mentre elabora rete/tick/completamenti; notifiche completamento riprendono scheduling senza spin o perdita. Disco bloccante resta fuori da mutex core/globali. Input/risultati copiati ai confini ownership.

Service.WaitApplied(ctx,LocalAppend{Index,Term}) restituisce risultato applicativo con indice/termine solo dopo applicazione. Cancellazione chiamante abbandona attesa, mai comando accettato. Termine diverso a quell’indice dà ErrSuperseded; risultati storici espulsi danno ErrResultExpired; errore infrastruttura rimane errore. Risultati conservati copiati e ispezionabili dopo shutdown normale. È consegna interna risultati, non exactly-once/linearizzabilità client esterna. CAS COMPARE_FAILED o altro rifiuto deterministico consuma slot applicativo committed e non viene ritentato/annullato come guasto infrastrutturale.

## Recovery e shutdown

node.StartCommitted abilita percorso M07; StartElection/StartReplication legacy conservano capacità M05/M06 e fixture tracce originali. NewService Options.Commit seleziona M07; ApplyCapacity è tuning locale limitato, MachineFactory è factory fidata opzionale che deve restituire macchina nuova finita con ownership esclusiva. Produzione crea sempre una nuova macchina M01.

Prima di listener/timer, ricostruisce solo indici 1..CommitIndex recuperato su tale macchina nuova tramite M02. Ricostruisce Scheduled/LastApplied da future completate; ignora suffisso non committed. Secondo avvio non riproduce sopra vecchia istanza mutata. Applicazione precedente al crash può rieseguirsi durante ricostruzione su mappa nuova, senza duplicare effetti sulla stessa mappa viva. Nessuna garanzia exactly-once esterna o cursore applied durevole necessari per questa macchina volatile.

Runtime M03 attuale ha checkpoint vuoto con base zero; snapshot nonzero restano non supportati. M07 rifiuta deliberatamente log/batch con entry read future anziché anticipare lettura ordinata. Richiesta capacità omogenea all’avvio; negoziazione feature/migrazione rolling non implementata. Log read legacy non supportato fallisce conservativamente senza modificarlo. Codifiche PUT/DELETE/CAS/noop invariate.

Shutdown normale rifiuta admission/cancella rete, completa scrittura iniziata, schedula/drena backlog committed durevole, termina M02/collector, poi node chiude WAL/rilascia LOCK. Chiamante con attesa 5 s cancellata segnala attesa incompleta mantenendo ownership. Su errore storage/apply fatale, admission chiude subito anche con applicazione sicura precedente ancora in drain. Lavoro worker accettato termina/abortisce secondo M02; nessun checkpoint non sincronizzato applicato, nessuna applicazione fallita/incerta ritentata sulla stessa mappa. Apply iniettato arbitrario che non termina non può essere interrotto forzatamente; owner conserva risorse fino al drain effettivo. Progresso runtime fallito è conservativo e inutilizzabile come garanzia completamento sano.

## Test ed evidenze indipendenti

14 nuove funzioni test: 12 raft, una proiezione committed, una integrazione nodo. Test core stabiliscono esplicitamente maggioranza di vecchio termine con commit invariato, poi confermano/sincronizzano noop corrente e committano prefisso; verificati maggioranza assente e un follower offline, monotonicità, risposta obsoleta, declassamento termine maggiore, cap prefisso verificato follower, checkpoint duplicato ed errore Sync. Frame suffisso devono precedere checkpoint; indici schedulati/completati ed epoch vecchie validati separatamente.

Il test backlog C-03 committa 5 poi 7 mentre prima mutazione è bloccata e capacità coda è 1. Prefisso ghost durevole confrontato byte per byte; vietati doppio scheduling, gap o lavoro perso. Rilascio worker produce esattamente sei mutazioni più un noop in ordine globale, inclusi CAS riuscita e COMPARE_FAILED applicato. Attese risultato/shutdown cancellate non rimuovono obbligo. Panic infrastrutturale fallisce conservativamente. Testati scadenza/supersessione risultati limitati ed esclusione letture M10.

Test errore checkpoint verificano nessuna applicazione/acknowledgment replica e rifiuto admission immediato mentre applicazione sicura precedente resta bloccata. Immagini WAL reali piccole tagliano checkpoint ai confini 0/20/24/finale-payload/completo, riaprono tramite M03 e riproducono nessuna entry oppure esattamente prefisso committed su macchina nuova. Riavvio ripetuto immagine completa ripete CAS-da-assente riuscita su macchina diversa; chiave secret speculativa mai presente. È interruzione modellata/recovery byte con OS vivo, non esperimento crash OS/alimentazione.

Simulazione deterministica tre nodi con registro ghost committed permanente indipendente e oracolo risultati applicativi per indice. Tre seed rete eseguiti due volte; esercitati perdita/ritardo/duplicazione, partizione, nuova elezione, vecchio leader isolato senza quorum e riavvio su macchina nuova. Scrittura speculativa vecchio leader mai committed/applicata; nuovo termine conserva dati committed precedenti. Ogni nodo applica tramite future M02 effettive e termina con stato/risultati identici, inclusa CAS fallita. [commit-v1-results.json](../fixtures/commit-v1-results.json) salva fingerprint tracce commit/apply (30 eventi per seed), digest stato finale uguali e CommitIndex=LastApplied=5. Simulatore usa eventi core espliciti con lavoro M02; backpressure asincrona servizio verificata separatamente con test runtime bloccato.

Test loopback reale tre nodi usa adattatori M07 produzione, CAS concorrenti ordinate (esattamente una riuscita), stop leader dopo commit, elezione maggioranza, altra CAS che prova stato conservato e riavvio/catch-up StartCommitted produzione. Ogni nodo fermato drena fino al proprio checkpoint durevole. Test ispezionano valori M01 solo dopo termine owner M02. Test M01–M06 precedenti, 20 seed elettorali/100 heartbeat, seed replica, lifecycle e regressione sequenza fisica WAL invalida invariati e superati.

Test finiti verificano commit quorum termine corrente, conservazione prefisso committed, nessun apply speculativo, applicazione esatta ordinata, cursori monotoni e stato replica corrispondente. Non costituiscono prova universale Raft/linearizzabilità, checker storie client esterne o evidenze safety snapshot.

## Risultati verifiche effettive

Go 1.27.4 darwin/arm64, profilo macOS/APFS locale esistente. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, 1 con compilatore esistente per race. Comandi equivalenti effettivi:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m07-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node ./internal/committed ./internal/applier ./internal/wal ./internal/transport
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Tutti superati. Race senza segnalazioni nei percorsi esercitati; raft/node rieseguiti dopo protezione finale admission fatale. Copertura istruzioni aggregata tra binari test: raft 88,5%, committed 92,3%, node 81,2%; applier/statemachine 100%, config 87,3%, peerprotocol 91,4%, strictjson 92,1%, transport 90,3%, storageformat 92,3%, storagefs 87,9%, WAL 82,7%. Copertura non è percentuale singolo binario stampata da -coverpkg né prova di ogni schedule. Nessun benchmark, guasto fisico o misura risorse dichiarati. Fixture originali/go.mod identici byte per byte, nessun go.sum/dipendenza; verificatore aritmetico esistente superato.

## File, accettazione e readiness successiva

Creati internal/raft/commit.go, application.go e commit_test.go, application_test.go, commit_recovery_test.go, commit_simulation_test.go; internal/committed/project.go e project_test.go; internal/node/m07_test.go; docs/fixtures/commit-v1-results.json e relazioni bilingui. Estesi stato core/hook replica, lifecycle applicazione servizio e startup/consegna risultati nodo. Fake storage memoria esistente ora registra transazioni commit. Produzione M01/M02/WAL/codec/filesystem/trasporto/configurazione/parser invariata. Nessun bug precedente bloccante trovato; revisione finale protegge nuovo percorso admission fatale/drain con test mirato.

FR-007/FR-008, AC-007/AC-008, T-007/T-008 e obbligo C-03 M07 backlog soddisfatti entro questa fase. Nessun bug bloccante noto rimane. Recovery completo processi/server/MVP M09, handoff/epoch snapshot M13, barriere lettura M10, dedup M11 e gate finali safety/storie indipendenti restano aperti. Stabilità API/OpenAPI C-01 ancora dovuta a M08.

**M07 COMPLETED.** M08 può rivedere ricevute locali durevoli interne e risultati applicati, ma deve aggiungere propri contratto HTTP/errori PUT/DELETE ed evidenze OpenAPI. Nessun codice M08, API client/read pubblica, garanzia sessione, ReadIndex, snapshot avanzato o deploy cloud introdotti. Aggiornati contratti/indici/ADR/stato bilingui pertinenti conservando evidenze fasi storiche. Nessun commit Git.

Budget aggiuntivo **0 €**. Solo toolchain esistente con libreria standard; runtime/test offline con file privati temporanei/porte loopback dinamiche e massimo tre nodi ordinari. Due processori, pacchetti seriali, deadline pacchetto <=30 s, attese limitate <=5 s; prevalentemente scenari memoria piccoli. Nessun pagamento, account, download, GPU, VM/container, scrittura massiva, stress prolungato, modifica personale/sistema o misura hardware inventata. Consultato paper Raft pubblico come fonte normativa; esecuzione locale non richiede servizi esterni. Stop a M07.

Verifica documentazione: 122 file Markdown, 60 coppie lingua, 1.561 link relativi, 106 tabelle e 26 coppie esempi tecnici non-Mermaid identici; zero errori strutturali. Verificati link reciproci lingua, parità titoli/ID, JSON e fence; equivalenza semantica rivista localmente. Nessuna certificazione esterna o verifica Mermaid renderizzato.
