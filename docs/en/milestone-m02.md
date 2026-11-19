# M02 — Ordered applier and bounded scheduling

[Index](README.md) · [Italiano](../it/milestone-m02.md)

**Status: COMPLETED — 19 November 2026. Local M02 subset only; no distributed guarantees.**

## Scope and resolved discrepancy

The earlier assignment asked for directly thread-safe Machine.Apply, while the approved [roadmap](implementation-roadmap.md) requires ownership, applier scheduling, bounded queues, worker shutdown and fakes. The preliminary attempt was BLOCKED before Go changes or new tests; it created the two reports and index links only. The new assignment explicitly resolves the discrepancy in favor of the roadmap. This report supersedes that blocking status and preserves its reason. No audit finding or future acceptance gate is removed.

Read Git state, roadmap, architecture, state-machine specification/code, both language audits, applicable ADR-003/ADR-008, acceptance criteria and the earlier reports. The [audit](architecture-review.md) permits isolated M01/M02; its C-01/C-02 storage conditions remain open. The existing M01 package is unchanged.

## Architecture and ownership

New [internal/applier](../../internal/applier/doc.go) has one worker per instance and a fixed ring queue. New transfers exclusive ownership of a concrete non-nil Machine to that worker until Done closes. No other goroutine may call that machine, including through another applier. Metadata uses a mutex; Apply runs outside that mutex, serially. RWMutex would not establish command order or ownership and is unnecessary for ordered GET.

The sole production dependency is the small Machine.Apply interface. Apply must return, must not reenter the applier and must not call runtime.Goexit. M01 meets that contract. An arbitrary dependency that never returns cannot be forcibly stopped safely; contexts limit waits rather than terminating its call. A failed machine has uncertain state and must not be reused as healthy merely because its worker ended.

Submit(ctx,Entry) admits work and returns a Future; TrySubmit is nonblocking. Future.Wait(ctx) returns Outcome{Index,Result}. Progress exposes a synchronized snapshot of State, Capacity, Queued, Active, Scheduled and LastApplied. Done is the worker completion boundary; Shutdown waits for it.

Inputs must remain unchanged throughout Submit/TrySubmit, including a space wait. Successful admission copies operation/key strings, values and Expected, preserving nil versus empty bytes. After return the caller may mutate its originals. Every successful Wait receives its own result buffer. Use the returned Future pointer without copying it; a dependency transfers its result buffer to the worker when Apply returns and must not mutate it afterward. Futures are per request; the applier retains no historical results table or per-request goroutine.

## Ordering and failure contract

The caller supplies global uint64 indices already ordered by its source; M02 neither assigns log indices nor establishes commit. InitialIndex defaults to 0 and must match the supplied machine's already-applied state. The first accepted index is InitialIndex+1. No recovery or snapshot loading is implied.

| Condition | Outcome |
|---|---|
| Next contiguous index | Accept once; Scheduled advances on admission. |
| Index at or below Scheduled | ErrDuplicate; no second Apply or cached-result lookup. Zero is the initial sentinel. |
| Index greater than Scheduled+1 | ErrGap immediately; no reorder buffer or waiting for holes. Caller retains and retries in order. |
| Scheduled equals MaxUint64 | ErrIndexExhausted takes precedence over index classification; no wraparound. |
| Small malformed command or ordinary M01 domain failure | Accepted index is consumed; Result carries INVALID_ARGUMENT, COMPARE_FAILED or other domain code; LastApplied advances. |
| Apply panics, including panic(nil) | Failed state; current future ErrApplyFailed with uncertain effect, pending futures ErrAborted and not applied. No automatic retry. |

Concurrent arrival and request creation time do not define application order. Multiple producers may race admission; only the next contiguous index can win. Rejected entries remain the producer's responsibility. The future Raft core is the single scheduling authority: retain committed entries, retry from Scheduled+1 and use TrySubmit while draining completions. Queue rejection must never mean deleting a committed log entry. Logical duplicate client operations at different indices require future replicated-session deduplication, not M02 index rejection.

LastApplied advances only after Apply returns a domain result; it never advances on enqueue or panic. M02 maintains LastApplied <= Scheduled. The future core must additionally enforce Scheduled <= commitIndex. No applyEpoch reset or snapshot barrier is implemented; those belong to the later snapshot work. Futures belong to one applier lifetime.

## Queue, backpressure and lifecycle

Capacity counts pending entries, excluding at most one active call. Default 64, configurable 1..1024; zero selects the default. Copied raw command fields are capped at 48 KiB in total before admission, with ErrTooLarge and no index advance. This bounds input retention even for malformed commands; it is not a serialization or WAL format, and does not change M01 key/value quotas. Clients retaining futures or launching unbounded producers remain responsible for their own memory.

A full queue makes TrySubmit return ErrFull. Submit waits efficiently on a change notification or its context; no busy waiting and no helper goroutine. Cancellation before admission returns no Future and consumes no index. If cancellation races admission, either rejection or a valid Future is possible; the return value determines acceptance.

