# Guida sviluppo e MVP locale

[Indice](README.md) · [English](../en/development-guide.md)

**Aggiornamento: 22 novembre 2026 — M01–M11 implementate nei perimetri di fase; MVP processi PUT/DELETE disponibile.**

## Prerequisiti gratuiti

MacBook Pro M5/macOS ARM64, terminale, Git, editor già disponibile e Go darwin/arm64. Baseline verificata in M01: **Go 1.27.4 darwin/arm64**; go.mod richiede almeno Go 1.27.4 e la [relazione M01](milestone-m01.md) registra la toolchain esatta usata. La [release history ufficiale](https://go.dev/doc/devel/release) resta il riferimento per gli aggiornamenti. Niente beta o download automatici durante test offline; `GOTOOLCHAIN=local` evita fetch impliciti. Aggiornare patch di sicurezza dopo revisione e suite; aggiornamenti minor pianificati, non a metà esperimento.

Go e standard library sono sufficienti per il runtime; toolchain con [licenza BSD](https://go.dev/LICENSE). Git è strumento di sviluppo, non dipendenza runtime. Nessun pacchetto Go di terze parti proposto inizialmente: zero generatori, CGO, DB esterni, Raft preconfezionato o servizi di telemetria. GitHub Free opzionale. Renderer Mermaid è opzionale per leggere i diagrammi; i sorgenti restano offline. La licenza di QuorumKV è da decidere dal proprietario prima della pubblicazione.

Ogni nuova dipendenza richiede ADR: necessità concreta, alternativa standard library, versione fissata, licenza, attività manutentori, vulnerabilità note, compatibilità ARM64 senza toolchain extra, RAM/disco e modalità acquisizione/cache offline. Non introdurre un pacchetto solo perché comune in progetti di produzione.

## Schema configurazione validato

Il binario nodo M09 è ora disponibile. Questo esempio n1 rispetta lo schema implementato; per n2/n3 cambiano node_id, data_dir e bind secondo [rete](network-protocol.md). I campi comuni sono identici. config_hash è derivato da cluster_id, lista membri ordinata per ID e quote replicate; esclude data_dir e bind locali. M04 fissa la codifica canonica esatta nella fixture condivisa: [contratto](configuration.md).

```json
{"config_version":1,"cluster_id":"11111111111111111111111111111111","node_id":"n1","client_addr":"127.0.0.1:7401","peer_addr":"127.0.0.1:7501","data_dir":".local/n1","members":[{"id":"n1","client":"127.0.0.1:7401","peer":"127.0.0.1:7501"},{"id":"n2","client":"127.0.0.1:7402","peer":"127.0.0.1:7502"},{"id":"n3","client":"127.0.0.1:7403","peer":"127.0.0.1:7503"}],"max_logical_bytes":4194304,"max_keys":10000,"max_sessions":1024,"heartbeat_ms":100,"election_min_ms":600,"election_max_ms":1000}
```

Porte distinte e libere, ID immutabili, stessa versione protocollo. Il primo avvio richiede `--bootstrap` esplicito su directory vuota e manifest comune; avvio ordinario non reinizializza directory vuote accidentalmente. Bootstrap non elegge un leader speciale: tutti partono follower e seguono Raft.

## Eseguibile nodo M09 e operazioni locali

Dalla radice repository, compilare una volta e creare directory private vuote per cluster nuovo. Le tre [configurazioni esempio](../../examples/n1.json) già condividono membership. Eseguire un comando per terminale; massimo tre processi nodo. node_started significa inizializzato/listener prenotati, non prova leader o quorum.

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 go build -p 1 -o /tmp/quorumkv ./cmd/quorumkv
install -d -m 0700 .local .local/n1 .local/n2 .local/n3
/tmp/quorumkv node --config examples/n1.json --bootstrap
/tmp/quorumkv node --config examples/n2.json --bootstrap
/tmp/quorumkv node --config examples/n3.json --bootstrap
```

Bootstrap solo per primo avvio directory vuota. Al restart eseguire stessi tre comandi SENZA --bootstrap; conservare identità nodo, porte e directory dati integre. Non eliminare dati per risolvere errore. SIGINT (Ctrl-C) o SIGTERM avvia shutdown; processo concede 5 s e restituisce fallimento su timeout, preservando file per recovery. Codici exit: 0 normale/help, 2 argomenti/config invalidi, 1 errore avvio/runtime/shutdown. Percorsi config/dati relativi alla directory lavoro salvo assoluti. `.local/` e binario build radice ignorati da Git.

Client M08 è test client Go a singolo tentativo, non quorumkvctl o CLI con sessione persistente. Ispezione locale opzionale con curl esistente: interrogare /v1/status sulle tre porte configurate, scegliere quella che riporta leader, poi inviare una PUT/DELETE a tale porta. Queste richieste assumono n1 attualmente leader; adattare solo ad altro membro configurato se necessario.

```sh
curl --max-time 3 http://127.0.0.1:7401/v1/status
curl --max-time 3 -H 'Content-Type: application/json' -d '{"key":"demo","value_b64":"","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":1}' http://127.0.0.1:7401/v1/kv/put
curl --max-time 3 -H 'Content-Type: application/json' -d '{"key":"demo","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":2}' http://127.0.0.1:7401/v1/kv/delete
```

Follower restituisce not_admitted; mai seguire hint arbitrari. Timeout/risposta persa è unknown e può avere avuto effetto. Nessun retry automatico: deduplicazione/recupero risultato originale è M11. CAS HTTP, snapshot/compaction e deployment produzione assenti; GET nel log disponibile sotto. Fermare ogni nodo alla fine. [Test/relazione M09](milestone-m09.md) distingue evidenza crash processi da perdita OS/alimentazione non verificata. Note storiche libreria M01–M07 sotto descrivono confini consegne originali.

## Ciclo di lavoro futuro

Seguire [roadmap](implementation-roadmap.md), mantenere main semplice e pacchetti secondo [architettura](architecture.md). Prima proprietà e modello, poi adattatori e misure. Ogni cambiamento semantico aggiorna entrambe le lingue, requisito, ADR, fixture e criterio. Nessun codice inizia automaticamente al termine di questa consegna: occorre approvazione separata della fase progettuale.

Per il modulo M01/M02 ora esistente: formattazione `gofmt`, `go vet ./...` e `go test ./...` sono verifiche eseguite su entrambi i package locali. Test race mirati dell’applier superati per M02; test integration restano proposti per le milestone successive, entro i limiti stabiliti. Cache toolchain e moduli pre-acquisite; verificare offline disattivando rete esterna senza bloccare loopback. Nessun container necessario.

## Dati, cleanup e troubleshooting

Tenere `.local/` fuori dal futuro controllo versione; usare solo dati sintetici. Backup manuali non sostituiscono replica o recovery; copiare una generazione attiva senza coordinamento non produce automaticamente backup consistente. Cleanup solo dopo stop e verifica del percorso, mai cancellare una directory per “risolvere” CRC o lock senza aver conservato evidenze.

| Sintomo | Controllo e azione |
|---|---|
| Porta occupata | Individuare PID proprietario; scegliere porte coerenti nei tre config o fermare soltanto il proprio processo. |
| Nessun leader | Verificare peer/config_hash e log elezione; attendere majority stabile, non assegnare manualmente il ruolo. |
| Timeout | Conservare unknown; niente retry automatico fino a M11. Verificare peer/storage, mai assumere rollback. |
| Lock occupato | Verificare processo attivo; mai bypassare esclusione. |
| CRC/manifest invalido | Quarantena e diagnosi; niente reset automatico. |
| Cap storage | Fermare nuove scritture; compaction non implementata prima di M13. Preservare WAL attivo e diagnosticare. |
| Mac sotto pressione | Fermare test e processi del run; ridurre dataset/concorrenza, conservare misure. |

## Verifiche ora disponibili per M01/M02

Esistono go.mod, internal/statemachine e internal/applier; Go 1.27.4 darwin/arm64 è stato effettivamente usato. Con questa toolchain installata, dalla radice sono eseguibili go test -timeout=30s ./... e go vet ./..., senza servizi o dipendenze esterne. In questa sessione è stata usata una copia temporanea in /tmp, con checksum ufficiale verificato, non un’installazione globale: [dettagli e riproduzione](milestone-m01.md). Restano proposti gli esempi CLI/configurazione dei nodi. M02 è completata; eseguire go test -race -p 1 -count=1 -timeout=30s ./internal/applier con CGO_ENABLED=1 e compilatore C già presente. Vedere [comandi e risultati M02](milestone-m02.md).

## Configurazione M04 e verifica locale

Lo schema configurazione è ora implementato in [config.Parse](configuration.md), con fixture canoniche e test loopback limitati. Node Start è una libreria, non la futura CLI qkv-node; listener/comandi client sopra restano proposte. Riprodurre M04 con i [comandi della relazione](milestone-m04.md), cache offline e porte loopback temporanee assegnate dal sistema. La policy di esecuzione locale deve consentire loopback; non occorre accesso esterno. M01–M04 completate localmente, M05 non iniziata.

## Integrazione libreria e riproduzione M05

node.StartElection(config,bootstrap) valida storage, apre endpoint peer ed esegue worker elettorali; ElectionStatus restituisce metadati separati. Chiamare Shutdown con contesto limitato e conservare ownership finché termina effettivamente; attesa cancellata ripetibile. Nessun qkv-node/CLI o comando client implementato. La [relazione M05](milestone-m05.md) registra comandi test/build/race offline e [tracce 20 seed salvate](../fixtures/election-v1-results.json); test ordinari verificano la fixture senza rigenerarla. Clock personalizzati partono dai millisecondi correnti forniti e trasferiscono durata a Service; riavvio simulatore fornisce l’istante virtuale corrente, produzione usa clock locale monotono. M06 resta non iniziata.

## Entry point libreria M06

Usare node.StartReplication(config,bootstrap) per M06. StartElection esistente resta harness solo M05; nessuna negoziazione feature. node.AppendLocal(ctx,kind,command) dà solo metadati Sync locale, con esito locale incerto se chiamante cancella dopo admission; nessuna garanzia dedup/commit client. Service Options.BatchSize accetta 1..16 prima degli eventi. Ispezionare ElectionStatus.Progress copiato o metadati log recuperato; mai applicare direttamente. [Comandi/evidenze M06](milestone-m06.md) includono fixture riproducibili, riavvio locale e race mirate. Ancora nessuna CLI nodo/API client.

## Uso libreria e limiti M07

node.StartCommitted(config,bootstrap) abilita commit/applicazione e replay prefisso su macchina nuova. AppendLocal restituisce ancora sola durabilità locale; WaitApplied(ctx,receipt) restituisce risultato applicativo interno copiato indice/termine, potenzialmente ErrSuperseded/ErrResultExpired o attesa cancellata/fallita. Ancora nessuna CLI/API server pubblica. Helper startup M05/M06 conservano comportamento legacy. Service Options.ApplyCapacity è 1..64; MachineFactory dipendenza fidata nuova esclusiva, produzione usa M01. [Comandi/risultati M07](milestone-m07.md) registrano verifiche offline limitate e tre tracce commit riprodotte. Client/OpenAPI a M08, gate read/dedup/snapshot successivi.

## Operazione GET M10 e limiti

Stesso eseguibile/esempi ora abilitano GET nel log su tutti e tre nodi. Usare stesso binario M10 ovunque; nessuna negoziazione capacità rolling e vecchi runtime commit-only rifiutano log contenenti read. Scegliere membro configurato che riporta leader; singolo tentativo seguente assume n1. Chiave assente è 404/completed con exists=false; valore presente vuoto è 200 con exists=true/value_b64="".

```sh
curl --max-time 3 -H 'Content-Type: application/json' -d '{"key":"demo","timeout_ms":2000}' http://127.0.0.1:7401/v1/kv/get
```

Usare POST con sola key/timeout opzionale, non identità/sequence client. Ogni chiamata appende/replica nuova read e scrive WAL/checkpoint; non interrogare KV continuamente. Status è solo diagnostica locale. Timeout/risposta persa è unknown; GET ripetuta manualmente è nuova lettura successiva e può osservare altro valore. Test client Go ora ha Client.Get e mai retry automatico. Limiti/incertezza PUT/DELETE invariati. CAS HTTP, sessioni/dedup e compaction restano indisponibili. Fermare tre processi nodo alla fine; [relazione M10](milestone-m10.md).

## Cluster nuovo profilo sessioni M11

Usare [esempi M11](../../examples/m11/n1.json) espliciti con percorsi privati nuovi vuoti e porte 7601–7603/7701–7703. Conservare dati v1 .local/n1–n3 esistenti; questi comandi non li migrano. Compilare stesso eseguibile corrente ed eseguire un comando nodo per terminale, massimo tre nodi. Riavviare STESSE directory v2 con stessa configurazione, omettendo --bootstrap.

```sh
install -d -m 0700 .local .local/m11 .local/m11/n1 .local/m11/n2 .local/m11/n3
/tmp/quorumkv node --config examples/m11/n1.json --bootstrap
/tmp/quorumkv node --config examples/m11/n2.json --bootstrap
/tmp/quorumkv node --config examples/m11/n3.json --bootstrap
```

Scegliere membro configurato che riporta leader; seguente assume n1. Conservare client_id/sequence e payload invariati per retry incerto. Avanzare sequence solo per mutazione nuova dopo aver risolto operazione pendente. request_id generato per tentativo trasporto non è identità logica. Retry CAS riuscito più recente restituisce applied=true/index originali anche se altro client ha poi cambiato valore. GET resta nuova lettura nel log.

```sh
curl --max-time 3 -H 'Content-Type: application/json' -H 'X-QuorumKV-Application-Version: 2' -d '{"key":"demo","expected":{"exists":false},"new_value_b64":"","client_id":"0123456789abcdef0123456789abcdef","sequence":1}' http://127.0.0.1:7601/v1/kv/cas
```

PUT/DELETE in questo profilo richiedono anch’esse header versione; Client.Write/CAS lo imposta da config v2 analizzata e valida conferma risposta. Libreria esegue singolo tentativo, non backoff automatico o persistenza disco identità pendenti. Chiamanti conservano identità/payload pendenti attraverso propri restart. Niente TTL/eviction: recuperabile solo ultimo risultato; sequenze precedenti rifiutate e tabella sessioni piena blocca nuovi ID. Retry conservati richiedono comunque quorum e spazio WAL. [Relazione M11](milestone-m11.md) dettaglia garanzie testate; niente upgrade in-place, snapshot/compaction, consegna exactly-once o metriche M12. Fermare ogni nodo dopo uso.
