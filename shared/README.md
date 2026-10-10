# Shared values

`shared/values` contains the smallest value vocabulary genuinely shared by two
or more UCUPA boundaries. It is a library, not a seventh product module and not
a place for domain records, Stores, services, clients, routing, or generic
framework code.

This README describes the current minimum-truth-corrected vocabulary. The
completed Core minimum-model correction
removed textual path and Address values without adding a replacement lineage,
resolver, or compatibility surface. The completed
[Realm minimum-model correction](../docs/architecture/realm-minimum-model-correction.md)
replaced the historical split permission and scope shapes without aliases or a
compatibility surface.
The completed
Realm local-durability contract
is the immutable-bootstrap persistence predecessor. The completed
[Realm-owned PermissionBinding-administration
contract](../docs/architecture/realm-owned-permission-binding-administration.md)
adds only exact `RealmScope` construction and typed `RealmTarget` projection for
same-Realm bootstrap-manager authority; it adds no public scope tag, byte codec,
path, wildcard, hierarchy, or generic introspection surface.
The completed
[Realm-owned online caller-authentication contract](../docs/architecture/realm-owned-online-caller-authentication.md)
keeps credential IDs, secrets, evidence, revisions, and operation IDs Realm-
owned; it adds no shared audience or authentication abstraction.

It depends only on the Go standard library. Core, Facet, and Realm may depend
on it; it depends on none of them.

The current vocabulary includes exact Principal, Element, Facet, Cluster, and
permission identities; exact `FacetKey`; opaque exact `OperationScope` values
constructed by `CoreElementScope`, `FacetElementScope`, `FacetKeyScope`, and `RealmScope`; and
`Requirement{Permission, Scope}`. `CoreElementTarget`, `FacetElementTarget`, `FacetKeyTarget`, and
`RealmTarget` return only the exact valid typed target for their own scope kind
and otherwise return the zero value plus false. Core receives stable Element
UUIDs directly and derives placement and ancestry from its own facts. Shared
values contain no Element path, `Audience`, shared action type,
`ExactScope`, routing, policy evaluation, ownership decision, or generic scope
codec.

Stage 1 · Phase 1 adds a distinct UUIDv4 `FactAuthorityRef`, generated once for
each logical Core, and the exact `local:<name>` `NextHop` address value.
`CoreElementScope` binds the target Core identity and Element UUID together;
there is no ownerless scope fallback. P4 now qualifies both Facet namespace and exact-key scopes by the receiving owner.
`NextHop` identifies a designated local service entry, not a structural path or
a Ref resolver. This paragraph describes the implementation increment only;
the broader contract and plan alignment is reviewed separately.

P6 extends `NextHop` with canonical `https://host:port` for native gRPC over TLS.
`NetworkTarget()` validates the network form; `IsLocal()` restricts real in-process
adapters to `local:` so an HTTPS address cannot be resolved by a local connection table.
`shared/api/network.proto` owns the common network representation and finite-session
messages; `shared/bindings/go/protobuf` contains only their generated bindings.
This generated dependency exception does not change the standard-library-only
rule for `shared/values`, nor expose CallerContext or authorization views on the wire.

The public `AttachmentKey` message is shared by Core read results and Facet
access requests. Its proposal is a public owner-bound token, not the local
`AttachmentRevision` bytes or a service control proof. There is one generated
definition, with no owner-specific alias or alternate protocol path.


## P4 local boundary values

P7.3a defines `FacetTypeRef` as an opaque 32-byte content-key component, with defensive copies and exact raw ordering. Facet does not resolve it to an artifact or validate the payload format.
`FacetAcceptanceKey` binds Core, Facet, Element and proposal revision, without addresses.
`AttachmentRevision`, `FacetReadCut` and `CoreCutEvidence` provide the smallest copied tokens
needed by both owner boundaries. Core projects its actual cut through `CoreReadCut.Evidence()`.
These local encodings do not define a public wire protocol, identity authentication or a
global cut; construction validates shape, while trusted owner ports establish provenance.

## P5 public boundary values

CallerContext and related AuthorizationView belong to trusted local ports, not JSON identity assertions.
AnchorClosureScope remains a separate exact scope kind. PublicToken uses owner/kind and opaque 32-byte value;
the [shared schema](api/values.schema.json) does not expose owner Store token encodings.
`ParsePublicJSON`, `ParseCanonicalJSON`, `CanonicalJSONValue` and immutable `PublicJSONLimits()`
implement bounded strict JSON and the declared JCS representation; they do not decide owner permissions or facts.
