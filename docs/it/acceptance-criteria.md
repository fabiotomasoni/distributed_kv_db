# Criteri oggettivi di accettazione

[Indice](README.md) · [English](../en/acceptance-criteria.md)

**Aggiornamento: 19 novembre 2026 — M01 locale implementata e verificata; le specifiche distribuite restano progettuali.**

## Regola di accettazione

I criteri completi rimangono aperti; i sottoinsiemi locali M01 di T-008/T-009 e M02 di T-008/T-018 sono **Passed**, come documentato sotto. Per chiudere una riga servono procedura ripetibile, evidenza associata a revisione/configurazione e condizione rispettata. “Nessun panic” o un log senza errori non è sufficiente. Verifica il [catalogo test](testing-strategy.md) per dettagli di scheduling e fixture.

| Criterio | Requisito / procedura | Evidenza obbligatoria | Condizione di superamento |
|---|---|---|---|
| AC-001 | FR-001 / T-001. Avviare tre config distinte e tentare seconda apertura di n1. | Manifest e exit code. | Tre directory e tre identità distinte; seconda istanza rifiutata. |
| AC-002 | FR-002 / T-002. Variare membri, cluster ID e hash in config e RPC. | Fixture valide/invalide e stato prima/dopo. | Manifest divergente o membro duplicato impedisce avvio. |
| AC-003 | FR-003 / T-003. Iniettare JSON invalido, envelope errato e corpo oltre cap. | Risposte e hash stato invariato. | Payload malformati respinti senza mutare stato. |
| AC-004 | FR-004 / T-004. Simulare split vote e crash subito dopo voto Sync; rieseguire per 20 seed. | Traccia voti/termini/leader e seed. | Nessun termine con due leader in traccia, inclusi crash dopo voto. |
| AC-005 | FR-005 / T-005. Avanzare clock virtuale per 100 heartbeat senza guasti. | Conteggio elezioni e timer. | Leader stabile non produce elezioni spurie nel simulatore senza guasti. |
| AC-006 | FR-006 / T-006. Costruire log divergenti, inviare heartbeat e risposte obsolete, poi heal. | Prefissi, matchIndex e transizioni. | Log divergenti convergono senza sovrascrivere entry committed. |
| AC-007 | FR-007 / T-007. Riprodurre caso entry vecchia su maggioranza, poi noop corrente. | Commit invariato prima e avanzato dopo. | Entry vecchia non committed soltanto contando repliche. |
| AC-008 | FR-008 / T-008. Applicare stessa sequenza su tre modelli, ritardando un applier. | Digest canonico e risultati per indice. | Stessa sequenza produce stesso stato e risultato su tre nodi. |
| AC-009 | FR-009 / T-009. Enumerare chiave assente, valore vuoto, input invalido e DELETE ripetuto. | Tabella fixture API e modello. | Assenza, valore vuoto e DELETE ripetuto rispettano la tabella API. |
| AC-010 | FR-010 / T-010. Lanciare due CAS da sessioni diverse sullo stesso valore, new diversi. | Storia con un solo cambiamento riuscito. | Due CAS concorrenti stesso expected: al massimo uno cambia il valore. |
| AC-011 | FR-011 / T-011. Duplicare ultima sequenza, variare payload, usare sequenza vecchia e gap. | Risultati e tabella sessioni dopo replay. | Retry identico non applica nuovamente; cambio payload è errore. |
| AC-012 | FR-012 / T-012. Scartare risposta dopo apply, cambiare leader, ritentare identico. | Una sola mutazione e risultato originale. | Risposta persa dopo commit risolta da retry con stesso identificatore. |
| AC-013 | FR-013 / T-013. Tagliare frame a ogni confine; simulare short write/Sync error e CRC errato. | Immagini disco e decisione recovery. | Cutpoint prima/dopo Sync preservano voto e prefisso confermato. |
| AC-014 | FR-014 / T-014. Riavviare tutti con suffisso non committed e checkpoint commit inferiore. | Stato ricostruito prima e dopo nuovo noop. | Crash e restart non espongono suffisso non committed. |
| AC-015 | FR-015 / T-015. Installare snapshot a follower molto arretrato con retry chunk e cambio termine. | Hash immagine, sessioni e indici finali. | InstallSnapshot ripristina lo stesso stato e dedup. |
| AC-016 | FR-016 / T-016. Arrestare a ogni cutpoint di pubblicazione/cleanup generazione. | CURRENT e generazione recuperata per caso. | Crash a ogni fase lascia almeno una generazione recuperabile. |
| AC-017 | FR-017 / T-017. Eseguire elezione e timeout, ispezionare status e log senza valori utente. | Campione log redatto e snapshot metriche. | Status e log correlano elezione e richiesta senza valori. |
| AC-018 | FR-018 / T-018. Inviare shutdown durante RPC e persistenza; controllare worker e lock. | Durata, goroutine e recovery successivo. | Arresto entro 5 s o terminazione segnalata con recovery successivo. |
| AC-019 | FR-019 / T-019. Stabilire isolamento completo e scartare messaggi pendenti; poi scrivere sulla maggioranza e invocare nuova GET sul vecchio leader. | Storia che esclude lettura stale di successo. | Leader isolato non restituisce lettura locale come successo. |
| AC-020 | FR-020 / T-020. Riprodurre due volte scenario con stesso seed e clock virtuale. | Tracce eventi identiche byte per byte. | Due replay simulati hanno stessa sequenza di transizioni. |
| AC-021 | NFR-001 / T-021. Enumerare piccoli schedule con 3 nodi, 3 proposte e fino a 2 guasti. | Report invarianti e bound esplorato. | Zero violazioni in suite deterministica finita; nessuna pretesa di prova esaustiva. |
| AC-022 | NFR-002 / T-022. Verificare 20 storie fino a 12 operazioni con CAS, retry e failover. | Ordine testimone o controesempio ridotto. | Checker trova ordinamento legale per tutte le storie piccole previste. |
| AC-023 | NFR-003 / T-023. Confermare dati, crash di processo su uno e poi tutti, recuperare directory intatte. | Lista successi confrontata con GET successivi. | Nessun successo perso nei test crash di processo del modello. |
| AC-024 | NFR-004 / T-024. Fermare follower, riavviarlo e attendere catch-up; poi fermare leader lasciando due nodi sani. | Timeline elezione e ripresa entro target locale. | Dopo stabilizzazione, con storage/CPU disponibili e quote non sature, ripresa entro il target locale di 3 s. |
| AC-025 | NFR-005 / T-025. Eseguire benchmark limitato secondo profilo risorse. | Campioni grezzi e metodologia. | Report contiene campioni, errori, hardware e intervallo di misura. |
| AC-026 | NFR-006 / T-026. Raggiungere quota dataset, code e disco simulato senza carico massivo. | Errori, RSS, CPU e spazio massimo. | Quota produce errore deterministico; profilo risorse e spazio di picco registrati. |
| AC-027 | NFR-007 / T-027. Confrontare alberi linguistici, ID, blocchi tecnici e link; revisione semantica. | Rapporto revisione con limiti dichiarati. | Parità di file, identificatori e link verificata a ogni revisione. |
| AC-028 | NFR-008 / T-028. Iniettare fake clock, PRNG, transport e storage nello stesso core. | Replay regressione senza rete reale. | Una regressione si riproduce da manifest e seed salvati. |
| AC-029 | NFR-009 / T-029. Registrare go version/GOARCH e compilare sul Mac ARM64. | Output build e suite locale. | Build e test locali documentano toolchain e architettura. |
| AC-030 | NFR-010 / T-030. Provare bind 0.0.0.0, input gigante e permessi larghi. | Rifiuti e permessi effettivi. | Bind pubblico rifiutato, file privati, payload eccessivi respinti. |
| AC-031 | NFR-011 / T-031. Disabilitare Internet mantenendo loopback ed eseguire integrazione offline autonoma; demo presentata a M16. | Manifest dipendenze e traccia offline. | Con rete esterna disabilitata l’integrazione offline delle operazioni previste riesce; la presentazione finale è separata. |
| AC-032 | NFR-012 / T-032. Revisionare dipendenze e procedura su macchina già posseduta. | Inventario licenze, costi e prerequisiti. | Inventario dipendenze e procedura non richiedono pagamenti né account obbligatori. |

