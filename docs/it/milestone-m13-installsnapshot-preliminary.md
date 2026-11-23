# M13 Fase E — InstallSnapshot isolato e recupero dei follower

[Indice](README.md) · [English](../en/milestone-m13-installsnapshot-preliminary.md)

**23 novembre 2026 — ISOLATED INSTALLSNAPSHOT PHASE COMPLETED. M13 complessiva: PARTIALLY COMPLETED. M14 NOT STARTED.** Revisione iniziale pulita `b9ebde7`; nessun commit Git eseguito in questo incarico.

## Autorizzazione e perimetro ufficiale

La [roadmap](implementation-roadmap.md) assegna a M13 snapshot, compaction, generazioni, installazione e sottoinsiemi pertinenti di T-015/T-016/T-026. M14 mantiene checker generale e proprietà. Esaminati prima del codice [revisione pre-M13](pre-m13-review.md), [A](milestone-m13-preliminary.md), [B](milestone-m13-persistence.md), [C](milestone-m13-runtime-recovery.md), [D](milestone-m13-compaction-preliminary.md), ADR-005/ADR-010, formati canonici, wire M04, termini M05, matching M06, apply M07 e sessioni M11, insieme a entrambe le roadmap e al simulatore M12.

In ingresso C-01/OQ-05 confermato; C-02/OQ-01 e C-03 complessivamente CONDITIONALLY READY; T-015/T-016/T-026 completi aperti. L'incarico e la continuazione approvata della Fase D consentono implementazione dormiente e verifica isolata con formati e primitive esistenti. Nessun gate vieta questo perimetro limitato. **Decisione precedente al codice: autorizzato in isolamento**, senza autorizzare il percorso ordinario del cluster. Nessun nuovo campo wire, versione di formato, criterio di accettazione o deroga.

## Componenti e file modificati

| File | Responsabilità |
|---|---|
| internal/snapshotinstall/receiver.go | Ricevente dormiente; un owner WAL, core Raft reale, worker M02 limitato, staging e handoff serializzato. |
| internal/snapshotinstall/transfer.go | Controller leader finito e immutabile; chunk correlati e AppendEntries espliciti del suffisso. |
| internal/snapshot/install.go | PlanInstallation puro e PublishReceived tramite il solo writer della Fase B. |
| internal/wal/snapshot_transition.go | Transizione esplicita dell'owner con LOCK continuo e token della nuova generazione. |
| internal/snapshot/persistence.go | Estrazione della pubblicazione in publishViewOwned; pubblicazione pubblica B e orchestrazione D conservano la semantica. |
| internal/wal/open.go; internal/wal/store.go | Conservazione dell'identità immutabile per riapertura controllata sotto lo stesso lock. |
| internal/raft/commit.go | EnableRecoveredCommit dormiente, solo all'avvio, con cursori già applicati ed epoch esplicita. |

Sette nuovi file di test: snapshotinstall/receiver_test.go, safety_test.go, lifecycle_test.go, term_test.go; snapshot/install_test.go; wal/snapshot_transition_test.go; raft/snapshot_dormancy_test.go. Contengono 29 funzioni principali, incluso un helper subprocess, e sottocasi limitati. Aggiornati i due report, indici, protocollo, formati, Raft, storage, architettura, failure model, testing/accettazione/tracciabilità, sviluppo, roadmap e ADR-005/ADR-010.

Nessun sorgente ordinario node, service, server, transport o election/replication importa snapshotinstall. StartAPIWithSnapshotRecovery rimane la capacità della Fase C; avvio/CLI ordinari e garanzie API client invariati. Il core ordinario continua a rifiutare gli effetti applicativi Snapshot con ErrUnsupported dopo l'eventuale persistenza reale M05 del termine; una regressione dedicata lo verifica. Nessun fallback automatico, listener, goroutine di trasferimento, trigger snapshot, compaction live, GC, eliminazione prefissi di produzione, migrazione o lavoro M14.

## Contratto wire esistente e stati di ricezione

