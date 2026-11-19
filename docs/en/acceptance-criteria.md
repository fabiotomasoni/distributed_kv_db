# Objective acceptance criteria

[Index](README.md) · [Italiano](../it/acceptance-criteria.md)

**Updated: 19 November 2026 — local M01 implemented and checked; distributed specifications remain design work.**

## Acceptance rule

Full criteria remain open; only the local M01 subsets of T-008/T-009 are **Passed**, as documented below. Closing a row requires a repeatable procedure, evidence tied to a revision/configuration and a satisfied condition. “No panic” or an error-free log is insufficient. See the [test catalog](testing-strategy.md) for scheduling and fixture details.

| Criterion | Requirement / procedure | Required evidence | Passing condition |
|---|---|---|---|
| AC-001 | FR-001 / T-001. Start three distinct configurations and attempt a second opening of n1. | Manifest and exit codes. | Three distinct directories and identities; second instance rejected. |
| AC-002 | FR-002 / T-002. Vary membership, cluster ID and hash in configuration and RPCs. | Valid/invalid fixtures and before/after state. | Divergent manifest or duplicate member prevents startup. |
| AC-003 | FR-003 / T-003. Inject invalid JSON, incorrect envelope and oversized body. | Responses and unchanged state hash. | Malformed payloads rejected without state mutation. |
| AC-004 | FR-004 / T-004. Simulate split votes and crash immediately after vote Sync; replay for 20 seeds. | Vote/term/leader trace and seed. | No term has two leaders in trace, including crash after voting. |
| AC-005 | FR-005 / T-005. Advance virtual time through 100 fault-free heartbeats. | Election count and timers. | Stable leader produces no spurious elections in a fault-free simulator. |
| AC-006 | FR-006 / T-006. Construct divergent logs, send heartbeats and obsolete responses, then heal. | Prefixes, matchIndex and transitions. | Divergent logs converge without overwriting committed entries. |
| AC-007 | FR-007 / T-007. Reproduce an old-term entry on a majority, then a current noop. | Unchanged commit before, advanced commit after. | Old entry is not committed merely by counting replicas. |
| AC-008 | FR-008 / T-008. Apply the same sequence to three models, delaying one applier. | Canonical digest and per-index results. | Same sequence produces identical state and results on all three nodes. |
| AC-009 | FR-009 / T-009. Enumerate absent key, empty value, invalid input and repeated DELETE. | API fixture table and model. | Absence, empty value and repeated DELETE match the API table. |
| AC-010 | FR-010 / T-010. Run two CAS from different sessions on the same value with different new values. | History with exactly one successful change. | Two concurrent CAS with the same expected: at most one changes the value. |
| AC-011 | FR-011 / T-011. Duplicate latest sequence, vary payload, use an old sequence and a gap. | Results and session table after replay. | Identical retry never reapplies; changed payload is an error. |
| AC-012 | FR-012 / T-012. Drop response after apply, change leader, retry identically. | One mutation and original result. | Response lost after commit resolved by retry with the same identifier. |
| AC-013 | FR-013 / T-013. Cut frames at every boundary; simulate short write/Sync errors and incorrect CRC. | Disk images and recovery decision. | Cutpoints before/after Sync preserve votes and acknowledged prefix. |
| AC-014 | FR-014 / T-014. Restart all with an uncommitted suffix and a lower commit checkpoint. | Reconstructed state before and after a new noop. | Crash and restart never expose an uncommitted suffix. |
| AC-015 | FR-015 / T-015. Install snapshot on a far-behind follower with chunk retries and term change. | Image hash, sessions and final indices. | InstallSnapshot restores identical state and deduplication. |
| AC-016 | FR-016 / T-016. Stop at every generation publication/cleanup cutpoint. | CURRENT and recovered generation for each case. | A crash at every phase leaves at least one recoverable generation. |
| AC-017 | FR-017 / T-017. Run election and timeout, inspect status and logs without user values. | Redacted log sample and metric snapshot. | Status and logs correlate an election and request without values. |
| AC-018 | FR-018 / T-018. Send shutdown during RPC and persistence; check workers and lock. | Duration, goroutines and subsequent recovery. | Shutdown within 5 s or reported termination followed by recovery. |
| AC-019 | FR-019 / T-019. Establish complete isolation and drop pending messages; then write on the majority and invoke a new old-leader GET. | History excluding successful stale reads. | Isolated leader never returns a local read as success. |
| AC-020 | FR-020 / T-020. Replay a scenario twice using the same seed and virtual clock. | Byte-identical event traces. | Two simulated replays have the same transition sequence. |
| AC-021 | NFR-001 / T-021. Enumerate small schedules with 3 nodes, 3 proposals and up to 2 faults. | Invariant report and explored bound. | Zero violations in the finite deterministic suite; no claim of exhaustive proof. |
| AC-022 | NFR-002 / T-022. Check 20 histories of at most 12 operations with CAS, retry and failover. | Witness order or reduced counterexample. | Checker finds a legal order for all specified small histories. |
| AC-023 | NFR-003 / T-023. Acknowledge data, crash one then all processes, recover intact directories. | Acknowledged-success list compared with later GETs. | No success lost in process-crash tests within the model. |
| AC-024 | NFR-004 / T-024. Stop a follower, restart it and await catch-up; then stop the leader leaving two healthy nodes. | Election timeline and resumption within local target. | After stabilization, with storage/CPU available and quotas unsaturated, resume within the 3 s local target. |
| AC-025 | NFR-005 / T-025. Run bounded benchmark under the resource profile. | Raw samples and methodology. | Report includes samples, errors, hardware and measurement interval. |
| AC-026 | NFR-006 / T-026. Reach dataset, queue and simulated-disk quotas without massive load. | Errors, RSS, CPU and peak space. | Quota yields a deterministic error; resource profile and peak disk recorded. |
| AC-027 | NFR-007 / T-027. Compare language trees, IDs, technical blocks and links; semantic review. | Review report with stated limitations. | File, identifier and link parity checked on every revision. |
| AC-028 | NFR-008 / T-028. Inject fake clock, PRNG, transport and storage into the same core. | Regression replay without real networking. | A regression reproduces from the saved manifest and seed. |
| AC-029 | NFR-009 / T-029. Record go version/GOARCH and build on the ARM64 Mac. | Build output and local suite. | Local build and tests record toolchain and architecture. |
| AC-030 | NFR-010 / T-030. Try binding 0.0.0.0, oversized input and broad permissions. | Rejections and actual permissions. | Public bind rejected, files private, oversized payloads rejected. |
| AC-031 | NFR-011 / T-031. Disable Internet while retaining loopback and run independent offline integration; present the demo at M16. | Dependency manifest and offline trace. | With external networking disabled, offline integration of planned operations succeeds; final presentation is separate. |
| AC-032 | NFR-012 / T-032. Review dependencies and procedure on already-owned hardware. | License, cost and prerequisite inventory. | Dependency inventory and procedure require neither payments nor mandatory accounts. |

