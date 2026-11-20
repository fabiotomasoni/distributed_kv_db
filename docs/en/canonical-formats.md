# Canonical formats v1 — C-01

[Index](README.md) · [Italiano](../it/canonical-formats.md)

**19 November 2026 — normative v1 baseline; local WAL/log recovery implemented; general snapshots and distributed integration remain future work.**

## Boundary and compatibility

C-01 requires byte-for-byte schema, kind/nullability, bootstrap, checksums and recovery fixtures before fixing M03 WAL/snapshot code. This document fixes that storage subset. Configuration/protocol fixtures are now implemented and checked in [M04](milestone-m04.md), with the exact [configuration hash](configuration.md); OpenAPI remains M08. [Static vectors](../fixtures/canonical-v1.json) and their [arithmetic checker](../fixtures/verify_canonical_v1.py) specify bytes, not a working WAL. [Review and gate decision](pre-m03-review.md).

M01 Command/Result are local Go values, with no serialized representation. M02 accepts indexed KV commands only; neither package gains sessions, noop handling or persistence here. A future replication adapter must map read to M01 get, process noop without a KV mutation while advancing the global apply boundary, and coordinate sessions atomically. It must not send an unknown noop opcode into M01 or skip its global index. That integration remains future work.

Persistent v1 is fixed for implementation and fixtures; an incompatible change increments the affected format version and requires a reviewed migration/new-directory plan. Unknown versions/kinds/fields fail closed; no rolling upgrade, guessing or silent downgrade. Envelope versions select nested entry/result schemas. Digest version is independently domain-separated. HTTP/JSON permits legal field order/escapes and transport metadata; it is validated and rebuilt from typed values, never copied directly into WAL or hashing bytes.

## Canonical JSON and scalar domains

Canonical bytes are ASCII UTF-8 with no BOM, whitespace, newline or trailing data. Every object sorts field names by ascending ASCII bytes, recursively; arrays retain their specified order. All schema strings are restricted to the ASCII domains below, so canonical strings use literal characters, without escapes (no slash escaping). Booleans are true/false. No floats, exponents, signed integers or negative zero. The only null is hard_state.voted_for. Optional fields are absent, never null. Duplicate names, unknown fields, wrong types and depth above 16 are errors. Persistent input must equal canonical re-encoding after full typed validation; accepting arbitrary map iteration or merely compacting JSON is insufficient.

| Type | Canonical representation / bounds |
|---|---|
| uint64 | JSON string: 0 or nonzero decimal without leading zeros, at most 18446744073709551615. |
| Versions | JSON number 1. Unknown versions rejected before interpreting payload. |
| Client sequence | JSON integer 1..9007199254740991, no leading zeros. |
| Key | Exact ASCII pattern [A-Za-z0-9][A-Za-z0-9._:/-]{0,255}; case sensitive, no normalization. |
| Value / expected bytes | RFC 4648 standard Base64, required padding, decode/re-encode equality; 0..16384 decoded bytes. Empty string is present empty bytes. |
| IDs / hashes | cluster_id and client_id: 32 lowercase hex; config_hash, command_sha256, snapshot_sha256, checksum: 64 lowercase hex. |
| Member / generation | n1, n2, n3; generation gen-N, N=1..MaxUint64 without leading zeros or path separators. |
| Counters | Index, term, base, counts and logical_bytes use uint64 strings. Index/term 0 only for empty-prefix/bootstrap metadata. Real entries have positive index and term. |

Validate declared byte lengths before allocation, then schema, bounds, checksum and semantic invariants before installing any effects. Overflow is fatal, not wrapping. KV limits remain 10000 keys and 4194304 logical bytes; at most 1024 sessions. No filesystem path is derived from a KV key. Synthetic IDs/hashes in fixtures identify format examples, not a verified M04 configuration.

## Entries and commands

Entry has exactly index, term, kind and (except noop) command. kind is noop, read, put, delete or cas. command has exactly these fields:

| kind | command fields |
|---|---|
| noop | command absent, not null; no client identity. |
| read | key only; no session. |
| put | key, value_b64, client_id, sequence. |
| delete | key, client_id, sequence. |
| cas | key, expected, new_value_b64, client_id, sequence. |

expected is exactly {exists:false} or {exists:true,value_b64:string}. No value field is allowed with exists=false. A local M01 nil put/CAS value projects to empty bytes; nil is never JSON null. Malformed external commands fail before log admission; a malformed stored entry is an integrity error. Local M01 INVALID_ARGUMENT tests remain valid and do not imply such commands must be persisted.

Maximum canonical entry: 49152 bytes including metadata. A maximum-size CAS at uint64-max index/term, 256-byte key, maximum sequence and two 16384-byte values is **44184 bytes**, as calculated by the fixture checker. This is distinct from the HTTP 48 KiB body cap, M02's raw 48 KiB copy cap and the 384 KiB peer/WAL payload caps.

