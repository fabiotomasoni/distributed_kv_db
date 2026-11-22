# Client API v1 specification

[Index](README.md) · [Italiano](../it/api-specification.md)

**22 November 2026 — v2 profile implements PUT/GET/DELETE/CAS and replicated sessions; legacy v1 keeps M10 semantics.**

## Proposed HTTP contract

The broad contract and examples below describe the eventual v1 interface. The M10 GET and explicit M11 session-profile contract take precedence as described at the end. HTTP/1.1 on loopback, `Content-Type: application/json`, no compression. All KV operations use POST to avoid ambiguous key paths and intermediary caches. Maximum body size is 48 KiB; reject with HTTP 413 before admission. Strict validation follows the [data model](data-model.md).

| Endpoint | Specific required fields | Normal result |
|---|---|---|
| `/v1/kv/put` | key, value_b64, client_id, sequence | HTTP 200; code OK, applied true. |
| `/v1/kv/get` | key | HTTP 200 with exists true and value_b64; HTTP 404 KEY_NOT_FOUND with exists false. |
| `/v1/kv/delete` | key, client_id, sequence | HTTP 200; code OK, deleted boolean. |
| `/v1/kv/cas` | key, expected, new_value_b64, client_id, sequence | HTTP 200 OK or 409 COMPARE_FAILED. |
| `/v1/status` (GET) | None | HTTP 200, potentially stale local diagnostics. |

For `expected`, the union is exactly `{exists:false}` or `{exists:true,value_b64:string}`. A value field with exists false is invalid. All requests accept optional integer `timeout_ms` 50..5000, default 2000. The local timeout starts at HTTP admission and includes queuing, disk, replication and apply; the server uses a monotonic clock. The client chooses a deadline at least 250 ms longer, without assuming delivery is guaranteed.

## Envelope and complete examples

Indices and terms in all JSON responses are canonical decimal strings, consistent with the peer protocol. Every KV response contains `code`, `request_id` (transport correlation, new for each attempt) and `outcome`: `completed`, `not_admitted` or `unknown`. A `completed` response includes `index`, the original result's log index for retries, and operation fields. Parsing errors may lack a client request_id, but the server generates one. Errors and logs never include values.

Design examples; indices illustrate a possible history, not a measurement:

```http
POST /v1/kv/put HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 123

{"key":"config/theme","value_b64":"ZGFyaw==","client_id":"0123456789abcdef0123456789abcdef","sequence":1,"timeout_ms":2000}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 80

{"code":"OK","request_id":"r1","outcome":"completed","index":"8","applied":true}
```

```http
POST /v1/kv/get HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 22

{"key":"config/theme"}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 102

{"code":"OK","request_id":"r2","outcome":"completed","index":"9","exists":true,"value_b64":"ZGFyaw=="}
```

```http
POST /v1/kv/cas HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 159

{"key":"config/theme","expected":{"exists":true,"value_b64":"ZGFyaw=="},"new_value_b64":"bGlnaHQ=","client_id":"0123456789abcdef0123456789abcdef","sequence":2}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 81

{"code":"OK","request_id":"r3","outcome":"completed","index":"10","applied":true}
```

```http
POST /v1/kv/delete HTTP/1.1
Host: 127.0.0.1:7401
Content-Type: application/json
Content-Length: 82

{"key":"config/theme","client_id":"0123456789abcdef0123456789abcdef","sequence":3}

HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 81

{"code":"OK","request_id":"r4","outcome":"completed","index":"11","deleted":true}
```

## Errors and outcome certainty

| HTTP / code | Outcome | Client action |
|---|---|---|
| 400 INVALID_ARGUMENT / INVALID_SEQUENCE | not_admitted | Correct input; this attempt proposed no entry. |
| 413 REQUEST_TOO_LARGE | not_admitted | Reduce body; this attempt proposed no entry. |
| 503 NOT_LEADER / NO_LEADER | not_admitted | Try a known member; optional `leader_id` and `leader_url` are hints. |
| 429 OVERLOADED | not_admitted | Backoff, same identity for mutation retry. |
| 504 TIMEOUT | unknown | Same identity and payload; do not assume failure. |
| 503 UNAVAILABLE | unknown after admission | As TIMEOUT; no quorum or leadership changed. |
| 507 STORAGE_UNAVAILABLE | unknown after admission | Node stops participating; try a healthy node. |
| 404 KEY_NOT_FOUND | completed | Absence observed at the GET barrier. |
| 409 COMPARE_FAILED | completed | No mutation, sequence consumed; new logical attempt uses the next sequence. |
| 409 REQUEST_CONFLICT / STALE_SEQUENCE / SEQUENCE_GAP | completed | Session protocol error; no mutation or sequence advancement. |
| 409 SESSION_LIMIT | completed | New session not registered; do not rotate IDs to evade the cap. |
| 409 RESOURCE_LIMIT | completed | No mutation, sequence consumed for an admitted session. |

