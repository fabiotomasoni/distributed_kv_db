# Roadmap di implementazione

[Indice](README.md) · [English](../en/implementation-roadmap.md)

**Aggiornamento: 22 novembre 2026 — M01/M02/M03/M04/M05/M06/M07/M08/M09 locali implementate e verificate; le specifiche distribuite restano progettuali.**

## Capacità e dipendenze

Piano indicativo 24 settimane (~6 mesi), 6–8 ore/settimana per uno studente individuale, capacità disponibile di 144–192 ore, che non è una stima validata dello sforzo necessario; è una stima incerta, non un impegno. Quattro mesi richiedono più ore o riduzione degli obiettivi opzionali, mai eliminazione dei gate di correttezza. Recuperare ritardi prima tagliando ottimizzazioni, poi estendendo calendario. Nessuna data di completamento è garantita.

M01 è **COMPLETED** dal 19 novembre 2026, dopo autorizzazione esplicita: [relazione ed evidenze](milestone-m01.md). M02 è **COMPLETED**: [relazione ed evidenze](milestone-m02.md). M03 è **COMPLETED** nel perimetro storage locale: [relazione WAL/recovery](milestone-m03.md). M04 è **COMPLETED**: [relazione RPC/configurazione](milestone-m04.md). M05 è **COMPLETED**: [relazione leader election](milestone-m05.md). M06 è **COMPLETED**: [relazione replica](milestone-m06.md). M07 è **COMPLETED**: [relazione commit/applicazione](milestone-m07.md). M08 è **COMPLETATA**: [sottoinsieme API ed evidenze](milestone-m08.md). M09 è **COMPLETATA**: [evidenze recovery processi/MVP](milestone-m09.md). M10–M16 sono **Non iniziate**. Nella tabella attività progettuali significano rivedere il relativo contratto prima di scrivere il modulo; deliverable significa artefatto futuro.

