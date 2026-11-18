# Fault injection riproducibile

[Indice](README.md) · [English](../en/fault-injection.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

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
