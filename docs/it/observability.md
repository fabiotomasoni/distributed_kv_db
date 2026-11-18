# Log, metriche e diagnosi

[Indice](README.md) · [English](../en/observability.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

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
