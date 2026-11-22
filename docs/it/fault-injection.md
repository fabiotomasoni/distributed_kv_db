# Fault injection riproducibile

[Indice](README.md) · [English](../en/fault-injection.md)

**22 novembre 2026 — profilo M12 implementato e verificato; sezioni progettuali storiche precisate dal contratto M12 in fondo.**

## Adattatori e autorità

Il framework futuro sostituirà clock, PRNG, transport e storage, lasciando invariato il core. Simulazione single-thread con priorità deterministica `(virtual_time,event_sequence)`; ogni invio, consegna, completamento disco ed elezione diventa evento. Non inserire sleep nel core. Un secondo harness orchestra tre processi reali: copre socket e filesystem ma non pretende scheduling identico.

Fault ammessi: drop, delay, duplicate, reorder, isolamento direzionale o bidirezionale per coppia, crash di processo, restart della stessa directory, short write, ENOSPC e Sync fallita. “Forzare leadership” significa isolare/crashare leader o avanzare timer, mai modificare ruolo o termine aggirando l'algoritmo.

## Formato scenario proposto

Esempio illustrativo, nessun runner esistente:

```json
{"scenario_version":1,"scenario_id":"partition-old-leader","seed":7,"virtual_deadline_ms":5000,"max_events":10000,"events":[{"at_ms":1200,"action":"isolate","node":"n1","direction":"both"},{"at_ms":3000,"action":"heal","node":"n1"}]}
```

La fixture stabilisce quale nodo è leader mediante una fase preliminare deterministica; se n1 non è leader, il setup fallisce anziché testare un caso diverso. Drop/delay selezionano tipo RPC, from/to, rpc_id o numero ordinale; la traccia salva ogni scelta casuale. Duplicati hanno event_id distinti e medesimo payload; reorder usa sequenza esplicita. `heal` rimuove regole future; messaggi già programmati seguono la politica dichiarata nello scenario, default consegna alla scadenza originale. Crash scarta RAM e messaggi indirizzati all'istanza morta; restart incrementa incarnation e non riutilizza vecchi waiter.

## Storage e crash controllati

L'adattatore distingue bytes consegnati al kernel, bytes sincronizzati e namespace visibile; seleziona esplicitamente il modello di crash seguente. Cutpoint nominati `wal.header`, `wal.payload`, `wal.sync`, `commit.sync`, `snapshot.sync`, `generation.rename`, `current.rename`, `directory.sync`, `cleanup`. ENOSPC è simulato con quota, senza riempire l'SSD reale. Non corrompere directory utente o attive: fixture copiate in directory temporanee del test.

La simulazione può esplorare riordini filesystem avversi come ipotesi; non dimostra il comportamento del vero APFS. I processi reali usano segnali solo per PID registrati dal run; mai terminazioni per nome generico. Conservare exit code e stato lock.

## Manifest e replay

Salvare scenario, seed, PRNG/versione, configurazione e hash, versione harness/core, lista ordinata di eventi, decisioni fault, cutpoint, log e storia client. Replay del simulatore non estrae nuove scelte: usa traccia salvata e confronta digest dopo ogni evento. Se diverge, fermarsi al primo evento. Minimizzare un fallimento rimuovendo eventi preservando le precondizioni e la violazione.

Limiti: 5 s virtuali, 10 000 eventi/scenario; 30 s reali per integration, massimo 120 s suite ordinaria. A fine run cancellare timer, chiudere connessioni e file, terminare solo processi del run e verificare porte libere. Fallimenti conservati entro 10 MiB con retention manuale; i successi si puliscono. Non attivare fault injection dal normale endpoint HTTP: occorre build/profilo test e configurazione esplicita, tutti su loopback.

## Criterio di utilità

Un fault è utile se produce una transizione osservabile e controlla una proprietà: drop prima di ack per INV-06, risposta persa per dedup, isolamento per GET, cutpoint snapshot per generazione recuperabile. [F-01–16](failure-model.md) e [T-020/T-028](testing-strategy.md) definiscono copertura e replay; carico elevato non sostituisce questi esperimenti.

## Modelli distinti di crash e sopravvivenza I/O

**ProcessCrash**: il kernel e la cache filesystem restano vivi. Conservare write e rename già completati anche senza Sync; una write in corso può lasciare nessun byte, un prefisso o tutti i byte. Nessun byte confermato come sincronizzato può sparire nel modello. Provare almeno crash prima di write, dopo write/prima di Sync e dopo Sync, inclusi frame completi non sincronizzati, ultimo frame parziale e CURRENT rinominato ma directory non ancora sincronizzata. Recovery accetta un frame completo valido sopravvissuto, ma non applica entry soltanto perché presenti.

