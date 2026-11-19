# Roadmap di implementazione

[Indice](README.md) · [English](../en/implementation-roadmap.md)

**Aggiornamento: 19 novembre 2026 — M01/M02 locali implementate e verificate; le specifiche distribuite restano progettuali.**

## Capacità e dipendenze

Piano indicativo 24 settimane (~6 mesi), 6–8 ore/settimana per uno studente individuale, capacità disponibile di 144–192 ore, che non è una stima validata dello sforzo necessario; è una stima incerta, non un impegno. Quattro mesi richiedono più ore o riduzione degli obiettivi opzionali, mai eliminazione dei gate di correttezza. Recuperare ritardi prima tagliando ottimizzazioni, poi estendendo calendario. Nessuna data di completamento è garantita.

M01 è **COMPLETED** dal 19 novembre 2026, dopo autorizzazione esplicita: [relazione ed evidenze](milestone-m01.md). M02 è **COMPLETED**: [relazione ed evidenze](milestone-m02.md). M03–M16 sono **Not started**. Nella tabella attività progettuali significano rivedere il relativo contratto prima di scrivere il modulo; deliverable significa artefatto futuro.

| ID / obiettivo | Prerequisiti | Settimane | Progettazione, implementazione e deliverable | Test / completamento | Rischio | Complessità |
|---|---|---|---|---|---|---|
| M01 — Macchina a stati KV (**COMPLETED**) | — | 1–2 | Mappa, semantica e fixture; tabella operazioni/quote. | T-008/T-009 nel sottoinsieme di fase sotto definito; evidenze salvate. | Semantica ambigua | Bassa |
| M02 — Ownership e concorrenza (**COMPLETED**) | M01 | 3 | Eventi, applier e backpressure; contratto di ownership. | T-008/T-018 nel sottoinsieme di fase sotto definito; evidenze salvate. | Data race | Media |
| M03 — Persistenza locale | M01 | 4–5 | Frame, hard state e lock; specifica binaria e cutpoint. | T-013 nel sottoinsieme di fase sotto definito; evidenze salvate. | Sync macOS | Alta |
| M04 — RPC e configurazione | M02,M03 | 6 | Envelope e validazione; fixture configurazione canonica. | T-001/T-002/T-003/T-029 nel sottoinsieme di fase sotto definito; evidenze salvate. | Parsing e identità | Media |
| M05 — Leader election | M03,M04 | 7–8 | Timer e voto persistente; simulatore elettorale. | T-004/T-005 nel sottoinsieme di fase sotto definito; evidenze salvate. | Timer resettati male | Alta |
| M06 — Replica log | M05 | 9 | AppendEntries e conflitti; tracce log divergenti. | T-006 nel sottoinsieme di fase sotto definito; evidenze salvate. | Troncamento errato | Alta |
| M07 — Commit distribuito | M06 | 10 | Regola termine corrente e apply; trace commit. | T-007/T-008 nel sottoinsieme di fase sotto definito; evidenze salvate. | Commit vecchi termini | Alta |
| M08 — API e client di test | M07 | 11 | PUT/DELETE, deadline, derive OpenAPI; fixture HTTP. | T-009/T-012 nel sottoinsieme di fase sotto definito; evidenze salvate. | Contratto instabile | Media |
| M09 — Crash recovery / MVP interno | M08,M03 | 12–13 | Replay committed e restart; report MVP limitato. | T-014/T-018/T-023 nel sottoinsieme di fase sotto definito; evidenze salvate. | Replay speculativo | Alta |
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