## Gate e deroghe

M09 chiude soltanto il sottoinsieme MVP della roadmap; M14 tutte le condizioni di correttezza; M15 il report risorse e performance; M16 demo, budget e consegna. Un test inconclusive resta aperto. Ogni deroga registra requisito, motivo e rischio in entrambe le lingue; non autorizza a pubblicizzare una garanzia non verificata. Nessuna deroga può introdurre spese o implementazione in questa fase.

## Sottocasi obbligatori aggiunti dall’audit

Gli AC restano Not run. AC-003/AC-009 richiedono parsing rigoroso e CAS massimo entro corpo 48 KiB; AC-008 include backlog di commit e coda piena senza doppio apply; AC-013/AC-016 includono write non Sync sopravvissute e hard state aggiornato nel passaggio generazione; AC-015 include applyEpoch e snapshot divenuto obsoleto durante il trasferimento; AC-019 distingue nuova GET dopo isolamento da risposta di read già committed; AC-022 richiede il controesempio con retry dopo completamento; AC-026 include prenotazione dei file di controllo.

I sottoinsiemi iniziali della [roadmap](implementation-roadmap.md) non chiudono questi criteri interi. AC-031 si verifica con integration offline autonoma a M14; la presentazione della demo a M16 non è un prerequisito circolare. Tutti i sottocasi devono comparire nelle evidenze del test primario, non sono nuovi requisiti non tracciati.

