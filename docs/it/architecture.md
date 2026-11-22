# Architettura e concorrenza

[Indice](README.md) · [English](../en/architecture.md)

**Aggiornamento: 20 novembre 2026 — componenti locali/elezioni M01–M07 implementati e verificati; commit/applicazione interni implementati; API client resta progettuale.**

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

## Confine effettivo di M01/M02

I package implementati sono internal/statemachine (core sequenziale M01 invariato) e internal/applier (scheduler M02). Il secondo accetta Entry{Index,Command}, restituisce una Future con Outcome{Index,Result} e possiede l’unico chiamante Apply. Produttori concorrenti forniscono indici contigui; nessun buffer di riordino. Scheduled avanza all’ammissione, LastApplied al completamento; il chiamante deve stabilire il commit. La coda contiene per default 64 entry pendenti, configurabili 1..1024, più una chiamata attiva. Submit attende spazio; TrySubmit restituisce ErrFull senza avanzare il cursore.

Shutdown chiude l’ammissione e drena; la cancellazione termina soltanto l’attesa del chiamante dopo l’accettazione. Le Future pubblicano senza richiedere un ricevente. I panic chiudono il worker con esiti espliciti incerto/corrente e abortito/pendenti. Contratti completi di ownership, limiti memoria e lifecycle: [relazione M02](milestone-m02.md). Epoch snapshot, sessioni, decisioni di commit e altri componenti restano progettuali. Il futuro adattatore di 64 completamenti non è implementato qui come canale; le Future evitano di bloccare il worker sulla disponibilità del destinatario.

## Confine storage M03 implementato

[internal/wal ed evidenze](milestone-m03.md) aggiungono un proprietario locale sincronizzato sopra storageformat/storagefs. Append restituisce token generazione/sequence fisica solo dopo Sync; Open ricostruisce metadati log validati e ripara soltanto coda finale incompleta ammissibile. Nessuna dipendenza statemachine/applier o percorso apply alternativo. Il futuro core deve stabilire commit e inviare storage bloccante fuori dal lock eventi. Rotazione snapshot, sessioni, rete e consenso restano previsti.

## Confine componenti M04

Implementate [interfacce e ownership](milestone-m04.md): node → config/transport/wal, transport → peerprotocol/config, peerprotocol → config/strictjson/storageformat. Configurazione immutabile e byte messaggio posseduti; identità storage e LOCK precedono bind, lavoro peer cancellato/drenato precede chiusura WAL. Nessuna dipendenza da statemachine/applier negli handler peer; nessuna entry recuperata applicata automaticamente. Il core Raft futuro possiede decisioni termine/voto/log e pianifica storage bloccante fuori dalla propria sezione critica.

## Ownership core e adattatore M05

Le [interfacce M05](milestone-m05.md) aggiungono internal/raft: core eventi puro con owner singolo, completamento persistente esplicito, Status copiato. Service serializza eventi ed esegue l’unica operazione WAL sincrona come adattatore disco fuori da mutex core/globali; avanzamento core sospeso con code peer limitate. Un owner più due worker rete fissi, 256 eventi peer in coda, due risultati, un job/chiamata attiva per peer. Snapshot atomici permettono lettura stato durante attese disco. Shutdown cancella admission/rete, attende disco e worker/handler, poi node rilascia storage. StartElection aggiunge l’integrazione mentre Start conserva injection M04. Nessun percorso applier o client collegato.

## Ownership replica M06

[M06](milestone-m06.md) estende stesso core/servizio senza altro worker pool. Entry canoniche possedute, array progresso fisso copiato, una richiesta registrata per peer; limiti restano 256 eventi e due worker/risultati rete. Node.StartReplication abilita batch=16, opzioni servizio consentono 1..16 solo in costruzione. AppendLocal restituisce posizione durevole locale, non commit client. WAL AppendProposal conserva riserva per admission locale, Append serve replica/controllo. Nessun applier o avanzamento commit collegato.

## Ownership e limiti applicazione M07

[M07](milestone-m07.md) aggiunge bridge proiezione committed, un worker M02 invariato e un collector FIFO future a owner/due worker peer esistenti. Nessun worker per entry. Coda pending M02 1..64, ticket/completamenti/future pendenti <=64, risultati recenti copiati 64. Owner usa TrySubmit, conserva lavoro committed non schedulato su ErrFull e drena completamenti mentre rete/tick proseguono. Shutdown normale drena lavoro committed durevole prima di rilasciare storage; admission fatale chiude prima di attendere applicazione attiva. Replay nuovo precede bind. Nessuna chiamata diretta M01 da core/HTTP o nuova interfaccia produzione M02.

