# Specifica API client v1

[Indice](README.md) · [English](../en/api-specification.md)

**22 novembre 2026 — PUT/GET/DELETE implementate; CAS e retry mutazioni sicuri restano M11.**

## Contratto HTTP proposto

Il contratto generale e gli esempi sotto descrivono l’interfaccia v1 finale. Il sottoinsieme implementato M10 e la precedenza sono descritti in fondo. HTTP/1.1 su loopback, `Content-Type: application/json`, niente compressione. Tutte le operazioni KV usano POST per evitare ambiguità di chiavi nei path e cache intermedie. Corpo massimo 48 KiB; rifiuto prima di ammissione con HTTP 413. Validazione rigorosa secondo [modello dati](data-model.md).

| Endpoint | Campi obbligatori specifici | Risultato normale |
|---|---|---|
| `/v1/kv/put` | key, value_b64, client_id, sequence | HTTP 200; code OK, applied true. |
| `/v1/kv/get` | key | HTTP 200 con exists true e value_b64; HTTP 404 KEY_NOT_FOUND con exists false. |
| `/v1/kv/delete` | key, client_id, sequence | HTTP 200; code OK, deleted boolean. |
| `/v1/kv/cas` | key, expected, new_value_b64, client_id, sequence | HTTP 200 OK oppure 409 COMPARE_FAILED. |
| `/v1/status` (GET) | Nessuno | HTTP 200, diagnostica locale eventualmente obsoleta. |

Per `expected`, l'unione è esattamente `{exists:false}` oppure `{exists:true,value_b64:string}`. Un campo value con exists false è invalido. Tutte le richieste hanno opzionale `timeout_ms` intero 50..5000, default 2000. Il timeout locale parte dall'ammissione HTTP e include coda, disco, replica e apply; il server usa clock monotono. Il client sceglie una deadline almeno 250 ms più lunga, sapendo che non è una garanzia di consegna.

## Envelope e esempi completi

Indici e termini in tutte le risposte JSON sono stringhe decimali canoniche, coerenti con il protocollo peer. Ogni risposta KV contiene `code`, `request_id` (correlazione di trasporto, nuova a ogni tentativo) e `outcome`: `completed`, `not_admitted` o `unknown`. Una risposta `completed` comprende `index`, indice log del risultato originale (per retry), e i campi dell'operazione. Errori di parsing possono non avere request_id del client, ma il server ne genera uno. Nessun valore è incluso in errori o log.

Esempi progettuali; gli indici illustrano una storia possibile, non una misura:

```http
POST /v1/kv/put HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 123

{"key":"config/theme","value_b64":"ZGFyaw==","client_id":"0123456789abcdef0123456789abcdef","sequence":1,"timeout_ms":2000}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 80

{"code":"OK","request_id":"r1","outcome":"completed","index":"8","applied":true}
```

```http
POST /v1/kv/get HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 22

{"key":"config/theme"}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 102

{"code":"OK","request_id":"r2","outcome":"completed","index":"9","exists":true,"value_b64":"ZGFyaw=="}
```

```http
POST /v1/kv/cas HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 159

{"key":"config/theme","expected":{"exists":true,"value_b64":"ZGFyaw=="},"new_value_b64":"bGlnaHQ=","client_id":"0123456789abcdef0123456789abcdef","sequence":2}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 81

{"code":"OK","request_id":"r3","outcome":"completed","index":"10","applied":true}
```

```http
POST /v1/kv/delete HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 82

{"key":"config/theme","client_id":"0123456789abcdef0123456789abcdef","sequence":3}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 81

{"code":"OK","request_id":"r4","outcome":"completed","index":"11","deleted":true}
```

## Errori e certezza dell'esito

