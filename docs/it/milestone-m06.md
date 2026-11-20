# M06 — Replica log, AppendEntries e conflitti

[Indice](README.md) · [English](../en/milestone-m06.md)

**20 novembre 2026 — COMPLETED per M06 ufficiale. M07 NOT STARTED.**

## Risoluzione perimetro e prerequisiti

Il nuovo incarico conferma definitivamente la [roadmap](implementation-roadmap.md): M06 è replica/risoluzione conflitti/tracking, FR-006/AC-006/T-006. Commit distribuito/regola quorum del termine corrente e apply restano M07/T-007/T-008. Il precedente blocco di ambito è **RESOLVED**, senza riassegnare milestone della roadmap. La relazione preliminare integrale è conservata in fondo, inclusi stato storico bloccato ed evidenze M05.

Baseline Git 19703fd; all’ingresso erano conservati documentazione preliminare non committed e due test rilascio risorse. La revisione preliminare correttezza M05 non rilevava blocchi; alle sue evidenze complete regressione/race seguono qui ulteriori regressioni M01–M05. Codec/fixture configurazione/protocollo originali C-01 invariati; C-02 conserva solo il profilo locale Darwin/APFS provato; C-03 resta ai gate successivi originali. Nessuna correzione di correttezza produzione precedente necessaria.

## Interfacce, ownership e limiti

[replication.go](../../internal/raft/replication.go) estende lo stesso core con owner singolo. EnableReplication(batch) accetta 1..16 soltanto prima degli eventi; capacità M05 esplicita zero/default conserva harness elettorale e fixture elettorali salvate esatte. StartReplication(config,bootstrap) abilita M06 con batch=16; StartElection e Start M04 mantengono i contratti. Service Options.BatchSize permette batch locali minori. È selezione all’avvio, non negoziazione runtime o supporto rolling upgrade.

Core.AppendLocal(now,kind,command) assegna indice contiguo/termine corrente propri e valida byte entry/comando canonici. Service.AppendLocal e node.AppendLocal restituiscono LocalAppend{Index,Term} **solo dopo Sync locale**. Non espongono operazione HTTP client o risultato quorum/commit. Un chiamante cancellato può avere un’entry locale già persistita; retry senza garanzia deduplicazione. Admission nonleader/disabilitata/invalida/piena/chiusa dà errore esplicito. Comandi limitati prima delle copie in coda e copiati difensivamente. Nessuna dipendenza macchina a stati/applier.

Core conserva byte entry canonici posseduti e lookup termini; LogEntries restituisce copie profonde. Status.Progress è un array fisso copiato di tre membri. Ogni follower ha nextIndex/matchIndex e massimo una richiesta effettiva. Sent registra invio dopo admission adattatore; offerte saltate non invalidano richiesta attiva. Failed rilascia solo richiesta corrispondente; Response valida identità richiesta originale/risposta, byte esatti della richiesta conservata, ID RPC, termine corrente e leadership prima del progresso. Termini maggiori osservati prima anche per richieste obsolete. Stato posseduto da un event loop, con 256 eventi peer/locali, un effetto disco attivo, due worker peer fissi e canali job/risultati limitati. Limiti loopback/socket/body/deadline e backoff su tick esistenti conservati.

## AppendEntries e riconciliazione

Il follower valida tutto il batch wire prima degli effetti, rifiuta termini obsoleti senza reset timer, riconosce leader del termine corrente/maggiore secondo regole elettorali, poi verifica indice/termine prev incluso confine conservato. Suffisso mancante restituisce lastIndex+1; termine divergente restituisce primo indice locale e conflict_term. Termine maggiore sincronizzato prima di rifiuto/successo dipendente. Heartbeat vuoti usano stessi controlli prefisso e non eliminano mai suffisso soltanto perché la richiesta finisce prima.

