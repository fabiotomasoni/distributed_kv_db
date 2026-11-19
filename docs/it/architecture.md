# Architettura e concorrenza

[Indice](README.md) · [English](../en/architecture.md)

**Aggiornamento: 19 novembre 2026 — M01 locale implementata e verificata; le specifiche distribuite restano progettuali.**

## Confini dei componenti

L'architettura avrà un core Raft a eventi, con un solo proprietario dello stato di consenso. HTTP, disco e osservabilità sono adattatori: il core non legge l'orologio reale, non apre socket e non accede al filesystem. Input espliciti (tick, RPC, proposta, completamento storage) producono transizioni ed effetti verificabili.

| Componente futuro | Responsabilità e interfaccia progettuale | Stato e dipendenze |
|---|---|---|
| CLI | Validare opzioni, leggere configurazione, invocare API; nessuna logica di consenso. | Indirizzi e sessione client persistita localmente. |
| Client API | Deadline, contatto del leader, serializzazione; retry con stessa identità. | Una richiesta in volo per sessione. |
| Node server | Ammissione, shutdown e ciclo di vita; trasformare HTTP in eventi. | Code limitate, dipende da core e adattatori. |
| Raft engine | `Step(event) -> effects`, elezione, replica e commit. | Un event loop possiede ruolo, termine, voto e indici. |
| Raft log | Lookup `(index,term)`, append, sostituzione suffisso, vista snapshot. | Accesso esclusivo dal core; nessun commit autonomo. |
| State machine | `Apply(index,command) -> result`, mappa e dedup. | Un applier ordinato; nessun I/O, tempo o random. |
| Storage engine | `Persist(batch) -> durable(generation_id,sequence)`; recovery di frame. | Un worker seriale, nessuna mutazione del core. |
| Transport | Invio RPC e consegna risposte con generazione richiesta. | Un worker per peer, massimo una RPC attiva per peer. |
| Snapshot manager | Cattura immutabile, pubblicazione, installazione e compaction. | Coordinato da core e applier; serializzato con storage. |
| Configuration manager | Validare manifest, quote, indirizzi e identità. | Immutabile dopo avvio. |
| Logging / metrics | Eventi strutturati e copie delle statistiche. | Buffer limitati; mai bloccare consenso per debug. |
| Fault injection | Decorare clock, rete e storage con eventi registrati. | Solo profilo test esplicitamente abilitato. |

## Ordine, sincronizzazione e ownership

Il core non deve attendere disco o rete mantenendo un mutex globale. Una transizione che richiede persistenza viene messa in attesa di `durable(generation_id,sequence)` prima di pubblicare messaggi dipendenti: voto positivo, AppendEntries riuscita, conteggio della replica locale. La prima implementazione può sospendere l'avanzamento del core durante questo effetto, lasciando agli handler soltanto code limitate; nessuna risposta anticipa il completamento storage. Eventi e buffer trasferiscono ownership o usano copie immutabili.

L'applier riceve soltanto intervalli committed contigui e restituisce `(index,result)`. Il core mantiene `lastApplied` dai completamenti, mai dall'accodamento. La mappa non è leggibile direttamente dagli handler HTTP: GET è un comando ordinato di lettura. Lo snapshot cattura mappa, sessioni e metadati allo stesso indice, sotto barriera dell'applier; serializzazione su copia, senza letture concorrenti di mappe Go.

Limiti iniziali: 64 proposte client in attesa per nodo, 256 eventi peer, 64 completamenti apply, una persistenza attiva, una RPC attiva per peer. Riservare capacità agli eventi di consenso; saturazione client produce `OVERLOADED`, non goroutine illimitate. Un client disconnesso rimuove solo il waiter; una proposta già ammessa continua.

## Percorsi principali

Scrittura: validazione → ammissione → entry → WAL Sync locale → replica durevole → commit → apply/dedup → risposta. GET segue una **barriera nel log** di termine corrente e legge nell'applier al suo preciso indice; è più costosa di ReadIndex ma riduce la complessità iniziale. Tutti i dettagli sono in [API](api-specification.md) e [consistenza](consistency-model.md).

Shutdown: smettere di ammettere, cancellare RPC in uscita, completare effetti storage e apply già sicuri, sincronizzare e chiudere. Dopo 5 s segnalare shutdown incompleto; il successivo avvio usa recovery. Non promettere successo a richieste non applicate e non cambiare configurazione durante shutdown.

## Organizzazione futura, non creata

`cmd/quorumkv` e `cmd/quorumkvctl`; `internal/{node,raft,raftlog,statemachine,storage,transport,snapshot,config,observability,faults}`; `tests/{simulation,integration,history}`. Interfacce minime possedute dai consumatori, non un framework generico. I test della macchina a stati non importano HTTP; quelli del core non dipendono dal disco reale.

## Diagrammi

L'[indice diagrammi](diagrams/README.md) contiene nove viste: sistema, nodo, scrittura, lettura, elezione, replica/commit, recovery, partizione e snapshot. Le frecce descrivono un progetto, non componenti già costruiti.

## Indici di scheduling e generazioni apply

Il core conserva `scheduledApplyIndex`, inizializzato a lastApplied dopo recovery, con `lastApplied <= scheduledApplyIndex <= commitIndex`. Accodare soltanto il tratto non ancora schedulato, anche se commit avanza mentre l'applier è lento; una coda piena lascia l'indice invariato. Un solo produttore, range contigui e completamenti con `(applyEpoch,index)` impediscono doppio apply. Il core continua a drenare completamenti e storage senza bloccare tentando di riempire la coda apply.

InstallSnapshot richiede una barriera: fermare nuovo scheduling, drenare o invalidare in modo coordinato i vecchi lavori, attendere che l'applier non stia mutando la vecchia mappa, poi sostituire stato e impostare `lastApplied=scheduledApplyIndex=S`, incrementando applyEpoch. Completamenti della vecchia epoch non modificano indici, waiter o sessioni. Leadership change da sola non resetta questi indici: il prefisso committed resta valido. INV-05 per lastApplied vale nel normale ciclo operativo; recovery ricostruisce dal checkpoint prima di rendere il nodo pronto.

## Confine effettivo di M01

Il solo package implementato è internal/statemachine, con New() e Apply(Command) Result: [relazione](milestone-m01.md). Il futuro contratto Apply(index,command) dell’applier incapsulerà questo core; indice, commit, sessioni, epoch e scheduling non sono stati anticipati. Il core riceve chiamate già ordinate dal proprietario, senza thread safety. Il resto dei componenti e delle frecce resta progettuale.