| HTTP / code | Esito | Azione client |
|---|---|---|
| 400 INVALID_ARGUMENT / INVALID_SEQUENCE | not_admitted | Correggere input; nessuna entry proposta da questo tentativo. |
| 413 REQUEST_TOO_LARGE | not_admitted | Ridurre corpo; nessuna entry proposta da questo tentativo. |
| 503 NOT_LEADER / NO_LEADER | not_admitted | Provare membro noto; `leader_id` e `leader_url` opzionali sono suggerimenti. |
| 429 OVERLOADED | not_admitted | Backoff, stessa identità se retry mutante. |
| 504 TIMEOUT | unknown | Stessa identità e stesso payload; non assumere fallimento. |
| 503 UNAVAILABLE | unknown dopo ammissione | Come TIMEOUT; quorum assente o leadership cambiata. |
| 507 STORAGE_UNAVAILABLE | unknown dopo ammissione | Nodo non partecipa più; provare nodo sano. |
| 404 KEY_NOT_FOUND | completed | Assenza osservata alla barriera della GET. |
| 409 COMPARE_FAILED | completed | Nessuna modifica, sequenza consumata; nuovo tentativo logico con sequenza successiva. |
| 409 REQUEST_CONFLICT / STALE_SEQUENCE / SEQUENCE_GAP | completed | Errore di protocollo sessione; nessuna modifica né avanzamento. |
| 409 SESSION_LIMIT | completed | Nuova sessione non registrata; non cambiare ID per aggirare il cap. |
| 409 RESOURCE_LIMIT | completed | Nessuna modifica, sequenza consumata per sessione ammessa. |

Per errori di sessione il campo index indica l'entry dove l'errore è valutato; i retry dell'ultima operazione valida restituiscono il suo indice originale. I retry non sono risolti da una cache locale del follower: passano dal leader e dal percorso ordinato, così la risposta non usa stato non confermato.

`not_admitted` riguarda soltanto quel tentativo: un tentativo precedente dello stesso ID potrebbe essere stato eseguito. Nessun redirect automatico HTTP e nessun forwarding follower; il client verifica leader_url contro i tre indirizzi configurati. In assenza di quorum non si offrono letture obsolete come fallback.

## Ciclo di vita della scrittura

**Ricevuta**: bytes arrivati, nessun effetto garantito. **Registrata**: entry in memoria. **Persistita localmente**: frame WAL sincronizzato; ancora revocabile se non committed. **Replicata**: almeno un peer ha confermato dopo Sync, ma non basta senza regola di termine. **Committed**: indice coperto dalla regola Raft. **Applicata**: macchina a stati e sessione avanzate. **Confermata**: risposta completed emessa dopo apply e replica durevole di maggioranza. Il client ha certezza del successo solo ricevendo e validando la risposta; TCP write completata non prova ricezione applicativa.

Se la risposta va persa dopo commit, il comando può essere già applicato. Ritentare con `(client_id,sequence)` e payload identici restituisce il risultato salvato senza nuovo effetto, finché è l'ultima sequenza. Un client non avanza alla sequenza successiva finché non ha risolto quella pendente; in caso contrario può perdere la possibilità di recuperarne il risultato storico. Deduplicazione significa effetto al massimo una volta per identità nel modello, non consegna esattamente una volta.

GET non usa sessione: ogni retry è una nuova lettura e può osservare un valore successivo. Retry mutanti: backoff iniziale 50 ms, raddoppio fino a 400 ms con jitter deterministico nei test; budget totale client 5 s, poi esito ancora incerto da conservare. Nessuna cancellazione rimuove entry già proposta.

## Concorrenza e schema

Il log ordina tutte le operazioni; tra richieste sovrapposte non si promette ordine di arrivo HTTP. Un risultato CAS riguarda il valore al suo indice. Non c'è atomicità fra chiavi. HTTP 200 di status non dimostra quorum, durabilità o linearizzabilità.

OpenAPI è stato valutato: utile per generazione client, ma anticiperebbe un secondo contratto da sincronizzare durante la progettazione. In questa revisione le tabelle e gli esempi sono normativi; M08 dovrà derivare OpenAPI 3.1, verificare richieste/risposte come fixture e risolvere qualsiasi divergenza prima di dichiarare stabile v1. Il sottoinsieme M08 dispone ora dell’artefatto verificato collegato sotto; stabilità v1 completa resta ai gate M10/M11.

## Esempi di esito negativo e retry

Esempi di risposte complete progettuali alla CAS di un valore diverso e alla stessa richiesta ammessa senza risposta entro deadline:

```json
{"code":"COMPARE_FAILED","request_id":"r5","outcome":"completed","index":"12","applied":false}
```

```json
{"code":"TIMEOUT","request_id":"r6","outcome":"unknown"}
```

Dopo r6 ritentare lo stesso corpo mutante con gli stessi client_id/sequence, senza avanzare sequenza né cambiare expected. Se era già stato applicato all'indice 13, il retry può rispondere come segue, con nuova correlazione e indice originale:

```json
{"code":"OK","request_id":"r7","outcome":"completed","index":"13","applied":true}
```

Questo esempio non autorizza a contare due effetti: applied=true descrive il risultato dell'operazione originale. Se la chiave manca, GET restituisce HTTP 404 e `{code:"KEY_NOT_FOUND", request_id:"r8", outcome:"completed", index:"14", exists:false}`; la forma in linea descrive campi, non un payload JSON da inviare.

## Compatibilità dei limiti CAS

Il cap corpo è 49 152 byte (48 KiB). Due valori raw da 16 384 byte occupano 43 696 byte Base64; una richiesta CAS JSON compatta con chiave da 256 byte, client ID, sequenza massima e timeout occupa 44 116 byte. Quindi tutti i valori ammessi sono rappresentabili in una richiesta compatta; whitespace eccessivo può comunque superare il cap. L'entry log è ricostruita dai campi validati, senza copiare whitespace o campi di trasporto, e deve restare entro 48 KiB. T-009 deve includere questo limite esatto e il rifiuto a cap+1.

## Confine della rappresentazione persistente

I [formati canonici](canonical-formats.md) definiscono la futura proiezione interna entry/digest/risultato. Ordine campi e escape leciti del wire non sono persistiti alla lettera; request_id, outcome e deadline sono campi trasporto. Stato HTTP e limiti corpo invariati. OpenAPI/fixture wire complete restano gate M08/M04, non soddisfatti dalle fixture storage.

## Sottoinsieme M08 implementato e precedenza

[Relazione M08](milestone-m08.md), [OpenAPI 3.1 JSON](../openapi/m08.json), [fixture HTTP](../fixtures/http-m08.json). Questa sezione governa il comportamento implementato; esempi retry/sessioni/GET/CAS sopra restano progetto futuro. node.StartAPI apre la porta client configurata dopo validazione/replay storage e prenotazione dei due listener. POST /v1/kv/put e /v1/kv/delete usano i campi obbligatori documentati; timeout_ms opzionale 50..5000, default 2000. GET /v1/status restituisce soltanto node_id, role e term/commit_index/last_applied come stringhe decimali canoniche; è diagnostica obsoleta. GET KV e CAS restituiscono 404 INVALID_ARGUMENT/not_admitted. Metodi/media errati restituiscono 405/415 INVALID_ARGUMENT/not_admitted. Nessun redirect o forwarding.

Validazione lessicale/schema rigorosa e cap di 49.152 byte precedono le proposte. Deadline dall’ammissione handler, incluso parsing. request_id generato per tentativo; fornirlo come campo richiesta è invalido. Solo campi comando validati entrano nello schema WAL invariato. Completed richiede commit maggioritario durevole del termine corrente, applicazione esatta indice/termine e controllo serializzato leadership nell’owner seguito da controllo HTTP conservativo dello stato. RESOURCE_LIMIT è 409/completed con index; PUT riuscita ha applied=true e DELETE ha deleted=true/false. DELETE su chiave assente è 200/deleted=false.

Un ticket cancellato prima della registrazione viene escluso permanentemente sotto il lock di ammissione: UNAVAILABLE/not_admitted per attesa scaduta/cancellata prima di registrazione. Dopo possibile registrazione, deadline è TIMEOUT/unknown; step-down osservato, cancellazione o errore infrastrutturale è UNAVAILABLE/unknown, oppure STORAGE_UNAVAILABLE/unknown per errore storage. Rifiuto esplicito WAL prima di scrittura per riserva è STORAGE_UNAVAILABLE/not_admitted. NOT_LEADER/NO_LEADER e OVERLOADED sono not_admitted solo prima della registrazione. Certezza relativa al solo tentativo. Disconnessione client non rimuove lavoro committed. Perdita non osservata dopo quorum/controllo legali non revoca commit passato; consegna TCP non atomica con cambi ruolo.

Sessioni ancora assenti. client_id e sequence validi sono metadati persistiti, non prova di deduplicazione; ripetere la stessa DELETE può restituire true poi false a indici diversi. Nessun retry automatico, garanzia at-most-once/exactly-once o recupero del risultato originale incerto. Il test client limitato a un tentativo accetta solo membri configurati, rifiuta hint non configurati/redirect e segnala errore trasporto unknown. Risolvere incertezza esternamente fino a M11; istruzioni retry future sopra non abilitate in M08.

