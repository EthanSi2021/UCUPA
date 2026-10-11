# Realm Minimum-Model Correction

**Status: Complete — immutable Realm semantic predecessor**

**Successor notice:** The completed
[private Store FormatID correction](private-store-format-id-correction.md)
supersedes this body's numbered marker references and marker-preservation gates.
The completed Realm local-durability contract is
the immutable-bootstrap persistence predecessor. The completed
[Realm-owned PermissionBinding-administration contract](realm-owned-permission-binding-administration.md)
and its completed
[online caller-authentication successor](realm-owned-online-caller-authentication.md)
together are the current Realm semantic, convergence, lifecycle, and
Store-recognition authority. This body's statements that call this correction current or describe
administration as absent remain accurate records of their as-built scope; they
are not current repository status and define no compatibility input.

The accepted [P4 local Core/Facet increment](facet-local-truth-and-core-lifecycle.md#7-authority-qualified-permissions)
also supersedes this body's remaining unqualified Facet scope examples and codec.
The exact PrincipalRef + PermissionID + OperationScope model remains unchanged;
no predecessor scope constructor or Store reader remains current.

This contract defines the bounded Realm result originally accepted under the
now-frozen minimum-truth predecessor. It replaces the
former `Audience + Action`, `ExactScope`, and `TrustedRealms` authorization
fixture with one exact permission relation while preserving Realm identity,
registered local Principal identity, default denial, and owner-local
enforcement.

"Realm-only" names the semantic authority boundary. Mechanical changes to
shared values, Core and Facet permission constants, Composition, acceptance
fixtures, callers, and the deterministic runner belong to the same completed
public-surface correction; they add no Realm Store, authentication authority,
or second policy owner.

This contract is implemented and is the immutable semantic predecessor of the
current bounded Realm successors. The
completed Core minimum-model correction remains the UUID-only Core base
predecessor. Completion of this contract authorized no later implementation
slice.

## 1. Bounded result

Realm's complete business decision is exactly:

```text
established PrincipalRef + PermissionID + OperationScope -> Allow / Deny
```

The corrected minimum public values are logically:

```text
PermissionID = one opaque exact-equality value

OperationScope = one opaque exact target:
  - ElementScope(ElementUUID)
  - FacetScope(FacetKey{ElementUUID, FacetType})

Requirement {
  Permission PermissionID
  Scope      OperationScope
}

PermissionBinding {
  Subject    PrincipalRef
  Permission PermissionID
  Scope      OperationScope
}

Seed {
  ID              RealmID
  LocalPrincipals []PrincipalID
  Bindings        []PermissionBinding
}
```

`PermissionBinding` is an authoritative relation row. It is not a grant
operation, credential, identity proof, delegation lineage, role assignment,
or policy document. `Seed` remains only the immutable in-memory bootstrap input
for this corrected proof; it is not a persistent Store format or credential
bundle.

Realm keeps only:

- one stable Realm ID;
- the set of registered local Principal IDs;
- the exact set of `PrincipalRef + PermissionID + OperationScope` bindings.

Allow/Deny is derived by exact lookup and is never stored as another fact.
Repeated identical local Principal or binding entries have set semantics; they
create neither multiplicity nor an implicit role.

## 2. Permission and scope vocabulary

### 2.1 PermissionID

`PermissionID` is one opaque, comparable, non-empty value. Realm compares it
for exact equality and neither parses it nor reconstructs an owner and action
as two policy fields.

Each operation owner defines its complete permission constants. The current
permission values are frozen as:

| Owner | Public constant | Exact PermissionID |
|---|---|---|
| Core | `PermissionCreateRoot` | `ucupa.core.anchor.create-root` |
| Core | `PermissionCreateAnchor` | `ucupa.core.anchor.create` |
| Core | `PermissionCreateEntity` | `ucupa.core.entity.create` |
| Core | `PermissionReadElement` | `ucupa.core.element.read` |
| Core | `PermissionReanchorSubject` | `ucupa.core.reanchor.subject` |
| Core | `PermissionReanchorDestination` | `ucupa.core.reanchor.destination` |
| Core | `PermissionDeleteEntity` | `ucupa.core.entity.delete` |
| Core | `PermissionDelegateMount` | `ucupa.core.mount.delegate` |
| Core | `PermissionServeMount` | `ucupa.core.mount.serve` |
| Core | `PermissionAcceptMount` | `ucupa.core.mount.accept` |
| Facet | `PermissionPut` | `ucupa.facet.put` |
| Facet | `PermissionGet` | `ucupa.facet.get` |
| Facet | `PermissionDelete` | `ucupa.facet.delete` |
| Facet | `PermissionReconcile` | `ucupa.facet.reconcile` |

The owner-qualified token prevents current module collisions without restoring
`Audience` as a separate Realm field. There is no public concatenation helper,
parser, wildcard, prefix authorization, or global permission registry.
Credential audience remains an authentication concern and is never inferred
from this token.

### 2.2 OperationScope

`OperationScope` is an opaque comparable tagged value owned by shared values.
The existing `ElementScope` and `FacetScope` constructors return it.

- `ElementScope` contains one canonical production Element UUID.
- `FacetScope` contains one exact `FacetKey`: canonical Element UUID plus
  non-empty FacetType.
- the zero value and malformed UUID or FacetKey are invalid;
- a syntactically valid prospective Element UUID is a valid scope before Core
  has created or claimed it;
- Realm never reads Core or Facet to establish scope validity or existence.

This contract adds no subtree, Realm-management, Cluster, path, ancestry,
wildcard, query, or deployment scope. It did not decide broader read/query scope
or Realm administration.

## 3. Established identity and exact authorization

`Realm.Authorize(actor, requirements...) bool` remains a pure in-memory
Allow/Deny function. Its `actor` argument is contractually an already
established `PrincipalRef`.

The current deterministic Composition and tests inject that established
identity directly. This proves the authorization relation only. It does not
prove credential authentication, issuer trust, workload identity, Cluster
registration, or a secure process boundary.

The decision is:

1. a nil Realm, invalid PrincipalRef, invalid requirement, or empty requirement
   set denies;
2. a local actor (`actor.Realm == Realm.ID`) must name a registered local
   Principal;
3. every requested `PermissionID + OperationScope` must have one exact binding
   for the same complete PrincipalRef;
4. all requirements must match; fields from different bindings never combine;
5. otherwise the result is Allow.

A local permission binding never registers its subject implicitly. A binding
for an established remote PrincipalRef may authorize that exact remote subject
without a second `TrustedRealms` predicate. The binding itself does not
establish or authenticate the remote identity.

There is no `TrustedRealms` seed field, Realm trust map, or `Trusts` API in the
corrected kernel. Authentication adapters may later establish a remote
PrincipalRef using issuer/key trust, rotation, and revocation control state,
but that state is not part of pure authorization and is not selected here.

Realm boundaries remain peers, not a Realm tree. A binding for one remote
carrier Principal:

- grants only that exact Principal the exact operation in this Realm;
- does not grant another member of the remote Realm the same permission;
- does not constrain or modify the remote Realm's own local bindings;
- is not transitive through a third Realm;
- does not create bounded re-delegation or scope attenuation.

This contract did not decide Realm administration or bounded permission
delegation. Neither may be simulated by transitive trust, shared Principal text,
or inherited scope.

## 4. Owner enforcement and availability

Core and Facet continue to construct exact requirements, call one Authorizer,
and then enforce their own current facts. This correction changes permission
vocabulary, not Core or Facet truth.

- Core preserves its existing exact scope mapping: CreateRoot uses the proposed
  root UUID; CreateAnchor uses the parent Anchor; CreateEntity uses the
  containing Anchor; reads and Delete use the subject; Reanchor requires both
  subject and destination scopes in one all-of decision.
- AdmitMount preserves one all-of decision for parent creation plus exact mount
  delegation, followed by the exact remote carrier subject's serve permission;
  AcceptMount uses the exact mount UUID in the carrier Realm.
- Facet constructs the exact Facet `PermissionID` and `FacetScope` for its
  existing operation.
- Facet aligns its public key validator with that scope: a noncanonical
  ElementID returns Facet invalid before authorization or a Core-existence
  probe. This is the only bounded input-validation correction outside Realm;
  it changes no Facet record, revision, mutation, or visibility truth.
- the same key invariant applies when a durable Facet Store is opened. A
  pre-correction `/2` fixture containing a noncanonical ElementID fails closed
  as Facet Store error without byte modification. This changes no marker,
  layout, encoding, or transaction boundary and adds no reader or migration;
- Realm does not inspect Elements, Facets, routes, mount contexts, Stores, or
  operation results.
- permission does not imply existence, ownership, reachability, or mutation
  success.

The Authorizer remains boolean in this slice because the corrected Realm is a
pure in-memory decision with no Store, process, or network dependency.
`UNAVAILABLE` is not a third permission meaning:

- a missing route remains Composition `UNAVAILABLE` before owner authorization;
- a selected owner whose Authorizer denies returns the owner's existing denied
  result;
- Facet's Core-existence `UNAVAILABLE` remains separate from its permission
  decision.

A later durable or process Realm boundary must represent its own inability to
decide without converting unavailable into Deny, but this contract does not
select that adapter or change current Core/Facet error surfaces.

The Core permissions `mount.delegate`, `mount.serve`, and `mount.accept` remain
ordinary exact operation permissions. They do not create, grant, or transfer a
Realm `PermissionBinding`; Core `DelegationBinding` and `MountAcceptance` remain
the only corresponding federation authority facts.

## 5. Implemented boundary

The completed bounded correction made exactly these implementation changes:

1. shared values add `PermissionID` and `OperationScope`; `Requirement` becomes
   exactly `{Permission, Scope}`;
2. shared `Audience`, `Action`, `ExactScope`, `AudienceCore`, and
   `AudienceFacet` are removed with no aliases;
3. Realm replaces public `Grant` with `PermissionBinding`, changes `Seed` to
   `{ID, LocalPrincipals, Bindings}`, and removes `TrustedRealms`, the trust map,
   and `Trusts`;
4. Core and Facet expose the owner-qualified `Permission*` constants in Section
   2.1 and construct the corrected requirements;
5. Facet aligns public `FacetKey` input validation with the canonical Element
   UUID required by `FacetScope`; non-UUID test fixtures receive mechanical UUID
   replacements;
6. the Facet `/2` loader applies the same corrected key invariant without a
   marker, layout, encoding, transaction, or byte change; a noncanonical
   pre-correction Store is rejected rather than migrated;
7. Composition, acceptance fixtures, tests, and `cmd/ucupa` receive shape
   updates, and obsolete trust-predicate assertions are explicitly replaced by
   the established-remote-identity expectations in Section 6.3;
8. current README and status documents are updated only after the corrected
   gate passes.

Facet formerly reused the shared `Action` type as a private retained mutation
kind. The correction split that implementation detail into a Facet-private
operation kind while preserving its existing Store bytes, marker, revision,
OperationID binding, and replay behavior. `PermissionID` did not become a new
persisted Core or Facet mutation fact.

No Core Store `/3` change is allowed. Facet Store `/2` keeps its marker, layout,
key/value encoding, transaction boundary, record semantics, revision,
OperationID, and retained outcomes; only decoded-key validation follows the
corrected canonical UUID invariant. No Realm Store, reader, migration,
dependency, or compatibility surface is added.

## 6. Completed black-box gate

### 6.1 Shared values and permission namespace

- zero `PermissionID` and zero `OperationScope` are invalid;
- Element scope accepts one canonical UUID and rejects empty, noncanonical,
  wrong-version, or wrong-variant Element IDs;
- Facet scope remains distinct from Element scope and requires the exact UUID
  plus non-empty FacetType;
- every permission constant in Section 2.1 is non-empty and globally unique;
- Core and Facet permissions with similar operation words remain unequal;
- there is no owner/action recombination, parsing, prefix match, or wildcard
  behavior.

### 6.2 Realm exact relation

- one registered local Principal plus one exact binding allows only the exact
  PermissionID and OperationScope;
- wrong Principal, Realm, permission, scope kind, Element UUID, or FacetType
  denies;
- an unknown local Principal cannot be made valid by including a binding;
- the same PrincipalID text in two Realm IDs remains two different subjects;
- permission from one binding and scope from another cannot be recombined;
- a multi-requirement decision allows only when every exact binding exists;
- nil Realm, invalid input, and an empty requirement set deny.

### 6.3 Remote established Principal boundary

- an established remote PrincipalRef with its exact local Realm binding allows
  without a `TrustedRealms` predicate;
- the same remote PrincipalRef without that binding denies;
- a binding for Realm B's Principal does not authorize Realm B's other
  Principals or any Realm C Principal;
- two independently constructed Realms keep independent local Principal sets
  and permission bindings;
- tests describe injected remote actors as already established and make no
  authentication, issuer, key, or revocation claim.

### 6.4 Owner integration and regression

- each currently authorization-gated Core operation presents exactly its
  frozen PermissionID and Element scope to the Authorizer;
- each currently authorization-gated Facet operation presents exactly its
  frozen PermissionID and Facet scope;
- `ConfirmElement`, `MatchesDelegation`, `MatchesMountAcceptance`, and route
  registration integrity checks remain record-free availability/integrity
  probes and receive no permission requirement;
- invalid Facet Element IDs return Facet invalid before the Authorizer or Core
  existence port is called; for valid inputs, validation-before-authorization
  and authorization-before-owner-state ordering remain unchanged;
- a valid `/2` Store with canonical keys reopens unchanged, while a
  pre-correction `/2` fixture containing a noncanonical ElementID returns Facet
  Store error without modifying its bytes;
- the owner-only mount routing positive and negative permission cases remain
  green without `TrustedRealms`;
- a missing route returns Composition `UNAVAILABLE` before calling an owner
  Authorizer; after the route exists, missing selected-owner permission returns
  that owner's denied result;
- a denied Facet call makes no Core-existence probe, while an authorized Facet
  call whose Core decision is unavailable returns Facet `UNAVAILABLE`;
- after one durable Core and one durable Facet success, the original authorized
  actor's exact OperationID replay returns the retained result; a different
  unauthorized actor is denied before operation lookup, and the same different
  actor becomes operation-conflict only after its exact permission is supplied;
  after durable reopen the actor-bound outcomes remain the same, and
  `PermissionID` never substitutes for the retained actor identity;
- missing create, delegate, or serve permission from `AdmitMount` leaves the
  parent with neither mount nor `DelegationBinding`;
- after successful admission, missing accept permission leaves the independent
  carrier without `MountAcceptance` and does not roll back the parent's mount
  or `DelegationBinding`; supplying any of these permissions only authorizes
  the corresponding Core operation and never creates a Realm binding;
- Core Store `/3`, Facet Store, restart, CAS, exact OperationID replay,
  pre-commit failure, lost-response, route-loss, and `cmd/ucupa` regressions all
  remain green;
- Realm, Core, Facet, and Composition add no compatibility branch.

### 6.5 Static subtraction and repository gate

The completed implementation gate includes executable checks equivalent to:

```powershell
if (rg -n --glob '*.go' '\bAudience\b|AudienceCore|AudienceFacet|\bExactScope\b|\bTrustedRealms\b|\bTrusts\s*\(|\btrusts\b' .) { throw 'removed Realm policy dimension remains' }
if (rg -n --glob '*.go' '\bvalues\.Action\b' .) { throw 'shared Action policy type remains' }
if (rg -n '\btype Action\b' shared/values) { throw 'shared Action declaration remains' }
if (rg -n --glob '*.go' '\bAction[A-Z][A-Za-z0-9_]*\b' ucupa-core ucupa-facet) { throw 'old public Action permission constant remains' }
if (rg -n --glob '*.go' '\bGrant\b|\bGrants\b|\bPrincipals\b' .) { throw 'old Realm Seed or Grant surface remains' }
if (rg -n '\bPermissionID\b' ucupa-core/store.go ucupa-facet/internal/namespace/store.go) { throw 'permission identity leaked into owner mutation Store' }
if (rg -n --glob '*.go' --glob '!*_test.go' '"ucupa/(ucupa-core|ucupa-facet|composition)"' ucupa-realm) { throw 'Realm imported a resource owner or Composition' }
if (rg -n --glob '*.go' --glob '!*_test.go' '"ucupa/ucupa-realm"' ucupa-core ucupa-facet) { throw 'resource owner imported Realm implementation' }
go test -count=1 ./...
go test -shuffle=on -count=2 ./ucupa-realm ./ucupa-core ./ucupa-facet ./composition ./acceptance
go vet ./...
go mod tidy -diff
go run ./cmd/ucupa
git diff --check
```

`gofmt -d` on changed Go files must produce no output. The dependency and
module graph must remain unchanged.

A narrowly scoped package-private Store gate additionally proves that the Facet
marker remains `ucupa.facet.local/2` and its Put, Delete, and Reconcile
operation codes remain exactly `1`, `2`, and `3`. Encoding and decoding cannot
change together and hide a format change. The same gate proves canonical-key
reopen and byte-preserving rejection of a noncanonical-key `/2` fixture.
`ucupa-core/store.go` has no production diff, and all existing Core `/3` format
gates remain green.

## 7. Explicit exclusions

This correction does not add or decide:

- Realm persistence, a database choice, schema, backup, revision/CAS,
  OperationID, retained result, mutation API, or runtime lifecycle;
- credential or token representation, credential audience, signing keys,
  issuer trust, online/offline validation, rotation, revocation, expiry, clock,
  or security evidence;
- an `EstablishedPrincipal` wrapper that would merely assert authentication
  without proving it;
- Realm administration, Principal creation/deletion, Principal ID reuse,
  binding grant/revoke operations, bounded delegation, attenuation, depth, or
  lineage;
- `RealmScope`, subtree scope, query/list scope, Cluster scope, wildcard scope,
  path scope, or a policy language;
- roles, role inheritance, groups, organizations, users, workflow, approval,
  resource discovery, metadata, or a global identity/permission directory;
- transitive Realm trust, a parent/child Realm hierarchy, or inherited carrier
  authority;
- Cluster bootstrap/registration, workload identity, Cluster–Realm–Principal–
  mount–endpoint binding, federation leases, or network transport;
- a Realm availability result, process API, RPC, server/client, deployment,
  telemetry, or infrastructure dependency;
- any Core or Facet authoritative-truth, mutation, or Store change beyond the
  explicitly bounded Facet invalid-key and decoded-key validation correction;
  any federation, Archive, Operator, or Console semantic change;
- a compatibility alias, dual policy path, Store migration, format version
  ladder, or permission parser/registry.

This correction answered none of the predecessor
production capability map's numbered questions.
In particular, it did not decide broader read/query scope, Cluster registration,
authentication, time, Realm administration/delegation, formal evolution, or
capacity.

## 8. Documentation and history boundary

Proposal review and the acceptance transition were limited to this contract,
the architecture index, and the development plan. They changed documentation
only and authorized no implementation. A later explicit start separately
authorized the bounded source, test, caller, runner, and documentation changes
listed in Section 5; it changed no dependency or generated artifact.

Current status and package documents are synchronized at completion.
Historical Concept/Phase documents receive at most a top-level successor
pointer; their as-built bodies are never rewritten to resemble the corrected
implementation.

## 9. Completion and stop rule

This contract is complete. Acceptance changed only the bounded documentation
state; a separate explicit start authorized the implementation now recorded by
this document. Completion authorizes no later shared value, Realm, Core, Facet,
Composition, acceptance, command, test, Store, dependency, generated artifact,
or documentation slice.

If a later change would require a Realm Store, authentication or issuer-trust
mechanism, trust topology, Realm administration or delegation, broader scope,
any Core or Facet Store byte/marker change, a new dependency, or an answer to a
question excluded by this contract, work stops and returns to architecture
review. None of those requirements is absorbed into this completed correction.
