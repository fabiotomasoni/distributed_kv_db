# M13 Fase D — Compaction isolata e verifica di equivalenza

[Indice](README.md) · [English](../en/milestone-m13-compaction-preliminary.md)

**23 novembre 2026 — ISOLATED COMPACTION PHASE COMPLETED. M13 complessiva: PARTIALLY COMPLETED. M14 NON INIZIATA.** Baseline Git pulita `243a48a`; Fasi A–B–C già registrate nella cronologia prima dell'incarico. Nessun commit effettuato in questa fase.

## Verifica preventiva dell'autorizzazione

La [roadmap](implementation-roadmap.md), letta integralmente in entrambe le lingue, assegna M13 a snapshot/compaction, generazioni atomiche, installazione e T-015/T-016/T-026 nel relativo sottoinsieme. M14 conserva il checker generale e le verifiche delle proprietà. [Revisione pre-M13](pre-m13-review.md), [Fase A](milestone-m13-preliminary.md), [Fase B](milestone-m13-persistence.md), [Fase C](milestone-m13-runtime-recovery.md), ADR-005/ADR-010 e formati canonici fissano già la rappresentazione base-S e il protocollo di pubblicazione.

La conclusione Fase C consente a un nuovo incarico di autorizzare progettazione/verifica della compaction sotto i gate esistenti. Questo incarico autorizza soltanto costruzione isolata non distruttiva: il formato base-S è già supportato da WAL/reader, il writer B e il recovery C sono verificati. Nessun ADR impone un nuovo formato o decisione per ricostruire questa rappresentazione in directory di test. **Esito della verifica prima del codice: implementazione isolata consentita**, senza trasformare C-02/C-03 CONDITIONALLY READY in autorizzazione runtime. Nessuna discrepanza bloccante o nuovo gate inventato.

## Scope e file

Creato `internal/snapshot/compaction.go`: PrepareCompaction, verifica di equivalenza e orchestrazione privata compactIsolated. Creati `internal/snapshot/compaction_test.go` e `internal/raft/compaction_boundary_test.go`. Modificato soltanto `internal/snapshot/persistence.go`: estrazione del protocollo Publish in publishOwned sotto lo stesso mutex e modalità condivisa del reader senza riparazione per le prove non distruttive. Publish pubblico e recovery B/C conservano il comportamento precedente.

PrepareCompaction è una funzione in memoria su input sintetici immutabili; non modifica file o core. L'orchestrazione di pubblicazione è privata del package e chiamata soltanto dai test su directory create dai test. Nessun chiamante node/service/server, opzione, trigger o integrazione client aggiunto. StartAPIWithSnapshotRecovery e il percorso ordinario restano invariati; nessun sorgente Raft di produzione modificato.

Documentazione aggiornata nelle due lingue: storage-and-recovery, canonical-formats, raft-consensus, architecture, failure-model, testing-strategy, acceptance-criteria, traceability, implementation-roadmap, development-guide, ADR-005/ADR-010, indici e README principale; aggiunte le due relazioni milestone-m13-compaction-preliminary. Fixture, dipendenze, codec e relazioni storiche non riscritti.

## Rappresentazione compatta e invarianti

Nessun nuovo formato: identità/configurazione applicativa v2, contenitore QSN1/payload v1 e CURRENT v1. CURRENT è il manifest autorevole, non un terzo file per generazione. gen-N contiene snapshot canonico e wal; base_index/base_term e hash whole-file devono concordare con l'immagine. Generazione nuova mai riusata, inclusi nomi staging, senza overflow.

Dato il log validato a base b, commit C e ultimo indice N, S deve soddisfare b<=S<=C<=N e il termine al confine deve coincidere con term[S]. Snapshot e sessioni rappresentano solo il prefisso **realmente applicato su una nuova istanza isolata**: PrepareCompaction non assume che commit/scheduled del nodo vivo equivalgano ad apply. Ricostruisce e applica fino a S, avvia un proprietario esclusivo al cursore S, parcheggia il worker e usa Capture Fase A con A=W=L=S e C sincronizzato della vista isolata. Release precede encoding; i worker precedenti sono terminati prima di trasferire la proprietà.

Il WAL riusa generationWAL B: sequence iniziale 1 con hard state corrente e commit=S, tutto S+1..N in transazioni limitate e checkpoint finale C. Indici <=S sono rappresentati dallo snapshot; S+1..C saranno applicati dal recovery; C+1..N rimangono byte per byte disponibili ma non applicati. Termine/voto provengono dalla vista finale, non dalla cattura. termS resta sentinella Log Matching. Sequence fisica e generation_id possono cambiare; non sono indici Raft. Il caso MaxUint64 non calcola S+1 con wraparound.

## Costruzione, pubblicazione e conservazione

