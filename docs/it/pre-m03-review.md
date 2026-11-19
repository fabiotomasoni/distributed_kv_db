# Revisione M02 e gate prima di M03

[Indice](README.md) · [English](../en/pre-m03-review.md)

**19 novembre 2026 — CONDITIONALLY READY. M03 NON INIZIATA.**

## Ambito, baseline e decisione

Esaminato il contenuto reale del repository alla baseline Git 9a73963, pulita all'ingresso, non soltanto i resoconti precedenti. Ambito: revisione concorrenza M02, test mirato del ramo mancante, progetto formati canonici e fonti storage macOS. Nessun file Go di produzione o sorgente M01 modificato; nessun componente WAL/storage/consenso/rete implementato. Nessun commit automatico.

M02 resta COMPLETED nel sottoinsieme locale approvato. Nessun difetto di produzione dimostrabile trovato nei percorsi esaminati. Il progetto formati storage C-01 è RESOLVED con schemi fissati e fixture byte verificate. La parte fonti/scelte C-02 è documentata; la validazione effettiva dell'adattatore è OPEN. Pertanto **CONDITIONALLY READY**, non READY FOR M03: soltanto conformità formati e test isolati dell'adattatore piattaforma sono proponibili all'inizio. L'integrazione writer/pubblicazione storage deve attendere le evidenze specificate. Il lavoro futuro richiede ancora un proprio incarico; questa revisione non inizia M03.

## Revisione M02 e interleaving

| Proprietà esaminata | Esito ragionato / evidenza |
|---|---|
| Unico proprietario Apply | New avvia un worker run; soltanto questo invoca Machine.Apply, fuori dal mutex dei metadati. Proprietà esclusiva della macchina resta precondizione del chiamante. |
| Indici ordinati / duplicati / buchi | Ammissione sotto un mutex accetta soltanto Scheduled+1; indici rifiutati non avanzano il cursore. Arrivo concorrente diverso da ordine log; produttore conserva i buchi per retry ordinato. |
| Limite coda / backpressure | Ring fisso 1..1024, default 64, più un comando attivo; input grezzi copiati <=48 KiB. Submit sospende su changed/context; TrySubmit rifiuta pieno senza perdita silenziosa. |
| Risultati correlati / ownership | Ogni entry accettata possiede una Future/indice; copie comandi e risultati per Wait preservano proprietà. Nessun ricevente necessario per finish. |
| Cancellazione contro ammissione | Entrambi i controlli context precedono enqueue sotto mutex. Prima dell'accettazione non si consumano indici; dopo, il context limita solo l'attesa. La competizione con enqueue può legittimamente restituire entrambi gli esiti. |
| Coda piena contro chiusura | Produttore in attesa rilascia mutex e cattura changed sotto lock; Shutdown cambia stato e notifica sotto lo stesso lock. I produttori risvegliati ricontrollano stato prima dell'ammissione. Nessun risveglio perso a quel confine. |
| Apply contro shutdown | Shutdown serializza chiusura ammissione, non interrompe Apply. Comando attivo e coda pendente drenati; waiter ripetuti/concorrenti osservano Done o errore del proprio context. |
| Chiusura canali | changed chiuso/sostituito soltanto sotto mutex; un worker chiude Done, un completamento scrive/chiude ogni Future. Nessuna send compete con close di un canale lavoro. |
| Progress contro completamento | LastApplied avanza dopo risultati di dominio, Scheduled all'ammissione. Una vista progress può precedere brevemente la pubblicazione Future; soltanto Done è il confine del drenaggio completo. |
| Panic / terminazione | Panic chiude: effetto corrente incerto, pendenti esplicitamente abortiti; niente retry. Dipendenza che non termina/rientra/usa Goexit viola il contratto e non può essere corretta forzatamente dalla cancellazione. |
| Confini leak/deadlock | Test esistenti osservano Done worker e join/canali produttori, incluso shutdown inattivo/attivo; nessun worker per richiesta o blocco di invio risultato. Evidenza di percorsi finiti, non prova esaustiva di scheduling. |
| Invarianti M01 | Core invariato; suite regressione validazione, quote, CAS sequenziale, assenza/vuoto e copie difensive superata. Nessun uso concorrente diretto autorizzato. |

Non generalizzare il rifiuto indici M02 in deduplicazione client replicata o dedurre commit dall'ammissione. Integrazione futura noop/sessioni/epoch esplicitamente fuori dall'applier KV corrente, dettagliata nei [formati](canonical-formats.md). I piccoli fake transport/storage sono confini del harness, non prova di integrazione rete/persistenza.

## Ramo mancante, modifiche e verifiche eseguite

Profilo baseline riprodotto: **99,3%** per applier. L'unica istruzione con conteggio zero era il return per cancellazione in applier.go dopo cloneCommand e prima di inserire nel ring. È significativo: la cancellazione può avvenire durante la copia anche con spazio immediato.