## Command digests and operation results

For command identity use SHA-256 over this typed sequence, without a JSON intermediary: ASCII QCMD, one byte 01 (version), one opcode byte (noop=00, put=01, delete=02, cas=03, read=04). Except noop, append key as uint32 big-endian byte length followed by ASCII bytes. put appends length-prefixed raw value. cas appends one expected-exists byte 00/01, length-prefixed raw expected only when present, then length-prefixed raw new value. delete/read have no further bytes. Noop ends after opcode. No terminators or implicit padding. Length prefixes always count bytes.

Digest excludes index, term, client_id, sequence, timeout and correlation: the client_id/sequence pair is checked separately, digest compares the operation payload. Absent expected and present empty expected have different preimages. Get/read and noop preimages are defined for deterministic comparison, not session registration. SHA-256 is integrity/identity under the non-Byzantine model, not authentication.

Canonical application result contains code and index, plus the following exact union. No request_id/outcome/HTTP status is persisted. Internal worker/context errors are not domain results and must not be serialized as successful application.

| Result | Additional fields |
|---|---|
| OK for put/cas | applied:true. |
| OK for delete | deleted:boolean. |
| OK for read | exists:true, value_b64:string. |
| KEY_NOT_FOUND for read | exists:false. |
| COMPARE_FAILED for cas; RESOURCE_LIMIT for put/cas | applied:false. |
| SESSION_LIMIT, REQUEST_CONFLICT, STALE_SEQUENCE, SEQUENCE_GAP | No additional fields; index is the evaluating entry. Never replace a cached valid session result. |
| noop completion | code OK and index only; no cached session. |

Only mutating OK, COMPARE_FAILED and RESOURCE_LIMIT results enter a session cache; canonical result <=512 bytes, original applied index retained on retry. Input INVALID_ARGUMENT/INVALID_SEQUENCE and transport failures are not cached. No old value is stored in mutation results. Future M11 owns the session transition; these are schema decisions, not implemented behavior.

## WAL transaction framing

Exactly 24 header bytes: offsets 0..3 ASCII QWAL, 4..5 uint16 version=1, 6..7 uint16 kind=1 (transaction), 8..11 uint32 payload length, 12..19 uint64 physical sequence, 20..23 uint32 CRC32C, all integers big endian. Payload is canonical JSON, 1..393216 bytes. Frame size is 24+payload; no alignment, separator or trailer. Sequence starts at 1 per generation, contiguous, never reused for an append within that generation. Overflow stops the writer. Every generation starts with a transaction containing exactly hard_state and commit_index equal to its snapshot base S, with no suffix replacement; subsequent frames build the suffix before checkpointing a higher commit.

CRC32C uses Castagnoli, reflected polynomial 0x82f63b78, init/xorout 0xffffffff; check value for ASCII 123456789 is e3069283. Cover header bytes 0..19 followed immediately by payload; checksum field excluded. This is not IEEE CRC32, and not authentication.

Transaction is a nonempty object with any of these optional fields, all present values non-null:

- hard_state: exactly current_term and voted_for. voted_for is null or a static member. Term never regresses; within one term a recorded member cannot change or clear. Higher term may reset the vote. Term 0 requires null.
- replace_suffix: exactly from_index and entries array, which may be empty. Require commitIndex < from_index <= lastLogIndex+1 before replacement, contiguous entries starting at from_index, no decreasing terms across the retained boundary, and entry terms <= post-transaction current_term. Keep the prefix, replace the whole suffix. Never overwrite/delete a committed index.
- commit_index: nondecreasing, between the snapshot boundary and the post-replacement lastLogIndex. It checkpoints an externally established legal commit, never infers consensus from log length.

Atomically validate the post-state in this order: hard state, suffix, commit. No effect becomes visible from a partially valid frame. At most 16 entries and 262144 total canonical entry bytes per transaction, with the payload cap also enforced. These conservative limits fit the peer batch caps. Log conflicts append a replacement transaction, never physically truncate valid history. Physical generations can contain many transactions.

## Images, manifests and bootstrap

Snapshot file: ASCII QSN1 (4), payload length uint64 big endian (8), canonical payload, then 32 raw SHA-256 bytes over the preceding 12-byte header plus payload. Exactly 44+payload bytes, at most 8388608. The whole-file SHA-256, including trailer, is the RPC snapshot ID and CURRENT.snapshot_sha256; it differs from the embedded trailer digest.

