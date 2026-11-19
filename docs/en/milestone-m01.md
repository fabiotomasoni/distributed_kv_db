# M01 — Deterministic state machine

[Index](README.md) · [Italiano](../it/milestone-m01.md)

**Status: COMPLETED — 19 November 2026. M02 has not started.**

## Scope checked before implementation

Read both [architecture audits](architecture-review.md), roadmap, requirements, data model, API, consistency, architecture, ADR-001/003/006/007, tests and acceptance criteria. Official M01 is the T-008/T-009 subset for the local sequential machine, including quotas and CAS. There is no material discrepancy with the assignment. Gates C-01/C-02 concern future persistence formats and filesystem adapters: M01 does not use them.

Implemented only the local `(map, logicalBytes)` projection. Sessions, client counters, deduplication, indices and consensus barriers remain in their scheduled milestones. Atomicity means one undivided sequential transition; no concurrent-access, distributed-linearizability or durability guarantee is claimed.

## Files and components

| File | Responsibility |
|---|---|
| [go.mod](../../go.mod) | Module matches the Git remote and requires Go 1.27.4 or later; no require or external dependency. |
| [doc.go](../../internal/statemachine/doc.go) | Package boundaries, sequential use and ownership. |
| [command.go](../../internal/statemachine/command.go) | Operation, Command, Expectation, Result, Code and approved limits. |
| [validate.go](../../internal/statemachine/validate.go) | ASCII grammar, sizes and command shape before any mutation. |
| [machine.go](../../internal/statemachine/machine.go) | Private map, apply-time quotas and deterministic operations. |
| [machine_test.go](../../internal/statemachine/machine_test.go) | Semantics, atomic rejection, sequential CAS and buffer isolation. |
| [limits_test.go](../../internal/statemachine/limits_test.go) | Exact value, key-count and logical-byte limits, and released quota. |
| [determinism_test.go](../../internal/statemachine/determinism_test.go) | Independent list/string oracle, replay and independent instances. |

Also updated the root README, shared docs index and, in both languages, the index, overview, roadmap, data-model, architecture, development-guide, testing-strategy, acceptance-criteria and traceability. Reports dated 18 November remain historical design-phase evidence rather than descriptions of current code. This report and the roadmap give current progress.

## Local Go interface

Package `github.com/fabiotomasoni/distributed_kv_db/internal/statemachine`:

```go
func New() *Machine
func (m *Machine) Apply(c Command) Result
```

`Command` contains Operation (`Put`, `Get`, `Delete`, `CAS`), string Key, opaque-byte Value and optional Expected. `Expectation{Exists:false}` compares absence; `Expectation{Exists:true,Value:...}` compares exact bytes. `Result` contains Code and the operation-specific Value/Exists, Applied or Deleted fields. Codes are `OK`, `INVALID_ARGUMENT`, `KEY_NOT_FOUND`, `COMPARE_FAILED`, `RESOURCE_LIMIT`. Domain outcomes are values, without I/O errors or implicit exceptions.

Non-CAS mutations forbid Expected; GET/DELETE forbid Value even for an empty non-nil slice; CAS requires Expected. `Expectation{Exists:false}` with non-nil Value is invalid. A nil PUT/CAS Value, or nil Value for a present expectation, represents an empty byte sequence. This is not JSON null: JSON/Base64 and their validation are not implemented. The future adapter must reject null under the wire contract. No JSON tags, encoder or persistence format is defined here.

Machine's zero value is usable. Never copy Machine after first use; create separate instances. The caller serializes Apply. Values are copied on input/output, so mutating a command or returned result cannot change state. No mutex, goroutine, worker or scheduler was introduced: those belong to M02.

## Local semantics and invariants

Case-sensitive ASCII keys `[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}`, raw values up to 16 KiB, 10,000 keys and 4 MiB of `sum(len(key)+len(value))`. The byte counter is uint64; validated sizes bound arithmetic. Replacement subtracts the old value without counting the key again. DELETE releases key/value bytes; absence succeeds with Deleted=false.

