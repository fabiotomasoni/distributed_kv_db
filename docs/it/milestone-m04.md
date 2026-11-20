# M04 — RPC e configurazione

[Indice](README.md) · [English](../en/milestone-m04.md)

**20 novembre 2026 — COMPLETED nel perimetro ufficiale M04. M05 NOT STARTED.**

## Perimetro, prerequisiti e decisione

Le [roadmap approvate](implementation-roadmap.md) italiana e inglese definiscono M04 come **RPC e configurazione**: envelope/validazione e fixture configurazione canoniche, dopo M02/M03, con T-001/T-002/T-003/T-029. Requisiti collegati: FR-001/FR-002/FR-003, NFR-009 e vincoli loopback/sicurezza. Non è la fase elezioni o replica. Baseline Git iniziale 79d24f0 pulita. Esaminati entrambe le roadmap, criteri accettazione, audit/revisioni pre-M03, relazione M03, ADR-002/ADR-004 applicabili e interfacce M01–M03 esistenti. C-01 formati storage rimane RESOLVED; il suo obbligo fixture configurazione/protocollo M04 è soddisfatto qui. C-02 resta VERIFIED solo per il profilo locale macOS/APFS provato. C-03 e gate end-to-end successivi sono conservati.

M03 è stata autorizzata come prerequisito soltanto dopo regressione/revisione copertura limitate e la correzione seguente. Contratti M01/M02, byte storage approvati e go.mod restano invariati. Non sono stati introdotti applicazione automatica dati recuperati, API client, timer, decisioni voto, replica log, staging/installazione snapshot, CLI o codice M05.

## Regressione preliminare M03 e correzione

La suite completa iniziale passava: WAL 82,9%, storageformat 81,7%, storagefs 80,1%, applier/statemachine 100% di copertura istruzioni per pacchetto. Esaminati classificazione coda streaming, percorsi errore/fatali, transizioni commit/voto, recovery, lock/close e rami non coperti. I test esistenti verificano conferma sincronizzata e guasti write/Sync/repair iniettati; nessuna necessità di cambiarne le interfacce.

Un test mirato ha riprodotto una lacuna nel rifiuto della corruzione: dopo un frame iniziale valido, un frame finale incompleto con sequenza fisica leggibile ma duplicata/con salto veniva classificato riparabile. Prima della correzione TestIncompleteTailWithWrongSequenceFailsClosed falliva sulla sequenza 1 con errore nil. reader.go ora valida la sequenza appena il campo completo è disponibile (20 byte), prima di consentire troncamento della coda; MaxUint64 non può fare wrap. La regressione copre sequenze 1/3 invece di 2, tagli a 20/25 byte, rifiuto Replay e rifiuto Open effettivo con byte file invariati. La suite WAL passa dopo la correzione. È una correzione minima di integrità, senza modificare formato o durabilità confermata. WAL ha ora 18 test principali; i precedenti 25 sottocasi restano. Invariati i limiti della coda nel modello di process crash senza corruzione arbitraria.

## Componenti, interfacce e ownership

| Componente | Interfaccia e responsabilità effettive | Contratto errori e durata |
|---|---|---|
| strictjson | Decode/Fields/Uint/Integer/Hex/Base64; base lessicale/schema limitata | ErrInvalid; nessun I/O/stato; controllo nomi duplicati dopo unescaping, Unicode, profondità ed EOF |
| config | Parse; accessori Config immutabili, copie Members e SharedBytes | ErrConfig; loopback esplicito/membership statica/quote/timing; nessun listener o storage |
| peerprotocol | DecodeRequest/DecodeResponse; Message posseduto con copie Kind/Envelope/Bytes | ErrIdentity distinto da ErrMessage; valida struttura/correlazione, non possiede termine/log; solo una richiesta validata può correlare una risposta |
| transport | New/Serve/Send/Shutdown; callback Processor | ErrHTTP/ErrClosed o errori contesto/rete; risorse HTTP/1.1 limitate, cancellazione e drain |
| node | Start/Peer/Recovered/Shutdown | Validazione storage/LOCK prima del bind; drain prima di chiudere WAL, restituisce errori listener/close |

Direzione dipendenze: node → config/transport/wal; transport → peerprotocol/config; peerprotocol → config/strictjson/storageformat; config → strictjson. Nessuna dipendenza inversa o chiamata a macchina a stati/applier. M02 resta il confine futuro di applicazione ordinata, senza collegarlo a messaggi di rete non committed.

Il [contratto configurazione](configuration.md) fissa campi esatti, accesso immutabile, sei endpoint loopback unici, byte hash canonici e ordine di avvio. [configuration-v1.json](../fixtures/configuration-v1.json) è preimage/riferimento hash verificato; [peer-v1.json](../fixtures/peer-v1.json) contiene byte esatti richiesta/risposta voto, append e snapshot, compresa risposta obsoleta. Non alterano canonical-v1.json. I voti reali rimangono indisponibili: un processor nil restituisce 503. Processor simulati esistono soltanto nei test.

