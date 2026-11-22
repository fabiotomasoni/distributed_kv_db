# Consistenza e linearizzabilità

[Indice](README.md) · [English](../en/consistency-model.md)

**22 novembre 2026 — baseline GET nel log e PUT/DELETE implementata; contratto futuro completo sessioni/CAS/checker ancora ai gate.**

## Contratto osservabile

La proprietà richiesta è linearizzabilità su una singola mappa: deve esistere un ordine sequenziale legale delle operazioni completate che rispetti l'ordine reale. Se A termina prima dell'invocazione di B, A precede B; operazioni sovrapposte possono ordinarsi in entrambi i modi. “Strong consistency” da solo è ambiguo: qui significa esclusivamente questo contratto. Il riferimento formale è [Herlihy e Wing](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf).

PUT, DELETE e CAS hanno punto logico di linearizzazione nell'applicazione della loro entry committed. Il log fissa l'ordine, l'applier non interlaccia comandi. CAS fallito è un risultato definitivo dello stesso ordine e consuma la sequenza. Un retry deduplicato rappresenta la stessa operazione logica e il punto originale; non è una nuova scrittura. Per GET il punto è il campionamento della mappa durante apply della barriera.

## Protocollo scelto per GET: lettura nel log

1. Validare la richiesta e inoltrarla al core locale solo se leader corrente; follower risponde NOT_LEADER. Nessuna lettura diretta dalla copia locale.
2. Creare una nuova entry `read(key)` nel termine corrente dopo l'invocazione. Il comando non cambia la mappa e non usa sessione client.
3. Persistenza e replica identiche a una scrittura. Il commit richiede maggioranza durevole e regola del termine corrente: la leadership viene quindi verificata dal quorum per questa entry, senza usare heartbeat precedenti o lease.
4. Attendere `lastApplied` fino alla precisa entry. L'applier valuta la chiave in quello slot e conserva il risultato per il waiter prima di applicare mutazioni successive. Non attendere un generico indice minimo e poi leggere una mappa che altri modificano senza sincronizzazione.
5. Rispondere solo con risultato acquisito e con generazione leader ancora valida al momento della consegna al server. Se nel frattempo si è osservato un termine maggiore, restituire unknown o far scadere; è una scelta conservativa. Una perdita di leadership non ancora osservata non invalida una barriera legalmente committed.

Questo protocollo impedisce il successo di una GET **invocata dopo l'isolamento completo**, quando nessun messaggio utile al suo nuovo round può raggiungere una maggioranza: non c'è quorum per la nuova entry. Include il requisito di commit di un'entry del termine corrente, spesso il noop di elezione; anche la stessa entry read può soddisfarlo, portando con sé il prefisso precedente. Non dipende dall'accuratezza del clock.

## Effetti e costi

Read-after-write vale quando la scrittura ha risposta completed e la GET successiva riesce: non vede uno stato precedente a quella scrittura, salvo una modifica successiva legale. Cambiare leader non riduce questa garanzia, se rimane una maggioranza recuperabile. L'assenza della chiave è una lettura anch'essa ordinata; 404 è un risultato definitivo, non un errore di rete.

Le letture richiedono rete, WAL e apply, aumentano spazio e usura SSD e non funzionano sulla minoranza. Per il carico didattico limitato è un compromesso deliberato. ReadIndex potrà ridurre I/O, ma richiederà ADR aggiornato, commit corrente, round quorum correlato iniziato dopo la richiesta e attesa apply del read index. Non è parte del percorso iniziale e non verrà attivato solo per migliorare benchmark.

## Esiti incerti e disponibilità

Timeout non significa rollback. Le operazioni pendenti possono essere completate o escluse in un'estensione legale della storia. Il client non può cambiare ID a una mutazione incerta senza rischiare un nuovo effetto logico. Non sono promesse transazioni multi-key, letture stale esplicite o disponibilità nella minoranza. La correttezza dipende anche dalla conservazione delle sessioni, non soltanto da Raft.

## Verifica

[Testing](testing-strategy.md) definisce storie con invocazioni, risposte, identità e risultati incerti, un modello sequenziale indipendente e ricerca degli ordini ammissibili. Test mirati: vecchio leader isolato, scrittura confermata prima di failover, apply ritardato, CAS concorrenti, risposta persa e retry dopo snapshot. Un test finito che trova un ordine è evidenza di quella storia; non dimostra tutte le possibili esecuzioni.

