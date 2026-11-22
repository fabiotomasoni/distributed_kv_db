# Contratto storage macOS — C-02

[Indice](README.md) · [English](../en/macos-storage-contract.md)

**19 novembre 2026 — contratto primitive locali VERIFIED nella fase preliminare M03; progetto storico e accettazione residua sotto.**

## Gate originale e limiti delle fonti

C-02 blocca la scelta del contratto adattatore M03 senza revisione della documentazione piattaforma e la chiusura della durabilità M03/M09 senza validazione leggera dell'implementazione. Colloca esplicitamente la validazione dell'adattatore durante l'implementazione futura. La revisione pre-M03 originale completava fonti/scelte con esperimenti OPEN; le evidenze correnti sono registrate nella sezione finale. [Decisione e futuro lavoro consentito](pre-m03-review.md). Nessun adattatore/probe esisteva a quella revisione storica; la fase preliminare successiva implementa solo il sottoinsieme isolato di primitive.

Fonti primarie consultate il 19 novembre 2026: [Apple fsync](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html), [fcntl](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fcntl.2.html), [rename](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/rename.2.html), [flock](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html) e [About APFS](https://developer.apple.com/documentation/foundation/about-apple-file-system). I manuali sono d'archivio; il vecchio elenco filesystem di fcntl precede APFS e non stabilisce il supporto APFS attuale. Descrizioni API e sorgente Go ispezionato guidano le scelte, senza certificare sistema installato, filesystem o SSD.

Apple distingue fsync dalla richiesta di flush delle cache del drive tramite F_FULLFSYNC. Rename fornisce semantica di sostituzione nello stesso filesystem; da sola non è un protocollo QuorumKV di pubblicazione durevole. Flock è advisory: tutti i writer cooperanti devono usarlo. Le funzioni APFS non rendono atomica una transazione applicativa su più file. Queste sono le premesse; protocollo e garanzie limitate sotto sono decisioni/inferenze ingegneristiche locali da verificare nell'implementazione.

## Scelte per file, directory e lock

Ispezionata la toolchain Go 1.27.4 effettiva: os/file_posix.go delega File.Sync a internal/poll; [fd_fsync_darwin.go](https://raw.githubusercontent.com/golang/go/go1.27.4/src/internal/poll/fd_fsync_darwin.go) richiede F_FULLFSYNC e ripiega su fsync soltanto per ENOTSUP, riprovando EINTR tramite il wrapper. File.Sync non è quindi un semplice fsync in questa baseline. È verifica del sorgente, non misura del flush. Rivedere il percorso ai cambi di toolchain.

Barriera file candidata: File.Sync sui file regolari scritti, propagando tutti gli errori; niente secondo full flush ridondante per default. Il ritorno riuscito stabilisce la barriera di progetto per crash processo con OS vivo, non prova che la richiesta forte sia rispettata end-to-end. Il fallback non giustifica garanzie di resilienza superiori. Limitare il futuro adattatore v1 a directory nodo privata su APFS locale; niente SMB/NFS, cartella sincronizzata cloud, device rimovibile o generazioni spostate fra filesystem nel profilo supportato. M03 deve registrare OS/build, architettura, filesystem/mount e toolchain reali.

Barriera namespace candidata: aprire la directory, chiamare Darwin syscall.Fsync sul descrittore, riprovare soltanto EINTR, controllare esito ed errore di close. Capacità/ordine del fsync directory vanno testati; non assumere che File.Sync/F_FULLFSYNC dei file regolari valga per le directory. EINVAL/ENOTSUP o altri errori non gestiti bloccano la pubblicazione; mai omettere silenziosamente la barriera. Un'alternativa supportata richiede revisione ADR e nuove evidenze prima dell'uso. Soltanto API della libreria standard e un futuro file specifico Darwin; nessuna nuova dipendenza o necessità CGO.

Usare syscall.Flock(LOCK_EX|LOCK_NB) su file LOCK stabile nella radice, conservarne il descrittore durante tutto il lavoro e shutdown, impostare close-on-exec. Non scollegare, troncare/ricreare o ruotare l'inode bloccato: un secondo inode potrebbe ammettere un altro proprietario. Contesa rifiuta l'avvio; errori diversi sono fatali. PID file soltanto diagnostico, mai lock. Nessun descrittore ereditato da un figlio. I test futuri devono verificare rifiuto da processo con apertura separata, rilascio dopo close normale e dopo uscita del figlio appartenente al run. Il lock è cooperativo, non protegge da modifiche arbitrarie di terzi.

Aprire soltanto file regolari/directory riconosciuti sotto la radice privata; rifiutare symlink, identità errata e nomi generazione malformati, impedire traversal. File 0600, directory 0700. Chiavi KV dentro immagine/log codificati, mai percorsi. Scritture diagnostiche limitate e nessun valore nei log.

## Ordinamento, errori e pubblicazione

Un worker storage seriale possiede append e pubblicazione. Scrivere buffer completi controllando i conteggi: un conteggio corto senza errore deve continuare dal resto non scritto; write senza progresso o con errore fa fallire l'effetto, mai ack di un prefisso. Non riprovare mai un append intero dopo scrittura parziale incerta. Un writer con buffer utente deve fare flush prima di Sync. Voto positivo, risposta di replica, conteggio replica locale, checkpoint apply e successo client non precedono la rispettiva barriera necessaria riuscita dell'[ordinamento storage](storage-and-recovery.md).

ENOSPC, EIO, errore Sync, namespace o close rilevante chiudono la partecipazione storage; conservare evidenze e richiedere recovery/riavvio espliciti. Cancellare il context non cancella una syscall filesystem in corso né dimostra assenza di scritture. La deadline shutdown segnala chiusura incompleta; non deve chiudere/riusare un descrittore mentre una write continua o pubblicare effetti incerti. Nessuna risposta autorizzata da completamento di generazione obsoleta.

Pubblicare soltanto con staging nello stesso parent/filesystem: riservare picco; creare nuova generazione; completare snapshot e WAL completo con voto/commit correnti sotto barriera finale; sincronizzare entrambi i file e directory generazione; rinominare generazione e sincronizzare radice nodo; scrivere/sincronizzare CURRENT.tmp; rinominarlo su CURRENT; sincronizzare radice. Solo allora cambiare writer e confermare pubblicazione. Creazione radice/IDENTITY deve analogamente sincronizzare file e parent pertinente prima di pubblicare bootstrap. M03 implementerà soltanto il proprio sottoinsieme; rotazione/installazione snapshot resta M13.

Se rename riesce e la barriera directory successiva fallisce, segnalare fatale/incerto, conservare entrambe le generazioni senza rollback di CURRENT o eliminazione di candidati. Recovery valida il CURRENT effettivamente presente. Cleanup soltanto dopo pubblicazione verificata, rimuove solo percorsi vecchi/staging riconosciuti non referenziati e sincronizza il parent dopo rimozione. Errore cleanup ferma ulteriori rotazioni/ammissioni quanto necessario per rispettare il cap; non giustifica eliminare dati attivi. Mai sostituire CURRENT tramite copia-e-cancella, scegliere vecchia generazione dopo corruzione o sostituire hard state corrente con voto snapshot obsoleto.

## Contratti di guasto e protezione del Mac

| Evento | Contratto / evidenza richiesta |
|---|---|
| Shutdown normale | Drenare storage accettato, barriera delle modifiche pendenti, chiudere file/lock, segnalare errori. Stop scaduto incompleto. Non ancora testato. |
| Crash processo, OS vivo | Obiettivo v1: write/rename completati possono sopravvivere anche senza Sync; write incomplete possono lasciare prefissi. Preservare dipendenze sincronizzate e riprodurre soltanto commit provato. Futuri T-013/T-023. |
| Crash/reboot OS | Nessuna garanzia validata; simulare soltanto un modello astratto dichiarato. Nessun test di crash reale autorizzato. |
| Perdita improvvisa alimentazione | Garanzia esclusa da v1; nessun esperimento fisico, nessuna deduzione da Sync o nome APFS. |
| Errore I/O / corruzione / perdita SSD | Chiusura su corruzione completa verificata, metadati incoerenti o errori I/O. Checksum rilevano casi, non riparano danni arbitrari. Perdita dell'SSD condiviso o maggioranza durevole senza garanzia di conservazione. |

Conservare tutte le sincronizzazioni necessarie. Evitare riscritture persistenti identiche, consentire batching causalmente sicuro senza ack anticipati e compattare soltanto alle soglie previste. Cap directory logica 32 MiB inclusi staging/riserva controllo, WAL 8 MiB, snapshot 8 MiB; nessun contatore applicativo garantisce allocazione fisica APFS o spazio riservato rispetto alle altre app. Non sommare spazio libero APFS condiviso fra volumi. Niente preallocazioni di immagini grandi o riempimento SSD per ENOSPC; iniettare errori.

## Validazione pendente e confine esatto di autorizzazione

| Evidenza / responsabile | Milestone e procedura finita | Condizione di superamento |
|---|---|---|
| Mount locale e primitive / sviluppatore | Primi test isolati adattatore M03; registrare OS/Go/arch/filesystem, piccoli file e barriere directory. | Profilo supportato registrato; ogni barriera richiesta riesce e gli errori sono propagati. |
| Lock cooperativo / sviluppatore | M03: parent e figlio del run, aperture separate, close normale e uscita forzata del figlio. | Secondo proprietario rifiutato durante lock; nuova apertura esplicita riesce dopo rilascio. Niente kill per nome globale. |
| Visibilità sostituzione / sviluppatore | M03: piccoli manifest vecchio/nuovo; massimo 16 cutpoint pubblicazione con subprocessi del run e barriere sincronizzate. | CURRENT recuperato seleziona generazione completa verificata; niente ack anticipati o fallback. |
| Policy write parziali/errori / sviluppatore | M03: iniettare write corte/nulle, Sync file/directory fallita, ENOSPC/EIO ed errori close rilevanti. | Nessun successo su effetto fallito/incerto; niente spin o retry cieco del frame intero. |
| Conformità reader / sviluppatore | M03 T-013 consuma fixture statiche frame/manifest/bootstrap; M13 aggiunge scenari snapshot/switch completi. | Byte positivi accettati, negativi rifiutati, troncamento solo al confine finale consentito. |
| Successo durevole dopo restart / sviluppatore | M09 T-023; M13 T-015/T-016 coprono rotazione, handoff voto e installazione. | Proprietà prefisso acknowledged e dedup nel modello crash processo. Nessuna estrapolazione OS/power loss. |

Budget iniziale probe reali M03: <=16 KiB di contenuto sintetico per caso, <=16 cutpoint, <=2 processi del run, <=1 MiB di scritture deliberatamente sottomesse complessive, <=30 s/caso e <=120 s suite. Contare byte tentati incluse ripetizioni; fermarsi al budget, non è un'asserzione sulle scritture interne SSD. Directory run dedicata, cleanup dei soli file del run, niente privilegi/flush cache/reboot/strumenti device. Test modello con piccole copie in memoria. Questi limiti sono requisiti di test futuri; nessun test storage eseguito in questa revisione.

C-02 è **CONDITIONALLY RESOLVED per la scelta documentata, OPEN per validazione implementativa**. Soltanto conformità formati e test isolati adattatore/capacità sono proponibili come prime attività M03. Prima di integrare writer/percorso pubblicazione, richiedere le evidenze primitive/lock/errori/visibilità sopra, oppure rivedere ADR e bloccare il percorso interessato. Accettazione durabilità M03/M09, claim OS/power loss, deploy di produzione e installazione snapshot M13 restano bloccati ai rispettivi gate. È la validazione per fasi già richiesta dall'audit originale, non una deroga. Questo incarico non inizia M03.

## Verifica C-02 corrente

[Relazione M03 preliminare](milestone-m03-preliminary.md): GATE C-02 VERIFIED per macOS 27.2.1/26B131, Go 1.27.4 darwin/arm64 su APFS locale /System/Volumes/Data. Riusciti File.Sync, probe capacità F_FULLFSYNC separato e fsync directory; superati lock interprocesso/rilascio close/kill, cutpoint pre/post-rename con riferimenti fixture verificati ed errori I/O/close iniettati. Il contratto candidato di primitive sopra è adottato per questo profilo; barriere non supportate continuano a bloccare.

Non è implementazione generazionale completa o accettazione recovery/durabilità. Conformità byte senza stato copre il sottoinsieme iniziale reader; replay/riparazione fra frame resta M03 T-013, durabilità successi processo M09 T-023, handoff/installazione snapshot M13 T-015/T-016. Claim OS/power loss esclusi. M03 PARTIALLY COMPLETED; nessun lavoro residuo parte automaticamente. I precedenti OPEN/NOT RUN nel documento descrivono il progetto pre-M03, non le evidenze primitive correnti.

## Completamento M03 successivo

20 novembre 2026: [M03 COMPLETED localmente](milestone-m03.md), con WAL sincrono, validazione transizioni persistenti, reader/recovery log limitati, bootstrap esplicito ed evidenze errore/processo. C-01/C-02 conservano ambito locale risolto/verificato. Nessun apply macchina, consenso, snapshot/rotazione completi o codice M04. Precedenti stati parziali/proposti descrivono la fase storica pertinente; durabilità end-to-end/fisica ancora non verificata ai gate successivi.

## Riconferma dell’incarico ripetuto

20 novembre 2026: [nuove verifiche nel perimetro preliminare](milestone-m03-preliminary.md) riconfermano C-02 VERIFIED sullo stesso profilo APFS locale. Sorgenti/fixture invariati; superati test limitati codec/adattatore/M01/M02 e race mirato. Il prompt preliminare ripetuto non annulla M03 completata separatamente né autorizza M04.

## M13 Fase B — 23 novembre 2026

La [Fase B](milestone-m13-persistence.md) riusa le primitive APFS/Darwin senza downgrade: LOCK stabile, file esclusivi privati, write complete, File.Sync e Close controllati, fsync directory separato e rename nello stesso parent. Test isolati su generazioni reali, 16 cutpoint, errori file/namespace e due uscite figlio pre/post CURRENT passano; C-02 estensione isolata VERIFIED, integrazione runtime CONDITIONALLY READY. Nessun cleanup attivato, nessun claim reboot/OS/alimentazione. File.Sync non equivale da solo a pubblicazione completa.