Aggiunto TestT018CancellationAtAdmissionRecheck in [lifecycle_test.go](../../internal/applier/lifecycle_test.go). Un wrapper di test cancella un vero context al secondo controllo Err, senza sleep o hook di produzione. Verifica nessuna Future, nessun avanzamento coda/progress e nessuna chiamata Apply per la copia rifiutata, poi riusa con successo indice 1. Chiude una lacuna di test, non un bug di produzione scoperto; l'implementazione era già corretta. Codice di produzione invariato.

Eseguiti con toolchain verificata Go 1.27.4 darwin/arm64 in /tmp, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; test ordinari CGO_ENABLED=0, race CGO_ENABLED=1 con Apple clang esistente. Comandi equivalenti dalla radice con toolchain installata:

```sh
gofmt -l internal
go vet ./...
go test -p 1 -count=1 -coverprofile=/tmp/quorumkv-pre-m03-after.cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/applier
python3 docs/fixtures/verify_canonical_v1.py
```

| Verifica effettiva | Esito |
|---|---|
| Suite baseline completa M01/M02 con copertura | Superata; M02 20 test, copertura 99,3%; M01 12 test/44 sottocasi, 100%. |
| gofmt | Nuovo test formattato; gofmt -l internal finale vuoto. |
| go vet ./... | Superato. |
| Suite finale completa M01/M02 | Superata, un run; M02 ora 21 test, M01 invariata 12/44. Entrambi i package 100,0% statement coverage; nessuna istruzione scoperta nel profilo finale. |
| Race mirato, un run, timeout 30 s | Superato; nessuna race segnalata nei percorsi M02 esercitati. |
| Aritmetica fixture statiche | Superati byte canonici, sei digest/risultati, due frame CRC32C, due trailer/hash file immagine, manifest e CAS massimo 44184 byte. |
| Controverifica indipendente | Verificatore Go temporaneo con libreria standard ha confermato indipendentemente i due CRC32C e i due SHA-256 trailer/file snapshot. Nessuna API storage esercitata. |
| Dipendenze / ambito | go.mod e M01/Go produzione invariati, nessun modulo esterno o installazione. |

Nessun benchmark pesante, test carico, misura RAM/CPU/SSD, test adattatore storage, crash processo reale, crash OS o interruzione alimentazione. Copertura e race sono evidenze finite. Il verificatore fixture controlla aritmetica e mutazioni selezionate; le 18 aspettative recovery e sei JSON invalidi sono specifiche per futuri parser/reader, non test di recovery eseguiti. Controlli bilingui strutturali su link/tabelle/fence/parità ID/esempi; revisione manuale traduzione senza certificazione esterna.

## Requisito originale C-01 e risoluzione

L'[audit originale, sezione E](architecture-review.md) richiede, prima di fissare l'implementazione WAL/snapshot M03, schema canonico byte-per-byte, kind/nullable, bootstrap, checksum e fixture recovery; completare fixture configurazione/protocollo a M04 e OpenAPI prima di stabilità M08. Non richiede un codec M03 completo prima di M03.

Erano aperti: canonicalizzazione JSON e forma numerica; oggetti esatti comando/risultato/sessione/snapshot; opzionale contro null; byte digest tipizzato; header completo/ambito checksum; transazione iniziale bootstrap; schemi generazione/file controllo; recovery/troncamento ammissibili contro corruzione. [Formati canonici v1](canonical-formats.md) fissa queste decisioni senza cambiare semantica M01 o confondere HTTP e persistenza. Conservati WAL transazionale CRC32C e framing snapshot SHA-256 esistenti. Limiti fissati compatibili con entry Raft e sessioni future; modifiche incompatibili richiedono revisione versione/migrazione.

Controlli chiusura C-01: regole schema/dominio/versione complete; kind/nullable espliciti; semantica bootstrap/current/generazione fissata; aspettative byte statiche positive/negative presenti; aritmetica e digest indipendenti verificati; conformità futura obbligatoria collegata a T-013/T-015/T-016. **RESOLVED per il prerequisito progettuale storage.** Accettazione codec/recovery resta aperta alle milestone implementative. Costruzione digest configurazione/completezza fixture protocollo resta M04 e OpenAPI M08 come nel gate originale; nessun protocollo/AC completo chiuso anticipatamente.

## Requisito originale C-02 e stato per fasi

L'[audit originale, sezione E](architecture-review.md) richiede, prima di scegliere contratto adattatore M03 e chiudere durabilità M03/M09, verifica documentazione e scelte lock/Sync/rename su macOS ARM64; validazione leggera adattatore durante implementazione futura, nessun test distruttivo o promessa di durabilità dal solo nome Sync.