Prima di scrivere, PrepareCompaction verifica il candidato ricostruendo entrambi i percorsi fino a C. compactIsolated mantiene il mutex dell'owner isolato durante lettura, preparazione, prova, pubblicazione e verifica finale, senza lock del core o mappe vive. Riusa il protocollo B, non un secondo writer: riserva contabile; gen-N.tmp; write complete/Sync/Close e rilettura snapshot/WAL; fsync staging; rename gen-N e fsync root; CURRENT.tmp canonico write/Sync/Close e rilettura; rename su CURRENT e fsync root.

Il nome generazione è restituito solo dopo verifica del recovery anche dai file pubblicati. Prima del rename CURRENT resta autorevole la generazione precedente; dopo rename o errore successivo l'esito può essere incerto. Nessun successo su errore, nessun rollback o selezione del gen-N più alto. Un fallimento della prova dopo pubblicazione avvelena l'owner e conserva CURRENT effettivamente presente e tutti i candidati.

Generazioni precedenti e WAL originali non vengono modificati o cancellati. La sola sostituzione autorizzata è CURRENT atomico nella directory del test. Staging/CURRENT.tmp falliti sono conservati e conteggiati; un CURRENT.tmp preesistente blocca nuova pubblicazione. Nessun cleanup, GC, rimozione di prefissi, risparmio fisico dichiarato o compaction runtime. I file dei test sono rimossi soltanto dal cleanup delle loro directory temporanee.

## Recovery senza mutazione della sorgente

La prova usa lo stesso reader B con riparazione disabilitata: una coda WAL incompleta è un errore e non viene troncata. Questo protegge i byte sorgente, inclusi i casi di corruzione nota introdotta dal test. Non modifica la politica M03/B/C: il recovery ordinario continua a poter riparare soltanto la coda finale strutturalmente incompleta ammessa dal modello, mai corruzione completa. Sync e Close continuano a essere controllati.

CURRENT, identità, profilo, immagine/hash/base e WAL devono tutti validare. Il recovery C ripristina KV/sessioni in una nuova istanza e applica solo S+1..C; A=L=C alla fine, N e il suffisso speculativo separati. Errori o cancellazione non espongono stato parziale. Corruzione della selezione corrente non autorizza fallback a vecchie generazioni, neppure se restano valide.

## Prove di equivalenza e deduplicazione

Percorso A nei test: replay diretto del prefisso committed completo, attraverso M02/M11 senza PrepareCompaction o RecoverCommitted. Percorso B: immagine candidata/pubblicata, decoder e recovery C del solo suffisso. Il confronto verifica ogni chiave/valore e la presenza, byte logici, ogni client/sequence/fingerprint/tipo/risultato/indice originario, commit, cursori, hard state, ultimo indice, nuovo confine e tutte le entry conservate, incluse speculative. Hash e conteggi non sostituiscono le righe diagnostiche. Entrambi i percorsi condividono volutamente la macchina di dominio del progetto; non è il checker indipendente M14.

La piccola storia contiene PUT, DELETE, read, noop, CAS riuscita/fallita, retry deduplicato, sequenza obsoleta e più client. Provati **tutti gli 11 confini S=0..10**, non solo quelli favorevoli, con N=11 e C=10. Hard state finale termine 4/voto n2 è conservato anche quando termS è 1 o 2. Il valore speculativo a 11 non contamina la mappa finale. Recovery ripetuto e riapertura esplicita producono le stesse righe; compaction successive a base nonzero conservano tutte le generazioni.

Un ulteriore caso realmente eseguito di 1024 client resta in memoria: cattura a S=512, replay fino a C=1024, rifiuto del nuovo client e avanzamento di uno esistente, senza eviction. Il caso MaxUint64 verifica solo aritmetica di confine senza nuove entry. Otto alterazioni lecite nella forma ma diverse nel significato (KV, sequence/digest/risultato/indice sessione, voto, byte o assenza del suffisso speculativo) devono fallire il confronto, anche quando la mappa KV finale coincide.

## Test, fault injection e risultati

13 nuove funzioni test: 11 snapshot e due scenari Raft puri; sottocasi tabellari limitati. Nessun nuovo processo distribuito per Fase D; le regressioni A–C comprendono i test locali a tre processi già disponibili.

