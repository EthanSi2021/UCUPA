# Realm-Owned Online Caller Authentication

**Status: Complete Realm-owned online caller-authentication contract —
implemented; slice closed**

P4 successor notice: the accepted
[Core/Facet increment](facet-local-truth-and-core-lifecycle.md#7-authority-qualified-permissions)
has replaced the remaining Facet scope profile and its private Realm marker.
The P4/P5 marker was `ucupa.realm.local@247057c2b3804bd989a1ef1c9e181f27`;
literal marker/Facet-scope examples below describe this contract's earlier
implementation boundary, not another accepted reader. Caller establishment,
credential and binding behavior remain governed here and by the binding contract.
The P4 local implementation has passed full verification and delivery acceptance;
its evidence and submission record are maintained in the development plan.

P7.1 implementation notice: the exact AuthorityScope binding discriminator uses
`ucupa.realm.local@a33aa9c69815480491bca89ce0c6d653` as the single current Store marker.
It changes no caller/credential or grant-administration rule and adds no RealmAttachment.
Verification and delivery acceptance are recorded in the [development plan](development-plan.md);
the [format record](private-store-format-id-correction.md#2-exact-accepted-replacement-formatids)
does not define a predecessor reader or compatibility path.

Stage 2 successor notice: the implemented
[RealmAttachment increment](realm-attachment-contract.md) adds owner-persisted
contacts and controlled local/network consumption of this same Realm kernel.
It supersedes the earlier local-only assembly boundary, not caller, credential
or binding decisions. Finite remote handles are run-local service references,
not serialized local proofs or offline permissions. Exact current scope and
verification limits are recorded in the authority manifest and development plan.

This completed contract defines the smallest Realm-owned mechanism that can
establish an already registered local `PrincipalRef` from caller-held evidence,
then rotate or revoke that evidence online. It is the implemented bounded answer
to the predecessor production-capability map's decision-time question 8.

Acceptance froze the observable contract. The user's later explicit Start
authorized the bounded implementation now recorded here. The completed
[Realm-owned PermissionBinding-administration contract](realm-owned-permission-binding-administration.md)
remains authoritative for binding administration; this successor adds only
Realm-owned caller establishment and credential lifecycle. Completion closes
the slice and authorizes nothing later.

Terminology is exact:

- **registered local Principal** means a `PrincipalID` already present in this
  Realm's immutable local Principal set;
- **credential** means one permanently claimed opaque `CredentialID` whose
  Realm-owned state is either active with one registered local Principal and
  verifier, or a revoked tombstone retaining only that immutable Principal
  association;
- **evidence** means the caller-held audience, credential ID, and bearer secret
  presented to Realm;
- **online authentication** means that the current Realm owner must evaluate
  current credential state for every establishment; it does not mean Store I/O,
  RPC, or an external identity provider;
- **established PrincipalRef** means only the successful result of one
  `Authenticate` call, not a session, token, capability, or durable login; and
- **Realm-owned** means one logical Realm is authoritative for the fact. It
  does not prescribe one process, machine, disk, or replication topology.

There is no general `Local` authentication profile shared by Realm, Core, and
Facet. Core and Facet own no Principal or credential truth.

## 1. Required observable result

The current Realm permission kernel deliberately accepts an already
established `PrincipalRef`. That value alone cannot prove who supplied it: it
is an ordinary constructible value and must remain so for exact permission
matching and trusted in-process adapters.

The accepted minimum establishment path is:

```text
caller-held exact Realm audience + opaque CredentialID + high-entropy bearer secret
+ current Realm-owned active verifier
-> exact registered local PrincipalRef or one undifferentiated rejection
```

The accepted minimum lifecycle path is:

```text
current caller proof
+ exact immutable bootstrap lifecycle authority
+ one expected CredentialSetRevision
+ one Realm-owner-scoped CredentialOperationID
-> atomically rotate or revoke one credential
+ retain the exact definitive result
```

Realm remains the sole owner of:

```text
CredentialID -> active {registered local PrincipalID + verifier}
             | permanent revoked {registered local PrincipalID}
```

The raw bearer secret is never a Realm-owned persisted fact. Realm receives it
transiently, derives a verifier, compares that verifier, and does not retain or
return the secret.

Authentication changes only whether evidence currently establishes a
`PrincipalRef`. Permission continues to mean the exact existing relation:

```text
established PrincipalRef + PermissionID + OperationScope -> Allow | Deny
```

Credential rotation or revocation does not edit a `PermissionBinding`.
Binding mutation does not issue, rotate, or revoke a credential.

## 2. One concrete Decision 8 profile

### 2.1 Public evidence values

Realm exposes these exact value shapes:

```go
type CredentialID [16]byte
type CredentialSecret [32]byte

type CredentialEvidence struct {
    Audience values.RealmID
    ID       CredentialID
    Secret   CredentialSecret
}

type CredentialBootstrap struct {
    Subject  values.PrincipalID
    Evidence CredentialEvidence
}
```

`CredentialID` is an opaque 128-bit value. The all-zero value is invalid. A
valid ID is permanently claimed within one Realm Store identity or one
in-memory Realm lifetime and is never reused, reactivated, or remapped to
another Principal.

`CredentialSecret` is exactly 256 bits. The all-zero value is invalid. It is a
bearer secret, not a password, passphrase, user PIN, recovery code, API label,
or textual identifier. Low-entropy caller-chosen text is unsupported.

The evidence producer must obtain both ID and secret from a cryptographically
secure random source and retry an all-zero result. Realm exposes no generation
helper, random-source error, issuer, enrollment service, or SDK convenience in
this contract.

Realm validates only the fixed length, nonzero shape, exact audience, and ID
claim. It cannot infer entropy, unpredictability, generator provenance, or
secret reuse from submitted bytes. The 256-bit security claim therefore
applies to a conforming evidence producer, not to every structurally accepted
candidate; low-entropy evidence violates this profile even though Realm cannot
distinguish it mechanically. Only the owning Realm can claim that a proposed ID
is unused.

### 2.2 Exact audience

The authentication audience is exactly the receiving Realm's current
`RealmID`. It is not a URL, process, host, Cluster, module, application, tenant,
path, generic string claim, or extensible audience list.

Every bootstrap or replacement credential must name the owning Realm's exact
ID. `Authenticate` returns `ErrUnauthenticated` when otherwise well-formed
evidence names another Realm. The same evidence therefore cannot establish a
caller in two Realms merely because they register equal Principal IDs.

This exact field is the complete audience model for the bounded contract. No
generic `Audience` abstraction or audience plugin is added.

This choice treats one logical Realm as one credential relying-party boundary.
It provides no isolation between Realm/Core/Facet host adapters, processes, or
endpoints inside that Realm: every adapter allowed to receive complete bearer
evidence is trusted not to replay it elsewhere in the same Realm. TLS would
protect transport but would not remove that trust. If a future deployment
requires mutually untrusted module or endpoint audiences, implementation must
stop and define service-bound evidence or proof of possession together with
the actual process boundary; this RealmID-only profile does not claim that
isolation.

### 2.3 Exact verifier derivation

Realm stores one 32-byte verifier for an active credential. Given evidence
`E`, the verifier is exactly HMAC-SHA-256 with `E.Secret` as the 32-byte key and
this canonical message:

```text
ASCII("ucupa.realm.online-credential\x00")
+ uint32-big-endian(len(exact bytes of E.Audience's underlying string))
+ exact bytes of E.Audience's underlying string
+ exact 16 bytes of E.ID
```

`RealmID` already has one underlying string representation. Its exact bytes are
encoded without adding a new UTF-8 invariant; there is no case folding,
trimming, Unicode normalization, parsing, aliasing, or alternate text form at
this boundary. A byte length greater than the representable unsigned 32-bit
length is invalid.

Authentication derives this verifier once and compares it with the active
stored verifier using constant-time byte comparison. For an unknown or revoked
ID it still performs the comparison against a fixed dummy sentinel and
accepts only when both active state and comparison match. An unknown ID,
revoked ID, wrong secret, and wrong audience all return the same public
`ErrUnauthenticated` and the zero `PrincipalRef`. The implementation must not
expose the stored/unknown/revoked distinction through errors or result fields.

This contract does not claim complete timing indistinguishability across map
lookup, process scheduling, or a future network. That requires the threat model
of the later process-boundary contract. It does require constant-time verifier
comparison and one public authentication-failure class.

No verifier scheme enum, hash negotiation, algorithm registry, salt field,
work-factor field, issuer, signature, certificate, signing key, KMS adapter, or
pluggable credential interface enters this profile.

The verifier checks knowledge of the bearer secret; it is not a MAC over the
credential row or Realm Store. A party able to rewrite authoritative Realm
bytes can already rewrite Principals and permission bindings, so Store/host
integrity remains a separate data-protection boundary and is not overstated as
a property of this HMAC.

### 2.4 Why the profile is online and not a token

The evidence is not self-authenticating. Acceptance depends on the current
active verifier or tombstone held by the Realm owner, so revocation is observed
on the next `Authenticate` call without a revocation list, expiry, or clock.

There is no signed token, offline verifier, bearer session, refresh token,
cookie, lease, expiration, grace interval, key-distribution protocol, issuer
trust graph, or cached authorization result. This contract did not answer the
predecessor map's question 9 and introduces no time semantics.

This bearer evidence is deliberately static and therefore replayable by any
party that obtains its complete bytes. The contract provides credential
verification and lifecycle, not confidentiality or communication replay
protection. A future process boundary must protect evidence in transit and
authenticate each request, or separately accept a different challenge/session
contract. This contract must not be described as supplying TLS, proof of
possession, channel binding, request nonces, or network replay prevention.

A public-key challenge is not hidden behind this simpler surface. Completing
that alternative would also require challenge ownership, transcript and
endpoint binding, one-use replay state, bounded cleanup, and a real process or
channel boundary. Those are not facts that the current in-process Realm can
truthfully establish, and authentication and network distribution are separate
reviewed slices in the accepted delivery rule. A signed token would additionally
require issuer/signing-key custody, offline-validation policy, expiry, and
revocation propagation. The accepted bearer profile is the smallest complete
Realm-owned establishment and immediate-revocation mechanism that leaves those
later concerns explicit.

## 3. Bootstrap facts and immutable management authority

### 3.1 Credential bootstrap

`Seed` contains:

```go
Credentials []CredentialBootstrap
```

Each bootstrap entry must:

- name a registered local `PrincipalID` in that Seed;
- contain structurally valid evidence whose audience equals `Seed.ID`; and
- use a `CredentialID` not repeated or otherwise claimed in the Seed.

An unregistered subject, remote subject representation, empty audience,
all-zero ID, all-zero secret, foreign audience, or duplicate ID makes the
entire Seed invalid with `ErrInvalidSeed`. Seed validation is atomic.

Seed may install zero credentials, multiple credentials for one Principal, or
credentials for only some registered Principals. A zero-credential Realm is
valid and deliberately cannot establish a caller through this mechanism.
There is no implicit credential, default administrator, root secret,
break-glass credential, recovery key, or requirement that every Principal be
reachable.

There is no online Add, Create, Enroll, or Reactivate operation. Rotation needs
one active old credential and preserves its Principal, so a Principal with no
active credential cannot gain one through this contract. Complete loss or
revocation therefore seals that Principal unless a separately accepted
recovery/issuance authority is introduced.

Realm derives each verifier during construction and does not retain the raw
secret in owner state or Store. The caller is responsible for distributing and
protecting its evidence. Go cannot guarantee erasure of every caller or runtime
copy; the implementation must nevertheless avoid deliberate secret retention,
formatting, logging, outcome binding, or persistence.

### 3.2 Two exact lifecycle permissions

Realm exposes:

```go
const (
    PermissionRotateOwnCredential values.PermissionID =
        "ucupa.realm.credential.rotate-own"
    PermissionRevokeCredential values.PermissionID =
        "ucupa.realm.credential.revoke"
)
```

Their only valid requirements are:

```text
PermissionRotateOwnCredential + RealmScope(current RealmID)
PermissionRevokeCredential    + RealmScope(current RealmID)
```

Like the current Put/Delete manager authority, these exact bindings may be
installed only by `Seed`, only for registered local Principals, and are
immutable online. Put/Delete binding validation must treat all four Realm
administration permissions as invalid online targets.

Rotate-own and Revoke authority may be seeded independently. A Realm with no
credential-lifecycle authority is valid and sealed against those mutations.
There is no implicit owner, authority inheritance, role, manager quorum,
last-manager exception, remote manager, or self-grant path.

These permissions govern credential lifecycle only. Rotate-own permits a
Principal to replace only the exact credential currently establishing that
same call. Revoke permits its holder to revoke another credential but not the
credential establishing that call. Neither permission creates or deletes a
Principal, edits permission bindings, recovers or replaces another Principal's
credential, or administers another Realm.

Third-party reset, recovery, issuance, or enrollment is absent. It requires a
separate accepted authority contract and cannot be inferred from either
lifecycle permission.

## 4. Accepted public Realm surface

Realm exposes:

```go
// CredentialOperationID identifies one exact Rotate/Revoke mutation within
// one Realm Store identity or one in-memory Realm lifetime. Its namespace is
// independent from the existing binding-administration OperationID.
type CredentialOperationID string

// CredentialSetRevision is an opaque equality token for the complete
// credential state of one Realm Store identity or in-memory lifetime.
type CredentialSetRevision struct {
    // private owner-controlled representation
}

func (realm *Realm) Authenticate(
    evidence CredentialEvidence,
) (values.PrincipalRef, error)

func (realm *Realm) CredentialSetRevision() CredentialSetRevision

func (realm *Realm) RotateCredential(
    actorProof CredentialEvidence,
    oldID CredentialID,
    replacement CredentialEvidence,
    expectedRevision CredentialSetRevision,
    operationID CredentialOperationID,
) (CredentialSetRevision, error)

func (realm *Realm) RevokeCredential(
    actorProof CredentialEvidence,
    targetID CredentialID,
    expectedRevision CredentialSetRevision,
    operationID CredentialOperationID,
) (CredentialSetRevision, error)
```

`Authenticate` is the only new establishment operation. It returns exactly:

- the local `PrincipalRef{Realm: realm.ID(), Principal: registeredSubject}` on
  valid active evidence;
- zero `PrincipalRef` plus `ErrInvalid` for structurally invalid evidence;
- zero `PrincipalRef` plus `ErrStore` for a nil, closed, or retired Realm; or
- zero `PrincipalRef` plus `ErrUnauthenticated` for every well-formed but
  unaccepted proof, including foreign audience, unknown ID, revoked ID, and
  wrong secret.

It performs no permission authorization and returns no credential status,
session, token, expiry, reason, replay indicator, or Store record.

`CredentialSetRevision()` returns the current opaque token and nothing else. A
nil, closed, or retired Realm returns the zero invalid token. As with the
current `BindingSetRevision`, an in-memory Realm remains active after `Close`
because its Close is a no-op.

Lifecycle operations accept proof rather than a caller-supplied
`PrincipalRef`. Each operation authenticates that proof inside the same Realm
owner critical section, derives the actor's exact local `PrincipalRef`, and
then checks the corresponding immutable lifecycle binding.

This is intentional asymmetry. Existing Core, Facet, and binding-administration
Go calls remain trusted in-process surfaces that receive an already established
`PrincipalRef`; this contract does not silently redesign every public method or
pretend an in-process value is unforgeable. A future process adapter must call
Realm authentication at its boundary before passing the established reference
to those methods.

## 5. Credential lifecycle semantics

### 5.1 One aggregate CredentialSetRevision

There is one owner-scoped `CredentialSetRevision` for the complete active and
revoked credential set of one Realm authority:

- every successfully constructed Realm starts with one valid nonzero token;
- it is equality-only and exposes no ordering or arithmetic;
- it is scoped to one Store identity or one in-memory Realm lifetime;
- one successful rotation or active revocation advances it exactly once;
- revoking an already revoked ID is a successful retained no-op and does not
  advance it; and
- mutation anywhere in the credential set conservatively conflicts with a
  stale expected token.

`BindingSetRevision` and `CredentialSetRevision` are separate aggregate facts.
A binding mutation does not invalidate a credential CAS, and a credential
mutation does not invalidate a binding CAS. They may have distinct private
scope and generation representations even though one Realm owner session
linearizes both.

### 5.2 Rotation

For one new authorized operation whose expected revision matches, rotation
requires:

- `oldID` equals the exact `actorProof.ID` establishing this call;
- the replacement audience equals the current Realm ID;
- the replacement ID is fresh and has never been active or revoked in this
  Realm authority; and
- replacement evidence is structurally valid.

The atomic result is:

```text
old active CredentialID -> permanent revoked tombstone
fresh replacement ID    -> active verifier for the same PrincipalID
CredentialSetRevision   -> one later token
```

The caller proposes and already possesses the replacement secret before the
call. Realm returns no secret. Rotation cannot change the subject or audience,
reuse the old ID, reactivate a tombstone, or map a previously claimed ID.

Successful actor-proof authentication plus exact ID equality proves that the
old credential is active inside the same owner critical section. Rotation has
no target-not-found outcome: an unknown, revoked, or wrong-secret proof fails
earlier as `ErrUnauthenticated`, while a valid proof naming a different old ID
returns unretained `ErrDenied`. A replacement ID that is already active or
tombstoned returns retained `ErrCredentialConflict`. No failure partially
changes a credential or revision.

Only self-rotation is permitted. For a new `CredentialOperationID`, an `oldID`
different from `actorProof.ID` returns unretained `ErrDenied` before CAS or
credential-state inspection. It cannot reset, replace, or inspect another
credential, including another credential of the same Principal; that credential
must establish its own rotation call.

If self-rotation commits, the old proof becomes invalid, but the replacement
proof establishes the same Principal and can replay the same
`CredentialOperationID`. If commit did not occur, the old proof remains valid
and can retry. Retained lookup therefore precedes the present-proof equality
check. Operation identity binds the authenticated Principal, not the particular
proof used to establish it.

### 5.3 Revocation

For one new authorized operation whose expected revision matches:

- an active target becomes a permanent revoked tombstone and advances the
  revision once;
- an already revoked target returns retained success with the unchanged
  revision; and
- a never-claimed target returns retained `ErrNotFound`.

Revocation deletes the active verifier but never the ID claim or its immutable
Principal association. The remaining `CredentialID + PrincipalID` tombstone is
the minimum control metadata: the permanent ID claim prevents reuse, while the
retained immutable Principal permits strict reopen to prove that every rotation
edge preserved its Principal. It contains no display name, reason, time, actor,
verifier, or lifecycle log.

For a new `CredentialOperationID`, `targetID == actorProof.ID` is invalid. The
contract forbids proof-self-revocation because a lost committed response could
otherwise destroy the only proof able to authenticate and resolve its retained
outcome. A retained exact replay or conflict is resolved first because
operation identity intentionally does not bind the particular actor proof; a
target ID that became the actor's current proof later must not change a
historical result. A revoke authority may revoke another credential belonging
to the same Principal; that credential may instead establish and rotate itself.

Revocation affects future `Authenticate` calls after publication. It does not
invalidate an ordinary copied `PrincipalRef`, cancel an operation whose
authentication already linearized, terminate a session, or roll back a Core or
Facet mutation. This contract creates no session or cross-owner transaction to
cancel.

## 6. Validation, authentication, authorization, CAS, and replay

### 6.1 Stable error boundary

The Realm surface uses:

```text
ErrInvalid
ErrUnauthenticated
ErrDenied
ErrNotFound
ErrCredentialConflict
ErrRevisionConflict
ErrOperationConflict
ErrStore
ErrInvalidSeed
```

`ErrUnauthenticated` is the one authentication rejection. It does not reveal
whether an ID is absent, revoked, foreign-audience, or paired with a wrong
secret.

`ErrCredentialConflict` means a proposed replacement ID was permanently
claimed. `ErrRevisionConflict` remains one Realm-owned error identity usable by
either aggregate CAS; the concrete method and token type identify which
aggregate conflicted. Its current message must not be treated as public
semantic data. `ErrNotFound` likewise identifies the stable absence class; its
generic English message is not semantic data.

Lifecycle operations return a valid `CredentialSetRevision` only on success.
Every error returns the zero token. There is no shared operation-result type,
replay flag, status enum, or error detail carrying credential state.

### 6.2 Authentication ordering

`Authenticate` preserves:

```text
evidence structural validation
-> active owner-session gate
-> exact current Realm audience and credential verification
-> exact local PrincipalRef result
```

Structurally invalid zero values return `ErrInvalid`. On an active owner,
well-formed foreign-audience or unmatched evidence returns
`ErrUnauthenticated`. A retired durable owner fails closed with `ErrStore`.

### 6.3 Lifecycle mutation ordering

Every lifecycle mutation preserves:

```text
complete request-shape validation
-> active owner-session gate
-> actor-proof authentication
-> exact immutable lifecycle authorization
-> Realm-specific replacement-audience check
-> retained CredentialOperationID lookup
-> present-proof ownership/safety check for a new CredentialOperationID
-> expected CredentialSetRevision comparison
-> target-state and fresh-ID checks
-> one atomic commit and publication
```

Request shape includes structurally valid, nonzero evidence and credential IDs,
a nonempty replacement audience where present, a valid expected token, and a
nonempty `CredentialOperationID`. The exact replacement-audience comparison
requires the active Realm ID and therefore follows the owner-session and
authority gates but still precedes replay; a foreign replacement returns
unretained `ErrInvalid`. Malformed input consumes no `CredentialOperationID`.
Authentication precedes replay so a caller cannot use a known operation ID
without current valid evidence.
Authorization also precedes replay; immutable bootstrap authority makes that
result stable for this contract.

Operation lookup then returns an exact retained result or operation conflict
before applying a present-proof rule. For only a new
`CredentialOperationID`, Rotate requires `oldID == actorProof.ID` and otherwise
returns unretained `ErrDenied`; Revoke requires `targetID != actorProof.ID` and
otherwise returns unretained `ErrInvalid`. CAS then precedes credential-state
inspection so a stale authorized caller learns only the aggregate conflict,
not which later credential state now holds.

### 6.4 One credential-lifecycle operation namespace

`CredentialOperationID` is caller-proposed, nonempty, opaque, and scoped to one
Realm Store identity or in-memory lifetime. Rotate and Revoke share this one
credential-aggregate namespace, so the same ID cannot denote both kinds.

The existing binding-administration `OperationID` remains in its existing
independent namespace. Equal underlying strings in `OperationID` and
`CredentialOperationID` are unrelated and do not conflict, reveal history, or
share retained records. Neither namespace orders or atomically groups the other
aggregate.

A credential operation binds only durable, non-secret normalized fields:

```text
authenticated actor PrincipalRef
+ operation kind
+ target/old CredentialID
+ replacement CredentialID and verifier when rotating
+ expected CredentialSetRevision
```

It never binds or stores the actor proof ID, actor secret, or replacement raw
secret. Binding the established actor rather than its proof permits exact
self-rotation recovery while preserving actor identity.

After current authentication and authorization:

| CredentialOperationID state | Result |
|---|---|
| Exact same normalized binding | Return its retained historical outcome without current CAS or target evaluation |
| Same ID with changed actor, kind, target, replacement verifier, or expected revision | Return `ErrOperationConflict` |
| New ID | Evaluate current CAS and credential state, then retain one definitive outcome |

Authorization before lookup is for the credential operation kind the caller
requested. Within the credential namespace, exact replay returns its retained
outcome and any changed bound field or Rotate/Revoke kind returns
`ErrOperationConflict`. No credential lookup consults binding-operation state.

Changed success, revoke no-op, Revoke not-found, credential conflict, and
revision conflict are definitive and retained. Invalid input, unauthenticated
proof, denial, present-proof ownership/safety rejection, operation conflict,
and definite pre-commit Store failure create no new credential operation
record.

Exact replay returns the historical result after later mutations and never
restores historical credential state. Normalized operation equality compares
the replacement verifier, not raw secret bytes that Realm deliberately does not
retain. Two raw secrets deriving the same verifier are indistinguishable at
this boundary; HMAC-SHA-256 makes that computationally infeasible for conforming
random secrets but the contract does not pretend the derivation is injective.

Two new lifecycle calls with distinct `CredentialOperationID` values and the
same expected revision are serialized. If the first changes credential state
and the second caller's proof remains active, the second reaches CAS and retains
revision conflict. If the first call rotates or revokes the proof needed by the
second, the second instead fails earlier as unretained `ErrUnauthenticated` and
consumes no `CredentialOperationID`. In particular, two competing
self-rotations using the same old proof produce one changed success and one
unretained authentication failure. A successful revoke no-op leaves the
revision available to the next distinct operation.

## 7. Linearization, lifecycle, and failure

One Realm owner session linearizes:

```text
Authenticate
Authorize
BindingSetRevision
CredentialSetRevision
PutBinding / DeleteBinding
RotateCredential / RevokeCredential
Close
```

Each method observes a complete owner state. A credential mutation's
authentication, lifecycle-authority check, operation lookup, present-proof
constraint, CAS, credential decision, durable commit when present, and
publication occur in one owner-controlled critical section. No caller observes
an active replacement before the old credential is revoked or a later revision
before both facts hold.

`Authenticate` and `Authorize` use the published in-memory state and perform no
Store I/O. A successful durable mutation publishes only after its complete
transaction commits.

A definite pre-commit failure changes no credential, revision, claim,
`CredentialOperationID`, or outcome and may leave the Realm owner session
active. A Store result whose commit status cannot be proved returns `ErrStore`
and retires the complete durable Realm. Thereafter both authentication and
authorization fail closed, all administration returns `ErrStore`, and the
caller must independently reopen the Store.

After reopen, a currently valid proof plus the same `CredentialOperationID`
resolves whether a definitive outcome committed. The self-rotation rule ensures
either the old or replacement proof is valid for this resolution. A failure
that leaves no current valid authorized proof is not repaired by a bypass; the
Realm is sealed until an independently designed recovery mechanism exists.

`Close` retires one durable owner and releases its Store lock. The caller must
discard that owner regardless of Close result. `Close` remains a no-op for an
in-memory or already retired Realm. Ordinary copies of the public `Realm`
handle continue to share one mutex, state, Store lifetime, and retirement gate.

## 8. Accepted Greenfield Store representation

The implemented Realm Store representation contains credential truth, a second
aggregate revision, and an independent credential-operation namespace. It
writes and recognizes exactly this opaque marker:

```text
ucupa.realm.local@57fd960065e71544aeef80c5979d71ef
```

This is one exact opaque identity, not `/n`, a schema number, semantic version,
UUID, hash, compatibility line, or parseable generation. Every non-current
marker is simply unrecognized and returns ordinary `ErrStore` without byte
modification.

There is no migration, compatibility reader, dual read/write, fallback,
repair, conversion, marker branch, predecessor input class, or retained test
fixture whose purpose is to support another representation. If preserving an
existing Store is required, implementation must stop and resolve Decision 12;
it may not add compatibility beneath this contract.

Realm continues to own exactly one bbolt Store and one long-lived exclusive
owner session. It adds no second database or dependency. The accepted exact
top-level buckets are:

```text
ucupa.realm.meta
ucupa.realm.local-principals
ucupa.realm.permission-bindings
ucupa.realm.binding-operations
ucupa.realm.credentials
ucupa.realm.credential-operations
```

The existing binding-operation bucket and its `OperationID` namespace remain
binding-administration-only. The credential-operation bucket tags exact Rotate
and Revoke records under `CredentialOperationID`. Equal underlying strings in
the two buckets are valid independent keys. There is no unified administration
operation bus or generic cross-module operation framework.

The Store contains only:

```text
exact recognition metadata and RealmID
immutable registered local Principal set
current exact PermissionBinding set
BindingSetRevision private scope and generation
CredentialID -> active {PrincipalID + verifier}
             | revoked {immutable PrincipalID}
CredentialSetRevision private scope and generation
exact binding-operation bindings and retained outcomes
exact credential-operation bindings and retained outcomes
```

No raw credential secret, evidence struct, password, token, signing key,
private key, session, clock, expiry, request log, audit event, or credential
history beyond permanent claims and convergence outcomes may be stored.

One credential transaction atomically commits all changed credential rows,
tombstones, the later `CredentialSetRevision`, and the operation binding and
outcome. An outcome-only transaction commits only the operation record. Binding
transactions preserve their existing bucket, namespace, binding, outcome, and
atomic semantic result. Neither aggregate revision advances for a change owned
by the other aggregate.

Open applies the completed binding-history validation only to binding records
and binding-operation records. It reconstructs the credential transition graph
only within credential records and credential-operation records. A record or
changed generation in one namespace cannot satisfy, conflict with, or repair
the other aggregate's history.

Open validates the exact marker, bucket set, Realm ID, registered subjects,
both revision tokens, every active verifier/tombstone, and every tagged
operation binding/outcome as one complete owner state. Unknown buckets,
malformed values, an active credential for an unregistered subject, a revoked
tombstone naming an unregistered subject, a noncanonical credential record,
impossible revision, a noncanonical operation key or record, an unexpected
evidence or secret field, or inconsistent current facts fail closed with
`ErrStore` and no repair. Equal key bytes across the two operation buckets are
independent and must reopen successfully.

Credential operation validation reconstructs the exact state-transition graph
needed to prove the current Store, without turning it into an audit log:

- every changed credential generation from 2 through the current generation
  has exactly one successful Rotate or Revoke operation whose result is the
  expected revision plus one in the same scope;
- every retained credential operation names a registered local actor holding
  the immutable exact lifecycle permission for its recorded kind; actor-proof
  bytes are intentionally unavailable and are not reconstructed;
- each ID has exactly one activation: generation 1 bootstrap or one successful
  replacement edge; and at most one later deactivation as a rotation source or
  revoke target;
- activation precedes deactivation, so a tombstone cannot rotate, reactivate,
  or become a later replacement;
- both ends of every successful rotation edge have the same immutable
  `PrincipalID`, old and new IDs differ, and its replacement verifier is exactly
  32 bytes; every 32-byte HMAC result, including the theoretically possible
  all-zero result, is valid because active/tombstone state has its own explicit
  tag;
- every retained Rotate operation names a permanently claimed old ID whose
  immutable Principal equals the recorded actor; only a successful rotation
  also binds the replacement ID to that Principal;
- current active/tombstone state equals the graph result; an active replacement
  verifier equals its retained operation verifier;
- at each changed generation a rotation source or revoke target was active
  immediately before the change, and a rotation replacement was still
  unclaimed;
- a revoke no-op target was already revoked at its expected generation;
- Rotate has no retained not-found outcome; retained Revoke not-found is valid
  only when its target was still unclaimed at the expected generation,
  including an ID first claimed later; and
- retained credential conflict is valid only when the replacement was already
  claimed and the old ID was active at its expected generation.

Historical revision conflict cannot be re-evaluated to an exact wall-clock
instant and is validated only by its canonical foreign/stale revision relation,
as in the current binding history. These transition records prove recovery and
retry convergence, not actor-proof history, time, causality, or tamper-evident
security evidence.

The current `CreateDurable`, strict read-only-then-read-write `OpenDurable`,
single-writer lock, no-create reopen, publication-after-commit, retirement, and
process-exit behavior remain applicable. Core and Facet Stores, markers, data,
dependencies, and operation namespaces remain byte-for-byte outside this
change.

## 9. Observable acceptance gate

Acceptance froze the following black-box requirements before implementation.

### 9.1 Evidence and exact establishment

- independent verifier golden vectors freeze the domain separator, big-endian
  audience length, exact audience bytes, credential ID bytes, and
  HMAC-SHA-256 result;
- the public Realm surface contains no generation helper or random-source
  error; the contract places cryptographically secure random generation and
  nonzero ID/secret conformance on the evidence producer;
- submitted evidence is checked only for exact shape, audience, and claim
  conflict; tests and documentation never imply that Realm can infer its
  entropy, generator provenance, or secret reuse;
- valid active evidence returns the exact registered local `PrincipalRef`;
- wrong audience, unknown ID, revoked ID, and wrong secret expose exactly zero
  reference plus `ErrUnauthenticated`;
- malformed zero values return `ErrInvalid`, while a closed or retired owner
  returns `ErrStore`; and
- raw secret bytes are absent from Store bytes, operation records, returned
  errors, and explicit diagnostic output.

### 9.2 Seed and authority

- Seed accepts zero or multiple credentials only for registered local
  Principals at the exact Seed Realm audience;
- remote/unregistered subjects, duplicate IDs, invalid evidence, and foreign
  audiences reject the complete Seed without partial Store creation;
- exact rotate-own and revoke authorities receive only their seeded behavior,
  while an unbound Principal and zero-authority Realm are denied;
- rotate-own can replace only the credential establishing that exact new call;
  it cannot reset or install evidence for another credential or Principal;
- all four Realm administration permissions remain immutable bootstrap-only
  rows at `RealmScope(current RealmID)`; and
- online binding Put/Delete cannot target any of those permissions or any
  Realm scope.

### 9.3 Rotation, revocation, and permanent claim

- rotation atomically tombstones the old ID, activates the fresh replacement
  for the same Principal and audience, and advances the revision once in
  memory, after durable reopen, and after process exit;
- no old, tombstoned, active, or otherwise claimed ID can be a replacement;
- unknown, revoked, or wrong-secret actor proof returns unretained
  unauthenticated; a valid proof plus a different old ID returns unretained
  denied; Rotate never retains not-found;
- a claimed replacement returns retained credential conflict without partial
  change;
- revoke-active advances once, revoke-revoked is retained no-op, and
  revoke-never-claimed is retained not-found;
- self-rotation survives lost response using old proof before commit or
  replacement proof after commit; and
- proof-self-revocation under a new `CredentialOperationID` is invalid and
  consumes no ID, while an exact retained result or conflict is resolved before
  that current-proof check.

### 9.4 CAS, replay, and concurrency

- binding and credential revisions are independently owner-scoped and do not
  conflict across aggregates;
- CAS precedes target inspection for every new credential operation;
- authentication and exact lifecycle authorization precede retained replay;
- retained lookup precedes Rotate proof-ownership and Revoke self-safety
  validation, so replacement proof can replay a committed self-rotation and a
  historical Revoke target becoming the actor's current proof cannot change its
  retained result;
- changed actor Principal, kind, target ID, replacement ID/verifier, or expected
  token under one `CredentialOperationID` returns operation conflict;
- Rotate and Revoke reject cross-kind reuse inside the credential namespace;
  the same underlying string in binding `OperationID` and
  `CredentialOperationID` remains independent and both operations replay;
- exact changed success, revoke no-op, Revoke not-found, credential conflict,
  and revision conflict replay after later state changes and reopen;
- two competing changed operations with distinct `CredentialOperationID`
  values whose caller proofs remain active produce one commit and one retained
  revision conflict, exposing no partial credential state; and
- two competing self-rotations using the same old proof produce one commit and
  one unretained unauthenticated result with no second operation record.

### 9.5 Failure, recognition, and non-effects

- definite pre-commit failure changes no fact or outcome, lost committed
  response is resolved by reopen and replay, and indeterminate result retires
  the complete Realm;
- successful revocation makes every later authentication of that proof fail,
  while a previously established reference or already-linearized operation is
  not retroactively canceled;
- nil/value-copy/concurrent Close behavior preserves one shared owner session;
- missing, empty, malformed, foreign-owner, non-current-marker, second-writer,
  structurally inconsistent, or noncanonical-operation Stores fail without
  creation, overwrite, repair, or compatibility behavior;
- malformed changed-operation revision relations, missing generations, a
  repeated/late activation, repeated/early deactivation, successful rotation
  edge with equal IDs or cross-Principal endpoints, malformed verifier length,
  graph/current-state disagreement, or invalid no-op/not-found/conflict timing
  fails recognition;
- every active record and revoked tombstone names one currently registered
  local Principal; an unknown or remote tombstone subject fails recognition;
- historical Revoke not-found followed only by a later claim and canonical
  historical revision conflict remain valid and replayable rather than being
  re-proved from the final state;
- binding and credential histories are validated independently; equal key bytes
  in their operation buckets remain valid and both exact operations reopen and
  replay without cross-aggregate conflict or repair;
- an already revoked actor proof is unauthenticated and cannot create Rotate
  history, while revoking an existing tombstone remains the distinct no-op;
- the new exact marker and independent golden Store fixture prevent writer and
  reader drift from accepting each other accidentally;
- existing exact `PermissionBinding`, `Authorize`, remote-subject binding,
  binding `OperationID` namespace and records, Core, Facet, Composition, and
  accepted regression behavior remain unchanged; and
- static checks find no clock, expiry, token, session, signature, KMS, RPC,
  TLS, role, issuer federation, Realm hierarchy, Decision 15 mechanism, second
  Store, new dependency, migration, or compatibility path, and make no claim of
  service/endpoint audience isolation within one Realm.

Completion satisfies focused Realm authentication, lifecycle, concurrency,
Store atomicity, independent-process, and golden-format tests plus the existing
complete commands:

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

## 10. Decision effects and preserved seams

This accepted contract answers Decision 8 as follows:

- credential representation: exact Realm audience, opaque 128-bit ID, and
  caller-held 256-bit bearer secret;
- token representation: none;
- validation: online against current Realm-owned verifier/tombstone state;
- key custody: the evidence holder owns the bearer secret, Realm stores only
  the exact HMAC-derived verifier, and there is no signing key;
- rotation: only the proof currently establishing a call may atomically
  tombstone itself and activate a fresh ID for the same local Principal; and
- revocation: permanent ID tombstone observed by every later authentication.

This completed slice answered only the predecessor map's decision-time question
8. It did not answer questions 2 through 7 or 9 through 15.

In particular:

- this contract did not decide clock, expiry, skew, lease, or grace semantics;
- this Greenfield contract defines one target representation and did not decide
  upgrade or formal Store-compatibility policy; and
- it did not decide identity lifecycle, management-authority
  transfer/attenuation, or cross-Realm administration.

The exact Realm audience is not a Realm hierarchy. A mount does not create a
child Realm, inherit credentials, or contact a parent Realm for authentication.
Each Realm validates only its own credentials and establishes only its own
registered local Principals. A future multi-Realm contract may compose explicit
relations but cannot infer trust, delegation, or parentage from this evidence.

The audience names the logical Realm, not a Store instance or writer epoch. It
does not fence two copied writable authorities that improperly claim the same
Realm ID. Restore identity, duplicate-authority prevention, and writer fencing
remain outside Decision 8 and must be solved at their existing owner boundaries;
no extra Store UUID is added to credential evidence as a substitute.

## 11. Explicit exclusions and stop rule

This contract does not add or decide:

- Principal creation, deletion, rename, reuse, profile, user account,
  organization, or identity recovery;
- remote-Principal authentication, cross-Realm credential acceptance,
  administration delegation, attenuation, Realm parentage, or trust graph;
- password authentication, password hashing, enrollment ceremony, secret
  delivery, escrow, recovery code, hardware key, certificate, OIDC, OAuth,
  workload identity, or external identity provider;
- third-party credential reset, recovery, issuance, enrollment, or a Realm
  evidence-generation helper;
- signed or encrypted token, session, cookie, refresh, proof-of-possession,
  challenge, nonce, request signature, channel binding, or replay cache;
- clock, expiry, skew, grace period, scheduled rotation, rate limit, lockout,
  abuse detection, security audit trail, or observability pipeline;
- process boundary, server/client, RPC, TLS, transport, Console, Operator,
  Archive, deployment, backup/restore, HA, Partitioning, or federation;
- mutable manager bindings, batch credential mutation, listing, introspection,
  history, pagination, event, outbox, notification, or cross-Store transaction;
- a new database, dependency, shared repository, generic credential adapter,
  public Store codec, schema version, migration, compatibility path, or second
  implementation; or
- any answer to production-capability-map decisions other than Decision 8.

Retained operation outcomes prove retry convergence only. They are not an
authentication log or immutable security audit record.

If implementation requires a password, signed token, offline validation,
external issuer, time, process boundary, transport protection, remote Realm,
third-party reset, recovery, issuance, compatibility reader, or second Store,
work must stop at this contract boundary. That concern requires its own accepted
decision rather than an optional field or hidden adapter in this slice.

## 12. Acceptance, start, and completion boundary

This document is accepted and implemented. Proposal, Acceptance, the user's
separate explicit Start, and Completion remain distinct decisions.

Completion records exactly the bounded result frozen above:

1. Realm now derives and stores only exact credential verifiers, establishes
   exact registered local `PrincipalRef` values online, and exposes no issuer,
   generator, token, session, clock, or recovery mechanism;
2. current-proof self-rotation and separate-authority revocation use their own
   aggregate revision and retained operation namespace without changing
   binding semantics or revision state;
3. the one Realm Store recognizes only the exact current marker and six-bucket
   representation, atomically persists credential facts and outcomes, strictly
   reconstructs their transition graph, and contains no compatibility path;
4. in-memory, durable reopen, process-exit, definite/indeterminate failure,
   concurrency, independent golden, corruption-rejection, and public
   Realm-to-Core/Facet acceptance gates pass; and
5. at this contract's completion boundary, Decision 8 was **ACCEPTED** and
   decisions 2 through 7 and 9 through 15 were **OPEN**; those labels are
   historical provenance, and completion authorized no later mechanism.

The slice is closed. The PermissionBinding-administration contract remains the
authority for exact binding mutation; this contract is the current authority
for Realm-owned online caller establishment, credential lifecycle, and the
current Realm Store representation.
