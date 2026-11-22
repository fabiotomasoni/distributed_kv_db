# Configurazione statica v1

[Indice](README.md) · [English](../en/configuration.md)

**20 novembre 2026 — implementata in M04.**

## Input e ownership immutabile

[config.Parse](../../internal/config/config.go) accetta un solo oggetto JSON UTF-8, massimo 16 KiB, con esattamente i campi di [configuration-v1.json](../fixtures/configuration-v1.json). Numeri interi obbligatori: config_version=1, max_logical_bytes=4194304, max_keys=10000, max_sessions=1024, heartbeat_ms, election_min_ms, election_max_ms. Vietati esponenti, frazioni, segni e zeri iniziali. Stringhe obbligatorie: cluster_id (32 hex minuscoli), node_id, client_addr, peer_addr, data_dir. members è un array di esattamente tre oggetti con id/client/peer; gli ID sono esattamente n1,n2,n3, una volta ciascuno. Nomi sconosciuti/duplicati, Unicode invalido, documenti multipli e annidamento eccessivo falliscono. Nessun default o espansione dell’ambiente.

I sei endpoint sono distinti, IP loopback espliciti canonici e porta decimale non nulla. Gli alias IPv4-mapped vengono confrontati dopo unmapping per l’unicità. Falliscono DNS, IP pubblici/wildcard, IPv6 con zona, porte implicite e URL. I bind locali devono coincidere con gli endpoint del membro corrispondente. data_dir è non vuota e normalizzata, senza attraversamento del parent; controlli effettivi su percorsi privati, file e lock esclusivo competono allo storage. Timing: 0 < heartbeat_ms < election_min_ms <= election_max_ms <= 2147483647. Sono solo configurazioni: M04 non avvia timer elettorali.

Parse restituisce un valore Config immutabile; i campi privati non sono modificabili, Members restituisce una copia dell’array, SharedBytes byte nuovi. ErrConfig significa nessuna configurazione utilizzabile. Il parsing non esegue I/O, DNS, creazione listener o mutazioni di stato. L’accesso concorrente al valore validato è sicuro.

## config_hash esatto

L’hash comprende solo cluster_id, members ordinati lessicograficamente per id e i tre campi quota replicati. Ordinare ricorsivamente i nomi ASCII dei campi JSON, usare numeri interi decimali, nessuno spazio o newline. Tutti i valori del preimage sono ASCII. SHA-256 di questi byte UTF-8, codificato con 64 hex minuscoli, è config_hash. Sono esclusi node_id, client_addr, peer_addr, data_dir locali, timing e config_version; gli endpoint dei membri condivisi restano inclusi. La compatibilità di versione viene controllata separatamente. Le quote sono fisse in v1; modificarle viene rifiutato anziché produrre silenziosamente una variante supportata.

La fixture registra preimage completo esatto e hash atteso:

```text
7331643cc3f13679fc939ad80f62ca4eec3f52c680c8538a2ea0e95ff834c400
```

Permutazione membri o modifiche locali conservano l’hash. Un cambiamento di endpoint condiviso o cluster lo modifica. Non esiste override dell’hash fornito dall’utente. Configurazione e [fixture peer](../fixtures/peer-v1.json) sono verificate da TestConfigurationCanonicalFixtureAndRejections e TestProtocolFixturesAndInvalidMessages. Fixture storage preesistenti invariate.

## Avvio e shutdown

[node.Start](../../internal/node/node.go) riceve il valore validato, un booleano bootstrap esplicito e un Processor trasporto opzionale. Bootstrap richiede cartella privata vuota preesistente; il riavvio usa wal.Open. Entrambi validano/acquisiscono lo storage prima del bind peer. cluster_id/node_id/config_hash persistenti devono coincidere esattamente. Ownership duplicata, metadati divergenti o errore storage impediscono la creazione del listener. Un errore di bind rilascia ownership e conserva i dati inizializzati. Nessun bootstrap automatico o reset cartella.