Riutilizzato Snapshot M04 su `/raft/v1/install-snapshot`, protocol_version 1. Envelope: cluster_id, config_hash, from, to, rpc_id, term. Campi richiesta: leader_id, snapshot_id, last_included_index, last_included_term, offset, data_b64, done, total_bytes, sha256. Interi secondo le regole esistenti delle stringhe decimali. snapshot_id=sha256=SHA256 del file QSN1 completo, trailer incluso, distinto dal digest interno del payload. La generazione sorgente vincola il controller locale leader e **non è un campo wire inventato**.

Ogni chunk raw <=64 KiB; immagine totale 44 byte..8 MiB; un trasferimento in memoria per ricevente. Il buffer cresce solo con chunk effettivamente validati, senza allocare una dimensione dichiarata incontrollata. L'offset esatto successivo aggiunge dati. Un intervallo già ricevuto integralmente e identico è un retry; un gap restituisce l'offset atteso, sovrapposizioni parziali/byte conflittuali invalidano staging, metadati incoerenti lo invalidano, altro ID viene rifiutato mentre occupato. Tempo monotono esplicito: scadenza dopo 5000 ms inattivi. Close/restart scartano il buffer e ripartono da offset zero. È ricezione limitata in memoria, non progresso parziale durevole.

Le risposte mantengono accepted e next_offset. Un'accettazione parziale modifica solo offset del mittente, mai matchIndex. Solo installed_index=S correlato, accepted/done e lunghezza completa dimostra installazione. Risposta obsoleta: accepted=false,next_offset=0,reason=obsolete,resume_index,resume_term; resume=max(S,base locale), con termine locale effettivo. Il leader verifica la coppia sul proprio log congelato; nextIndex può passare a resume+1 ma **matchIndex resta invariato fino al successivo AppendEntries positivo valido**. Una conferma finale persa conduce quindi a retry sicuro oppure obsolete+scambio verificato del suffisso, senza duplicare apply.

## Termini, identità e proprietà del trasferimento

Ogni richiesta viene rivalidata da peerprotocol prima degli effetti. Il ricevente guida il **core M05 reale**, sincronizza gli esatti effetti persistenti tramite WAL e restituisce token Durable reali. Un termine superiore aggiorna prima l'hard state locale secondo le regole del voto; un leader valido nello stesso termine conserva il voto locale. Termini inferiori non installano. Vote/AppendEntries di un nuovo termine/leader invalidano staging precedente; chunk vecchi ritardati non sovrascrivono il nuovo stato. Leader conflittuali nello stesso termine portano a errore secondo M05.

Il controller leader vincola termine, generazione, nonce di processo e correlazione RPC. Risposte ritardate di termine/generazione locale superati non avanzano il progresso; correlazioni estranee vengono rifiutate. Risposte con termine superiore producono HigherTerm senza progresso: il driver deve consegnare la risposta al core M05 reale e persistere lo step-down. Un test elegge realmente un leader ed esegue questa transizione durevole. Il modello è a owner singolo, finito e guidato esplicitamente; non introduce un altro motore elettorale né algoritmi automatici di retry/backtracking di rete.

## Indici compattati e riconciliazione del suffisso

Siano b la base locale, S il confine in ingresso, A l'applicato reale, C il commit e N l'ultimo indice. base_index/base_term rappresentano la sentinella; entry b+1..N. Indici <=b mai entry applicative; con suffisso vuoto LastLogIndex=b e LastLogTerm=base_term. RequestVote esistente confronta ultimo termine/indice dopo restart; AppendEntries accetta prev alla sentinella soltanto col termine corretto, rifiuta indici precedenti e riconcilia il suffisso disponibile tramite M06. Nessuna entry zero fittizia o indice di slice ribasato.

L'installazione prima parcheggia/drena lavoro accettato e completamenti reali, provando A=ScheduledApplyIndex=cursore worker. Ricontrolla leader corrente, termine e apply epoch; S<=A è obsoleto, anche se lo è diventato durante il drain. Non si usa un cursore commit supposto.

| Relazione al confine | Nuova generazione |
|---|---|
| Termine locale[S]=termineS in ingresso | Conserva **tutte** le entry S+1..N, speculative incluse; C diventa max(C precedente,S). |
| Nessun confine corrispondente e S<=C precedente | Rifiuta: non si possono scartare dati committed già noti. |
| Nessun confine corrispondente e S>C precedente, incluso log più corto | Snapshot stabilisce base/C/N=S; scarta soltanto il suffisso speculativo incompatibile nella **nuova** generazione. |

