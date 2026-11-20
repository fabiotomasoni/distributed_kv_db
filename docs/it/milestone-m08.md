# M08 — HTTP PUT/DELETE e test client

[Indice](README.md) · [English](../en/milestone-m08.md)

**21 novembre 2026 — COMPLETED nel perimetro ufficiale M08. M09 NON INIZIATA.**

## Scope ufficiale e revisione bloccante M07

Entrambe le [roadmap](implementation-roadmap.md) integrali assegnano M08 a HTTP PUT/DELETE, deadline, classificazione unknown/not_admitted, OpenAPI derivata e fixture HTTP/test client, sottoinsiemi T-009/T-012. FR-009/FR-012 e AC-009/AC-012 soddisfatti solo in tale sottoinsieme. M09 possiede recovery crash completa/MVP server; GET è M10, CAS/sessioni/deduplicazione M11, osservabilità avanzata M12, snapshot M13 e checker finale M14. Nessuna riassegnazione roadmap o implementazione successiva.

Revisione iniziale be097f4 pulita. Esaminati codice/relazioni M06/M07, specifiche API/consistenza/Raft/storage, future e ownership M02, trasporto/configurazione M04, ADR pertinenti e gate C-01/C-02/C-03. Tutti i test M01–M07 eseguiti prima dell’implementazione: superati. Selezione maggioranza del termine corrente include solo match durevoli correlati, conserva vecchi prefissi attraverso commit corrente e sincronizza checkpoint prima di apply. Commit follower limitato al prefisso verificato. Applicazione distingue Scheduled da LastApplied, usa risultati esatti indice/termine e ricostruisce solo entry committed recuperate su macchina nuova. Test esistenti vecchio termine, risposte obsolete, partizione, errore checkpoint, backlog apply 5→7, replay e failover nuovamente superati. Nessun difetto bloccante M07 o correzione produzione precedente necessari.

## Componenti, ownership e lifecycle

Nuovo internal/clientapi fornisce validazione/proiezione wire rigorosa, server HTTP limitato, ID per tentativo e test client a singolo tentativo. internal/raft/write.go aggiunge ticket ammissione espliciti e controlli finali all’owner servizio esistente; algoritmi commit/applicazione e byte storage invariati. Richieste client in coda separata di 64 slot, preservando capacità esistente di 256 eventi peer. Massimo 64 chiamate Write servizio attive e 64 messaggi controllo finale; conservazione risultati resta ring M07 da 64. Risultati restituiti includono indice e termine ricevuta. Risultati scaduti o sostituiti falliscono con incertezza anziché usare risultato di altra operazione.

Lock ticket rende cancellazione prima di registrazione esclusione permanente. Owner valida transizione pura, marca registrazione PRIMA di eseguire possibile I/O disco, rilascia lock ticket e usa effetto WAL sincrono esistente. Cancellazione successiva abbandona solo attesa. Singole risposte bufferizzate impediscono blocco owner per client abbandonati. Rifiuto esplicito riserva WAL prima di scrittura è esclusione storage sicura circoscritta. Nessuna goroutine consegna risultato per entry o coda illimitata. Ticket cancellati in coda possono occupare temporaneamente slot limitati fino a drain.

node.StartAPI prenota entrambi listener configurati solo dopo identità/LOCK/recovery e replay committed su macchina nuova, poi avvia componenti immutabili. Due server HTTP necessari perché M04 già specifica porte client e peer distinte. Condividono registro/limitatore connessioni accettate M04; trasporto/protocollo peer non reimplementato. Cap socket client 32 incluse idle, slot handler scrittura 64; header/body/write/idle limitati a 500 ms/5 s/6 s/1 s. Errore avvio chiude listener acquisiti e rilascia storage. Shutdown chiude ammissione, cancella/attende waiter HTTP, drena obbligo applicazione committed esistente, attende trasporto peer e solo poi chiude WAL/LOCK. Chiamante shutdown cancellato limita la propria attesa, non ownership storage.

## API esposte e semantica risultati

POST /v1/kv/put richiede key/value_b64/client_id/sequence; POST /v1/kv/delete richiede key/client_id/sequence. Entrambe consentono timeout_ms intero canonico 50..5000 (default 2000). Deadline dall’ammissione handler, incluso parsing body. GET locale /v1/status restituisce solo node_id/role/term/commit_index/last_applied obsoleti. GET KV/CAS non supportate sono 404 INVALID_ARGUMENT/not_admitted; metodo/media errati 405/415. Nessun forwarding o redirect. Follower/candidate restituisce NO_LEADER o NOT_LEADER con hint soltanto da membership configurata.

