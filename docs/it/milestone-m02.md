# M02 — Applier ordinato e scheduling limitato

[Indice](README.md) · [English](../en/milestone-m02.md)

**Stato: COMPLETED — 19 novembre 2026. Solo sottoinsieme locale M02; nessuna garanzia distribuita.**

## Ambito e discrepanza risolta

L'incarico precedente chiedeva Machine.Apply direttamente thread-safe, mentre la [roadmap](implementation-roadmap.md) approvata richiede ownership, scheduling dell'applier, code limitate, chiusura worker e fake. Il tentativo preliminare era BLOCKED prima di modifiche Go o nuovi test; aveva creato soltanto le due relazioni e i link negli indici. Il nuovo incarico risolve esplicitamente la discrepanza confermando la roadmap. Questa relazione supera quello stato di blocco conservandone la motivazione. Nessun rilievo dell'audit o gate futuro è rimosso.

Esaminati stato Git, roadmap, architettura, specifica/codice della macchina a stati, audit nelle due lingue, ADR-003/ADR-008 applicabili, criteri di accettazione e relazioni precedenti. L'[audit](architecture-review.md) consente M01/M02 isolate; restano aperte le condizioni storage C-01/C-02. Il package M01 esistente è invariato.

## Architettura e ownership

Il nuovo [internal/applier](../../internal/applier/doc.go) ha un worker per istanza e una coda circolare fissa. New trasferisce al worker la proprietà esclusiva di una Machine concreta non nil fino alla chiusura di Done. Nessun'altra goroutine può chiamare quella macchina, neppure tramite un altro applier. Un mutex protegge i metadati; Apply esegue fuori da quel mutex, serialmente. RWMutex non stabilirebbe ordine dei comandi o ownership ed è superfluo per GET ordinati.

L'unica dipendenza di produzione è la piccola interfaccia Machine.Apply. Apply deve terminare, non deve rientrare nell'applier né chiamare runtime.Goexit. M01 rispetta il contratto. Una dipendenza arbitraria che non termina non può essere arrestata forzatamente in sicurezza; i context limitano le attese senza interromperne la chiamata. Una macchina fallita ha stato incerto e non va riutilizzata come sana soltanto perché il worker è terminato.

Submit(ctx,Entry) ammette lavoro e restituisce una Future; TrySubmit non blocca. Future.Wait(ctx) restituisce Outcome{Index,Result}. Progress espone una vista sincronizzata di State, Capacity, Queued, Active, Scheduled e LastApplied. Done è il confine di completamento del worker; Shutdown lo attende.

Gli input devono restare invariati durante tutta Submit/TrySubmit, inclusa l'attesa di spazio. L'ammissione riuscita copia stringhe operation/key, valori ed Expected, preservando byte nil e vuoti. Dopo il ritorno il chiamante può mutare gli originali. Ogni Wait riuscita riceve un proprio buffer del risultato. Usare il puntatore Future restituito senza copiarlo; la dipendenza trasferisce il buffer del risultato al worker quando Apply ritorna e non deve mutarlo successivamente. Le Future sono per richiesta; l'applier non conserva una tabella storica di risultati né una goroutine per richiesta.

## Contratto di ordine ed errori

Il chiamante fornisce indici globali uint64 già ordinati dalla propria fonte; M02 non assegna indici di log né stabilisce il commit. InitialIndex vale 0 per default e deve corrispondere allo stato già applicato della macchina fornita. Il primo indice accettato è InitialIndex+1. Non implica recovery o caricamento snapshot.

| Condizione | Esito |
|---|---|
| Prossimo indice contiguo | Accettato una volta; Scheduled avanza all'ammissione. |
| Indice minore o uguale a Scheduled | ErrDuplicate; nessun secondo Apply o lookup di risultato in cache. Zero è la sentinella iniziale. |
| Indice maggiore di Scheduled+1 | ErrGap immediato; nessun buffer di riordino o attesa dei buchi. Il chiamante conserva e riprova in ordine. |
| Scheduled uguale a MaxUint64 | ErrIndexExhausted precede la classificazione dell'indice; nessun wraparound. |
| Piccolo comando malformato o normale errore di dominio M01 | L'indice accettato è consumato; Result contiene INVALID_ARGUMENT, COMPARE_FAILED o altro codice di dominio; LastApplied avanza. |
| Panic di Apply, incluso panic(nil) | Stato Failed; Future corrente ErrApplyFailed con effetto incerto, pendenti ErrAborted e non applicate. Nessun retry automatico. |

