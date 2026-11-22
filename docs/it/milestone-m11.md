# M11 — CAS HTTP e deduplicazione delle richieste

[Indice](README.md) · [English](../en/milestone-m11.md)

**22 novembre 2026 — COMPLETED nel profilo sessioni v2. M12 NON INIZIATA.**

## Perimetro ufficiale e revisione M10

Entrambe le [roadmap](implementation-roadmap.md) assegnano a M11 CAS HTTP e sessioni persistite/deduplicazione, T-010/T-011/T-012: FR-010/AC-010/T-010, FR-011/AC-011/T-011 e FR-012/AC-012/T-012. M12 mantiene fault injection generale, manifest riproducibili e osservabilità; snapshot/compattazione M13, checker generale M14 e misure M15. Nessuna riassegnazione del perimetro. C-01 riceve le nuove fixture versionate; C-02 conserva il solo contratto locale macOS/APFS verificato; C-03 soddisfa l'obbligo M11 senza chiudere snapshot/checker successivi.

Baseline Git pulita 3dd071a. Rivisti contratti, ADR, relazioni M08–M10 e percorso GET: ammissione limitata, append corrente, persistenza, quorum durevole, checkpoint, M02, risultato nello slot esatto e controllo completamento owner. PUT A / GET / PUT B conserva il risultato dello slot GET anche con consegna tardiva. Nessuna lettura locale sostitutiva; vecchio leader isolato e quorum assente non autorizzano nuove letture definitive. Regolarità di cancellazione, cambio termine e replay mantenuta. Suite M01–M10 superata prima delle modifiche e nuovamente alla fine. Nessun difetto bloccante M10 o correzione di bug preesistente individuato.

## Compatibilità e decisione architetturale

[ADR-010](adr/ADR-010-session-profile.md) documenta una necessità verificabile: i log v1 M08–M10 possono contenere identità ripetute con effetti distinti già confermati. Deduplicarli retroattivamente perderebbe dati logici. M11 richiede config_version=2/application_version=2, hash condiviso che include il profilo e IDENTITY sigillata format_version=2. Bootstrap esplicito su directory nuove; nessuna migrazione automatica, reset o conversione in-place. Il profilo v1 continua con semantica M10. Un'apertura incrociata fallisce prima di replay o riparazione della coda WAL. Avvio parziale v2 senza API/sessioni rifiutato.

Entry, digest QCMDv1, frame/transazioni WAL, CURRENT e snapshot vuoto mantengono i codec v1 originali. Nessuna modifica alle fixture precedenti. I peer misti hanno hash incompatibili. Le mutazioni v2 richiedono esattamente un header X-QuorumKV-Application-Version: 2; il server conferma il profilo nella risposta. Il client verifica la conferma prima di fidarsi del risultato: vecchi binari potrebbero ignorare header nuovi. Header assente/errato/multiplo è rifiutato prima dell'ammissione; v1 rifiuta il nuovo header. GET mantiene corpo e contratto M10. [Fixture profilo](../fixtures/configuration-v2-sessions.json) ed esempi examples/m11 separano anche cluster ID, porte e directory.

## CAS e contratto HTTP

POST /v1/kv/cas ordina il comando canonico esistente. Confronto e modifica sono un'unica transizione M01 attraverso M02, senza GET/confronto/PUT nel gestore. expected distingue chiave assente da presente con valore vuoto. Successo: 200 OK/completed/applied=true. Confronto negativo: 409 COMPARE_FAILED/completed/applied=false, entry regolarmente committed. RESOURCE_LIMIT è un risultato applicativo negativo memorizzato. CAS di client distinti con valori nuovi diversi e stesso expected non possono entrambi modificare con successo; CAS che riscrivono lo stesso valore possono entrambi riuscire nell'ordine legale. Nessuna soluzione al problema ABA.

Validazione rigorosa prima della proposta: chiave ASCII massima 256 byte, valori massimi 16 KiB, Base64 canonico, expected discriminato, campi esatti, identità/sequenza canoniche, timeout_ms 50..5000 (default 2000), corpo massimo 49152 byte. Dimensione massima richiesta CAS canonica verificata 44116 byte; entry massima 44184, compatibile con il limite M02 invariato. Follower/candidate non inoltra richieste. not_admitted prova esclusione prima della registrazione; dopo possibile append errori, timeout, cancellazione, shutdown e perdita leadership restano unknown. Unknown non contiene un risultato applicativo definitivo.

[OpenAPI 3.1 M11](../openapi/m11.json) e [18 fixture HTTP](../fixtures/http-m11.json) definiscono CAS, errori sessione, header e risposte. Schemi richiesta PUT/DELETE e operazioni GET/status confrontati con M10; il profilo v2 aggiunge esplicitamente il requisito header e gli esiti sessione, senza riscrivere gli artefatti storici. Validatore offline finito con controlli negativi e test handler/client; nessuna certificazione OpenAPI esterna.

## Identità, algoritmo e retention

