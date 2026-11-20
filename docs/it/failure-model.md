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