## Operazioni in volo al momento della partizione

Una read già committed prima della partizione può completare dopo l'isolamento se risultato e controlli del protocollo sono ancora validi; analogamente una scrittura già committed può ricevere risposta tardiva. È linearizzabile perché l'intervallo iniziava prima del guasto. Non richiedere impossibile revoca del quorum passato. T-019 deve scartare i messaggi in transito rilevanti, stabilire l'isolamento e soltanto dopo invocare la GET che deve fallire; un secondo caso consente la risposta di una barriera già committed.

I punti di linearizzazione sono del servizio logico ordinato, non un punto fisico simultaneo su tutte le repliche. Apply locali possono essere ritardati o rifatti durante recovery; un retry rimane associato all'effetto originale. Un failover può rendere osservabile un comando pendente tramite una replica che lo applica prima del vecchio leader, sempre nell'ordine committed.

## Confronto rivalutato: log contro ReadIndex

| Aspetto | A: GET nel log, baseline v1 | B: ReadIndex, estensione futura |
|---|---|---|
| Correttezza | Entry read del termine corrente committed, campionata nel suo slot apply. | Commit noto di un'entry del termine corrente, round quorum fresco correlato e attesa apply del read index. |
| Cambio leader | Stessa regola di commit delle scritture; nessuna lettura locale senza barriera. | Contesti legati a termine/generazione; abortire se si osserva perdita leadership prima del completamento. |
| Quorum | Necessario per ogni nuova entry read. | Necessario per un round di lettura; possibile batching solo per richieste già invocate prima dell'inizio del round. |
| Latenza | Replica con persistenza, checkpoint commit e apply; nessun numero misurato. | Round quorum e attesa apply, senza WAL per la singola GET; non necessariamente più veloce se apply è arretrato. |
| Entry | Una per tentativo GET ammesso. | Zero per GET, noop iniziale del termine ammortizzato. |
| SSD | Entry e checkpoint sulle repliche, poi compaction; write amplification reale da misurare. | Nessuna persistenza aggiuntiva per la GET a termine stabile; persistono i normali obblighi Raft. |
| Complessità | Un percorso ordinato condiviso, scheduling corretto comunque necessario. | Stato readIndex/context, correlazione ack, attesa apply e gestione waiter/leadership separati. |
| Test | Riusa commit/apply; aggiunge risultato esatto allo slot e partizione. | Aggiunge ack vecchi, round iniziati troppo presto, apply lag, batching e termine cambiato. |
| MacBook | Adeguato come ipotesi per dataset e run limitati, non misurazione di usura o RAM. | Riduce I/O read-only, con maggior costo di implementazione e revisione. |

**Decisione confermata:** A per la prima versione, poiché il carico è limitato e l'obiettivo iniziale è verificare un solo percorso di ordinamento. Il vincolo SSD si gestisce con test brevi, zero riscritture inutili e misure, non inventando prestazioni. Nessuna lettura locale senza quorum.

Percorso futuro B, soltanto dopo M14/M15 e aggiornamento ADR-006: (1) commit di noop/entry del termine corrente; (2) alla ricezione GET catturare R=commitIndex e creare contesto univoco legato a termine e generazione; (3) iniziare un nuovo round AppendEntries/heartbeat con quel contesto; (4) raccogliere risposta positiva correlata da un peer distinto più il leader, nel termine corrente; (5) rivalidare ruolo/generazione e attendere lastApplied>=R; (6) fare acquisire il valore all'applier sotto barriera di lettura coerente e completare il waiter. Non riusare ack ricevuti prima della richiesta e non esporre la mappa agli handler.

Timeout o termine maggiore invalidano il contesto; una risposta tardiva non completa round nuovi. L'estensione richiede evoluzione esplicita dello schema RPC per il contesto, non campi sconosciuti accettati silenziosamente in v1. Usare un round nuovo per ogni GET inizialmente, prima di qualsiasi batching. Testare vecchio leader, ack ritardati, noop non committed e apply arretrato; richiedere lo stesso checker indipendente e stessi esiti di errore della baseline. Si confrontano poi bytes scritti, Sync e latenza nelle stesse condizioni, senza attivare una strategia ibrida non documentata.