Identità logica (client_id, sequence), distinta da request_id del trasporto: client_id è 32 cifre esadecimali minuscole; sequence intero JSON canonico 1..2^53-1. Digest SHA-256 sul comando tipizzato QCMDv1: operazione, chiave, discriminatore/valore expected e valore nuovo; esclusi identità, sequenza, indice, termine, deadline e rappresentazione JSON. Riutilizzare la stessa identità con payload diverso produce REQUEST_CONFLICT.

Wrapper sessioni posseduto esclusivamente da M02 attorno a M01 invariata. Nuovo client: prima capacità, poi sequenza 1. Client noto: last+1 applica una volta e memorizza OK/COMPARE_FAILED/RESOURCE_LIMIT; last con stesso digest restituisce risultato/indice originari; last con digest diverso REQUEST_CONFLICT; inferiore STALE_SEQUENCE; salto SEQUENCE_GAP. Questi errori di protocollo non mutano KV e non sostituiscono il record valido. Read/noop non registrano sessioni. Nessun controllo preventivo HTTP decide la deduplicazione.

Massimo 1024 sessioni, nessun TTL/eviction: nuovo client a capacità riceve SESSION_LIMIT; client esistenti continuano. Solo ultimo risultato compatto, massimo 512 byte, senza vecchi valori, con highwater/digest/tipo/indice. Avanzare perde il risultato storico ma impedisce la riapplicazione delle sequenze inferiori. Identificatori non riciclabili tramite scadenza; limiti esauriti richiedono una futura decisione esplicita, non cancellazione silenziosa. Nessuna deduplicazione perpetua di tutti i risultati o exactly-once generale.

## Quorum, retry e recovery

Ogni tentativo, duplicati inclusi, attraversa nuovo append del termine corrente, replica/quorum durevole, checkpoint, applicazione M02 e controllo owner leadership/termine/commit/apply. Il risultato logico può riferirsi all'indice originario; il controllo usa indice/termine del tentativo attuale. Vecchio leader isolato con cache già popolata non può completare un retry nuovo senza quorum. Risultati nei 64 slot temporanei possono scadere: risposta unknown conservativa, senza secondo effetto.

Sessioni ricostruite solo dal prefisso committed del WAL su macchina nuova; nessun file o database aggiuntivo. Suffisso non committed non registra deduplicazione, ma può essere applicato se successivamente committed legalmente. Failover, replay e crash di tutti i processi ricostruiscono anche esiti CAS falliti. Retry conserva client_id/sequence/payload originali; nuova operazione avanza solo dopo aver risolto l'esito pendente. Client.CAS/Write effettua un tentativo con deadline; nessun retry infinito/backoff o persistenza automatica dell'identità client. Conservare identità e comando pendenti dopo un crash client è responsabilità del chiamante. GET ripetuta rimane una nuova lettura.

## Test e invarianti osservate

20 nuovi test principali in nove file: tre committed, uno config, uno storageformat, uno wal, sei servizio raft più una simulazione raft, cinque clientapi, uno nodo e uno processi. Coprono CAS assente/presente-vuoto/successo/fallimento, digest, massimo comando, sequenze/collisioni/client distinti, quota e 1024 sessioni senza eviction. Apply bloccato dimostra nessuna conferma prima del completamento, una sola transizione per duplicati concorrenti, cancellazione del solo waiter, backpressure condivisa e shutdown con drain. Errori proposta/checkpoint producono unknown senza applicazione prematura; ripartenza e retry recuperano un unico effetto. La nuova prova I/O è passata dieci volte con race detector.

Simulazione esplicita a tre nodi con seed 42/117/203, due esecuzioni ciascuno con tracce identiche, ghost committed permanente e oracolo indipendente basato su argomenti normalizzati/highwater/valori. Verifica sequenza dei risultati e indici originali, partizione con perdita messaggi, leader isolato, nuovo leader, retry riuscito/fallito dopo cambio stato, healing e replay fresco. Regressioni esistenti conservano messaggi duplicati, risposte obsolete, conflitti e regola commit termine corrente. Non è il framework M12 o checker generale M14.

Cluster HTTP reale: risposta CAS 200 persa dopo autorizzazione/applicazione, unknown al client, arresto leader e retry sul nuovo con indice originario; CAS concorrenti di client diversi, duplicati su connessioni distinte, conflitto payload, gap/stale, quorum assente. Tre processi reali: crash follower, crash leader, crash di tutti, restart directory intatte e retry sia del CAS riuscito sia di quello fallito dopo cambio stato, seguiti da GET di verifica. Nuova sequenza e vecchio retry non riapplicano. Timeout espliciti e cleanup; al massimo tre nodi temporanei.

## Verifiche eseguite e copertura