**SystemCrashModel**: esplorazione astratta separata, non promessa hardware; conservare dipendenze sincronizzate secondo il contratto modellato e variare la sopravvivenza degli effetti non sincronizzati, incluse operazioni namespace. La politica “solo ultimo Sync” è uno dei casi possibili, non l'unico. Non attribuire questi risultati a un vero crash macOS o power loss.

T-013 e T-016 coprono entrambe le immagini logiche entro cutpoint finiti; T-023 verifica soltanto crash reali del processo. Registrare model_kind, scelta di sopravvivenza e ordine I/O nel manifest di replay. Installare un'immagine corrotta resta un caso separato dal crash incompleto. Il kernel di rete non è un oracle: aggiungere risposte ritardate prima/dopo restart e non usare incarnation del simulatore per nascondere al core messaggi che il protocollo reale potrebbe ricevere.

## Profilo M12 implementato e contratto del manifest

[M12](milestone-m12.md) implementa internal/faultsim e il binario separato quorumkv-sim; nessuna fault API nel server ordinario. Tre core Raft reali, M02 reale e macchine/sessioni v2 nuove per istanza, scheduler a priorità (at_ms,event_id), tick virtuali 25 ms. Ogni persistenza è una completion sincrona ordinata, coerente con il driver storage attuale; trace registra send/delivery, Sync, fault e transizioni tramite digest dopo ogni evento. Nessuna sleep, socket o accesso alle directory dati nel simulatore.

Schema scenario_version=1: ID ASCII minuscolo/cifre/trattino massimo 64, seed uint64 nonzero, model_kind ProcessCrash o SystemCrashModel, tre configurazioni v2 coerenti ordinate n1/n2/n3, deadline virtuale 1..5000 ms, max_events 1..10000, massimo 128 azioni ordinate. JSON rigoroso, nomi duplicati/ignoti rifiutati. Azioni: network (ritardo casuale limitato a delay_ms+1, delay_ms<=200; drop_every/duplicate_every 0..32), partition/heal su coppia direzionale con both opzionale, crash/restart di un nodo, storage e propose PUT/DELETE/CAS/read. Nodo leader è un selettore con precondizione di un unico leader locale; mai forzatura di ruolo. Ogni RPC ha timeout virtuale 250 ms: Failed libera solo il tentativo correlato. Messaggi ritardati/duplicati mantengono RPC ID; risposte già in rete attraversano restart e vengono valutate dal protocollo. Request indirizzate alla precedente istanza morta sono scartate. Heal cambia regole future, non annulla consegne già programmate.

Storage usa frame/transazioni originali e wal.Replay per validare ogni immagine: short_write a wal.header conserva 12 byte, a wal.payload conserva il frame meno l'ultimo byte; enospc a wal.header non scrive (rifiuto provato delle proposte, errore fatale per controllo); sync_fail a wal.sync/commit.sync conserva il frame completo ma non fornisce token di successo. ProcessCrash conserva tutte le scritture completate, anche non sincronizzate. SystemCrashModel richiede scelta survival=all o synced, conservando rispettivamente tutti i byte o soltanto il prefisso sincronizzato. È esplorazione astratta, non crash reale del Mac. Restart tronca solo una coda incompleta valida e ricostruisce solo il checkpoint committed. Nessun cutpoint snapshot/generazione M13 è abilitato.

Manifest_version=1 conserva scenario/config/hash, versioni core/harness/Go/piattaforma, revisione build e stato modificato quando disponibili (altrimenti unversioned-worktree/unknown), PRNG xorshift64-v1, draw tape con intervalli/valori, fault plan, trace ordinata con SHA-256 stato e storia sintetica di ogni tentativo. Il checksum del manifest esclude il proprio campo sha256; è integrità, non autenticazione. Replay non consulta il PRNG: ricostruisce lo scheduler usando le scelte salvate e confronta ogni evento/digest, fermandosi alla prima divergenza. Versioni incompatibili sono rifiutate. Fixture reale: [manifest salvato](../fixtures/m12-session-retry.json), [scenario](../../examples/m12/session-retry.json). La storia distingue invocazione, completed, unknown, not_admitted e indice originale; il completamento simulato conserva termine/ruolo corrente, commit e apply, senza cache locale sostitutiva.

Limiti: input/evidenza 10 MiB, queue 10000 eventi, 128 tentativi, 10 s reali per comando, timeout/cancellazione e cleanup M02. Esaurire budget rende inconclusive, mai passed. Trace invarianti non campionata. Output creato 0600 con esclusione di file esistenti; nessuna cancellazione automatica di directory. Minimize elimina al massimo le 128 azioni esplicite solo se il predicato fornito mantiene precondizioni e violazione; non è il checker M14. Conservare manualmente fallimenti entro quota, eliminare soltanto file di successo posseduti dalla propria esecuzione. Le specifiche snapshot/rename sopra restano M13.
