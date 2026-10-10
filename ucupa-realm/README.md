# UCUPA Realm

## Code navigation

The module root is documentation only. Follow the shared
[engineering rules](../ENGINEERING.md) for directory and dependency constraints.

```text
ucupa-realm/
  api/realm.proto
  bindings/go/protobuf/     generated protocol bindings
  contract/                public values, errors and opaque revision aliases
  instance/                construction, Store opening and identity assembly
  internal/
    policy/                one authorization owner, commit and Store replay
    identity/              bounded identity issuer and verifier
    token/                 private revision representation, no fact state
  sdk/go/                  pinned client and response checks
  transport/grpc/
    config.go, server.go   TLS host assembly and lifecycle
    internal/
      admission/           service admission, challenges and finite handles
      entry/               actual typed RPC implementations against narrow ports
      wire/                protocol conversion and representation checks
  tests/instance/          public construction and original-Store reopen
  README.md
```

Go consumers construct through `ucupa/ucupa-realm/instance` and refer to public
values through `ucupa/ucupa-realm/contract`. `instance.Realm` is the actual policy
handle, not a forwarding facade or a second owner. The old root import no longer
exists. Private algorithm and Store tests live with the implementation they verify;
the host tests retain direct-entry and real TLS assembly coverage. The SDK is still
small and cohesive; it does not gain an empty `internal` directory for symmetry.

This engineering move preserves Store recognition, bytes, replay, public wire
messages and authorization semantics. S2P2 and S2P3 implementation and finite
validation records are in the [development plan](../docs/architecture/development-plan.md);
document authority classification is separate from code completion.

## Responsibilities

Realm owns caller establishment and one exact authorization relation:

```text
exact Realm-audience credential evidence + current active verifier
-> exact registered local PrincipalRef or rejection

established PrincipalRef + PermissionID + OperationScope -> Allow/Deny
```

Core scopes are exact `CoreElementScope(FactAuthorityRef, ElementID)` values:
equal Element UUIDs in different Core authorities are different permission
targets. Realm compares the complete scope and does not resolve the authority
or infer ownership. The accepted P4 increment directly replaces unqualified
Facet scopes with `FacetElementScope(FacetAuthorityRef, ElementID)` and
`FacetKeyScope(FacetAuthorityRef, ElementID, FacetTypeRef)`. Namespace and key
scopes are distinct exact targets, not an inheritance or wildcard mechanism.
FacetTypeRef is an opaque exact 32-byte value, preserved by the binding codec;
Realm neither retrieves a type artifact nor validates payload content.

P5 adds the exact `AnchorClosureScope(CoreRef, AnchorID)` discriminator. Realm still
compares whole scopes; only Core proves membership at the operation cut. There is
no inherited Realm, implicit wildcard grant, or scope-kind conversion.

P7.1 adds exact `AuthorityScope(FactAuthorityRef)` for Core/Facet writer control.
It grants neither Element, closure nor Facet content access, and no existing CRUD
scope is converted into it. Its binding codec has a distinct discriminator and
uses the new private marker below; caller, credential, binding and view rules are
unchanged. [The development plan](../docs/architecture/development-plan.md) records
implementation and full verification separately from user acceptance. This does
not introduce a separate writer authority in Realm.

`TrustLocalCaller` is an explicit trusted assembly step; `EstablishCaller` uses
current credential proof. Their sealed run-local caller contexts cannot be
manufactured by a JSON Principal field. `AuthorizeView` / `ValidateView` bind only
the required grants and their change epochs: revoke/regrant invalidates old views,
unrelated edits and no-op/replayed grants do not. Reopening retires prior contexts.
See the [P5 contract](../docs/architecture/public-access-and-observation.md) for
the original view semantics; the implemented Stage 2 contact and network adapter
reuse these semantics rather than replacing the permission model.

The completed
[Realm-owned PermissionBinding-administration contract](../docs/architecture/realm-owned-permission-binding-administration.md)
defines exact binding mutation. Its completed
[online caller-authentication successor](../docs/architecture/realm-owned-online-caller-authentication.md)
defines credential establishment, lifecycle, and credential persistence.
The Core-qualified scope arrived in Phase 1; P4 replaces the remaining Facet
scope codec and Store marker without changing credential or binding semantics.
The local P4 scope increment remains the basis of the later Stage 2 contact and
remote-access implementation.
Neither mechanism enlarges Realm into a user, organization,
resource, or policy-language owner.

`Seed` installs immutable registered local Principals, exact permission
bindings, and zero or more credentials. A credential permanently claims one
opaque 128-bit ID for one local Principal. Caller-held evidence contains the
exact Realm audience, ID, and 256-bit bearer secret; Realm retains only the
specified HMAC-SHA-256 verifier while active, then only the immutable Principal
association after revocation. `Authenticate` returns the exact local
`PrincipalRef`; unknown, revoked, wrong-secret, and foreign-audience evidence
all return the same `ErrUnauthenticated`.
The evidence producer must generate both ID and secret with a cryptographically
secure random source and retry all-zero output; Realm intentionally exposes no
generation helper.