| ID / obiettivo | Prerequisiti | Settimane | Progettazione, implementazione e deliverable | Test / completamento | Rischio | Complessità |
|---|---|---|---|---|---|---|
| M01 — Macchina a stati KV (**COMPLETED**) | — | 1–2 | Mappa, semantica e fixture; tabella operazioni/quote. | T-008/T-009 nel sottoinsieme di fase sotto definito; evidenze salvate. | Semantica ambigua | Bassa |
| M02 — Ownership e concorrenza (**COMPLETED**) | M01 | 3 | Eventi, applier e backpressure; contratto di ownership. | T-008/T-018 nel sottoinsieme di fase sotto definito; evidenze salvate. | Data race | Media |
| M03 — Persistenza locale (**COMPLETED**) | M01 | 4–5 | Frame, hard state e lock; specifica binaria e cutpoint. | T-013 nel sottoinsieme di fase sotto definito; evidenze salvate. | Sync macOS | Alta |
| M04 — RPC e configurazione (**COMPLETED**) | M02,M03 | 6 | Envelope e validazione; fixture configurazione canonica. | T-001/T-002/T-003/T-029 nel sottoinsieme di fase sotto definito; evidenze salvate. | Parsing e identità | Media |
| M05 — Leader election (**COMPLETED**) | M03,M04 | 7–8 | Timer e voto persistente; simulatore elettorale. | T-004/T-005 nel sottoinsieme di fase sotto definito; evidenze salvate. | Timer resettati male | Alta |
| M06 — Replica log (**COMPLETED**) | M05 | 9 | AppendEntries e conflitti; tracce log divergenti. | T-006 nel sottoinsieme di fase sotto definito; evidenze salvate. | Troncamento errato | Alta |
| M07 — Commit distribuito (**COMPLETED**) | M06 | 10 | Regola termine corrente e apply; trace commit. | T-007/T-008 nel sottoinsieme di fase sotto definito; evidenze salvate. | Commit vecchi termini | Alta |
| M08 — API e client di test (**COMPLETATA**) | M07 | 11 | PUT/DELETE, deadline, derive OpenAPI; fixture HTTP. | T-009/T-012 nel sottoinsieme di fase sotto definito; evidenze salvate. | Contratto instabile | Media |
| M09 — Crash recovery / MVP interno (**COMPLETATA**) | M08,M03 | 12–13 | Replay committed e restart; report MVP limitato. | T-014/T-018/T-023 nel sottoinsieme di fase sotto definito; evidenze salvate. | Replay speculativo | Alta |
| M10 — GET linearizzabile | M09 | 14 | Barriera read nel log; storia stale-leader. | T-019 nel sottoinsieme di fase sotto definito; evidenze salvate. | Lettura fuori slot | Alta |
| M11 — CAS e deduplicazione | M10 | 15–16 | CAS e sessioni persistite; matrice retry. | T-010/T-011/T-012 nel sottoinsieme di fase sotto definito; evidenze salvate. | ABA e sessioni | Alta |
| M12 — Fault injection e osservabilità | M11 | 17–18 | Scenari e replay, log/metriche; manifest riproducibile. | T-017/T-020/T-028 nel sottoinsieme di fase sotto definito; evidenze salvate. | Non determinismo | Alta |
| M13 — Snapshot e compaction | M12 | 19–20 | Generazioni atomiche e install; fixture crash snapshot. | T-015/T-016/T-026 nel sottoinsieme di fase sotto definito; evidenze salvate. | Picco spazio e dedup persa | Alta |
| M14 — Verifica delle proprietà | M13 | 21–22 | Checker indipendente e suite safety; report AC. | T-021/T-022/T-024/T-030/T-031 nel sottoinsieme di fase sotto definito; evidenze salvate. | Checker incompleto | Alta |
| M15 — Misure limitate | M14 | 23 | Baseline latenza/risorse; report campioni e limiti. | T-025/T-026 nel sottoinsieme di fase sotto definito; evidenze salvate. | Ottimizzazione prematura | Media |
| M16 — Demo e consegna | M15 | 24 | Esperimenti, revisione bilingue e portfolio; pacchetto evidenze. | T-027/T-032 nel sottoinsieme di fase sotto definito; evidenze salvate. | Promesse oltre evidenze | Media |

## Gate

M09 è il primo MVP interno: tre nodi, PUT/DELETE, replica durevole e recovery; non pubblicizzare ancora GET linearizzabile, CAS, retry sicuri o storage compattabile. I test unitari possono già progettare queste semantiche senza esporle. M10–M11 chiudono il contratto client finale. M13 è necessario per la prima versione completa con storage limitato; M14 è il gate di correttezza, M15–M16 quello di valutazione.

Ogni milestone: (1) rivedere specifica e questioni aperte, (2) implementare solo dipendenze pronte, (3) eseguire test associati e regressioni rilevanti, (4) registrare evidenze e rischi residui, (5) aggiornare entrambe le lingue. T-027 vale a ogni milestone, T-021 cresce dai primi test core, non nasce soltanto alla fine. Non rinviare tutte le prove al mese sei.

Se M03 non valida sync/lock, blocca ogni promessa di durabilità; M05 non chiusa blocca replica; M07 non chiusa blocca API di successo; M13 non chiusa blocca claim di snapshot. Nessun benchmark precede il gate safety. Nuove feature avanzate richiedono nuova stima, non si inseriscono silenziosamente nel calendario.

## Sottoinsiemi di test e gate senza dipendenze circolari

| Milestone | Completamento consentito nel sottoinsieme | Criteri completi ancora aperti |
|---|---|---|
| M01 | T-008/T-009 su modello sequenziale locale e fixture, incluse quote e semantica CAS; nessuna rete. | AC-008 distribuito a M07; AC-009 API completa a M10. |
| M02 | Scheduling senza duplicati, coda satura, worker chiusi nel harness; clock/transport/storage fake definiti qui. | AC-018 shutdown server a M09; AC-028 replay completo a M12. |
| M08 | PUT/DELETE HTTP, classificazione unknown/not_admitted, OpenAPI del sottoinsieme. | AC-009 GET a M10; AC-012 dedup/failover a M11. |
| M09 | Recovery e durabilità mediante ispezione controllata del modello/applier; nessuna GET client necessaria al test. | AC-023 end-to-end con GET dopo M10; snapshot a M13. |
| M13 | Quote deterministiche, picco storage, installazione e compaction. | Parte RAM/CPU e report AC-026 a M15. |
| M14 | Suite safety/linearizzabilità/sicurezza e integrazione offline, anche prima della presentazione finale. | Report prestazioni a M15, demo presentabile e inventario finale a M16. |

