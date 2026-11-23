# Profilo applicativo versionato per sessioni replicate

[Indice](../README.md) · [English](../../en/adr/ADR-010-session-profile.md)

**ADR-010 — Progetto M11 accettato, 22 novembre 2026; evidenze implementazione registrate nella relazione M11.**

## Problema

M08–M10 persistevano client_id/sequence come metadati senza deduplicazione. Replay degli stessi byte committed con regole sessioni può rimuovere effetti già confermati. Abilitare silenziosamente sessioni su directory v1 è insicuro. Cache solo leader non risolve.

## Decisione e compatibilità

Aggiungere configurazione v2 esplicita che richiede application_version=2, includendo campo nel config_hash condiviso. Nodo v2 usa IDENTITY con checksum format_version=2 e stessi campi identità; versione denota semantica sessioni dall’origine vuota. Configurazione v1 e IDENTITY v1 esistenti conservano byte esatti e semantica M10. Eseguibile nodo esistente seleziona profilo dalla configurazione; niente negoziazione rolling o conversione automatica.

M11 richiede cluster nuovo/directory private vuote con bootstrap esplicito e tutti tre membri v2. Apertura di qualunque profilo identità con l’altro fallisce prima di riparazione WAL/replay/listener; hash condivisi impediscono anche RPC peer con profili misti. Conservare directory v1 esistenti e usarle solo come profilo legacy M10. Nessuna migrazione in-place, copia dati attivi o reset silenzioso. Migrazione export/import offline richiederebbe progetto separato e qui non implementata.

Frame/transazioni WAL, comandi/digest, CURRENT e snapshot vuoto restano codifiche v1 approvate. Nessun nuovo campo replicato necessario: identità e sequence già presenti. Mutazioni nel profilo v2 richiedono inoltre header HTTP `X-QuorumKV-Application-Version: 2`; v1 rifiuta tale header e v2 rifiuta versione assente/diversa prima di ammissione. Test client lo deriva dalla configurazione. Risposte mutanti v2 confermano stesso header versione e client M11 rifiuta conferma assente/diversa come unknown. Binari storici possono ignorare header richiesta sconosciuti, quindi nessun completamento tra versioni viene considerato affidabile. GET invariata. Evita che vecchio client tratti silenziosamente sequenza ripetuta come operazione nuova su endpoint riusato. Nuova versione identità cambia esplicitamente contratto applicativo, anziché modificare silenziosamente significato vecchi byte. Snapshot vuoto v2 senza sessioni; snapshot completi/compaction restano M13.

## Ownership e risultati

Envelope comando committed privato porta metadati entry canonica limitati attraverso worker M02 invariato a wrapper sessioni attorno a M01 invariata. Wrapper controlla atomicamente identità/digest, applica al massimo una volta e conserva sequence/digest/ultimo risultato compatto/indice originale. Solo lavoro committed lo raggiunge. Replay nuovo ricostruisce mappa e sessioni insieme; nessun secondo database persistente sessioni. Entry duplicate avanzano comunque indici apply effettivi. Controlli completamento/leadership usano indice/termine nuovo tentativo committed; HTTP riporta indice risultato originale conservato.

## Limiti e validazione

Massimo 1024 sessioni, niente TTL/eviction, solo ultimo risultato <=512 byte ciascuno; sequenze vecchie mai riapplicate. Risultato compatto senza valori precedenti. Esiti unknown ancora possibili e ogni retry attraversa nuovo tentativo leader/quorum/apply. Testare rifiuto vecchio/nuovo senza modifiche file, vettori canonici identità/configurazione v2, CAS duplicati concorrenti, conflitti payload, errori ordinati, failover e recovery tutti processi. Preservare fixture originali. Nessuna garanzia universale exactly-once, OS/alimentazione o M12/M13.

## Decisione progettuale pre-M13 — 22 novembre 2026

La [revisione pre-M13](../pre-m13-review.md) conferma snapshot QSN1/payload v1 con sessioni complete e identità/config v2 per il runtime stateful futuro; nessuna migrazione v1 o modifica ai vettori. Scelti capture dopo drain/parking del worker e copia atomica KV/sessioni, barriera finale distinta con hard state aggiornato, CURRENT autorevole senza fallback, riserva picco e InstallSnapshot come parte M13. Contratto progettuale precisato; writer/codec generale/install/recovery snapshot non implementati e proprietà T-015/T-016/T-026 non validate. C-01 design RESOLVED, estensione C-02 e C-03 snapshot CONDITIONALLY READY; nessuna accettazione durabilità OS/alimentazione. M13 NON INIZIATA.

