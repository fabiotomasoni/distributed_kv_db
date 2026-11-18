# Specifica API client v1

[Indice](README.md) · [English](../en/api-specification.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Contratto HTTP proposto

Tutti gli esempi in questo documento descrivono l'interfaccia futura. Nessun endpoint è disponibile. HTTP/1.1 su loopback, `Content-Type: application/json`, niente compressione. Tutte le operazioni KV usano POST per evitare ambiguità di chiavi nei path e cache intermedie. Corpo massimo 48 KiB; rifiuto prima di ammissione con HTTP 413. Validazione rigorosa secondo [modello dati](data-model.md).

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

OpenAPI è stato valutato: utile per generazione client, ma anticiperebbe un secondo contratto da sincronizzare durante la progettazione. In questa revisione le tabelle e gli esempi sono normativi; M08 dovrà derivare OpenAPI 3.1, verificare richieste/risposte come fixture e risolvere qualsiasi divergenza prima di dichiarare stabile v1. Non esiste ancora una specifica OpenAPI validata.

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