## Adattatore client M08

[M08](milestone-m08.md) aggiunge internal/clientapi e Service.Write. Due server HTTP standard library necessari per porte client/peer distinte già fissate da configurazione M04; condividono limitatore connessioni accettate M04. Nessun trasporto/protocollo Raft duplicato. Proiezione tipizzata PUT/DELETE entra in coda limitata separata da eventi RPC peer; owner Raft unico esistente esegue effetti storage. Goroutine chiamanti attendono ricevute bufferizzate, completamento M02 reale e controllo finale serializzato ruolo/termine; nessun worker consegna per scrittura. StartAPI costruisce entrambi endpoint immutabili prima del lavoro in background. Shutdown rifiuta/cancella attese HTTP, attende handler, drena lavoro Raft/applicazione committed, chiude I/O peer e solo poi rilascia WAL/LOCK. Errore bind libera listener/storage già acquisiti. MVP CLI/recovery processi resta M09.

## Owner processo e MVP recovery M09

[M09](milestone-m09.md) aggiunge solo cmd/quorumkv → internal/server → node.StartAPI. Segnali cancellano attesa processo limitata; Node.Done segnala teardown automatico completato. WAL invariato valida CURRENT autorevole, identità, frame e commit; servizio invariato ricostruisce macchina M01 nuova tramite M02 esattamente sul prefisso checkpoint prima dei listener. Nessun cursore applied persistito, percorso applicativo alternativo, secondo motore recovery o nuova RPC/API. Suffisso resta speculativo fino a commit quorum legale nuovo termine. Livello processo segnala exit fatali/timeout anziché dichiarare nodo sano attivo. Ispezione solo test accede alla mappa ricostruita solo dopo terminazione worker.

## Capacità letture nel log M10

[M10](milestone-m10.md) estende adattatore client e percorso condiviso limitato ammissione/completamento servizio esistenti. Service.Read crea entry read; committed.ProjectWithReads le mappa a Get M01 nel worker M02 invariato. Valore esatto acquisito copiato nel ring 64 risultati esistente e copiato alla consegna. Massimo 64 valori conservati da massimo 16 KiB aggiungono budget payload limitato di 1 MiB, esclusi normali overhead oggetti/code. Nessuna tabella waiter illimitata o worker per lettura.

Options.Reads richiede Commit e abilita Core.EnableLoggedReads prima di EnableCommit/replay/eventi. StartAPI/CLI produzione la abilita; StartCommitted/test commit-only legacy conservano rifiuto read. È capacità startup omogenea, non negoziazione feature rolling. Tutti membri devono eseguire stesso runtime M10; vecchie modalità falliscono chiuse su recovery contenente read. Accettazione read leader/follower e replay nuovo usano schema v1 esistente; nessuna modifica formato WAL/RPC. Status, endpoint scrittura e ownership risorse shutdown invariati.

## Wrapper sessioni replicate posseduto da M02 in M11

[M11](milestone-m11.md) aggiunge committed.Sessions attorno a macchina M01 nuova, dentro worker M02 unico esistente. Envelope comando privato limitato porta byte entry canonica approvata, inclusi identità/sequence/indice, attraverso limite copia aggregato M02 invariato di 48 KiB; CAS massimo ammesso vi rientra. Solo wrapper possiede mutazione mappa/sessioni e restituisce risultato compatto/indice originale approvati tramite envelope risultato privato limitato. Interfacce produzione M01/M02 invariate; HTTP mai accede mappa sessioni.

Servizio estrae risultato nel ring indice-corrente/termine-corrente esistente, con array parallelo 64 elementi indici originali. LastApplied e controlli completamento usano nuovo tentativo effettivamente committed; HTTP usa indice risultato logico conservato. Nessuna goroutine/cache per richiesta, secondo store persistente sessioni o modifica commit Raft. I 1024 record recenti ricostruiti insieme alla mappa tramite replay nuovo prefisso committed. Configurazione/identità versionate e conferma profilo HTTP impediscono reinterpretazione silenziosa semantiche operazioni legacy. Snapshot generali/export sessioni, metriche e infrastrutture guasti riusabili restano milestone successive.
