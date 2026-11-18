# Modello dei dati e transizioni

[Indice](README.md) · [English](../en/data-model.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Dominio e codifica

Chiave: stringa ASCII non vuota che rispetta `[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}`; massimo 256 byte. Confronto esatto, case-sensitive, senza normalizzazione. Valore: byte opachi da 0 a 16 KiB (16 384 byte), trasmessi in Base64 RFC 4648 con padding canonico; stringa vuota codifica valore vuoto valido. `null`, Base64 non canonico e campi sconosciuti sono errori. La stringa JSON deve essere UTF-8 valida prima del parsing; rifiutare chiavi duplicate e numeri non interi dove richiesti.

La mappa distingue assenza e valore vuoto: `exists=false` non equivale a `value_b64=""`. Non esistono TTL, interpretazione JSON, conversioni numeriche o aggiornamenti parziali. `PUT("resource/7", JSON)` sostituisce l'intero blob; modificare una proprietà richiede GET, trasformazione client e CAS sull'intero vecchio valore.

## Funzione sequenziale

| Operazione | Precondizione logica | Risultato e nuovo stato |
|---|---|---|
| PUT | Chiave e valore validi; quota disponibile. | Sostituisce/crea atomicamente; `OK`, `applied=true`. |
| GET | Chiave valida; barriera committed. | `OK` con byte e `exists=true`, oppure `KEY_NOT_FOUND`. Non muta la mappa. |
| DELETE | Chiave valida. | Rimuove se presente; `OK`, `deleted=true/false`. Assenza è successo idempotente. |
| CAS | `expected` è missing oppure value. | Confronto byte esatto; se coincide scrive newValue e `OK`; altrimenti `COMPARE_FAILED` senza modifica. |

`expected={"exists":false}` permette create-if-absent. `expected={"exists":true,"value_b64":""}` confronta valore vuoto presente. `new_value_b64` è sempre un valore, mai eliminazione; usare DELETE per rimuovere. Due CAS con identico expected e newValue diverso dall'expected non possono entrambi riuscire senza una scrittura intermedia. Se CAS riscrive lo stesso valore, più successi sono legali.

## Determinismo e quote

Stato logico: `(map, logicalBytes, sessions, lastApplied)`. `logicalBytes = sum(len(key)+len(value))`, massimo 4 MiB per copia e 10 000 chiavi. La quota è valutata **in apply**, sullo stato ordinato; un controllo preventivo del leader non decide il risultato. DELETE libera byte; PUT/CAS sostitutivi sottraggono la dimensione precedente. Superamento produce `RESOURCE_LIMIT`, senza modifica della mappa, con risultato deduplicato e sequenza consumata per una sessione ammessa.

Nessun wall clock, random, iterazione non ordinata, I/O o float nella transizione. Lo snapshot ordina le chiavi e i client ID lessicograficamente; l'hash di confronto canonico usa byte espliciti. I contatori sono `uint64`; overflow causa arresto controllato, non wraparound. Indice 0/termine 0 rappresentano il prefisso vuoto.

## Sessioni e identità

Il comando mutante contiene `client_id` (32 cifre esadecimali minuscole), `sequence` (1..2^53-1, JSON intero), operazione e argomenti. Massimo 1 024 client ID conservati, senza TTL e senza espulsione automatica. Primo comando deve avere sequence 1; la registrazione avviene in apply. Sessione sconosciuta a cap pieno: `SESSION_LIMIT`, nessuna modifica né registrazione. L'ID non è una credenziale.

Per sessione si conservano ultima sequenza, digest del comando e ultimo risultato compatto (massimo 512 byte; mai vecchi valori). Se `seq=last+1`, applicare e memorizzare risultato anche per CAS fallito o quota. Se `seq=last` e digest identico, restituire risultato originale; digest diverso: `REQUEST_CONFLICT`. Se `seq<last`: `STALE_SEQUENCE`; se `seq>last+1`: `SEQUENCE_GAP`. Questi tre errori non avanzano la sequenza. Stesse regole dopo replay e snapshot. Nessuna promessa di risultati storici oltre l'ultimo.

Il digest SHA-256 comprende opcode e campi tipizzati con prefisso di lunghezza big-endian, incluso il discriminante missing/value; esclude deadline, trace ID e spaziatura JSON. La collisione crittografica è fuori dal modello. Il client persiste ID, contatore e payload pendente prima dell'invio, mantiene una sola operazione pendente e non riusa identità perse.

## Esempi d'uso

Configurazione `config/theme` → byte `dark`; inventario `stock/pen` → byte `12`; contatore `count/jobs` → GET e CAS da `12` a `13`; metadata `resource/7` → intero documento. Questi sono esempi progettuali, non dati di un server esistente. CAS su valori non protegge dall'ABA: una sequenza A→B→A può non essere rilevata. Un CAS su versione è una futura estensione, non parte dell'API iniziale.
