# M03 — WAL locale e crash recovery

[Indice](README.md) · [English](../en/milestone-m03.md)

**20 novembre 2026 — COMPLETED nel perimetro ufficiale di storage locale M03. M04 NON INIZIATA.**

## Obiettivo e confine effettivo

La [roadmap](implementation-roadmap.md) richiede frame, hard state persistente e proprietà esclusiva, specifica binaria e cutpoint, con evidenze T-013 locali. C-01 ha fissato gli schemi; C-02 verificato le primitive APFS locali. Questo incarico completa quello strato storage locale usando codec/adattatore esistenti. Baseline Git dd4a3f67 pulita. M01/M02 e fixture approvate/go.mod invariati; nessuna dipendenza esterna, commit Git o implementazione successiva.

Implementati: bootstrap esplicito su directory vuota, proprietario WAL sincrono ordinato, reader streaming limitato, validazione atomica hard-state/log/commit fra frame, verifica generazione selezionata, recovery della coda finale, limiti fisici fissi e test deterministici di errore/processo. Il recovery si ferma al log. Non chiama Machine.Apply o applier, non stabilisce commit Raft, non crea sessioni né presenta risultati client. Riavvio server completo come follower, durabilità end-to-end, cattura/installazione/rotazione snapshot e compaction restano M09/M13 o lavoro successivo, come già collocato nella roadmap.

## Architettura e contratto pubblico

[internal/wal](../../internal/wal/doc.go) usa storageformat e storagefs, mai statemachine/applier. Bootstrap(root,Identity) richiede directory privata preesistente vuota e cluster_id/node_id/config_hash espliciti validi. Open(root,Identity) richiede metadati inizializzati e acquisisce lo stesso lock esclusivo interprocesso. Store serializza Append, AppendProposal e Close con un mutex; possiede un descrittore senza goroutine in background o coda illimitata di richieste. La futura integrazione consenso dovrà schedulare questo proprietario bloccante fuori dal lock del core Raft; tale adattatore non è implementato qui.

Append riceve payload transazione canonico, valida/codifica col codec QWAL esistente, calcola post-stato provvisorio, riserva spazio fisico, scrive il frame completo e chiama File.Sync. Soltanto dopo Sync riuscita installa il post-stato in memoria e restituisce Token{Generation,Sequence}. Il token identifica un effetto locale generazione/sequence fisica, non indice entry Raft, quorum o certificato commit. Input immutabili per tutta la chiamata. State restituisce copia separata; include entry committed e non committed. CommittedEntries copia soltanto il suffisso coperto dal checkpoint, senza essere percorso apply automatico.

Schemi/transizioni invalidi e quote di ammissione falliscono prima dell'I/O senza invalidare il proprietario o avanzare sequence. Write parziali continuano soltanto se la write sottostante riesce con conteggio corto. Errori write/Sync/ispezione chiudono il writer: token zero, stato acknowledged in memoria invariato, causa conservata, niente retry cieco o rollback. Append successivi restituiscono ErrFailed fino a close/reopen espliciti. Un frame completo fallito/non acknowledged può sopravvivere e comparire dopo recovery; un errore precedente non dimostra mai assenza dell'effetto.

Close attende Append serializzato, chiude WAL prima di rilasciare LOCK, conserva errori precedenti/close ed è ripetibile. Tutte le write accettate con successo erano già sincronizzate; Close non trasforma un append fallito incerto in acknowledgment riuscito. Le syscall OS non si possono cancellare forzatamente in sicurezza; deadline shutdown server completo restano lavoro futuro separato.

## Formato, metadati e ordine bootstrap