Gli altri gate richiedono i relativi test completi quando tutte le dipendenze esistono. Un sottoinsieme passato non chiude l'intero AC né autorizza una feature non disponibile. M14 chiude gli AC di correttezza; M15–M16 quelli di misura e consegna. T-031 usa a M14 una procedura integration offline autonoma, senza dipendere dal completamento della demo M16. La matrice indica il gate finale del requisito; la roadmap può anticiparne fixture e sottoinsiemi.

M12 completa fault injection, non ne avvia le fondamenta: introdurre fake clock e scheduler a M02, cutpoint storage a M03 e oracoli elettorali a M05. Rivalutare il calendario dopo M03; 4–6 mesi è un orizzonte desiderato, con priorità al MVP interno. Snapshot e checker possono richiedere più tempo senza introdurre spese o ridurre correttezza.

## Readiness pre-M03 dopo revisione M02

[Revisione del 19 novembre 2026](pre-m03-review.md): CONDITIONALLY READY. Schema/fixture storage C-01 risolti; revisione fonti piattaforma C-02 completata, validazione implementativa ancora aperta. Un futuro incarico M03 può iniziare soltanto conformità formati e test isolati adattatore locale nel [piano finito di risorse](macos-storage-contract.md). Richiedere evidenze primitive/lock/errori/visibilità prima di integrare writer/percorso pubblicazione storage. Barriere fallite o non supportate bloccano quel percorso e richiedono revisione ADR; niente downgrade silenzioso. T-013 resta gate accettazione M03; T-023 durabilità M09 e test snapshot M13 aperti. In quella revisione pre-M03 M03 era Not started; questa revisione progettuale non autorizza benchmark, cluster o claim di successo durevole.

## Esito M03 preliminare

[Evidenze locali](milestone-m03-preliminary.md): GATE C-02 VERIFIED per le primitive Darwin/APFS provate; M03 PARTIALLY COMPLETED. Passano conformità senza stato frame/manifest/entry-digest e test isolati lock/barriere/pubblicazione. Nessun writer WAL/motore recovery completo implementato. Un nuovo incarico potrà autorizzare lavoro residuo WAL/hard-state/bootstrap limitato e T-013; milestone successive e accettazione durabilità completa restano bloccate. Fermarsi a questa fase.

## Esito M03 locale finale — 20 novembre 2026

[M03 COMPLETED](milestone-m03.md): WAL con frame, validazione persistente hard-state/log/commit, recovery streaming, bootstrap esplicito, CURRENT autorevole, quote/riserve ed evidenze errore/processo. C-01/C-02 conservati. Recovery espone soltanto stato log; non applica comandi né stabilisce consenso. Gate M09 server/durabilità e M13 snapshot/compaction aperti. Alla consegna M03 i prerequisiti storage M04 erano pronti per revisione, ma M04 era Not started e richiedeva nuovo incarico; precedenti stati parziali sono storici.

## Esito M04 locale finale — 20 novembre 2026

[M04 COMPLETED](milestone-m04.md): configurazione statica/hash immutabili e fixture, envelope/tre schemi RPC rigorosi, trasporto HTTP loopback limitato e lifecycle storage prima del listener. T-001/T-002/T-003/T-029 superati nel confine di fase M04. Corretta regressione integrità sequenza coda M03; fixture configurazione/protocollo C-01 soddisfatte, limiti locali C-02 conservati. Nessuna elezione Raft, replica o installazione snapshot. Alla consegna M04 M05 era NOT STARTED; il nuovo incarico ha rivisto e consumato queste interfacce.