For session errors, index identifies the entry evaluating the error; retries of the latest valid operation return its original index. Retries are not answered from a follower's local cache: they pass through the leader and ordered path so that responses never depend on unconfirmed state.

`not_admitted` refers only to that attempt: an earlier attempt with the same ID may have executed. No automatic HTTP redirects and no follower forwarding; the client validates leader_url against its three configured addresses. Without a quorum, stale reads are not offered as a fallback.

## Write lifecycle

**Received**: bytes arrived, no guaranteed effect. **Recorded**: in-memory entry. **Locally persisted**: synchronized WAL frame; still revocable unless committed. **Replicated**: at least one peer acknowledged after Sync, but insufficient without the term rule. **Committed**: index covered by Raft's rule. **Applied**: state machine and session advanced. **Acknowledged**: completed response emitted after apply and durable majority replication. The client knows success only upon receiving and validating the response; a completed TCP write does not prove application receipt.

If a response is lost after commit, the command may already be applied. Retrying with identical `(client_id,sequence)` and payload returns the stored result without another effect while it remains the latest sequence. A client never advances to the next sequence until resolving the pending one; otherwise it may lose the ability to retrieve the historical result. Deduplication means at-most-once effect per identity within the model, not exactly-once delivery.

GET does not use a session: each retry is a new read and may observe a later value. Mutation retries: initial backoff 50 ms, doubling to 400 ms with deterministic jitter in tests; total client budget 5 s, then retain the still-uncertain outcome. Cancellation never removes an already-proposed entry.

## Concurrency and schema

The log orders all operations; overlapping requests have no promised HTTP arrival order. A CAS result concerns the value at its index. There is no cross-key atomicity. HTTP 200 from status proves neither quorum, durability nor linearizability.

OpenAPI was evaluated: useful for client generation, but it would introduce a second contract to synchronize during design. In this revision, tables and examples are normative; M08 must derive OpenAPI 3.1, validate requests/responses as fixtures and resolve any divergence before declaring v1 stable. The M08 subset now has the checked artifact linked below; full v1 stability remains gated by M10/M11.

## Negative-outcome and retry examples

Complete proposed responses for a CAS comparing a different value and for the same admitted request without a response before deadline:

```json
{"code":"COMPARE_FAILED","request_id":"r5","outcome":"completed","index":"12","applied":false}
```

```json
{"code":"TIMEOUT","request_id":"r6","outcome":"unknown"}
```

After r6 retry the identical mutating body with the same client_id/sequence, without advancing sequence or changing expected. If it was already applied at index 13, the retry may respond as follows, with new correlation and original index:

```json
{"code":"OK","request_id":"r7","outcome":"completed","index":"13","applied":true}
```

This example does not count as two effects: applied=true describes the original operation's result. If the key is missing, GET returns HTTP 404 and `{code:"KEY_NOT_FOUND", request_id:"r8", outcome:"completed", index:"14", exists:false}`; this inline notation describes fields rather than a JSON payload to send.

## CAS limit compatibility

The body cap is 49,152 bytes (48 KiB). Two raw 16,384-byte values occupy 43,696 Base64 bytes; a compact CAS JSON request with a 256-byte key, client ID, maximum sequence and timeout occupies 44,116 bytes. Thus all admitted values are representable in a compact request; excessive whitespace may still exceed the cap. Rebuild the log entry from validated fields rather than copying whitespace or transport fields, keeping it within 48 KiB. T-009 must include this exact boundary and rejection at cap+1.

## Persistent representation boundary

[Canonical formats](canonical-formats.md) defines the future internal entry/digest/result projection. Wire field order and allowed escapes are not persisted verbatim; request_id, outcome and deadlines are transport fields. Existing HTTP status and body limits remain unchanged. OpenAPI/full wire fixtures remain M08/M04 gates, not satisfied by storage fixtures.

## M08 implemented subset and precedence