PUT replaces the entire value. GET distinguishes absence from empty value. CAS checks existence and bytes, then performs PUT's quota checks within one call; comparison/quota failure leaves both map and counter unchanged. A CAS rewriting the same value may succeed multiple times; value CAS does not protect against ABA. Partial JSON modification does not exist.

No time, randomness, I/O, mutable global state or map iteration determines results. Tests recompute byte sum/key count after every sequence step. Ordered state views exist only in tests and are neither a snapshot API nor a canonical encoding.

## Actual tests and traceability

| Subset | Implemented evidence | Closure limit |
|---|---|---|
| T-008/M01, FR-008 | 128 reproducible commands, test seed 19, three separate instances including zero value, per-step results/state checked against a list-of-strings oracle. Fixed-sequence replay and instance independence. | Distributed AC-008 and M02/M07 scheduling remain open. |
| T-009/M01, FR-009 | Insert/read/update, repeated DELETE, absence, empty/binary values, case sensitivity and whole-document replacement. | No HTTP API or Raft reads; full AC-009 remains open. |
| Local CAS semantics, FR-010 | Match/mismatch, missing/present-empty, create-if-absent, unchanged value, both sequential competition orders. | Not concurrent T-010; sessions/failover remain M11. |
| Logical quotas, NFR-006 | 0/1/16 KiB values, 256/257-byte keys, exactly 10,000 keys, exactly 4 MiB and one-byte overflow; allowed replacement and reusable space after shrinking/deleting. | Cluster RAM, CPU, WAL and disk are not measured or implemented. |
| Rejection and ownership | 19 invalid commands, unchanged state/counter, no input/output alias, validation before CAS comparison. | JSON parsing and transport security remain future work. |

The key-limit fixture is built privately in tests, avoiding 10,000 requests; it is checked before exercising the boundary. The byte fixture uses fewer than 300 transitions and roughly 4 MiB of logical data. These are not load tests. The fixed-seed PRNG belongs only to tests, never application code.

## Checks actually performed

Toolchain **go1.27.4 darwin/arm64** downloaded from the [official Go index](https://go.dev/dl/?mode=json) into `/tmp/quorumkv-m01-toolchain`, without global installation. Archive SHA-256 checked against the index: `91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4`. The toolchain is a development prerequisite rather than a package runtime dependency.

Commands ran from the repository with GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, CGO_ENABLED=0, GOMAXPROCS=2 and temporary cache/GOPATH:

```sh
gofmt -w internal/statemachine/*.go
go test -p 1 -count=1 -timeout=30s ./...
go vet -p 1 ./...
go test -json -p 1 -count=3 -cover -timeout=30s ./...
go list -m all
go list -f '{{.ImportPath}}: {{join .Imports ", "}}' ./...
gofmt -l internal/statemachine/*.go
```

Outcome: **PASS**, 12 top-level tests and 44 subcases per execution. The first suite passed; three subsequent repetitions produced 168 pass events and no failures, with **100.0% statement coverage of this package only**. Go reported 0.232 s for the first run and 0.262 s for the three-repeat run; these are not benchmarks. `go vet` exited 0; `gofmt -l` returned no output. The module is the only entry in `go list -m all`; application code imports only standard-library `bytes`.

Final documentation check: 100 Markdown files and 49 language pairs, 1,075 resolved relative links, 72 tables, aligned sections/identifiers and 15 matching pairs of non-Mermaid technical examples. No detected errors; git diff --check reported no issues.

No race detector, concurrency tests, Raft simulation, benchmarks or storage tests ran. No database RAM/CPU/SSD measurements. Coverage and finite replay do not prove correctness for every possible sequence. After the initial download, compilation/tests required no network services or dependencies.

## Outcome and next work

**M01: COMPLETED.** The entire official local subset is implemented and required checks passed; no known issue violates M01 invariants. No additional spending, external dependency, infrastructure or persistence format was introduced. Audit gates for M03 and beyond remain open.

M02 can proceed without known issues originating in M01, designing ownership/scheduling around the sequential core. **M02 has not started**: this delivery stops at M01 and remains available for review.
