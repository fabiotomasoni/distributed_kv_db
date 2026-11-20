// Package wal owns one local generation's synchronous WAL writer and log-level
// recovery. It does not establish Raft commit, apply commands, deduplicate client
// sessions or install/compact snapshots. Callers supply legally established
// commit checkpoints. Ordinary Open never bootstraps or falls back generations.
// Only a structurally incomplete final frame after a fully valid initial prefix
// is truncated and synchronized, within the declared process-crash model.
package wal
