# M10 — GET linearizzabile tramite entry lettura nel log

[Indice](README.md) · [English](../en/milestone-m10.md)

**22 novembre 2026 — COMPLETED nel perimetro ufficiale M10. M11 NON INIZIATA.**

## Scope ufficiale, requisiti e gate

Entrambe le [roadmap](implementation-roadmap.md) complete definiscono M10 come GET linearizzabile tramite barriera read nel log e storia vecchio leader, T-019. Requisiti applicabili FR-019/AC-019/T-019 e porzione GET FR-009/AC-009/T-009; evidenza recovery processi tramite GET complementa NFR-003/AC-023/T-023. M11 possiede CAS HTTP, sessioni replicate e deduplicazione. Snapshot/compaction restano M13, checker generale storie/proprietà M14 e misure M15. Nessun ReadIndex, lease, fallback locale stale, retry automatico o exactly-once. Nessuna riassegnazione roadmap o nuova contraddizione scope bloccante.

C-01 conserva contratti canonici/config/peer e HTTP M08 immutabili, estesi da artefatto OpenAPI M10. C-02 resta contratto locale Darwin/APFS crash processi testato, non garanzia OS/alimentazione. C-03 acquisisce evidenze M10 slot esatto/lettore obsoleto/controesempio; obblighi successivi sessioni/snapshot/checker generale aperti. [ADR-006](adr/ADR-006-linearizable-reads.md) adotta strategia letture nel log già scelta per questa baseline.

## Revisione prerequisito M09

Revisione Git iniziale 4062142 pulita. Esaminati lifecycle eseguibile/server/nodo effettivi, configurazione/identità/LOCK, recovery prima di servire, cleanup su errore bind/start, segnali, attese processo limitate e cleanup figli. Esaminati relazioni M07–M09, contratti API/consistenza/Raft/storage/guasti, audit architettura e ADR pertinenti. Suite completa M01–M09 superata prima delle modifiche codice.

M09 riacquisisce ownership esclusiva, valida CURRENT autorevole e WAL, ricostruisce solo prefisso coperto da checkpoint su macchina M01 nuova tramite M02 e parte follower. Suffisso sopravvissuto non dedotto committed. Test esistenti verificano recovery termine/voto/log/checkpoint, terminazione follower/leader/tutti processi, catch-up, byte entry confermate/replay effettivo, bootstrap/corruzione invalidi, attese cancellate, porte e lock. Shutdown conserva ownership storage fino a drain effettivo e segnala fallimento se attesa processo scade. Test Raft esistenti voto/elezione, log matching, risposte obsolete, commit termine corrente, backlog e certezza HTTP nuovamente superati. Nessun difetto bloccante M09 o correzione bug produzione precedente necessari. Nessuna garanzia OS/alimentazione aggiunta.

## Implementazione e ownership

Core.EnableLoggedReads e Options.Reads selezionano esplicitamente M10 prima di EnableCommit/replay/eventi; Reads richiede Commit. node.StartAPI produzione ed eseguibile la abilitano per ogni membro. StartCommitted/modalità commit-only legacy conservano rifiuto read e fixture originali. È capacità startup omogenea, non negoziazione rolling; vecchi runtime non riaprono log contenenti read. Hash configurazione e byte wire/storage v1 invariati poiché read(key) era schema già approvato.

Service.Read usa ticket proposte limitato, owner Raft unico e controllo completamento esistenti. Appende read(key) fresca del termine corrente dopo invocazione. Solo Sync proposta locale non è completamento; richiesti replica normale maggioranza durevole correlata, regola commit termine corrente e checkpoint commit sincronizzato. Stessa entry read può soddisfare commit termine corrente e coprire prefisso vecchio. Ack precedente invocazione non copre il nuovo indice. Nessun secondo algoritmo quorum letture o accesso mappa diretto da handler.

committed.ProjectWithReads proietta read canonica in Get M01 nel worker M02 invariato. Risultato acquisito a quell’esatto indice prima di mutazioni successive, conservato con indice/termine e copiato alla consegna. LastApplied avanza solo dopo completamento future effettivo; ScheduledApplyIndex distinto. Service.Read attende tale risultato e attraversa controllo owner leadership/termine/commit/apply serializzato esistente. Livello HTTP ricontrolla conservativamente stato pubblicato/deadline; non sostituisce ruolo locale a prova quorum. Step-down osservato prima di autorizzazione produce unknown. Consegna TCP non atomica con cambi ruolo e quorum passato legale mai revocato.

