# M13 Fase A — Codec e cattura in memoria

[Indice](README.md) · [English](../en/milestone-m13-preliminary.md)

Data: 22 novembre 2026. Base Git: `cce883d`; nessun commit effettuato. **Fase: PRELIMINARY PHASE COMPLETED. M13 complessiva: PARTIALLY COMPLETED. M14: NON INIZIATA.**

## Mandato e gate iniziali

La definizione ufficiale [M13 — Snapshot e compaction](implementation-roadmap.md) richiede generazioni atomiche, installazione, fixture crash e T-015/T-016/T-026 nel sottoinsieme della roadmap. Questa fase realizza esclusivamente codec, cattura atomica e restore su una nuova istanza in memoria, come autorizzato dalla [revisione pre-M13](pre-m13-review.md). I contratti approvati prevalgono; nessun criterio viene ridotto per dichiarare conclusa la milestone.

C-01/OQ-05 era RESOLVED nel design: ora la conformità isolata del codec è verificata. C-02/OQ-01 conserva la validazione delle primitive precedenti ma resta CONDITIONALLY READY per nuove generazioni. C-03 resta CONDITIONALLY READY per handoff e safety runtime; la cattura isolata aggiunge evidenze, non chiude il gate.

## Componenti e inventario

Nuovi sorgenti: `internal/storageformat/snapshot.go` (EncodeSnapshot, DecodeSnapshot, ValidateSnapshot); `internal/statemachine/state.go` (CopyState, FromState); `internal/committed/session_state.go` (CopySessions, SessionsFromState); `internal/applier/barrier.go` (Park, Inspect, Release); `internal/snapshot/memory.go` (NewMachine, Capture, Restore). Modificato `internal/applier/applier.go` per coordinare la barriera nel worker esistente. Ogni nuovo sorgente ha il corrispondente file `_test.go`.

Aggiunta [fixture snapshot v2](../fixtures/snapshot-v2.json). Aggiornati in entrambe le lingue: architecture, canonical-formats, data-model, storage-and-recovery, testing-strategy, acceptance-criteria, traceability, failure-model, requirements, development-guide, implementation-roadmap, indici e ADR-005/ADR-010; aggiunta questa relazione e aggiornato README principale. Nessuna modifica al runtime node/Raft/WAL/server, al filesystem adapter o alle fixture storiche.

## Formato e compatibilità

Il contenitore approvato resta QSN1 versione 1: magic di 4 byte, lunghezza payload uint64 big-endian, payload JSON ASCII canonico, trailer SHA-256 di header e payload. Dimensione totale 44 byte più payload, massimo 8 MiB. L'hash dell'intero file è distinto dal trailer. Profilo persistente v2 significa configurazione/applicationVersion/IDENTITY v2 e hash condiviso; non introduce un contenitore snapshot versione 2.

Campi e oggetti canonici hanno ordine deterministico; indici, termini, quote e conteggi uint64 sono stringhe decimali, mentre sequence è un numero JSON intero sicuro, KV e sessioni sono ordinati per chiave e client_id; valori base64 canonici. Il payload conserva confine indice/termine, cluster/hash, quota logica, fingerprint QCMDv1, ultima sequenza e risultato originale con indice, incluso CAS fallito. Il risultato originale non può essere successivo al confine.

Prima delle allocazioni dipendenti dal contenuto sono verificati lunghezza totale, overflow, magic, digest e conteggi. Parsing incrementale degli array evita la costruzione di grafi JSON sproporzionati; limiti per riga derivano dai massimi legali esistenti. Rifiutati campi sconosciuti, escape/spazi non canonici, duplicati, disordine, versioni incompatibili, unioni risultato incoerenti e somme errate. Il decoder ricodifica e confronta tutti i byte; su errore non restituisce stato parziale.

I vettori v1 originali rimangono byte-esatti. La nuova fixture è costruita indipendentemente con la libreria standard Python e contiene casi vuoto/stateful, payload, esadecimale, digest trailer e hash file. I fingerprint dei comandi sintetici non pretendono di rappresentare una storia cluster eseguita. Il codec può verificare i vettori storici; Restore accetta soltanto configurazione v2 e identità/hash esatti. Nessuna migrazione v1→v2.