## Chiusura limitata di M01

M01 è COMPLETED nel solo sottoinsieme ufficiale: semantica locale, CAS sequenziale, validazione, quote, ownership dei byte e determinismo verificati con test reali. [Relazione M01](milestone-m01.md). Non si chiudono AC-008 distribuito, AC-009 HTTP/GET Raft, AC-010 concorrente, AC-011 dedup né AC-026 risorse cluster. Questo registro M01 non chiude criteri completi successivi; seguono le evidenze locali M02.

## Evidenze revisione M02 e gate pre-M03

Sottoinsiemi locali T-008/T-018 M02 Passed, riesaminati dal sorgente con 21 test e race mirato; AC completi server/distribuiti aperti. [Relazione pre-M03](pre-m03-review.md): progetto C-01 risolto, validazione implementativa C-02 aperta, CONDITIONALLY READY soltanto per test iniziali conformità/adattatore. AC-013 richiede che il futuro codec consumi [fixture statiche](canonical-formats.md) positive/negative, oltre a test write parziali/Sync/errori/cutpoint. AC-016 richiede test pubblicazione/handoff alla propria fase; AC-023 ancora evidenze reali processo/riavvio. Nessuno è chiuso da questa documentazione.

## Evidenze successive M03 preliminare

[Relazione di fase](milestone-m03-preliminary.md): GATE C-02 VERIFIED per primitive locali, M03 PARTIALLY COMPLETED. Passano 7 test storageformat/18 sottocasi e 13 funzioni test storagefs/16 sottocasi; superati race mirato e regressioni M01/M02 invariate. Ambito codec: frame/entry/digest/manifest senza stato; nessun motore WAL/replay o chiusura completa AC-013/AC-014/AC-023. Gate snapshot e successivi aperti. Cifre/stati precedenti descrivono la fase storica pertinente.