## Esito M05 finale — 20 novembre 2026

[M05 COMPLETED](milestone-m05.md): elezioni follower/candidate/leader deterministiche, termine/voto sincronizzati, noop elettorale locale, heartbeat vuoti, integrazione servizio/nodo limitata. T-004: 20 seed salvati riprodotti due volte con split vote e crash dopo Sync voto; oracolo indipendente voto/quorum/leader superato. T-005: 100 round heartbeat sani senza elezioni spurie. Regressioni M01–M04 e race mirate superate; limiti C-01/C-02 conservati, gate successivi C-03 invariati. Nessuna replica/commit/apply o implementazione M06. M06 NOT STARTED.

## Gate preliminare storico perimetro M06

[Relazione preliminare](milestone-m06.md): regressione M05 e test mirati rilascio risorse superati. Alla consegna preliminare implementazione M06 restava NOT STARTED in attesa del chiarimento esplicitamente imposto dalla sezione 3 del nuovo incarico: M06 è replica/T-006, M07 è commit distribuito/apply/T-007/T-008. Nessuna riassegnazione roadmap o implementazione successiva. Il blocco riguarda perimetro, non un difetto correttezza M05 rilevato.

## Esito M06 finale — 20 novembre 2026

[M06 COMPLETED](milestone-m06.md): chiarimento perimetro RESOLVED confermando roadmap invariata. Replica entry AppendEntries, riconciliazione suffisso dal primo conflitto, effetti WAL sincronizzati e nextIndex/matchIndex correlati limitati superano FR-006/AC-006/T-006. Superati cinque seed replica riprodotti, tagli WAL, riavvio follower/loopback e regressioni/race precedenti. Checkpoint commit noti conservati e congelati. Commit/regola termine corrente/apply M07 NOT STARTED.

## Esito M07 finale — 20 novembre 2026

[M07 COMPLETED](milestone-m07.md): commit quorum del termine corrente con checkpoint M03 sincronizzato, propagazione follower limitata a prefisso verificato, worker M02 invariato più adattatore noop/mutazioni, cursori schedulati/completati distinti e replay prefisso committed su macchina nuova. T-007/T-008 e obbligo C-03 M07 backlog superati; evidenze controesempio maggioranza vecchio termine, apply bloccato 5→7, ghost committed/tre modelli, tagli checkpoint e failover/CAS reali. M08 NOT STARTED. M09 conserva recovery completo server/processi/MVP; questa fase implementa solo ricostruzione libreria su macchina nuova richiesta per applicazione committed.

## Esito finale M08 — 21 novembre 2026

[M08 COMPLETATA](milestone-m08.md): HTTP PUT/DELETE, ammissione limitata, deadline con not_admitted provato contro unknown, autorizzazione risultato applicato esatto/leadership, test client a singolo tentativo e OpenAPI 3.1/fixture HTTP verificate. Revisione/regressione M07 senza difetti bloccanti. Chiuso solo sottoinsieme ufficiale T-009/T-012; GET resta M10, sessioni/CAS M11. Sottoinsieme C-01 soddisfatto, gate successivi invariati. M09 NON INIZIATA; nessun MVP CLI/recovery processi, misura o commit Git.

## Esito finale M09 — 22 novembre 2026

[M09 COMPLETATA](milestone-m09.md): MVP eseguibile PUT/DELETE a tre processi, bootstrap/restart espliciti, lifecycle segnali/terminale, recovery reale crash follower/leader/tutti processi e ricostruzione solo committed. Superati FR-014/AC-014/T-014 e FR-018/AC-018/T-018, più sottoinsieme roadmap NFR-003/AC-023/T-023. Revisione M08 senza blocchi; formati canonici/HTTP invariati. Evidenza durabilità completa basata su GET resta dopo M10, gate snapshot M13. M10 NON INIZIATA; nessuna GET, CAS, deduplicazione o ReadIndex.