Entry corrispondenti conservate byte per byte. Al primo conflitto effettivo di termine o entry mancante, conserva prefisso e persiste suffisso sostitutivo. Indici committed noti recuperati da M03 non sostituibili; tentativi di conflitto causano arresto conservativo senza scrittura/risposta positiva. Byte diversi allo stesso indice/termine coerente causano errore integrità. Batch duplicati/corti conservano eventuale suffisso aggiuntivo coerente senza riscrittura log superflua. match_index positivo è esattamente prev+len(entries), non tutta la lunghezza log follower.

Alla leadership, il noop nuovo termine già sincronizzato inizializza match locale all’ultimo indice durevole, match remoti=0 e next remoti=last+1. Successo correlato avanza match monotonicamente e next=match+1. Rifiuto usa hint termine/indice conflitto approvato, limitato dal log leader disponibile e match+1, riducendo posizione next tentata quando possibile. Trasmissione o risposta non correlata/duplicata/vecchia generazione non contano come replica. Nuova generazione leadership azzera conoscenza remota; suffissi di ex leader cambiano solo dopo passaggio follower. Catch-up snapshot per prefissi rimossi non implementato prima di M13; runtime M03 usa ancora generazione disponibile con base zero.

## Ordinamento WAL e interruzioni

M03 supporta già transazioni canoniche replace_suffix, quindi **nessuna modifica produzione WAL, codec o formato storage** necessaria. Core costruisce batch hard_state+replace_suffix esistente; servizio serializza Append fuori da mutex core/globali. Completamento deve coincidere con generazione/sequenza fisica esatte. Installa suffisso in memoria e conferma solo dopo Sync riuscita. Nessuna manipolazione diretta dei file persistenti in produzione.

Proposte locali usano AppendProposal esistente per conservare riserva consenso 512 KiB; replica/controllo usa Append. ErrLimit proposta noto prima della scrittura lascia leader/log/indice utilizzabili senza consumare sequenza. Write incerto/fallito, errore persistenza replica/controllo o token completamento errato sono fatali, mai voto negativo mascherato da successo. Costruzione servizio M06 richiede interfaccia storage con proposte; interfaccia M05 rimane compatibile.

Batch in uscita rispettano numero entry configurato e cap conservativo byte entry canoniche 256 KiB, che rispetta anche cap comandi RPC. Una RPC in ingresso valida può avere <=256 KiB comandi ma superare i <=256 KiB byte entry totali del WAL. L’intero batch viene validato prima; tali suffissi sono persistiti in transazioni consecutive limitate. La prima sostituisce suffisso divergente, le successive appendono resto; un’unica risposta positiva segue ultima Sync. Interruzione può lasciare prefisso durevole validato, mai transazione parziale o acknowledgment positivo di frame mancanti. Recovery riproduce prefisso con M03 e retry duplicati appendono resto mancante in sicurezza. Entry committed note conservate a ogni taglio.

## Test ed evidenze finite invarianti

11 nuove funzioni test implementazione finale: 10 raft, una integrazione nodo, più due test preliminari nodo conservati. Asserzioni/fixture elettorali esistenti rimangono; il fake storage memoria acquisisce sostituzione suffisso/AppendProposal e il literal risultato esteso usa campi nominati. Codice produzione M01/M02/storage/trasporto/parser/configurazione invariato.

Test core coprono log vuoti/corti/divergenti/identici, entry singole/multiple, duplicati, heartbeat corti, conflitto prefisso committed, byte incoerenti stesso termine, termini obsoleti/maggiori, hint conflitto, modifiche next/match, invio effettivo rispetto a messaggi offerti/saltati, tentativi cancellati, risposte obsolete/duplicate, reset generazione, limiti avvio/batch e Sync locale distinta da conoscenza remota. Errori Sync/quota incerta iniettati non danno acknowledgment replica; quota admission proposta nota è ritentabile. Un batch wire massimo lecito esercita due effetti WAL limitati senza risposta prima del completamento finale.

Il test WAL reale prepara checkpoint commit controllato coerente, taglia frame riconciliazione ai confini 0/20/24/finale-payload/completo, riapre con wal.Open, verifica stato intero vecchio/nuovo della transazione e prefisso committed identico, poi ritenta catch-up. Scrittura completa sopravvissuta esplicitamente non prova acknowledgment precedente. Nessuna interruzione OS/alimentazione dichiarata.