## Chiusura M03 locale

[Evidenze finali](milestone-m03.md): M03 COMPLETED; T-013/parte storage AC-013 locali Passed. WAL con 17 funzioni principali/25 sottocasi, 82,9% statement coverage locale; passano regressioni M01/M02, codec/adattatore e race mirato WAL/adattatore/applier. Tutte le 18 aspettative recovery canoniche coperte al confine byte/namespace pertinente. Integrazione risposte Raft FR-013 completa, restart server AC-014 e AC-023 end-to-end dipendono da milestone successive; accettazione snapshot AC-015/AC-016 a M13. Copertura/race/test processo non certificano resilienza a guasti fisici. Nessun ID requisito/AC primario o gate distribuito finale rimosso.

## Evidenze accettazione M04

[M04 COMPLETED](milestone-m04.md): AC-001/AC-002/AC-003/AC-029 superati per FR-001/FR-002/FR-003/NFR-009 nel sottoinsieme ufficiale di fase. Evidenze: tre avvii libreria isolati, errore lock/manifest prima del bind, hash configurazione/fixture protocollo esatti, messaggi malformati/eccessivi senza consegna processor, scambio locale limitato e build/test ARM64. UX exit code CLI runtime e successivi criteri consenso/client/durabilità non dichiarati. Corretta regressione corruzione coda M03 e superati test componenti precedenti.

## Evidenze accettazione M05

[M05 COMPLETED](milestone-m05.md): AC-004/T-004/FR-004 superati con tutte le 20 storie seed salvate e fingerprint tracce riprodotti due volte, split vote forzato e crash modello subito dopo Sync voto; nessun voto durevole o leader per termine duplicato. AC-005/T-005/FR-005 superati con 100 round virtuali sani, 200 heartbeat peer e leader/termine invariati. Integrazione loopback/WAL reali, riapertura termine/voto, errore/shutdown e race mirate superati. Non chiude AC-006/AC-007/AC-008 o criteri safety/client/durabilità distribuiti.

## Esito accettazione M06

[M06 COMPLETED](milestone-m06.md): FR-006/AC-006/T-006 superati con log ordinari/in ritardo/divergenti, prefisso committed noto immutabile, nextIndex/matchIndex correlati, retry/risposte vecchie, errori storage e riavvio. Cinque tracce replica con seed riprodotte due volte contro oracolo Log Matching/append-only; riavvio follower loopback reale recupera. Checkpoint resta fisso; AC-007/AC-008 e regola commit quorum termine corrente restano M07. Tutti i test/fixture precedenti conservati.

## Evidenze accettazione M07

[M07 COMPLETED](milestone-m07.md): FR-007/AC-007/T-007 superano maggioranza vecchio termine/senza-noop-corrente contro commit indiretto corrente legale, assenza quorum/risposta obsoleta ed errore/recovery checkpoint. FR-008/AC-008/T-008 superano stato/risultati per indice identici tre modelli, backlog bloccato 5→7 capacità 1, nessun apply speculativo/duplicato e worker M02 invariato. Obbligo C-03 M07 backlog/ghost soddisfatto; gate handoff snapshot/sessioni/read/storie alle rispettive fasi. Nessuna accettazione client pubblico/durabilità completa inferita.

## Evidenze di fase M08

[Relazione M08](milestone-m08.md): FR-009/FR-012, AC-009/AC-012 e T-009/T-012 superati solo per sottoinsieme roadmap PUT/DELETE, validazione HTTP rigorosa, deadline, unknown/not_admitted e OpenAPI. Test includono cancellazione con disco/apply bloccati, perdita osservata a termine maggiore, ammissione limitata con capacità peer separata, tre nodi reali con scritture concorrenti/elezione/minoranza, risposta persa senza retry automatico e DELETE ripetuta con nuovo risultato. Checker schema/fixture HTTP include controlli negativi ed estensioni canoniche runtime. Regressioni M01–M07 e race mirato superati. GET AC-009 completa resta M10; retry dedup/failover AC-012 resta M11, con snapshot/checker storie ai gate successivi. Obbligo artefatto/fixture sottoinsieme M08 C-01 soddisfatto; nessuna stabilità v1 completa dichiarata. C-02 e obblighi successivi C-03 mantengono limiti originali.

