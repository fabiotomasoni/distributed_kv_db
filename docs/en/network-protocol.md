# Peer protocol and transport

[Index](README.md) · [Italiano](../it/network-protocol.md)

**20 November 2026 — M04–M07 peer transport, elections, replication and commit propagation implemented and checked; client/snapshot effects remain future work.**

## Transport and addresses

Implemented transport v1: persistent HTTP/1.1 over TCP, UTF-8 JSON, standard-library `net/http` and `encoding/json`. Compared with binary RPC: more bytes and parsing, but inspectability and no generator or dependency. TCP does not eliminate timeouts, duplicate attempts or obsolete responses. No TLS in the trusted loopback model, no compression, no ambient proxy for peer connections.

| Node | Client API | Peer RPC | Proposed directory |
|---|---|---|---|
| n1 | 127.0.0.1:7401 | 127.0.0.1:7501 | .local/n1 |
| n2 | 127.0.0.1:7402 | 127.0.0.1:7502 | .local/n2 |
| n3 | 127.0.0.1:7403 | 127.0.0.1:7503 | .local/n3 |

Every POST RPC includes `protocol_version:1`, `cluster_id` (32 hex), `config_hash` (SHA-256 hex), `from`, `to`, process-unique `rpc_id` and uint64 `term`. JSON indices/terms are canonical decimal strings (no sign or leading zeros except "0") to avoid precision loss; client sequence remains a number in its restricted domain. Responses echo rpc_id, current term and result. Unknown identity or mismatched cluster/version/config: HTTP 400/409 before core delivery, without updating the term.

## Normative messages

| RPC | Request beyond envelope | HTTP 200 response beyond envelope |
|---|---|---|
| `/raft/v1/request-vote` | candidate_id=from, last_log_index, last_log_term | vote_granted boolean |
| `/raft/v1/append-entries` | leader_id=from, prev_log_index, prev_log_term, entries[], leader_commit | success boolean, match_index on success; conflict_index and optional conflict_term on failure |
| `/raft/v1/install-snapshot` | leader_id=from, snapshot_id, last_included_index, last_included_term, offset, data_b64, done, total_bytes, sha256 | accepted boolean, next_offset; installed_index only after final durable publication |

Entry: `{index,term,kind,command}`; kind is `noop`, `put`, `delete`, `cas` or `read`. noop has no command; read has only key; other commands have API fields except timeout and request_id. Indices are contiguous from prev+1; terms never decrease and no entry term exceeds the RPC term. A malformed batch has no partial effects.

Design example of empty AppendEntries (complete envelope):

```json
{"protocol_version":1,"cluster_id":"11111111111111111111111111111111","config_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"n1","to":"n2","rpc_id":"n1-42","term":"3","leader_id":"n1","prev_log_index":"8","prev_log_term":"3","entries":[],"leader_commit":"8"}
```

Design response:

```json
{"protocol_version":1,"cluster_id":"11111111111111111111111111111111","config_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"n2","to":"n1","rpc_id":"n1-42","term":"3","success":true,"match_index":"8"}
```

## Limits and flow

Maximum peer body 384 KiB; AppendEntries has at most 16 entries and 256 KiB of total serialized commands, also respecting the body cap. Maximum single entry 48 KiB (CAS with two Base64 values). Snapshots use raw 64 KiB chunks, maximum image 8 MiB, one incoming transfer per node, exact offsets or verified identical retransmissions. Different bytes at the same snapshot_id and offset invalidate staging. Obsolete snapshots never regress commit/apply; term and leadership are revalidated at every chunk and before installation. Staging idle timeout is 5 s; cleanup never touches the active snapshot.

Heartbeat every 100 ms; election timeout drawn uniformly from [600,1000] ms on each reset. Peer deadline 300 ms, dial 100 ms; header read 500 ms, peer request 1 s. These are initial parameters to measure: broadcast including persistence must be much shorter than election timeout. Timers are monotonic; laptop suspension or a slow SSD can disrupt liveness without authorizing safety shortcuts.

One active RPC per peer; after a timeout, retry on a tick using current state. Connection errors use 50..400 ms backoff, never more than one attempt per tick/peer. Transport retries never hide duplication from the core. Responses are tied to rpc_id, term and leader generation: old responses never reduce matchIndex or revive old leadership. A valid higher term forces step-down even in a late response.

Queues and backpressure follow [architecture](architecture.md). Reuse connections, limit the pool to two per peer, always close bodies; no unbounded retry goroutines. Non-200 HTTP is a transport/protocol error, not a negative vote. Disk errors: no positive success, node enters fatal-storage and stops voting/replicating; other nodes observe timeouts or 503.

## Shutdown and interoperability

Close the client listener, then workers and the peer listener within the shutdown deadline; flushing application logs never authorizes Raft acknowledgment. Membership and format are fixed; differing versions fail explicitly. Rolling upgrades and protocol negotiation are not planned. Unknown fields are rejected: evolution requires a version increment and migration plan.

## Admission details and obsolete snapshots

