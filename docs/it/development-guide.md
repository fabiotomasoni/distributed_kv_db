# Guida al futuro sviluppo

[Indice](README.md) · [English](../en/development-guide.md)

**Aggiornamento: 19 novembre 2026 — M01 locale implementata e verificata; le specifiche distribuite restano progettuali.**

## Prerequisiti gratuiti

MacBook Pro M5/macOS ARM64, terminale, Git, editor già disponibile e Go darwin/arm64. Baseline verificata in M01: **Go 1.27.4 darwin/arm64**; go.mod richiede almeno Go 1.27.4 e la [relazione M01](milestone-m01.md) registra la toolchain esatta usata. La [release history ufficiale](https://go.dev/doc/devel/release) resta il riferimento per gli aggiornamenti. Niente beta o download automatici durante test offline; `GOTOOLCHAIN=local` evita fetch impliciti. Aggiornare patch di sicurezza dopo revisione e suite; aggiornamenti minor pianificati, non a metà esperimento.

Go e standard library sono sufficienti per il runtime; toolchain con [licenza BSD](https://go.dev/LICENSE). Git è strumento di sviluppo, non dipendenza runtime. Nessun pacchetto Go di terze parti proposto inizialmente: zero generatori, CGO, DB esterni, Raft preconfezionato o servizi di telemetria. GitHub Free opzionale. Renderer Mermaid è opzionale per leggere i diagrammi; i sorgenti restano offline. La licenza di QuorumKV è da decidere dal proprietario prima della pubblicazione.

Ogni nuova dipendenza richiede ADR: necessità concreta, alternativa standard library, versione fissata, licenza, attività manutentori, vulnerabilità note, compatibilità ARM64 senza toolchain extra, RAM/disco e modalità acquisizione/cache offline. Non introdurre un pacchetto solo perché comune in progetti di produzione.

## Configurazione proposta

Nessun binario o file config operativo è incluso. Questa è una proposta di configurazione n1; per n2/n3 cambiano node_id, data_dir e bind secondo [rete](network-protocol.md). I campi comuni sono identici. config_hash è derivato da cluster_id, lista membri ordinata per ID e quote replicate; esclude data_dir e bind locali. La codifica canonica deve essere fissata in M04 come fixture condivisa.

```json
{"config_version":1,"cluster_id":"11111111111111111111111111111111","node_id":"n1","client_addr":"127.0.0.1:7401","peer_addr":"127.0.0.1:7501","data_dir":".local/n1","members":[{"id":"n1","client":"127.0.0.1:7401","peer":"127.0.0.1:7501"},{"id":"n2","client":"127.0.0.1:7402","peer":"127.0.0.1:7502"},{"id":"n3","client":"127.0.0.1:7403","peer":"127.0.0.1:7503"}],"max_logical_bytes":4194304,"max_keys":10000,"max_sessions":1024,"heartbeat_ms":100,"election_min_ms":600,"election_max_ms":1000}
```

Porte distinte e libere, ID immutabili, stessa versione protocollo. Il primo avvio futuro richiederà `--bootstrap` esplicito su directory vuota e manifest comune; avvio ordinario non reinizializza directory vuote accidentalmente. Bootstrap non elegge un leader speciale: tutti partono follower e seguono Raft.

## Interfaccia CLI prevista, non eseguibile oggi

```text
quorumkv node --config examples/n1.json --bootstrap
quorumkv node --config examples/n2.json --bootstrap
quorumkv node --config examples/n3.json --bootstrap
quorumkvctl --endpoints 127.0.0.1:7401,127.0.0.1:7402,127.0.0.1:7403 put config/theme dark
```

Questi nomi e percorsi sono esempi progettuali, non file o comandi già disponibili. Ai restart omettere bootstrap. CLI deve salvare sessione e payload pendente in file privato prima della richiesta; non rigenerare ID ad ogni retry. Per chiudere inviare SIGTERM al PID del proprio run; deadline 5 s, poi harness registra terminazione forzata e conserva dati per recovery.

## Ciclo di lavoro futuro

Seguire [roadmap](implementation-roadmap.md), mantenere main semplice e pacchetti secondo [architettura](architecture.md). Prima proprietà e modello, poi adattatori e misure. Ogni cambiamento semantico aggiorna entrambe le lingue, requisito, ADR, fixture e criterio. Nessun codice inizia automaticamente al termine di questa consegna: occorre approvazione separata della fase progettuale.

Per il modulo M01 ora esistente: formattazione `gofmt`, `go vet ./...` e `go test ./...` sono verifiche eseguite sul package locale. Test race e integration restano proposti per le rispettive milestone, entro i limiti stabiliti. Cache toolchain e moduli pre-acquisite; verificare offline disattivando rete esterna senza bloccare loopback. Nessun container necessario.

## Dati, cleanup e troubleshooting

Tenere `.local/` fuori dal futuro controllo versione; usare solo dati sintetici. Backup manuali non sostituiscono replica o recovery; copiare una generazione attiva senza coordinamento non produce automaticamente backup consistente. Cleanup solo dopo stop e verifica del percorso, mai cancellare una directory per “risolvere” CRC o lock senza aver conservato evidenze.

| Sintomo | Controllo e azione |
|---|---|
| Porta occupata | Individuare PID proprietario; scegliere porte coerenti nei tre config o fermare soltanto il proprio processo. |
| Nessun leader | Verificare peer/config_hash e log elezione; attendere majority stabile, non assegnare manualmente il ruolo. |
| Timeout | Consultare outcome e retry stesso ID; controllare Sync e peer, non presumere rollback. |
| Lock occupato | Verificare processo attivo; mai bypassare esclusione. |
| CRC/manifest invalido | Quarantena e diagnosi; niente reset automatico. |
| Cap storage | Esaminare compaction e temporanei non attivi; non rimuovere WAL attivo. |
| Mac sotto pressione | Fermare test e processi del run; ridurre dataset/concorrenza, conservare misure. |

## Verifiche ora disponibili per M01

Esistono go.mod e internal/statemachine; Go 1.27.4 darwin/arm64 è stato effettivamente usato. Con questa toolchain installata, dalla radice sono eseguibili go test -timeout=30s ./... e go vet ./..., senza servizi o dipendenze esterne. In questa sessione è stata usata una copia temporanea in /tmp, con checksum ufficiale verificato, non un’installazione globale: [dettagli e riproduzione](milestone-m01.md). Restano proposti gli esempi CLI/configurazione dei nodi. M02 non iniziata.