Nessun formato/fixture cambiato: header QWAL v1 transazione kind-1 di 24 byte big endian, JSON canonico limitato, sequence fisica da 1 e CRC32C su byte header 0..19 più payload. Validazione entry/comandi e digest tipizzati esistenti autorevoli. Transaction/HardState/Suffix/LogEntry decodificati sono viste tipizzate degli stessi byte, non altra codifica persistita. FrameSize controlla magic/versione/kind/lunghezza disponibili prima di allocare payload; campi contraddetti di header parziale sono corruzione/formato non supportato, non coda interrotta plausibile.

Validazione stato atomica: primo frame contiene esattamente hard_state e commit uguale alla base selezionata, senza sostituzione. In seguito sequence contigua senza wrap; termine non regredisce; voto esistente nello stesso termine non cambia né si cancella. Termine superiore può resettare voto. Sostituzione suffisso inizia strettamente oltre vecchio commit ed entro il confine esistente, conserva prefisso, richiede indici contigui e termini non decrescenti limitati dal currentTerm post-stato. Commit non regredisce o supera log dopo sostituzione. Nessuna transazione installa effetti parziali. Commit stabilito legalmente resta responsabilità del futuro chiamante; questi controlli strutturali non eseguono Raft.

Bootstrap ricontrolla directory vuota sotto LOCK per rifiutare race/tentativi parziali. Scrive IDENTITY con checksum e sincronizza file/radice; crea gen-1.tmp; scrive/sincronizza checkpoint QSN1 vuoto esatto approvato e transazione WAL iniziale; sincronizza directory staging; rinomina in gen-1 e sincronizza radice; sigilla/pubblica CURRENT tramite PublishCurrent esistente. Solo allora apre WAL selezionato e restituisce proprietario. L'encoder checkpoint supporta soltanto l'immagine bootstrap vuota obbligatoria, non snapshot generali. CURRENT non richiede riscrittura agli append ordinari perché il WAL referenziato è append-only. Nessun nuovo cursore persistente o campo applied-index.

Qualunque fallimento bootstrap conserva evidenze e rilascia proprietà. CURRENT mancante con altri file, inclusi residui solo LOCK, non avvia mai inizializzazione/resume impliciti. Se una chiamata fallita aveva già pubblicato CURRENT completamente, Open esplicita successiva può validarlo; Bootstrap rifiuta comunque sovrascrittura. Allocazione/rotazione generazioni di produzione non implementata oltre bootstrap vuoto iniziale.

## Reader e algoritmo recovery

1. Rifiutare storage non inizializzato; acquisire LOCK; validare IDENTITY canonica/con checksum rispetto all'identità fornita, poi CURRENT. Soltanto CURRENT sceglie generazione; niente directory maggiore o fallback a vecchi dati validi.
2. Validare generazione privata selezionata e identità/base/hash file completo del checkpoint vuoto esatto. Base snapshot nonzero esplicitamente non supportata fino a M13, mai interpretata come vuota o caricata parzialmente.
3. Conteggiare byte directory, aprire WAL privato esistente senza truncate e rifiutare file già eccessivo. Leggere streaming un header di 24 byte e al massimo un payload limitato per volta; mai allocare da lunghezza non verificata o leggere intero WAL in memoria.
4. Validare frame completi/checksum/schema e ogni post-stato provvisorio fra frame. Conservare in memoria solo suffisso vivo; non applicare comandi KV. Qualunque errore completo integrità/semantica/lettura non restituisce risultato parziale utilizzabile e fa fallire Open senza cambiare file.
5. Soltanto incompletezza strutturale EOF dopo prefisso iniziale interamente valido produce coda finale riparabile. Troncare all'offset verificato e sincronizzare file; errore Truncate/Sync fa fallire Open. WAL iniziale vuoto/incompleto, magic/versione/lunghezza errati, checksum completo errato, violazione sequence/voto/termine/commit o corruzione interna sono fatali. Mai cercare altro magic.
6. Risincronizzare frame completi sopravvissuti prima di esporre storage locale recuperato pronto, incluse write complete senza Sync/ack precedenti. Posizionarsi alla fine verificata e riprendere da sequence fisica successiva. Recovery ripetuto non duplica né riscrive frame validi.

