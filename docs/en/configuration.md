# Static configuration v1

[Index](README.md) · [Italiano](../it/configuration.md)

**20 November 2026 — implemented in M04.**

## Input and immutable ownership

[config.Parse](../../internal/config/config.go) accepts one UTF-8 JSON object, at most 16 KiB, with exactly the fields in [configuration-v1.json](../fixtures/configuration-v1.json). Required integer numbers: config_version=1, max_logical_bytes=4194304, max_keys=10000, max_sessions=1024, heartbeat_ms, election_min_ms, election_max_ms. No exponents, fractions, signs or leading zeros. Required strings: cluster_id (32 lowercase hex), node_id, client_addr, peer_addr, data_dir. members is an array of exactly three objects with id/client/peer; IDs are exactly n1,n2,n3, each once. Unknown/duplicate names, invalid Unicode, multiple documents and excessive nesting fail. No defaults or environment expansion.

Each of the six endpoints is distinct, an explicit canonical loopback IP and nonzero decimal port. IPv4-mapped aliases are compared after unmapping for uniqueness. DNS, public/wildcard IPs, scoped IPv6, implicit ports and URLs fail. The local bind fields must equal the matching member's endpoints. data_dir is nonempty and cleaned, without parent traversal; actual private-path, file and exclusive-lock checks belong to storage. Timing requires 0 < heartbeat_ms < election_min_ms <= election_max_ms <= 2147483647. Timings are configuration only; M04 starts no election timer.

Parse returns an immutable Config value; private fields cannot be changed, Members returns an array copy, SharedBytes returns fresh bytes. ErrConfig means no usable configuration. Parsing performs no I/O, DNS lookup, listener creation or state mutation. Concurrent access to a parsed value is safe.

## Exact config_hash

Hash only cluster_id, members sorted lexicographically by id, and the three replicated quota fields. Recursively sort ASCII JSON field names, use decimal integer numbers, no whitespace or newline. All values in this preimage are ASCII. SHA-256 of these UTF-8 bytes, encoded as 64 lowercase hex digits, is config_hash. Local node_id, client_addr, peer_addr, data_dir, timing and config_version are excluded; the shared member endpoints remain included. Version compatibility is checked separately. Quotas are fixed in v1; changing them is rejected rather than silently producing a supported variant.

The fixture records the complete exact preimage and expected hash:

```text
7331643cc3f13679fc939ad80f62ca4eec3f52c680c8538a2ea0e95ff834c400
```

Member permutation or local configuration changes preserve this hash. A shared endpoint or cluster change changes it. There is no user-supplied hash override. Configuration and [peer fixtures](../fixtures/peer-v1.json) are checked by TestConfigurationCanonicalFixtureAndRejections and TestProtocolFixturesAndInvalidMessages. Existing storage fixtures are unchanged.

## Startup and shutdown

[node.Start](../../internal/node/node.go) consumes the parsed value, an explicit bootstrap boolean and an optional transport Processor. Bootstrap requires a pre-existing private empty directory; restart uses wal.Open. Both validate/acquire storage before binding the peer endpoint. Persistent cluster_id/node_id/config_hash must match exactly. Duplicate ownership, divergent metadata or storage failure prevents listener creation. Bind failure releases ownership and preserves initialized data. No automatic bootstrap or directory reset.

After an unexpected listener failure, peer work is canceled/drained and storage closed; Shutdown returns the serve error. Shutdown(ctx) closes admission, cancels network work, waits for accepted processors/outbound calls and server termination, then closes WAL/releases LOCK. A canceled waiter does not release the lock early; waiting can be repeated. Processor must honor cancellation and finish; arbitrary callbacks cannot be forcibly stopped by Go. Recovered returns a detached WAL state and never applies commands. The client endpoint is reserved/validated but not opened before M08. There is no node CLI yet. See [M04 evidence](milestone-m04.md).

## M09 executable node lifecycle

[cmd/quorumkv](../../cmd/quorumkv/main.go) now implements `quorumkv node -config FILE [-bootstrap]`; direct flags without `node` are also accepted. Configuration files are bounded to 16 KiB and parsed by the existing strict schema. [n1](../../examples/n1.json), [n2](../../examples/n2.json), [n3](../../examples/n3.json) share membership/hash and use distinct data directories. Relative paths resolve from the process working directory. Pre-create private empty directories for explicit bootstrap; ordinary restart omits bootstrap and never initializes or resets storage.

StartAPI retains M03 identity/LOCK/CURRENT/WAL validation and M07 fresh committed-prefix replay before either configured listener is served. SIGINT/SIGTERM stops admission and cancels HTTP waits; the existing owner finishes active persistence and drains committed apply before releasing both ports and LOCK. Node.Done closes only after actual resource termination, including automatic runtime failure. The process waits at most 5 s for shutdown, then exits 1 with shutdown_timeout_recovery_required; intact files are retained for explicit recovery. An unresponsive filesystem syscall during startup cannot be forcibly canceled by a Go context.

Exit 0 means normal stop/help, 2 invalid arguments/configuration, 1 startup/runtime/shutdown failure. Fixed diagnostics do not print raw config, user values or filesystem error contents. node_started means storage verified and listeners reserved, not elected leader or quorum-ready. Fatal runtime termination is observed through Node.Done and produces a failure exit. No quorum bootstrap mode, forced reset, membership change, CLI session storage or new HTTP endpoint. [M09 evidence](milestone-m09.md).

## M11 explicit application profile v2

[ADR-010](adr/ADR-010-session-profile.md) fixes a necessary compatibility boundary. Configuration v1 retains M10 behavior and exact original hash bytes. Configuration v2 requires the additional canonical integer application_version=2; that field is included in the shared hash, while local paths/binds remain excluded. Unknown/missing/other versions fail. [Golden configuration/hash/identity](../fixtures/configuration-v2-sessions.json) and [n1](../../examples/m11/n1.json)/[n2](../../examples/m11/n2.json)/[n3](../../examples/m11/n3.json) examples share the v2 contract.

Use a new cluster and private empty directories with explicit bootstrap for M11. V2 uses checksummed IDENTITY format_version=2 and the existing WAL/CURRENT/empty-snapshot encodings. V1 and v2 openings reject the other identity before WAL repair/replay/listeners; no conversion, reset or fallback. The shared application-version hash also isolates peer RPC from another profile. All three members must use the same profile. A v2 config is accepted only by the full StartAPI/runtime, not legacy partial election/replication helpers. Existing v1 directories remain usable with v1 configuration/M10 semantics; no in-place migration is implemented. Preserve them rather than copying active files.

Profile-v2 mutations require one X-QuorumKV-Application-Version: 2 request header and acknowledge version 2 in the response. The M11 client derives/validates this from its configuration; a missing/mismatched acknowledgment is unknown. Current legacy mode rejects the new mutation header, and v2 rejects an absent/other header before admission. Historical binaries may ignore new request headers, so their completions are not trusted by the M11 client. GET remains unchanged. [M11 evidence](milestone-m11.md).
