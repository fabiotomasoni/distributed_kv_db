# Bilingual glossary

[Index](README.md) · [Italiano](../it/glossary.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Shared terminology

Identifiers, RPC names and error codes stay identical in both languages. “Safety” concerns distributed correctness; “security” concerns access protection: they are not synonyms.

| English | Italiano | Definition |
|---|---|---|
| Raft term | Termine Raft | Monotonic logical election epoch, not physical time. |
| Quorum | Quorum | Majority of 2 out of 3 static members. |
| Leader | Leader | Elected node proposing and replicating entries in its term. |
| Follower | Follower | Replica receiving log and permitted to vote. |
| Candidate | Candidato | Node requesting votes for the current term. |
| Log entry | Entry di log | Command with a global index and originating term. |
| Commit | Commit | Consensus decision making a prefix immutable within the model. |
| Apply | Applicazione | Deterministic execution of a committed command. |
| Linearizability | Linearizzabilità | Equivalence to a legal sequential order respecting real-time order. |
| Read barrier | Barriera di lettura | Current read entry committed and sampled at its apply slot. |
| WAL | Write-ahead log | Persistent record synchronized before dependent acknowledgments. |
| Durability | Durabilità | Preservation conditional on the failure/storage model. |
| Snapshot | Snapshot | Consistent data/session image at a known committed index. |
| Compaction | Compaction | Safe removal of history covered by a durable snapshot. |
| CAS | Compare-and-swap | Atomic byte comparison and update of one key. |
| ABA | Problema ABA | Value returns to its original value after changes invisible to value-based CAS. |
| Deduplication | Deduplicazione | Recognizing one identity to prevent additional effects. |
| Unknown outcome | Esito incerto | Client uncertainty: the request may be committed. |
| Safety | Sicurezza algoritmica | No forbidden state occurs, even without progress. |
| Liveness | Progresso | Operations advance under delivery/scheduling assumptions. |
| Crash-stop | Crash-stop | Process stops and does not return within the scenario. |
| Crash-recovery | Crash-recovery | Process restarts from persistent state. |
| Byzantine fault | Guasto bizantino | Arbitrary or malicious behavior, excluded. |
| Backpressure | Backpressure | Slowing/rejecting admission due to bounded queues/resources. |
| Seed | Seed | Initial reproducible random state, insufficient alone for OS scheduling. |
| RSS | Memoria residente | OS-reported resident process memory, distinct from Go heap alone. |
| fsync / Sync | Sincronizzazione | Persistence request to the system, with guarantees depending on underlying layers. |