Una simulazione memoria tre nodi parte con prefisso committed noto comune e code non committed divergenti, elegge leader aggiornato, poi inietta perdita, ritardo, duplicazione e riordino con seed. Arresta/riavvia n2 dopo Sync suffisso e prima dell’acknowledgment. Oracolo indipendente su viste disco verifica Log Matching tra prefissi con stesso indice/termine, leader append-only, commit noto invariato e convergenza finale tracker/log dopo heal. Cinque seed eseguiti due volte con tracce scheduler identiche; [replication-v1-results.json](../fixtures/replication-v1-results.json) salva fingerprint/risultati (469 eventi registrati per replay complessivo dei cinque seed). Tutti arrivano all’indice 6 con commit controllato fisso a 1.

L’integrazione loopback usa massimo tre nodi ordinari, batch 2, appende tre entry, attende log durevoli identici e match correlati, ferma follower, appende ancora, lo riavvia con StartReplication di produzione e verifica catch-up/shutdown. Commit resta 0 su tutti i nodi. Canali/contesti, clock virtuali e ticker stato limitati sostituiscono coordinamento tramite sleep. Evidenze M05 20 seed/100 heartbeat e regressione sequenza fisica WAL invalida superate invariate.

Sono osservazioni finite di Log Matching, leader append-only, persistenza-prima-ack, termini/tracker monotoni e conservazione prefisso committed noto. Leader Completeness e state-machine safety conservano gate consenso/apply successivi; nessuna dimostrazione formale/universale safety dichiarata.

## Verifiche eseguite e risultati

Go 1.27.4 darwin/arm64, profilo macOS/APFS locale esistente; GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2. CGO_ENABLED=0 normalmente, 1 con compilatore esistente per race. Comandi equivalenti effettivi:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m06-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node ./internal/transport ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Tutti superati. Race mirate senza segnalazioni nei percorsi esercitati; raft/node rieseguiti dopo modifica finale guardia avvio. Copertura istruzioni aggregata tra binari strumentati: raft 90,3%, node 81,3%; statemachine/applier 100%, config 87,3%, peerprotocol 91,4%, strictjson 92,1%, transport 90,3%, storageformat 92,0%, storagefs 87,9%, WAL 82,7%. Denominatori produzione diversi spiegano variazioni da M05; copertura non certifica ogni guasto/schedule. Fixture originali identiche byte per byte, verificatore aritmetico superato, solo modulo radice, nessun go.sum/nuova dipendenza. Nessun benchmark o misura guasti fisici/risorse eseguiti.

## File, accettazione e confine successivo

Creati internal/raft/replication.go, replication_test.go, replication_recovery_test.go, replication_simulation_test.go; internal/node/m06_test.go; docs/fixtures/replication-v1-results.json. Estesi internal/raft/core.go e service.go, internal/node/node.go; adattato minimamente fake test esistente service_test.go. Conservati pre_m06_test.go e tutte le evidenze preliminari. Aggiornati relazione bilingue, indici pertinenti, roadmap, follow-up Raft/rete/architettura/storage/recovery/guasti/testing/accettazione/tracciabilità e ADR-003/ADR-005. Allocazione milestone roadmap invariata.

**FR-006 / AC-006 / T-006: Passed. M06 COMPLETED**, nessun bug bloccante noto. M07 può iniziare la propria revisione con progresso replica durevole per peer. Deve ancora implementare commit legale quorum/termine corrente, avanzamento/propagazione commit persistito e integrazione applier ordinato. In M06 leaderCommit trasporta stato recuperato invariato e deliberatamente non avanza checkpoint follower; nessuna transazione commit_index emessa a runtime. Nessuna conferma commit client, barriera lettura, deduplicazione, API client completa o installazione snapshot.

