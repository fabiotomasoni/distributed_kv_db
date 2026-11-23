# Modello e scenari di guasto

[Indice](README.md) · [English](../en/failure-model.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Modello

Si modellano crash-stop e crash-recovery non bizantini. Rete asincrona per safety, parzialmente sincrona per liveness. Byzantine fault, menzogne su Sync, attaccanti peer, RAM corrotta e perdita completa dell'SSD condiviso sono esclusi. Corruzione rilevabile porta a indisponibilità prudenziale, non recupero garantito.

Nella tabella, “GET/write” significa nuove operazioni client linearizzabili. Un retry già completato non autorizza un follower a servire dati da cache. Ogni scenario usa inizialmente tre nodi e dati committed, salvo stato iniziale diverso indicato. Gli identificatori delle invarianti sono definiti in [Raft](raft-consensus.md).

| Scenario / stato iniziale e guasto | Reazione attesa | Disponibilità | Invarianti | Recupero | Test |
|---|---|---|---|---|---|
| F-01 — Cluster sano; crash follower | Leader continua con altro peer. | GET/write sulla maggioranza. | INV-02/04/06 | Restart e catch-up log o snapshot. | T-024 |
| F-02 — Cluster sano; crash leader | Timeout, elezione su due nodi; pendenti incerti. | Pausa poi servizio maggioranza. | INV-01/03 | Vecchio leader torna follower. | T-004/T-024 |
| F-03 — Cluster sano; due crash | Nessun quorum; nessun nuovo commit. | Nessun nuovo GET/write; status locale. | INV-03/04 | Riavviare almeno due directory intatte. | T-019/T-023 |
| F-04 — Replica attiva; rete lenta | Timeout e retry correlati; possibili elezioni. | Progresso solo con consegna tempestiva. | INV-01/05 | Rimuovere ritardo; rieleggere se necessario. | T-020 |
| F-05 — RPC in volo; perdita messaggi | Ritrasmissione non cambia effetto logico. | Ridotta; nessuna garanzia senza consegna. | INV-02/06 | Ripristinare rete e catch-up. | T-003/T-020 |
| F-06 — RPC in volo; duplicazione | Voti, append e chunk idempotenti. | Normale se maggioranza comunica. | INV-01/02 | Ignorare duplicati tramite correlazione e indici. | T-006/T-015 |
| F-07 — Cambio leader; risposta riordinata | Ignorare progresso obsoleto, osservare termine maggiore. | Normale sulla nuova maggioranza. | INV-05 | Nuovo tentativo con generazione attuale. | T-006 |
| F-08 — Tre nodi; partizione 1 contro 2 | Lato 2 elegge/continua; isolato non completa nuove barriere iniziate dopo isolamento. | Solo maggioranza. | INV-01/03/04 | Heal e sostituzione suffisso non committed. | T-019/T-021 |
| F-09 — Nodo fermo; riavvio | Recovery follower; nessuna lettura locale client. | Altri due disponibili. | INV-04/06 | Replay committed poi sync da leader. | T-014 |
| F-10 — Leader isolato; torna dopo elezione | Step-down sul termine maggiore; no overwrite committed. | Maggioranza nuova. | INV-02/03 | Riconciliare log. | T-006/T-019 |
| F-11 — Persistenza attiva; write/Sync fallisce | Nodo fatal-storage; nessun ack positivo. | Solo se altri due operativi. | INV-06 | Riparare causa e riavviare con recovery. | T-013 |
| F-12 — WAL vicino cap; ENOSPC | Quota fisica locale; fermarsi senza cancellare committed. | Maggioranza se due directory sane e spazio host disponibile. | INV-04/06 | Liberare solo artefatti non attivi verificati. | T-026 |
| F-13 — Restart; CRC errato | Fail-closed; directory in quarantena. | Due peer sani se disponibili. | INV-04/06 | Ripristino manuale verificato, mai salta-record. | T-013/T-016 |
| F-14 — Richiesta ammessa; client si disconnette | Rimuovere waiter, non annullare comando. | Cluster continua. | INV-04 | Retry stessa identità e payload. | T-012 |
| F-15 — Commit/apply; risposta persa | Esito client incerto, risultato sessione conservato. | Cluster disponibile. | INV-04/06 | Retry recupera ultimo risultato senza nuovo effetto. | T-011/T-012 |
| F-16 — Tre processi stesso Mac; guasto host | Tutti indisponibili; nessuna indipendenza fisica. | Nessuna. | INV-06 (condizionata) | Recovery solo con dati persistenti intatti; power loss non verificato. | T-023 |

## Distinguere sicurezza e progresso

Un nodo può credersi leader durante una partizione, ma non può confermare nuove operazioni senza quorum. Non sono ammesse due leadership dello stesso termine; leader di termini diversi possono temporaneamente coesistere come convinzione locale. Testare il primo fatto dalle tracce complete, il secondo mediante barriere fallite sul lato isolato. Nessuna deadline autorizza ad applicare una scrittura senza commit.

## Evidenze implementate di guasto storage locale

[M03](milestone-m03.md) verifica write parziali, errori Sync/lettura/close/ispezione, frame finali interrotti, corruzione completa, pubblicazione bootstrap fallita e terminazione processo attorno a write/Sync. Esiti append ignoti restano ignoti; byte completi sopravvissuti recuperabili senza provare acknowledgment precedente. Nessun recovery locale applica entry speculative o usa fallback su corruzione. Osservazioni crash processo/OS vivo, non test crash OS o power loss; scenari distribuiti completi aperti.

## Guasti elettorali M05 implementati

Le [evidenze M05](milestone-m05.md) esercitano split vote, perdita/ritardo/duplicati/riordino messaggi, partizione/heal con vecchio leader isolato, crash modello dopo Sync voto/prima risposta, riavvio e rifiuto secondo voto incompatibile. Errori storage fermano il voto, inclusi quelli scoperti durante shutdown; worker/LOCK non vengono abbandonati per nascondere attesa fallita. Termini maggiori da risposte vecchie valide vengono ancora persistiti. Progresso heartbeat sano ed Election Safety stesso termine sono osservazioni finite. Scenari generali GET/write/replica sopra restano successivi; nessuna garanzia guasti OS/alimentazione o disponibilità fisica aggiunta.

## Evidenze guasti replica M06

I [test M06](milestone-m06.md) coprono perdita/ritardo/duplicazione/riordino, code divergenti, riavvio dopo Sync suffisso prima dell’acknowledgment e tagli frame WAL reali. Oracolo verifica prefissi disco durevoli, non soli messaggi inviati. Progresso vecchia generazione ignorato, termini maggiori validi persistiti. Errore Sync/storage incerto fatale senza ack replica; rifiuto quota proposta noto prima della scrittura ritentabile. Frame completi sopravvissuti non confermati recuperabili, senza inferire commit o applicazione. Guasti OS/alimentazione fuori evidenze.

## Evidenze guasti commit/apply M07

[M07](milestone-m07.md) testa vecchio leader isolato senza nuovo commit quorum, elezione maggioranza con prefisso ghost conservato, errori Sync checkpoint, apply bloccato con commit nuovi, consegna cancellata, panic applicazione e riavvio su istanza nuova. CAS negativa di dominio avanza LastApplied; errore infrastruttura non autorizza retry su mappa viva incerta. Checkpoint completo non confermato può recuperare; ricostruito solo prefisso coperto. Stop normale drena lavoro committed noto, stop fatale rifiuta admission prima di attendere effetti accettati. Nessuna garanzia exactly-once esterna/guasti OS/alimentazione.

## Incertezza HTTP M08

[M08](milestone-m08.md) distingue esclusione provata prima di registrazione da possibile append. Timeout/disconnessione dopo tale confine non cancella replica o apply committed né prova fallimento. Perdita leadership osservata prima dell’autorizzazione completamento è unknown. Leader isolati non confermano nuove scritture senza quorum. Risposta può perdersi dopo apply; test client a singolo tentativo segnala unknown senza retry. DELETE ripetuta è nuovo effetto/risultato fino a M11; sessioni successive restano necessarie. Shutdown HTTP cancella waiter limitati prima del drain committed esistente e rilascio lock. Nessuna nuova garanzia crash processo, OS o perdita alimentazione.

## Confine crash processo M09

[M09](milestone-m09.md) esercita kill brusco follower, scritture maggioritarie continuate, restart/catch-up follower, kill brusco leader/nuova elezione, poi kill di tutti e tre e restart directory integre. SIGTERM separato interrompe scrittura pendente dopo crescita WAL senza quorum: risposta unknown, stato committed sicuro ricostruito e chiave speculativa assente prima di successivo commit legale. Startup rifiuta ownership duplicata, bootstrap ripetuto e corruzione completa senza reset o fallback. Sono crash processi non bizantini con OS/filesystem attivi, non macchine indipendenti, crash OS o perdita alimentazione. Mancanza maggioranza durevole o perdita SSD condiviso invalida assunzioni conservazione. Nessun retry automatico richiesta unknown.

## Evidenze lettore obsoleto e risultato ritardato M10

[M10](milestone-m10.md) testa vecchio leader completamente isolato dopo scarto traffico pendente, nuova scrittura/lettura maggioranza e nuova read vecchio leader che mai committa/applica/completa. Riconnessione demote e sostituisce ricevuta. Risultato separato già committed prima di isolamento può ancora completare; quorum passato legale non revocato. Test apply bloccato ritardano read mentre scrittura successiva committa, provando acquisizione risultato nello slot esatto; cancellazione/perdita termine maggiore dà unknown mentre apply committed finisce comunque. Errori storage proposta/checkpoint non espongono valori. GET minoranza HTTP reale scade unknown, GET follower è not_admitted e letture dopo crash usano nuove barriere quorum. Nessun fallback locale, lease, retry mutazioni sicuro o indipendenza host fisica.

## Risposta persa, duplicati e profilo incompatibile M11

[M11](milestone-m11.md) perde risposta CAS riuscita reale solo dopo autorizzazione handler produzione, poi cambia leader e ritenta stessa identità logica. Nuovo leader restituisce risultato/indice originale senza altro effetto. Verificati duplicati concorrenti, riuso incompatibile, CAS fallito conservato dopo cambio stato, cancellazione durante apply committed e shutdown/replay. Leader completamente isolato non risponde nemmeno retry localmente conservato senza nuovo quorum. Oracoli a seed partizione/ripristino/replay conservano storia committed; processi figli reali esercitano kill follower, leader e tutti processi. Profili incompatibili falliscono prima di replay o ammissione HTTP; assenza conferma risposta v2 è unknown, non prova esclusione. Nessuna cache risolve richiesta senza percorso committed disponibile, nessuna eviction sessioni finge retention e limiti OS/alimentazione/perdita host invariati.

## Evidenze fault M12

[M12](milestone-m12.md) aggiunge diagnostica locale e un simulatore esplicito separato dal server. Osservazioni e log non autorizzano commit, apply o risposte client e non cambiano gli algoritmi. Sessioni v2, risultato GET nello slot, CAS atomico e retry con nuovo quorum rimangono invariati; nessuna migrazione v1. Il simulatore usa core/M02/codec WAL reali, ghost committed permanente e confronto risultati per indice; replay confronta ogni evento con scelte salvate. ProcessCrash conserva anche scritture non sincronizzate completate; SystemCrashModel esplora sopravvivenza all/synced solo in memoria. Trace incomplete sono inconclusive. Nessun crash OS/alimentazione reale, snapshot o compattazione; gate M13/M14 conservati. Specifiche operative nei [fault](fault-injection.md) e nella [diagnostica](observability.md).

## Fault snapshot pianificati, non eseguiti

La [matrice pre-M13](pre-m13-review.md) assegna a T-015/T-016/T-026 errori cattura, header/payload/Sync, digest/versioni errati, rename generazione/CURRENT, fsync directory, switch RAM/writer, cleanup, epoche obsolete, voto aggiornato, follower arretrato e restart con dedup. Tutte le righe snapshot sono NOT RUN. ProcessCrash e SystemCrashModel restano distinti; nessun crash OS/alimentazione reale. Se CURRENT rename è riuscito ma la barriera fallisce, conservare entrambe le generazioni, nessun ack/rollback; corruzione dell'autorità è errore esplicito. M12 resta un modello finito, non prova di linearizzabilità generale.

## Errori isolati M13 Fase A

[Fase A](milestone-m13-preliminary.md) verifica frame snapshot troncati/corrotti, lunghezze/count/versioni invalide, copie senza alias, metadati incoerenti, freeze prima del completamento, cancellazione/chiusura durante copy e panic del controllo. Nessun effetto di scrittura, eliminazione o installazione. Il worker non interrompe Apply/callback già accettati per far sembrare concluso uno shutdown; attesa scaduta non trasferisce ownership. Le chiamate fidate devono terminare come Machine.Apply. Le prove reali di crash fra generazione/CURRENT/cleanup e token/epoche runtime sono ancora T-015/T-016 futuri. Nessuna garanzia nuova OS/alimentazione.

## M13 Fase B — 23 novembre 2026

Le [prove Fase B](milestone-m13-persistence.md) distinguono write parziali/errori, gen-N pronta e selezione CURRENT, errori incerti dopo tentativo rename e uscita processo con OS vivo. Un errore non produce successo; owner failed, candidati e generazioni precedenti conservati; recovery non usa fallback. Solo coda WAL finale strutturalmente incompleta riparabile dopo validazione, corruzione completa fatale. Cleanup/switch/installazione e crash OS/alimentazione non verificati. T-015/T-016/T-026 restano aperti.

## M13 Fase C — 23 novembre 2026

La [Fase C](milestone-m13-runtime-recovery.md) è RUNTIME RECOVERY PHASE COMPLETED: RecoverCommitted ripristina S e applica soltanto S+1..C con M02, poi drena/chiude il worker isolato prima del nuovo applier a C. WAL e snapshot sono validati sotto LOCK continuo. L'avvio è esplicito via node.StartAPIWithSnapshotRecovery per v2; StartAPI/CLI ordinari mantengono il guard base zero. Nessun fallback con CURRENT/snapshot corrotti. KV/sessioni, hard state e A=L=C verificati con crash/restart follower/leader/tutti processi. C-01/OQ-05 confermato; sottoinsieme recovery C-02/OQ-01 verificato, C-02/C-03 complessivi CONDITIONALLY READY. T-015/AC-015/FR-015, T-016/AC-016/FR-016 e T-026/AC-026/NFR-006 completi aperti: compaction, cleanup e InstallSnapshot non implementati. M13 PARTIALLY COMPLETED, M14 non iniziata.