Arrivo concorrente e tempo di creazione della richiesta non definiscono l'ordine di applicazione. Più produttori possono contendere l'ammissione; può vincere soltanto il prossimo indice contiguo. Le entry rifiutate restano responsabilità del produttore. Il futuro core Raft sarà l'unica autorità di scheduling: conserverà le entry committed, riproverà da Scheduled+1 e userà TrySubmit mentre drena i completamenti. Il rifiuto della coda non deve mai significare cancellazione di una entry committed dal log. Operazioni client logicamente duplicate a indici diversi richiedono deduplicazione futura tramite sessioni replicate, non il rifiuto di indici M02.

LastApplied avanza soltanto quando Apply restituisce un risultato di dominio; mai all'accodamento o al panic. M02 mantiene LastApplied <= Scheduled. Il futuro core deve imporre anche Scheduled <= commitIndex. Reset applyEpoch e barriera snapshot non sono implementati; appartengono al lavoro successivo sugli snapshot. Le Future appartengono a una sola vita dell'applier.

## Coda, backpressure e ciclo di vita

La capacità conta le entry pendenti, escludendo al massimo una chiamata attiva. Default 64, configurabile 1..1024; zero seleziona il default. I campi grezzi copiati del comando sono limitati complessivamente a 48 KiB prima dell'ammissione, con ErrTooLarge e nessun avanzamento dell'indice. Limita la ritenzione di input anche malformati; non è un formato di serializzazione o WAL e non cambia le quote chiave/valore M01. Client che conservano Future o avviano produttori illimitati restano responsabili della propria memoria.

A coda piena TrySubmit restituisce ErrFull. Submit attende efficientemente una notifica di cambiamento o il proprio context; nessun busy waiting o goroutine ausiliaria. La cancellazione prima dell'ammissione non restituisce Future e non consuma indici. Se compete con l'ammissione, sono possibili sia rifiuto sia Future valida; il valore restituito determina l'accettazione.

Ciclo normale Running → Draining → Stopped. Shutdown chiude atomicamente l'ammissione, risveglia con ErrClosed i produttori bloccati e completa in ordine tutto il lavoro accettato. Chiamate ripetute/concorrenti sono sicure. Il context cancellato/scaduto termina soltanto l'attesa di quel chiamante; il drenaggio continua. Nuovo lavoro è rifiutato dopo la chiusura. Nessuna competizione send/close su un canale di lavoro.

La cancellazione di Future.Wait non rimuove lavoro accodato o attivo. Un errore di context lascia l'esito ignoto al waiter; la stessa Future può essere attesa ancora. Un risultato già pubblicato è ottenibile anche con context cancellato. Il completamento conserva il risultato e chiude una notifica senza richiedere un ricevente: waiter abbandonati non bloccano il worker. Done si chiude dopo che tutte le Future accettate hanno esiti terminali. Un panic applicativo passa a Failed e Shutdown restituisce ErrApplyFailed senza dichiarare un drenaggio riuscito.

## Test ed evidenze effettive

20 nuovi test principali coprono backlog ordinato 1..5 poi 6..7 prima del completamento, duplicati/buchi/esaurimento, 16 produttori concorrenti che conservano lavoro rifiutato, correlazione esatta dei risultati, errori di dominio, copie input/output, ammissione limitata, code sature e ripresa della backpressure. I test lifecycle coprono tutti i momenti della cancellazione, deadline controllata, shutdown ordinario/ripetuto/concorrente, chiusura durante Apply attivo, risveglio produttori bloccati, risultati non osservati, istanze indipendenti, completamento worker inattivi e panic applicativo.