Budget aggiuntivo **0 €**, lavoro locale con libreria standard e strumenti esistenti. Due processori, pacchetti seriali, deadline pacchetto <=30 s, attese limitate <=5 s, massimo tre nodi reali, altrimenti scenari memoria piccoli. Nessun servizio pagamento/cloud/API, download, GPU, VM/container, stress prolungato, scrittura SSD massiva, modifica file personali/sistema o commit Git automatico. Test richiedono solo accesso esecuzione loopback, nessuna rete esterna. Nessuna implementazione M07. Stop a M06.

## Revisione preliminare storica (superata)

<details>
<summary>Relazione preliminare originale, prima della conferma perimetro</summary>

**20 novembre 2026 — M06 BLOCKED in attesa del chiarimento obbligatorio del perimetro; implementazione NOT STARTED. Revisione preliminare M05 superata entro i limiti seguenti.**

## Perimetro ufficiale e discrepanza

Entrambe le [roadmap](implementation-roadmap.md) approvate assegnano M06 alla **replica log**, AppendEntries/conflitti e tracce log divergenti, FR-006/AC-006/T-006. M07 possiede separatamente **commit distribuito**, regola del termine corrente e apply, T-007/T-008. Il nuovo incarico è intitolato “Log Replication & Distributed Commit” e include calcolo/propagazione commit obbligatori e scenari commit di vecchi termini. Questi superano il confine M06/M07.

La sezione 3 dell’incarico impone esplicitamente: se M06 ufficiale differisce sostanzialmente dal perimetro replica/commit descritto, non iniziare implementazione, riportare discrepanza e chiarire il perimetro prima di seguire la roadmap. Di conseguenza non sono stati introdotti codice produzione M06, test replica log, nuova logica commit o riassegnazione roadmap. Una richiesta di chiarimento propone M06 limitata ad AppendEntries, riconciliazione log e nextIndex/matchIndex, conservando commit distribuito/apply a M07. Il silenzio non è approvazione. È un gate di perimetro, non un errore Election Safety scoperto.

Baseline Git iniziale 19703fd pulita. Definizione roadmap esistente conservata. Nessuna milestone dichiarata completata da questa revisione preliminare.

## Revisione correttezza M05

Rivisti core/service internal/raft effettivi e lifecycle nodo rispetto a relazione M05, specifica elettorale, contratti persistenti e ADR/audit pertinenti. Core con owner singolo, handler accodano eventi limitati, un solo effetto persistente sincronizzato sospende avanzamento e worker rete non possono mutare stato consenso. Voto locale e risposte voto attendono completamento esatto generazione/sequenza fisica. Termini maggiori declassano e azzerano vecchio voto; risposte obsolete non contano in nuova elezione, ma termini maggiori validi rimangono osservati. I voti concessi impongono esclusione persistente nello stesso termine e confronto ultimo log prima per termine. Reset timer limitati a inizio elezione, voto concesso e RPC leader riconosciuto non obsoleto. Recovery parte follower; nessun dato non committed applicato.

Rieseguiti tutti i test M01–M05 esistenti, inclusi 20 seed elettorali salvati riprodotti due volte ciascuno, crash-dopo-Sync-voto-modello e rifiuto secondo voto, 100 round heartbeat sani, risposte obsolete/maggiori, riapertura voto WAL reale, errori I/O servizio ed elezione loopback tre nodi. Regressione M04 TestIncompleteTailWithWrongSequenceFailsClosed invariata e superata. Nessun difetto correttezza M05 bloccante noto rilevato nella revisione limitata; test finiti non dimostrano tutte le esecuzioni Election Safety.

Obblighi C-01 storage/configurazione/protocollo soddisfatti, OpenAPI resta M08. C-02 conserva solo il profilo Darwin/APFS locale precedentemente verificato, senza prova crash OS/perdita alimentazione. C-03 resta ai gate originali M07/M10/M11/M13/M14. Nessuno autorizza ad anticipare l’implementazione commit M07 in M06.

## Revisione mirata copertura integrazione