JSON rigoroso rifiuta nomi decodificati duplicati, campi sconosciuti/case errato, UTF-8/surrogate invalidi, profondità eccessiva, documenti successivi e forme intere/Base64 non canoniche. Chiavi, ID client, sequence e valori decodificati usano validazione canonica esistente. Cap body di 49.152 byte testato esatto/cap+1, chiave a 256/257 e valore a 16.384/16.385 byte; valori vuoti validi. request_id/deadline trasporto mai nello schema comando persistente invariato. ID generati per tentativo con prefisso casuale e contatore. Valori assenti da errori/log.

Solo Sync locale insufficiente per completed. Service.Write attende applicazione M02 effettiva della ricevuta esatta, poi controllo owner serializzato verifica leadership/termine corrente sano e commit durevole/LastApplied sufficienti. Livello HTTP ricontrolla conservativamente stato pubblicato e deadline prima di codificare. Autorizza completamento a quell’evento; cambi ruolo e consegna TCP non possono essere fisicamente atomici. Perdita non osservata dopo quorum legale non revoca commit passato. PUT restituisce 200/OK/applied=true; DELETE 200/OK/deleted=true oppure false per chiave assente. RESOURCE_LIMIT è 409/completed deterministico con index e nessuna mutazione. Nessun accesso pubblico mappa o scorciatoia lettura.

## Deadline, incertezza e confine retry

Prima di registrazione, ticket in coda cancellato/scaduto escluso permanentemente: UNAVAILABLE/not_admitted. Errori input/body, follower/candidate e overload sono anch’essi not_admitted. Rifiuto riserva WAL prima di scrittura è STORAGE_UNAVAILABLE/not_admitted. Certezza relativa a questo tentativo, mai a precedente con identità condivisa. Dopo possibile registrazione, deadline è 504 TIMEOUT/unknown; perdita leadership osservata, cancellazione o errore infrastrutturale è 503 UNAVAILABLE/unknown, errore storage 507 STORAGE_UNAVAILABLE/unknown. Unknown omette index/campi successo e mai implica rollback. Lettura body lento limitata a 5 s; deadline richiesta già scaduta dopo parsing non può appendere.

Disconnessione dopo registrazione non cancella apply committed. Risposta persa dopo apply lascia client incerto. Test client ha massimo quattro chiamate attive, niente proxy ambiente/compressione/redirect, target membri configurati e hint esatti configurati validati, cap risposta e controlli envelope rigorosi. Esegue un POST con margine timeout di 250 ms, nessun replay automatico, inseguimento hint o retry. Errori trasporto unknown; validazione/cancellazione locale prima di invio not_admitted.

client_id/sequence sono solo metadati validati/persistiti. Nessuna tabella sessioni replicata, deduplicazione, garanzia exactly-once/at-most-once, risultato originale stabile o promessa retry dopo restart. DELETE identica ripetuta può restituire true poi false a indici diversi; test lo verificano esplicitamente. Esempi retry API finale restano progetto futuro M11. Barriere GET e ReadIndex assenti; GET nel log resta strategia futura approvata.

## Test ed evidenze finite

14 nuovi test principali tra raft (5), clientapi (7 incluso contratto) e node (2). Test servizio bloccano applicazione M02 reale e persistenza disco, cancellano tentativi registrati contro accodati, assicurano che comando cancellato in coda mai appenda, preservano capacità peer sotto saturazione client, rifiutano controlli finali obsoleti ed esercitano errori non leader/chiuso/invalido/riserva. AppendEntries di termine maggiore mentre apply è bloccato forza esito unknown dopo completamento comunque del lavoro committed. Test diretti controllo finale rifiutano separatamente apply insufficiente, ruolo cambiato e termine cambiato.

Test HTTP coprono input malformati/eccessivi, proiezione canonica, valori vuoti/massimi e limiti chiave/body, schema timeout, ID richieste nuovi, matrice stato/errori, hint configurati, route future non supportate, validazione host/media/query, overload e shutdown che attende attesa registrata. Controlli client rifiutano hint falsi, indici/booleani successo invalidi e divergenza certezza/stato. Socket reale interrotto dopo applicazione handler ma prima di qualsiasi byte risposta: client segnala unknown, esattamente una chiamata backend e DELETE successiva prova effetto. Ripetizione DELETE prova assenza deduplicazione.