| Gruppo | Stato osservabile e risultato |
|---|---|
| Confini 0..10, base nonzero ripetuta, raw input invalido/cancellato | Equivalenza dettagliata oppure rifiuto prima di creare staging; sorgenti preservate. |
| 16 cutpoint B: prima create, snapshot/WAL written/synced, prima/dopo rename generazione, CURRENT prima create/written/synced/prima/dopo rename e root Sync | CURRENT vecchio prima del proprio rename; nuovo dopo rename completato; recovery committed equivalente e file vecchi intatti. |
| Nove fault file: partial ENOSPC, Sync e Close su snapshot/WAL/CURRENT.tmp | Errore esplicito, nessuna nuova selezione, residui conservati. |
| Cinque fault namespace: directory staging, rename generazione/CURRENT e root Sync prima/dopo CURRENT | Nessun successo; selezione nuova solo se CURRENT rename completato. |
| Quota insufficiente, CURRENT.tmp già presente | Nessuna eliminazione per spazio o pulizia automatica; rifiuto e preservazione evidenze. |
| Dieci corruzioni: snapshot troncato/checksum/versione, WAL troncato/checksum/buco/termine confine, manifest, CURRENT e profilo | Errore senza fallback o riparazione della sorgente nella procedura di compaction. |
| Corruzione dopo pubblicazione e cancellazione del waiter dopo root Sync | Esito incerto; CURRENT nuovo conservato, nessun rollback; recovery esplicito valido nel caso sola cancellazione. |
| Close concorrente durante write bloccata | LOCK mantenuto fino al drain, nessun deadlock o successo anticipato. |
| Follower corto/divergente, termine confine errato e conflitto committed | Sentinella conservata; rifiuto o riconciliazione del solo suffisso uncommitted, nessuna replica fittizia. |
| Cleanup/rimozione prefissi runtime | Non implementati e non eseguiti; relative prove restano mancanti. |

Le prove riusano i cutpoint e wrapper I/O B con primitive reali su directory run-owned; non estendono lo scenario M12 con azioni non supportate. La matrice fissa input e ordinamento, senza stress o dipendenza da sleep. I test ispezionano file selezionati, righe applicative e byte originali. Le corruzioni sono introdotte deliberatamente dal solo test sui suoi file.

Superati test mirati D e suite completa M01–M12/M13 A–D con copertura, timeout 75 s/package, GOMAXPROCS=2 e package seriali. Package snapshot circa 7,4 s nella regressione, node circa 17,2 s con i precedenti test a tre processi. Race mirato completo superato su snapshot (circa 14,6 s), raft (circa 6,5 s), wal, storagefs, applier e committed. Superati gofmt, go vet, build macOS ARM64, fixture canoniche, dipendenze e git diff --check. Il verificatore Python controlla aritmetica dei vettori: i test Go, distinti, verificano il recovery. go list -m all restituisce solo il modulo del progetto, nessun go.sum o dipendenza aggiunta. Controllo bilingue: 144 Markdown, 71 coppie, link/tabelle/ID/blocchi tecnici senza errori. Il controllo del perimetro conferma soltanto persistence.go modificato nei sorgenti precedenti, nessuna chiamata runtime alla compaction e HEAD invariato a 243a48a.

Copertura statement misurata sull’intera suite, deduplicando i blocchi importati da più package: snapshot 87,3%, nuovo compaction.go 83,8% (88/105), raft 88,8%, wal 83,5%, applier 95,1%, committed 85,1%. Profilo temporaneo `/tmp/quorumkv-m13d.cover`. Non è una prova di correttezza o safety dei percorsi runtime non implementati.

## Follower in ritardo e condizioni per il futuro handoff

Un leader base-S non può costruire AppendEntries se nextIndex<=S; anche un follower con log che non raggiunge la sentinella S/termS non può accettare il primo suffisso. I test verificano ErrUnsupported nel builder sotto base, rifiuto del follower corto e matchIndex remoto invariato: possedere uno snapshot locale non prova che il follower lo abbia installato. Con base compatibile, un suffisso divergente non committed può essere riconciliato dalle regole M06 esistenti; conflitto nel committed è fatale. Nessun core di produzione modificato.

Prima di attivare compaction o rendere eliminabile un prefisso nel runtime occorrono ancora:

1. Cattura del vero A=W=L=S, poi barriera storage finale distinta: sospendere nuovi effetti persistenti/rotazioni, drenare I/O e fissare termine, voto, C e N aggiornati. Conservare anche le entry arrivate dopo cattura.
2. Pubblicazione completa con riserva old+new+staging, switch writer/core a token generation_id/write_sequence corretti, invalidazione dei completamenti vecchi. La mappa viva può essere oltre S: non resettare A/L né riapplicare il prefisso.
3. Conservare S/termS, immagine completa e suo digest whole-file/configurazione, tutto il suffisso necessario e hard state locale. Non inventare avanzamenti nextIndex/matchIndex e non cancellare l'unica sorgente necessaria ai follower.
4. InstallSnapshot verificato: leader/termine/identità validi, chunk <=64 KiB, immagine <=8 MiB, offset/retry identico, timeout staging 5 s, quote e digest. Prima dell'install finale drain dell'applier, rivalutazione S contro A e leader/epoche correnti; obsolete se S<=A, nessuna perdita committed su mismatch, conservazione del suffisso solo nelle condizioni approvate. Preservare voto locale aggiornato.
5. Pubblicazione prima della sostituzione atomica KV/sessioni, nuova applyEpoch con overflow controllato, invalidazione waiter/result ring/completion obsoleti. Solo ack finale correlato installed_index può autorizzare il progresso leader; mai un ack chunk. Gestire ack perso secondo contratto.
6. Prove di cancellazione/GC soltanto dopo autorità e recuperabilità verificate, mai cancellare attivi per quota; errori I/O post-CURRENT fermano partecipazione senza rollback. Shutdown deve completare o dichiarare incompleto il drain prima del rilascio LOCK. La deadline non annulla syscall incerte.

