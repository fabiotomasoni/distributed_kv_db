# Primary references and verification status

[Index](README.md) · [Italiano](../it/references.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Provenance and verification

Primary sources accessed on the web on **18 November 2026**: documents were accessible and relevant. “Accessed” does not mean independent proof of every statement or validation of a QuorumKV implementation. No long quotations are reproduced; references distinguish theoretical properties from local decisions.

| ID | Source | Use and limitation |
|---|---|---|
| REF-01 | [Ongaro e Ousterhout / Ongaro and Ousterhout, In Search of an Understandable Consensus Algorithm (Extended Version), 2014](https://raft.github.io/raft.pdf) | Algorithm, Figure 2, election, replication and snapshots. Local choices are QuorumKV specifications, not quotations from the paper. |
| REF-02 | [The Raft Consensus Algorithm](https://raft.github.io/) | Author-maintained site and access to primary materials for further study. |
| REF-03 | [etcd documentation](https://etcd.io/docs/) | Conceptual comparison and operational reference; no compatibility promised. Pin page versions before importing any future specification. |
| REF-04 | [Go documentation](https://go.dev/doc/) | Language, standard library and free tools; not proof of project correctness. |
| REF-05 | [Herlihy and Wing, Linearizability: A Correctness Condition for Concurrent Objects, ACM TOPLAS 12(3), 1990](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf) | Formal linearizability definition; checker and bounds are designed here. |
| REF-06 | [The Go Memory Model](https://go.dev/ref/mem) | Synchronization and concurrent access; not a distributed consensus model. |
| REF-07 | [Go Data Race Detector](https://go.dev/doc/articles/race_detector) | Usage and limits of data-race detection; distributed verification remains separate. |
| REF-08 | [Go os.File.Sync](https://pkg.go.dev/os#File.Sync) | File synchronization API; APFS/controller guarantees require local verification. |
| REF-09 | [Pillai et al., All File Systems Are Not Created Equal: On the Complexity of Crafting Crash-Consistent Applications, OSDI 2014](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/pillai) | Primary research on crash consistency and filesystem ordering; not validation of APFS on M5. |
| REF-10 | [Go Release History](https://go.dev/doc/devel/release) | Go 1.27.4 baseline and release policy; recheck before installation. |
| REF-11 | [Go license](https://go.dev/LICENSE) | Toolchain license; distinct from the still-open QuorumKV license. |
| REF-12 | [OpenAPI Initiative](https://openapis.org/) | Specification entry point; schema generation deferred to M08. |
| REF-13 | [Go encoding/json](https://pkg.go.dev/encoding/json) | Accessed during audit: v1 duplicate-key, field-name and UTF-8 behavior; implement explicit validation. |

## Future verification

APFS/SSD power-loss behavior, directory sync and macOS locking on the actual machine have not been experimentally verified. No hardware source is presented as proof of that guarantee. M03 requires platform-documentation verification and process tests; if a future source is inaccessible, record it as unverified with date and open question. Retain the URL and version used without vendoring whole copyrighted texts.

## Platform sources checked for C-02 — 19 November 2026

The following primary sources were read during this review, without copying complete texts. Archival API pages are not proof of support on current APFS; the [contract](macos-storage-contract.md) records limitations and pending tests.

| ID | Source | Use and limitation |
|---|---|---|
| REF-14 | [Apple fsync](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fsync.2.html) | API distinction from full flush; not device validation. |
| REF-15 | [Apple fcntl](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/fcntl.2.html) | F_FULLFSYNC description; archival filesystem list predates APFS. |
| REF-16 | [Apple rename](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/rename.2.html) | Namespace replacement, not a multi-file transaction. |
| REF-17 | [Apple flock](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html) | Cooperative exclusive lock; actual release tests pending. |
| REF-18 | [Apple About APFS](https://developer.apple.com/documentation/foundation/about-apple-file-system) | Filesystem features/space sharing, not database durability proof. |
| REF-19 | [Go 1.27.4 Darwin Fsync source](https://raw.githubusercontent.com/golang/go/go1.27.4/src/internal/poll/fd_fsync_darwin.go) | Checked against the local toolchain: full-flush request and ENOTSUP fallback. |
