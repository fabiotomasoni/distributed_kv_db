# Fonti primarie e stato della verifica

[Indice](README.md) · [English](../en/references.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Provenienza e verifica

Fonti primarie consultate via web il **18 novembre 2026**: documenti accessibili e pertinenti. “Consultata” non significa prova indipendente di ogni loro affermazione né validazione di un'implementazione QuorumKV. Nessuna citazione testuale lunga è riprodotta; riferimenti distinguono proprietà teoriche da decisioni locali.

| ID | Fonte | Uso e limite |
|---|---|---|
| REF-01 | [Ongaro e Ousterhout / Ongaro and Ousterhout, In Search of an Understandable Consensus Algorithm (Extended Version), 2014](https://raft.github.io/raft.pdf) | Algoritmo, figura 2, elezione, replica e snapshot. Le scelte locali sono specifiche QuorumKV, non citazioni del paper. |
| REF-02 | [The Raft Consensus Algorithm](https://raft.github.io/) | Sito degli autori e accesso a materiali primari, per approfondimento. |
| REF-03 | [etcd documentation](https://etcd.io/docs/) | Confronto concettuale e riferimento operativo; nessuna compatibilità promessa. Versione delle pagine da fissare se si importa una specifica in futuro. |
| REF-04 | [Go documentation](https://go.dev/doc/) | Linguaggio, libreria standard e strumenti gratuiti; non prova di correttezza del progetto. |
| REF-05 | [Herlihy and Wing, Linearizability: A Correctness Condition for Concurrent Objects, ACM TOPLAS 12(3), 1990](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf) | Definizione formale di linearizzabilità; checker e limiti sono progettati qui. |
| REF-06 | [The Go Memory Model](https://go.dev/ref/mem) | Sincronizzazione e accessi concorrenti; non modello di consenso distribuito. |
| REF-07 | [Go Data Race Detector](https://go.dev/doc/articles/race_detector) | Uso e limiti del rilevamento data race; suite distribuita resta separata. |
| REF-08 | [Go os.File.Sync](https://pkg.go.dev/os#File.Sync) | API di sincronizzazione file; garanzie APFS/controller da verificare localmente. |
| REF-09 | [Pillai et al., All File Systems Are Not Created Equal: On the Complexity of Crafting Crash-Consistent Applications, OSDI 2014](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/pillai) | Ricerca primaria su crash consistency e ordinamento filesystem; non verifica di APFS M5. |
| REF-10 | [Go Release History](https://go.dev/doc/devel/release) | Baseline Go 1.27.4 e politica di release; ricontrollare prima di installare. |
| REF-11 | [Go license](https://go.dev/LICENSE) | Licenza toolchain; distinta dalla licenza QuorumKV ancora aperta. |
| REF-12 | [OpenAPI Initiative](https://openapis.org/) | Punto di accesso alla specifica; generazione schema rimandata a M08. |
| REF-13 | [Go encoding/json](https://pkg.go.dev/encoding/json) | Consultata durante audit: limiti di duplicate key, nomi campo e UTF-8 del decode v1; implementare validazione esplicita. |

## Verifiche future

Comportamento power loss APFS/SSD, sync directory e lock macOS sul computer reale non sono stati verificati sperimentalmente. Non viene indicata una fonte hardware come se provasse tale garanzia. M03 richiede verifica documentale della piattaforma e test di processo; se una fonte futura è irraggiungibile, registrarla come non verificata con data e questione aperta. Conservare URL e versione usata, senza vendorizzare interi testi coperti da copyright.

## Fonti piattaforma verificate per C-02 — 19 novembre 2026

Le seguenti fonti primarie sono state lette durante questa revisione, senza copiare testi completi. Pagine API d’archivio non provano supporto su APFS attuale; il [contratto](macos-storage-contract.md) registra limiti e test pendenti.

| ID | Fonte | Uso e limite |
|---|---|---|
| REF-14 | [Apple fsync](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html) | Distinzione API dal full flush; non validazione device. |
| REF-15 | [Apple fcntl](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fcntl.2.html) | Descrizione F_FULLFSYNC; elenco filesystem d’archivio precedente ad APFS. |
| REF-16 | [Apple rename](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/rename.2.html) | Sostituzione namespace, non transazione multi-file. |
| REF-17 | [Apple flock](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html) | Lock esclusivo cooperativo; test rilascio reali pendenti. |
| REF-18 | [Apple About APFS](https://developer.apple.com/documentation/foundation/about-apple-file-system) | Funzioni filesystem/spazio condiviso, non prova durabilità database. |
| REF-19 | [Sorgente Go 1.27.4 Darwin Fsync](https://raw.githubusercontent.com/golang/go/go1.27.4/src/internal/poll/fd_fsync_darwin.go) | Confrontato con toolchain locale: richiesta full flush e fallback ENOTSUP. |
