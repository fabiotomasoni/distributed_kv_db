# Protocollo peer e trasporto

[Indice](README.md) · [English](../en/network-protocol.md)

**20 novembre 2026 — trasporto peer, elezioni, replica e propagazione commit M04–M07 implementati e verificati; effetti client/snapshot futuri.**

## Trasporto e indirizzi

Trasporto implementato v1: HTTP/1.1 persistente su TCP, JSON UTF-8, libreria standard `net/http` e `encoding/json`. Rispetto a RPC binario: più byte e parsing, ma ispezionabilità e nessun generatore o dipendenza. TCP non elimina timeout, duplicazione di tentativi o risposte obsolete. No TLS nel modello loopback fidato, no compressione, no proxy ambientale per connessioni peer.

| Nodo | API client | RPC peer | Directory proposta |
|---|---|---|---|
| n1 | 127.0.0.1:7401 | 127.0.0.1:7501 | .local/n1 |
| n2 | 127.0.0.1:7402 | 127.0.0.1:7502 | .local/n2 |
| n3 | 127.0.0.1:7403 | 127.0.0.1:7503 | .local/n3 |

Ogni RPC POST include `protocol_version:1`, `cluster_id` (32 hex), `config_hash` (SHA-256 hex), `from`, `to`, `rpc_id` univoco nel processo e `term` uint64. Indici/termini JSON sono stringhe decimali canoniche (senza segno o zeri iniziali, salvo "0") per evitare perdita di precisione; sequence client resta numero nel suo dominio ristretto. Risposta ripete rpc_id, term corrente e risultato. Identità sconosciuta, cluster/versione/config divergenti: HTTP 400/409 prima di consegnare al core, senza aggiornare il termine.

## Messaggi normativi

| RPC | Richiesta oltre envelope | Risposta HTTP 200 oltre envelope |
|---|---|---|
| `/raft/v1/request-vote` | candidate_id=from, last_log_index, last_log_term | vote_granted boolean |
| `/raft/v1/append-entries` | leader_id=from, prev_log_index, prev_log_term, entries[], leader_commit | success boolean, match_index se success; conflict_index e conflict_term opzionale se fallimento |
| `/raft/v1/install-snapshot` | leader_id=from, snapshot_id, last_included_index, last_included_term, offset, data_b64, done, total_bytes, sha256 | accepted boolean, next_offset; installed_index solo dopo pubblicazione durevole finale |

Entry: `{index,term,kind,command}`; kind in `noop`, `put`, `delete`, `cas`, `read`. noop non ha command; read ha solo key; altri comandi hanno i campi API esclusi timeout e request_id. Gli indici sono contigui da prev+1; termini non decrescono, nessun termine entry maggiore del termine RPC. Un batch malformato non ha effetti parziali.

Esempio progettuale AppendEntries vuoto (envelope completo):

```json
{"protocol_version":1,"cluster_id":"11111111111111111111111111111111","config_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"n1","to":"n2","rpc_id":"n1-42","term":"3","leader_id":"n1","prev_log_index":"8","prev_log_term":"3","entries":[],"leader_commit":"8"}
```

Risposta progettuale:

```json
{"protocol_version":1,"cluster_id":"11111111111111111111111111111111","config_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"n2","to":"n1","rpc_id":"n1-42","term":"3","success":true,"match_index":"8"}
```

## Limiti e flusso

Corpo peer massimo 384 KiB; AppendEntries massimo 16 entry e 256 KiB di comando serializzato complessivo, rispettando il limite del corpo. Entry singola massimo 48 KiB (CAS con due valori Base64). Snapshot a chunk raw 64 KiB, immagine totale massimo 8 MiB, un trasferimento in ingresso per nodo, offset esatto o ritrasmissione identica verificata. Stesso snapshot_id e offset con byte diversi invalida lo staging. Snapshot obsoleti non fanno regredire commit/apply; termine e leadership rivalidati a ogni chunk e prima di installare. Idle staging 5 s, cleanup senza toccare snapshot attivo.

Heartbeat ogni 100 ms; elezione estratta uniformemente in [600,1000] ms a ogni reset. Deadline peer 300 ms, dial 100 ms; lettura header 500 ms, richiesta peer 1 s. Sono parametri iniziali da misurare: broadcast con persistenza deve essere molto inferiore a election timeout. Timer monotoni; pausa del laptop o SSD lento possono interrompere liveness senza autorizzare scorciatoie di safety.

Una RPC attiva per peer; dopo timeout, ritentare al tick con stato corrente. Dopo errore di connessione usare backoff 50..400 ms, non superare un tentativo per tick/peer. Nessun retry del transport nasconde duplicazioni al core. Risposte associate a rpc_id, termine e generazione leader: risposte vecchie non riducono matchIndex o ripristinano una vecchia leadership. Un termine maggiore valido forza step-down anche da risposta tardiva.