Normal lifecycle is Running → Draining → Stopped. Shutdown atomically stops admission, wakes blocked producers with ErrClosed and drains all accepted work in order. Repeated/concurrent Shutdown calls are safe. Its canceled/deadline context only ends that caller's wait; draining continues. New work is rejected after closing. There is no concurrent send/close on a work channel.

Cancellation of Future.Wait removes neither queued nor active work. A context error leaves the outcome unknown to that waiter; the same Future can be awaited again. An already-published result can be obtained even with a canceled context. Completion stores the result and closes a notification without requiring a receiver, so abandoned waiters cannot block the worker. Done closes after all accepted futures have terminal outcomes. An application panic transitions to Failed and Shutdown returns ErrApplyFailed instead of reporting a successful drain.

## Tests and actual evidence

20 new top-level tests cover ordered backlog 1..5 then 6..7 before completion, duplicate/gap/exhaustion handling, 16 concurrent producers retaining rejected work, exact result correlation, domain failures, input/output copying, bounded admission, saturated queues and resumable backpressure. Lifecycle tests cover all cancellation stages, controlled deadline expiration, ordinary/repeated/concurrent shutdown, closing during active Apply, blocked-producer wakeup, unobserved results, independent instances, idle worker completion and application panic.

Test-only gateMachine, manualClock, fakeTransport and fakeStorage control delays, duplicate delivery, injected refusal and cleanup. They are tiny harness boundaries, not a network stack, durable store or generic simulator. No unused production clock/transport/storage abstraction was added. Real timing/election behavior and full replay remain future work. Channels/barriers coordinate tests; no time.Sleep or global goroutine-count assertions. Worker termination is observed through Done and producer completion through joins/response channels; test contexts are bounded to 3 s, package timeout 30 s.

Executed locally with Go 1.27.4 darwin/arm64, GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOMAXPROCS=2, temporary cache/toolchain paths. Ordinary checks use CGO_ENABLED=0; targeted race uses CGO_ENABLED=1 with existing Apple clang 21.0.0. From the project root, equivalent installed-toolchain commands are:

```sh
gofmt -w internal/applier/*.go
go vet ./...
go test -p 1 -count=3 -cover -timeout=30s ./...
go test -race -p 1 -count=1 -timeout=30s ./internal/applier
go list -m all
```

| Executed check | Result |
|---|---|
| M01 independently before implementation | Passed. |
| gofmt and go vet ./... | Passed. |
| Full suite, three repetitions | Passed: 20 M02 tests and 12 M01 tests with 44 M01 subcases per repetition. |
| Statement coverage over those runs | applier 99.3%; statemachine 100.0%. |
| Targeted race detector, one verified run | Passed; no data races reported in exercised paths. |
| Dependencies | Unchanged go.mod, no go.sum, only standard library and local statemachine; no external module. |

The absence of race reports proves neither correctness across every interleaving nor distributed safety. Coverage is statement coverage, not proof of correctness. No consensus/checker, server shutdown, crash recovery, benchmark or RSS/CPU/SSD measurement was run. Documentation checks passed: 102 Markdown files, 50 language pairs, 1,123 relative links, 76 tables and 16 matching pairs of non-Mermaid technical examples; no structural errors detected. These checks do not establish semantic translation equivalence or render Mermaid. Audit and M01 historical reports retain their original evidence dates.

## Files and roadmap acceptance

Created four production files: [applier.go](../../internal/applier/applier.go), [types.go](../../internal/applier/types.go), [future.go](../../internal/applier/future.go), [doc.go](../../internal/applier/doc.go); four test files: [scheduling_test.go](../../internal/applier/scheduling_test.go), [lifecycle_test.go](../../internal/applier/lifecycle_test.go), [dependencies_test.go](../../internal/applier/dependencies_test.go), [helpers_test.go](../../internal/applier/helpers_test.go). No M01 source changed.

Updated this report and its translation, shared/language indexes, architecture, roadmap, testing strategy, traceability, consistency, development guide and applicable ADR addenda in both languages. The old blocked status is resolved here; historical audits remain intact.

T-008/M02 and T-018/M02 are Passed, with T-028 fake foundations available. Full AC-008 remains at M07, server AC-018 at M09 and replay AC-028 at M12. All M02 completion criteria are met within that approved subset. M03 is not started: it may be addressed after review and closure of the applicable C-01/C-02 gates (canonical formats/fixtures and macOS storage contract). No Raft, networking, WAL, snapshots, disk database, server, deployment or automatic Git commit. No external service, paid API, dependency installation, additional hardware or spending; €0, small synthetic datasets and bounded runs only.

## Subsequent review

The [pre-M03 review](pre-m03-review.md) reran M01/M02 and added one significant cancellation-boundary test: current applier count 21, coverage 100.0%, targeted race passed. The original 20-test/99.3% figures above remain historical delivery evidence. Production code unchanged; C-01 design resolved and C-02 validation still conditional/open as recorded in the review.