## Cattura atomica e ownership

Park congela nuove ammissioni a B = ScheduledApplyIndex e lascia terminare il prefisso già accettato nel worker singolo M02. Dopo che il coordinatore seriale ha consumato i completamenti, Capture richiede A = W = L = B <= C: LastApplied, indice effettivo del worker, scheduled e barriera coincidono e non superano commitIndex. Un scheduled più avanzato non viene fotografato prematuramente: viene drenato prima della copia.

Proof deve provenire dal coordinatore proprietario del log; TermAt legge la stessa vista sincronizzata, non uno status HTTP. Il confine zero richiede termine zero; gli altri confini richiedono termine positivo coerente. La macchina verifica il conteggio realmente applicato, indice dell'envelope delle mutazioni, termine noto non regressivo e confine restaurato. Read/noop avanzano l'indice senza cambiare KV. Proof non dimostra autonomamente quorum o commit Raft; l'integrazione futura deve produrre quelle evidenze. ApplyEpoch è limitato all'istanza isolata 1, non è un protocollo runtime di cambio epoca.

Inspect copia KV e sessioni nella stessa esecuzione proprietaria, senza un secondo writer e senza lock metadata dell'applier durante la copia. La callback fidata deve terminare, non invocare Goexit e non attendere lo stesso applier. Il caller rilascia la barriera prima della serializzazione. Una sola lease e un solo controllo pendente limitano le richieste; ammissione congelata non significa copia pronta.

Cancellare l'attesa non scarta comandi già accettati o callback in corso. Release riceve conferma dal worker; cancellare la sua attesa non riapre prematuramente le ammissioni. Shutdown libera il worker e rifiuta nuovi controlli; panic applicativo/di ispezione fallisce in sicurezza. Le copie di valori, record e digest non condividono buffer mutabili: modifiche successive e alterazioni del caller non contaminano macchina o cattura.

## Deduplicazione e restore isolato

La copia include ultimo risultato per sessione, sequenza, tipo, digest e indice originale; nessuna cache parallela, scadenza o eviction. Il limite resta 1024 sessioni. La CAS e il suo aggiornamento sessione sono indivisibili rispetto alla cattura, anche nel caso negativo. PUT, DELETE, GET, noop, retry, conflitti e sequenze obsolete conservano la semantica precedente.

Restore valida codec, profilo e termine contro currentTerm fornito dalla vista hard-state validata del caller; costruisce mappe e wrapper sessioni nuovi, restituisce Point e richiede InitialIndex corrispondente nell'applier isolato. Non riapplica 0..S, non sostituisce mappe live, non apre storage e non seleziona percorsi di bootstrap. Il suffisso S+1 riprende ordinatamente e i retry conservano risultati e indici originali, inclusi CAS falliti anche se lo stato corrente permetterebbe ora il successo.

## Test introdotti ed errori esercitati

17 funzioni test principali in cinque file: quattro codec, quattro barriera, una stato KV, una sessioni, sette cattura/restore. Vettori confrontati byte per byte, round-trip, ordine indipendente dall'inserimento, input corrotti/troncati, digest/lunghezze/conteggi/versioni errati, duplicati e unioni invalide. Verificati anche confini uint64, 10000 chiavi, chiave 256 byte, valore 16 KiB, 4 MiB logici e 1024 sessioni; il caso massimo combinato usato è verificato entro il limite conservativo di 6782978 byte, sotto 8 MiB.

La concorrenza usa canali e gate espliciti: CAS bloccata dopo la mutazione KV prima della registrazione sessione, GET già scheduled, drain e confronto del prefisso esatto. Verificati cancellazione, shutdown, panic, callback errata, metadati incoerenti, term regression, indice envelope errato e copie difensive. Restore viene confrontato con la macchina originale continuata sullo stesso suffisso, con retry, sequenze obsolete/gap/conflitto, nuove sessioni e quota piena. La fixture v2 viene restaurata e ricatturata byte per byte.