Riferimento teorico: [Raft, sezione 8](https://raft.github.io/raft.pdf). La sequenza operativa e i gate qui sopra sono la proposta QuorumKV, non un'affermazione di implementazione già esistente.

## Confine implementato M02

L’[applier locale](milestone-m02.md) serializza comandi indicizzati dal chiamante e restituisce risultati nel loro esatto slot di applicazione. Creazione/arrivo concorrente non definisce l’ordine: è ammesso soltanto il prossimo indice contiguo. Fornisce un componente locale ordinato, non prova di commit, quorum o linearizzabilità distribuita. L’ammissione rifiutata non consuma indici; attese cancellate dopo l’ammissione non revocano l’operazione.

## Evidenze ordinamento interno M07

[M07](milestone-m07.md) implementa ora commit legale del termine corrente e applicazione esatta ordinata mutazioni/noop. CAS fallita rimane committed/applicata con COMPARE_FAILED. LocalAppend resta sola ricevuta Sync locale; WaitApplied è consegna interna indice/termine con conservazione limitata, cancellazione ed errori supersessione. Non è API pubblica linearizzabile/exactly-once: comportamento HTTP/errori/sessioni esterni e barriere GET nel log restano fasi successive. M07 rifiuta entry read future anziché abilitare percorso lettura anticipato. Contratto client generale sopra resta progettuale.

## Confine scritture esterne M08

[M08](milestone-m08.md) espone ora risultati PUT/DELETE dopo commit legale durevole, applicazione esatta e controllo finale ruolo/termine serializzato nell’owner. Handler ricontrolla progresso pubblicato prima di codifica. Questo non rende consegna rete atomica con step-down né revoca operazione legalmente committed per perdita non osservata. Cancellazione prova esclusione solo prima della registrazione; dopo unknown non implica rollback. Scritture completed rispettano ordine log committed; arrivi HTTP sovrapposti senza ordine fissato. Identità ripetuta è nuovo tentativo fino a M11, quindi collasso retry/sessioni sopra resta progetto futuro. Nessuna GET KV, CAS HTTP, ReadIndex o checker storie completo. Test coprono piccole scritture concorrenti e DELETE dopo PUT completed attraverso elezione, senza dichiarare prova universale di linearizzabilità.

## Replay e suffisso incerto M09

[M09](milestone-m09.md) verifica ricostruzione nuova del prefisso committed prima di servire. Nessun suffisso speculativo esposto solo perché byte sopravvissuti. Dopo elezione, noop termine corrente può committare legalmente suffisso incerto ereditato; precedente risposta unknown non prova rollback. Entry confermate completed sopravvivono ai crash processo testati con directory integre e maggioranza recuperabile. Test confrontano byte entry originali e macchina realmente ricostruita con piccolo oracolo esterno di storia confermata. Non aggiunge GET HTTP, retry sicuri, collasso identità sessioni o checker linearizzabilità completo.

## Ordinamento implementato ed evidenze M10

[M10](milestone-m10.md) implementa protocollo scelto letture nel log. Entry fresca del termine corrente creata dopo invocazione, quindi acknowledgment maggioranza durevole riuscito deve coprire tale entry o successivo confine legale termine corrente. Heartbeat precedenti invocazione non stabiliscono quel nuovo prefisso. Output M02 esatto indice/termine conservato e copiato prima di mutazioni successive; mai sola attesa LastApplied seguita da campionamento mappa attiva. Osservazioni chiave assente e valore vuoto distinte.

Controllo completamento owner rifiuta perdita ruolo/termine osservata o commit/apply insufficienti; snapshot HTTP finale è solo rivalidazione conservativa, non prova quorum. Read committed prima di isolamento può completare dopo isolamento, nuova read isolata non può. Storie a tre nodi con seed scartano esplicitamente traffico in volo prima di invocazione vecchio leader, conservano ghost committed permanente e confrontano slot lettura con modello indipendente lista/stringhe. Storia HTTP reale di cinque operazioni cerca ordini legali delle sovrapposizioni e checker rifiuta controesempio stale noto. Queste verifiche finite soddisfano gate letture M10; checker generale storie sessioni/CAS/snapshot resta M14. GET ripetuta è nuova lettura logica senza collasso sessioni.
