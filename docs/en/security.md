# Local security model

[Index](README.md) · [Italiano](../it/security.md)

**Status: pre-implementation design, revision 0.2 — 18 November 2026. No functionality or measurement has been verified.**

## Trust boundary

Single-user prototype in a trusted local environment, not production-ready. Bind only explicit loopback IP addresses; reject `0.0.0.0`, `::` and hostnames resolving outside loopback. No port forwarding, public tunnels or external discovery. Verify static configuration, owner and directory before opening listeners.

Local processes running as the same user can impersonate clients or peers: cluster_id and config_hash prevent configuration mistakes but **do not authenticate** an adversary. Other local users and browsers remain risks: no permissive CORS, accept application/json only on POST endpoints and validate Host against configured addresses to limit accidental requests and DNS rebinding. These controls do not replace authentication.

## Mandatory controls

Validate UTF-8, strict JSON, duplicate/unknown fields, Base64, keys, numbers and sizes before large allocation or proposal. API cap 48 KiB, peer cap 384 KiB, raw snapshot chunk 64 KiB; limit connections, headers (8 KiB), queues and deadlines. Reject Content-Encoding other than identity; never decompress untrusted data. No user string becomes a file path or shell command.

Directories 0700/files 0600, no shared directories or symlinks for active data. The future lock/filesystem adapter verifies ownership and regular files; do not follow data configuration symlinks without an explicit choice. No passwords or values in logs; synthetic datasets only in tests and published evidence. Never upload WAL/snapshot directories to GitHub.

Runtime limits new proposals when queues/dataset fill, without evicting data or sessions. Local storage may stop on a full disk: do not promise denial-of-service resilience. Fault/profiling endpoints are disabled except during explicit loopback test sessions. Errors return stable codes without stack traces or personal paths.

## Exclusions and review

Advanced authentication, multiuser authorization, mandatory TLS/mTLS and multi-tenant isolation are outside the MVP. Any use outside the trusted computer requires a new threat model and ADRs, peer/client authentication, TLS, key management and audit: the current design does not authorize public exposure.

T-030: public bind rejected; oversized/malformed payload leaves state unchanged; actual permissions are private; no sensitive log data; fault endpoint absent in the normal profile. Maintain and review dependencies under the [guide](development-guide.md), without mandatory cloud scanning or paid services.