Dopo un errore inatteso del listener, il lavoro peer viene cancellato/drenato e lo storage chiuso; Shutdown restituisce l’errore serve. Shutdown(ctx) chiude admission, cancella il lavoro di rete, attende processor accettati/chiamate in uscita e termine del server, poi chiude WAL/rilascia LOCK. Un chiamante cancellato non rilascia il lock prematuramente; l’attesa è ripetibile. Processor deve rispettare cancellazione e terminare; Go non può interrompere forzatamente callback arbitrarie. Recovered restituisce stato WAL separato e non applica comandi. L’endpoint client viene riservato/validato ma non aperto prima di M08. Non esiste ancora una CLI nodo. Vedi [evidenze M04](milestone-m04.md).

## Lifecycle nodo eseguibile M09

[cmd/quorumkv](../../cmd/quorumkv/main.go) implementa ora `quorumkv node -config FILE [-bootstrap]`; accetta anche flag diretti senza `node`. File configurazione limitati a 16 KiB e analizzati dallo schema rigoroso esistente. [n1](../../examples/n1.json), [n2](../../examples/n2.json), [n3](../../examples/n3.json) condividono membership/hash e usano directory dati distinte. Percorsi relativi risolti dalla directory di lavoro del processo. Precreare directory private vuote per bootstrap esplicito; restart ordinario omette bootstrap e mai inizializza o resetta storage.

StartAPI conserva validazione identità/LOCK/CURRENT/WAL M03 e replay prefisso committed M07 su macchina nuova prima di servire entrambi listener configurati. SIGINT/SIGTERM ferma ammissione e cancella attese HTTP; owner esistente finisce persistenza attiva e drena apply committed prima di rilasciare entrambe porte e LOCK. Node.Done chiude solo dopo terminazione effettiva risorse, incluso errore runtime automatico. Processo attende massimo 5 s per shutdown, poi esce 1 con shutdown_timeout_recovery_required; file integri conservati per recovery esplicita. Syscall filesystem non responsiva durante startup non interrompibile forzatamente da contesto Go.

Exit 0 significa stop normale/help, 2 argomenti/configurazione invalidi, 1 errore avvio/runtime/shutdown. Diagnostica fissa non stampa config grezza, valori utente o contenuto errori filesystem. node_started significa storage verificato e listener prenotati, non leader eletto o quorum pronto. Terminazione runtime fatale osservata tramite Node.Done produce exit fallimento. Nessun bootstrap quorum, reset forzato, cambio membership, storage sessioni CLI o nuovo endpoint HTTP. [Evidenze M09](milestone-m09.md).

## Profilo applicativo v2 esplicito M11

[ADR-010](adr/ADR-010-session-profile.md) fissa confine compatibilità necessario. Configurazione v1 conserva comportamento M10 e byte hash originali esatti. Configurazione v2 richiede intero canonico aggiuntivo application_version=2; campo incluso nell’hash condiviso, mentre percorsi/bind locali restano esclusi. Versioni sconosciute/mancanti/diverse falliscono. [Configurazione/hash/identità golden](../fixtures/configuration-v2-sessions.json) ed esempi [n1](../../examples/m11/n1.json)/[n2](../../examples/m11/n2.json)/[n3](../../examples/m11/n3.json) condividono contratto v2.

Usare cluster nuovo e directory private vuote con bootstrap esplicito per M11. V2 usa IDENTITY con checksum format_version=2 e codifiche WAL/CURRENT/snapshot vuoto esistenti. Aperture v1/v2 rifiutano altra identità prima di riparazione WAL/replay/listener; nessuna conversione, reset o fallback. Hash condiviso versione applicativa isola anche RPC peer da altro profilo. Tutti tre membri devono usare stesso profilo. Config v2 accettata solo da StartAPI/runtime completo, non helper parziali elezione/replica legacy. Directory v1 esistenti utilizzabili con configurazione v1/semantica M10; nessuna migrazione in-place implementata. Conservarle anziché copiare file attivi.

Mutazioni profilo v2 richiedono un header richiesta X-QuorumKV-Application-Version: 2 e confermano versione 2 nella risposta. Client M11 lo deriva/valida dalla configurazione; conferma assente/diversa è unknown. Modalità legacy corrente rifiuta nuovo header mutante e v2 rifiuta header assente/diverso prima di ammissione. Binari storici possono ignorare nuovi header richiesta, quindi client M11 non considera affidabili loro conferme. GET invariata. [Evidenze M11](milestone-m11.md).