Letture condividono 64 chiamate servizio/proposte accodate attive, 64 messaggi controllo finale, 64 risultati conservati e 32 connessioni client accettate esistenti; capacità peer resta separatamente limitata. Massimo 64 valori lettura conservati di 16 KiB danno limite payload 1 MiB, esclusi normali overhead oggetti/code e altre copie. Nessuna tabella waiter illimitata, worker per read o nuova dipendenza. Backpressure, cancellazione e shutdown riusano percorso esistente. Rifiuto riserva disco prima di registrazione è not_admitted; possibile errore disco/checkpoint è unknown e non espone valore.

## Interfacce HTTP e Go

POST /v1/kv/get accetta solo key obbligatoria e timeout_ms intero canonico opzionale 50..5000, default 2000. Nessun client_id/sequence, expected/value o request_id chiamante. Cap body 49.152 byte; chiavi grammatica ASCII 256 byte esistente. Controlli rigorosi nomi duplicati/UTF-8/surrogate/case/campi sconosciuti/documenti successivi/profondità/numeri precedono proposta. Timeout/correlazione trasporto mai persistiti nel comando read. HTTP GET sul path KV è 405; GET locale /v1/status invariata diagnostica obsoleta.

Chiave presente restituisce 200 OK/completed con index stringa canonica, exists=true e value_b64 canonico; presente-vuoto è stringa vuota. Assenza restituisce 404 KEY_NOT_FOUND/completed con index ed exists=false, omettendo value_b64. Nessuno dei due muta KV. Follower/candidate restituisce NOT_LEADER/NO_LEADER con soli hint configurati; niente forwarding/redirect. Timeout dopo possibile registrazione è 504 TIMEOUT/unknown, perdita leadership osservata/cancellazione/errore infrastrutturale 503 UNAVAILABLE/unknown ed errore storage 507 STORAGE_UNAVAILABLE/unknown. Unknown/not_admitted omettono campi risultato read. Esclusione provata prima di registrazione conserva semantica M08.

Client.Get invia POST singola limitata a membro configurato e valida forma risultato, indice/Base64 canonici, stato/esito e hint configurati. Mai retry automatico. GET ripetuta manualmente è nuova operazione successiva e può vedere altro valore. Cancellazione/disconnessione abbandona attesa senza annullare applicazione committed. Nessuna garanzia risultato originale stabile, sessione o exactly-once. Schemi/comportamento PUT/DELETE invariati; CAS ancora indisponibile.

## Test ed evidenze indipendenti

Dieci nuovi test principali: cinque raft, tre API client, uno nodo e uno scenario processi. Test servizio bloccato committa read indice 3 mentre scrittura successiva a 4 committa ma apply è bloccato. Nessuna risposta read prima di completamento M02 effettivo; anche quando scrittura successiva finisce prima al chiamante, lettura ritardata restituisce proprio valore precedente. Mutare buffer risultato non cambia output conservato. Lettura successiva vede nuovo valore e read mancante restituisce KEY_NOT_FOUND. Cancellazione e demotion osservata a termine maggiore producono unknown mentre apply committed finisce. Rifiuto riserva/errore storage incerto proposta ed errore checkpoint non producono lettura/applicazione/valore prematuri.

Simulazione T-019 a tre nodi con eventi espliciti usa seed 42/117/203, ogni esecuzione ripetuta con tracce identiche. Transizioni core reali, disco fake sincrono e lavoro M02 effettivo conservano ghost committed permanente; modello lista/stringhe indipendente verifica output read a ogni slot esatto. Isola completamente n1 e scarta traffico in volo prima della nuova read vecchio leader, elegge n2 sulla maggioranza e vi scrive/legge valore più nuovo. Vecchio leader localmente si crede ancora leader, ma nuova read non committa/applica né supera controllo completamento. Read separata committed prima di isolamento può consegnare risultato acquisito. Riconnessione demote/sostituisce ricevuta incerta; replay nuovo prefisso committed contenente read conserva risultato indicizzato corretto. Seed variano ordine messaggi; perdita per partizione esplicita. Regressioni precedenti ack ritardati/duplicati/obsoleti invariate, senza dichiarare nuovo framework generale guasti rete.

Test HTTP coprono risultati presente/binario/vuoto/assente, cap body esatto/cap+1, campi/numeri invalidi rigorosi, assenza campi sessioni, forme timeout unknown, metodi e risposte avverse. Nove [fixture GET](../fixtures/http-m10.json) verificate contro [OpenAPI 3.1](../openapi/m10.json), proiezione runtime, stato/body handler effettivo e decoder test client. Artefatto M10 contiene tutti i tre schemi path M08 originali equivalenti a livello oggetti JSON; [artefatto M08](../openapi/m08.json)/fixture originali invariati. Validatore schema finito offline esistente include controlli negativi ed estensioni canoniche runtime; nessuna certificazione OpenAPI esterna dichiarata.