Il [contratto storage macOS](macos-storage-contract.md) registra fonti Apple primarie e sorgente Go effettivo, inode stabile per lock advisory, sincronizzazione file regolari, barriera directory controllata separatamente, pubblicazione seriale nello stesso filesystem e gestione errori fatali/incerti. Distingue stop normale, crash processo, crash OS, power loss e corruzione. Nessuna sincronizzazione richiesta per safety rimossa. [ADR-005](adr/ADR-005-persistence-strategy.md) registra formalmente questo candidato per fasi; non serve nuova strategia architetturale o ID ADR.

**CONDITIONALLY RESOLVED per scelte documentate; OPEN per validazione implementativa.** Mancano: risultati reali mount/capacità, Sync directory, lock/rilascio fra processi, cutpoint sostituzione, errori write/Sync/close iniettati e conformità recovery storage. Il contratto indica responsabile (sviluppatore), milestone e condizioni per ciascuno. In M03 consentire prima soltanto conformità fixture e test isolati adattatore: <=16 KiB/caso, <=16 cutpoint, <=2 processi del run, <=1 MiB totale di scritture intenzionalmente sottomesse, <=30 s/caso e <=120 s suite. Nessun device o file utente alterato. Prima dell'integrazione writer/pubblicazione richiedere evidenze primitive/lock/errori/visibilità, rivedendo/bloccando in caso di fallimento. Chiusura M03 richiede T-013; durabilità M09 T-023; handoff/installazione snapshot ancora M13 T-015/T-016. Garanzie OS/power loss restano escluse. Segue il requisito di test per fasi dell'audit, senza rinominare un gate insoddisfatto in approvazione incondizionata.

## Questioni aperte, documenti e readiness finale

OQ-01 conserva la prova implementativa pendente; OQ-05 risolta soltanto per byte storage, con deliverable M04/M08 pendenti. C-03 consenso/dedup/snapshot/checker resta aperto; soltanto il sottoinsieme locale apply-backlog ha evidenza. Timing/risorse, resilienza fisica e licenza restano ai gate esistenti. Nessuna evidenza giustifica chiusura completa AC-013/AC-023.

Creati canonical-formats.md, macos-storage-contract.md e questa relazione in entrambe le lingue; fixture condivise canonical-v1.json e verify_canonical_v1.py di sola lettura. Modificati soltanto test ramo mancante e indici pertinenti, link formati dati/API/peer/Raft, specifica storage, appendici ADR-005/ADR-007, registro gate/audit/questioni, requisiti/accettazione/tracciabilità/roadmap/evidenze test. La relazione M02 originale resta storica con rimando alla revisione. Italiano e inglese specificano stessi byte, limiti, stati ed evidenze; esempi tecnici condividono la fonte fixture.

File condivisi modificati: README.md e docs/README.md. Per ciascuna lingua: README.md, acceptance-criteria.md, adr/ADR-005-persistence-strategy.md, adr/ADR-007-client-retry-semantics.md, api-specification.md, architecture-review.md, data-model.md, design-review.md, implementation-roadmap.md, milestone-m02.md, network-protocol.md, raft-consensus.md, references.md, requirements.md, storage-and-recovery.md, testing-strategy.md e traceability.md. Unica modifica Go: internal/applier/lifecycle_test.go. Validazione strutturale: 108 file Markdown, 53 coppie linguistiche, 1227 link relativi, 92 tabelle e 17 coppie identiche di esempi tecnici non Mermaid, zero errori strutturali rilevati.

**Esito finale: CONDITIONALLY READY.** M02 verificata nel perimetro locale; progetto C-01 fissato; C-02 richiede ancora evidenze dell'adattatore reale. Un futuro incarico esplicito M03 potrà partire soltanto dal lavoro limitato conformità/capacità sopra e superare il gate interno solo con successo registrato. Nessuna autorizzazione indiscriminata per durabilità, snapshot, cluster o deploy; nessuna implementazione M03 in questa consegna. Costo aggiuntivo 0 €, niente servizi/acquisti/hardware nuovo, soli dati sintetici piccoli e verifiche limitate sul Mac esistente. Consultazione fonti via rete in questa revisione; test locali e verifiche fixture senza dipendenza da rete/servizi.

## Evidenze successive M03 preliminare

[Relazione di fase](milestone-m03-preliminary.md): GATE C-02 VERIFIED per primitive locali, M03 PARTIALLY COMPLETED. Passano 7 test storageformat/18 sottocasi e 13 funzioni test storagefs/16 sottocasi; superati race mirato e regressioni M01/M02 invariate. Ambito codec: frame/entry/digest/manifest senza stato; nessun motore WAL/replay o chiusura completa AC-013/AC-014/AC-023. Gate snapshot e successivi aperti. Cifre/stati precedenti descrivono la fase storica pertinente.