## Protocollo, limiti e decisioni tecniche

Sono validati ora tutti e tre gli schemi RPC futuri; supportare lo schema non implementa gli effetti di consenso. Campi esatti, identità/versione/configurazione, stringhe uint64 canoniche, identità candidato/leader, entry contigue limitate, schema comandi storage, termini non decrescenti e correlazione risposte vengono controllati prima della consegna. I chunk snapshot vengono decodificati/limitati senza file staging o considerare un digest come immagine verificata. match_index di risposta è correlato al batch inviato; resume_index obsoleto non può precedere il confine snapshot inviato. Decisioni semantiche voto/termine/leadership restano al core futuro.

JSON UTF-8 rigoroso rifiuta chiavi decodificate duplicate, nomi con case diverso/sconosciuti, surrogati isolati, profondità >16, documenti concatenati, interi/Base64 non canonici. Ordine campi/escape equivalenti leciti si normalizzano correttamente; il codec persistente rigoroso viene riusato solo dopo proiezione canonica entry. Cap configurazione 16 KiB; RPC 384 KiB; append <=16 entry, <=256 KiB comandi, <=48 KiB per entry; chunk snapshot grezzo <=64 KiB, immagine <=8 MiB. rpc_id usa 1–128 lettere/cifre ASCII o -_.:, con unicità di responsabilità del chiamante/processo; gli effetti di leadership di risposte obsolete correlate rimangono al core futuro.

Il trasporto usa solo libreria standard: niente proxy, seguito redirect, compressione, TLS o servizi esterni. Listener loopback esplicito corrispondente, Host configurato, POST/percorsi noti e media type JSON; 400 schema, 409 envelope, 413 dimensione, 415 media, 404 percorso, 405 metodo, 503 processor assente/invalido. Non-200 è errore trasporto, mai un voto. Dial 100 ms, totale uscita 300 ms inclusa admission stesso peer, header 500 ms, handler/read/write ingresso e idle 1 s; pool <=2 per host remoto e 16 socket accettati compresi idle. Una chiamata attiva in uscita per peer, attese cancellabili possedute dal chiamante, un tentativo senza retry nascosti. Nessuna coda worker/retry. Risposte limitate, chiuse e rivalidate.

Shutdown chiude definitivamente admission, cancella contesto base e I/O, chiude socket/pool, drena il lavoro ammesso, poi node chiude WAL. Node conserva LOCK finché un processor sta drenando anche se un chiamante abbandona l’attesa. Un errore serve inatteso avvia lo stesso cleanup e resta osservabile tramite Shutdown. Processor è un adattatore fidato finito e rispettoso del contesto; Go non può forzare il termine di callback arbitrarie. Il trasporto diretto è una libreria iniettabile di basso livello; storage-prima-del-bind è imposto da node.Start. Nessun listener client o CLI di produzione ancora.

## Test e verifiche effettive

12 nuovi test principali M04: strictjson 2, node/integrazione 7, transport 3; più la regressione M03. TestConfigurationCanonicalFixtureAndRejections verifica hash esatto, invarianza locale/permutazione, copie immutabili, ID/endpoint duplicati, alias IPv4-mapped, porte/path/quote/timing/versione invalidi. TestProtocolFixturesAndInvalidMessages verifica tutte le union fixture, rifiuto ruoli richiesta/risposta impropri, correlazione, errori duplicati/case/numeri/identità ed escape equivalenti leciti. TestProtocolSizeAndSnapshotBoundaries copre cap aggregato comandi, valore massimo, wrap indice, limiti snapshot/offset/done/Base64/digest e union append negativa.

TestThreeNodeStartupIdentityAndLock avvia tre identità temporanee isolate, rifiuta ownership duplicata/identità persistente divergente prima della creazione listener, inietta errore bind e verifica rilascio lock/conservazione dati. TestTransportRoundTripValidationCancellationAndShutdown verifica richiesta/risposta loopback reali, errori JSON/dimensione/media/Host/metodo prima del processor, cancellazione admission, nessun retry e shutdown/attesa ripetuta. TestNodeShutdownKeepsLockUntilProcessorDrains e TestListenerFailureDrainsAndReleasesStorage verificano ordine lifecycle/visibilità errori. I test trasporto iniettano inoltre peer fermo, risposta malformata, redirect, 503, processor invalido e fonte finita di socket accettati per cap esatto 16 connessioni/rilascio.

Tutti i test concorrenti usano canali/contesti e join limitati, nessuna sincronizzazione time.Sleep. Porte loopback reali assegnate dal sistema e mantenute da listener test; una factory listener solo test conserva le verifiche dell’ordine di avvio. L’accesso a listener locali ha richiesto invocazione test meno ristretta perché l’ambiente ristretto negava il bind loopback; nessuna connessione esterna. Rieseguite regressioni esistenti WAL con processi figli/lock/pubblicazione/guasti.