Cluster loopback reale verifica rifiuto follower, valore vuoto, cinque operazioni PUT/GET/DELETE sovrapposte/sequenziali, read-after-write completed su failover e GET minoranza isolata scaduta unknown senza campi valore. Ricerca esaustiva indipendente limitata a una chiave verifica precedenza tempo reale e risultati osservati per tale piccola storia; controesempio stale noto deve essere rifiutato. È checker specifico test, non checker M14 completo sessioni/CAS/snapshot o prova universale linearizzabilità.

Nuovo scenario tre processi scrive dati, esegue GET presenti/assenti, uccide bruscamente tutti server, riavvia directory integre contenenti read, esegue nuove GET presenti/assenti, poi DELETE seguita da GET assenza. Nessuna lettura locale in cache o risposta HTTP basata solo su replay evita nuova barriera quorum. Test M09 esistenti crash follower/leader/tutti processi, avvio, shutdown e risorse anch’essi superati. Massimo tre nodi temporanei, dati piccoli e timeout/cleanup espliciti; nessun cluster resta attivo.

## Verifiche effettive e copertura

Go 1.27.4 darwin/arm64, profilo locale macOS/APFS esistente. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, race 1 con compilatore esistente. Controlli equivalenti effettivi:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m10.cover -timeout=60s ./...
go test -race -p 1 -count=1 -timeout=60s ./internal/raft ./internal/clientapi ./internal/node ./internal/server ./internal/committed
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Tutti superati, inclusa regressione completa M01–M09; race senza segnalazioni nei percorsi esercitati. cmd/quorumkv 0.0%; internal/applier 100.0%; internal/clientapi 82.4%; internal/committed 93.3%; internal/config 89.2%; internal/node 81.7%; internal/peerprotocol 91.4%; internal/raft 88.1%; internal/server 90.2%; internal/statemachine 100.0%; internal/storageformat 92.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. Copertura statement aggregata tra binari test parent non copre ogni scheduling, esecuzione main standalone/solo figli né prova consenso/linearizzabilità. Esecuzione HTTP iniziale con loopback ristretto non poteva aprire porte; esecuzione rete locale autorizzata superata senza workaround codice. Nessun benchmark, stress prolungato, misura CPU/RSS/usura SSD, test crash OS o perdita alimentazione. Nessun nuovo modulo/go.sum; checker aritmetica fixture originali superato. Controlli locali struttura/link/esempi bilingui e revisione semantica superati; nessuna certificazione esterna.

## File, accettazione e consegna

Modificati internal/raft/core.go, commit.go, replication.go, service.go, application.go e write.go; internal/committed/project.go; internal/clientapi/api.go e client.go; internal/node/node.go. Creati internal/raft/read_test.go e read_simulation_test.go, internal/clientapi/read_test.go, internal/node/m10_test.go e internal/server/m10_test.go; docs/openapi/m10.json e docs/fixtures/http-m10.json; queste relazioni. Aggiornati documenti bilingui API/consistenza/architettura/Raft/storage/guasti/test/accettazione/requisiti/tracciabilità/sviluppo, ADR-006, indici e roadmap. Produzione M01/M02, WAL/filesystem/config/parser/trasporto e precedenti fixture/test invariati. Nessun bug precedente o problema bloccante trovato; nuovo livello riusa confini verificati.

FR-019/AC-019/T-019 e GET FR-009/AC-009/T-009 superati in M10, con recovery processi tramite GET che complementa AC-023/T-023. CAS/dedup/AC-012 restano M11, snapshot M13, AC-022/T-022 finali/verifiche safety generali M14. Obbligo M10 C-03 soddisfatto senza chiudere obblighi successivi. Risultati read possono scadere da conservazione limitata e produrre unknown; cap storage può fermare nuove letture poiché scrivono WAL. Tutti membri richiedono stessa capacità startup. Crash OS/alimentazione, indipendenza fisica host condiviso, maggioranza durevole corrotta/persa e stalli kernel arbitrari conservano limitazioni precedenti.

**M10 COMPLETED. M11 NON INIZIATA.** Base letture ordinate/recovery pronta per revisione/incarico M11 separati; nessuna CAS HTTP o sessione/deduplicazione replicata implementata. Spesa aggiuntiva **0 €**, standard library offline, package seriali/due processori, piccoli test limitati e soli processi nodo temporanei. Nessun servizio/download a pagamento, GPU, VM/container obbligatorio, scritture SSD massive o cluster permanente. Nessun commit Git. Stop a M10.

Verifica documentazione: 128 file Markdown, 63 coppie lingua, 1.723 link relativi, 106 tabelle e 31 esempi tecnici accoppiati; zero errori strutturali.