[M08 report](milestone-m08.md), [OpenAPI 3.1 JSON](../openapi/m08.json), [HTTP fixtures](../fixtures/http-m08.json). This section controls implemented behavior; retry/session/GET/CAS examples above remain future design. node.StartAPI binds the configured client port after storage validation/replay and both listener reservations. POST /v1/kv/put and /v1/kv/delete use the documented required fields; optional timeout_ms is 50..5000, default 2000. GET /v1/status returns only node_id, role and canonical decimal-string term/commit_index/last_applied; it is stale diagnostics. KV GET and CAS return 404 INVALID_ARGUMENT/not_admitted. Wrong methods/media return 405/415 INVALID_ARGUMENT/not_admitted. No redirect or forwarding.

Strict lexical/schema validation and the 49,152-byte cap precede proposals. Deadlines start at handler admission and include parsing. The request_id is generated per attempt; supplying it as a request field is invalid. Only validated command fields enter the unchanged WAL schema. Completed requires durable current-term majority commit, exact index/term application and a serialized owner leadership fence followed by a conservative HTTP status check. RESOURCE_LIMIT is 409/completed with index; successful PUT has applied=true and DELETE has deleted=true/false. Missing-key DELETE is 200/deleted=false.

A ticket canceled before recording is permanently excluded under its admission lock: UNAVAILABLE/not_admitted for an expired/canceled wait before recording. After possible recording, deadline is TIMEOUT/unknown; observed step-down, cancellation or infrastructure failure is UNAVAILABLE/unknown, or STORAGE_UNAVAILABLE/unknown for storage failure. Explicit pre-write WAL reserve rejection is STORAGE_UNAVAILABLE/not_admitted. NOT_LEADER/NO_LEADER and OVERLOADED are not_admitted only before recording. All certainty concerns this attempt alone. Client disconnect does not remove committed work. An unobserved loss after a legal quorum/fence cannot revoke the past commit; TCP delivery is not atomic with role changes.

There are no sessions yet. Valid client_id and sequence are persisted metadata, not deduplication proof; repeating the same DELETE can return true and then false at different indices. No automatic retry, at-most-once or exactly-once claim, nor recovery of an original unknown result. The bounded single-attempt test client accepts only configured members, rejects nonconfigured hints/redirects and reports transport failure as unknown. Resolve uncertainty externally until M11; the future retry instructions above are not enabled in M08.

Limits: 32 accepted client connections including idle, 64 handler write slots, 64 service write waiters/queued tickets and 64 completion-fence messages, separate from 256 peer events. Canceled queued tickets can occupy bounded queue space until the owner drains it. Four active test-client calls maximum; no ambient proxy, compression or redirect. Header/body/write/idle bounds are 500 ms/5 s/6 s/1 s. Slow bodies can consume the 5 s read cap; an already-expired requested deadline is excluded before append. OpenAPI assertions and runtime lexical/canonical extensions are checked offline with fixtures and negative controls; this is not external OpenAPI certification. Full FR-009/AC-009 and FR-012/AC-012/T-012 remain gated at M10/M11.

## M09 compatibility and restart

[M09](milestone-m09.md) exposes the same M08 endpoints through the node executable; OpenAPI/HTTP fixtures and errors are byte-unchanged. Restart replays committed state before listening and begins as follower, with fresh per-attempt correlation. An unknown operation may disappear or become committed later through a new-term noop; restart does not recover an original client result or add deduplication. GET/CAS HTTP remain unavailable. SIGTERM can lose a response while preserving the command's existing Raft obligations.

## M10 implemented GET contract and precedence

[M10](milestone-m10.md) now adds POST /v1/kv/get with required key and optional canonical integer timeout_ms 50..5000, default 2000. No client_id, sequence, value or request_id is accepted in the request. HTTP GET on this KV path is 405; GET /v1/status remains stale local diagnostics. CAS HTTP and session/deduplication semantics remain future M11. This section extends and takes precedence over the historical M08/M09 availability statements; PUT/DELETE behavior is unchanged.

The node executable/StartAPI enables logged reads on all three members. Every attempt appends a new current-term canonical read(key), synchronizes/replicates it, commits through the existing durable-majority rule/checkpoint and captures its result while M02 applies that exact index. The handler does not read a local map. Completed requires the matching applied index/term, an owner-serialized leadership fence and conservative final HTTP status/deadline checks. Previously completed mutations precede later successful reads; overlapping calls have no HTTP arrival-order promise.

200 OK/completed includes canonical string index, exists=true and canonical value_b64, including an empty string for a present empty value. 404 KEY_NOT_FOUND/completed includes index and exists=false, with value_b64 absent. Unknown/not_admitted replies omit read-result fields. Body/key/value/connection/queue bounds, generated request_id, configured leader hints and deadlines retain M08 semantics. A follower/candidate cannot serve a local read; an isolated old leader cannot complete a new read begun after full isolation. A read already legally committed before isolation may return its captured result if completion checks still hold. No past quorum is revoked.