## Gates and waivers

M09 closes only the roadmap MVP subset; M14 closes all correctness conditions; M15 closes the resource/performance report; M16 closes demo, budget and delivery. An inconclusive test stays open. Each waiver records the requirement, reason and risk in both languages; it never authorizes advertising an unverified guarantee. No waiver may introduce expenses or implementation in this phase.

## Mandatory subcases added by the audit

ACs remain Not run. AC-003/AC-009 require strict parsing and maximum CAS within a 48 KiB body; AC-008 includes commit backlog and full queues without duplicate apply; AC-013/AC-016 include surviving non-Sync writes and updated hard state at generation switch; AC-015 includes applyEpoch and a snapshot becoming obsolete during transfer; AC-019 distinguishes a new GET after isolation from a response for an already-committed read; AC-022 requires the retry-after-completion counterexample; AC-026 includes control-file reservation.

Early [roadmap](implementation-roadmap.md) subsets do not close these whole criteria. AC-031 uses independent offline integration at M14; demo presentation at M16 is not a circular prerequisite. Every subcase must appear in its primary test evidence; these are not new untraced requirements.

## Limited M01 closure

M01 is COMPLETED only within its official subset: local semantics, sequential CAS, validation, quotas, byte ownership and determinism verified by actual tests. [M01 report](milestone-m01.md). Distributed AC-008, HTTP/Raft-GET AC-009, concurrent AC-010, dedup AC-011 and cluster-resource AC-026 are not closed. No M02-or-later criterion is claimed passed.