Termine/voto correnti locali restano autorevoli, mai il voto del mittente. KV e sessioni rappresentano lo stesso prefisso S; poi si applica il suffisso committed conservato S+1..C, mentre C+1..N resta disponibile ma non applicato. Sequence/token generazione sono coordinate storage, non indici Raft. Controlli aritmetici evitano wrap. Il formato rappresenta MaxUint64; l'attivazione rifiuta N=MaxUint64 prima della pubblicazione perché il core esistente non può formare nextIndex=N+1. Anche overflow epoch rifiutato prima di pubblicare.

## Validazione, persistenza e handoff atomico

Decode/canonicalità validano container/payload QSN1 v1, righe KV/sessioni complete e cap 1024, configurazione/identità persistenti v2, cluster/config hash, lunghezza immagine, hash del file completo, confine e hard state locale corrente. Un file valido da solo non autorizza. PlanInstallation valida una vista locale finale immutabile e riutilizza generationWAL/Replay. PublishReceived rilegge CURRENT senza tail repair e verifica sequence/base/termine/hard-state/commit/ultimo indice sotto il lock esclusivo dello store. Selezione non valida mai recuperata scegliendo una generazione precedente.

TransitionSnapshot serializza Append/Close e presta lo **stesso LOCK stabile** a un publisher finito e non rientrante; non rilascia/riacquisisce proprietà. PublishReceived usa la sola sequenza B: riserva spazio vecchio+nuovo+staging; gen-N.tmp nuovo; scrittura completa snapshot/WAL, Sync, Close, rilettura; fsync directory staging; rename generazione/fsync root; scrittura/Sync/Close/rilettura CURRENT.tmp canonico; rename CURRENT/fsync root. gen-N contiene soltanto snapshot e wal, CURRENT è il manifest autorevole. Nessun terzo manifest, writer alternativo, selezione della generazione massima o rollback.

Il WAL include nuova base/hash snapshot, hard state locale aggiornato e suffisso/commit coerenti. Prima del rename CURRENT vale la vecchia selezione; dopo, il recovery seleziona la nuova anche se il chiamante fallisce. Errori callback/riapertura/Close del vecchio file rendono l'owner inutilizzabile, conservano candidati e non confermano. Una transizione riuscita deve selezionare davvero una nuova generazione, aggiornare file/stato/token e corrispondere all'hash/base ricevuti. Vecchi file conservati; le legittime append M05 dell'hard state prima dello switch preservano il prefisso del vecchio WAL, senza promettere che l'intero file resti invariato.

Il mutex del ricevente esclude RPC/lifecycle/nuove ammissioni durante l'handoff isolato. Il lavoro M02 già accettato viene drenato anche se la richiesta è cancellata, consumando completamenti reali, rilasciando la barriera e terminando il vecchio worker **prima della pubblicazione**. Dopo pubblicazione e adozione writer si ripristinano insieme KV/sessioni su macchina nuova, si incrementa applyEpoch logica, si costruiscono nuovo core/applier a S, si eliminano vecchi pending/risultati e si applica il suffisso committed fino a C. Solo dopo si invia installed_index=S. Completamenti della vecchia epoch ignorati. View diagnostica cattura una macchina nuova con epoch di cattura Fase A pari a 1; la distinta epoch logica del core cresce all'handoff. Non si implica riuso di waiter live o result ring storici.

## Catch-up deterministico ed equivalenza

Il modello leader possiede base S=4, termineS=2, termine 5, C=10,N=11 e suffisso immutabile. Il follower parte prima di S; nextIndex<=S non è servibile da AppendEntries ordinari sul log attivo leader. Invio esplicito chunk, installazione, quindi ripresa AppendEntries M06 con prev=S/termineS. I test confrontano log completo e righe applicative con applicazione diretta del prefisso committed completo, non solo digest/conteggio.