Code e backpressure secondo [architettura](architecture.md). Connessioni riusate, pool limitato a due per peer, corpi sempre chiusi; no goroutine per retry senza limite. HTTP non-200 è errore di trasporto/protocollo, non un voto negativo. Errore disco: niente success positivo, nodo passa in stato fatal-storage e smette di votare/replicare; gli altri osservano timeout o 503.

## Shutdown e interoperabilità

Chiudere listener client, poi worker e listener peer secondo deadline di shutdown; nessun flush di log applicativo autorizza acknowledgment Raft. Membership e formato sono fissi; versioni diverse falliscono esplicitamente. Non sono previste rolling upgrade o negoziazione del protocollo. Campi sconosciuti sono respinti: evoluzione richiede incremento versione e piano di migrazione.

## Dettagli di ammissione e snapshot obsoleti

Cap connessioni accettate: 32 sul listener client e 16 sul listener peer per nodo, incluse idle; rifiutare nuovi socket al cap e mantenere deadline per quelli ammessi. I pool in uscita restano limitati a due connessioni per peer; il cap non sostituisce code ed entry limitate.

Se uno snapshot è già coperto da lastApplied, non riscrivere stato. Rispondere `accepted:false`, `reason:"obsolete"`, `next_offset:"0"` e includere `resume_index=max(S,snapshotIndex)` e il relativo `resume_term` locale. Questo confine è già coperto da apply e rappresentabile dal log o dal confine snapshot locale; non usare sempre il solo snapshotIndex, che potrebbe precedere inutilmente il log del leader. Il leader abbandona il trasferimento e riallinea AppendEntries a resume_index+1 soltanto se indice/termine coincide con il proprio log o snapshot; altrimenti ferma il tentativo come incompatibilità di integrità da diagnosticare. matchIndex avanza solo dopo una successiva AppendEntries positiva correlata. Non prendere un indice ricevuto come prova di nuove entry replicate. Gli offset, total_bytes e next_offset sono stringhe decimali; data_b64 usa Base64 canonico. Snapshot ID è il digest SHA-256 del file intero e resta identico nei retry.

## Validazione JSON oltre i default della libreria

La standard library resta sufficiente, ma il normale decode in struct non realizza da solo il contratto rigoroso. Prima del decode semantico verificare UTF-8 valido sul corpo raw; eseguire una scansione token con stack di oggetti per rifiutare chiavi duplicate (anche in expected/comandi annidati), nomi campo non esattamente conformi e profondità >16. Chiavi duplicate dopo unescape JSON sono ancora duplicate. Richiedere un solo oggetto JSON seguito soltanto da whitespace fino a EOF; no secondo documento.

