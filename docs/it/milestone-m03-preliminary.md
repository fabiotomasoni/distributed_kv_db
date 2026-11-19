# M03 — Validazione preliminare dello storage

[Indice](README.md) · [English](../en/milestone-m03-preliminary.md)

**19 novembre 2026 — GATE C-02 VERIFIED per il contratto della piattaforma locale provata. M03 complessiva: PARTIALLY COMPLETED.**

## Ambito ufficiale e autorizzazione

La [roadmap](implementation-roadmap.md) approvata definisce M03 come persistenza locale: frame, hard state e lock, specifica binaria e cutpoint, con T-013 nel sottoinsieme di fase. Questo incarico autorizza soltanto il primo passo [pre-M03](pre-m03-review.md): conformità canonica e test isolati filesystem/lock/barriere/pubblicazione. Non autorizza writer WAL completo, motore recovery, snapshot o milestone successive.

Esaminati baseline Git f16f5bb, formati/fixture effettivi, contratti dei due gate, ADR-005/ADR-007, requisiti/criteri storage e interfacce M01/M02. Le modifiche .DS_Store e internal/.DS_Store erano presenti all'ingresso e sono lasciate intatte. M01/M02, go.mod e fixture approvate invariati. Nessun commit Git.

## Componenti e contratti

[internal/storageformat](../../internal/storageformat/doc.go) è senza stato e senza dipendenza filesystem. Valida JSON ASCII canonico, campi/nullabilità esatti, forma/limiti scalari, domini chiave/valore e union entry. EncodeFrame/DecodeFrame implementano header QWAL approvato di 24 byte big endian, versione/kind, payload limitato, campo sequence fisica e CRC32C. Decode restituisce copia privata del payload e dimensione consumata di un frame; input incompleto produce ErrIncomplete senza effetti. Tale errore non autorizza mai da solo il troncamento. CommandDigest coincide con preimmagini QCMD tipizzate e SHA-256 golden; SealManifest/ValidateManifest coincidono con byte canonici e checksum CURRENT/IDENTITY.

EncodeFrame richiede una transazione JSON già canonica e valida nei tipi, senza normalizzare silenziosamente input ambiguo. Valida indici contigui intra-frame, termini non decrescenti, cap batch/byte e limite termine corrente se fornito. Non può validare voti precedenti, suffissi già committed, continuità sequence bootstrap, identità/configurazione referenziata o commit Raft legale senza un proprietario storage. Nessun replay/riparazione con stato implementato. Codec snapshot/risultati/sessioni restano futuri; vettori immagine esistenti verificati dal controllore aritmetico originale e piccolo harness di test, non da un motore snapshot di produzione.

[internal/storagefs](../../internal/storagefs/doc.go) separa operazioni Darwin/APFS dai formati. Acquire usa inode LOCK privato stabile, O_NOFOLLOW/O_CLOEXEC, flock esclusivo non bloccante e proprietà del descrittore per tutta la vita. Rifiuta permessi insicuri, symlink/file speciali e radici non APFS; distingue contesa dagli altri errori. Close è serializzato con pubblicazione, idempotente e non elimina LOCK. Mutazioni esterne/writer non cooperanti restano fuori contratto. Escludere cartelle sincronizzate cloud è precondizione del chiamante, non una capacità dichiarata dall'adattatore.

WriteAll continua write corte riuscite, rifiuta assenza di progresso/conteggi invalidi e termina su errore parziale senza riprovare l'intero append. File.Sync precede Close controllata. SyncDirectory usa fsync directory Darwin, riprovando solo EINTR e conservando errori close. PublishCurrent valida <=4096 byte immutabili prima dell'I/O, crea CURRENT.tmp esclusivamente, scrive/sincronizza/chiude, rinomina e sincronizza il parent. Dati invalidi non toccano staging. Fallimento I/O invalida il proprietario fino a chiusura/riavvio espliciti, preservando evidenze. Dopo il tentativo rename un errore è prudenzialmente ErrUncertain; niente rollback, cleanup automatico o falso successo. La validazione del chiamante deve terminare, non rientrare e non mutare input.