Una lunghezza corrotta può imitare payload finale interrotto. La regola automatica della coda vale soltanto nel modello approvato crash processo/non corruzione arbitraria; non è riparazione bit rot generale. Nessuna policy deduce acknowledgment mancante o storia di guasti fisici da questi soli byte. Presente, sincronizzato, committed, applicato e acknowledged al client restano distinti. Store espone solo metadati log; coordinamento apply/dedup/noop/epoch dovrà usare l'architettura applier.

## Limiti fisici e policy errori

Cap WAL assoluto 8 MiB. AppendProposal ammette nuove proposte solo sotto tale cap meno riserva consenso 512 KiB; Append è percorso replica/controllo e può usare riserva. I futuri chiamanti consenso dovranno classificare correttamente l'ammissione. Cap inclusivo directory nodo 32 MiB, conteggiando file vecchi/staging/controllo, con almeno 64 KiB di margine controllo prima della crescita. Nessun prefisso necessario eliminato per fare spazio. Senza compaction M13 il raggiungimento cap impedisce deliberatamente crescita; disponibilità a quota esaurita non promessa.

Il contatore directory usa lunghezze logiche file, non garantisce allocazione fisica APFS, altre app o scritture SSD. Ispezione directory fallita è fatale; rifiuto quota noto resta riprovabile senza mutazione file. Percorsi/file privati e controlli no-follow riusano adattatore verificato. Mutazioni esterne non cooperanti, corruzione OS/SSD e perdita fisica fuori dal contratto crash processo.

## Test, evidenze e limiti

Test coprono categorie A–O richieste: append/reopen/ordine reali; byte esatti bootstrap/frame/immagine/manifest fixture; ogni taglio di un piccolo frame finale; checksum completi/interni errati; formati non supportati; write parziale/Sync fallita con token zero; repair/recovery ripetuti; restart figlio normale/forzato; proprietà esclusiva; CURRENT autorevole senza fallback da generazione corrotta; cutpoint pubblicazione bootstrap; regressioni componenti precedenti.

Tutti i 18 scenari recovery approvati hanno ora evidenze locali: 13 casi immagine byte passano da Replay, cinque aspettative namespace nei test bootstrap/manifest. Sette cutpoint bootstrap nominati preservano directory esplicitamente incompleta oppure pubblicazione interamente verificabile. I test filesystem esistenti coprono errori prima/dopo rename CURRENT. Il nuovo harness figlio copre write completa prima di Sync (nessun commit dedotto), dopo checkpoint sincronizzato e close ordinaria. Readiness sincronizzata via pipe, deadline figlio 5 s, soltanto PID del run uccisi/attesi. Voti recuperati controllati esplicitamente e secondo voto nello stesso termine rifiutato dopo restart. Niente stress casuale o sincronizzazione time.Sleep.

Piccoli wrapper iniettano write parziale/ENOSPC, Sync/EIO fallita, errore close, Truncate/Sync repair fallite, errori lettura e ispezione/quote directory. Test quote simulano cursori/uso invece di allocare file 8/32 MiB o riempire SSD. Copie stato separate mutate nei test; Append/Close concorrenti attesi e controllati con race detector.

Eseguiti con Go 1.27.4 darwin/arm64, macOS 27.2.1 build 26B131, APFS locale registrato dalla suite regressione filesystem. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; ordinari CGO_ENABLED=0, race CGO_ENABLED=1 con compilatore esistente. Comandi equivalenti con toolchain installata:

```sh
gofmt -l internal
go vet ./...
go test -json -p 1 -count=1 -cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/wal ./internal/storagefs ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Evidenza finale effettiva | Esito |
|---|---|
| gofmt / go vet | Superati. |
| Suite WAL ordinaria | 17 funzioni principali incluso harness figlio, 25 sottocasi superati; 82,9% statement coverage. |
| Componenti precedenti | Superati: M01 12/44, M02 21/0, storageformat 7/18, storagefs 13/16 (principali/sottocasi). |
| Copertura misurata locale ai package | statemachine/applier 100,0%; storageformat 81,7%; storagefs 80,1%. Nuovi helper esercitati dai test integrazione WAL non sono strumentati nei binari test dei rispettivi package con semplice -cover. |
| Race | WAL, storagefs e applier superati, incluso rerun WAL finale dopo ultima modifica; nessuna race segnalata nei percorsi esercitati. |
| Fixture / dipendenze | Aritmetica fixture originale superata; fixture, go.mod e M01/M02 invariati; lista moduli contiene soltanto modulo radice. |

Test in directory temporanee private possedute, piccoli record sintetici, dataset limitati e deadline package <=30 s, suite ordinaria sotto 120 s. Niente file giganti allocati, benchmark, stress prolungato, ENOSPC reale, crash OS, interruzione alimentazione, operazione disco privilegiata, servizi cloud/API, software a pagamento, nuovo hardware o installazione. Nessuna misura RAM/CPU/SSD o prova universale di durabilità inventata. Copertura non dimostra tutti gli errori I/O/schedule. Evidenze token durevole/recovery locale non provano maggioranza durevole distribuita o linearizzabilità client; T-023 end-to-end resta M09 e T-015/T-016 snapshot M13.

## File, accettazione e readiness M04

Creati internal/wal/doc.go, state.go, reader.go, store.go, open.go, bootstrap.go; test reader_test.go, store_test.go, namespace_test.go, process_test.go. Aggiunti storageformat/transaction.go (viste tipizzate validate), bootstrap.go (sola immagine vuota), storagefs/files_darwin.go (aperture controllate/letture limitate); esteso minimamente storageformat/frame.go per lunghezza header streaming e aggiornato il commento ownership di storageformat/doc.go. Riutilizzate serializzazione, Sync/lock e pubblicazione CURRENT originali. Nessuna modifica M01/M02.

Aggiornate relazioni M03 bilingui e indici pertinenti, contratti architettura/storage/formati/macOS, modello guasti/test, ADR-005, roadmap, tracciabilità/accettazione e rimandi audit/relazione preliminare storici. Cifre storiche etichettate per fase. C-01 RESOLVED e C-02 VERIFIED conservano esattamente i propri limiti. T-013/M03 locale e parte storage AC-013 Passed; claim completi risposte Raft, recovery server e guasti fisici non chiusi anticipatamente.

**Stato finale: COMPLETED per M03.** Presenti tutti i deliverable locali ufficiali: persistenza con frame, recovery hard-state/log/commit, proprietà esclusiva, coordinamento bootstrap/manifest, crescita limitata ed evidenze errori. Nessun blocco M03 irrisolto. Prerequisito storage M04 pronto per revisione; fixture configurazione/hash/protocollo e gate M04 restano da affrontare in nuovo incarico. M04 non iniziata. Budget aggiuntivo **0 €**; nessun commit automatico. Fermarsi a M03.

Verifica documentale: 112 file Markdown, 55 coppie linguistiche, 1309 link relativi, 98 tabelle e 19 coppie identiche di esempi tecnici non Mermaid; zero errori strutturali. Controllati link reciproci, titoli/ID, esempi JSON e fence codice. Traduzione semantica rivista localmente; nessuna certificazione esterna o rendering Mermaid. Aggiornati README.md/docs/README.md condivisi e, per lingua, README.md, architecture.md, canonical-formats.md, storage-and-recovery.md, failure-model.md, adr/ADR-005-persistence-strategy.md, implementation-roadmap.md, testing-strategy.md, acceptance-criteria.md, traceability.md, architecture-review.md, macos-storage-contract.md e milestone-m03-preliminary.md, insieme a questa relazione.