Queste sono condizioni già approvate da verificare in un futuro incarico, non nuova implementazione o autorizzazione permanente. InstallSnapshot e cancellazione runtime rimangono **non pronti all'attivazione**.

## Gate e criteri ufficiali

| Gate / criterio | Evidenze A–C e aggiunta D | Stato finale / prove mancanti |
|---|---|---|
| C-01 / OQ-05 | Codec, profilo v2 e riferimenti canonici riusati senza nuovi byte. | RESOLVED, conformità verificata. |
| C-02 / OQ-01 | Primitive/publicazione B e recovery runtime C conservati; D prova costruzione/pubblicazione/recovery compatto isolato. | Sottoinsieme isolato VERIFIED; gate complessivo CONDITIONALLY READY per switch live/cleanup/guasti residui. |
| C-03 | Capture/dedup/handoff C conservati; equivalenza D e sentinella/suffisso verificati in isolamento. | CONDITIONALLY READY per installazione, epoche/completion obsolete e safety cancellazione runtime. |
| T-015 / FR-015 / AC-015 | Codec, capture, restore+suffix, retry dopo restart ed equivalenza compatta. | Aperto: il criterio ufficiale richiede follower molto arretrato, retry chunk/cambio termine e InstallSnapshot. |
| T-016 / FR-016 / AC-016 | Pubblicazione/cutpoint B, crash processo C ed equivalenza D con sorgenti preservate. | Aperto: cleanup/GC, cancellazione prefissi e handoff compaction nel runtime vivo. |
| T-026 / NFR-006 / AC-026 | Quote A/B, code/reader C, D cap reale 1024, conservazione del suffisso, riserva e rifiuto pre-write. | Aperto: interazioni runtime/cleanup e report RSS/CPU/spazio completo previsto dalla roadmap. |

Le definizioni originali in [test](testing-strategy.md) e [accettazione](acceptance-criteria.md) non cambiano. Nessuna prova isolata chiude un criterio che richiede funzionalità ancora assenti.

## Problemi, garanzie e limiti

Nessun bug M01–M12 o Fasi A–C rilevato richiede correzione. La nuova modalità di lettura senza repair è una scelta necessaria alla prova non distruttiva, non una migrazione o nuova interpretazione del WAL. Il protocollo B è stato soltanto fattorizzato per conservare un'unica sequenza di pubblicazione e serializzarla con la prova D. Non sono emersi problemi bloccanti nelle verifiche eseguite.

Verificato: rappresentazione compatta recuperabile e logicamente equivalente nel perimetro sintetico, dedup e committed preservati, speculative disponibili ma non applicate, errori espliciti e byte delle generazioni precedenti intatti. Non dimostrati: compaction live, recupero di follower senza prefisso, InstallSnapshot, GC sicura, checker generale M14, RSS/CPU, durabilità contro reboot OS/perdita alimentazione o perdita SSD. Hash e Sync non sono una prova formale di consenso o atomicità fisica multi-file.

Budget aggiuntivo 0 €, completamente offline, sola libreria standard e toolchain/cache Go 1.27.4 darwin/arm64 preesistenti. GOMAXPROCS=2, package seriali, test a timeout esplicito, poche generazioni e dati piccoli; storia da 1024 client solo in memoria. Nessun cloud, GPU, VM/Docker, stress, benchmark intensivo o scrittura massiva. Solo directory dei test e cleanup controllato; nessuna directory effettiva del cluster modificata, nessun cluster lasciato attivo.

## Decisione e prosecuzione

**Fase D: ISOLATED COMPACTION PHASE COMPLETED. M13: PARTIALLY COMPLETED.** C-01 confermato, C-02/C-03 complessivi CONDITIONALLY READY, T-015/T-016/T-026 aperti. Nessun blocco conosciuto nel perimetro isolato completato. Un nuovo incarico può autorizzare InstallSnapshot e le prove di handoff/quote/epoche richieste, oppure ulteriori verifiche isolate; compaction permanente, GC e cancellazione prefissi non sono autorizzate dalla sola Fase D. M14 non iniziata. Nessun commit Git.