## Conformità isolata M13 Fase A — 22 novembre 2026

[Fase A](../milestone-m13-preliminary.md) implementa il formato QSN1/payload v1 e copie owner-only KV/sessioni, con v2 binding e restore esclusivamente nuovo/isolato. Nessuna versione o migrazione aggiunta; vettori precedenti immutati. Confermato controllo worker separato dai comandi indexed, drain B e proof coerente prima di copiare, encoding dopo Release. C-01 conformance isolata verificata; nessuna accettazione delle generazioni, InstallSnapshot, compaction o crash consistency runtime. C-02/C-03 e T-015/T-016/T-026 completi restano aperti.

## Persistenza isolata M13 Fase B — 23 novembre 2026

La [Fase B](../milestone-m13-persistence.md) verifica build/pubblicazione/recovery isolato QSN1 e CURRENT, voto finale aggiornato e conservazione completa sessioni v2. Confermati formato e protocollo esistenti, senza terzo manifest o migrazione. Nuove evidenze C-02 isolate VERIFIED; integrazione C-02/C-03 CONDITIONALLY READY. Cleanup, switch runtime, replay suffisso, compaction e InstallSnapshot non implementati; T-015/T-016/T-026 completi aperti, nessun claim OS/alimentazione.

## Recovery runtime M13 Fase C — 23 novembre 2026

La [Fase C](../milestone-m13-runtime-recovery.md) adotta il recovery base-S e il replay del solo suffisso committed su KV/sessioni nuovi prima dei listener, tramite capability esplicita v2 e LOCK continuo. Termine/voto da WAL finale, cursori A=L=C, nuova epoca iniziale, nessun result ring storico. Percorso predefinito base zero invariato; nessun nuovo formato/configurazione o migrazione. C-02 recovery runtime verificato soltanto nel sottoinsieme; C-02/C-03 restano CONDITIONALLY READY per ciclo live, compaction/installazione ed epoche. T-015/T-016/T-026 completi aperti, nessuna garanzia OS/alimentazione.

## Compaction isolata M13 Fase D — 23 novembre 2026

La [Fase D](../milestone-m13-compaction-preliminary.md) adotta la rappresentazione già approvata senza nuovi campi: snapshot QSN1 e WAL base-S, hard state finale, suffisso completo e CURRENT v1 con identità v2. Il nuovo incarico consente la verifica isolata non distruttiva; nessuna ulteriore decisione di formato necessaria. Cattura derivata da apply effettivo, confronto KV/sessioni/indici/suffisso prima e dopo pubblicazione B/C. Reader senza repair soltanto nella prova D; comportamento ordinario invariato. C-02/C-03 complessivi CONDITIONALLY READY e T-015/T-016/T-026 aperti. Nessuna attivazione live, GC, cancellazione o InstallSnapshot autorizzata dalla sola equivalenza.

## M13 Fase E installazione isolata — 23 novembre 2026

La [Fase E](../milestone-m13-installsnapshot-preliminary.md) è **ISOLATED INSTALLSNAPSHOT PHASE COMPLETED**: ricevente/controller dormienti, chunk M04 esistenti, persistenza reale M05 di termine/voto, suffisso locale coerente, pubblicazione B sotto LOCK continuo, KV/sessioni nuovi e apply epoch, ACK finale correlato e catch-up deterministico del suffisso verificati in isolamento. Nessuna integrazione ordinaria node/service/server/transport. C-01/OQ-05 confermato; aggiunte evidenze isolate C-02/OQ-01 e C-03, entrambi complessivamente CONDITIONALLY READY. T-015/AC-015/FR-015, T-016/AC-016/FR-016 e T-026/AC-026/NFR-006 completi restano aperti. InstallSnapshot runtime, compaction live, GC ed eliminazione prefissi non attivati; M13 PARTIALLY COMPLETED, M14 non iniziata.

Decisioni esistenti invariate: un protocollo canonico di pubblicazione, termine/voto **locali** aggiornati, conservazione suffisso con confine corrispondente, restore atomico KV/sessioni dopo CURRENT, ACK parziale mai prova di replica. LOCK stabile prestato senza rilascio; vecchio apply completa prima della sostituzione, epoch nuova rifiuta completamenti obsoleti. Nessuna autorizzazione compaction/GC runtime e nessuna migrazione v1.
