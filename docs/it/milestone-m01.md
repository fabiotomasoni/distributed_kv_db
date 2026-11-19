# M01 — Macchina a stati deterministica

[Indice](README.md) · [English](../en/milestone-m01.md)

**Stato: COMPLETED — 19 novembre 2026. M02 non iniziata.**

## Ambito verificato prima di implementare

Letti entrambi gli [audit architetturali](architecture-review.md), roadmap, requisiti, modello dati, API, consistenza, architettura, ADR-001/003/006/007, test e criteri. M01 ufficiale è il sottoinsieme T-008/T-009 su macchina sequenziale locale, incluse quote e CAS. Non ci sono discrepanze sostanziali con l'incarico. I gate C-01/C-02 riguardano formati persistenti e adattatore filesystem futuri: M01 non li usa.

Implementata soltanto la proiezione locale `(map, logicalBytes)`. Le sessioni, i contatori client, la deduplicazione, gli indici e le barriere di consenso restano alle milestone previste. Atomicità significa una transizione sequenziale indivisa; nessuna garanzia di accesso concorrente, linearizzabilità distribuita o durabilità viene dichiarata.

## File e componenti

| File | Responsabilità |
|---|---|
| [go.mod](../../go.mod) | Modulo corrispondente al remote Git, con requisito minimo Go 1.27.4; nessun require o dipendenza esterna. |
| [doc.go](../../internal/statemachine/doc.go) | Confini del package, uso sequenziale e ownership. |
| [command.go](../../internal/statemachine/command.go) | Operation, Command, Expectation, Result, Code e limiti approvati. |
| [validate.go](../../internal/statemachine/validate.go) | Grammatica ASCII, dimensioni e forma dei comandi prima di qualunque modifica. |
| [machine.go](../../internal/statemachine/machine.go) | Mappa privata, quote in apply e operazioni deterministiche. |
| [machine_test.go](../../internal/statemachine/machine_test.go) | Semantiche, rifiuti atomici, CAS sequenziali e isolamento dei buffer. |
| [limits_test.go](../../internal/statemachine/limits_test.go) | Limiti esatti di valori, numero chiavi e byte logici, rilascio delle quote. |
| [determinism_test.go](../../internal/statemachine/determinism_test.go) | Oracle indipendente a lista/stringhe, replay e istanze indipendenti. |

Aggiornati anche README principale, indice docs comune e, in entrambe le lingue, indice, overview, roadmap, data-model, architecture, development-guide, testing-strategy, acceptance-criteria e traceability. I rapporti del 18 novembre restano evidenze storiche della fase di progettazione, non descrizioni del codice corrente. Questa relazione e la roadmap riportano lo stato attuale.

## Interfaccia Go locale

Package `github.com/fabiotomasoni/distributed_kv_db/internal/statemachine`:

```go
func New() *Machine
func (m *Machine) Apply(c Command) Result
```

`Command` contiene Operation (`Put`, `Get`, `Delete`, `CAS`), Key string, Value byte opachi e Expected opzionale. `Expectation{Exists:false}` confronta assenza; `Expectation{Exists:true,Value:...}` confronta byte esatti. `Result` contiene Code e i campi specifici Value/Exists, Applied oppure Deleted. I codici sono `OK`, `INVALID_ARGUMENT`, `KEY_NOT_FOUND`, `COMPARE_FAILED`, `RESOURCE_LIMIT`. Il risultato di dominio è un valore, senza errori I/O o eccezioni implicite.

Le mutazioni non CAS vietano Expected; GET/DELETE vietano Value anche se slice vuota non-nil; CAS richiede Expected. `Expectation{Exists:false}` con Value non-nil è invalida. Value nil su PUT/CAS e Value nil per expected presente rappresentano una sequenza vuota di byte. Non è il JSON null: JSON/Base64 e loro validazione non sono implementati. L'adattatore futuro dovrà rifiutare null secondo il contratto wire. Nessun tag JSON, encoder o formato persistente è definito qui.

Lo zero value di Machine è utilizzabile. Non copiare Machine dopo il primo uso; usare istanze separate. Il chiamante serializza Apply. I valori vengono copiati in ingresso e in uscita, così modificare un comando o un risultato già restituito non cambia lo stato. Nessun mutex, goroutine, worker o scheduler è stato introdotto: questi appartengono a M02.

## Semantica e invarianti locali

Chiavi ASCII case-sensitive `[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}`, valori raw fino a 16 KiB, 10 000 chiavi e 4 MiB di `sum(len(key)+len(value))`. Il contatore byte è uint64; i calcoli sono limitati dai valori validati. Ogni sostituzione sottrae il vecchio valore senza contare di nuovo la chiave. DELETE libera chiave e valore; una chiave assente è successo con Deleted=false.

