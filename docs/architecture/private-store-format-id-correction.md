# Owner-Private Store FormatID Correction

**Status: Complete — owner-private recognition rule remains current; the
literal markers below are historical implementation records**

**Successor notice:** The completed
Core Anchor structural-mutation contract
supersedes only the Core marker and Core operation-format gates recorded here;
the owner-private exact-identity rule remains current. The P4 and subsequent P5 marker changes
are explicitly recorded in Section 2; P7.1 adds its accepted implementation record
there as well, without adding another reader. The
completed Realm local-durability contract is the
Realm marker's immutable-bootstrap implementation predecessor. The completed
[Realm-owned PermissionBinding-administration contract](realm-owned-permission-binding-administration.md)
defines its binding-administration successor, and the completed
[online caller-authentication contract](realm-owned-online-caller-authentication.md)
defines Realm's current marker and Store-recognition authority. This body's
statements about the then-current Core and Realm markers, absent Realm Store,
and no later active slice remain accurate records of their as-built scope; they
are not current repository status and define no compatibility input.

This completed contract records one bounded replacement of ordinal-looking
`/n` suffixes in the current private Store identity of each authoritative
owner. It changes only recognition metadata. It does not change any UCUPA
business fact, public API, Store record, transaction, ownership boundary, or
failure domain. The corrected implementation has no concept of a legacy Store:
each owner knows one current FormatID and every other value is simply not that
owner's Store.

The completed result is:

- Core writes and recognizes only the exact Core FormatID in Section 2;
- Facet writes and recognizes only the unrelated exact Facet FormatID in
  Section 2;
- Realm source is unchanged and has no Store implementation. The accepted but
  not started Realm local-durability contract uses
  the Realm FormatID frozen by this correction.

The implementation changes only Core and Facet private recognition metadata
and their test-only gates. It adds no compatibility path, Realm source,
Realm Store, public API, Store record, transaction, ownership boundary, or
failure domain. Completion starts neither the accepted Realm local-durability
contract nor another Production-baseline slice.

## 1. First-principles rule

Persistent bytes need one exact identity so an owner can reject missing,
foreign, stale, or semantically incompatible Stores before writable access.
They do not need an ordinal, ordering relation, compatibility ladder, or
negotiation protocol.

The accepted rule is:

```text
one Store owner + one accepted private format
  -> one immutable, owner-qualified, opaque FormatID
  -> exact byte equality only
```

`FormatID` is a term in this contract, not a new public Go type. Each owner
keeps its identifier as a private constant and owns its comparison. There is no
shared registry, parser, codec, generator, adapter, or Store abstraction.

For one compiled owner profile:

- exactly one FormatID is written and recognized;
- the complete value is compared by exact bytes;
- no prefix, suffix, punctuation, hexadecimal text, or embedded field is
  parsed at runtime;
- no FormatID is ordered, incremented, downgraded, negotiated, or treated as
  compatible with another;
- a missing, non-current, or foreign FormatID returns the owner's existing
  `ErrStore` with no returned owner object;
- recognition occurs during the existing read-only identification pass and
  does not modify the rejected file;
- a matching FormatID never bypasses the selected owner's existing bbolt
  open/read errors or bucket, record, graph, revision, operation, and semantic
  validation.

This correction adds no `Tx.Check` or other database consistency mechanism to
Core or Facet. The separately accepted future Realm durability consistency gate
remains unchanged and is implemented only if Realm durability is independently
started later.

FormatID is private control metadata. It is not UCUPA product versioning, API
versioning, a wire version, a migration level, a security proof, or resolution
of the formal-version policy in the
production capability map.

## 2. Exact accepted replacement FormatIDs

P7.2 implementation-in-progress note (2026-09-14): Core adds its bounded transfer
intent/cancellation/source-fence journal, canonical source evidence, inactive
departure claims and exact fence-to-commit binding to the same original Store
transaction and history validation. Its sole working implementation identifier is
`ucupa.core.local@74839e6154d0498e9f6147ce9a2edf74`; there is no P7.1 reader or
automatic conversion. Facet and Realm retain their P7.1 identifiers below.
This working-tree format change is not P7.2 completion or delivery acceptance.

