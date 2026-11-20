# M05 — Leader election

[Indice](README.md) · [English](../en/milestone-m05.md)

**20 novembre 2026 — COMPLETED per M05 ufficiale. M06 NOT STARTED.**

## Perimetro ufficiale e verifica prerequisiti

Entrambe le [roadmap](implementation-roadmap.md) approvate definiscono M05 come **Leader election**, dopo M03/M04: timer e voto persistente, simulatore elettorale, T-004/T-005. FR-004/AC-004 richiede elezione a maggioranza e un voto durevole per termine, inclusi split vote e crash subito dopo Sync del voto su 20 seed. FR-005/AC-005 richiede heartbeat vuoti e 100 round virtuali senza guasti e senza elezioni spurie. INV-01, parti elettorali INV-05/INV-06, NFR-001/NFR-008/NFR-009 e vincoli locali risorse/sicurezza si applicano entro questo confine; la safety Raft completa resta successiva.

Baseline Git iniziale 05898d7 pulita. Letti entrambe le roadmap, contratti Raft/guasti/consistenza, architettura e requisiti accettazione/testing, condizioni audit pertinenti e ADR-001–005, interfacce storage e relazione/trasporto M04. Suite iniziale M01–M04 superata, incluso TestIncompleteTailWithWrongSequenceFailsClosed. Obblighi C-01 storage/configurazione/protocollo soddisfatti; OpenAPI dovuta a M08. C-02 conserva solo il profilo locale Darwin/APFS verificato. C-03 riguarda M07/M10/M11/M13/M14, non è prerequisito bloccante M05. Nessun blocco noto nei prerequisiti.

## Architettura implementata e interfacce

[internal/raft/core.go](../../internal/raft/core.go) è una macchina a eventi con owner singolo. New(config, stato WAL recuperato, generazione, epoch processo, Draw iniettato, millisecondi correnti) parte follower con termine/voto recuperati e copia la vista dei termini log. Tick, Request, Response e Durable restituiscono Effects: un solo batch persistente canonico, messaggi in uscita limitati o risposta correlata. Stop chiude la macchina. Status è un valore separato. Nessun filesystem, socket, clock reale, goroutine, accesso KV o chiamata applier appartiene a Core; casualità iniettata.

[service.go](../../internal/raft/service.go) adatta lo stesso core a M03/M04. NewService prende in prestito Storage (State/Status/Append), Options fornisce Clock/Draw/epoch opzionali; default con ticker monotono, timeout uniformi inclusivi crypto/rand ed epoch processo nuova di 128 bit. Start prende in prestito Sender e avvia un owner eventi/disco più esattamente due worker peer. Handle fornisce il Processor trasporto; Status è pubblicato tramite snapshot atomici; Done e Shutdown coprono il termine effettivo dei worker. Nessun formato persistente o dipendenza aggiuntivi.

[node.StartElection](../../internal/node/node.go) è l’integrazione libreria di produzione. Verifica/acquisisce WAL prima del bind, collega Service.Handle al trasporto esistente, poi avvia elezioni. node.Start preesistente e il suo processor M04 iniettabile mantengono il contratto. ElectionStatus espone stato separato. Il termine fatale del servizio o del listener avvia shutdown nodo. Cancellazione admission/rete e drain owner/handler precedono chiusura WAL e rilascio LOCK. Recovered resta vista log separata, non percorso apply. Ancora nessuna CLI nodo/listener client.

Direzione dipendenze: node → raft/transport/wal; raft → config/peerprotocol/storageformat e tipi valore/interfacce WAL. Nessuna dipendenza inversa da storage/trasporto e nessuna dipendenza statemachine/applier. L’owner servizio esegue Append sincrono fuori da mutex core/globali; avanzamento sospeso per l’unico effetto pendente, admission peer limitata. È l’architettura iniziale semplice esplicitamente consentita dalla specifica approvata.

## Stato, transizioni e persistenza

Avvio sempre follower, voto recuperato; nessuna leadership recuperata. La scadenza elettorale viene estratta uniformemente dai limiti inclusivi configurati (default 600–1000 ms), con nuove estrazioni dopo reset e riavvio all’istante corrente del clock. Heartbeat default 100 ms. L’adattatore reale emette tick ogni min(25 ms, heartbeat/2), minimo 1 ms; il core legge soltanto timestamp monotoni espliciti degli eventi. Nessun timer callback o busy loop; tick accorpati non producono raffiche di recupero.