gateMachine, manualClock, fakeTransport e fakeStorage, solo nei test, controllano ritardi, consegna duplicata, rifiuto iniettato e cleanup. Sono piccoli confini del harness, non rete, storage durevole o simulatore generale. Nessuna astrazione clock/transport/storage inutilizzata aggiunta alla produzione. Timing/elezioni reali e replay completo restano futuri. Canali/barriere coordinano i test; niente time.Sleep o asserzioni sul conteggio globale delle goroutine. Terminazione worker osservata tramite Done e completamento produttori tramite join/canali di risposta; context dei test limitati a 3 s, timeout package 30 s.

Eseguiti localmente con Go 1.27.4 darwin/arm64, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2, percorsi temporanei di cache/toolchain. Verifiche ordinarie con CGO_ENABLED=0; race mirato con CGO_ENABLED=1 e Apple clang 21.0.0 già presente. Dalla radice del progetto, comandi equivalenti con toolchain installata:

```sh
gofmt -w internal/applier/*.go
go vet ./...
go test -p 1 -count=3 -cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/applier
go list -m all
```

| Verifica eseguita | Risultato |
|---|---|
| M01 indipendente prima dell'implementazione | Superata. |
| gofmt e go vet ./... | Superati. |
| Suite completa, tre ripetizioni | Superata: 20 test M02 e 12 test M01 con 44 sottocasi M01 per ripetizione. |
| Statement coverage di quei run | applier 99,3%; statemachine 100,0%. |
| Race detector mirato, un run verificato | Superato; nessuna data race segnalata nei percorsi esercitati. |
| Dipendenze | go.mod invariato, nessun go.sum, solo libreria standard e statemachine locale; nessun modulo esterno. |

L'assenza di race segnalate non dimostra ogni interleaving o safety distribuita. La copertura riguarda istruzioni, non prova di correttezza. Non eseguiti consenso/checker, shutdown server, crash recovery, benchmark o misure RSS/CPU/SSD. Controlli documentali superati: 102 file Markdown, 50 coppie linguistiche, 1 123 link relativi, 76 tabelle e 16 coppie identiche di esempi tecnici non Mermaid; nessun errore strutturale rilevato. Questi controlli non dimostrano equivalenza semantica delle traduzioni né effettuano rendering Mermaid. Audit e relazioni storiche M01 conservano le date originali delle evidenze.

## File e accettazione nella roadmap

Creati quattro file di produzione: [applier.go](../../internal/applier/applier.go), [types.go](../../internal/applier/types.go), [future.go](../../internal/applier/future.go), [doc.go](../../internal/applier/doc.go); quattro file test: [scheduling_test.go](../../internal/applier/scheduling_test.go), [lifecycle_test.go](../../internal/applier/lifecycle_test.go), [dependencies_test.go](../../internal/applier/dependencies_test.go), [helpers_test.go](../../internal/applier/helpers_test.go). Nessun sorgente M01 modificato.

Aggiornate questa relazione e la traduzione, indici condivisi/linguistici, architettura, roadmap, strategia test, tracciabilità, consistenza, guida sviluppo e appendici degli ADR applicabili nelle due lingue. Il precedente stato bloccato è risolto qui; gli audit storici restano integri.

T-008/M02 e T-018/M02 sono Passed, con fondamenta fake T-028 disponibili. AC-008 completo resta a M07, AC-018 server a M09 e AC-028 replay a M12. Tutti i criteri di completamento M02 sono soddisfatti nel sottoinsieme approvato. M03 non iniziata: potrà essere affrontata dopo revisione e chiusura dei gate C-01/C-02 applicabili (formati/fixture canonici e contratto storage macOS). Nessun Raft, rete, WAL, snapshot, database su disco, server, deploy o commit Git automatico. Nessun servizio esterno, API a pagamento, installazione di dipendenze, hardware aggiuntivo o spesa; 0 €, piccoli dataset sintetici e soli run limitati.

## Revisione successiva

La [revisione pre-M03](pre-m03-review.md) ha rieseguito M01/M02 e aggiunto un test significativo sul confine cancellazione: ora 21 test applier, copertura 100,0%, race mirato superato. Le cifre originali 20 test/99,3% sopra restano evidenze storiche della consegna. Codice di produzione invariato; progetto C-01 risolto e validazione C-02 ancora condizionata/aperta come registrato nella revisione.