Non è necessario estendere il simulatore M12 per questi guasti in memoria; le sue regressioni restano eseguite. Nessun test di pubblicazione viene trasformato in writer attivo.

## Verifiche realmente eseguite

Go 1.27.4 darwin/arm64, solo libreria standard, GOPROXY/GOSUMDB disattivati. Suite completa M01–M12 e nuovi test con copertura, timeout 75 s per package, GOMAXPROCS=2 e parallelismo package 1. Race mirato su applier, snapshot, storageformat, statemachine e committed con timeout 40 s. Gofmt, go vet, build macOS ARM64, verifica fixture originali, dipendenze e controlli documentazione/perimetro completati. I comandi riproducibili sono nella [guida sviluppo](development-guide.md).

Copertura statement misurata sulla suite completa, unendo per blocco i profili dei package: applier 95,1%, snapshot 93,5%, storageformat 91,5%, statemachine 100%, committed 85,1%. Il profilo temporaneo è `/tmp/quorumkv-m13a.cover`; i blocchi importati da più package sono contati una sola volta. Coverage e race non costituiscono una prova formale di correttezza.

## Criteri ufficiali ancora aperti

| Criterio | Evidenza della fase A | Evidenza ancora mancante |
|---|---|---|
| T-015 / AC-015 / FR-015 | Codec integro, copia coerente, restore isolato e dedup. | Installazione a follower molto arretrato, retry chunk, cambio termine, hash/sessioni/indici finali nel cluster. |
| T-016 / AC-016 / FR-016 | Corruzioni immagine e rifiuti isolati. | Arresto a ogni cutpoint di pubblicazione/cleanup, CURRENT autorevole e almeno una generazione recuperabile. |
| T-026 / AC-026 / NFR-006 | Quote logiche e sessioni, limiti codec e richieste di copia. | Code e disco simulato nel nuovo percorso, picco directory/riserve; RSS/CPU e report risorse previsti dalla roadmap. |

Tutti e tre restano aperti: la copertura di un sottoinsieme non supera il criterio completo. C-02 nuove generazioni e C-03 handoff runtime rimangono CONDITIONALLY READY.

## Problemi, costi e limiti

Durante lo sviluppo sono stati corretti un harness fixture che tentava di interpretare metadati root come record snapshot e una cancellazione di contesto segnalata da vet. Le verifiche finali includono tali correzioni. Nessun nuovo difetto delle milestone precedenti richiede correzione: M02 riceve soltanto la capacità isolata di barriera; l'ordine e la semantica ordinaria restano regressivamente verificati.

Budget aggiuntivo 0 €; esecuzione offline, nessuna dipendenza, cloud, GPU, Docker o VM. Dataset piccoli, un singolo caso in memoria ai limiti approvati, pochi processi e cluster temporanei al massimo a tre nodi, timeout e cleanup. Nessuna scrittura massiva snapshot, stress prolungato o crash fisico. Non sono misurati RSS/CPU né certificati guasti di alimentazione, SSD o crash consistency snapshot. Il digest e il restore isolato non dimostrano pubblicazione durevole o compaction sicura.

## Decisione e fase successiva

**PRELIMINARY PHASE COMPLETED** per il perimetro autorizzato. **M13: PARTIALLY COMPLETED**; C-01/OQ-05 RESOLVED nel design con conformità codec verificata; C-02/OQ-01 nuove generazioni e C-03 CONDITIONALLY READY; T-015/T-016/T-026 aperti; M14 NON INIZIATA.

Un nuovo incarico può autorizzare writer e protocollo di generazione in directory temporanee, cutpoint/errori/barriere/quote e verifica selezione recovery secondo contratto. Integrazione e attivazione runtime, handoff con epoche, pubblicazione CURRENT, compaction e InstallSnapshot restano subordinate all'autorizzazione e alle rispettive evidenze. Nessun trigger, RPC snapshot, cancellazione WAL o recovery bootstrap snapshot è attivato in questa fase. Nessun commit Git.