## Evidenze MVP interno M09

[M09](milestone-m09.md) soddisfa FR-014/AC-014/T-014 e FR-018/AC-018/T-018, più sottoinsieme roadmap NFR-003/AC-023/T-023. Nuovo test startup controllato verifica replay solo follower a commit 1 con suffisso 2, poi applicazione legale noop nuovo termine. Test reali processi figli uccidono un follower, leader, tutti, riavviano storage integro, confrontano byte confermati/valori realmente ricostruiti e riprendono PUT/DELETE. SIGTERM scrittura pendente verifica unknown, esclusione suffisso e rilascio risorse. Test CLI/errori coprono input limitato/redatto, inizializzazione mancante, lock duplicato, bootstrap ripetuto, corruzione, cancellazione prima di avvio e coerenza configurazioni esempio. Rieseguite regressioni M01–M08 e race. Test usano un cluster temporaneo a tre nodi per volta, attese limitate e dati piccoli; nessun benchmark o guasto fisico. Variante GET HTTP AC-023 resta M10, snapshot M13, checker completo M14. OpenAPI/fixture M08 e gate successivi C-01/C-02/C-03 invariati.

## Evidenze gate letture M10

[M10](milestone-m10.md): superati FR-019/AC-019/T-019 e porzione GET FR-009/AC-009/T-009. T-019 esegue seed 42/117/203 due volte con ordinamento eventi esplicito, isolamento dopo scarto messaggi in volo, ghost committed e oracolo read per slot indipendente. Superati test disco/apply/checkpoint bloccati, fixture GET HTTP/OpenAPI rigorose, storia HTTP cinque operazioni con ricerca ordini/controesempio indipendente e verifiche reali follower/failover/minoranza. Crash/restart tre processi verifica inoltre recovery dati confermati tramite GET, complementando AC-023/T-023 senza snapshot. Regressione completa M01–M09 e race mirato registrati nella relazione. M11 CAS/dedup/AC-012 e snapshot M13 restano aperti; checker generale AC-022/T-022 resta M14. C-03 acquisisce solo evidenze M10 lettore obsoleto/slot, non chiusura di ogni condizione.

## Evidenze gate CAS/sessioni M11

[M11](milestone-m11.md) soddisfa FR-010/AC-010/T-010, FR-011/AC-011/T-011 e FR-012/AC-012/T-012 per profilo sessioni v2 esplicito. Evidenze includono modelli indipendenti risultati/highwater ordinati, cap 1024 sessioni senza eviction, CAS canonico massimo, duplicato committed/cancellazione/shutdown bloccati, esclusione suffisso speculativo, tre seed riprodotti due volte, retry reale risposta persa/nuovo leader, CAS/duplicati concorrenti e recovery crash follower/leader/tutti processi. Nuovi vettori configurazione/identità/header testano confine compatibilità senza cambiare fixture vecchie. Conservati slot/partizione/replay GET e tutti test M01–M10. Evidenza dedup M11 C-03 soddisfatta; snapshot M13 e checker generale M14 aperti. Infrastruttura guasti/osservabilità/metriche M12 non implementata. Nessuna suite finita o race costituisce prova formale.

## Evidenze M12 e revisione M11