Limiti: 32 connessioni client accettate incluse idle, 64 slot handler scrittura, 64 waiter/ticket servizio e 64 messaggi controllo finale, separati da 256 eventi peer. Ticket cancellati in coda possono occupare spazio limitato finché l’owner li drena. Massimo quattro chiamate test client attive; niente proxy ambiente, compressione o redirect. Limiti header/body/write/idle 500 ms/5 s/6 s/1 s. Body lenti possono consumare cap lettura di 5 s; deadline richiesta già scaduta esclude append. Asserzioni OpenAPI ed estensioni runtime lessicali/canoniche verificate offline con fixture e controlli negativi; non certificazione OpenAPI esterna. FR-009/AC-009 e FR-012/AC-012/T-012 completi restano ai gate M10/M11.

## Compatibilità e restart M09

[M09](milestone-m09.md) espone gli stessi endpoint M08 tramite eseguibile nodo; OpenAPI/fixture HTTP ed errori byte-invariati. Restart ricostruisce stato committed prima di ascoltare e parte follower, con correlazione nuova per tentativo. Operazione unknown può sparire o diventare committed successivamente tramite noop nuovo termine; restart non recupera risultato client originale né aggiunge deduplicazione. GET/CAS HTTP restano indisponibili. SIGTERM può perdere risposta preservando obblighi Raft già esistenti del comando.

## Contratto GET M10 implementato e precedenza

[M10](milestone-m10.md) aggiunge ora POST /v1/kv/get con key obbligatoria e timeout_ms intero canonico opzionale 50..5000, default 2000. Richiesta non accetta client_id, sequence, value o request_id. HTTP GET su questo path KV è 405; GET /v1/status resta diagnostica locale obsoleta. CAS HTTP e semantiche sessioni/deduplicazione restano future M11. Questa sezione estende e prevale sulle dichiarazioni storiche disponibilità M08/M09; comportamento PUT/DELETE invariato.

Eseguibile nodo/StartAPI abilita letture nel log su tutti e tre membri. Ogni tentativo appende nuova read(key) canonica del termine corrente, la sincronizza/replica, committa tramite regola maggioranza durevole/checkpoint esistenti e acquisisce risultato mentre M02 applica quell’esatto indice. Handler non legge mappa locale. Completed richiede indice/termine applicato corrispondente, controllo leadership serializzato owner e controlli finali conservativi HTTP stato/deadline. Mutazioni precedentemente completate precedono letture successive riuscite; chiamate sovrapposte senza promessa ordine arrivo HTTP.

200 OK/completed include index stringa canonica, exists=true e value_b64 canonico, inclusa stringa vuota per valore presente vuoto. 404 KEY_NOT_FOUND/completed include index ed exists=false, con value_b64 assente. Risposte unknown/not_admitted omettono campi risultato lettura. Limiti body/chiave/valore/connessioni/code, request_id generato, hint leader configurati e deadline conservano semantica M08. Follower/candidate non serve lettura locale; vecchio leader isolato non completa nuova lettura iniziata dopo isolamento completo. Read già legalmente committed prima di isolamento può restituire risultato acquisito se controlli completamento ancora validi. Nessun quorum passato revocato.

Esclusione provata prima di registrazione è not_admitted. Dopo possibile registrazione read, timeout è TIMEOUT/unknown, step-down osservato/cancellazione/errore infrastruttura UNAVAILABLE/unknown, errore storage STORAGE_UNAVAILABLE/unknown. Cancellazione/disconnessione abbandona attesa, non applicazione committed. Retry letture sono letture successive indipendenti e possono vedere valori più nuovi; Client.Get esegue singolo tentativo senza retry automatico. Nessuna identità client persistente o garanzia exactly-once.

[OpenAPI M10](../openapi/m10.json) e [fixture GET](../fixtures/http-m10.json) estendono [artefatto M08](../openapi/m08.json)/[fixture](../fixtures/http-m08.json) immutabili. Test confrontano esattamente tutti schemi path originali e verificano schemi, decoder runtime rigoroso, stato/body HTTP emessi e decoder test client. Regole lessicali canoniche superano normali annotazioni JSON Schema e sono controllate in Go; evidenza contrattuale finita offline, non certificazione OpenAPI esterna. Letture nel log consumano scritture WAL/commit e riserva/quota fisica esistenti. Nessun ReadIndex, lease o compressione.
