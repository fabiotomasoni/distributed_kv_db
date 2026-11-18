# Scope, assumptions and limitations

[Index](README.md) · [Italiano](../it/scope.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Boundaries

The final goal of the first version is a static three-node Raft group on loopback, with four atomic single-key operations, a synchronous WAL, recovery and snapshots. Each node stores the entire dataset. `cluster_id`, membership, addresses and deterministic limits are identical everywhere; startup fails if configuration contradicts the persistent manifest.

## Increments and first MVP

The first **internal MVP** is the M09 gate: election, replication, commit, PUT/DELETE, WAL and recovery, exercised by a test client. It is not the complete first version and does not yet expose GET as a guaranteed read. M10 adds linearizable GET; M11 adds CAS and sessions/retry; M13 adds snapshots and compaction; M14 closes the correctness gate for the first version. Do not mark an extension's requirement complete at an earlier MVP gate.

After the first version, ReadIndex instead of logged reads, WAL batching and diagnostic improvements may be evaluated. These are optional and require new ADRs and evidence. Pre-vote and check-quorum are not necessary for safety and remain extensions; without pre-vote, an isolated node can cause unnecessary terms and elections after rejoining.

## Exclusions

No sharding, dynamic membership, SQL, multi-key transactions, geographic replication, public hosting, multi-tenancy, Kubernetes or commercial compatibility. No external KV engine or Raft implementation. No advanced authentication, multiuser authorization or mandatory TLS in the local prototype. No paid services, credit cards, trials, domains, additional hardware or runtime Internet dependency.

## Operating assumptions

Crash-stop and crash-recovery faults with non-malicious processes; messages may be lost, duplicated, delayed or reordered. Safety does not rely on synchronized clocks. Liveness needs a functioning majority and sufficiently long periods of timely delivery. Persistent storage must meet the [recovery](storage-and-recovery.md) contract; corruption and simultaneous loss of the shared disk are not masked by Raft.

Trusted environment, macOS ARM64 on a MacBook Pro M5 with 32 GB. Additional budget is €0; existing electricity and connectivity are not new expenses, but are not physically free. Dependencies are acquired once, after which the database and tests must work offline. GitHub Free is optional for publishing the portfolio, never required to run the system.

## Declared limitations

Resource use, timing and power-loss durability are hypotheses to measure, not results. Initial limits are specified in [resources](performance-and-resources.md). Network security assumes trusted loopback only. The repository license is **for the owner to choose**; an open-source grant is not assumed. The [review](design-review.md) records outstanding gates.
