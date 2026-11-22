# M12 — Revisione M11, fault injection e osservabilità

[Indice](README.md) · [English](../en/milestone-m12.md)

**22 novembre 2026 — COMPLETED. M13 NON INIZIATA.**

## Perimetro ufficiale e gate

Entrambe le [roadmap](implementation-roadmap.md), lette integralmente prima delle modifiche, assegnano M12 a fault injection e osservabilità: scenari/replay, log/metriche e manifest riproducibile. Requisiti FR-017/AC-017/T-017, FR-020/AC-020/T-020, NFR-008/AC-028/T-028. M11 rimane CAS/sessioni v2; M13 possiede snapshot, installazione e compattazione, M14 il checker generale indipendente, M15 le misure. Nessuna riassegnazione dei deliverable o nuova funzionalità M13. C-01 mantiene codec/fixture canoniche e API esistenti; C-02 resta il contratto locale Darwin/APFS verificato; C-03 snapshot/checker rimane aperto ai gate originali. ADR-008 accettato nel profilo limitato descritto qui. Nessun nuovo gate bloccante emerso.

## Revisione M11 e compatibilità

Baseline Git pulita 18a2932, commit M11 già presente all'ingresso. Rivisti relazioni bilingui, OpenAPI/fixture, ADR, consistenza, API, storage/recovery e interfacce reali. Suite M01–M11 passata prima delle modifiche. CAS percorre append, replica, quorum corrente, checkpoint e M02; confronto/modifica rimangono una transizione M01 e risultato esatto. Nessun GET+PUT alternativo o risposta definitiva dalla cache di un leader isolato. Risultato CAS negativo conservato; duplicati in volo decisi dal wrapper M02. not_admitted/unknown e autorizzazione termine/ruolo/commit/apply invariati.

Nuova prova mirata avanza sequenze 1..12, ripete 12 con stesso comando/indice, rifiuta 11 come STALE_SEQUENCE, 14 come SEQUENCE_GAP e 12 con altro payload come REQUEST_CONFLICT. Sequenza 13 esegue CAS fallito e ne conserva l'esito. Riempie 1024 client, rifiuta il 1025esimo senza eviction e verifica ancora il risultato del primo client; ripete l'intera storia su wrapper/macchina nuovi confrontando ogni risultato/indice. Regressioni mantengono client distinti, duplicati concorrenti, failover, suffisso speculativo escluso, risposta persa, crash follower/leader/tutti i processi e retry del CAS fallito dopo cambio stato.

Profilo persistente v2 invariato: configurazione/hash e IDENTITY v2 obbligatori, nuove directory e bootstrap esplicito, apertura incrociata rifiutata prima di riparare WAL/replay. Fixture temporanee verificano v1 senza toccare dati personali. Il client verifica header di conferma v2; v1 mantiene M10 e nessuna migrazione automatica. Limite sessioni e ultimo risultato sono scelte approvate, non bug da eliminare con TTL. Nessun bug safety CAS/dedup riprodotto. Il race finale ha esposto la finestra preesistente di pubblicazione progresso descritta sotto e corretta senza cambiare algoritmo o formati.

## Simulatore e responsabilità

internal/faultsim e il binario separato cmd/quorumkv-sim forniscono un profilo sintetico offline, senza socket o directory dati. Il coordinatore possiede tre core Raft invariati, PRNG xorshift64-v1, queue ordinata (tempo virtuale,sequenza), immagini WAL e storia tentativi. M02 reale possiede le transizioni M01/sessioni; ogni riavvio ricrea questi stati dal prefisso checkpointed. Persistenze sincrone conservano la responsabilità del driver corrente, con fault controllati e nessun token prima del successo. Requisiti, schemi, ownership, errori e limiti sono specificati nei [fault](fault-injection.md).

Azioni limitate: ritardo/drop/duplicate/reorder, partizione direzionale/bidirezionale e heal, timeout RPC, crash/restart, short write header/payload, ENOSPC prima della scrittura, Sync fallita WAL/checkpoint. Nessuna forzatura di termine/ruolo. Richieste e risposte ritardate mantengono correlazione; risposte già in rete possono attraversare un riavvio. Le due scelte astratte SystemCrashModel all/synced non sono simulazioni reali di crash OS/alimentazione; ProcessCrash conserva scritture completate anche non sincronizzate. Regole, cutpoint e sopravvivenza sono nel manifest; snapshot/rename M13 rifiutati.

