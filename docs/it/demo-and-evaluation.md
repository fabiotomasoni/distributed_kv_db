# Demo tecnica e valutazione

[Indice](README.md) · [English](../en/demo-and-evaluation.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Preparazione comune

Demo futura, eseguibile solo dopo M14 e revisione dei risultati. Nessun esperimento è stato eseguito. Hardware e cap in [risorse](performance-and-resources.md); 3 nodi loopback secondo [rete](network-protocol.md), massimo 2 client e 100 operazioni per esperimento, 30 s massimo ciascuno. Durata complessiva sessione <=10 minuti incluse spiegazioni, senza carico continuativo. Dati esclusivamente sintetici, connettività esterna disabilitata e dipendenze già disponibili.

Salvare manifest run con revisione, configurazione/hash, toolchain, seed, directory e PID registrati. Tutti gli esperimenti conservano log di ruolo/termine/commit/apply e storia request_id/client_id/sequence. Misurare latenza, errori/unknown, elezione ove presente, recovery dove pertinente, RSS/CPU e spazio picco. Non attribuire un valore misurato a una fase diversa.

| Esperimento | Preparazione/configurazione | Azioni | Risultato atteso e successo | Invarianti | Test |
|---|---|---|---|---|---|
| D-01 — Cluster a tre nodi | Directory vuote con manifest comune, seed 7. | Bootstrap tre follower; attendere elezione e noop committed. | Un leader nel termine; status di tre membri. | INV-01 | T-001/T-004 |
| D-02 — Scrittura e replica | D-01 completato, una sessione nuova. | PUT config/theme e GET; attendere apply uguale nei follower. | Valore atteso e stesso digest a stesso indice. | INV-02/04/06 | T-008/T-009 |
| D-03 — Crash follower | D-02 completato; identificare follower. | Fermare follower, PUT sulla maggioranza, poi riavviarlo e catch-up. | Scrittura riesce e follower recupera. | INV-03/04 | T-024 |
| D-04 — Crash leader | D-03 guarito; di nuovo tre nodi vivi. | Fermare leader; tentare richiesta e registrare timeout. | Nessun falso successo da vecchio leader; pending incerto. | INV-06 | T-012/T-024 |
| D-05 — Nuova elezione | D-04, due follower sani e raggiungibili. | Attendere nuovo leader e noop, scrivere, poi riavviare vecchio leader. | Termine cresce, scritture riprendono entro target 3 s dopo stabilizzazione. | INV-01/03 | T-004/T-024 |
| D-06 — Partizione di rete | Tre vivi dopo D-05, fault adapter attivo. | Isolare leader da due peer e scartare messaggi pendenti; poi invocare GET sul vecchio, PUT+GET sui due, infine heal. | Lato isolato non completa GET; maggioranza continua e log convergono. | INV-02/03/04 | T-019/T-021 |
| D-07 — CAS concorrenti | Chiave stock/pen=12, due client ID diversi. | CAS concorrenti expected=12 verso 11 e 10; retry identico del vincitore. | Uno solo cambia il valore; retry restituisce stesso risultato originale. | INV-04 | T-010/T-011 |
| D-08 — Restart e snapshot | Successi noti, snapshot pubblicato e sessioni presenti. | Crash tre processi, restart stesse directory; GET e retry ultima sequenza. | Successi conservati, nessuna doppia applicazione; distinguere replay e catch-up. | INV-04/06 | T-014/T-015/T-023 |
| D-09 — Storia linearizzabile | Checker validato con fixture legali e impossibili. | Registrare <=12 operazioni con CAS, GET, timeout e leader change; verificare. | Checker produce witness legale, non timeout/inconclusive. | INV-04 | T-022 |

## Log, metriche e cleanup per esperimento

D-01 conserva voti/termini e tempo primo noop; D-02 indici e hash replica, latenza e Sync; D-03/D-04/D-05 PID/exit, quorum e tempo ripresa; D-06 regole partizione e timeline heal, esiti per lato; D-07 confronto CAS e dedup_hits, numero cambiamenti; D-08 CURRENT/generazione e indici, replay/catch-up; D-09 storia integrale e witness, tempo checker.

Cleanup di ogni esperimento: rimuovere fault rule, riavviare solo nodi fermati dal run, attendere catch-up oppure dichiarare scenario fallito e fermare il run. D-03 deve riavviare follower prima di D-04, altrimenti non resterebbe quorum. D-06 elimina staging obsoleto dopo heal verificato; D-07 chiude le sessioni lato client senza espellere metadati replicati; D-08 mantiene directory per ispezione; D-09 chiude checker e salva esito. A fine sessione fermare tutti i PID del run, chiudere file, archiviare evidenze entro 10 MiB e cancellare solo dati sintetici esplicitamente scelti.

## Presentazione al colloquio

Aprire con obiettivo, stato effettivo e limiti di tre processi sullo stesso host. Mostrare una scrittura, un failover e un controesempio che il checker respinge, poi una storia valida. Discutere regola del termine corrente, voto prima di ack, read barrier, timeout incerto, dedup nello snapshot e pubblicazione generazioni. Presentare misure solo insieme al loro profilo sperimentale. Se una demo fallisce, usare log e seed per spiegare il problema; non sostituire il risultato con screenshot o numeri inventati.
