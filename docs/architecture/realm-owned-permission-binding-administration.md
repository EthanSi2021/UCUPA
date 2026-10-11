# Realm-Owned PermissionBinding Administration

**Status: Complete Realm-owned PermissionBinding-administration contract —
current binding-mutation authority; slice closed**

**Successor notice:** The completed
[Realm-owned online caller-authentication contract](realm-owned-online-caller-authentication.md)
preserves this exact binding model and adds only caller establishment,
credential lifecycle, an independent revision/operation namespace, and the
current six-bucket Realm Store representation. This contract remains the
authority for binding administration; it is no longer the complete current
Realm surface by itself.

The accepted [P4 local Core/Facet increment](facet-local-truth-and-core-lifecycle.md#7-authority-qualified-permissions)
supersedes only the remaining Facet scope profile, codec and private marker:
namespace and key scopes now include the receiving Facet owner. This contract
continues to own binding administration; earlier scope examples are not alternate
accepted inputs. P4 adds no RealmAttachment or new authorization model.

This completed contract defines the smallest online mutation boundary needed to
change the exact operational `PermissionBinding` set owned by one Realm after
bootstrap. This Realm-owned behavior is orthogonal to and does not answer or
subdivide the predecessor
production-capability map's decision-time question 15,
which asks about identity creation, transfer or attenuation of management
authority, and administration of another Realm. This contract is the current
implemented runtime authority for this bounded behavior.

At this contract's completion boundary, the predecessor map still recorded
question 15 as **OPEN**, questions 1 and 8 as **ACCEPTED**, and questions 2
through 7 and 9 through 14 as **OPEN**. Those labels are historical provenance,
not current plan state. The
completed [Realm minimum-model correction](realm-minimum-model-correction.md) and
Realm local-durability contract remain completed
predecessors; this contract is their implemented successor for exact binding
semantics and mutation convergence.

Acceptance changed documentation only. The user's later explicit Start
separately authorized the bounded shared value, Realm API and Store, tests,
mechanical acceptance coverage, and completion-document changes now recorded.
Completion closes that slice and authorizes nothing later.

Terminology is deliberately disjoint in this contract:

- **same-Realm PrincipalRef** or **local subject** means only that
  `PrincipalRef.Realm` equals this Realm's ID; registration is a separate fact,
  and **registered local Principal** means that the referenced `PrincipalID` is
  also present in this Realm's registered local set;
- **Realm-owned** or **owner-local** means that an authoritative fact or outcome
  has one logical Realm owner; it does not mean one machine, local disk, or
  unreplicated persistence;
- **owner-scoped** means that a revision or operation namespace is valid only
  within one Store identity or one in-memory lifetime of that owner; and
- **durable-local profile** means only the completed standalone Store profile,
  not a permanent deployment topology.

There is no `LocalRealm`, `RemoteRealm`, or shared cross-module `Local` profile.
A Realm-owned relation may contain an exact binding for an already established
remote `PrincipalRef` without changing these definitions. Existing private
`.local@...` FormatIDs are exact owner-private Store-format recognition markers,
not Store-instance identities, Realm identities, locality claims, or business
facts.

## 1. Required observable result

The predecessor immutable Realm relation could authorize only scopes known when
its `Seed` was constructed. That was insufficient for ordinary runtime creation:
Core may create a caller-proposed Element UUID, but later reads, reanchors,
deletes, and Facet operations require exact scopes containing that UUID.

The implemented minimum behavior is therefore:

```text
an established registered local Principal
+ exact immutable bootstrap management authority in this Realm
+ one exact operational PermissionBinding
+ the expected current owner-scoped BindingSetRevision
+ one owner-scoped OperationID
-> retain one exact Put/Delete result
+ atomically publish the resulting exact binding set
```

Realm remains the only owner of this relation. Core and Facet neither edit nor
mirror it. Realm does not inspect Element or Facet existence, and a caller does
not submit a path, UUID ancestry chain, role, policy document, or delegation
lineage.

The business truth stays minimal:

```text
RealmID
+ registered local PrincipalID set
+ exact PermissionBinding set
```

The accepted owner-scoped `BindingSetRevision` and retained operation outcomes
are owner-local convergence metadata. They are not another permission fact,
resource identity, credential, role, or security-audit record.

## 2. Exact scope and bootstrap authority

### 2.1 One additional exact scope kind

The accepted shared value surface adds one scope constructor and its narrow
typed projection:

```go
func RealmScope(realm RealmID) OperationScope
func (scope OperationScope) RealmTarget() (RealmID, bool)
```

`RealmScope` contains one non-empty exact `RealmID`. It is distinct from
`ElementScope` and `FacetScope` and is not a wildcard, prefix, path, subtree,
organization, deployment, or descendant scope.

`RealmTarget` returns that exact Realm ID only for a valid Realm scope and
otherwise returns the zero Realm ID plus `false`. Constructor and projection
together are the slice's complete shared-scope increment; no generic scope kind
enum, visitor, parser, or codec is exposed.

This slice permits `RealmScope` only for Realm's own two administration
permissions:

```go
const (
    PermissionPutBinding values.PermissionID =
        "ucupa.realm.binding.put"
    PermissionDeleteBinding values.PermissionID =
        "ucupa.realm.binding.delete"
)
```

An administration requirement always uses the current exact Realm ID:

```text
PermissionPutBinding    + RealmScope(current RealmID)
PermissionDeleteBinding + RealmScope(current RealmID)
```

Realm interprets only this same-Realm administration combination. A Realm
permission with an Element or Facet scope, a non-Realm permission with a Realm
scope, or a Realm scope naming another Realm is invalid.

### 2.2 Explicit immutable bootstrap

Administration bindings are ordinary exact rows in the Seed relation, but
their permitted shape is narrower:

- the subject is a registered local Principal in the Seed Realm;
- the permission is exactly `PermissionPutBinding` or
  `PermissionDeleteBinding`; and
- the scope is exactly `RealmScope(Seed.ID)`.

They can be established only by `Seed`. Online `PutBinding` and
`DeleteBinding` reject every target containing a Realm scope or either Realm
administration permission. The registered local Principal set also remains
immutable.

This removes the recursive cases from the bounded model. An online manager
cannot:

- grant management authority to itself or another Principal;
- delete its own or another manager's bootstrap authority;
- make a remote Principal a Realm manager; or
- create a "last administrator", recovery, or self-revocation special case.

A Seed with no administration binding is valid and produces a deliberately
sealed Realm. There is no implicit owner, default administrator, emergency
bypass, or hidden rule that at least one manager must exist. Put and Delete
authority may be seeded independently and use exact default-deny semantics.

### 2.3 Online operational target

The target of an online Put or Delete is exactly the existing value:

```text
PermissionBinding {
  Subject    PrincipalRef
  Permission PermissionID
  Scope      ElementScope(ElementUUID) |
             FacetScope(FacetKey{ElementUUID, FacetType})
}
```

The complete target must be valid and must not contain an administration
permission or Realm scope. For a local subject, the exact Principal ID must
already be registered in this Realm. An operation never creates, deletes, or
implicitly registers a Principal.

A valid remote `PrincipalRef` may be the subject of an operational binding.
Realm does not contact its home Realm, check its existence, or establish its
credentials. The row changes only this Realm-owned authorization relation;
it neither modifies the remote Realm nor grants management, attenuation,
re-delegation, or transitive authority.

As in the current contract, a syntactically valid prospective Element UUID is
a valid target before Core creates or claims it. Permission does not prove that
the Element or Facet exists.

## 3. Accepted public owner surface

The accepted Realm-owned values are comparable and equality-only:

```go
// OperationID identifies one exact administration mutation within one Realm
// Store identity or one in-memory Realm lifetime.
type OperationID string

// BindingSetRevision is an opaque equality token for the complete binding-set
// state of one Realm Store identity or one in-memory Realm lifetime.
type BindingSetRevision struct {
    // private owner-controlled representation
}
```

The accepted operations are:

```go
func (realm *Realm) BindingSetRevision() BindingSetRevision

func (realm *Realm) PutBinding(
    actor values.PrincipalRef,
    binding PermissionBinding,
    expectedRevision BindingSetRevision,
    operationID OperationID,
) (BindingSetRevision, error)

func (realm *Realm) DeleteBinding(
    actor values.PrincipalRef,
    binding PermissionBinding,
    expectedRevision BindingSetRevision,
    operationID OperationID,
) (BindingSetRevision, error)

func (realm *Realm) Close() error
```

There is one owner-scoped `BindingSetRevision` for the complete binding set of
one logical Realm authority, not one Revision per binding and not one Revision
shared across Realm authorities. It denotes this Realm's complete current exact
binding set:

- every successfully constructed Realm starts with one valid nonzero
  `BindingSetRevision`;
- the token is equality-only and carries no public ordering or arithmetic;
- it is scoped to one Store identity or one in-memory Realm lifetime, so the
  same Realm ID in two independent authorities cannot share tokens;
- an actual set insertion or removal produces one later `BindingSetRevision`;
- a successful no-op retains and returns the current `BindingSetRevision`; and
- a mutation anywhere in the set conservatively conflicts with another stale
  expected `BindingSetRevision`.

This aggregate CAS matches the authoritative aggregate and avoids per-row
revisions, tombstones, a binding read API, and a second conflict language. The
expected conservative serialization is acceptable for the bounded,
single-owner administration rate. A demonstrated need for atomic multi-binding
changes or higher write concurrency requires a separate contract.

`BindingSetRevision()` exposes only the current opaque `BindingSetRevision`. It
does not enumerate, describe, or prove any binding. `Authorize` remains the
only accepted public point decision over the relation; this slice adds no Get,
List, query, page, history, or diff API. A nil or retired Realm returns the
zero, invalid `BindingSetRevision`; an in-memory Realm remains active because
its `Close` is a no-op.

## 4. Put and Delete semantics

### 4.1 Put as desired membership

For one new authorized operation whose expected `BindingSetRevision` matches:

- if the exact target binding is absent, `PutBinding` atomically adds it and
  returns a later `BindingSetRevision`;
- if the exact target binding is already present, `PutBinding` succeeds as a
  retained no-op and returns the unchanged current `BindingSetRevision`.

It never replaces a partial row, merges fields, registers a Principal, expands
a scope, or derives a narrower permission from a broader one. Equality is over
the complete `Subject + Permission + Scope` value.

### 4.2 Delete as desired absence

For one new authorized operation whose expected `BindingSetRevision` matches:

- if the exact target binding is present, `DeleteBinding` atomically removes it
  and returns a later `BindingSetRevision`;
- if the exact target binding is absent, `DeleteBinding` succeeds as a retained
  no-op and returns the unchanged current `BindingSetRevision`.

Delete affects exactly one Realm-owned relation row. It does not invalidate a
`PrincipalRef`, revoke a credential, rotate a key, cancel an already authorized
Core or Facet operation, remove another permission, or change another Realm.

No-op success makes Put and Delete express desired set membership without an
extra existence API. It does not weaken CAS: a stale expected
`BindingSetRevision` returns revision conflict even if the requested desired
state now happens to hold.

### 4.3 Same-Realm subject registration absence

A structurally valid same-Realm target whose `PrincipalID` is not registered
returns retained `ErrNotFound`. It does not become invalid input and never
creates the Principal. Remote subjects have no corresponding existence check
because this Realm does not own their identity set.

## 5. Validation, authorization, CAS, and replay

### 5.1 Stable error boundary

The accepted Realm administration surface uses only Realm-owned errors:

```text
ErrInvalid
ErrDenied
ErrNotFound
ErrRevisionConflict
ErrOperationConflict
ErrStore
```

Existing `ErrInvalidSeed` remains the construction error for an invalid Seed.
There is no shared operation-result abstraction and no success flag or replay
flag. Put and Delete return a valid `BindingSetRevision` only on success; every
error, including retained not-found or revision conflict and an operation
conflict, returns the zero `BindingSetRevision`.

### 5.2 Public ordering

Each call preserves this order:

```text
request-shape validation
-> active owner-session gate
-> exact current bootstrap-manager authorization
-> retained OperationID lookup
-> expected BindingSetRevision comparison
-> target same-Realm registration check
-> desired membership decision and atomic commit
```

Request-shape validation covers the nonempty OperationID, valid nonzero
expected `BindingSetRevision`, complete valid target binding, and the
prohibition on an online administration target. A retired durable Realm then
returns `ErrStore` before consulting manager authority or retained operations.
On an active Realm, the actor is required to be an established, registered
local Principal with the exact Put or Delete permission at
`RealmScope(realm.ID())`. An invalid, remote, unregistered same-Realm, or unbound
actor is denied.

Authorization precedes operation lookup, so an unauthorized caller cannot use
a known OperationID to reveal retained history. This is stable because online
operations cannot modify manager bindings or local Principal registration.

For a new OperationID, expected `BindingSetRevision` comparison precedes
same-Realm target registration and binding membership. A stale caller therefore
receives one stable revision conflict rather than learning which later relation
change occurred.

### 5.3 Exact operation identity

An OperationID is caller-proposed, nonempty, opaque, and scoped to one Realm
Store identity or one in-memory Realm lifetime. It binds:

```text
actor + operation kind + complete target PermissionBinding
      + expected BindingSetRevision
```

After current authorization:

| OperationID state | Result |
|---|---|
| Exact same binding | Return the retained historical outcome without current CAS or membership evaluation |
| Same ID, any changed bound field | Return `ErrOperationConflict` |
| New ID | Evaluate the current `BindingSetRevision` and relation, then retain one definitive outcome |

Success with a changed set retains the committed later `BindingSetRevision`.
Success with an unchanged set retains the current `BindingSetRevision`.
`ErrNotFound` and
`ErrRevisionConflict` retain only their exact negative outcome. Exact replay
returns that historical result after any later relation mutations and never
restores historical state.

Invalid input, denial, operation conflict, and a definitely pre-commit Store
failure create no new operation binding. Success, successful no-op, not-found,
and revision conflict are definitive owner-local results and are retained. A
Store failure whose commit status cannot be proved is indeterminate: it returns
no successful `BindingSetRevision`, retires the durable instance, and requires
independent reopen plus the same OperationID to discover whether a result
committed.

Two new operations using the same expected `BindingSetRevision` are serialized.
If the first changes the set, it advances this Realm's `BindingSetRevision` and
the second retains revision conflict. The second does not become a no-op merely
because the first created its desired state. If the first is a no-op, the
`BindingSetRevision` remains valid and a distinct second operation is evaluated
normally.

## 6. Linearization and authorization visibility

One Realm owner session linearizes:

```text
Authorize
BindingSetRevision
PutBinding
DeleteBinding
Close
```

A mutation's current manager check, operation lookup, CAS, relation decision,
durable commit when present, and publication of the immutable in-memory
snapshot form one owner-controlled critical section. Readers may observe the
complete state before or after a mutation, never a partially changed set.

For a successful Delete that changes the set:

1. the removed binding, later `BindingSetRevision`, exact operation binding,
   and retained outcome are committed atomically;
2. the complete later in-memory snapshot is published; and
3. only then does the public operation return success.

Every `Authorize` beginning after that return denies the removed exact
requirement unless another later Put has restored it. An owner operation that
completed its authorization check before Delete linearized may still finish;
this slice creates no cross-owner cancellation or transaction.

`Authorize` continues to perform exact in-memory lookup with no Store I/O. A
definitely pre-commit failure leaves the last committed snapshot current and
may leave the session active. Any Store or commit failure whose outcome cannot
be proved retires the complete durable Realm immediately: subsequent
authorization fails closed and administration returns Store failure until an
independent reopen reconstructs the committed state. This prevents a possibly
committed Delete from coexisting with a stale allowing snapshot.

`Close` retires a durable Realm instance and releases its Store lock; the caller
must discard it regardless of the Close result. A retired instance is not a
supported current snapshot: authorization fails closed and administration
returns Store failure. Close is a no-op for an in-memory or already retired
Realm.

## 7. One Greenfield Store contract

The completed mutable Realm profile uses one new
owner-private representation because its authoritative lifecycle and facts
changed. The implementation writes and recognizes exactly this opaque
FormatID:

```text
ucupa.realm.local@a6fc711b2b794fa1ac6c26c3afddba03
```

The prefix is diagnostic and the suffix opaque. Neither part is parsed as a
sequence, schema number, compatibility line, UUID, or hash. Exactly this byte
string identifies the one implemented Realm Store representation; every other
marker is unrecognized and fails read-only recognition with `ErrStore` and no
byte change.

The completed representation has no predecessor input class,
migration, compatibility reader, dual read, fallback, repair, rewrite,
conversion, numbered format line, or fixture whose purpose is to preserve
another representation. Development artifacts with another marker would be
outside that completed Greenfield contract.

The implementation retains the existing
Realm-owned bbolt dependency and Store ownership, but replaces the closed
immutable-snapshot lifecycle with the same single long-lived owner pattern
already used by the mutable Core and Facet profiles:

- `CreateDurable` exclusively creates, initializes, and returns one open
  read-write Realm owner;
- `OpenDurable` first performs a complete read-only, no-create validation,
  closes that handle, then reopens read-write with no-create semantics and
  revalidates the same Store identity and complete state before returning one
  owner;
- that owner exclusively holds the file lock until `Close` or process exit;
- a second simultaneous durable owner is unsupported and fails as Store
  inability; and
- `New`, `CreateDurable`, and `OpenDurable` expose the same relation,
  `BindingSetRevision`, Put/Delete, replay, and authorization semantics where
  durability is relevant.

The owner-private Store contains only what is necessary to reconstruct and
validate this Realm:

```text
RealmID
immutable registered local Principal set
current exact binding set
BindingSetRevision scope and current generation
exact administration operation bindings and retained outcomes
private recognition metadata
```

Realm scope receives one private scope tag in Realm's codec. The codec uses the
typed `RealmTarget` projection and rejects every invalid or noncanonical scope
shape. No public codec, schema registry, generic Store, shared repository, or
new dependency is added.

Every definitive operation uses one Realm Store transaction. A changed Put or
Delete commits the relation change, later `BindingSetRevision`, exact operation
binding, and retained outcome together. A no-op, not-found, or
revision-conflict result commits only the exact operation binding and outcome
without advancing the `BindingSetRevision`. In-memory publication follows
durable commit.

A definite pre-commit failure leaves no binding change, `BindingSetRevision`,
OperationID, or outcome. An indeterminate Store result retires the instance. If
the process exits or a response is lost after commit, `OpenDurable` plus exact
replay returns the committed historical result. Store open validates the
complete current set, `BindingSetRevision`, and structurally canonical operation
bindings/outcomes as one owner state. The retained operation records are the
complete convergence record; there is no second event or mutation-history
truth. Malformed records, foreign or structurally impossible tokens, and
inconsistent current facts fail closed as `ErrStore`. The Store does not claim
to re-prove from current state that every historical revision conflict was true
when originally decided.

## 8. Observable completion gate

The completed slice proves all of the following.

### 8.1 Exact bootstrap authority

- `RealmScope` and `RealmTarget` round-trip one exact non-empty Realm ID, while
  every other or invalid scope fails the typed projection;
- Seed accepts exact Put and Delete manager bindings only for registered local
  Principals and only at `RealmScope(Seed.ID)`;
- an unregistered or remote manager subject, wrong Realm ID, wrong scope kind,
  or Realm permission/scope mismatch is rejected as an invalid Seed;
- a Seed with no manager is valid and remains sealed;
- Put-only and Delete-only managers receive only their exact operation; and
- no online call can Put or Delete an administration binding or Realm-scoped
  operational permission.

### 8.2 Dynamic operational bindings

- a manager creates no Principal and queries no Core or Facet fact;
- after Core creates a runtime-generated Element UUID, the manager can Put
  exact read, reanchor, delete, and Facet bindings for a registered local
  subject or a syntactically valid exact remote subject;
- those owner operations then authorize through the unchanged exact relation;
- a prospective valid UUID may be bound before creation without making it
  exist;
- deleting one exact binding makes later exact authorization deny immediately
  while leaving every other binding unchanged;
- an unregistered same-Realm target returns retained not-found and never
  becomes a Principal; and
- an exact remote subject can receive an operational binding owned by this
  Realm without remote lookup, management authority, or a change to its home
  Realm, and the binding itself does not establish that remote identity.

### 8.3 BindingSetRevision, convergence, and concurrency

- every profile starts with a valid owner-scoped `BindingSetRevision`;
- actual insertion/removal advances it; Put-present and Delete-absent return a
  retained success with the unchanged `BindingSetRevision`;
- stale `BindingSetRevision` wins before same-Realm target registration or
  binding-membership evaluation, including when the desired state already
  holds;
- two competing changed operations from one expected `BindingSetRevision`
  produce one commit and one retained revision conflict;
- exact replay returns each changed success, no-op, not-found, and revision
  conflict after later relation changes;
- changing actor, kind, target binding, or expected `BindingSetRevision` under one
  OperationID returns operation conflict;
- invalid and denied requests do not reveal or consume an OperationID; and
- concurrent `Authorize`, `BindingSetRevision`, Put, Delete, and Close calls
  expose only complete linearized states under the supported single owner
  session.

### 8.4 Durability and strict recognition

- `New`, durable create, independent durable reopen, and process-exit recovery
  produce the same applicable behavior matrix;
- changed and outcome-only operations are atomic under definite pre-commit
  failure and exact lost-response process-exit gates;
- an indeterminate Store result retires the instance and can be resolved only
  by independent reopen plus exact replay;
- close retires the instance, releases the lock, and permits one independently
  validated reopen;
- the exact accepted FormatID and independent golden fixtures prevent encode
  and decode from drifting together;
- one arbitrary non-current marker, malformed Realm scope, malformed operation
  binding/outcome, impossible `BindingSetRevision` token, and inconsistent
  current state all fail without byte modification;
- Realm, Core, and Facet continue to reject each other's Stores; and
- Core and Facet Store bytes, markers, dependencies, owner boundaries, and
  operation histories remain unchanged.

### 8.5 Repository gate

The completion gate includes:

```powershell
go test -count=1 ./...
go test -shuffle=on -count=2 ./ucupa-realm ./ucupa-core ./ucupa-facet ./composition ./acceptance
# In a race-enabled build environment:
go test -race -count=1 ./ucupa-realm
go vet ./...
go mod tidy -diff
go run ./cmd/ucupa
git diff --check
```

Static checks must prove that no role, group, policy inheritance, wildcard,
path scope, Principal lifecycle, compatibility marker, migration path, shared
Store abstraction, public Store codec, new dependency, or Realm import of Core
or Facet implementation enters the slice.

## 9. Decision-time boundary and multi-Realm extension seam

This contract decides only how an established registered local Principal with
immutable bootstrap management authority may Put or Delete exact operational
`PermissionBinding` rows in that same Realm. It does not decide whether or how
a Principal may create identities, transfer management authority, or attenuate
or administer another Realm's relation.

This contract did not partially answer the predecessor map's question 15 and
created no subdecision. Any later decomposition belongs to an independent
contract that actually addresses that subject.

The Realm-owned contract freezes only these negative multi-Realm effects; it
does not prescribe a future multi-Realm mechanism:

- each Realm remains one logical authority for its own identity set and exact
  binding set; its `BindingSetRevision` namespace is scoped to one Store
  identity or one in-memory lifetime under that logical Realm authority, and
  one current fenced write authority publishes mutations for that logical owner;
  physical persistence may use shared infrastructure, adapters, or replication
  without merging those logical authorities;
- this contract creates no federation-wide super-authority, implicit Realm
  owner, cross-Realm revision, or global authority transaction;
- a binding for an exact remote `PrincipalRef` remains only a direct
  authorization fact owned by the target Realm; it neither establishes the
  remote identity nor grants or transfers Realm administration;
- a Cluster mount continues to relate Core ownership through delegation and
  acceptance; it does not create a child Realm, inherit Realm policy, or grant
  Realm administration;
- any future cross-Realm administration must be defined by a separate explicit
  accepted contract with named authority ownership; no such right may be
  inferred from a bootstrap manager binding, an ordinary remote-subject
  binding, deployment hierarchy, or relation transitivity; and
- tree- or mesh-shaped deployment may be projected from explicit relations,
  but this contract introduces no `ParentRealmID`, implicit membership
  inheritance, or Realm hierarchy as authorization truth.

These are non-effects of this contract, not an API, physical Store
topology, trust graph, or active federation slice. A future multi-Realm
contract must start from an observable need and define its own authority facts
rather than adding optional fields to this same-Realm mutation surface.

## 10. Explicit exclusions and stop rule

This contract does not add or decide:

- Principal creation, deletion, reuse, rename, profile, metadata, or lifecycle;
- online creation, transfer, rotation, deletion, or recovery of bootstrap
  management authority;
- roles, groups, organizations, policy documents, inheritance, wildcard,
  prefix, path, ancestry, subtree, tag, query, or attribute-based policy;
- cross-Realm administration, permission delegation, attenuation, depth,
  lineage, Realm hierarchy, transitive trust, or remote existence lookup;
- binding enumeration, list, query, pagination, current/change feed, diff,
  batch mutation, transaction bundle, event, outbox, or retention eviction;
- credential or token representation, authentication audience, issuer or key
  trust, key custody, rotation, credential revocation, expiry, time, or security
  evidence;
- Core or Facet truth, automatic permission creation from an owner mutation,
  cross-Store transaction, rollback, cancellation, or mirrored authorization;
- process API, transport, TLS, server/client, Console workflow, Operator,
  Archive, deployment, telemetry, backup/restore, HA, Partitioning, capacity,
  or production federation;
- a new database, dependency, shared Store, repository, operation-result type,
  public codec, migration, compatibility path, or parallel implementation; or
- any answer to production-capability-map decisions 2 through 15.

Retained operation history proves retry convergence only. It is not immutable
security evidence and must not be presented as an audit trail.

If a later implementation request requires mutable managers, identity
lifecycle, atomic multi-binding changes, authorization enumeration, cross-Realm
attenuation, an availability result for remote authorization, or another
primary failure domain, it must stop at this contract boundary. Those concerns
require separate architecture decisions rather than flags or optional fields in
this contract.

## 11. Documentation and completion boundary

Acceptance was limited to this contract, the architecture index, and the
development plan. It did not rewrite the accepted production capability map,
the completed Realm contracts, Concept v0, or completed Phase history at that
documentation-only boundary.

The user's later explicit Start authorized only the bounded implementation and
proof named here. Completion records `RealmScope`, immutable bootstrap manager
authority, aggregate `BindingSetRevision`, exact Put/Delete replay, the one
long-lived Realm-owned Store, strict recognition, retirement, and the vertical
acceptance result. Completion synchronizes only current-implementation summaries
and predecessor notices; it did not rewrite historical contract bodies or
decision-time capability-map horizons, and it did not answer the map's other
questions. It left question 15 outside this contract.

The completion gate passes, this contract is the current exact Realm
implementation authority, and the slice is closed. Completion authorized no
later implementation slice.