Ghost committed permanente, registro voti e leader per termine, controllo Log Matching e risultati per indice preservati attraverso restart. Dopo ogni evento scheduler si controllano commit/apply e log; ogni immagine passa il validatore WAL reale e ogni risultato M02 è confrontato con quello già osservato allo stesso indice. La suite M11 mantiene inoltre un modello logico indipendente. Il nuovo runner non è il checker generale di linearizzabilità M14 e non costituisce prova formale.

## Manifest, replay ed esiti

Manifest versionato con scenario/config/hash, versioni core/harness/Go/piattaforma, revisione build quando disponibile, PRNG/draw tape, fault/cutpoint, trace ordinata e storia sintetica di invocazioni/risposte. Replay ricostruisce scheduler senza nuove scelte casuali, controlla ogni evento/digest e si ferma alla prima divergenza; checksum e versioni incompatibili rifiutati. La [fixture salvata](../fixtures/m12-session-retry.json) contiene 669 eventi, seed 7, risposta CAS persa dopo applicazione, crash del leader n3, retry su n1 con indice originario 2 e GET successiva del valore A. Replay da file confrontato byte per byte; nuovo run con stesso seed confrontato separatamente.

Gli esiti simulati distinguono pending, completed, unknown e not_admitted. Timeout, perdita risposta/leadership, crash o failure storage non inventano rollback; il risultato definitivo richiede indice applicato/committed e leadership/termine del tentativo. Retry conserva esito/indice originale nel contratto M11. Massimo 5 s virtuali, 10000 eventi, 128 azioni/tentativi, 10 MiB input/evidenza, 10 s reali per comando più cleanup limitato. Timeout/limiti rendono inconclusive. Trace invarianti non campionata. File output 0600, creazione esclusiva; nessun overwrite di evidenza o rimozione di directory. Riduttore locale limitato alle azioni e al predicato/precondizioni forniti.

## Osservabilità e interazioni

internal/diagnostics possiede contatori fissi, finestre di 2048 durate e ring 256 log <=1 KiB; nessuna crescita per chiave/client. Hook servizio registrano transizioni, RPC, Append+Sync, apply, recovery e dedup; hook HTTP correlano request_id/outcome/codice. Dati copiati, accessi sincronizzati, nessun I/O log nell'owner Raft o accesso handler alla mappa sessioni. Metriche GET /v1/metrics locali, potenzialmente obsolete e disponibili senza quorum; status e tutte le operazioni M11 restano identiche nell'[OpenAPI M12](../openapi/m12.json). [Fixture metriche](../fixtures/metrics-m12.json), validatore finito offline e controlli negativi verificano schema e limiti.

Il coordinatore processo drena JSON ogni 2 s e allo shutdown, default stderr; opzione -log-dir privata e separata usa quattro file di massimo 256 KiB, totale 1 MiB, mai dati WAL. Copy riservata della singola failure storage fatale, tentativo stderr anche con rotazione. Record sovrascritti contati, successo heartbeat non genera INFO. Errori diagnostici non diventano prove consenso. Snapshot include gauge ruolo/indici/match, proposte/lag, heap/goroutine e WAL bytes; nessuna chiave, valore, payload CAS, ID client completo o percorso grezzo. Durate documentate precisamente: elezione locale candidatura→primo commit corrente, recovery del prefisso nel servizio, non intero startup o perdita esterna del leader. Non sono benchmark o misure RSS/CPU. [Contratto completo](observability.md).

## Test, problemi e correzioni

16 nuovi test principali: sei faultsim, due diagnostics, due clientapi, due raft, uno node, uno server, uno CLI e una revisione nel file sessioni committed esistente. Seed 7/42/117 verificano perdita risposta, cambio leader, restart, originale risultato/indice, stessa traccia su nuovo run e replay da manifest serializzato. Traccia alterata e checksum errato sono controlli negativi; partizione/heal, delay/drop/duplicate e vecchio leader senza quorum sono osservabili, non fault nominali. Sei combinazioni di tre failure I/O con i due modelli crash, più cutpoint header/commit Sync, verificano assenza di conferma prematura e recupero. Budget, JSON malformato, cancellazione, riduzione e incompatibilità sono testati.