La suite iniziale ha riprodotto copertura aggregata **raft 92,3%, node 74,3%** con -coverpkg=./... e unione blocchi tra binari test. Esaminati rami nodo non coperti senza obiettivo 100%. Percorsi importanti residui: cleanup dopo allocazione servizio elettorale ma errore avvio listener, indirizzo listener divergente e termine indipendente servizio che avvia cleanup nodo. Wrapper/accessori semplici e rami errore irraggiungibili con configurazione valida/servizio nuovo non giustificavano meccanismi artificiali di injection.

Aggiunto soltanto [internal/node/pre_m06_test.go](../../internal/node/pre_m06_test.go):

- TestElectionStartupFailureReleasesStorage: errore bind iniettato e indirizzo listener divergente, nessun nodo utilizzabile, errore bind conservato e riapertura WAL riuscita che prova rilascio ownership.
- TestElectionServiceTerminationClosesNode: stop indipendente del servizio elettorale, attesa del watcher Done nodo fino a cleanup peer/storage, shutdown ripetuto e riapertura WAL per verificare rilascio LOCK.

Entrambi superati senza cambiare comportamento produzione. Il secondo esercita il percorso comune cleanup per termine servizio; non è un nuovo guasto disco reale iniettato. Test raft servizio esistenti coprono errori storage terminali e conservazione errori durante shutdown. Copertura aggregata nodo salita a **81,4%**, raft rimasto **92,3%**. Nessuna correzione produzione M03–M05 o modifica interfaccia necessaria.

## Controlli eseguiti e vincoli

Go 1.27.4 darwin/arm64, profilo macOS/APFS locale esistente. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, race mirate CGO_ENABLED=1 con compilatore esistente.

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m06-preliminary.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node
```

Tutti superati. Race nodo rieseguita dopo l’aggiunta dei due test, senza race segnalate nei percorsi esercitati. Il comando copertura strumenta tutti i pacchetti; percentuali dei singoli binari test non sono la copertura aggregata per pacchetto sopra. Fixture precedenti, sorgenti produzione e go.mod invariati. Nessuna nuova dipendenza o codifica persistente, nessun commit Git. Documentazione verificata per struttura/link/coppie lingua; nessuna certificazione formale.

Budget **0 € aggiuntivi**, test soltanto locali/offline, cartelle temporanee private e socket loopback dinamici. Due processori, pacchetti seriali, deadline pacchetto <=30 s, attese <=5 s, dati sintetici piccoli; nessun servizio cloud/API, download, benchmark, stress, GPU, VM/container, scrittura grande o modifica file personali/sistema. I test listener locali hanno richiesto la deroga esistente alla policy di esecuzione per loopback, non una connessione esterna.

## Deliverable M06 e readiness successiva

Replica AppendEntries non vuote, riconciliazione suffissi divergenti, nextIndex/matchIndex, admission append leader, scenari errore/riavvio replica ed evidenze Log Matching M06 **non implementati da questa consegna**. Calcolo commit/regola termine corrente/propagazione e integrazione applier restano **non iniziati a M07**. Nessuna nuova interfaccia replica, estensione WAL o conferma replica positiva dichiarata. Gestione heartbeat M05 esistente invariata.

Revisione prerequisito tecnico M05 superata entro evidenze finite. **M06 resta BLOCKED esclusivamente in attesa del chiarimento perimetro imposto dall’incarico dell’utente**, e la sua implementazione resta NOT STARTED. Dopo conferma si può implementare il perimetro M06 autorizzato della roadmap senza modificarla. Readiness M07 non stabilibile prima dell’accettazione M06. Nessun lavoro M07 o commit automatico.

</details>

Verifica documentazione: 120 file Markdown, 59 coppie lingua, 1.503 link relativi, 106 tabelle e 25 esempi non-Mermaid appaiati; zero errori strutturali. Verificati link reciproci lingua, parità sezioni/ID, esempi JSON e fence, equivalenza semantica rivista localmente. Nessuna certificazione esterna o rendering Mermaid.