La piccola storia include PUT, DELETE, read, noop, CAS riuscito/fallito, duplicati e richieste stale, più client e una entry speculativa non applicata. Confronti su byte/presenza, client/sequence/fingerprint/kind/risultato cached/indice originale di ogni sessione, C/A/scheduled, hard state, confine e intero suffisso. Dopo restart e sostituzione dell'entry speculativa, i retry mantengono i risultati originali CAS riusciti e falliti anche quando il KV corrente darebbe un risultato diverso. Il caso crash con stesso KV ma sessioni diverse rileva la scelta della generazione errata. Una cattura/trasmissione effettiva di 1024 client verifica assenza di eviction, rifiuto deterministico del client 1025 e avanzamento di un client esistente.

Due esecuzioni catch-up con input fissi e nonce test fisso confrontano byte delle tracce richiesta/risposta e immagine/log finali, indicando l'indice della prima divergenza. Gli scenari M12 esistenti con manifest/seed restano regressioni; il simulatore non riceve azioni snapshot non supportate. Il nuovo driver isolato finito usa core/storage/applier reali e seam privati di pubblicazione. Nessuna pretesa di checker generale indipendente M14.

## Test ed evidenze fault

| Gruppo | Osservazioni eseguite |
|---|---|
| Confini/elezioni | Suffisso vuoto, prev corretto/errato/prima/dopo confine, freshness RequestVote prima/dopo restart, aritmetica MAX/attivazione rifiutata. |
| Ricezione | Duplicato esatto, gap, overlap, byte conflittuali, metadati/ID cambiati, scadenza, regressione clock, Close/restart parziale. |
| Autorizzazione/validazione | Termine inferiore/uguale/superiore, nuovo leader/voto, chunk/risposte vecchi ritardati, identità/profilo/versione/hash/confine incompatibili, limiti wire, CURRENT corrotto. |
| Suffisso/apply | Conservazione committed+speculativo, scarto mismatch non committed solo nella nuova generazione, rifiuto conflitto committed, vecchio worker bloccato e snapshot diventato obsoleto durante drain. |
| Idempotenza | Risposta finale persa, obsolete+AE verificato, immagine duplicata, recovery ripetuto, CAS falliti/riusciti cached, sequence stale, cap sessioni pieno. |
| Pubblicazione | Tutti i 16 cutpoint B esistenti attraverso pubblicazione ricevuta; prima rename CURRENT selezione vecchia, dopo nuova; replay dettagliato e sorgenti preservate. |
| Guasti file | 12 combinazioni: snapshot/WAL/CURRENT.tmp con ENOSPC parziale, Sync, Close o rilettura corrotta; nessuna selezione di generazione non valida. |
| Handoff | Errori iniettati a image.validated, before-generation, generation.published e memory.installed; nessun ACK finale. |
| Uscite reali processi | Un figlio alla volta esce a before-generation, generation.published o memory.installed; restart ripetuto seleziona stato vecchio/nuovo recuperabile e termine persistito, retry installa una volta o restituisce obsolete. |
| Proprietà | Probe LOCK prima/dopo pubblicazione/dopo memoria rifiutano sempre un secondo opener; errori callback/riapertura/pubblicazione assente/vecchio Close rendono inutilizzabile; append successiva riuscita usa token nuova generazione. |
| Risorse | Limiti dataset/code esistenti, limiti trasferimento/chunk/sessioni, rifiuto riserva di picco prima delle scritture; nessun cleanup per fare spazio. |
| Dormienza | Effetti Snapshot del core ordinario ancora non supportati; nessun import di integrazione produzione o trasferimento automatico. |

Directory temporanee, cutpoint deterministici e wrapper I/O usano primitive reali esistenti. Le uscite dei figli simulano perdita processo, non reboot OS/alimentazione. CURRENT corrotto rifiuta senza fallback. I test di pubblicazione esaminano byte/stato selezionati oltre agli errori. Residui staging/control conservati e conteggiati, non rimossi silenziosamente; vengono poi rimosse solo directory temporanee di proprietà dei test.

## Verifiche eseguite e copertura misurata

Superati tutti i nuovi test mirati e la suite completa di regressione M01–M12/M13 A–E, con copertura, package seriali, GOMAXPROCS=2, impostazioni moduli offline e timeout 75 s/package. Tempi esecuzione completa: snapshotinstall 2.052 s, snapshot 8.586 s, raft 0.614 s, wal 0.948 s, node 17.125 s (incluso precedente recovery a tre processi), server 8.571 s. Comando completo e riproduzioni mirate nella [guida sviluppo](development-guide.md).