Go 1.27.4 darwin/arm64 sul profilo locale macOS/APFS. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, 1 per race con compilatore già disponibile. Build con GOOS=darwin/GOARCH=arm64. Comandi equivalenti realmente eseguiti:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m11.cover -timeout=75s ./...
go test -race -p 1 -count=1 -timeout=75s ./internal/committed ./internal/raft ./internal/clientapi ./internal/node ./internal/server ./internal/wal ./internal/config ./internal/storageformat
go test -race -p 1 -count=10 -timeout=40s ./internal/raft -run TestSessionCASIOFailureAndUncertainRecovery
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Suite completa M01–M11 superata; race mirata superata senza segnalazioni nei percorsi esercitati. gofmt senza file da correggere, vet/build superati, modulo unico senza nuove dipendenze/go.sum. Verifica aritmetica fixture canoniche originale superata. Rete esterna non utilizzata; autorizzato soltanto localhost temporaneo per i test che richiedono socket.

Copertura aggregata per istruzioni, unione dei blocchi omonimi nei profili delle diverse suite: cmd/quorumkv 0.0%; internal/applier 100.0%; internal/clientapi 81.7%; internal/committed 82.9%; internal/config 89.7%; internal/node 81.8%; internal/peerprotocol 91.4%; internal/raft 88.3%; internal/server 90.2%; internal/statemachine 100.0%; internal/storageformat 93.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. La misura non copre tutte le schedulazioni né attribuisce al main il codice esercitato soltanto nei processi figli. Non è una prova formale di consenso, linearizzabilità o deduplicazione.

## Inventario e problemi risolti

Creati internal/committed/sessions.go e nove file di test: internal/committed/sessions_test.go, internal/config/session_profile_test.go, internal/storageformat/session_identity_test.go, internal/wal/session_profile_test.go, internal/raft/sessions_test.go, internal/raft/session_simulation_test.go, internal/clientapi/cas_test.go, internal/node/m11_test.go, internal/server/m11_test.go. Modificati internal/config/config.go, internal/storageformat/manifest.go, internal/wal/open.go, bootstrap.go e store.go, internal/raft/service.go, application.go e write.go, internal/node/node.go, internal/clientapi/api.go e client.go. Le modifiche storage/config sono il confine versionato necessario; core Raft, M01/M02, filesystem, trasporto e parser restano invariati.

Creati OpenAPI/fixture M11, fixture configurazione v2, esempi examples/m11/n1.json–n3.json, ADR-010 e queste relazioni. Aggiornati in entrambe le lingue API, modello dati/consistenza, architettura, formati canonici, configurazione, recovery, guasti, requisiti/tracciabilità, test, accettazione, guida sviluppo, ADR-007, indici e roadmap; aggiornati README principali. File e fixture delle milestone precedenti non riscritti.

Durante l'integrazione il nuovo test golden ha individuato un hash costruito chiamando l'accessor pubblico prima della validazione: corretto usando un builder privato condiviso, mantenendo byte v1 identici. Corretto nel mock HTTP il nome header secondo la normalizzazione Go. La nuova prova I/O richiedeva avanzare esplicitamente il clock manuale durante il retry dopo un heartbeat in volo: corretto il solo harness e verificato dieci volte con race. Rafforzato il rifiuto di config v2 senza capacità sessioni già nel costruttore servizio. Nessuna fixture originale indebolita; nessuna correzione retroattiva di bug M01–M10. Nessun problema bloccante conosciuto residuo.

## Limiti, accettazione e passaggio

Verificati atomicità CAS, risultati originali, assenza di secondo effetto per duplicati riconosciuti, assenza di apply/risposta prematuri, replay deterministico, failover e classificazione dell'incertezza negli scenari descritti. I criteri M11 assegnati FR-010/AC-010/T-010, FR-011/AC-011/T-011 e FR-012/AC-012/T-012 sono soddisfatti nel profilo v2. Le garanzie richiedono identità/payload stabili, prefisso durevole disponibile e cluster omogeneo; solo l'ultimo risultato per sessione è recuperabile. Retry consuma WAL e richiede spazio/quorum; raggiungere le quote storage può impedirlo. Nessuna migrazione v1, snapshot/compattazione, riciclo sessioni, persistenza identità client o exactly-once generale.

Nessuna prova formale o checker esaustivo, stress prolungato, benchmark, misura RSS/CPU/usura SSD, perdita maggioranza durevole, crash OS o perdita alimentazione. Crash di processo non dimostra durabilità assoluta rispetto a OS/alimentazione. Tre processi sul medesimo Mac/SSD non danno indipendenza fisica; restano i limiti del contratto macOS locale. Verifica documentale: 132 Markdown, 65 coppie linguistiche, collegamenti relativi/struttura/esempi tecnici allineati senza errori; revisione semantica eseguita, nessuna certificazione esterna.

**M11 COMPLETED. M12 NON INIZIATA.** Readiness preliminare M12: disponibili confini apply/sessioni, fixture v2 e scenari riproducibili; il prossimo incarico dovrà rivedere gate e progettare manifest/metriche/fault framework senza anticiparli qui. Budget aggiuntivo **0 €**: libreria standard/offline, nessun servizio/acquisto/download o dipendenza nuova. Due processori e package seriali, piccoli dataset, massimo tre nodi, directory temporanee isolate e cleanup; nessun cluster lasciato attivo. Nessuna GPU, VM/Docker obbligatori, scrittura massiva o simulazione reale di perdita alimentazione. Nessun commit Git. Arresto del lavoro a M11.