Four exact `RealmScope(current RealmID)` permissions are Seed-only and immutable
online: Put/Delete binding, Rotate-own credential, and Revoke credential.
Binding Put/Delete use one owner-scoped `BindingSetRevision` and retained
`OperationID` namespace. Credential Rotate/Revoke use a separate
`CredentialSetRevision` and `CredentialOperationID` namespace. Rotation may
replace only the credential establishing that call and preserves its Principal;
revocation may target another credential but not the proof establishing the
call. Credential IDs remain permanently claimed, retained outcomes replay
exactly, and the two revisions never invalidate one another.

`New` constructs the in-memory profile. `CreateDurable` and `OpenDurable`
construct one exclusive long-lived bbolt owner using the exact current private
FormatID:

```text
ucupa.realm.local@a33aa9c69815480491bca89ce0c6d653
```

The Store recognizes exactly six owner-private buckets for metadata, local
Principals, permission bindings, binding operations, credentials, and
credential operations. It atomically commits each aggregate mutation before
publication, strictly reconstructs binding and credential histories on open,
stores no raw bearer secret, and recognizes no predecessor format or
compatibility input. `Close` retires a durable owner and releases its lock; a
retired owner fails authentication and administration closed with `ErrStore`.
In-memory `Close` is a no-op. `Authenticate` and `Authorize` read only published
in-memory state and perform no Store I/O.

`ForkCaller` verifies an existing credential-backed, run-local caller and derives
an isolated establishment retaining its Principal and CredentialID. This supports
the target transport's restricted read delegation without exposing credentials or
serializing CallerContext. It cannot elevate `TrustLocalCaller` into network
evidence, bypass current grants, or survive credential revocation / Realm restart.
It adds no Store record, grant model, remote endpoint, or token/session manager to Realm.

P6 additionally provides `IdentityIssuer` and `IdentityVerifier`: explicit Ed25519
issuer trust, finite signed identity proofs, and run-local liveness checked against
the original credential. Proofs bind issuer/subject, actor, opaque target challenge,
issue/expiry times and a nonce; they contain no permissions, secrets or caller proof.
The verifier never produces a trusted caller by itself. Composition verifies both
the signature and authenticated issuer liveness before target-local establishment;
the target still checks its own bindings. Issuer/key configuration does not modify
the Store. Core transport owns challenges, network sessions and restricted grants.
See the [network contract](../docs/architecture/network-access-contract.md#82-显式身份签发信任已交付).

S2P3a adds the module's network service boundary beside the unchanged kernel.
`transport/grpc` serves the three typed services (target check, identity,
authorization) over pinned gRPC/TLS with SPKI-to-owner service admission, and
`sdk/go` is the pinned typed client; protocol bindings are generated by the fixed
`cmd/protogen` flow. The SDK sends user credentials and protected authorization
bodies only after `CheckTarget` has verified the actual Realm identity on the
current connection — a missing, failed or pre-reconnect verification blocks
them. The check registers the immutable local identity of the TLS transport
that actually answered it; an old reply cannot verify a replacement transport.
Protected calls prevent a replacement handshake from becoming ready while
their send/response is in flight, and release that guard on every exit path.
`cmd/ucupa-realm` opens one existing durable Realm Store as
an independent product process (see its [README](../cmd/ucupa-realm/README.md)).
The network layer adapts the original credential, authorization and view
mechanisms without copying grants, credentials, binding epochs or signing keys:
caller/view/denial handles are unguessable run-local references in one bounded
registry, revalidated online on every operation and never persisted; a Realm
restart invalidates every prior-run handle. Service admission is separate from
user permission. Foreign identity keeps its P6 signed-proof shape: without the
product's `foreign` section `EstablishForeignCaller` fails closed as
unavailable, and an explicitly configured trusted issuer (pinned signing key
plus online liveness over the existing P6 proof check) assembles the full
chain. The request's explicit `actor` is the P6 signed relay identity already
authenticated by the receiving Core, not that Core's own service owner. The
signature must match this actor; mTLS still determines the receiving owner,
which owns the challenge and returned handle. Missing or substituted actor
is rejected, with no legacy-message fallback or self-reported principal path.

Realm still does not implement Principal lifecycle, credential generation or
enrollment, recovery, third-party reset, general-purpose session services,
KMS integration, mutable manager authority, roles, inheritance,
wildcard or path scope, cross-Realm administration,
delegation permission policy, audit evidence, backup, HA, or
production federation. The kernel package depends only on the Go standard
library, public value contracts, private revision tokens, and the existing bbolt
module; the network peripheral packages add only the declared gRPC/protobuf
dependencies. No Realm production package imports Core or Facet; one existing
durability test uses their public constructors to verify a real consumer chain.