P7.1 accepted implementation note (2026-09-14): the Core/Facet writer journals
and Realm authority-scope discriminator use these unrelated exact identifiers:

```text
Core   = ucupa.core.local@6dce3f7b965c4e8781c999d090246e2c
Facet  = ucupa.facet.local@8e3ba80764154ad9b2d6ee59fee49648
Realm  = ucupa.realm.local@a33aa9c69815480491bca89ce0c6d653
```

At the P7.1 delivery each compiled owner recognized only its one identifier above. There is no
predecessor reader, conversion, negotiation or sequential format version.
P7.1 has passed full verification and delivery acceptance; the user authorized
its separate commit as recorded in the [development plan](development-plan.md). The notes below retain their
historical implementation boundaries, not additional recognized input formats.

P4 implementation note: Core now persists its attachment control journal in
the same owner Store and commit sequence; its one implemented marker is
`ucupa.core.local@c5dd7cf8348e4b539df761e852b452dc`. Facet additionally persists its stable owner
identity, exact acceptance/contact/content history and type witness pins; its one implemented marker is
`ucupa.facet.local@e92345fc6b28421383e2b276faa8c703`. Realm's owner-qualified
Facet namespace/key codec uses `ucupa.realm.local@247057c2b3804bd989a1ef1c9e181f27`.
The values below record the earlier marker-only correction, not additional recognized formats.
P5 directly replaces the working implementation markers with
`ucupa.core.local@1aeac7f65b1b4326be9bb065c201faf2` and
`ucupa.facet.local@54e8a69c08964ef1a6a2cd96f3de2b49` for persistent public revision keys,
exact public request recovery bindings and verified type artifact mirrors.
Realm retains its current marker with the exact closure-scope discriminator and 32-byte TypeRef codec.
No predecessor reader or automatic key/mirror repair is present; P5 delivery status is recorded in the development plan.
The accepted [P4 local contract](facet-local-truth-and-core-lifecycle.md) now owns these
implemented marker/codec increments; identity, credential and binding semantics remain with Realm.

The accepted replacement identifiers are the following exact ASCII byte
strings:

```text
Core   = ucupa.core.local@3644475c1288407eae9fe23423af5c26
Facet  = ucupa.facet.local@076e599dbe0c4a238cd2d273e293138f
Realm  = ucupa.realm.local@0e2e85e40b0d436b952183a759f7d6a6
```

The owner-qualified prefix is diagnostic. The 32-character suffix is a fixed
opaque token assigned once by this contract. It is not parsed as a UUID, hash,
timestamp, semantic label, or version. Runtime code compares the entire string
and has no knowledge of this presentation rule.

Opaque tokens are chosen instead of semantic format names because a semantic
name can remain linguistically plausible after bytes or meaning have changed
and can therefore invite unsafe reuse. Automatic content fingerprints are also
excluded: the repository has no canonical manifest covering layout,
validation, and fact interpretation, and introducing one would create a second
format contract without proving semantic completeness.

The unqualified strings `ucupa.core.local`, `ucupa.facet.local`, and
`ucupa.realm.local` are namespaces, not reusable FormatIDs. No owner may treat
one of them as a permanent marker across incompatible formats.

## 3. Future incompatible changes

An incompatible change includes any change to:

- canonical stored bytes or bucket/key/value structure;
- the set of accepted structures or validation invariants;
- the interpretation or authority of a persisted fact;
- revision, operation-binding, retained-outcome, or lineage semantics in a way
  that can make previously recognized bytes mean something different.

Such a change requires a separately reviewed, unrelated, never-reused
FormatID. The future decision does not increment the current token and cannot
infer compatibility from a shared prefix. It must again choose exactly one
current identifier and define fail-closed treatment of every other value.

A pure implementation optimization or a correction that only makes code obey
an already frozen byte and semantic contract does not require a new FormatID.
The deciding question is whether the same accepted bytes can acquire a
different validity or meaning, not whether source code changed.

Any future dual read, export/import, migration, backup conversion, or rolling
mixed-format operation requires its own accepted contract. This contract
creates none of those paths.

