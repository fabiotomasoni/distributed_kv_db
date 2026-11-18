# Data model and transitions

[Index](README.md) · [Italiano](../it/data-model.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Domain and encoding

Key: a non-empty ASCII string matching `[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}`; maximum 256 bytes. Exact, case-sensitive comparison without normalization. Value: opaque bytes from 0 to 16 KiB (16,384 bytes), transmitted as RFC 4648 Base64 with canonical padding; an empty string encodes a valid empty value. `null`, non-canonical Base64 and unknown fields are errors. JSON input must be valid UTF-8 before parsing; reject duplicate keys and non-integer numbers where integers are required.

The map distinguishes absence from an empty value: `exists=false` is not `value_b64=""`. There are no TTLs, JSON interpretation, numerical conversions or partial updates. `PUT("resource/7", JSON)` replaces the entire blob; modifying a property requires GET, client-side transformation and CAS against the entire previous value.

## Sequential function

| Operation | Logical precondition | Result and new state |
|---|---|---|
| PUT | Valid key and value; quota available. | Atomically replace/create; `OK`, `applied=true`. |
| GET | Valid key; committed barrier. | `OK` with bytes and `exists=true`, or `KEY_NOT_FOUND`. Does not mutate the map. |
| DELETE | Valid key. | Remove if present; `OK`, `deleted=true/false`. Absence is idempotent success. |
| CAS | `expected` is missing or value. | Exact byte comparison; match writes newValue and returns `OK`; otherwise `COMPARE_FAILED` without mutation. |

`expected={"exists":false}` provides create-if-absent. `expected={"exists":true,"value_b64":""}` compares a present empty value. `new_value_b64` is always a value, never deletion; use DELETE to remove. Two CAS operations with identical expected values and new values different from expected cannot both succeed without an intervening write. When CAS rewrites the same value, multiple successes are legal.

## Determinism and quotas

Logical state: `(map, logicalBytes, sessions, lastApplied)`. `logicalBytes = sum(len(key)+len(value))`, maximum 4 MiB per copy and 10,000 keys. Quotas are evaluated **during apply**, against ordered state; a leader precheck does not decide the result. DELETE releases bytes; replacing PUT/CAS subtracts the previous size. Exceeding the limit returns `RESOURCE_LIMIT` without map mutation, with a deduplicated result and consumed sequence for an admitted session.

No wall clock, randomness, unordered iteration, I/O or floats in transitions. Snapshots sort keys and client IDs lexicographically; canonical comparison hashes use explicit bytes. Counters are `uint64`; overflow causes controlled shutdown rather than wraparound. Index 0/term 0 represents the empty prefix.

## Sessions and identity

Mutating commands contain `client_id` (32 lowercase hexadecimal digits), `sequence` (1..2^53-1, JSON integer), operation and arguments. At most 1,024 client IDs are retained, with no TTL or automatic eviction. The first command must have sequence 1; registration occurs during apply. An unknown session at capacity returns `SESSION_LIMIT`, without mutation or registration. The ID is not a credential.

Each session stores its latest sequence, command digest and compact latest result (maximum 512 bytes; never old values). For `seq=last+1`, apply and store the result, including CAS failure or quota rejection. For `seq=last` with the same digest, return the original result; a different digest returns `REQUEST_CONFLICT`. For `seq<last`, return `STALE_SEQUENCE`; for `seq>last+1`, return `SEQUENCE_GAP`. These three errors do not advance the sequence. Identical rules apply after replay and snapshots. No historical result promise extends beyond the latest one.

The SHA-256 digest includes the opcode and typed fields with big-endian length prefixes, including the missing/value discriminator; it excludes deadlines, trace IDs and JSON whitespace. Cryptographic collision is outside the model. The client persists its ID, counter and pending payload before sending, keeps only one pending operation and never reuses a lost identity.

## Use examples

Configuration `config/theme` → bytes `dark`; inventory `stock/pen` → bytes `12`; counter `count/jobs` → GET then CAS from `12` to `13`; metadata `resource/7` → an entire document. These are design examples, not data from an existing server. Value-based CAS does not protect against ABA: A→B→A may go undetected. Version-based CAS is a future extension, not part of the initial API.