A proved pre-record exclusion is not_admitted. After possible read recording, timeout is TIMEOUT/unknown, observed step-down/cancellation/infrastructure error is UNAVAILABLE/unknown, storage failure STORAGE_UNAVAILABLE/unknown. Canceling/disconnecting abandons the wait, not committed application. Read retries are independent later reads and may see newer values; Client.Get performs a single attempt with no automatic retry. No persistent client identity or exactly-once claim.

[OpenAPI M10](../openapi/m10.json) and [GET fixtures](../fixtures/http-m10.json) extend the immutable [M08 artifact](../openapi/m08.json)/[fixtures](../fixtures/http-m08.json). Tests compare all original path schemas exactly and check schemas, strict runtime decoder, emitted HTTP status/body and test-client decoding. Canonical lexical rules exceed ordinary JSON Schema annotations and are checked in Go; this is offline finite contract evidence, not external OpenAPI certification. Logged reads consume WAL/commit writes and the existing physical reserve/quota. No ReadIndex, lease or compression.

## M11 CAS, sessions and versioned wire contract

[M11](milestone-m11.md), [OpenAPI M11](../openapi/m11.json) and [HTTP fixtures](../fixtures/http-m11.json) apply to configuration/application v2 from [ADR-010](adr/ADR-010-session-profile.md). POST /v1/kv/cas requires key, expected, new_value_b64, client_id and sequence, with optional timeout_ms 50..5000/default 2000. Expected is exactly exists=false, or exists=true with canonical value_b64; absent and present-empty differ. CAS performs the comparison/replacement in one unchanged M01 transition, never GET then PUT. A successful CAS is 200 OK/completed/applied=true; mismatch is 409 COMPARE_FAILED/completed/applied=false. Create-if-absent is expected.exists=false; a same-value CAS may legally succeed repeatedly for different identities. ABA protection by versions is not provided.

All v2 PUT/DELETE/CAS requests must include exactly one X-QuorumKV-Application-Version: 2 header. V2 mutation responses acknowledge that header; Client.Write/CAS requires the acknowledgment before trusting a result. Missing/wrong request versions are 400 INVALID_ARGUMENT/not_admitted. The current v1 mode rejects that header, keeps original semantics and does not expose CAS. This explicit profile prevents retroactive deduplication of old committed logs. GET schemas and body fields of PUT/DELETE are unchanged; the original M08/M10 artifacts remain immutable. V2 uses new directories and does not migrate old data.

Mutation identity is (client_id,sequence), not the generated per-attempt request_id. A latest identical retry returns the saved application code/booleans and ORIGINAL index, with fresh transport request_id; deadline and legal JSON representation differences do not change the typed command digest. Changed arguments at the same latest sequence return 409 REQUEST_CONFLICT. Lower/gapped sequences return 409 STALE_SEQUENCE/SEQUENCE_GAP. New IDs at the 1024-session cap return 409 SESSION_LIMIT. These protocol errors are completed at the evaluating index and omit operation fields; they do not advance or overwrite a valid cached session. RESOURCE_LIMIT is 409/completed/applied=false for PUT/CAS and consumes the admitted sequence, as does COMPARE_FAILED. DELETE absence remains OK/deleted=false and is cached too.

Every retry proposes a fresh current-term entry and waits for legal quorum commit/checkpoint, exact M02 application and the CURRENT attempt's leadership fence. HTTP reports the original cached index only after that fence. Neither followers nor isolated old leaders can return a cached mutation as a shortcut. A duplicate still consumes a physical log slot; it does not repeat the logical mutation. Concurrent duplicates with matching identity/digest return the same original result. Different clients with sequence 1 are independent.

Not_admitted/unknown, deadline, cancellation, backpressure and storage-failure semantics remain M08. Unknown still may have taken effect. The caller must keep the same client_id/sequence and logical payload for a retry and only advance for a new operation; the test client remains single-attempt with no automatic backoff/identity persistence. A caller needing restart-safe request tracking must retain its pending identity/payload itself. GET retries remain fresh reads without sessions. Latest-result recovery lasts until that client's next admitted sequence; older sequences are rejected forever while the session highwater survives, without promising their historical results. There is no TTL/eviction, general exactly-once delivery, arbitrary old-result query or retry liveness guarantee when quorum/storage is unavailable.