Race detector mirato superato: snapshotinstall 5.351 s, snapshot 16.653 s, wal 3.232 s, raft 6.480 s, applier 1.233 s, committed 2.259 s e storagefs 2.439 s. Gofmt senza formattazione pendente; go vet e build darwin/arm64 superati. Verificatore canonico superato; i test Go esistenti verificano separatamente codec/recovery e fixture wire. go list -m all restituisce solo il modulo del progetto, go.mod invariato e nessun go.sum/dipendenza aggiunta. git diff --check superato; HEAD resta b9ebde7. Revisione chiamanti conferma assenza di integrazione ordinaria, eliminazione automatica o trasferimento runtime.

Copertura istruzioni unione, deduplicando blocchi identici importati da più package test da `/tmp/quorumkv-m13e.cover`: snapshotinstall 84.2% (331/393), snapshot 87.0%, raft 88.6%, wal 84.4%, applier 95.1%, committed 85.1%. Nuovi receiver.go 84.2%, transfer.go 84.3%, snapshot/install.go 87.5%, wal/snapshot_transition.go 92.9%. Le righe cross-coverage per package del log grezzo non sono queste percentuali unione. Copertura non dimostra safety runtime assente.

Controllo bilingue meccanico: 146 Markdown, 72 coppie linguistiche, 2191 link relativi, 132 tabelle e 42 esempi non Mermaid identici; nessun errore link, fence, colonne/ID/conteggio sezioni o parità blocchi tecnici. Revisione semantica mantiene allineati perimetro/stati/limiti e tutti i criteri ufficiali. Inventario: 58 sorgenti Go interni di produzione, 80 file di test interni. Nuovi test trasferimento/sessioni effettivi piccoli e finiti. Cap logici esistenti: dataset 4 MiB, 10000 chiavi, valore 16 KiB e 1024 sessioni; immagine <=8 MiB, WAL <=8 MiB inclusa riserva consenso 512 KiB, directory <=32 MiB inclusa riserva controllo 64 KiB. Pubblicazione con riserva rifiuta prima delle scritture senza eliminare vecchi file. Nessuna misura RSS/CPU o picco SSD fisico dichiarata.

## Criteri ufficiali e decisioni sui gate

| Criterio / obiettivo ufficiale | Evidenze A–D, poi aggiunta E | Evidenze mancanti e prerequisiti |
|---|---|---|
| T-015 / FR-015 / AC-015: follower molto arretrato, retry chunk e cambio termine; hash immagine, sessioni, indici finali | A codec/cattura/restore, B immagine durevole, C suffisso/restart, D vista compatta equivalente. E aggiunge chunk isolati, persistenza termini reale, epoch/drain obsoleto, ACK finale/retry e catch-up suffisso. | **APERTO completo**: RPC/fallback leader ordinari, replica reale concorrente, integrazione elezioni/retry/shutdown. Necessario prefisso logicamente compattato; GC non necessario per provare il trasferimento stesso. |
| T-016 / FR-016 / AC-016: arresto a ogni cutpoint pubblicazione/cleanup; selezione recuperabile dopo ciascuno | A integrità, B pubblicazione, C recovery processi, D equivalenza con sorgenti conservate. E aggiunge generazione ricevuta e transizione writer/memoria, guasti file e uscite figli attorno alla pubblicazione. | **APERTO completo**: switch compaction live, eliminazione prefissi e cutpoint cleanup/GC. Richiedono retention e autorità di eliminazione sicure; installazione isolata non prova cleanup. |
| T-026 / NFR-006 / AC-026: quote dataset, code e disco simulato; errori, RSS, CPU, spazio di picco | A cap logici, B riserva, C code/recovery, D cap 1024/suffisso conservato. E aggiunge limiti chunk/immagine/staging, trasferimento al cap, rifiuto reale della riserva di pubblicazione. | **APERTO completo**: interazioni runtime ammissione/trasferimento/cleanup e report misurato RSS/CPU/spazio di picco. GC incide sullo spazio sostenuto, non serve per verificare rifiuti isolati limitati. |