Payload has exactly format_version, cluster_id, config_hash, last_included_index, last_included_term, logical_bytes, key_count, session_count, kv and sessions. kv is an array of objects {key,value_b64}, strictly ascending ASCII key, no duplicates. sessions is strictly ascending client_id, no duplicates; each record has exactly client_id, sequence, command_sha256, kind and result. kind is put/delete/cas; result is its cached union above with 1<=index<=snapshot index. Validate counts, decoded values, exact logical sum and all quotas. The empty snapshot is boundary (0,0), empty arrays and zero counters. A positive included index requires positive included term; included term <= recovered current term. Read/noop add no user data. Array ordering must be explicit, never filesystem/map iteration. A conservative encoded-size bound is ceil(4*4194304/3) + 10000*40 + 1024*768 + 4096 + 44 = 6782978 bytes, below 8 MiB: 40 covers each KV row/separator and Base64 rounding, 768 covers a session record including a <=512-byte result, and 4096 covers top-level metadata. This is format arithmetic, not a measured snapshot or storage allocation.

CURRENT and IDENTITY are canonical JSON files without newline, each <=4096 bytes. CURRENT data fields: format_version, generation, snapshot_sha256, base_index, base_term. IDENTITY data fields: format_version, cluster_id, node_id, config_hash. For each, checksum is lowercase SHA-256 of the canonical object with checksum absent; then include checksum and canonically encode the final object. Reject extra fields, wrong checksum, traversal and mismatched snapshot/base/identity/config. CURRENT is the only authority for selection; never pick the highest directory as recovery fallback.

Generation directory has exactly snapshot and wal as required data files. LOCK is a stable root-level advisory lock file, not a serialized database record. Generation names are local publication IDs, not Raft indices. Allocate a fresh unused N above recognized existing generation IDs under the lock, including staging; never reuse a name or wrap. This scan does not authorize selecting an unreferenced generation for recovery.

Explicit bootstrap requires a previously empty node directory, before creating LOCK. Create checksummed IDENTITY, an empty snapshot and WAL first transaction {hard_state:{current_term:"0",voted_for:null},commit_index:"0"}, physical sequence 1. Synchronize and publish using the same generation protocol as later. No empty WAL is valid. A partial bootstrap (including LOCK-only) is rejected by ordinary startup; no automatic delete/rebootstrap/resume.

At rotation write first transaction with current hard state and commit=S, append the complete retained S+1..last suffix in bounded transactions, then checkpoint the captured current commit after all needed entries. Sync all before CURRENT publication. If S=MaxUint64 the suffix is empty; never compute a wrapping S+1. Preserve current vote at the final barrier, not at snapshot capture. M13 implements rotation/installation; M03 must reserve these formats without implementing snapshots early.

## Recovery fixtures and closure criteria

[Fixture file](../fixtures/canonical-v1.json) fixes six entry/digest vectors, six result shapes, two WAL frames, empty and stateful snapshot bytes, IDENTITY/CURRENT checksums, six negative JSON cases and 18 recovery scenarios. Complete surviving non-Sync frames may be replayed; their presence does not prove a response was sent. Only a structurally incomplete final frame may be truncated to the previous verified offset and synchronized, under the process-crash/non-corruption model. Empty WAL, complete bad checksum, unknown version/kind, invalid schema, sequence gaps, vote/term regression, committed-suffix deletion or commit beyond log fail closed. Never scan for another magic. An incomplete first transaction cannot yield a valid generation.

A corrupt length can mimic a torn tail: automatic arbitrary-corruption repair is excluded. Suspected corruption or failure after an acknowledged boundary is fatal; retain evidence. Missing CURRENT with other files, incomplete bootstrap or a corrupt published generation is fatal. Unpublished staging never supersedes CURRENT; clean only after validating the selected generation. A surviving published generation is authoritative even if the previous one remains.

C-01 storage design is RESOLVED when: all byte/type/union/version rules above are fixed; bootstrap/publication/recovery decisions are unambiguous; the static vectors' canonical bytes, lengths and checksums are independently checked; future M03/M13 reader tests are required to consume both positive and negative cases. The checker validates fixture arithmetic, not recovery code. Actual codec/recovery conformance remains T-013/T-015/T-016 at their milestones; full config/protocol/OpenAPI fixtures retain the original later gates.

## Implemented preliminary subset

[Phase report](milestone-m03-preliminary.md): internal/storageformat now implements exact QWAL frame encode/decode, entry validation/typed digests and canonical CURRENT/IDENTITY sealing/validation. Approved fixture bytes are unchanged. DecodeFrame is stateless: it does not authorize truncation, validate cross-frame commit/votes or select a generation. Snapshot/result/session codecs and complete recovery conformance remain future work. C-01 design is resolved; full M03 T-013 is not closed.

## M03 conformance now implemented

[Final M03 evidence](milestone-m03.md): all 13 byte-image recovery cases and five namespace expectations are exercised. Typed decoded transactions reuse the exact canonical validator; no schema or approved fixture bytes changed. EmptySnapshot supports only the required bootstrap (0,0) image. FrameSize permits bounded streaming allocation and rejects contradicted partial magic/version/kind/length before treating a final tail as incomplete. WAL validates first transaction, physical continuity, persistent vote/term, suffix and commit atomically; only valid final EOF incompleteness can be truncated and synchronized. General snapshot/result/session application remains at later gates.
