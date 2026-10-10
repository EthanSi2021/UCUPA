# UCUPA Platform — non-normative overview

**Status: current non-normative projection of the accepted axioms and concept baseline.
This document has no specification or implementation authority and does not authorize code.**

This document presents UCUPA through its purpose, truth categories, and module organization.
Normative semantics come only from the accepted
[foundational axioms](docs/architecture/ucupa-foundational-axioms.zh-CN.md) and
[concept and mainline baseline](docs/architecture/ucupa-concept-mainline-baseline.zh-CN.md).
Read the current contracts in the [authority manifest](docs/architecture/README.md) for delivered
bounded capabilities, and the [snapshot status](docs/architecture/development-plan.md#1-当前在哪里)
for implementation and validation limits.
If this overview conflicts with those specifications, this overview is wrong. Resolve omissions
or ambiguity at the normative source rather than inventing semantics here.

## 1. Purpose

**Spatial infrastructure for the united cyber universe**

UCUPA stands for **The system of United Cyber Universe Position and Anchor**.
It organizes a world as a tree of local coordinate frames, designed for growth in spatial extent
and local detail. Independent operators store and maintain their own parts of that tree, using
common identity and spatial rules rather than one central world database. (A1, A10; D1–D4, D6–D9)

This positioning does not reduce the project to bridges between separate spaces or promise
verified infinite capacity or precision. “Engineering prototype” describes current maturity,
not positioning; read current contracts and delivery records for supported and verified scope.

The minimum public spatial semantics center on Element:

- `ElementUUID` is stable, non-reusable identity, not a path, position, permission, owner,
  or endpoint. (A2; D4)
- Core represents spatial existence and organization as
  `Structure + parent-relative Pose + Range`. (A3, A4; D1–D4)
- Facet is optional opaque content. A Core-only Element is complete; Facet neither copies
  Core truth nor owns Element existence. TypeRef does not require Facet to validate content
  formats. (A6; D5, D6)
- Authority, access control, reachability, and deployment are orthogonal; none implies
  another. (A7, A10; D6–D8, D12)
- Applications may consume and extend these facts, but must not create competing UCUPA
  world truth. (A10)

“Logically unbounded” means finite spaces and autonomous authorities can continue joining
without changing public semantics. It does not promise infinite precision, unlimited single-owner
capacity, permanent availability, or traversal of the entire space in one request.
(A1, A9, A10; D3, D10)

## 2. Five public behaviors

These product-neutral categories do not freeze Go APIs, wire spelling, processes, or deployment.

1. **Enter**: obtain a verifiable boundary and explicit entry from `SpaceReference`, combine
   caller trust evidence, verify the connection target, and establish a temporary
   `SpaceHandle`. No prior reference-to-address directory lookup is required. (A8; D7–D9)
2. **Discover**: find unknown Elements within an explicit structural closure through
   `StructuralAddress`, direct structure, or `QueryRegion`. Valid queries return evidenced
   `COMPLETE` or `PARTIAL`; if no valid query can be formed, return an explicit
   `INVALID`, `DENIED`, or `UNAVAILABLE` outcome before results. (A8, A9; D9, D10)
3. **Read**: obtain an exact record, absence, or failure from the established Core/Facet
   authority, with Revision, ReadCut, and source evidence. (A6, A9; D4–D6, D11)
4. **Observe**: continue from an owner read cut through committed changes. Interruption,
   expiry, and source-graph changes must be recoverable or explicitly report GAP. (A9; D11)
5. **Propose**: submit identity, preconditions, and stable OperationID to the relevant fact
   owner. New facts exist only after that owner accepts and commits. (A7; D4–D8, D12)

`Resolve`, `ListDirect`, and `QueryRegion` are different Discover forms.
Create, update, reanchor, delete, and Facet mutation are owner-specific Propose forms.
The public common interface does not require a generic CRUD API. (D4, D5, D10)

## 3. Platform view

```text
Applications and external resources
Applications / robotics / other consumers
                    │
      Enter / Discover / Read / Observe / Propose
                    │
══════════ UCUPA public spatial interface ══════════
 Core: Element existence / Structure / Pose / Range
       + per-Element FacetAttachment
 Facet: optional exact opaque content keyed by Element + TypeRef
───────────────────────────────────────────────────
 fact authority / access-control / reachability / deployment
───────────────────────────────────────────────────
 Replaceable Store / index / transport / gateway / deployment
```

### 3.1 Core spatial view

Core exclusively owns Element spatial semantics. Exactly one Core-kind `FactAuthorityRef`
owns each valid fact cut. An ordinary record includes stable `ElementUUID`, closed `Kind`,
current Structure, parent-relative LocalPose, Range, and owner-local Revision.
A root has no LocalPose. (A2–A4, A7; D1–D4, D6)

Range conservatively covers the structural closure rooted at that Element for discovery.
An Entity Range covers its occurrence; an ordinary Anchor Range covers its zone.
A bounded parent must directly contain each child's entire projected Range.
An independent space's sole parentless Anchor uses typed `UNBOUNDED_ROOT`. (D2)

Coordinate truth is right-handed Cartesian `SE(3)`, with uniform units and no scale.
Polar or geodetic representations are boundary projections only. Numerical uncertainty must
favor no missed intersections, not create false negatives. (D1, D3)

### 3.2 Facet content view

Facet is optional specialized content. An Element with only Core spatial facts is complete.
When content needs an independent owner, permissions, Revision, or lifecycle, exact
`(ElementUUID, FacetTypeRef)` identifies Facet truth: payload and owner-local Revision.
`FacetTypeRef` is an opaque fixed 32-byte key; no resolvable type artifact or central schema
service is required. Facet stores and returns bounded payload bytes unchanged, without parsing
or schema validation. Applications agree on and validate format and business meaning.
The relevant owner still checks permissions, capacity, acceptance, and Revision preconditions.
(A6; D5)

Core existence is a prerequisite for Facet visibility and mutation. Facet cannot resurrect
a deleted Element, and Core/Facet do not pretend to share a cross-owner atomic transaction.
(A6, A10; D5, D6, D11)

Core also owns and returns the Element's Facet relationship: `NONE`,
`ACTIVE(exact FacetAuthorityRef)` with matching proposal/acceptance, `DETACHED` without an
inferred deletion, and `RETIRED` after explicit P7 disposal. Retirement is not “never attached”
and cannot be hidden or automatically revived; terminal rules are in the
[lifecycle contract](docs/architecture/owner-transition-contract.md).
This relation is separate from the Element spatial record. Its pending/ACTIVE contact records
hold the expected Facet identity and specified `FacetNextHop`. Facet retains a maintainable
`CoreNextHop` per exact AcceptanceKey, without overwriting immutable acceptance or creating
a current-Core pointer.

Core existence alone does not make Facet content serviceable. A Core exact read forms
`FacetAccessContext` from its ACTIVE attachment; Facet selects the exact relation contact,
fresh-reads the source Core, and verifies matching acceptance. Only a matching `ACTIVE` cut
locates the full namespace. Get/type enumeration also rechecks the captured Facet cut and contact.
(D5, D6, D11)

Core mounts and Facet attachments share the contact principle:
expected `FactAuthorityRef` plus owner-specified `NextHop`. Each local owner maintains its
own direction. Contact-only changes advance that contact token and owner cut, not Element/Facet
Revision, relationship identity, lifecycle, or writer fence. Deployment supplies service entries;
the relation contact selects an entry. Not every reference contains an address, and an authority
need not have one global address. There is no mandatory Ref-to-address directory. Claim target
authentication, network transport, and multi-address support separately under their contracts
and evidence. TypeRef does not address services; Facet has no built-in type-contract parser.
(A5–A8; D5, D6, D8)

### 3.3 Entry, structural addressing, and discovery

`SpaceReference` is shareable bootstrap information. `SpaceHandle` is a temporary context
assembled for one caller's entry. Neither is a new world object, permanent locator, or
transferable permission. (A8; D9)

`StructuralAddress` is an upward/downward structural path from a known Anchor in an entered
space, derived from the selected cut's Structure. It respects explicit boundaries; reparenting
may change the current address. Bare `ElementUUID` establishes identity only, not global
lookup across an open system. (A2, A8; D9)

Discover selects Scope using Start and upward depth, then searches that Scope and all descendants
without a downward depth limit. Upward depth -1 continues to a proven root or explicit entry
boundary. It does not enlarge permission or equate disconnection with reaching a root.
UpDepth selects semantic scope; execution limits such as forwarding hops, fan-out, deadline,
and page size must not silently shrink it or treat unchecked space as empty.

Each Core can configure SnapshotRetention at startup: omission disables it, an explicit valid
positive duration enables it, and invalid input fails configuration. There is no default duration.
Queries, pagination, and same-cut exact reads remain available when retention is off:
CurrentVerified rechecks that the original cut remains current and explicitly expires on change.
Retained holds the old view for a finite period; ordinary writes do not invalidate its pages.

A cross-Core chain uses CurrentVerified if any actual source, including an evidence provider,
has retention disabled; it uses Retained only if all enable it. The first page fixes and exposes
ReadMode. Resource shortage does not silently switch modes. Disabled retention is not
unreachability, expiry, or missing content. Ordinary writes remain durable, ordinary reads default
to current state, and old chains do not switch to latest. Sources and relations remain subject
to validation; this is not a network-wide consistency promise. (A9; D9, D10)

### 3.4 Realm authorization contacts

Core/Facet access to Realm follows the same contact principle: `RealmAttachment` names an
existing `RealmID` and specified `NextHop`. The receiver verifies the target and performs
identity/permission checks through that controlled contact, without a reference directory.
This is an authorization access chain; it copies no Realm PermissionBinding and changes
no fact ownership.

In the delivered S2 implementation, each Core/Facet stores one contact in its own Store,
not per Element or payload. Supported profiles are controlled local assembly, a network contact
to an independent Realm service, and specified-address maintenance within the same Realm.
Network waits occur outside fact locks; final read/write boundaries recheck request,
authorization evidence, and contact generation. Unreachability never becomes offline Allow.
Realms remain autonomous without a required global Realm or replicas. See the
[RealmAttachment contract](docs/architecture/realm-attachment-contract.md).
These capabilities do not establish verified real multi-host Realm operation, production scale,
or HA. (D7, D8)

## 4. Minimum-truth matrix

| Category | Projected content | Sole owner/source | Explicitly not |
|---|---|---|---|
| Core world truth | Element identity, Kind, existence, Structure, LocalPose, Range, ElementRevision, permanent non-reuse of deleted UUIDs | Relevant Core-kind `FactAuthorityRef` (A2–A4; D1–D4, D6) | WorldPose, path, index, Route, Endpoint, Cluster |
| Facet world truth | Optional exact `FacetKey`, payload, FacetRevision | Relevant Facet-kind `FactAuthorityRef` (A6; D5, D6) | Element existence, FacetAttachment, copied Core space, type-service endpoint |
| Fact-authority control | `FactAuthorityRef`, Core delegation/acceptance, per-Element FacetAttachment proposal/acceptance, writer fence, transition state | Explicit owner of each control fact (A7, A10; D6, D12) | Realm Allow, actual reachability, deployment location |
| Identity/permission control | Caller evidence, owner-local `PrincipalRef`, PermissionBinding, closure-bounded authorization view | Access-control/trust owner; packaged as Realm here (A6, A7; D7) | Core/Facet ownership, fact existence, Route |
| Contact control | Expected target identity and specified NextHop per exact cross-service relation; FactAuthorityRef for fact services, RealmID for RealmAttachment; revisions and persistence defined by each contract | Owner storing that direction's contact (A5, A7; D6–D8) | Ref-to-address directory, one global address, ownership switch, permission, writer activation |
| Deployment control | Cluster, process, Store, placement, runtime topology | Deployment owner, outside public spatial truth (A7; D8, D12) | Element identity, FactAuthorityRef, spatial structure |
| Derived values | WorldPose, StructuralAddress, SubtreeEnvelope, indexes, query results, derived events, cursors, caches, address-resolution results, actual connection/reachability state | Reconstructed from authoritative facts, contact control, and current runtime context (A5, A9; D8–D11) | Second truth, global Revision, second writer, arbitrarily discardable contact-control history |
| Deferred mechanisms | API/wire spelling, Store schema, index algorithm, partitioning, credentials/TLS, lease clock, deployment, product Facet contracts | Selected through later explicit contracts (D1, D3, D5, D7–D12) | Current spatial axioms or implementation choices this overview can make |

Every fact and control state requires a unique verifiable owner boundary. Multi-owner reads
retain per-owner cuts; no implicit authority combines them into a global Revision, globally
atomic snapshot, or cross-owner transaction. (A7, A9, A10; D6, D10, D11)

## 5. Authority, federation, and transitions

- Parent Core authority retains the mount Anchor record. The accepted carrier Core owns
  contents coverage below that boundary, without copying the mount Anchor. (D6)
- Delegation, acceptance, access permission, specified contact, and deployment are distinct.
  None automatically grants another. (A7, A10; D6–D8)
- Realm establishes identity and decides permission, not Core/Facet truth. Cluster represents
  hosting infrastructure, not a fact owner. (A7; D6–D8)
- Contact information says how this relation reaches the expected authority. Verify target
  and relation after connecting to the specified entry. Contact failure means unavailable
  or partial results, never fact deletion or authority transfer. (A5, A7; D8, D10)
- An owner transition accepted through D12 continuity evidence preserves ElementUUID.
  Persistently fence the old writer before enabling the new one. Explicit unavailable windows
  are allowed; shadow objects, dual write, and simultaneous visibility are not.
  (A2, A7, A10; D12)
- Reanchor changes Structure/Pose, not ElementUUID. Cross-Core transition does not automatically
  migrate or reuse Facet attachment acceptance. (D4, D6, D12)

## 6. Current product organization

This table describes repository packaging, not a public protocol, axiom, mandatory future module,
or permanent module count. Other implementations may split packages, processes, and deployment
differently while preserving normative owners, control boundaries, and public behavior.
(A1, A7, A10; D4–D12)

| Module | Current packaging role | Does not imply |
|---|---|---|
| `ucupa-core` | Core spatial facts | A fixed process, Store, or deployment shape |
| `ucupa-facet` | Exact opaque Facet content | Element existence, schema validation, or copied Core space |
| `ucupa-realm` | Reference access-control/trust implementation | Spatial autonomy or Core/Facet ownership |
| `ucupa-operator` | Candidate deployment-intent and operations packaging | Frozen APIs, state, or an independent owner in the baseline |
| `ucupa-archive` | Candidate offline semantic exchange | Online authority, Store backup, or an online request path |

UCUPA has no Console product or built-in visual panel. Human-facing browsing, administration,
and visualization belong to external clients using public owner operations and authorization.
UCUPA retains programmable operations, observation, and diagnostics; it needs no UI to run or
be used, and Operator does not inherit the old Console packaging. (A10; mainline baseline §9)

SDKs, hosts, servers, transports, Store adapters, indexes, and conformance tools are not new
product truth owners. Address-based DNS/transport adapters do not constitute a UCUPA
Ref-to-address directory. Co-located or separate processes do not change these semantic
boundaries. (A1, A5, A7, A10)

## 7. Greenfield exclusions

- No global World, bare-UUID locator, central Element/owner/Cluster directory, global Revision,
  global transaction, or central spatial index. (A1, A7, A10; D6, D9–D11)
- Do not merge the Anchor tree, authority graph, Realm trust/permission, Route, and Cluster
  topology into one tree. (A7, A10; D6–D9)
- Do not promote WorldPose, structural addresses, indexes, query results, events, cursors,
  or caches into competing truth. (A5, A9; D9–D11)
- No scale, Collider, asset bounds, LOD, rendering, high-frequency physics, or product
  business semantics in Core. (A3, A4, A10; D1–D4)
- Metadata and displayName are outside the current baseline. Only a real consumer and
  independent contract can justify them as optional capabilities; neither may become a
  prerequisite for identity, structure, spatial queries, addressing, routing, or authorization.
  (A2, A4, A8; D4, D9)
- Text names, aliases, and name resolution are not prerequisites for correct identity,
  addressing, authorization, or discovery. (A2, A8; D9)
- Exact UUID Grants do not replace authorization for unknown discovery. UpDepth explicitly
  selects Scope; execution hops or timeouts must not silently change it. (A8, A9; D7, D10)
- Do not conceal transitions with mirrored truth, shadow Elements, dual write, unreachability
  fencing, or cross-owner atomic promises. (A7, A10; D12)
- No version suffixes, old Store readers, converters, migration branches, or compatibility
  facades for removed prototypes. (Axioms §4; mainline baseline §8)
- Databases, message buses, Kubernetes, present module counts, and implementation mechanisms
  are not spatial truth. (A1, A5, A10; D8, D11, D12)

## 8. Authority rules for this overview

1. Axioms constrain all downstream design; D1–D12 are the sole detailed source of this
   overview's concrete semantics.
2. This is a lossy reading projection. It accepts, changes, completes, or rejects no UCUPA rule.
3. On conflict, the normative source prevails and this overview must be corrected. Do not
   reinterpret the source to preserve wording here.
4. Unsourced statements here cannot justify API, Store, wire, deployment, or implementation decisions.
5. Every later implementation contract must identify the normative clauses it covers and
   public counterexample gates. Appearing here does not authorize code.
6. Accepting or updating this overview does not automatically authorize implementation.