Timeout incrementa termine, entra candidate, prepara voto per sé e richiede una scrittura hard-state. Il voto locale viene contato e RequestVote emessa soltanto dopo il token esatto generazione/sequenza fisica. Un peer distinto più sé forma quorum 2. Prima di pubblicare leadership, append/sincronizzazione del noop locale obbligatorio del nuovo termine. Quel noop non è replicato, committed o applicato in M05. Nessun nextIndex/matchIndex, riparazione conflitti o percorso proposte implementato.

RequestVote confronta (lastLogTerm,lastLogIndex) lessicograficamente, incluso il confine snapshot conservato quando il suffisso è vuoto. Concede solo con termine non obsoleto, log aggiornato e nessun voto incompatibile nello stesso termine. Termine maggiore azzera voto precedente e declassa; nuovo termine e eventuale voto condividono una transazione hard-state sincronizzata. Ripetere lo stesso voto concesso non richiede riscrittura, ma resetta il timeout. Voto negato o RPC obsoleta non lo resettano. Status.Term/Vote pendenti sono provvisori mentre Pending=true; né stato né voto locale costituiscono acknowledgment durevole. Un errore rende il nodo terminale anche con esito sottostante incerto.

AppendEntries vuota riconosciuta e non obsoleta declassa un candidate dello stesso termine senza azzerare il voto e resetta il timeout anche se prev diverge. Heartbeat positivo verifica il prefisso esistente e riporta solo quel confine verificato. Non tronca mai suffisso, non avanza commit e non muta KV. Heartbeat di termine inferiore rifiutato senza reset. Identità leader incompatibili nello stesso termine causano arresto prudenziale nel core/simulatore. AppendEntries non vuote e InstallSnapshot valide nello schema possono ancora comunicare un termine maggiore, sincronizzato, ma restituiscono indisponibilità per effetti non implementati. Staging/installazione snapshot resta M13.

La correlazione risposte valida richiesta originale e risposta rispetto alla configurazione locale. Voti obsoleti non eleggono nuove generazioni; ogni votante conta una volta. Un termine maggiore valido declassa e viene sincronizzato anche se la richiesta originale appartiene a un’elezione vecchia. Il vecchio leader isolato può conservare la convinzione di termine inferiore fino al ripristino delle comunicazioni; non crea due leader nello stesso termine e non autorizza operazioni client.

ErrBusy richiede il completamento Durable pendente; ErrInput indica vista/evento/clock invalido, ErrUnsupported comportamento di milestone successive, ErrFull admission limitata, ErrClosed admission terminale. Storage, entropia, overflow o token errati causano arresto con ErrFatal. Errori grezzi rete/storage non diventano voti negativi. Sequenza, termine, indice, scadenza e contatore RPC non possono fare wrap producendo progresso accettato.

## Limiti, cancellazione e shutdown

Admission eventi peer con 256 eventi, oltre all’evento attivo. Esattamente un effetto persistente; una RPC attiva per peer e un worker/job slot fisso per peer, canale risultati capacità 2. Tentativi in uscita con deadline 300 ms/dial 100 ms di M04, socket e body limitati; nessun retry nascosto nel trasporto. Tick successivi del core usano stato corrente, con backoff errore connessione per peer 50/100/200/400 ms limitato a 400, senza worker retry parallelo. Epoch processo più contatore monotono limitato impediscono riuso ID RPC tra riavvii.

Servizio non avviato/cancellato/pieno rifiuta prontamente. Cancellazione prima dell’elaborazione salta un evento; dopo l’inizio della scrittura voto abbandona il waiter senza poter cancellare il voto durevole. Risposte con canale a un risultato bufferizzato non possono trattenere l’owner. Shutdown rifiuta admission, cancella worker in uscita, attende la scrittura sincrona attiva e termina i worker. Un chiamante Shutdown cancellato può attendere nuovamente; storage rimane posseduto fino al drain effettivo. Errori write durante shutdown restano osservabili. Un canale clock fermato fallisce conservativamente. Clock/Draw/Sender/Storage iniettati devono rispettare i contratti finiti/monotoni; una scrittura OS non può essere interrotta forzatamente in sicurezza. I chiamanti usano attesa shutdown 5 s e segnalano attesa incompleta senza rilasciare LOCK prematuramente.

