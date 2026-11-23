# Data model and transitions

[Index](README.md) · [Italiano](../it/data-model.md)

**Updated: 19 November 2026 — local M01 implemented and checked; distributed specifications remain design work.**

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

## Projection implemented in M01

M01 implements only map and byte counter with sequential operations and quotas: [local interface](milestone-m01.md). Sessions and lastApplied in the full model remain future work. Go commands carry raw bytes; nil in an admitted value means an empty sequence, while Expected.Exists distinguishes presence/absence. JSON null remains invalid in the future HTTP adapter. M01 has no serialization, snapshots or canonical format.

## Canonical storage projection

[Canonical formats v1](canonical-formats.md) fixes typed digest bytes, entry/result/session schema, counters and snapshot ordering. Local M01 Go values and M02 admission remain unchanged; future adapters must distinguish nil/empty bytes from JSON null and the read/noop log kinds from M01 operations.

## M07 application subset

[M07](milestone-m07.md) maps approved put/delete/cas bytes to unchanged M01 commands through M02 and handles noop inside a private worker bridge. Quotas/CAS failures are deterministic applied domain results. The complete session/sequence rules above remain M11: client_id/sequence stay validated/persisted metadata, with no cross-index deduplication in M07. Recent internal results are bounded to 64 indexed/termed slots and do not replace the future session table. Logged reads are not enabled.

## M11 deterministic session application

[M11](milestone-m11.md) implements the session rules above only for the explicit v2 application profile. An M02-owned wrapper around unchanged M01 checks (client_id,sequence) and the existing typed SHA-256 command digest. For a new session, capacity is checked first; at 1024 return SESSION_LIMIT without registration. Otherwise sequence must be 1. For a known session, last+1 applies and caches OK/COMPARE_FAILED/RESOURCE_LIMIT; last with equal digest returns the original compact result/index; last with different payload returns REQUEST_CONFLICT; lower is STALE_SEQUENCE; a gap is SEQUENCE_GAP. Protocol/session errors neither mutate KV nor replace the valid latest record.

Noop/read do not register or advance a session. Failed CAS and logical quota rejection consume an admitted session sequence. Retain only latest sequence/digest/kind/result/original index, no old values, no TTL/eviction, at most 1024 IDs and a <=512-byte canonical cached result. Older identities are never reapplied but their original results are not recoverable after advancing. Every duplicate still crosses a new committed application slot; actual apply/term progress is distinct from the original logical result index. Fresh replay reconstructs both map and sessions from the v2 committed prefix. V1 retains its historical metadata-only identity semantics.

## Consistent capture designed for M13

Future contract in the [pre-M13 review](pre-m13-review.md): map, counters and every session at the same S prefix, defensive copies and explicit ASCII ordering. Retain fingerprint/latest original result, including failed CAS/quota; no TTL, eviction or persisted GET outputs. Read/noop may end a snapshot with unchanged KV but advanced index. Restore reconstructs the atomic pair and then only S+1..commit; never renews results for old waiters. No export/import or snapshot API added now.

## M13 Phase A memory images

M01.CopyState returns sorted records/copied values and logical count; FromState validates everything before constructing a fresh machine without applying old commands. Sessions.CopySessions copies latest records in client order; SessionsFromState constructs a new wrapper with digest, sequence, kind, original result/index. Mutation results contain no value buffer. Both are owner-only APIs, not independently safe concurrent reads: capture calls them once inside the same parked M02 control. Isolated restore reconstructs KV/sessions and point S/termS together without importing into a node. Successful/failed/quota retry, stale/gap/conflict, DELETE, read/noop and 1024 cap checked after restore plus suffix. No eviction/general exactly-once. [Evidence](milestone-m13-preliminary.md).

## M13 Phase C — 23 November 2026

[Phase C](milestone-m13-runtime-recovery.md) is RUNTIME RECOVERY PHASE COMPLETED: RecoverCommitted restores S and applies only S+1..C through M02, then drains/closes the isolated worker before a fresh applier at C. WAL and snapshot validated under continuous LOCK. Startup is explicit through node.StartAPIWithSnapshotRecovery for v2; ordinary StartAPI/CLI retain the base-zero guard. No fallback on corrupt CURRENT/snapshot. KV/sessions, hard state and A=L=C verified with follower/leader/all-process crash/restarts. C-01/OQ-05 confirmed; recovery C-02/OQ-01 subset verified, overall C-02/C-03 CONDITIONALLY READY. Full T-015/AC-015/FR-015, T-016/AC-016/FR-016 and T-026/AC-026/NFR-006 open: compaction, cleanup and InstallSnapshot unimplemented. M13 PARTIALLY COMPLETED, M14 not started.