## 4. Bounded owner effects

### 4.1 Core

The implementation changes only the private marker value written at Core's
existing reserved marker key:

```text
ucupa.core.local/3
  -> ucupa.core.local@3644475c1288407eae9fe23423af5c26
```

The marker key, buckets, revision scope, bbolt sequence, Anchor/Entity records,
claims, operation bindings, retained outcomes, private codecs, transactions,
locks, constructor/open/close behavior, validation order, Revision/CAS,
`OperationID`, replay, permanent claim, and public errors remain unchanged.

Core writes and recognizes only the new exact FormatID. The displaced `/3`
literal does not remain in active Go source or tests, and no branch recognizes
or classifies it. Any marker other than the one current Core FormatID is the
same ordinary FormatID mismatch. There is no compatibility alias or fallback.

### 4.2 Facet

Facet changes only the private marker value written at its existing reserved
marker key:

```text
ucupa.facet.local/2
  -> ucupa.facet.local@076e599dbe0c4a238cd2d273e293138f
```

The marker key, records and operations buckets, revision scope, bbolt sequence,
FacetKey/record/binding/outcome codecs, private operation codes, transactions,
locks, constructor/open/close behavior, canonical-key validation,
Revision/CAS, `OperationID`, replay, reconciliation, Core existence boundary,
and public errors remain unchanged.

Facet writes and recognizes only the new exact FormatID. The displaced `/2`
literal does not remain in active Go source or tests, and no branch recognizes
or classifies it. Any marker other than the one current Facet FormatID is the
same ordinary FormatID mismatch. The existing noncanonical-key corruption gate
is rebuilt with the new current FormatID and remains byte-preserving.

### 4.3 Realm

Realm has no Store implementation in this correction. The accepted
Realm local-durability contract directly uses:

```text
ucupa.realm.local@0e2e85e40b0d436b952183a759f7d6a6
```

All other accepted Realm durability semantics remain frozen: the exact three
fact sets, `Seed` boundary, `CreateDurable`/`OpenDurable` result, transient
load-close lifecycle, pure bool `Authorize`, bbolt selection, buckets, tuple
codec, atomic snapshot, corruption gate, capacity boundary, exclusions, and
stop rule.

When Realm durability is separately started in the future, it must write and
recognize only that exact FormatID. It does not implement or test an ordinal
predecessor. This correction does not add `ErrStore`, a Realm Store, scope
projectors, source, tests, dependency, or lifecycle today.

### 4.4 Greenfield workspace boundary

Implementation and its gates used new Store paths. Development artifacts
written before the correction are outside the active system and are removed by
the operator before it runs; UCUPA receives no legacy-file input and exposes no
legacy-file outcome. Production code and tests do not identify, preserve,
delete, rewrite, migrate, or otherwise model those artifacts.

This boundary is valid because the project is Greenfield and has made no data
preservation or mixed-format promise. If such a promise is ever required, it is
a separate product decision rather than behavior retained by this correction.

## 5. Why all three owners are included

Changing only the not-yet-implemented Realm marker would leave ordinal
FormatIDs as the active Core/Facet pattern and make the repository's private
identity rule inconsistent. This contract deliberately corrects the rule once
before the Production baseline is complete.

The consistency is behavioral, not a shared implementation:

- each owner has a different exact identifier and private comparison;
- each owner continues to own its own file, buckets, codec, transaction, and
  error surface;
- owners mutually reject each other's Stores;
- no module imports another owner's private Store code;
- no common `FormatID` type, registry, helper, repository, or database wrapper
  is introduced.

## 6. Completion gate

### 6.1 Exact identity and rejection

Owner-package, independently specified golden gates prove:

- a newly created Core Store contains the exact accepted Core FormatID;
- a newly created Facet Store contains the exact accepted Facet FormatID;
- production constants match the independent expected bytes rather than an
  encoder and decoder merely agreeing with each other;
- each owner starts from a Store created by its current writer, replaces only
  the FormatID with one arbitrary same-prefix non-current value, and proves
  read-only open returns `ErrStore` with no owner object and no byte change;