Toolchain: Go 1.27.4 darwin/arm64; macOS 27.2.1 build 26B131, APFS locale. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, 1 per race con compilatore esistente. Comandi equivalenti con toolchain installata effettivamente eseguiti:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m04-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/node ./internal/transport ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Verifica | Risultato effettivo |
|---|---|
| Formattazione, vet, build | Superati, darwin/arm64 nativo. |
| M04 più regressione M01–M03 | Tutti i pacchetti superati; nuovi test M04 e regressione WAL superati. |
| Race detector | node, transport, wal, storagefs, applier superati; nessuna race segnalata nei percorsi esercitati. |
| Fixture/dipendenze | Verificatore fixture esistente superato, fixture originali/M01/M02/go.mod invariati; solo modulo radice, nessun go.sum o nuova dipendenza. |
| Copertura istruzioni aggregata | config 86,3%, peerprotocol 91,4%, strictjson 92,1%, transport 90,3%, node 80,0%; WAL 82,7%, storageformat 91,7%, storagefs 87,9%, statemachine/applier 100%. |

La copertura è l’unione dei blocchi strumentati tra binari test con -coverpkg=./..., non la copertura per singolo pacchetto. Le righe standalone a zero di config/peerprotocol sono superate dalla copertura di integrazione esercitata. Differisce dalla misura preliminare M03 per pacchetto; i denominatori cambiati non rendono la percentuale WAL prova di regressione. Rimangono rami di guasto/schedule non provati; nessuna pretesa di esaustività o dimostrazione formale.

## File e documentazione

Creati internal/strictjson/json.go e json_test.go, internal/config/config.go, internal/peerprotocol/protocol.go, internal/transport/peer.go e peer_test.go, internal/node/node.go e m04_test.go. Modificati soltanto internal/wal/reader.go e reader_test.go nell’implementazione precedente. Aggiunte due fixture JSON condivise e configuration.md/milestone-m04.md bilingui. Aggiornati indici condivisi/lingua pertinenti, roadmap, architettura, guida sviluppo, contratti rete/formati canonici, ADR-002/ADR-004, testing/accettazione/tracciabilità e rimandi storici audit/M03. Risultati storici restano etichettati per fase. Nessun contratto precedente bypassato.

## Accettazione, limiti e readiness successiva

| Criterio | Esito M04 e confine |
|---|---|
| T-001 / AC-001 / FR-001 | Superato per avvio libreria, tre identità/cartelle, owner esclusivo ed errore prima del bind; UX exit code CLI processo resta successiva. |
| T-002 / AC-002 / FR-002 | Superato per membership rigorosa/hash canonico, identità persistente divergente e rifiuto RPC/configurazione incompatibili. |
| T-003 / AC-003 / FR-003 | Superato per scambio validato, input malformato/eccessivo e nessun effetto processor; M04 non possiede stato applicativo da mutare. |
| T-029 / AC-029 / NFR-009 | Build/test ARM64 nativi superati con toolchain registrata. |
| Gate e perimetro | Fixture C-01 M04 soddisfatte; limiti locali C-02 conservati; C-03/OpenAPI M08 e gate successivi aperti. Nessun criterio M04 obbligatorio insoddisfatto. |

Nessuna operazione database distribuito o prova safety Raft dichiarata. Macchina stati/pubblicazione snapshot, timing/backoff, gestione transizioni elezioni/termini, replica, sessioni/API client, comandi processo server, durabilità completa e osservabilità restano alle milestone rispettive. Nessuna evidenza crash OS/perdita alimentazione/alta disponibilità fisica. Contratto processor fidato e assenza CLI runtime sono limiti espliciti di integrazione, non implementazioni nascoste di fasi successive.

Budget **0 € aggiuntivi**, solo lavoro locale con libreria standard, nessun pagamento/cloud/API/installazione/GPU/VM/container. Due processori, esecuzione pacchetti seriale, deadline pacchetto <=30 s, limiti figli/contesti <=5 s, file sintetici temporanei piccoli; nessun benchmark, riempimento disco, stress prolungato, modifica file personali/sistema o operazione distruttiva. Nessuna misura risorse inventata.

**Stato finale M04: COMPLETED.** M05 dispone di confini tipizzati/configurazione/trasporto/storage utilizzabili nella propria revisione preliminare, ma test elettorali, integrazione voto persistente e gate di accettazione devono ancora essere implementati con nuovo incarico. M05 NOT STARTED. Nessun commit Git eseguito.

Verifiche documentazione: controllati 116 file Markdown, 57 coppie lingua, collegamenti relativi, struttura tabelle/fence, parità esempi tecnici e link reciproci lingua senza errori strutturali. Equivalenza semantica rivista localmente; nessuna certificazione esterna o rendering visivo Mermaid.