Prove reali HTTP a tre nodi correlano elezione e timeout CAS, fermano due follower e verificano metriche 200 mentre il retry rimane unknown. Unit test controllano redazione, snapshot senza alias, concorrenza, cap campioni/log, overflow e conservazione della failure fatale. Rotazione supera una sola volta circa 1 MiB sintetico per provare quattro file, privacy e rifiuto symlink. Test processo/coordinatore verifica startup/shutdown log, esclusione directory dati e riapertura lock/WAL. Regressioni reali follower/leader/tutti i processi M11 passano senza cambiare aspettative.

Problemi emersi soltanto nel nuovo lavoro: validazione scenario iniziale richiedeva normalizzare il JSON comando come il core; corretta senza cambiare codec. La partizione ha evidenziato che il nuovo driver doveva schedulare timeout RPC e chiamare Failed sul tentativo esatto, come il driver produzione: corretto e replay verificato. Cleanup usa una deadline indipendente dal caller cancellato e classifica anche failure durante setup/cleanup come failed/inconclusive. La regressione M09 sotto race ha riprodotto un risultato WaitApplied disponibile prima dello snapshot LastApplied aggiornato: applyCompleted notificava il buffer prima di pubblicare il progresso. Diagnostica aggiunta ampliava questa finestra preesistente. Correzione minima: pubblicare il cursore dopo Core.Applied e prima della notifica. Nuovo test con consegna bloccata fallisce senza correzione e passa tre volte con race dopo; aspettative M09 originali invariate. Nessun cambiamento di commit, CAS o deduplicazione. Corretto inoltre il nuovo logger perché un ErrLimit provato prima della scrittura non venga classificato come failure fatale; prova dedicata nello scenario diagnostico. Il primo tentativo HTTP senza permesso localhost non poteva aprire socket; le esecuzioni locali autorizzate sono passate. Nessun workaround nel protocollo.

## Verifiche reali e copertura

Go 1.27.4 darwin/arm64, profilo locale macOS/APFS. GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2; CGO_ENABLED=0 ordinario, 1 per race con compilatore esistente. Build GOOS=darwin/GOARCH=arm64. Comandi equivalenti effettivamente eseguiti:

```sh
gofmt -l cmd internal
go vet ./...
go build -p 1 ./...
go test -p 1 -count=1 -coverpkg=./... -coverprofile=/tmp/quorumkv-m12.cover -timeout=75s ./...
go test -race -p 1 -count=1 -timeout=75s ./internal/faultsim ./internal/diagnostics ./internal/raft ./internal/clientapi ./internal/node ./internal/server ./cmd/quorumkv-sim ./internal/committed
go test -race -p 1 -count=1 -timeout=40s ./internal/faultsim -run 'Test(WALHeader|SavedManifest|ScenarioValidation)'
go test -race -p 1 -count=3 -timeout=30s ./internal/raft -run 'Test(AppliedStatus|Diagnostics)'
go test -race -p 1 -count=1 -timeout=75s ./internal/raft ./internal/node
go run ./cmd/quorumkv-sim -replay docs/fixtures/m12-session-retry.json
python3 docs/fixtures/verify_canonical_v1.py
go list -m all
```

Suite completa M01–M12 superata nello stato finale. Race iniziale dell'intero gruppo superata; successiva prova M09 ha rilevato la finestra documentata, riprodotta dal nuovo test senza correzione e corretta. Ri-verificate con race le componenti finali interessate e i nuovi test cleanup/cutpoint/fixture; nessuna segnalazione di data race nelle esecuzioni passate. La suite faultsim iniziale sotto race ha richiesto circa 63 s: scenario piccolo, esecuzione finita, non stress test. Nessun cluster simultaneo fra esecuzioni: package seriali e massimo tre nodi. Formattazione, vet, build, checker fixture canoniche originale, modulo unico e controllo OpenAPI passati. Nessun nuovo modulo/go.sum; nessuna connessione esterna.