## Test ed evidenze salvate

17 nuovi test principali: 16 in raft, uno integrazione nodo. I test core coprono avvio follower, timeout, barriere Sync voto locale/noop, token esatti, restrizioni elettorali log/snapshot, rifiuto secondo voto nello stesso termine, regole reset timer, risposte duplicate/vecchie/termine maggiore, step-down, contesti RPC estranei, operazioni non supportate, corruzione/errore/overflow e Stop. Nessuno stato client/apply può essere modificato da test o interfacce.

Il simulatore finito forza tre voti sincronizzati per sé prima della consegna, poi inietta perdita, ritardo, duplicazione e riordino messaggi con seed. Per ciascun seed 0–19 arresta un nodo subito dopo Sync simulato del voto e prima di callback durevole/risposta, lo riavvia con vista persistente e timeout/epoch nuovi e rifiuta esplicitamente un voto incompatibile nello stesso termine. Un oracolo indipendente voto durevole/quorum/leader controlla transizioni, non soltanto ruolo finale. Ogni seed viene eseguito due volte e deve riprodursi esattamente. [election-v1-results.json](../fixtures/election-v1-results.json) salva tutti i 20 seed, storie voto/termine/leader e fingerprint SHA-256 delle tracce scheduler complete: 20.527 eventi, 369 record storia conservati e 20 elezioni leader distinte seed/termine in un replay; ogni seed comprende il proprio crash. I fingerprint rilevano variazioni ma non sostituiscono l’oracolo né dimostrano safety esaustiva.

Un simulatore sano separato avanza 100 round heartbeat, emette 200 heartbeat peer e conserva un leader/termine con entrambi i follower stabili. Un altro isola il vecchio leader, elegge sulla maggioranza, ripristina e verifica declassamento sul termine maggiore. I test servizio bloccano completamento disco per verificare nessun voto anticipato, riempiono i 256 slot senza centinaia di goroutine, cancellano waiter/shutdown, iniettano errore Sync anche durante stop, verificano limiti retry e termine di entrambi i worker rete bloccati. Close/Open WAL reale conserva voto e rifiuta il secondo. L’integrazione nodo elegge con tre endpoint loopback/cartelle WAL reali, verifica noop locale sincronizzato e commit=0, poi chiude/riapre tutti gli store e verifica rilascio lock. Usa polling stato limitato; timing/oracoli protocollo sono deterministici nel simulatore.

Il crash simulato è un crash del modello; il test WAL reale M05 è close/Open controllato, non crash OS/interruzione alimentazione. Rieseguiti test esistenti M03 con processi figli e guasti I/O. Nessuna nuova dichiarazione recovery/apply di intero processo o durabilità distribuita.

## Verifiche effettivamente eseguite

Go 1.27.4 darwin/arm64, profilo macOS/APFS esistente. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 normalmente, 1 con compilatore esistente per race. Comandi equivalenti:

```sh
gofmt -l internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m05-final.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/raft ./internal/node ./internal/transport ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Verifica | Risultato effettivo |
|---|---|
| Formattazione/vet/build nativa | Superati. |
| M05 e regressione M01–M04 | Tutti i pacchetti superati; regressione originale sequenza coda WAL conservata/superata. |
| Accettazione elettorale | Tutti i 20 seed salvati riprodotti due volte; nessuna violazione oracolo. 100 round heartbeat sani, nessuna elezione spuria. |
| Race mirato | raft/node/transport/wal/storagefs/applier superati; nessuna race segnalata nei percorsi esercitati. raft/node rieseguiti dopo modifiche finali. |
| Copertura unione tra binari test | raft 92,3%, node 74,3%; statemachine/applier 100%, config 87,3%, peerprotocol 91,4%, strictjson 92,1%, transport 90,3%, storageformat 91,7%, storagefs 87,9%, WAL 82,7%. |
| Controlli dipendenze/fixture | Solo modulo radice, nessun go.sum/nuova dipendenza pacchetto. Fixture precedenti e codec invariati; verificatore aritmetico esistente superato. |

La copertura usa -coverpkg=./... e unisce blocchi esercitati tra binari test; righe standalone a zero per config/peerprotocol non descrivono la copertura di integrazione. Non è prova esaustiva di guasti/schedule. Nessun benchmark, misura throughput/latenza, power loss o disponibilità fisica eseguiti. Documenti verificati per struttura/link/parità lingua più revisione semantica locale; nessuna certificazione esterna.

## File, decisioni e componenti precedenti

Creati internal/raft/core.go, service.go, core_test.go, simulation_test.go, service_test.go; internal/node/m05_test.go; election-v1-results.json condiviso; milestone-m05.md bilingui. Esteso soltanto internal/node/node.go nell’implementazione precedente, mantenendo API M04 e ordine avvio/shutdown. Nessuna modifica M01/M02/WAL/codec/filesystem/trasporto/configurazione/parser o fixture precedenti; nessuna correzione di correttezza delle milestone precedenti necessaria. Durante revisione M05, i test hanno corretto conservazione errori del nuovo runtime durante shutdown e inizializzazione esplicita del tempo virtuale di riavvio prima della consegna.

Aggiornati indici pertinenti, roadmap, contratti Raft/architettura/rete/sviluppo, ADR-003/ADR-005, evidenze requisiti/tracciabilità/accettazione/testing/guasti e rimandi consegna storica M04 nelle due lingue. Sospensione per effetto sincrono, worker finiti, noop elettorale locale e barriera token validato conservano architettura approvata senza framework generale di simulazione o secondo sistema persistente.

## Accettazione, limiti e readiness M06

AC-004/T-004/FR-004 e AC-005/T-005/FR-005 superati nel perimetro elettorale ufficiale. INV-01 e parti elettorali INV-05/INV-06 hanno le evidenze finite sopra. Nessun criterio M05 obbligatorio insoddisfatto e nessun bug bloccante noto. Log matching completo, leader completeness, safety commit/apply, linearizzabilità client, durabilità maggioranza persistente e semantica snapshot non sono chiusi dai test elettorali.

Budget **0 € aggiuntivi**; solo libreria standard e toolchain esistente. Tutti i test locali/offline dopo preparazione cache; l’accesso a listener loopback temporanei ha richiesto deroga alla policy di esecuzione, non rete esterna. Due processori, pacchetti seriali, deadline pacchetto <=30 s, attese/figli <=5 s, tre nodi reali piccoli soltanto nell’integrazione breve, altrimenti simulazione in memoria. Nessun cloud, API a pagamento, account, installazione, container/VM, GPU, benchmark stress, scrittura SSD massiva o modifica file personali/sistema. Uso hardware limitato dalla procedura, senza misure inventate.

**Stato finale: M05 COMPLETED.** M06 può iniziare la propria revisione con viste termine/log recuperate, trasporto rigoroso e ruoli elettorali testati; deve ancora realizzare sostituzione log, gestione conflitti e progresso replica correlato. Commit/apply resta M07, funzioni client/server successive. Nessuna implementazione M06, nessun commit Git. Stop a M05.

Verifica documentazione: controllati 118 file Markdown, 58 coppie lingua, 1.445 link relativi, 106 tabelle e 23 coppie esempi tecnici non-Mermaid identici senza errori strutturali. Verificati link reciproci lingua, titoli/ID, JSON e fence; traduzioni riviste localmente.

## Addendum regressione pre-M06

La [revisione preliminare](milestone-m06.md) ha rieseguito M01–M05 e race mirate, confermato tracce elettorali salvate e 100 round heartbeat, aggiunto due test rilascio risorse nodo (bind/listener divergente e termine indipendente servizio). Nessuna modifica produzione o blocco M05 noto; copertura aggregata nodo ora 81,4%, raft 92,3%. Il 74,3% sopra registra la consegna M05. Chiarimento perimetro M06 in attesa; nessuna replica o commit implementati.

## Aggiornamento consegna M06 finale

Il gate preliminare di perimetro sopra è ora risolto dall’incarico successivo: [M06 completata](milestone-m06.md) con replica/tracking conflitti, conservando comportamento/fixture M05 originali e test preliminari. Commit/apply M07 resta non iniziato. Copertura storica M05 ed evidenze preliminari sopra restano specifiche della fase.