Criteri originali in [testing](testing-strategy.md) e [accettazione](acceptance-criteria.md) invariati. Nessun criterio completo indebolito o chiuso da un sottoinsieme isolato.

C-01/OQ-05 **RESOLVED/confermato**: codec/wire/profilo v2 e fixture canoniche invariati. C-02/OQ-01: sottoinsieme isolato pubblicazione ricevuta/transizione owner/restart verificato, complessivamente **CONDITIONALLY READY** per ciclo live/cleanup/fault rimanenti. C-03: handoff isolato termini/barriere/epoch/dedup/suffisso verificato, complessivamente **CONDITIONALLY READY** per lifecycle RPC/replica reale e sicurezza eliminazione. Codec, generazioni, recovery combinato, compaction isolata e InstallSnapshot isolato hanno evidenze; InstallSnapshot runtime, compaction runtime e GC restano non attivati/non verificati.

## Problemi, correzioni e limiti

La revisione ha aggiunto rifiuto della stessa generazione nella transizione con lock prestato e verifica hash/base dell'immagine dopo adozione, evitando falsi handoff riusciti se il publisher non fa nulla o seleziona una vista inattesa. Test coprono pubblicazione assente ed errori riapertura/Close. Il solo protocollo di pubblicazione è stato estratto per accettare la vista locale riconciliata; nessuna seconda persistenza o riparazione di vecchi formati. Nessun difetto bloccante noto rimane nel perimetro isolato dopo le verifiche eseguite.

Garanzie verificate limitate da storie sintetiche e modelli fault: nessuna regressione committed/sessioni nota, pubblicazione coerente validata, vecchi candidati conservati, vecchio worker drenato, epoch/token aggiornati e progresso finale correlato. Non dimostrati: InstallSnapshot distribuito ordinario, concorrenza con code/waiter/timer del service, liveness con guasti arbitrari, eliminazione/GC sicuri, durabilità fisica APFS dopo power loss, guasto SSD e correttezza formale del consenso. Suite finita verde o race detector non sono una prova.

Budget aggiuntivo **0 €**, tutto offline con libreria standard e cache/toolchain Go 1.27.4 darwin/arm64 esistenti sul Mac ARM64 già posseduto. GOMAXPROCS=2, package seriali, deadline brevi esplicite; storie minime, massimo tre nodi ordinari nelle regressioni precedenti, un figlio crash alla volta. L'immagine delle 1024 sessioni viene trasferita in chunk limitati <=64 KiB; nessuno stress vicino agli 8 MiB. Nessun cloud, API a pagamento, hardware nuovo, GPU, VM/Docker, benchmark intensivo, crash OS o modifica di directory reali di produzione.

## Decisione e requisiti prima dell'attivazione

**Fase E: ISOLATED INSTALLSNAPSHOT PHASE COMPLETED. M13: PARTIALLY COMPLETED.** C-01 confermato; C-02/C-03 complessivamente CONDITIONALLY READY; T-015/T-016/T-026 completi APERTI. M14 non iniziata, nessun commit Git.

InstallSnapshot runtime richiede ancora un incarico esplicitamente autorizzato per integrare un owner service/RPC reale: serializzazione snapshot/suffisso per peer, selezione leader/step-down reali e retry/deadline, barriere apply/I/O/lifecycle, epoch completamenti, token nuova generazione, invalidazione waiter, quote, stop in caso di errore/esito incerto ed evidenze reali a tre nodi con risposta persa/cambio termine/crash. Il modello isolato non si registra come handler di produzione.

La compaction runtime richiede separatamente cattura reale A=W=L=S seguita da barriera I/O finale distinta, conservando hard state aggiornato, C/N e tutte le entry di suffisso arrivate dopo, senza resettare mappe live già applicate. GC/eliminazione prefissi sicuri richiedono autorità CURRENT verificata, sorgente di recovery follower installabile conservata, esclusione generazione attiva, cutpoint completi eliminazione/fsync, contabilità spazio e gestione shutdown/I/O incerto. Queste modifiche pericolose sono rimandate e non autorizzate dall'incarico. Arresto al completamento della Fase E.