PUT sostituisce il valore intero. GET distingue assenza da valore vuoto. CAS confronta esistenza e byte, poi esegue la stessa verifica di quota di PUT in un'unica chiamata; confronto fallito e quota superata non cambiano la mappa né il contatore. CAS che riscrive lo stesso valore può riuscire più volte; il CAS su valore non protegge da ABA. Una modifica parziale di JSON non esiste.

Non si usa tempo, random, I/O, stato globale mutabile o iterazione della mappa per decidere risultati. Dopo ogni passo delle sequenze di test si ricalcolano somma byte e numero chiavi. Le viste ordinate dello stato esistono soltanto nei test e non costituiscono API snapshot o codifica canonica.

## Test effettivi e tracciabilità

| Sottoinsieme | Evidenza implementata | Limite della chiusura |
|---|---|---|
| T-008/M01, FR-008 | 128 comandi riproducibili, seed test 19, tre istanze separate incluso zero value, confronto risultato e stato dopo ogni passo con oracle a lista di stringhe. Replay di sequenza fissa e indipendenza fra istanze. | AC-008 distribuito e scheduling M02/M07 restano aperti. |
| T-009/M01, FR-009 | Inserimento, lettura, update, DELETE ripetuto, assenza, vuoto, byte binari, case sensitivity e sostituzione di documenti interi. | Nessuna API HTTP o lettura Raft; AC-009 completo resta aperto. |
| Semantica CAS locale, FR-010 | Match/mismatch, missing/present-empty, create-if-absent, valore invariato, due ordini sequenziali di competizione. | Non è T-010 concorrente; sessioni e failover restano M11. |
| Quote logiche, NFR-006 | Valore 0/1/16 KiB, chiave 256/257 byte, esattamente 10 000 chiavi, esattamente 4 MiB e superamento di un byte; update ammessi e spazio riusabile dopo riduzione/delete. | RAM, CPU, WAL e disco del cluster non misurati o implementati. |
| Rifiuti e ownership | 19 comandi invalidi, stato e contatore invariati, input/output senza alias, validazione prima del confronto CAS. | Parsing JSON e sicurezza di trasporto restano futuri. |

La fixture al limite chiavi è costruita privatamente nei test, evitando 10 000 richieste; viene verificata prima di esercitare il limite. La fixture byte usa meno di 300 transizioni e circa 4 MiB di dati logici. Non sono test di carico. Il PRNG è solo nel test a seed fisso, mai nel package applicativo.

## Controlli realmente eseguiti

Toolchain **go1.27.4 darwin/arm64** scaricata dall'[indice ufficiale Go](https://go.dev/dl/?mode=json) in `/tmp/quorumkv-m01-toolchain`, senza installazione globale. SHA-256 dell'archivio controllato contro l'indice: `91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4`. La toolchain è un prerequisito di sviluppo, non una dipendenza runtime del package.

Comandi eseguiti dal repository con GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, CGO_ENABLED=0, GOMAXPROCS=2 e cache/GOPATH temporanei:

```sh
gofmt -w internal/statemachine/*.go
go test -p 1 -count=1 -timeout=30s ./...
go vet -p 1 ./...
go test -json -p 1 -count=3 -cover -timeout=30s ./...
go list -m all
go list -f '{{.ImportPath}}: {{join .Imports ", "}}' ./...
gofmt -l internal/statemachine/*.go
```

Esito: **PASS**, 12 test principali e 44 sottocasi per esecuzione. La prima suite è passata; le successive tre ripetizioni hanno prodotto 168 eventi pass e nessun fail, con **100.0% statement coverage del solo package**. Go ha riportato 0.232 s nel primo run e 0.262 s nel run con tre ripetizioni; non sono benchmark. `go vet` exit 0; `gofmt -l` senza output. Il modulo è l'unico in `go list -m all`; il codice applicativo importa soltanto `bytes`, della standard library.

Controllo documentale finale: 100 file Markdown e 49 coppie linguistiche, 1 075 collegamenti relativi risolti, 72 tabelle, sezioni/identificatori e 15 coppie di esempi tecnici non Mermaid allineati. Nessun errore rilevato; git diff --check senza segnalazioni.

Non sono stati eseguiti race detector o test concorrenti, simulazioni Raft, benchmark o test storage. Nessuna misura di RAM/CPU/SSD del database. Coverage e replay finiti non dimostrano correttezza per ogni possibile sequenza. Dopo il download iniziale, compilazione e test non hanno richiesto dipendenze o servizi di rete.

## Esito e attività successive

**M01: COMPLETED.** Tutto il sottoinsieme ufficiale locale è implementato e i controlli richiesti sono passati; non sono noti problemi che violino le invarianti M01. Nessuna spesa aggiuntiva, dipendenza esterna, infrastruttura o formato persistente introdotti. I gate dell'audit per M03 e oltre restano aperti.

M02 può essere affrontata senza problemi noti derivanti da M01, progettando ownership e scheduling attorno al core sequenziale. **M02 non è iniziata**: questa consegna si ferma a M01 e rimane disponibile per revisione.