[M12](milestone-m12.md) soddisfa FR-017/AC-017/T-017, FR-020/AC-020/T-020 e NFR-008/AC-028/T-028: log redatti correlano elezione e timeout HTTP, metriche locali restano disponibili senza quorum, replay da manifest salvato è identico e rileva prima divergenza. Seed 7/42/117, partizione/heal, delay/drop/duplicate, crash/restart, short write/ENOSPC/Sync failure, modelli distinti e limiti inconclusive verificati. Revisione M11 controlla sequenze 12/11/13, gap/conflitto, cap 1024 e replay su macchina nuova. Regressioni profilo v1/v2/failover/recovery/GET/CAS mantenute; M13 snapshot e M14 checker generale restano aperti.

## Gate snapshot prima di M13

La [revisione pre-M13](pre-m13-review.md) classifica C-01/OQ-05 RESOLVED nel design, C-02/OQ-01 RESOLVED per il contratto/primitive esistenti e CONDITIONALLY READY per nuove generazioni, C-03 snapshot CONDITIONALLY READY. AC-015/T-015, AC-016/T-016 e sottoinsieme AC-026/T-026 non sono superati: richiedono la futura implementazione. AC-013/AC-023 storici non vengono estesi a snapshot o power loss. Nessun nuovo ID/gate, M13 e M14 non iniziate.

## Evidenze parziali M13 Fase A

[Relazione M13 Fase A](milestone-m13-preliminary.md): PRELIMINARY PHASE COMPLETED, M13 complessiva PARTIALLY COMPLETED. T-015/AC-015: codec, copia atomica e restore isolato/sessioni verificati, non trasferimento a follower o cambio termine durante install. T-016/AC-016: integrità immagini e rifiuti verificati; nessuna nuova pubblicazione, generazione, switch o crash recovery, criterio aperto. T-026/AC-026: limiti logici 4 MiB/10000 chiavi/16 KiB valori/1024 sessioni, immagine <=8 MiB e controllo copia a capacità uno verificati in memoria; picco directory, riserve fisiche, RAM/CPU/SSD non misurati. C-01 design RESOLVED e conformance codec isolata verificata; estensione C-02/OQ-01 e C-03 snapshot CONDITIONALLY READY. Nessun gate completo chiuso dal solo sottoinsieme; M14 non iniziata.

## M13 Fase B — 23 novembre 2026

La [Fase B](milestone-m13-persistence.md) aggiunge evidenze isolate su scrittura/rilettura, CURRENT, generazioni conservate, recovery nuovo senza replay del suffisso, 16 cutpoint e due uscite processo pre/post rename. T-015/AC-015/FR-015: persistenza e dedup, non trasferimento/installazione. T-016/AC-016/FR-016: pubblicazione isolata, non cleanup/compaction/switch live. T-026/AC-026/NFR-006: riserva old+new+staging e quote simulate, non report RSS/CPU o ammissione runtime. C-01/OQ-05 RESOLVED; C-02/OQ-01 isolato VERIFIED ma integrazione e C-03 CONDITIONALLY READY. M13 PARTIALLY COMPLETED, criteri completi aperti, M14 non iniziata.

## M13 Fase C — 23 novembre 2026

La [Fase C](milestone-m13-runtime-recovery.md) è RUNTIME RECOVERY PHASE COMPLETED: RecoverCommitted ripristina S e applica soltanto S+1..C con M02, poi drena/chiude il worker isolato prima del nuovo applier a C. WAL e snapshot sono validati sotto LOCK continuo. L'avvio è esplicito via node.StartAPIWithSnapshotRecovery per v2; StartAPI/CLI ordinari mantengono il guard base zero. Nessun fallback con CURRENT/snapshot corrotti. KV/sessioni, hard state e A=L=C verificati con crash/restart follower/leader/tutti processi. C-01/OQ-05 confermato; sottoinsieme recovery C-02/OQ-01 verificato, C-02/C-03 complessivi CONDITIONALLY READY. T-015/AC-015/FR-015, T-016/AC-016/FR-016 e T-026/AC-026/NFR-006 completi aperti: compaction, cleanup e InstallSnapshot non implementati. M13 PARTIALLY COMPLETED, M14 non iniziata.
