# QuorumKV overview

[Index](README.md) · [Italiano](../it/overview.md)

**Status: pre-implementation design, revision 0.1 — 18 November 2026. No functionality or measurement has been verified.**

## Purpose and motivation

QuorumKV will be a distributed key-value database written from scratch in Go: a map from keys to byte sequences, replicated by three independent processes. It is an educational and portfolio project for one developer, intended to make consensus, concurrency, durability and recovery observable. This repository contains design only: servers, a CLI, executable tests and benchmarks do not yet exist.

A key-value database retrieves an entire value by key; it does not interpret internal properties, tables or joins. Distributing it means keeping copies on nodes communicating through messages, without assuming shared memory, identical state or an always-available network. The central problem is agreeing on an update order despite crashes and delays.

## Three replicas, one logical machine

One Raft group will have three static members: `n1`, `n2`, `n3`. Its majority is `floor(3/2)+1 = 2`. Any two majorities intersect: together with persistent voting, the election restriction and the commit rule, this supports safety. Intersection alone does not prove implementation correctness.

The leader proposes commands in an ordered log; each replica applies only the committed prefix, in order, to a deterministic state machine. The intended logical result is one map. Raft separates elections, replication and safety into inspectable subproblems: this suits the learning objective, but it will be implemented directly without consensus libraries.

A majority can progress with one stopped process if the network, storage and scheduling permit progress. Two unavailable nodes prevent new linearizable operations. All three processes share a computer, power supply and SSD: they provide neither physical independence nor service availability against failure of the Mac.

## Planned features and uses

Planned features are `PUT`, `GET`, `DELETE`, `CAS`, WAL persistence, restart, snapshots, diagnostics and local fault injection. Examples include synthetic configuration, small inventories, resource metadata and counters updated through CAS. Do not use it for credentials, payments, irreplaceable data or production workloads.

| System | Difference from QuorumKV's objective |
|---|---|
| PostgreSQL | Relational model, SQL and transactions: outside the scope of this single-key map. |
| Redis | Ecosystem of data structures and operational features: this project studies four operations and consensus only. |
| etcd | Mature reference for a Raft-backed KV; neither a dependency nor a promise of compatibility or equivalence. |

The portfolio's value will come from evidence quality: election traces, checked concurrent histories, crash cutpoints, reasoned decisions and explicit limits. Feature count cannot replace this evidence.

## Reading path

Start with [requirements](requirements.md), [scope](scope.md) and [architecture](architecture.md). For a correctness review, read [Raft](raft-consensus.md), [consistency](consistency-model.md), [storage](storage-and-recovery.md) and [testing](testing-strategy.md) together. The [roadmap](implementation-roadmap.md) describes the future transition from design to evidence.
