# Log, metriche e diagnosi

[Indice](README.md) · [English](../en/observability.md)

**22 novembre 2026 — profilo M12 implementato e verificato; sezioni progettuali storiche precisate dal contratto M12 in fondo.**

## Log strutturati

Proposta: `log/slog`, JSON line-oriented. Campi stabili: timestamp UTC per lettura umana, elapsed monotono per misure, level, event, node_id, cluster_id, term, role, leader_id, rpc_id, request_id, client_id abbreviato, sequence, log_index, commit_index, applied_index, duration_us, error_code, scenario_id. Non inserire valori, payload CAS, percorsi personali o chiavi arbitrarie. client_id non va usato come label di metrica.

INFO: avvio, shutdown, cambio termine/ruolo, elezione vinta, snapshot pubblicato e recovery completato. WARN: timeout aggregati, rifiuto quota e retry eccessivi. ERROR: storage fatal, corruzione e violazioni invarianti. DEBUG: progresso replica campionato; disattivato normalmente. Heartbeat riusciti non producono una riga INFO ciascuno.

Correlazione: request_id cambia per tentativo HTTP; `(client_id,sequence)` identifica la mutazione; rpc_id identifica tentativo peer. Il log di apply conserva indice e codice, senza valore. Un retry può avere nuovo request_id e index originale: non dedurre il numero di modifiche contando risposte HTTP.

## Metriche locali

GET `/v1/status` è copia locale eventualmente obsoleta, HTTP 200 anche senza quorum: `node_id`, `role`, `term`, `leader_id`, `commit_index`, `applied_index`, `last_log_index`, `snapshot_index`, `ready`, `storage_error`. `ready` indica recovery e listener pronti, non maggioranza verificata per una nuova operazione.

GET `/v1/metrics` futuro restituisce JSON locale: contatori RPC sent/failed per tipo e peer (dominio fisso), election_started, leader_changes, client_completed/unknown/rejected per codice, dedup_hits, snapshot_count; gauge pending_proposals, match_index per peer, apply_lag, goroutines, heap_bytes, wal_bytes, snapshot_bytes, directory_bytes; distribuzioni in memoria di request/RPC/Sync/apply/election/recovery duration. Campioni limitati a 2 048 osservazioni per distribuzione; report indica finestra e numero campioni. Endpoint metrics non è sorgente di stato di consenso.

RSS e CPU richiedono osservazione del processo tramite macOS; heap Go non equivale a RSS. Tempo elezione: dalla perdita introdotta dal coordinatore al primo noop committed del nuovo leader. Recovery: da avvio processo a replay completato, distinto da catch-up e prima operazione riuscita.

## Overhead e retention

Polling manuale o ogni 2 s, massimo 30 s per sessione ordinaria. Nessun Prometheus/Grafana/cloud obbligatorio. Buffer log 256 record, massimo 1 KiB/record; se saturo, scartare DEBUG/INFO campionabili e incrementare dropped_logs, mai perdere errori fatal senza tentare stderr. Errori ripetitivi aggregati una volta al secondo. Una scrittura log non influenza quorum.

Rotazione proposta 256 KiB, quattro file totali = 1 MiB/nodo, inclusi file corrente e ruotati; chiusura ordinata, nessun fsync per ogni riga di debug. Test evidenze separati entro quota 10 MiB. Le tracce di test che verificano invarianti non possono essere campionate: se finiscono spazio o buffer, test inconclusive e stop.

## Diagnosi per sintomo

Nessun leader: verificare term/vote, peer address, config_hash e timer. Commit fermo: osservare matchIndex, errori RPC e Sync; non aumentare timeout senza misurare. Apply lag: controllare applier e code. Crescita disco: controllare prefisso compattabile e staging residuo. RAM alta: heap/goroutine e code; non confondere cache OS con memoria del processo. [Guida](development-guide.md) e [risorse](performance-and-resources.md) precisano la procedura locale.

## Diagnostica M12 implementata e limiti effettivi

[M12](milestone-m12.md) espone GET /v1/metrics nel listener client loopback esistente, Cache-Control: no-store, JSON metrics_version=1/local_only=true. Nessun quorum richiesto; status M10 invariato. [OpenAPI M12](../openapi/m12.json) aggiunge soltanto questa operazione, mantenendo tutte le operazioni M11 identiche. Configurazioni/identity v1 e v2, schemi PUT/DELETE/GET/CAS e header v2 invariati; nessuna migrazione automatica.

Recorder sincronizzato e limitato per istanza: contatori aggregati a nomi fissi per RPC inviate/fallite, elezioni, acquisizioni leadership, risposte completed/unknown/rejected, dedup_hits, apply, recovery ed errori storage. Nessuna etichetta arbitraria, chiave o client_id. Gauge locali: ruolo/termine/leader, commit/apply/log/snapshot (snapshot zero), match_index, proposte attive, apply_lag, goroutine/heap Go e byte WAL sincronizzati/osservati dal driver. Heap non è RSS. Snapshot contatori/durate e gauge possono riferirsi a istanti vicini diversi; non sono una lettura linearizzabile.

Finestre indipendenti di massimo 2048 campioni recenti in microsecondi: request, RPC, chiamata Append+Sync, apply dominio (esclude noop), elezione locale da candidatura a primo commit termine corrente, ricostruzione del prefisso committed nel servizio. Count è totale dall'avvio dell'istanza; samples_us è solo la finestra recente, senza percentili o benchmark. Recovery non misura l'intero avvio processo/WAL-open; election non misura il tempo da perdita del leader esterno. Nessuna misura RSS/CPU/directory_bytes o metriche snapshot anticipate; fan-out RPC per peer/tipo e aggregazioni temporali avanzate restano estensioni, non promesse di questa versione.

Log JSON strutturati con timestamp UTC, elapsed_us monotono, livello, evento, nodo/cluster, termine/ruolo/leader, indici, request_id server e codice/outcome/durata quando pertinenti. Mai chiavi, valori, payload CAS, client_id completi o percorsi/errori grezzi. Eventi ruolo, recovery, startup/shutdown, apply e risultato HTTP; nessuna riga INFO per heartbeat sano. Ring 256 record, <=1 KiB ciascuno; sovrascritture contate in dropped_logs. La singola failure storage fatale dell'istanza ha copia riservata anche dopo sovrascrittura. Il core non esegue I/O log e non attende consumer esterni. Il coordinatore processo drena ogni 2 s e in shutdown: default stderr, oppure -log-dir in cartella privata separata e non annidata nella directory dati. Rotazione quattro file fissi di massimo 256 KiB, totale 1 MiB; nessun fsync per riga. Failure fatale tenta anche stderr. Errori output diagnostici sono segnalati senza cambiare consenso; blocchi arbitrari del filesystem/stderr non hanno bound garantito, come gli altri I/O locali.

Poll manuale o ogni 2 s per massimo 30 s nella sessione ordinaria; nessuna dashboard/cloud/processo permanente. Log operativi campionabili e trace invarianti completa hanno autorità distinta: dropped_logs non rende falsa la diagnostica, ma una trace simulata incompleta interrompe il test come inconclusive. [Fixture metriche](../fixtures/metrics-m12.json), prove HTTP reali di elezione/timeout senza quorum, race, redazione e rotazione verificano il profilo.