- Core and Facet continue to reject each other's recognized Stores, after which
  each original owner can reopen its own Store unchanged;
- existing gates continue to reject missing paths, unrecognized bytes, foreign
  databases, and locked Stores without turning them into FormatID categories;
- a correct FormatID with an invalid bucket, record, graph, binding, revision,
  or operation history still fails the owner's existing strict validation.

Realm FormatID gates belong to the later Realm durability implementation, not
this correction. This correction freezes only their future expected bytes.

### 6.2 No hidden Store change

Review and owner-package logical fixtures prove that only the marker value
changes for Core and Facet. In particular:

- reserved marker and revision-scope keys remain unchanged;
- top-level bucket sets and bucket sequences remain unchanged;
- all record and operation keys/values remain byte-for-byte governed by the
  existing codecs;
- Core Store generation/replay validation remains unchanged;
- Facet operation codes remain Put=`1`, Delete=`2`, Reconcile=`3`;
- commit atomicity, fail-before-commit rollback, no-Close process-exit recovery,
  owner isolation, and mutual Store rejection continue to pass.

No test-only public API, production inspection endpoint, migration helper, or
post-commit seam is added.

### 6.3 Static and repository gates

The completed implementation gate includes:

```powershell
go test -count=1 ./...
go test -shuffle=on -count=2 ./ucupa-core ./ucupa-facet ./composition ./acceptance
go vet ./...
go mod tidy -diff
go run ./cmd/ucupa
git diff --check
```

Static checks must prove:

- active Go, including tests, contains no numbered private Store marker matching
  `ucupa.(core|facet|realm).local/[0-9]`;
- within active Go source, the exact new Core and Facet FormatIDs occur only in
  their owner-private Store production file and owner-package tests; contract,
  current-status, and historical documentation are outside this code gate;
- no predecessor-marker literal, alias, decoder, fixture, migration, rewrite,
  or fallback enters active code;
- `ucupa-realm` production remains unchanged;
- `go.mod` and `go.sum` remain unchanged;
- no shared Store or FormatID abstraction and no new external package enters.

Historical contract bodies are excluded from the numbered-marker zero-match
gate because they remain evidence of the implementation they originally
proved; they do not define active input classes or tests.

## 7. Explicit exclusions and stop rule

This contract does not include:

- any public Core, Facet, Realm, Composition, acceptance, command, or shared
  value API change;
- any authoritative fact, authorization, existence, mutation, revision, CAS,
  operation, replay, reconciliation, mount, route, or federation change;
- Realm durability implementation, authentication, credential/key/time state,
  administration/delegation, broader scope, process API, or online mutation;
- migration, compatibility read, dual marker, fallback, auto-delete, repair,
  backup/restore, export/import, rolling mixed-format operation, or data copy;
- a public FormatID, registry, parser, ordering, semantic name, generator,
  content fingerprint, manifest, negotiation, or formal-version policy;
- bucket, key, value, codec, operation-code, sequence, transaction, locking,
  synchronization, capacity, dependency, or database-profile changes;
- edits that rewrite completed Concept, Phase, or correction bodies to resemble
  the future implementation.

Work stops and returns to architecture review if implementation requires any
Store byte change beyond the exact marker value, a second recognized marker,
data preservation, Realm Store source, a shared abstraction, a new dependency,
or an answer to any question excluded by this correction.

## 8. Completion state and documentation boundary

Proposal review and acceptance were limited to this document, the architecture
index, the development plan, and the Realm durability FormatID pointer. This
Greenfield clarification synchronizes this contract with the accepted Realm
local-durability contract. It authorizes no source, test, Store, package README,
Realm durability implementation, command, dependency, generated artifact,
Concept, Phase, Platform, capability-map, or minimum-truth change.

This correction is complete. Core and Facet implement their accepted exact
FormatIDs and recognize no predecessor-specific input class. Realm remains the
unchanged in-memory permission kernel with no Store; its accepted local
durability contract still requires a separate explicit start.

Current status and Core/Facet package documents are synchronized. Historical
bodies remain unchanged except for narrow top-level successor pointers.
Completion activates neither Realm durability nor another Production-baseline
slice.