Copertura per istruzioni, unione dei blocchi dei profili di test: cmd/quorumkv 0.0%; cmd/quorumkv-sim 75.9%; internal/applier 100.0%; internal/clientapi 82.7%; internal/committed 82.9%; internal/config 89.7%; internal/diagnostics 90.5%; internal/faultsim 82.5%; internal/node 81.5%; internal/peerprotocol 91.4%; internal/raft 88.9%; internal/server 87.8%; internal/statemachine 100.0%; internal/storageformat 93.6%; internal/storagefs 87.9%; internal/strictjson 92.1%; internal/transport 90.1%; internal/wal 82.7%. Non copre ogni schedulazione, non attribuisce al main le istruzioni eseguite soltanto nei figli e non prova formalmente consenso o linearizzabilità. Le prove controllano risultati intermedi, indici e tracce, non soltanto valori finali.

## File e documentazione

Creati internal/faultsim/{types.go,runner.go,runner_test.go}, internal/diagnostics/{recorder.go,rotation.go,recorder_test.go}, internal/raft/diagnostics.go e diagnostics_test.go, internal/clientapi/metrics_test.go, internal/node/m12_test.go, internal/server/m12_test.go, cmd/quorumkv-sim/main.go e main_test.go. Modificati internal/raft/service.go e application.go, internal/clientapi/api.go, internal/node/node.go, internal/server/run.go e internal/committed/sessions_test.go. Il core Raft, M01/M02, wrapper sessioni, client mutation/read, config, codec e writer/recovery WAL di produzione rimangono invariati; application.go contiene la minima correzione di pubblicazione osservabile.

Creati docs/openapi/m12.json, docs/fixtures/metrics-m12.json, docs/fixtures/m12-session-retry.json, examples/m12/session-retry.json e queste relazioni. Aggiornati nelle due lingue fault injection, osservabilità, ADR-008, architettura, consistenza, storage/recovery, guasti, API, requisiti/tracciabilità, test, criteri, guida, roadmap e indici. Artefatti M01–M11 originali preservati; nessun cambiamento alle fixture canoniche o alle aspettative dei test storici. Verifica finale di 134 Markdown/66 coppie, link relativi, tabelle, fence, ID requisito ed esempi tecnici senza errori; revisione semantica effettuata, nessuna certificazione esterna.

## Accettazione, limiti e readiness M13

**M12 COMPLETED.** FR-017/AC-017/T-017, FR-020/AC-020/T-020 e NFR-008/AC-028/T-028 soddisfatti nel profilo esplicito; nessun requisito obbligatorio M12 residuo o blocco noto. Termini/voti, Log Matching, ghost committed, regola commit corrente, apply ordinato, LastApplied<=commit, slot GET, CAS/sessioni e incertezza risultano rispettati nei casi eseguiti. M11 mantiene massimo 1024 sessioni senza eviction e solo ultimo risultato; nessun exactly-once generale, migrazione v1 o miglioramento nascosto di retention.

Il simulatore è un modello finito: storage sincrono, tre nodi, azioni limitate e programmi sintetici; non riproduce la schedulazione del kernel. Replay versionato non prova correttezza universale del modello. Nessun checker esaustivo M14, benchmark/stress M15, misura RSS/CPU/SSD o crash reale OS/alimentazione. Perdita della maggioranza durevole e guasto del Mac/SSD condiviso restano fuori dalle garanzie. Metriche locali possono essere obsolete; log campionabili possono perdere record con contatore esplicito. Eventuali stalli arbitrari del filesystem/stderr non hanno limite dimostrato. Nessuna certificazione esterna.

**M13 NON INIZIATA.** Readiness soltanto preliminare: disponibili scenari/replay/cutpoint e diagnostica per verificare il futuro handoff snapshot. Il prossimo incarico deve riesaminare barriera applier/storage, snapshot di mappa+sessioni atomico, epoche, hard state aggiornato, generazioni/CURRENT, installazione e quota di picco; i gate snapshot C-03 e successivi non sono chiusi da M12. Non è implementato alcun componente snapshot/compattazione.

Spesa aggiuntiva **0 €**, Go/libreria standard offline, nessun servizio, acquisto o download. Dataset piccoli, timeout espliciti, due processori e package seriali, massimo tre nodi ordinari, cartelle temporanee isolate e cleanup. Nessun cluster lasciato attivo, GPU, VM/Docker obbligatori, scrittura massiva o test distruttivo. Nessun commit Git. Arresto a M12.