PublishCurrent è soltanto una primitiva di file controllo. Il chiamante deve prima validare/sincronizzare la generazione referenziata. I test preparano piccole copie dello snapshot vuoto/frame bootstrap approvati e verificano la generazione scelta da CURRENT; nessun costruttore generazioni, cattura snapshot, compaction, listener o worker storage collegato a M01/M02 in produzione.

## Test implementati ed esiti reali piattaforma

| Area | Evidenza |
|---|---|
| Formati esatti | Due frame golden encode/decode/round-trip byte per byte; sei digest entry e due seal manifest coincidono con fixture invariate. |
| Input invalidi | Rifiutati ogni taglio di un piccolo frame, CRC/payload/magic errati, versione/kind sconosciuti, lunghezza zero/eccessiva, campi non canonici/duplicati/ignoti, errori null/tipi/numeri/Base64 e confini entry/batch malformati. CAS massimo ancora esattamente 44184 byte. |
| Proprietà lock | Aperture separate e processo figlio indipendente rifiutano secondo proprietario; inode stabile conservato. Close normale figlio e kill dal parent rilasciano lock; readiness/uscita sincronizzate, mai sleep arbitrari. |
| Primitive piattaforma | File.Sync regolare, un probe esplicito capacità F_FULLFSYNC e fsync directory riescono. Modi file 0600 e radice 0700 verificati; verificato close-on-exec. |
| Pubblicazione | Manifest vecchio/nuovo completi; nessuno staging residuo dopo successo; due cutpoint reali figlio prima/dopo rename. CURRENT sopravvissuto riferisce immagine fixture completa con checksum e frame bootstrap valido. Nessun fallback o successo anticipato. |
| Percorsi errore | Iniettati permesso negato, ENOSPC, EIO, rename fra device ed errore directory non supportata; propagati errori close file/directory. Ordine completo delle operazioni verificato; errore dopo rename incerto anche se nuovi byte visibili. |
| Cleanup e concorrenza | Validazione fallita, staging conservato, proprietario fallito/chiuso, rifiuto symlink/file speciali, percorsi mancanti e Close/Publish concorrenti. Ogni figlio atteso; interessati soltanto PID/directory temporanee del run. |

Piattaforma registrata: **macOS 27.2.1, build 26B131, darwin/arm64, Go 1.27.4**. Statfs delle radici test e ispezione progetto in sola lettura riporta **apfs**, mount **/System/Volumes/Data**, device **/dev/disk3s5**. Scritture reali soltanto in t.TempDir private sotto la radice temporanea di sistema, mai dati nodo/utente. Nessuna asserzione su altri mount o versioni macOS.

Il primo run di sviluppo ha rifiutato i permessi iniziali della directory temporanea; il harness imposta ora esplicitamente 0700. Il contratto non è stato allentato. Run finali superati. Il sorgente Go File.Sync era già verificato: richiede F_FULLFSYNC con fallback ENOTSUP; il probe diretto separato ne verifica la disponibilità su questo mount, non il comportamento delle cache fisiche o la sopravvivenza a power loss.

## Verifiche eseguite e limiti

Toolchain/cache temporanee verificate, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2. Test ordinari CGO_ENABLED=0; race mirato CGO_ENABLED=1 con compilatore Apple esistente. Comandi equivalenti con toolchain installata:

```sh
gofmt -l internal
go vet ./...
go test -json -p 1 -count=1 -cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/applier ./internal/storagefs
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

| Verifica finale | Esito |
|---|---|
| gofmt e go vet | Superati. |
| storageformat | 7 test principali, 18 sottocasi superati; 90,8% statement coverage. |
| storagefs | 13 funzioni principali incluso harness figlio, 16 sottocasi superati; 89,0% statement coverage. |
| Regressioni M01/M02 | Superate: statemachine 12 test/44 sottocasi, applier 21 test; entrambi copertura 100,0%. Sorgenti invariati. |
| Race mirato | Superato per applier/storagefs; nessuna race segnalata nei percorsi esercitati. |
| Aritmetica fixture/dipendenze esistenti | Superata; fixture approvate/go.mod invariati, nessun modulo esterno, installazione o servizio. |

La suite finale usa due cutpoint reali di pubblicazione; ripetizioni limitate di sviluppo/finali rimaste nel budget di fase di 16 cutpoint. Ogni caso con file reali scrive solo piccole fixture/manifest, molto sotto 16 KiB. Un limite conservativo da dimensioni contenuti/numero invocazioni per la fase è inferiore a 128 KiB di contenuti file sintetici deliberatamente forniti, sotto il budget 1 MiB; è aritmetica del carico, non misura SSD/amplificazione interna. Deadline figli 5 s, package 30 s, massimo parent+un figlio contemporanei, suite ordinaria sotto 120 s. Niente stress, benchmark, scritture massive, disco riempito, flush cache, privilegi, VM/container, GPU o test fisici di guasto. Contenuti temporanei rimossi soltanto dal cleanup del test proprietario.

Race/copertura non provano tutti gli schedule o errori I/O. Terminazione subprocessi prova kernel vivo e namespace filesystem visibile, non crash OS o power loss. I 18 scenari recovery salvati restano specifiche: replay con stato di voti/commit/suffissi e troncamento finale attendono la fase WAL effettiva. AC-013/AC-014/AC-023 completi e AC-015/AC-016 snapshot non chiusi.

## Decisione gate e M03 residua

**GATE C-02 VERIFIED** per scegliere questo contratto di primitive locali Darwin/APFS: mount/permessi, lock cooperativo interprocesso e rilascio, barriere file/directory, sostituzione sincronizzata, cutpoint finiti e gestione esplicita errori I/O hanno evidenze concrete. Risolve il gate piattaforma pre-integrazione dell'audit. L'obbligo di conformità reader della tabella pre-M03 ha evidenza per il sottoinsieme autorizzato senza stato frame/manifest/byte bootstrap; accettazione recovery con stato resta T-013 nel resto di M03 come richiede la roadmap. Nessun requisito derogato o trasformato in garanzia power loss. Progetto formato C-01 resta risolto; implementato soltanto il sottoinsieme codec provato.

**M03 PARTIALLY COMPLETED**: lavoro futuro autorizzato separatamente potrà implementare proprietario/writer WAL limitato, validazione transizioni hard-state/log/commit, token persistenti con generazione, bootstrap/riavvio, quote e test T-013 completi di recovery/cutpoint. Non dedurre commit da lunghezza log o collegare M01/M02 a storage parzialmente validato. Cattura/installazione/compaction snapshot, claim client di maggioranza durevole, Raft, RPC, HTTP e milestone successive restano bloccati ai propri gate. Completamento M03 richiede le evidenze residue, non soltanto fsync riuscito.

Creati storageformat/doc.go, canonical.go, entry.go, frame.go, manifest.go e format_test.go; storagefs doc.go, write.go, fs_darwin.go, write_test.go e fs_darwin_test.go. Aggiornati questa relazione bilingue di fase, indici/roadmap correnti, specifiche canoniche/storage/macOS, ADR-005, appendici audit/gate/questioni, test/accettazione/tracciabilità e rimandi della revisione storica. Le due lingue condividono formati, API, limiti, evidenze e gate residui. Costo aggiuntivo **0 €**, solo Mac esistente. Nessun commit automatico o prosecuzione oltre questa fase; attendere nuovo incarico.

Documenti modificati per ciascuna lingua: README.md, acceptance-criteria.md, adr/ADR-005-persistence-strategy.md, architecture-review.md, canonical-formats.md, design-review.md, implementation-roadmap.md, macos-storage-contract.md, pre-m03-review.md, storage-and-recovery.md, testing-strategy.md e traceability.md, oltre a README.md/docs/README.md condivisi e questa nuova relazione di fase. Controlli strutturali superati: 110 file Markdown, 54 coppie linguistiche, 1269 link relativi, 96 tabelle e 18 coppie identiche di esempi non Mermaid; nessun errore strutturale. Nessun rendering Mermaid o certificazione esterna della traduzione.
