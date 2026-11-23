# WAL sincrono e generazioni snapshot

[Indice](../README.md) · [English](../../en/adr/ADR-005-persistence-strategy.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-005 — WAL sincrono e generazioni snapshot**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Crash di processo non deve cancellare successi nel modello dichiarato.

## Problema

Coordinare log, voto, commit checkpoint, snapshot e limiti disco.

## Alternative valutate

RAM-only; WAL async; WAL sync e snapshot; motore database esterno.

## Decisione

WAL frame CRC32C transazionali, Sync prima di ack, snapshot generazionali con CURRENT.

## Motivazione

Separa decisioni di consenso e durabilità locale, permette cutpoint riproducibili.

## Conseguenze

Snapshot include sessioni; errore storage ferma il nodo; checkpoint commit locale.

## Svantaggi e limiti

Latenza Sync, picco temporaneo e semantica power loss APFS non provata.

## Requisiti collegati

FR-013, FR-014, FR-015, FR-016, FR-018, NFR-003, NFR-006. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M03/M13: sync directory, lock, formato finale e cutpoint; power loss resta garanzia separata. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../storage-and-recovery.md); [fonti REF-08/09](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

WAL sincrono confermato, non atomicità fisica della write: recovery accetta soltanto transazioni complete verificate. Specificata barriera finale, hard state aggiornato, token generation_id/write_sequence e installazione coordinata con applier. Il cap 32 MiB include riserva controllo. Contratti separati per crash processo, sistema operativo e power loss; solo il primo è obiettivo verificabile v1 con test di processo. Non vengono rimosse Sync necessarie. OQ-01/OQ-05 restano gate prima dello storage durevole.

## Decisione pre-M03 — 19 novembre 2026

Conservare strategia e stato implementativo Proposed; nessun nuovo ADR necessario. I [formati canonici](../canonical-formats.md) risolvono il prerequisito progettuale C-01 con byte verificati. Il [contratto macOS](../macos-storage-contract.md) è il candidato formalmente scelto: File.Sync per file regolari sulla baseline Go ispezionata, fsync directory esplicito controllato, inode LOCK stabile con flock esclusivo non bloccante, rename/pubblicazione seriale nello stesso filesystem, errori con chiusura. Fonti/scelte riviste; test capacità NOT RUN.

C-02 è CONDITIONALLY RESOLVED per scelte documentate e OPEN per validazione implementativa, coerentemente con la fase test futuro adattatore dell’audit originale. Ammissibili all’inizio solo test isolati conformità/capacità; integrazione writer/pubblicazione attende evidenze primitive/lock/errori/visibilità. Barriere directory non supportate richiedono revisione qui, non errore ignorato o garanzia generica Sync. T-013/T-023 e T-015/T-016 restano gate accettazione implementativa. [Relazione readiness](../pre-m03-review.md). Nessun codice M03 o claim OS/power loss.

## Validazione preliminare M03

[Evidenze](../milestone-m03-preliminary.md): GATE C-02 VERIFIED per le scelte di primitive locali Darwin/APFS provate. Adottati ownership flock stabile, File.Sync/fsync directory controllati e sostituzione CURRENT nello stesso parent con errori incerti espliciti. Strategia generazionale/WAL complessiva resta Proposed fino ai test completi di fase; non accetta recovery/snapshot non implementati o garanzie OS/power loss. M03 PARTIALLY COMPLETED, in attesa di incarico separato per il residuo.

## Decisione implementativa M03 locale — 20 novembre 2026

[WAL locale M03](../milestone-m03.md) adotta questo ADR per append con frame della generazione iniziale, recovery hard-state/log/commit, lock e bootstrap esplicito. Evidenze T-013 locali superate; token generazione/sequence seguono contratto esistente. Riutilizzate stesse barriere File.Sync/directory e PublishCurrent. Parte snapshot/rotazione completa resta Proposed fino a M13, durabilità end-to-end fino a M09; nessuna garanzia OS/power loss accettata. Nessun formato persistente nuovo o cursore applied ridondante.

## Implementazione elettorale M05 — 20 novembre 2026

[M05](../milestone-m05.md) adotta la decisione per ruoli/timer deterministici, un voto persistente per termine, completamento esatto generazione/sequenza e noop elettorale locale. Un adattatore storage sincrono seriale sospende avanzamento core senza mutex globale; nessun output dipendente positivo prima di Sync. Shutdown conserva errori I/O e ownership fino al drain. Superati oracolo elettorale 20 seed, 100 round heartbeat sani e race mirate. Replica/commit/apply/snapshot completi e garanzie guasti fisici restano ai gate originali; non rende tutto Raft o rotazione generazioni Accepted.

## Decisione implementativa M06

[M06](../milestone-m06.md) conserva questo ADR: sostituzione suffisso canonica sopra confine committed noto, completamento sincronizzato con token esatto e progresso peer limitato. Byte/primitive WAL esistenti riusati; proposte locali conservano riserva con AppendProposal. RPC lecite grandi possono attraversare transazioni WAL limitate con un ack finale e prefisso recuperabile sicuro all’interruzione. Commit/quorum termine corrente/apply e semantica snapshot completa restano gate successivi. Nessuna accettazione Raft completo o durabilità OS/alimentazione inferita.

## Evidenze implementazione M07

[M07](../milestone-m07.md) conserva decisione per prova quorum termine corrente, commit_index sincronizzato, replay stato volatile nuovo tramite M02 invariato e cursori completamento espliciti. Superati oracoli committed/risultati indipendenti, backlog bloccato, tagli checkpoint e test loopback complementari. Nessun checkpoint aggiuntivo, byte persistente nuovo o goroutine per entry. Evidenze C-03 M07 non chiudono gate successivi snapshot/sessioni/read/checker o garanzie guasti fisici.

## Durabilità condizionata processi M09 — 22 novembre 2026

[M09](../milestone-m09.md) fornisce evidenza reale kill/restart singolo/tutti processi per MVP interno, con byte storia confermata e ispezione stato applicato nuovo. Conservare protocollo WAL/checkpoint sincrono esistente; nessun nuovo cursore applied o formato storage. Test richiedono directory integre e maggioranza recuperabile. Accetta solo sottoinsieme roadmap crash processi: evidenza GET HTTP resta dopo M10 e snapshot generali M13. Nessuna accettazione OS/perdita alimentazione o prova completa guasti.

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