Accepted-connection caps: 32 on the client listener and 16 on the peer listener per node, including idle connections; reject new sockets at capacity and retain deadlines for admitted sockets. Outbound pools remain capped at two connections per peer; connection caps do not replace queue/entry limits.

If lastApplied already covers a snapshot, never rewrite state. Respond `accepted:false`, `reason:"obsolete"`, `next_offset:"0"` and include `resume_index=max(S,snapshotIndex)` with its local `resume_term`. This boundary is already applied and representable through the local log or snapshot boundary; do not always use snapshotIndex alone, which may unnecessarily precede the leader log. The leader abandons the transfer and realigns AppendEntries at resume_index+1 only if index/term matches its own log or snapshot; otherwise stop the attempt as an integrity incompatibility to diagnose. Advance matchIndex only after a subsequent correlated successful AppendEntries. Never treat a received index as proof of newly replicated entries. Offsets, total_bytes and next_offset are decimal strings; data_b64 uses canonical Base64. Snapshot ID is the whole-file SHA-256 digest and stays identical across retries.

## JSON validation beyond library defaults

The standard library remains sufficient, but ordinary struct decoding alone does not enforce the strict contract. Before semantic decoding validate UTF-8 on the raw body; scan tokens with an object stack to reject duplicate keys (including nested expected/commands), field names that do not match exactly and depth >16. Keys duplicated after JSON unescaping still count as duplicates. Require one JSON object followed only by whitespace through EOF; no second document.

Preserve numbers without float conversion, validate lexical form and ranges for sequence/timeout and decimal uint64 strings; reject exponents, fractions, signs and leading zeros where the contract requires canonical integers. Reject unpaired Unicode surrogates in text before silent replacement, and non-canonical Base64 by decode/encode comparison. Rebuild the hashing/WAL canonical format from typed fields; JSON input may use equivalent legal escapes. T-003 and T-009 include duplicate keys, case differences in field names, invalid UTF-8, unpaired surrogates and concatenated JSON. See [Go encoding/json](https://pkg.go.dev/encoding/json).

## Canonical entry projection

The [storage format](canonical-formats.md) fixes exact nested kind/command fields and canonical entry size, separate from HTTP envelopes. noop omits command; read contains key only. Rebuild validated entries canonically for persistence; protocol/configuration fixture coverage is implemented and checked in M04.

## M04 implementation boundary and fixtures

The previously proposed transport/envelope layer is now implemented: [M04 report](milestone-m04.md), [static configuration](configuration.md), [exact RPC fixtures](../fixtures/peer-v1.json). All three schemas are checked, but no voting, replication or snapshot staging/installation is implemented. One attempt, caller-owned same-peer admission, 100 ms dial/300 ms total outgoing deadline, 500 ms headers, 1 s incoming handler/read/write/idle, two pooled sockets per peer and 16 accepted sockets. nil processor gives 503; callbacks must honor cancellation. rpc_id is 1–128 ASCII letters/digits or -_.:, unique by caller/process responsibility. Configured Host/HTTP1.1/POST/JSON are required; invalid schema 400, identity 409, size 413, media 415, route 404, method 405. Strict parsing and all fixture byte checks pass. Retries/backoff, term/leader-generation effects, snapshot offsets/retransmission state and durable positive decisions remain future core/integration work.

## M05 election processor

StartElection now attaches Service.Handle: RequestVote implements durable voting, empty AppendEntries handles election heartbeat/prev validation. New-term noop is local only and is not transmitted as replication. Recognized nonobsolete heartbeat resets election time even if prev mismatches; stale heartbeat/denied vote does not. Valid higher terms, including old correlated replies or unsupported append/snapshot requests, are persisted before dependent output. Nonempty append/snapshot effects return 503/unavailable; no commit advancement or progress accounting. Core retries occur on later ticks with 50..400 ms peer error backoff, never parallel with the same peer. Existing envelope/body/socket/deadline contracts are unchanged. [Tests and limits](milestone-m05.md).

## M06 actual AppendEntries semantics

[M06](milestone-m06.md) handles nonempty AppendEntries, identical-entry retries, bounded conflict index/term hints and prefix match acknowledgments after Sync. Outgoing batches cap canonical entry bytes at 256 KiB as well as configured 1..16 entries. Legal incoming batches exceeding the WAL entry-byte cap split into bounded sequential durable transactions, one final reply; complete wire validation precedes the first effect. Sent/Failed register actual transport attempts; stale/duplicate replies cannot change current progress, while a valid higher term still demotes. leaderCommit is preserved in the schema but operational commit advancement waits for M07. Snapshot installation remains unavailable.

## M07 leaderCommit behavior

StartCommitted enables [M07](milestone-m07.md): outgoing AppendEntries carries the synchronized checkpoint; a follower advances only through the verified request boundary after suffix/checkpoint Sync. Rejected/stale traffic does not propagate commit. Positive replication reply proves durable prefix/checkpoint, not application or client success. Future read entries and InstallSnapshot effects remain unavailable; original schema/body/socket/deadline limits unchanged. All cluster nodes must select compatible capability at startup, no rolling feature negotiation.