Test reale loopback a tre nodi usa adattatori produzione WAL/trasporto/Raft/applicazione/API/test client. Rifiuta scrittura follower, conferma PUT e applicazione repliche, completa due scritture sovrapposte a indici distinti, ferma leader, elimina valore conservato attraverso successore eletto e ripete identità come operazione indipendente. Fermare terzo membro lascia leader solo: nuova scrittura registrata localmente scade unknown senza conferma commit. Tutti i sei indirizzi listener e tre lock WAL riacquisiti dopo shutdown. Errore bind client verifica separatamente cleanup peer/storage. Nessun cluster resta attivo.

[OpenAPI 3.1](../openapi/m08.json) copre esattamente PUT/DELETE e status locale. [20 fixture comuni](../fixtures/http-m08.json) verificate contro schemi richiesta/risposta, decoder runtime, stato/body server e validazione test client. Validatore finito offline copre ogni keyword asserzione usata nell’artefatto, rifiuta asserzioni non supportate e include controlli negativi per campi mancanti/extra e violazioni range. uint64/Base64 canonici e hint configurati usano inoltre controlli/estensioni runtime. Evidenza contrattuale limitata, non certificazione OpenAPI esterna indipendente o prova universale linearizzabilità. Vecchie fixture canoniche storage/config/peer/elezione/replica/commit invariate.

## Verifiche effettive e copertura

Go 1.27.4 darwin/arm64, profilo locale macOS/APFS esistente. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, 1 con compilatore esistente per race. Comandi equivalenti effettivi:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m08.cover -timeout=45s ./...
go test -race -p 1 -count=1 -timeout=45s ./internal/raft ./internal/clientapi ./internal/node ./internal/transport
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Tutti superati; race senza segnalazioni nei percorsi esercitati. Copertura statement aggregata tra binari: applier 100.0%; clientapi 82.5%; committed 92.3%; config 88.2%; node 79.8%; peerprotocol 91.4%; raft 88.1%; statemachine 100.0%; storageformat 92.6%; storagefs 87.9%; strictjson 92.1%; transport 90.1%; wal 82.7%. Copertura non prova ogni scheduling, crash o proprietà di consistenza. Formati originali/go.mod invariati, nessun go.sum o nuova dipendenza; inventario moduli contiene solo repository. Nessun benchmark, esperimento guasto fisico o misura hardware/risorse. Struttura documentazione, link reciproci, esempi accoppiati e parità bilingue verificati localmente; nessuna certificazione esterna.

## File, gate, vincoli e consegna

Creati internal/raft/write.go e write_test.go; internal/clientapi/api.go, client.go, api_test.go e contract_test.go; internal/node/m08_test.go; docs/openapi/m08.json; docs/fixtures/http-m08.json; queste relazioni bilingui. Modificati solo dispatch servizio, avvio/shutdown nodo e riuso limitatore trasporto nel precedente codice produzione. Produzione M01/M02/core commit/WAL/codec/filesystem/config/parser rigoroso invariata. Aggiornati entrambi indici lingua, API, architettura, consistenza, Raft, modello guasti, test, accettazione, tracciabilità, roadmap e ADR-004/ADR-007 più indice radice. Evidenze storiche M06/M07 conservate.

Sottoinsieme OpenAPI/fixture M08 C-01 soddisfatto; stabilità API completa richiede M10/M11. C-02 resta contratto locale Darwin/APFS testato; nessuna garanzia OS/perdita alimentazione. Evidenze backlog M07 C-03 preservate, gate successivi snapshot/sessioni/letture/storie restano aperti. Nessun difetto bloccante noto in M08. Checker linearizzabilità client completo, GET, CAS HTTP, dedup, snapshot, CLI/recovery processi server e deployment restano esclusi.

**M08 COMPLETED. M09 NON INIZIATA.** Interfacce libreria prerequisite pronte per revisione/incarico M09 separati; accettazione completa recovery/MVP non svolta qui. Spesa aggiuntiva **0 €**, standard library offline, massimo tre nodi ordinari temporanei, piccoli dataset sintetici, due processori/package seriali e timeout brevi espliciti. Nessun servizio/download a pagamento, GPU, VM/container obbligatorio, stress prolungato, scritture SSD massive o modifica file personali/sistema. Nessun commit Git. Stop a M08.

Verifica documentazione: 124 file Markdown, 61 coppie lingua, 1.607 link relativi, 106 tabelle e 27 esempi tecnici accoppiati; zero errori strutturali.