Usare numeri conservati senza conversione float, validare lessico e range di sequence/timeout e stringhe decimali uint64; vietare esponenti, frazioni, segni e zeri iniziali dove il contratto richiede intero canonico. Rifiutare surrogate Unicode isolate nei testi prima che vengano sostituite silenziosamente, e Base64 non canonico confrontando decode/encode. Il formato canonico per hashing/WAL si ricostruisce dai campi tipizzati; input JSON può usare escape leciti equivalenti. T-003 e T-009 includono duplicate key, differenze di maiuscole nei nomi, UTF-8 invalido, surrogate isolate e JSON concatenati. Vedere [Go encoding/json](https://pkg.go.dev/encoding/json).

## Proiezione canonica entry

Il [formato storage](canonical-formats.md) fissa campi kind/command annidati esatti e dimensione entry canonica, separati dagli envelope HTTP. noop omette command; read contiene soltanto key. Ricostruire canonicamente le entry validate per persistenza; copertura fixture protocollo/configurazione implementata e verificata in M04.

## Confine implementativo M04 e fixture

Il livello trasporto/envelope prima proposto è ora implementato: [relazione M04](milestone-m04.md), [configurazione statica](configuration.md), [fixture RPC esatte](../fixtures/peer-v1.json). Tutti e tre gli schemi vengono controllati, senza implementare voto, replica o staging/installazione snapshot. Un tentativo, admission stesso peer posseduta dal chiamante, dial 100 ms/totale uscita 300 ms, header 500 ms, handler/read/write/idle ingresso 1 s, due socket nel pool per peer e 16 socket accettati. Processor nil dà 503; le callback devono rispettare cancellazione. rpc_id usa 1–128 lettere/cifre ASCII o -_.:, unico per responsabilità chiamante/processo. Richiesti Host configurato/HTTP1.1/POST/JSON; schema invalido 400, identità 409, dimensione 413, media 415, percorso 404, metodo 405. Parsing rigoroso e verifiche byte fixture superati. Retry/backoff, effetti termine/generazione leader, stato offset/ritrasmissione snapshot e decisioni positive durevoli restano al core/integrazione futuri.

## Processor elettorale M05

StartElection collega ora Service.Handle: RequestVote implementa voto durevole, AppendEntries vuota gestisce heartbeat elettorale/validazione prev. Il noop nuovo termine è solo locale e non viene trasmesso come replica. Heartbeat riconosciuto non obsoleto resetta il timeout anche con prev divergente; heartbeat obsoleto/voto negato no. Termini maggiori validi, incluse risposte vecchie correlate o richieste append/snapshot non supportate, vengono persistiti prima di output dipendente. Effetti append non vuote/snapshot danno 503/indisponibile; nessun avanzamento commit o conteggio progresso. Retry core su tick successivi con backoff errori peer 50..400 ms, mai paralleli verso stesso peer. Contratti envelope/body/socket/deadline esistenti invariati. [Test e limiti](milestone-m05.md).

## Semantica AppendEntries M06 effettiva

[M06](milestone-m06.md) gestisce AppendEntries non vuote, retry entry identiche, hint indice/termine conflitto limitati e acknowledgment match prefisso dopo Sync. Batch uscita limitano byte entry canoniche a 256 KiB oltre a 1..16 entry configurate. Batch ingresso leciti oltre cap byte entry WAL divisi in transazioni durevoli sequenziali limitate, una risposta finale; validazione wire completa prima del primo effetto. Sent/Failed registrano tentativi trasporto effettivi; risposte obsolete/duplicate non cambiano progresso corrente, termine maggiore valido declassa comunque. leaderCommit conservato nello schema ma avanzamento operativo commit attende M07. Installazione snapshot resta indisponibile.

## Comportamento leaderCommit M07

StartCommitted abilita [M07](milestone-m07.md): AppendEntries uscente trasporta checkpoint sincronizzato; follower avanza solo fino al confine richiesta verificato dopo Sync suffisso/checkpoint. Traffico rifiutato/obsoleto non propaga commit. Risposta replica positiva prova prefisso/checkpoint durevoli, non applicazione o successo client. Entry read future ed effetti InstallSnapshot restano indisponibili; limiti schema/body/socket/deadline originali invariati. Tutti i nodi devono selezionare capacità compatibile all’avvio, nessuna negoziazione rolling feature.

## Contratto InstallSnapshot precisato prima di M13

La [revisione pre-M13](pre-m13-review.md) mantiene schema/limiti peer esistenti e precisa il fence finale: drain apply, nuova verifica S contro LastApplied e termine/leader, conservazione del suffisso soltanto con match (S,termS), nessuna distruzione del committed e voto locale aggiornato. installed_index segue pubblicazione durevole e sostituzione atomica KV/sessioni; chunk ack non prova replica. Se S diventa obsolete durante transfer usare resume e successiva AppendEntries verificata, senza inventare matchIndex. Nessun handler InstallSnapshot o transfer manager implementato qui.

## M13 Fase E — 23 novembre 2026

La [Fase E](milestone-m13-installsnapshot-preliminary.md) è **ISOLATED INSTALLSNAPSHOT PHASE COMPLETED**: ricevente/controller dormienti, chunk M04 esistenti, persistenza reale M05 di termine/voto, suffisso locale coerente, pubblicazione B sotto LOCK continuo, KV/sessioni nuovi e apply epoch, ACK finale correlato e catch-up deterministico del suffisso verificati in isolamento. Nessuna integrazione ordinaria node/service/server/transport. C-01/OQ-05 confermato; aggiunte evidenze isolate C-02/OQ-01 e C-03, entrambi complessivamente CONDITIONALLY READY. T-015/AC-015/FR-015, T-016/AC-016/FR-016 e T-026/AC-026/NFR-006 completi restano aperti. InstallSnapshot runtime, compaction live, GC ed eliminazione prefissi non attivati; M13 PARTIALLY COMPLETED, M14 non iniziata.

Il modello dormiente snapshotinstall riusa tutti i campi richiesta/risposta esistenti; il vincolo generazione resta locale. Chunk raw <=64 KiB, immagine <=8 MiB, un buffer staging, offset esatto o retry byte identico, scadenza inattiva 5 s. Ricezione chunk solo in memoria, mai avanzamento matchIndex. installed_index segue CURRENT durevole, adozione writer, sostituzione KV/sessioni e applicazione suffisso committed. ACK finale perso: obsolete/resume con termine verificato localmente e successivo AppendEntries positivo prima del progresso match. Termini non validi/superiori/inferiori passano da core/WAL reali; effetti Snapshot ordinari ancora unsupported.
