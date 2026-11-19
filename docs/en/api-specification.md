# Client API v1 specification

[Index](README.md) · [Italiano](../it/api-specification.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Proposed HTTP contract

Every example in this document describes the future interface. No endpoint is available. HTTP/1.1 on loopback, `Content-Type: application/json`, no compression. All KV operations use POST to avoid ambiguous key paths and intermediary caches. Maximum body size is 48 KiB; reject with HTTP 413 before admission. Strict validation follows the [data model](data-model.md).

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

OpenAPI was evaluated: useful for client generation, but it would introduce a second contract to synchronize during design. In this revision, tables and examples are normative; M08 must derive OpenAPI 3.1, validate requests/responses as fixtures and resolve any divergence before declaring v1 stable. A validated OpenAPI specification does not yet exist.

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
