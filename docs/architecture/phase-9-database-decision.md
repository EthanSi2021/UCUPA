# Phase 9 — Embedded Database Decision

**Status: Accepted and implemented for Phase 9 only**

Phase 9 uses [`go.etcd.io/bbolt` v1.5.0](https://pkg.go.dev/go.etcd.io/bbolt@v1.5.0)
as the embedded transactional database behind the independent Facet-owned local
Store.

This is a new owner-local decision, not an automatic extension of the accepted
Phase 8 database decision. Selecting the same
database keeps one dependency and one understood durability substrate while the
Core and Facet Stores remain separate authorities with separate files, handles,
locks, markers, schemas, transactions, and lifecycles.

This decision was implemented only after the separate explicit Phase 9 start.

## 1. Why bbolt fits Facet

The accepted Phase 9 contract requires one
local single-writer Store whose operations are exact point reads and atomic
create, replace, or delete of opaque values no larger than 1 MiB. bbolt matches
that shape directly:

- it is an embedded pure-Go key/value database with no server process;
- it provides fully serializable ACID transactions with one writer;
- successful commits use the database's synchronized two-phase page/meta-page
  write path, while incomplete commits are ignored after interruption;
- it supports Windows and provides a file lock with a finite timeout option;
- its bucket API preserves exact keys and opaque values without adding SQL,
  an object model, a value log, or background compaction machinery;
- v1.5.0 is already pinned by Phase 8, so Facet adds no module or version change.

The authoritative upstream references are the
[v1.5.0 release](https://github.com/etcd-io/bbolt/releases/tag/v1.5.0),
[package documentation](https://pkg.go.dev/go.etcd.io/bbolt@v1.5.0),
[v1.5.0 source overview](https://github.com/etcd-io/bbolt/blob/v1.5.0/README.md),
[key/value limits](https://github.com/etcd-io/bbolt/blob/v1.5.0/bucket.go), and
[module file](https://github.com/etcd-io/bbolt/blob/v1.5.0/go.mod).

Re-evaluating the alternatives does not expose a missing Phase 9 capability.
Pebble omits transactions and adds LSM/compaction machinery for a throughput
problem this slice does not have; Badger adds an LSM value log, MVCC, garbage
collection, caches, compaction, and a wider dependency surface; SQLite adds SQL,
driver and journal configuration, and either a native or transpiled runtime.
Those mechanisms solve workloads outside this exact-key local slice. Choosing a
second database would add complexity without closing another accepted result.

This is not a universal database choice for every UCUPA module.

## 2. Independent owner boundary

The Facet Store must be a different bbolt database file from every Core Store.
It owns its own `DB` handle, file lock, current-format marker, private record
encoding, transactions, and close lifecycle.

The implementation must not:

- place Facet buckets inside a Core file or Core buckets inside a Facet file;
- share one transaction, handle, marker, loader, schema, or in-memory Store
  object between Core and Facet;
- import or call Core's private Store implementation;
- introduce a shared Store, repository, unit-of-work, database adapter, or
  combined durable Cluster abstraction.

A recognized Core Store is a foreign Store to Facet, and a recognized Facet
Store is foreign to Core. Each owner rejects the other's file without changing
it. Reusing the same database product never means sharing authority.

Facet's exact key is the pair `FacetKey(ElementID, FacetType)`. Its private
encoding must be unambiguous for every representable pair and must distinguish a
present empty payload from a missing record. The decision does not freeze a
bucket layout, delimiter, record header, or public storage format. No Facet
tombstone or permanent key claim is stored.

## 3. Required bbolt safety profile

The Phase 9 Facet Store must:

- pin and use the already selected `go.etcd.io/bbolt v1.5.0`;
- use one `DB.Update`, never `DB.Batch`, for each accepted Facet mutation;
  callback errors provide the required whole-transaction rollback without
  exposing bbolt through Facet's public boundary;
- retain safe synchronization defaults: `NoSync=false`, `NoGrowSync=false`,
  `NoFreelistSync=false`, and `InitialMmapSize=0`;
- impose no backend total-size cap in this slice and use a finite file-lock
  timeout rather than wait forever;
- create a new Store with exclusive-create semantics through `Options.OpenFile`;
- open an existing Store in two stages: first read-only with no-create semantics
  and validate the Facet marker without writing, then close and reopen
  read-write with no-create semantics and validate the marker again before
  serving;
- reject missing, empty, Core-owned, foreign, or unrecognized persistence during
  the read-only stage before a writable open can initialize or convert it;
- read or copy key/value bytes within their transaction lifetime;
- perform no Core existence call, orphan scan, Reconcile, or visibility decision
  while opening the Facet Store;
- close only its own bbolt handle when the Facet instance retires.

The private format marker and Facet records remain inside that one Facet-owned
file. Open, transaction, representability, and close failures map to Facet's
stable Store-failure boundary; bbolt errors and APIs never enter UCUPA's public
surface. No bbolt command, backup, compaction, or statistics helper is added.

## 4. Dependency boundary

This choice adds no Go module dependency and changes no selected version. The
root module already contains:

```text
go.etcd.io/bbolt v1.5.0
golang.org/x/sys v0.45.0 // indirect
```

The Windows production package closure remains exactly:

```text
go.etcd.io/bbolt
go.etcd.io/bbolt/errors
go.etcd.io/bbolt/internal/common
go.etcd.io/bbolt/internal/freelist
golang.org/x/sys/windows
```

The complete upstream module graph may still list bbolt's own CLI, test, and
tool dependencies. They are not UCUPA production imports. Phase 9 must preserve
the Phase 8 dependency checks, and `go mod tidy -diff` must remain empty.

## 5. Capacity and topology limits

bbolt v1.5.0 permits values far larger than the accepted 1 MiB Facet payload,
so the Facet-owned input boundary remains authoritative and is enforced before
authorization, Core lookup, or Store access.

bbolt limits a raw key to 32,768 bytes. The private collision-free encoding of
`ElementID + FacetType` consumes part of that limit. A key outside the backend's
representable range fails atomically as the accepted Facet Store-failure outcome
before authoritative mutation. This is not a production key-length profile or a
compatibility promise; key profiles remain a later capacity decision.

bbolt serializes write transactions. That matches the accepted one-writer local
Facet Store. Later scale comes from independent Facet Stores and Partitions, not
from concurrent writers to one authoritative file.

File shrinking, compaction, total-size limits, backup, corruption repair,
multi-process writes, schema migration, and mixed-version opening remain outside
Phase 9.

## 6. Database-specific decision gate

The accepted Phase 9 contract remains the sole semantic implementation
authority. This database decision adds only three proofs:

1. Facet uses bbolt v1.5.0 through the Section 3 safety profile, with a file and
   private owner format independent from Core;
2. the selected module versions and external production package closure remain
   unchanged;
3. public Facet behavior proves the database-specific representation boundary:
   two distinct production-valid FacetKeys retain different payloads, and a
   present empty payload remains distinct from a missing record, after
   process-exit/reopen; exactly 1 MiB remains representable; and Core and Facet
   each reject the other's Store without mutation before the original owner
   reopens it successfully. Phase 11's fixed-width UUIDv4 Element-ID profile
   supersedes the earlier variable-length raw-concatenation fixture without
   changing Facet's collision-free private key requirement.

Tests do not inspect the private schema or add a corruption/crash-prefix matrix.

The implementation satisfies this decision without changing the selected
module versions or adding a dependency. It remains local to the Facet-owned
Store and creates no general database mandate for later modules.
