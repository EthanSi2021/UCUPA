# UCUPA concept and mainline baseline

**Status: accepted current detailed target semantics; governed by the foundational axioms; not an implementation contract or development plan; does not authorize code**

> Contact amendment: the user confirmed one principle for Core and Facet. Each relationship
> requiring cross-service access retains its expected `FactAuthorityRef` and an
> authority-specified `NextHop`, connects to that address, and verifies the target; there is
> no Ref-to-address directory. This replaces the former common Ref Route prerequisite. It
> does not add an address to every Ref or give each authority one globally unique address.
> The authority manifest distinguishes confirmed targets from contract details awaiting
> review. Code still requires explicit start authorization. This amendment aligns documents;
> it neither authorizes code nor claims that network authentication has been implemented.

> Realm contact supplement: the same specified-contact principle uses `RealmAttachment`
> for a Core/Facet's contact with its authorization manager, targeting the existing
> `RealmID` and specified `NextHop`. This completes the access chain of the established
> authorization model; it does not redefine Realm's role or introduce a new model. The common
> direction is confirmed; record placement, interfaces, and recovery still require successor
> implementation contracts.

> P3 query amendment: the user confirmed the final target contract for selecting Scope by
> upward depth, fully searching the selected descendants, and optional snapshots. Each Core
> may configure SnapshotRetention at startup. There is no default retention duration;
> omission disables retention and a valid positive duration enables it. Reads select
> CurrentVerified or Retained according to their actual source; writes remain acknowledged
> after reliable commit. The finalized query contract refines D9's local upward path and
> D10's profiles without repeated item-by-item confirmation. The development plan alone
> records implementation authorization, delivery, and acceptance.

> Planning responsibilities: §6.2 and §9 distinguish capability-proof dependencies from
> execution order. The development plan alone maintains the confirmed Stage/Phase sequence.
> Reordering that plan does not change D1–D12. The query amendment above is a separately
> declared semantic change; it neither labels unimplemented capabilities complete nor
> accepts all candidate implementation details merely because a plan was recorded.

This document derives UCUPA's complete concept mainline from the accepted
[foundational axioms](ucupa-foundational-axioms.md) and resolves twelve interdependent classes
of downstream questions together. It is not twelve independently valid local designs, a
Go API, wire format, Store design, or deployment plan.

Acceptance freezes target semantics without treating concept decisions as current APIs,
Stores, or code:

- The [foundational axioms](ucupa-foundational-axioms.md) govern this sole detailed target.
  Platform, minimum-truth, and capability views may only project it.
- The [authority manifest](README.md) alone maintains current implementation contracts;
  the [development plan](development-plan.md) alone maintains the current phase and next action.
- Range/spatial-root derivations, the capability reassessment ledger, historical capability
  map, and open-infrastructure feasibility assessment are frozen evidence, not current-status
  sources.
- Type names fix conceptual roles, not language, serialization, or process boundaries.
  Every implementation still needs an independent contract and explicit start authorization.

## 1. Main conclusion

> **UCUPA's public mainline is this: a caller enters an explicit Anchor structural closure
> through a verifiable, non-centralized spatial entry; resolves structure or discovers unknown
> Elements under bounded authorization; reads Core's `ElementUUID + Structure + Pose + Range`
> and optional Facet exact typed records; and observes or proposes changes using evidence
> from each fact authority. Federation connects these independent authorities without
> merging them.**

```text
Shareable SpaceReference
        │  separately verify spatial entry / authority / trust / specified contact
        ▼
Caller's temporary SpaceHandle
        │
        ├─ Resolve / ListDirect / QueryRegion ──► Core spatial facts
        ├─ Read / ListFacetTypes              ──► Facet typed facts
        ├─ Observe / Resume                   ──► derived observation of committed facts
        └─ Propose                            ──► corresponding fact authority

Core Element truth = ElementUUID + Kind + Structure + LocalPose + Range
                     + ElementRevision
Core control truth = delegation / per-Element FacetAttachment / relation contacts
Facet truth        = (ElementUUID, FacetTypeRef) -> exact payload + FacetRevision

Independent relationships: Structure ≠ Fact authority ≠ Access control ≠ Reachability ≠ Deployment
```

There is no global World object, bare-UUID directory, global Revision, central spatial index,
Realm tree, or cross-owner atomic transaction. The system grows by adding finite spaces,
authorities, and entries, not by making one central node bear “infinity.”

## 2. Why the decisions must be resolved together

The twelve decisions form a directed dependency graph, not twelve isolated votes:

1. Spherical Range has cross-implementation meaning only with common rigid-body coordinate
   algebra and deterministic numeric rules.
2. Core/Facet truth determines fact coverage first; fact authority, access, and reachability
   then determine trustworthy entry.
3. SpaceHandle, query universe, authorization for unknown discovery, and the meaning of
   `COMPLETE` must agree.

Review may follow dependency order, but the final design must pass a whole-system consistency
gate. Local correctness must not depend on hidden truth when combined:

```text
Accepted foundational axioms
        │
        ├─ D1 Coordinate algebra ─┐
        └─ D2 Range/Root ─────────┴─► D3 Numeric safety ─► D4 Core spatial truth ─► D5 Facet typed truth
                                                                 │                     │
                                                                 └──────────┬──────────┘
                                                                            ▼
                                                 D6 Fact authority / delegation / acceptance
                                                                 │                     │
                                                                 ▼                     ▼
                                                         D7 Access/trust      D8 Contact/reachability
                                                                 └──────────┬──────────┘
                                                                            ▼
                                                 D9 SpaceReference / address / handle
                                                                            ▼
                                                 D10 Resolve / discovery / query
                                                                            ▼
                                                 D11 evidence / observe / resume
                                                                            ▼
                                                 D12 transition / fencing
                                                                            ▼
                                                        One coherent concept successor
```

Cluster, Store, Transport, indexes, Operator, Archive, and module counts are not public spatial
semantics. Successor production contracts may implement or consume these results. Human-facing
visualization is an external use, not a requirement for a built-in Console.

## 3. Minimum vocabulary and identity boundaries

| Concept | Sole meaning | Explicitly not |
|---|---|---|
| `ElementUUID` | Stable, non-reusable Anchor/Entity identity | Path, position, owner, permission, endpoint |
| `FacetTypeRef` | Stable opaque 32-byte type identifier used in Facet content keys | Service address, mutable name, proof that Facet validated content format |
| `FactAuthorityRef` | Stable logical authority identity for a Core or Facet fact slice | Realm, Principal, Cluster, Store, Pod, NextHop |
| mount Anchor | Anchor still owned by its parent Core authority, whose direct content coverage is accepted by another Core authority | New Element kind, remote root, replication boundary |
| carrier role | Core-kind FactAuthorityRef responsible for content coverage of an exact mount acceptance | Mount Anchor owner, Cluster, service address |
| `PrincipalRef` | Caller identity established by an explicit trust context | Fact ownership, write fence, contact information |
| `RealmID` | Identity of the owner of identity and permission relationships | Space, Anchor root, organizational parent, Cluster |
| `ClusterID` | Deployment/hosting environment identity | Element or fact-authority identity |
| `BoundaryAnchorRef` | Exact verified pair `(AnchorUUID, owning Core-kind FactAuthorityRef)` after entry | SpaceReference publisher, NextHop, credential, read cut, bare-UUID locator, permanent path |
| `StructuralAddress` | Upward/downward structural path from a known Origin through selected Structure within an explicit boundary | Permanent identity, human name, authorization proof |
| `SpaceReference` | Shareable spatial bootstrap description | Current permission, guaranteed current reachability, persistent shared truth |
| `SpaceHandle` | Temporary context composed for a caller's particular entry | New space object, global locator, transferable credential |
| `FacetAccessContext` | Temporary AcceptanceKey from an ACTIVE attachment in one Core exact read, selecting an exact Facet acceptance and its Core contact | Persistent FacetKey, existence evidence, global UUID locator, address resolver |
| `NextHop` | Target-service entry specified by an authorized party in a cross-service relationship, used directly by transport | Target identity, permission, delegation, globally unique address, guaranteed current reachability |
| Contact record | Expected target identity, NextHop, and independent contact revision bound to an exact relationship; facts use FactAuthorityRef, Realm uses existing RealmID | Ref-to-address directory, authority-wide unique address, Element spatial record, Facet payload |
| `RealmAttachment` | Contact with the expected Realm after Core/Facet selects its authorization context; contains existing RealmID and specified NextHop | PermissionBinding copy, caller-claimed authorization basis, new fact authority, mandatory spatial field on each Element |

Four independent needs establish the stable owner role `FactAuthorityRef`: one Realm Principal
may control multiple fact slices; fact authority remains identifiable across Cluster, endpoint,
and credential changes; queries and observation need distinct read cuts; and Core/Facet have
independent owners. It replaces the narrower candidate name `SpatialAuthorityID`, without
creating a global authority directory or requiring a new globally resolvable ID. Any stable
opaque reference satisfying these invariants may carry the role.

Refs support identity comparison and evidence binding only. Only cross-service relationships
need contact records. One authority may use different entries in different relationships;
these must not become a mandatory complete authority-address table. `FacetTypeRef` is only a
Facet content key. A consumer needing Payload interpretation or validation holds its own
rules; it must not assign that work to the fact service or create a fact-Ref address directory.

This document adds no `WorldID`, `ConnectedSpaceID`, or `SpaceRoot` object. Current Anchor
Structure, valid delegation/acceptance, and the selected semantic boundary define a traversable
space; its root Anchor already supplies the necessary spatial identity anchor.

## 4. Twelve integrated decisions

### D1. Core has one authoritative coordinate algebra

1. Core Pose is a right-handed Cartesian rigid-body transform from an Element's local frame
   to its direct parent Anchor frame: three-dimensional `SE(3)`. The first interoperability
   successor uniquely determines canonical rotation/scalar encoding; it is not the algebra.
2. Linear dimensions use metres. Pose contains no scale, shear, projection parameters, or
   coordinate-system labels.
3. A rooted space's root Anchor has no parent frame and therefore no `LocalPose`; its
   derived pose in its own root basis is identity.
4. WorldPose, projections to any ancestor frame, and inverse transforms derive from current
   Structure and LocalPose, not competing persisted truth.
5. Polar, cylindrical, spherical, geodetic, astronomical-scale partitioned coordinates, and
   rendering coordinates are boundary projections/profiles only. A projection declares
   source, target, error, and applicability; representation preferences do not enter Core records.
6. Independent root bases have no implicit transform. Without a projection relationship
   held by an explicit owner with revision/evidence, they are not one geometric Query universe.
   Displaying two query results together does not establish shared coordinates.

This keeps spherical Range spherical under any Anchor rotation and gives Pose composition,
reanchor, and intersection a closed mathematical model. Native scale or non-rigid Core frames
would require reopening D1, D2, D3, and D10 together, not adding a bypass field.

### D2. Range conservatively covers the structural closure for discovery

Authoritative Range has exactly two forms:

```text
Range = BOUNDED_SPHERE(radius) | UNBOUNDED_ROOT
```

1. Ordinary `Range(E)` is a finite nonnegative sphere centered at the origin of `E`'s local frame.
2. Entity is a leaf occurrence. Its Range covers its domain-neutral spatial existence, not
   a Collider, LOD, visibility distance, permission area, or business influence zone.
3. Anchor Range is stable conservative coverage of its complete Structure closure. False
   positives are permitted; false negatives are forbidden.
4. Only a parentless root Anchor uses `UNBOUNDED_ROOT`. This removes a fixed root-radius
   ceiling, not precision/capacity limits or the need for bounded queries.
5. Every parented direct child Element uses `BOUNDED_SPHERE`. A direct parent
   Anchor with `BOUNDED_SPHERE` requires:

   ```text
   distance(parent.origin, child.origin) + child.radius <= parent.radius
   ```

   With an `UNBOUNDED_ROOT` parent, any valid finite child Range is contained by type;
   do not read a nonexistent `parent.radius` or compare finite spheres. Checking these
   two direct parent–child cases inductively covers all descendants. Tangency is valid;
   orientation does not change sphere radius.
6. Only explicit caller commits change Structure, Pose, and Range. UCUPA does not automatically
   change ancestors or children to make a request valid. Child creation, changes, and deletion
   do not automatically enlarge/shrink ancestor Range or advance ancestor Revision.
7. Ordinary non-root Anchor and Entity Range may stay unchanged, grow, or shrink. The final
   sphere must fit inside its direct parent; an Anchor's final sphere must also contain every
   direct child. Validate all constraints in one committed state. Failure rejects atomically,
   with no intermediate Parent, Pose, or Range.
8. Anchor Range is declared conservative coverage, not necessarily a minimum enclosing sphere.
   Shrinking is an ordinary explicit update, not a separate zone-governance behavior.
   Read-only coordinate conversion, containment validation, and intersection do not change
   stored truth. Future cross-owner coverage proof and commit coordination require successor
   contracts; local atomic validation makes no cross-owner atomic promise.
9. `SubtreeEnvelope`, Octree, BVH, children indexes, and query caches are rebuildable
   optimizations. Extra candidates are permitted. Missing, dirty, stale, or uncertain coverage
   must fall back to stable Range or report incompleteness, never justify a complete miss.

Sibling Anchors may overlap. Mandatory non-overlap would impose partitioning rules on general
spatial organization. Budgets and `PARTIAL` results handle pathological overlap.

### D3. Numeric uncertainty must resolve toward safety

1. Public authoritative position, orientation, and radius are finite, canonical, interoperable
   values. NaN, infinity, and numeric sentinels do not encode semantic states.
   `UNBOUNDED_ROOT` is a typed variant; negative radius is always invalid.
2. Mathematically equivalent rotations/encodings cannot be different truth. After acceptance,
   an owner stores and returns only the public canonical representation specified by the
   first interoperability successor. Caller input encoding is not a second source of truth.
3. Coordinates, radius, Pose composition, and projections have explicit profile limits.
   An unrepresentable or unprovable value yields a stable failure or `UNAVAILABLE`, not
   overflow, truncation, or silently reduced precision.
4. Authoritative containment writes must not accept mathematically out-of-bounds tuples due
   to rounding. If containment cannot be proved, reject the write.
5. Query pruning must not exclude potentially intersecting Elements due to rounding.
   If non-intersection cannot be proved, retain the candidate.
6. Deep-chain projection uses deterministic exact predicates, intervals/error envelopes,
   safety-directed rounding, or traversal fallback when proof is insufficient. An empirical
   epsilon cannot decide an authoritative write or complete miss.
7. Independent owners need no shared global numeric execution. Each result carries authoritative
   values and projection evidence; cross-owner composition obeys the same conservative rules.
   The first interoperability successor chooses only one canonical scalar/rotation encoding,
   decision algorithm, and conformance corpus, not historical profiles or multiple authoritative
   representations in parallel.

D3 freezes observable safety semantics, not math libraries, SIMD, index layout, or wire bytes.

### D4. Minimum Core truth and the local operation loop

Target Core records are:

```text
Anchor {
  ElementUUID
  ParentAnchorUUID?  # absent only for root
  LocalPose?         # present if and only if a parent exists
  Range              # root-only UNBOUNDED_ROOT; otherwise BOUNDED_SPHERE
  ElementRevision
}

Entity {
  ElementUUID
  ContainingAnchorUUID
  LocalPose
  Range              # BOUNDED_SPHERE
  ElementRevision
}
```

Required invariants:

1. Anchor and Entity are closed kinds. UUID is unchanged through creation, Pose/Range
   updates, reparent, reanchor, route/deployment changes, and proven owner transition,
   and is never reused after deletion.
2. A root has no parent, LocalPose, or bounded Range. Ordinary mutations cannot reparent
   or delete it. Retiring a whole space is separately approved governance, not Element CRUD.
3. Changes that jointly determine validity are atomically validated as the final tuple:

   ```text
   final spatial tuple = parent/containing Anchor + LocalPose + Range
   ```

   Convenience create/update/reanchor/reparent APIs must not require a temporarily invalid
   intermediate state.
4. Reanchor/reparent within one authority is one semantic commit. Moving an ancestor does
   not rewrite descendant records or Revisions; WorldPose is derived again.
5. Ordinary Anchor deletion does not cascade. Its owner must prove empty direct content
   and no `ACTIVE`/`DETACHED` delegation coverage in the same commit domain. Mount
   Anchors are not ordinarily deleted: a future explicit mount-retirement operation must
   first fence admission/writer, then use matching delegation/acceptance and a final carrier
   cut to prove empty direct coverage or explicit completed transition of all content.
   Unreachable carriers, revoked acceptance, and detachment never prove emptiness.
   Element deletion ends Core existence. Facet immediately hides according to the three Core
   existence states (`PRESENT`, confirmed `NOT_FOUND`, `UNAVAILABLE`), with independent
   physical cleanup. `DENIED` is orthogonal authorization before fact-state disclosure,
   not a fourth existence state.
6. An Element Revision identifies only the authoritative cut of its current spatial record,
   not time, ancestry, Facet attachment, or cross-owner order. Real spatial-tuple changes
   advance Element Revision and Core read cut; Facet attachment has the independent revision
   in D6.3. Every mutation uses owner-local OperationID, explicit CAS preconditions, and
   convergent retained results.
7. Core exposes at least these owner results; names and DTOs are decided separately:
   - Exact read of a known Element in its current owner context.
   - Paginated spatial records of all direct child Anchors/Entities at a stable read cut.
     This does not disclose FacetAttachment; the corresponding Element exact read must do so.
   - Read/verify the current parent chain and project to a specified Anchor frame.
   - Atomic creation, final-tuple update, reanchor/reparent, and non-cascading deletion.
   - Owner, record Revision, read cut, and three-state existence evidence.
8. `ListDirect` is a necessary Structure read, not an optional administrative convenience.
   Without it consumers must build a competing shadow directory. Parent listing permission
   does not implicitly grant every direct child's Facet-entry read permission.
9. Current Core defines no Metadata, dedicated name/displayName/text-path fields. Spatial
   records store no WorldPose, ancestry, indexes, Endpoint, Cluster, permissions, or business
   relationships. Mount and Facet attachment contacts may authoritatively store NextHop
   under D6/D8, but it is not a spatial field. Metadata and displayName remain future
   candidates, with no reserved fields, APIs, Store, or Revision semantics.

D4 defines owner-local semantics only. D6, D9, D10, and D12 compose cross-authority resolution,
queries, and migration without contaminating local records.

### D5. FacetTypeRef is an opaque content key, not a name or service address

Facet is an optional specialized extension surface. An Element with only Core spatial facts
is already complete and valid. Establish a Facet contact when an independent owner is needed
for content, permissions, Revision, or lifecycle. Its minimum openly interoperable truth is:

```text
FacetKey(ElementUUID, FacetTypeRef) -> ExactPayload + Revision
```

1. At most one record for a `FacetKey` exists at a valid cut. There is no generic `slot`,
   `subjectKind`, or free-form metadata map.
2. `FacetTypeRef` is an arbitrary fixed 32-byte opaque key. Element plus TypeRef selects
   one exact content record, requiring no resolvable type artifact, content address,
   signature, or central schema service.
3. Facet stores and returns bounded Payload as original bytes. Put does not parse it or
   validate schemas/business types. The relevant owners still validate TypeRef representation,
   permission, capacity, acceptance relationship, and Revision preconditions.
4. Higher-level agreements about meaning, format, version, or validation are managed and
   checked there. A stored Facet record is not proof of conformance. Reinterpreting the same
   identifier is that higher-level system's responsibility; UCUPA provides no implicit migration
   or automatic reinterpretation.
5. Add no internal type-contract resolver, artifact, pin/mirror, or second `contractRef`.
   TypeRef is not a fact-authority address or route entry.
6. Facet owners provide exact Get/Put/Delete and complete enumeration of current
   `FacetTypeRef` values per Element. Enumeration enables open consumers to discover stable
   extensions; it is not optional debugging. Batch optimizes item-level reads; change reads
   belong to D11 Observe. Both require successor contracts freezing evidence, pagination,
   and failures before implementation, without expanding local minimum truth.
7. Every single-subject Get/enumerate/Put/Delete binds Core existence and `FacetAttachment`
   to that subject's same explicit Core read cut. Confirm both `PRESENT` and
   `ACTIVE(exact responding FacetAuthorityRef, matching proposal/acceptance evidence)`.
   The call carries the AcceptanceKey from Core exact read as `FacetAccessContext`.
   Facet selects the retained exact acceptance and `CoreNextHop`, directly contacts and
   verifies exact CoreAuthorityRef, then fresh-reads current Core without a Ref directory.
   Neither key nor contact is a persistent FacetKey, existence evidence, or stored current-Core
   pointer. `NONE` means no accepted namespace. `DETACHED`, authority mismatch, or failed
   verification yields only `UNAVAILABLE`/not-current-owner; the former authority cannot
   return or commit current payload. Only PRESENT/KEY_NOT_FOUND/complete-empty responses
   passing `ACTIVE(self) + matching acceptance` carry an explicit Facet cut together with
   Core-existence and attachment evidence. Core-proven NOT_FOUND/NONE/DETACHED/authority
   mismatch before namespace entry carries only Core evidence, not a fabricated Facet cut.
   Get/enumerate captures candidate results, acceptance, and contact token at Facet cut F0,
   checks Core outside the lock, then verifies F0 and contact still hold. A change restarts
   or explicitly fails; do not splice Facet facts committed after the Core current check
   into that result. Mutation acceptance also binds the same active attachment revision and
   verified contact. Core and Facet fabricate no cross-owner linearization point: fresh
   reads after concurrent detach/delete hide records bound only to an old Core cut, while
   in-flight results already linearized at that old cut carry their old evidence.
8. Reanchor, reparent, Pose/Range updates, and NextHop/Cluster changes do not change FacetKey
   or Facet Revision. Reads newly established after Element deletion's cut return no payload.
   In-flight responses linearized before deletion may finish with original evidence;
   deletion does not rewrite history. Physical orphan cleanup is independent, retryable
   Reconcile by the Facet owner.
9. External assets, accounts, physics, workflows, and real-time simulation remain with their
   own systems. Facet may hold opaque references/records, not absorb the resources themselves.

Public representation and interoperability rules may specify 32-byte TypeRef encoding without
giving Facet content-format interpretation or validation responsibilities.

### D6. FactAuthorityRef, delegation, and acceptance prove the sole owner

#### D6.1 Stable fact-authority identity

`FactAuthorityRef` identifies only the stable logical fact owner. Bound `AuthorityEvidence`
declares Core/Facet kind, current coverage, authority epoch, and available read-cut space;
these mutable attributes are not identity. Ref survives process, Pod, Cluster, endpoint,
credential, and same-authority writer-handoff changes. Actual owner/coverage replacement may
establish a new Ref, with continuity evidence under D12.

Every valid fact cut has exactly one active authority:

```text
Core fact slice  -> one Core-kind FactAuthorityRef
Facet fact slice -> one Facet-kind FactAuthorityRef
```

`FactAuthorityRef` is not itself a permission principal. Established PrincipalRefs may be
authorized to operate it, but Realm Allow, credentials, Cluster hosting, and contact information
do not make them fact owners.

#### D6.2 Core mount authority boundary

A mount requires two independent revision-bounded control facts:

```text
parent Core authority:
  DelegationBinding(mount ElementUUID, carrier Core-kind FactAuthorityRef, delegation revision)
  outgoing contact(exact relation, carrier Ref, carrier NextHop, contact token)

carrier Core authority:
  MountAcceptance(exact delegation, accepted admission context, acceptance revision)
  outgoing contact(exact relation, parent Ref, parent NextHop, contact token)
```

1. The parent Core alone owns the mount Anchor's Element record, Pose, Range, Revision,
   Facet attachment, and delegation.
2. The active carrier Core alone owns direct Entities, direct child Anchors, and descendants
   not delegated again within that mount. It does not copy or claim the mount Anchor.
3. Each delegation revision has at most one active carrier coverage. Multi-carrier partitioning
   of one mount is outside the baseline; discovered routes do not establish the owner set.
4. Carrier writes use independent `MountAdmissionContext`: mount UUID, Range cut,
   delegation/acceptance evidence, and admission epoch. Ancestor WorldPose is not local containment.
5. Cross-space projection separately uses `CoordinateProjectionContext`: mount UUID,
   WorldPose, and read evidence covering the entire ancestor Structure/Pose chain. Ancestor
   movement invalidates projection context, not automatically still-valid local admission.
6. `MountAcceptance` makes mount UUID the carrier's external adjacency root. The carrier
   can completely list/query its owned direct content in that context, without owning,
   copying, or modifying the mount Anchor record.
7. Nested mounts may exist. A Structure/coverage ancestry must be acyclic with unique owners
   per cut, but the authority-node graph need not be globally acyclic. A delegating to B in
   one branch and B delegating to A in another may be valid. Route-loop detection cannot
   replace cycle prevention for an individual fact ancestry.
8. Each contact holder maintains its direction's NextHop with local authorization and CAS.
   Address changes advance only that contact token and owner-local cut, not ElementRevision,
   delegation identity, acceptance contents, or admission epoch. A new address for the same
   target cannot establish a mount, replace a carrier, or bypass write-gate coordination.

#### D6.3 Facet attachment is Core's authoritative per-Element entry

Core reachability does not imply Facet reachability. To locate an Element's extension surface
uniquely without a global Facet directory or broadcast, every Element at a valid Core cut
has a `FacetAttachment` control fact retained and returned by its current Core authority:

```text
FacetAttachment lifecycle(ElementUUID)
  -> NONE(AttachmentRevision)
   | ACTIVE(AttachmentRevision,
             one Facet-kind FactAuthorityRef,
             exact accepted ProposalRevision)
   | DETACHED(AttachmentRevision,
               former Facet-kind FactAuthorityRef,
               former accepted ProposalRevision)

Core-side contact(exact pending / ACTIVE relationship)
  -> expected FacetAuthorityRef + FacetNextHop + contact token

Facet-side contact(exact AcceptanceKey)
  -> expected CoreAuthorityRef + CoreNextHop + contact token
```

This is per-Element control truth in Core Store, not the Element spatial record. Core's pending
proposal and ACTIVE contact retain target FacetAuthorityRef, authority-specified
`FacetNextHop`, and an independent contact token, not copied Facet owner cuts or payload.
AcceptanceKey remains `(FacetAuthorityRef, CoreAuthorityRef, ElementUUID, ProposalRevision)`,
without an address. Facet durably retains the corresponding immutable acceptance and a
maintainable `CoreNextHop` contact for D5's fresh source-Core check. That contact neither
overwrites immutable acceptance nor points to a current owner. D8 connects by the relationship's
specified address and separately verifies expected identity; successful connection does not
create ownership.

Core exact Element read may return record, attachment, and relationship contact at one Core cut.
An internal minimum-disclosure port may project only existence and attachment from that same cut,
but cannot maintain a second existence fact. The `ACTIVE` authority alone owns the complete
`FacetKey` namespace, providing complete TypeRef enumeration and authoritative exact-key
reads/changes. A `DETACHED` former AcceptanceKey and current Core cut preserve “previously ACTIVE,
content not proved deleted, current entry unavailable”; do not collapse this to `NONE`.
Retained old contact information cannot reactivate it. Successor continuity evidence is added
only through D12 transition.

Element creation initializes `NONE` in the same Core commit. AttachmentRevision identifies only
that Element's attachment-control lineage. Proposal, cancel, activate, and detach advance it and
Core read cut without fabricating ElementRevision changes. Pose/Range/Structure changes advance
ElementRevision and Core cut, not active FacetAuthorityRef or AttachmentRevision. Element deletion
ends current existence and attachment validity in one Core commit, without cross-Store removal
of Facet bytes.

Each contact holder independently authorizes, CAS-checks, and durably commits address-only changes:
Core maintains FacetNextHop, Facet maintains CoreNextHop per AcceptanceKey. These advance only the
contact token and that owner's cut, not AttachmentRevision, ElementRevision, FacetRevision,
ProposalRevision, AcceptanceKey, or immutable acceptance. Address changes do not implicitly
replace an owner, reaccept a namespace, or grant permission. Historical acceptance remains
checkable; current serviceability always depends on fresh evidence from the exact source Core.

This observable logical relationship does not freeze private Store layout. The baseline does
not repartition or redelegate an Element's Facet namespace; otherwise complete enumeration
would need another coverage proof. Core-confirmed `NONE` at an explicit cut means no currently
accepted UCUPA Facet namespace, not a demand to broadcast to all Facet services.
`DETACHED` returns unavailable evidence under D6.4.

Core cannot unilaterally write a Facet pointer to establish `NONE -> ACTIVE`. It first persists
a proposal bound to exact CoreAuthorityRef, ElementUUID, AttachmentRevision, and target
FacetAuthorityRef, plus FacetNextHop. The authorized first Accept caller explicitly supplies
`InitialCoreNextHop`. Facet contacts and verifies the expected Core's current exact proposal
at that address, then atomically stores immutable acceptance and initial CoreNextHop for that
AcceptanceKey in one local transaction, using the Facet cut as `AcceptedAt`. Replaying old
acceptance must not silently overwrite a maintained address. Core commits `ACTIVE` only while proposal
and acceptance still match; Facet serves under D5 only after observing that active attachment.

Core cancel and in-flight Accept are not cross-Store atomic exclusion. After verifying an earlier
proposal, Facet may commit a late historical acceptance. It cannot reactivate a canceled proposal
or serve the current namespace merely from acceptance. Core rechecks proposal at final activation;
Facet performs D5's fresh check on every service call. No redundant independent AcceptanceRevision
is added. This two-party control handshake copies no payload and introduces neither a central
Facet service nor an authority-wide fixed peer.

Changing `ACTIVE` FacetAuthorityRef is a D12 authority transition; same-Core reanchor does not
replace it. Different Elements of one Core may attach to different Facet authorities, and a
Facet may accept Elements from different Cores. This minimum entry solves attachment, sole
ownership, and completeness without copying Facet existence or payload into Core.

#### D6.4 Revocation is not deletion

When delegation, the current effect of immutable acceptance, or FacetAttachment is revoked,
the committed acceptance remains retained, undeleted, and unchanged:

- Subsequent local writes by the former carrier/Facet are no longer accepted through that
  relationship as connected-space facts. Revocation does not replace a durable writer fence.
- Held content is not considered deleted or automatically owned by the parent, Realm, or
  contact-record administrator.
- The relationship owner retains detached/retirement evidence so later source-set reads
  distinguish unresolved former coverage from coverage that never existed.
- Until successful handoff, restoration, or explicit retirement, reads of the branch/namespace
  through that entry return `UNAVAILABLE`/detached evidence. Core-confirmed `NONE` at an
  explicit cut instead means no accepted Facet namespace.
- Both `ACTIVE` and `DETACHED` delegation prevent ordinary mount Anchor deletion.
  Unreachable/revoked/detached does not prove empty carrier coverage. Explicit D4
  retirement/transition must finish before the Anchor can be disposed of.
- Acknowledged writes and UUIDs remain preserved; D12 defines recovery.

### D7. Access-control/trust owners manage identity and permissions; Realm is the current implementation role

1. The trust boundary first establishes credential/signature/session evidence as a currently
   trusted `PrincipalRef`; the access-control owner then computes the exact relationship:

   ```text
   PrincipalRef + PermissionID + OperationScope -> Allow / Deny
   ```

2. Realm currently implements the access-control/trust owner role. Core/Facet define their
   own required PermissionIDs and Scope semantics, enforcing them against current resource
   facts at final read/mutation boundaries. Realm does not read or copy Core/Facet truth.
3. Baseline Scope supports at least:
   - `CoreElementScope(CoreAuthorityRef, ElementUUID)`.
   - `FacetElementScope(FacetAuthorityRef, ElementUUID)`.
   - `FacetKeyScope(FacetAuthorityRef, ElementUUID, FacetTypeRef)`.
   - Exact Realm administration target.
   - `AnchorClosureScope(BoundaryAnchorRef)`, permission over an explicit Anchor closure
     in an entered space.
4. Exact Core/Facet scope binds fact authority so real owner transition does not make old
   Grants automatically valid at the new owner. Ref limits a Grant's owner, not makes the
   access-control owner a fact owner. `AnchorClosureScope` is not a bare-UUID locator, text
   wildcard, or Realm tree. Access-control owners compare exact scopes only; Core verifies
   membership in the current closure at the relevant Structure read/commit cut. Caller-supplied
   ancestry is not authoritative.
5. `BoundaryAnchorRef` is the exact pair above and can be a durable Scope value. Same-authority
   writer handoff, NextHop/credential changes, and boundary reparent do not change it. Anchor
   deletion retires it forever; cross-Core transition creates a new Ref without automatic Grant
   migration. Closure scope is evaluated per operation cut: it includes new descendants then
   under the boundary, ceases to cover those moved out, and covers those moved in.
6. Scope crossing a mount does not transmit permission. Every contributing fact authority
   independently establishes the caller and authorizes local coverage. Delegation, acceptance,
   contacts, Cluster membership, and remote-issuer trust do not automatically grant reads/writes.
7. Unknown-result Discover requires closure-bounded discovery permission. Requiring an exact
   Grant for every still-unknown UUID is contradictory.
8. Request-level failure returns `DENIED` without asserting resource existence/absence,
   never fact-owner-confirmed `NOT_FOUND`. Internal traversal and result disclosure are
   separate: each authority may prune/forward as needed within its own coverage, without
   leaking denied node identities, counts, or topology. There is no globally privileged
   cross-owner coordinator.
9. Baseline discovery carries verifiable StructuralAddress/projection evidence. Disclosing an
   Element also requires disclosure of the Anchor chain needed to explain and continue
   addressing it. If any necessary node cannot be disclosed, the Element is outside that
   per-owner authorization view's disclosable result set; do not count it as allowed and
   silently omit it. Future opaque descendant handles require their own proven contract.
10. `COMPLETE` is relative to the caller-visible recursive source graph, disclosable per-owner
    authorization-view vector, and aggregate opaque closure commitment. Each access-control
    owner establishes its own local view. Only disclosable owners expose view stamps;
    undisclosable branches' local authorization/traversal results fold into an aggregate
    commitment leaking no owner, nodes, counts, or topology. Missing evidence discloses only
    permitted sources. A potentially unclosed branch yields partial/unavailable. Any view
    change invalidates old continuation; revoked authorization cannot be reused.
11. Access-control owners do not own role inheritance, organizational trees, resource directories,
    spatial roots, workflows, or a generic policy DSL. Higher-level products produce explicit
    PermissionBindings rather than changing the UCUPA permission kernel.

#### Realm authorization access-chain supplement

`RealmAttachment` completes only how a receiver contacts its selected authorization manager;
it does not change the exact principal/operation/target relationship. It uses
`RealmID + NextHop`: contact the specified address, verify the expected Realm, and check caller
and current permissions within the proven identity-establishment boundary. RealmID equality is
not remote authentication. Caller-supplied Realm/address cannot replace the receiver's controlled
authorization contact. After Allow, Core/Facet independently validate their own invariants.

Realm still owns authorization and credential-validation records; attachment does not copy them
into Core/Facet or make Realm an Element/Facet fact owner. The name does not require Facet's
proposal/acceptance state machine, fields on every Element/payload, or a new `RealmAuthorityRef`
or identity-address directory.

Successor contracts define durable placement, protected-fact context binding, same-owner
hosting recovery, address maintenance, and cross-owner transition checks. Implementation must
retain owner-qualified scopes and no automatic old-Grant migration. Deployment configuration
and contact changes must not silently reinterpret authorization. Missing contacts, wrong
targets, or incomplete required checks cannot allow access or disguise unreachability as
confirmed denial or absence.

Independent Realms manage their own identities/permissions without a system-wide Realm or
automatic synchronization. Completing this access chain requires no Realm replicas, consensus,
or HA. When this supplement was adopted, local assembly already supported Realm but
RealmAttachment was not yet implemented; future contacts were not prerequisites for that local
authorization capability. The development plan alone maintains implementation scheduling and
current status.

### D8. Core and Facet use relationship-specified contacts

Core mount/Facet attachment fact-service contacts explicitly retain:

```text
exact relationship contact {
  expected target FactAuthorityRef
  authoritative NextHop
  owner-local contact token / read cut
}
  -> call NextHop
  -> verify expected target + exact relationship + required caller authorization
```

Realm contacts use the same expected-identity/specified-address principle, with D7's
`RealmID`, not `FactAuthorityRef`. Do not merge these identities or let this example freeze
RealmAttachment Store or API design.

1. Expected Ref answers “who”; NextHop answers “how this relationship specifies contacting
   them.” Calls enter transport by NextHop, without a bare-Ref address-directory lookup or
   hidden resolver when no address exists. Ref contains no address and is no global locator.
2. Core mount and Facet attachment follow one principle. Parent mount contact targets carrier;
   carrier contact targets parent. Core attachment contact targets Facet; Facet's exact
   AcceptanceKey contact targets source Core. Each local contact owner maintains its direction
   at its own authorization/commit boundary. No authority-wide peer pair, globally unique
   authority address, or contact field on every Ref is required.
3. Deployment supplies service entries such as stable service names, gateways, or local
   transport entries. The relationship owner selects and durably commits the entry.
   Deployment may change processes/IPs behind it, not rewrite ownership for the relationship.
   DNS, IP routing, and address-based local port assembly are not UCUPA Ref directories.
4. Contact updates have independent tokens/owner-local cuts, authorization, CAS, OperationID,
   and convergent results. Address-only changes do not alter relationship generation,
   FactAuthorityRef, ElementRevision, FacetRevision, AttachmentRevision, immutable acceptance,
   or writer fence, and cannot activate pending/detached relations. Persistent specified
   contacts cannot be discarded as caches; connections, resolved IPs, and reachability caches
   can be rebuilt.
5. Senders carry expected target, exact relation/operation, and caller context from the
   confirmed relationship. Receivers independently check them; response provenance must
   match. Trusted local assembly proves only its local target binding. Ref string equality
   is not remote authentication. Successor contracts prove public network authentication,
   credentials, service capability, and freshness evidence.
6. Missing/stale addresses, disconnects, or target/security-binding mismatches yield
   `UNAVAILABLE`, or explicit partial aggregation under D10. They do not return `NOT_FOUND`
   or revoke delegation, permission, Elements, or FacetAttachment. Old addresses yield
   failure or verified in-flight results, not a new owner.
7. `ResolveRoute(FactAuthorityRef)`, RouteRegistration, a separate Ref directory, referral
   chains, and leases are not mandatory Core/Facet prerequisites; the old system is not
   merely postponed. Any network transport, update propagation, multiple addresses, balancing,
   or read replicas require separate proof that they consume specified contacts without
   controlling ownership, permission, or writer activation. This common contact principle
   does not claim those runtime capabilities implemented.
8. Structural path hops may have hop/fan-out/deadline limits. Exhaustion explicitly fails,
   continues, or produces partial results without changing fact topology. `FacetTypeRef`
   selects content records, not a service Ref-to-address resolution, and does not require
   Facet to resolve type contracts.

### D9. SpaceReference is shareable, SpaceHandle temporary, and structural addresses are relative UUID paths

#### D9.1 Semantic spatial boundary

Entry declares a `boundary Anchor`. Structural resolution and default spatial queries use
that Anchor's closure at specified Structure/read cuts as their semantic universe, not a
guessed common ancestor or a fixed N-level ascent from the current location. The boundary
may be the root of a rooted space or an explicitly published and authorized finite subspace Anchor.

A rooted Structure closure has exactly one root Anchor. Separate `UNBOUNDED_ROOT` instances
have no implicit common coordinates, Structure, or complete query. Multiple SpaceReferences
give a consumer results from multiple independent universes, not a fabricated global World.

#### D9.2 Two entry artifacts

```text
SpaceReference  # shareable, no caller secrets, stale hints permitted
  boundary Anchor reference
  initial Core FactAuthorityRef/evidence candidate
  trust bootstrap reference
  initial Core NextHop + source / freshness evidence

SpaceHandle     # temporarily established by the caller, not permanent truth
  resolved boundary
  established PrincipalRef + root authorization request/view context
  validated authority/delegation/acceptance contexts
  current reachability contexts
  independent freshness/read evidence
```

SpaceReference carries no transferable caller credentials and guarantees neither current
authority nor NextHop validity. Any organization may publish it. It carries or points to
bootstrap evidence, not creates, changes, or replaces authority bindings. Callers connect
through explicit entries and validate the live authority chain under their trust policy.
Missing addresses do not become bare-Ref lookups; no global UCUPA entry directory is needed.

SpaceHandle owns no shared Revision and preloads no undiscovered remote PrincipalRefs/views.
Authority, authorization, relationship-contact tokens, and connections are independently
validated. On reaching a new owner, obtain expected identity/NextHop from the relationship.
That owner's trust/access-control owner independently establishes local PrincipalRef/view
evidence from caller-submitted identity/trust evidence. Root PrincipalRef or issuer trust
does not transfer automatically. Each update rebuilds only the relevant context; a Handle
can be discarded and established again from SpaceReference.

#### D9.3 StructuralAddress

`StructuralAddress` is a structural path from a known Origin Anchor to a target within an
entered space. It may ascend parent edges, then descend an ElementUUID chain. Each edge is
verified by parentage/placement at selected read cuts and necessary mount evidence.
Nonterminal items must be Anchors; the terminal may be Anchor or Entity. Handle boundary may
be the default Origin; downward-only is a common case. Origin is not automatically a semantic
boundary: the explicit handle boundary still applies and cannot be crossed by ascent.

```text
{ OriginAnchor, upward parent edges, downward ElementUUID chain }
```

Consequences:

- UUIDs arrange the path; a UUID itself still identifies only identity.
- Reparent/reanchor changes the address, not ElementUUID.
- An old address may return owner-confirmed `NOT_FOUND` or optional short-lived stale/referral
  evidence; no permanent redirect is required.
- Resolution verifies each edge from known Origin; ascent requires real parent evidence.
  Both mount directions use their direction's specified NextHop and separately verify
  authority evidence, authorization, contact token, and response provenance.
- Address ascent count is not query UpDepth. Queries return a rereadable path from original
  entry to actual Scope; do not directly append Scope-relative matches to the original start.
- Bare ElementUUID without authority/space context is insufficient; do not broadcast the
  entire open system.

`AnchorUUIDPath` is only an explanatory name for StructuralAddress's Anchor prefix, not
another address truth or persistent type.

### D10. Structural discovery and spatial Query use one explicit universe

The baseline supplies three complementary discovery behaviors:

1. **Resolve:** verify each hop of a known current StructuralAddress and read the terminal Element.
2. **ListDirect:** paginate an Anchor's complete direct adjacency at one authority read cut.
3. **QueryRegion:** resolve explicit Scope from known Start and upward depth, then discover
   unknown Elements by spatial intersection throughout its full closure. Scope cannot
   exceed the declared SpaceHandle boundary.

Minimum spatial-request semantics:

```text
SpatialQuery {
  SpaceHandle
  Start + UpDepth      # -1 reaches the proven boundary; nonnegative limits ascent; no downward depth
  resolved Scope       # resolved at selected structural cuts, not a separate filter
  QueryRegion          # finite sphere in an explicit Anchor basis
  selector             # Anchor / Entity / both
  result basis
  execution budget
}
```

1. `QueryRegion` is temporary input, not Element Range. Its basis resolves through current
   StructuralAddress/projection evidence within the handle and must project into the semantic
   boundary. Input outside that context is `INVALID`; currently unobtainable evidence is
   `UNAVAILABLE`.
2. Selector determines result kinds, not Anchor traversal. An Entity-only query must still
   traverse every intersecting Anchor zone that may contain Entities.
3. Ordinary Element matching means closed-sphere intersection or tangency:

   ```text
   distance(query.center, element.origin) <= query.radius + element.radius
   ```

   An Element with origin outside the query sphere must still match if its Range intersects.
4. When selecting Anchors, their closure Range determines matches. `UNBOUNDED_ROOT` matches
   any finite QueryRegion. A product UI may omit root from its selector, not change this meaning.
5. Ascend verified parent edges from Start to explicit Scope, then search Scope itself and
   every descendant. There is no downward-depth setting; ancestor-direct Entities, sibling
   branches, and descendants cannot be omitted. Start and query basis are independent;
   a deep basis does not shrink Scope. Structure proves ascent; carriers do not guess parents
   or treat the receiving service as a spatial root.
6. Structural Scope and spatial region are independent constraints: UpDepth selects the former;
   the query sphere defines the latter. Confirmed UpDepth=-1 means unrestricted ascent to a
   proven spatial root or declared SpaceHandle boundary, not system-wide traversal, permission
   bypass, or crossing independent roots. Values below -1 are invalid; nonnegative values
   select by ascent count. Content outside selected Scope is not searched and does not itself
   cause PARTIAL; Complete applies only to Scope. Profiles specify zero/omission/root behavior.
   Unknown parent edges, refusal, or disconnect do not prove boundary reached. Query entry
   service and explicit SpaceHandle boundary are distinct.
7. **UpDepth is a semantic Scope selector, not an execution budget.** Forwarding hops, fan-out,
   deadline, page size, result limit, memory/cost quotas, and exact-proof work bounds are
   execution limits and cannot change the selected set. After context establishment, unfinished
   descendants require continuation/PARTIAL or a contract-defined failure, not assumed empty
   unvisited branches. Deployment boundaries do not add/reset structural depth.
8. Spatial indexes produce candidates and safe negative evidence only. Their result must equal
   an unpruned linear oracle at the same cuts. Any possible false negative invalidates that profile.
9. Core queries do not filter Facet payload, assets, Collider, physics, or product fields.
   Consumers discover Elements, then read Facet under D5. Cross-plane queries require separately
   proven behavior with both owners' evidence, not hidden Store coupling.
10. A verified match is not a bare UUID. It associates at least ElementUUID, kind, StructuralAddress
    relative to this boundary, source FactAuthorityRef/read cut, and intersection/projection
    evidence. Later `Read` obtains full Core/Facet payload; Discover creates no second record truth.

#### D10.1 Completeness is not a decorative boolean

`INVALID`, `DENIED`, and `UNAVAILABLE` are request/execution outcomes before a
`DiscoveryResult`: `INVALID` means no valid normalized query can form in the supplied context;
`DENIED` means request-level authorization was not established; `UNAVAILABLE` means semantic
boundary, query basis, or root fact-authority cut cannot currently be established, so no
semantically defined query can begin. None returns matches or masquerades as an empty result.
Only diagnostic evidence disclosable for that outcome may be returned, not an unestablished
source graph. In particular, `DENIED` cannot confirm targets, branches, or matches exist.

A valid query result carries at least:

```text
DiscoveryResult {
  verified matches
  status: COMPLETE | PARTIAL
  caller-evidence identity + authorization request identity
  normalized query identity {
    handle boundary + Start + UpDepth + resolved Scope / ancestor evidence
    QueryRegion + query basis
    selector + result basis
  }
  caller-visible authorization evidence {
    disclosable per-owner authorization-view vector {
      fact/access-control owner
        + locally established PrincipalRef/evidence + view stamp
    }
  }
  disclosable contributing Core FactAuthorityRefs + per-authority read cuts
  caller-visible recursive source-evidence graph {
    disclosable visited parent + complete ListDirect cut/pages
    disclosable safely pruned edge evidence
    disclosable mount delegation + matching acceptance cuts
  }
  aggregate privacy-preserving opaque closure commitment
    # binds undisclosable internal authorization/traversal results without leaking owner, nodes, count, topology
  disclosable projection/range coverage evidence
  authorized missing or dirty branches/sources
  relation contact revisions + independent connection evidence for disclosable sources
  continuation?
  opaque observation/resume state?  # only when ongoing observation is requested
}
```

Planners/gateways may use a fuller internal traversal graph within each contributing owner's
local authorization boundary, but must not serialize it directly to callers. Public
`DiscoveryResult` contains only views' disclosable owner vector/subgraph. Undisclosable branches'
local authorization/traversal results fold into an aggregate opaque closure commitment bound
to this request/cuts, leaking no owner, Anchor, count, or topology. `COMPLETE` proves closure
of “caller-visible evidence graph + aggregate opaque-closed branches,” not access to the
complete internal graph.

- `COMPLETE`: normalized query identity, semantic boundary/root authority cut, and root-local
  Principal/view are established. Every recursively discovered contributing owner independently
  establishes local PrincipalRef. Disclosable owners provide establishment evidence/view stamps
  bound to this request; undisclosable owner-local decisions fold only into privacy-preserving
  aggregate commitment. Caller-visible vector, source graph, and commitment together close all
  potentially intersecting, disclosable branches through safe pruning or complete processing.
  Only then do empty matches confirm no results.
- `PARTIAL`: query-root context is established, but at least one descendant source remains
  unclosed due to missing/dirty/exhausted-budget conditions. Returned matches are verifiable
  and may be empty. An unavailable descendant makes the defined aggregate query `PARTIAL`,
  not `UNAVAILABLE`. Authorized missing evidence may be opaque; diagnostics must not leak
  denied branch identities, counts, or topology. Unreturned items are not proved absent.

At query start, freeze only normalized query identity, caller-submitted identity/trust-evidence
identity, authorization request identity, semantic boundary/root authority cut, and root-local
Principal/authorization-view evidence. Recursively build caller-visible source graph,
disclosable per-owner vector, and aggregate opaque closure commitment. Do not assume a global
source snapshot enumerating the whole graph at startup or global reuse of root PrincipalRef,
issuer trust, or undiscovered remote views:

1. Each visited parent internally proves complete direct adjacency through fully paginated
   `ListDirect` at a stable authority cut. Parent/pages/edges enter caller-visible evidence
   only if the corresponding view permits disclosure.
2. Every candidate edge is safely pruned by conservative Range evidence or processed recursively.
3. Every potentially intersecting mount edge not safely pruned by parent-owned mount Range
   binds exact `DelegationBinding` and matching `MountAcceptance` cuts, then continues at
   carrier-local cuts. Safely excluded mounts retain prune evidence only, requiring no carrier
   connection, acceptance, or remote availability.
4. On first reaching an authority, its trust/access-control owner independently establishes
   local PrincipalRef/view for the submitted identity/trust evidence and same authorization
   request. Disclosable establishment evidence/view stamps join the vector; otherwise only
   owner-local decisions join opaque commitment. Unobtainable/unverifiable evidence cannot
   silently become an authorized empty branch.
5. `COMPLETE` requires closure of every potentially intersecting, disclosable branch in
   caller-visible evidence and inclusion of all undisclosable local results in a commitment
   leaking no owner/nodes/count/topology. A list of planned owners proves neither source nor
   authorization closure.
6. Edges added after a parent cut are outside that cut. Retained holds selected facts/evidence
   for a finite period; ordinary current fact changes do not automatically invalidate old
   continuation. CurrentVerified holds no cross-request old view: continuation proves original
   cuts still current or explicitly expires, never switches to latest. Expired/lost retained
   basis or unavailable current permission/required connection validity must invalidate,
   revalidate, or yield PARTIAL at a still-verifiable root according to failure source,
   never silently read latest.
7. Continuation binds the same normalized query, caller-evidence identity, authorization
   request, obtained disclosable principal/view vector, source graph, opaque commitment, and
   disclosable cuts. Hidden owner/view/cut/traversal state binds only through privacy-safe opaque
   continuation, not raw serialization. While the source graph is unsealed, newly discovered
   owners may independently establish Principal/views; a profile defines source admission after
   pagination starts. P3 seals fixed sources/gaps before the first page. Later adding an original
   Excluded source could omit smaller UUIDs and is forbidden. No method may silently replace
   Scope, existing cuts, or caller evidence. Retaining facts does not freeze Allow; every page
   rechecks current permission.

#### D10.2 Optional snapshots and fixed-cut pagination

Owners acknowledge writes after validation and reliable persistence. Delaying commit does
not provide pagination consistency. Ordinary reads default to latest committed. Each Core
independently configures optional `SnapshotRetention` at startup: omission disables old-view
retention; there is no default duration. Only a representable finite positive duration enables
it; invalid configuration fails. Disabled retention does not disable queries/pagination or
a temporary consistent view in one call.

`ReadMode` is selected and disclosed on the first page, with no automatic upgrade/downgrade:

| ReadMode | Fixed read basis and later behavior |
|---|---|
| CurrentVerified | No old fact view held across requests. Queries, pages, and in-scope exact reads remain available. Reuse proves original cuts still current; changed cuts explicitly expire rather than attach new results to the old chain. No guarantee that old pagination finishes under continuous writes. |
| Retained | Selected views retained for the configured finite period. Ordinary writes do not invalidate them. Pages/in-scope exact reads use original cuts until release, expiry, or contract-defined invalidation. |

CoreReadCut marks facts; Retained run-local handles manage old-view lifetime. This is neither
arbitrary timestamp querying nor another permanent truth.

A page chain fixes ReadMode, Start, UpDepth, Scope, Basis, conditions, source cuts, and ordering
position; a different depth requires a new query. Retention has finite TTL, capacity, and release
rules. Expiry never falls back to latest; both modes' exact reads independently authorize.
Contracts define context validity after a source restarts; P3 does not restore lost run-local
retention handles.

Cross-Core mode follows actual sources, including owners supplying only relationship,
adjacency, or projection evidence. Any source with retention disabled makes the whole chain
CurrentVerified; only all-retaining sources permit Retained. Disabled retention is not
unreachability, expiry, or missing coverage. Excluded sources are not acquired sources and
cannot later join an old chain. Contracts handle capacity failures without switching modes or
calling unreachable sources confirmed changed cuts.

Cross-Core results still require matching relationship/acceptance/projection evidence and a
consistency witness for the promised combination, not timestamps alone. P3's first bracketed
dual-Core read proves both states coexisted. CurrentVerified checks original cuts on reuse;
Retained uses held views without requiring every current cut to equal its old cut on every page.
Neither becomes a global atomic snapshot or adds unacquired sources after the first page.
Owner contracts define how current revocation, mount closure, and specified-contact changes stop
further disclosure; caches do not decide.

Multi-owner results have no synthetic `WorldRevision`. Each owner selects its own cut; mount/
projection evidence connects cuts. Unprovable root connection yields `UNAVAILABLE`; unclosed
descendant connection requires restart or `PARTIAL`.

### D11. Read evidence and committed observation form a recoverable chain

#### D11.1 Exact-read evidence

Keep three distinct evidence responsibilities even when one service performs several:

- **Core/Facet facts and delegation/acceptance/namespace-attachment/contact control facts:**
  Core- or Facet-kind `FactAuthorityRef`, `AuthorityEvidence`, owner-local `ReadCut`,
  record Revision/retirement evidence. Authoritative NextHop revisions belong to the contact
  holder's cut, not spatial records, payload, or relationship identity.
- **Authorization:** access-control owner identity (currently `RealmID`), Principal establishment
  evidence, Allow/Deny, and authorization-view stamp.
- **Actual reachability:** target authentication, endpoint binding, and independent freshness
  from connecting through specified contacts. This neither replaces persistent contact
  revisions nor joins spatial/payload Revision or proves writer activation. Lost connection
  caches are rebuildable; lost contact-control history is a Store/recovery problem.

Exact reads distinguish at least `PRESENT`, `NOT_FOUND`, `UNAVAILABLE`, and `DENIED`.
`DENIED` asserts no existence state. `NOT_FOUND` requires verified exact fact authority,
scope, and read cut. Core-confirmed `FacetAttachment = NONE` at an explicit cut proves no
accepted Facet namespace. Unconfirmed attachment, invalid contact, unverifiable permission,
Store failure, and incomplete source sets are not `NOT_FOUND`.

Aggregate internal cuts form a vector, but public results disclose only permitted owners/cuts;
hidden branches use D10 opaque closure commitment. The vector states which disclosable owner
was read at which cut, not global simultaneity, total order, or cross-owner transaction.

#### D11.2 Observation and resume

Every authoritative owner supports committed changes following a returned read cut:

```text
owner snapshot/read cut
  -> committed observation cursor
  -> ordered owner-local changes
  -> resume(cursor)
  -> explicit GAP when retained history is insufficient
```

1. Observation describes only committed facts/control facts. Events are not the facts themselves;
   a messaging system is not the owner Store.
2. Order exists within one authority stream only. Public cross-owner observation returns only
   disclosable cursor vectors, not global sequences or hidden owner lists.
3. Coordinators may internally maintain fuller observation graphs/hidden cursors within owner
   authorization boundaries, never serialize them directly. Callers receive only privacy-safe
   opaque resume state bound to normalized query, caller-evidence identity, disclosable view
   vector, and aggregate opaque commitment. It leaks no hidden owner, stream, nodes, counts,
   topology, or change frequency.
4. Delivery may be at-least-once. Disclosable changes have stable owner-local identity/cursor
   for deduplication. Hidden branch fact changes produce no public event while aggregate opaque
   closure validity remains provable.
5. Cursor/opaque-state retention is finite. Cursor expiry, missing segments, discontinuous
   source epochs, or unprovable opaque closure/view validity require `GAP` and a new
   snapshot/query. Recent apparent equality does not prove nothing was missed.
6. Continuous spatial observation derives from
   `initial COMPLETE/PARTIAL query + disclosable owner streams + opaque observation state`.
   Enter, Exit, Move, TypeAdded, and similar events are rebuildable projections, not written
   back into Core/Facet truth.
7. Parent `Structure/ListDirect`, `DelegationBinding`, and matching `MountAcceptance`
   control streams determine the internal Core source graph. Facet also requires tracking
   `FacetAttachment` proposal/acceptance streams. New/replaced disclosable sources begin with
   snapshot+cursor; hidden sources update only privacy-safe state, not public source-set
   changes. Unprovable continuity from prior cursors, opaque state, and control facts yields
   `GAP`. Aggregation becomes partial at a still-defined root context; unestablished root
   context or exact FacetAttachment is unavailable.
8. Any per-owner view change may revoke future disclosure and invalidate continuations/
   subscriptions. Newly disclosable sources start from new snapshot+cursor, not leaked
   hidden-cursor history. Authorization changes do not rewrite committed Core/Facet facts.
9. Polling, outbox, NATS, database change feeds, SDK watchers, gateway state, or opaque signed
   resume tokens may implement this behavior, provided snapshot-to-stream continuity, detectable
   gaps, privacy boundaries, and no promotion of bus/token into authoritative truth.

### D12. Owner transition preserves stable UUIDs but permits explicit unavailable windows

The same Element or an explicitly owned authority coverage slice may preserve every
ElementUUID across fact-authority changes. Shadow Elements, dual writes, dual visibility,
and global cross-authority atomic transactions are forbidden. A subtree spanning independent
authorities is no implicit atomic migration: each contributing slice must separately retain
or transition owner evidence, or the aggregate remains unavailable/incomplete.

Core Structure and fact-authority changes are orthogonal. Reanchor may preserve owner;
owner replacement may preserve Structure. Cross-authority reanchor combines both.
Facet authority transition is covered separately in D12.2:

| Change | FactAuthorityRef | StructuralAddress | ElementUUID |
|---|---|---|---|
| Endpoint/Pod/hosting Cluster change | Unchanged | Unchanged | Unchanged |
| Same-authority writer/durable-owner handoff | Unchanged; commit epoch advances | Unchanged | Unchanged |
| Same-authority reanchor/reparent | Unchanged | Follows new Structure | Preserved |
| Cross-authority reanchor/reparent | Changes with continuity evidence | Follows new Structure | Preserved |
| Actual authority/carrier replacement with unchanged Structure | Changes with continuity evidence | Unchanged | Preserved |

#### D12.1 Same-authority writer handoff

1. Permanently advance/close the old writer's commit fence/epoch in the authority's own durable
   commit domain first. Unreachability, changed NextHop, Operator assignment, and new leases
   do not prove fencing.
2. The new writer commits only with a higher installed fence. Relevant contact owners then
   update their specified contacts. Address changes do not install writer fences or require
   a system-wide address-update point.
3. If the old fence cannot be confirmed, do not enable the new writer; return `UNAVAILABLE`.
   This is preferable to possible dual writes.

#### D12.2 Semantic continuity across authorities

Cross-owner reanchor, carrier replacement, Facet authority replacement, and accepted coverage
replacement require a unique authority-binding switch. D12 adds no global transition service.
The switch belongs to an existing control relationship with one owner:

- Carrier replacement changes the parent Core's `DelegationBinding`.
- Facet replacement changes `FacetAttachment` proven by the Element's current Core.
- Cross-owner reanchor of one Element or an explicitly owned Core coverage slice is released
  irreversibly with successor evidence solely by its current Core.
- Root FactAuthorityRef replacement claims continuity only with successor evidence from current
  root authority. Updating SpaceReference is not proof. Permanent loss of old authority without
  a preexisting fact-authority control binding leaves the old space `UNAVAILABLE`; a new
  reference is a new entry and cannot inherit old root identity by itself.

Cross-owner reanchor proves two orthogonal facts:

1. **Identity/owner continuity:** old Core or existing binding owner irreversibly released
   UUID/coverage with final read cut and successor evidence.
2. **Target topology placement:** target parent's active direct-content coverage owner accepted
   the final spatial tuple at explicit parent/Range/Structure cuts, with containment and
   acyclicity checks. Mount placement additionally matches parent `DelegationBinding` and
   carrier `MountAcceptance`.

Only both proofs permit target `NEW_ACTIVE`. Contacts, target self-assertion, or continuity
proof alone cannot replace placement acceptance.

Facet replacement has the same orthogonal structure, but target acceptance is of the complete
namespace after the old final cut, not spatial placement. Core switches `FacetAttachment`
only after the former namespace is fenced and the new acceptance holds.

Only these three observable states and their forward order are frozen here. A transition
successor defines operation identity, byte transfer, persistence records, and recovery:

```text
OLD_ACTIVE
  -- old owner durable fence + final read cut -->
UNAVAILABLE
  -- unique control owner commits successor/rebinding evidence
     + matching target acceptance -->
NEW_ACTIVE
```

- `OLD_ACTIVE`: only old owner is visible/writable.
- `UNAVAILABLE`: old owner is fenced; new owner is not visible/writable.
- `NEW_ACTIVE`: only new owner is visible/writable. Old owner permanently rejects active
  commits/disclosure of the old placement. Private retention may keep non-authoritative
  historical bytes and continuity evidence, neither query results nor a second truth.
- Failure/recovery stays at or converges forward to one of these states; the same UUID cannot
  simultaneously appear in old and new Structure.

The target may prepare bytes in a private, undiscoverable, unwritable transfer area, not a
`shadow Element`, second truth, or query result. Zero interruption is not promised.
UUID continuity, legal final tuple, preservation of acknowledged writes, and at most one
visible owner/writer at any time are promised.

Same-Core reanchor does not change FacetAttachment. Cross-Core transition does not by default
move Facet targets or records; old Core-owned attachment loses validity with the old fence.
New Core may target the same Facet, but cannot silently reuse acceptance bound to old Core
proposal/existence cuts. Facet must give matching reacceptance for the new Core proposal before
new Core commits `ACTIVE`. Core transition may finish independently before reacceptance,
but new Core returns `DETACHED`/unavailable attachment evidence until acceptance is restored.
Facet authority handoff follows the same single-writer rule. Observation ends the old stream
with successor evidence and continues at an explicit new-authority cut, not a naturally
shared cursor.

Delegation revocation without completed transition leaves the branch detached/`UNAVAILABLE`,
not deleted or automatically returned to parent.

## 5. How the decisions close the five public behaviors

| Public behavior | Minimum input | Required result/guarantee | Main decisions |
|---|---|---|---|
| `Enter` | SpaceReference + caller trust evidence | Temporary SpaceHandle; independently verified boundary, authority, authorization, specified contact, and connection | D6–D9 |
| `Discover` | SpaceHandle + StructuralAddress or QueryRegion | `INVALID`/`DENIED`/`UNAVAILABLE` outcome, or verified `COMPLETE`/`PARTIAL` result with source cuts, disclosable view vector, opaque closure commitment, completeness, and privacy-safe missing evidence | D9–D11 |
| `Read` | Discovered Core/Facet reference | Exact record/absence/unavailable/denied, owner, Revision, ReadCut | D4–D6, D11 |
| `Observe` | Owner ReadCut, or DiscoveryResult caller-visible cuts + opaque observation/resume state | Disclosable committed changes/cursors, privacy-safe resume, gaps, and authorized source-graph changes | D11 |
| `Propose` | Identity, authority evidence, final tuple/payload, CAS, OperationID | Convergent committed/denied/conflict/unavailable/indeterminate results | D4–D8, D12 |

`Resolve`, `ListDirect`, and `QueryRegion` are Discover forms; create, update, reanchor,
delete, and Facet Put are owner-specific Propose forms. The narrow waist does not require a
generic CRUD interface shared by every module.

## 6. Planes and phases are independent dimensions

### 6.1 Four planes

| Plane | Frozen here | Not frozen here |
|---|---|---|
| Truth model | Core `Structure + Pose + Range`, optional Facet exact payload records, Element/Facet identity invariants | Authority, permission, contacts, Store schemas, caches, indexes, event projections |
| Owner-local operations | Exact read, ListDirect, final-tuple mutation, Facet enumeration, and sole owners/observable outcomes of delegation/acceptance/authorization/reachability control relationships | Go methods, RPCs, database transactions, private control layouts |
| Business operations | Enter/Discover/Read/Observe/Propose, completeness, failures | Application gameplay, business workflows, real-time physics queries |
| Production operation | Single-writer fencing, contact/connection freshness, explicit capacity, recoverable transition, rebuildable derived layers | Kubernetes, etcd, NATS, particular deployment topology |

Authority, delegation, permission, specified contacts, and fences have their own authoritative
control state, but are orthogonal support planes, not UCUPA world truth. StructuralAddress,
indexes, query results, events, and connection caches are derived. First identify a capability's
plane, then its implementation phase. Deferring federation execution does not remove its
constraints on current truth/query design; deferring mechanisms does not make them world truth.

### 6.2 Capability-proof dependencies and execution order

Required facts/evidence cannot be omitted, but do not impose one whole-phase chain on every mechanism:

1. Core operations, mounts, and queries use valid D1–D4 facts. Mounts additionally need D6
   bilateral control, independent authorization, and contacts; combined queries prove source
   cuts/coverage. Facet is optional, not a prerequisite for Core mounting.
2. D5 Facet services satisfy D6 attachment/acceptance and D7 authorization with current source
   Core evidence. They need no completed Core mounting or public-network deployment.
3. D11 owner-local Observe requires owner cut, commit order, cursor, and gap evidence.
   Aggregation separately needs source graph, owner streams, views, and private recovery;
   one stream cannot replace it.
4. Cross-machine access first proves public operation encoding, D8 contacts, target authentication,
   caller establishment, and independent authorization. Full open interoperability additionally
   proves D6–D11 public behaviors and §7's independent-implementation gate, not same-machine
   or single-vendor substitutes.
5. D12 transition needs real fences, final cuts, unique control-binding switches, and target
   acceptance; observation continuity additionally needs D11. Address changes, unreachability,
   or deployment size cannot substitute, nor must every unrelated transition form be implemented.
6. Production/scale proof specifies capabilities, failures, and capacity profiles with appropriate
   correctness evidence. Bounded experiments need not wait for unrelated capabilities, but claims
   of open federation, handoff, or HA require their relevant proofs. Indexes replace execution
   strategy only after public query contracts close and same-cut conservative oracles agree.

The complete target still covers D1–D12 and axiom counterexamples. Smaller initial slices do
not make observation, federation, or transition optional. The
[development plan](development-plan.md) alone maintains phase numbers, default sequence,
progress, and slices; local completion does not prove an entire semantic group.

## 7. Integrated counterexamples and successor falsifiability gate

Use these scenarios at two levels, without a circular “implement before accept, accept before implement” rule:

1. **Concept acceptance:** logical counterexamples have been reviewed. Declared truth, control
   facts, and public outcomes give unique consistent expectations without hidden directories,
   second truth, global transactions, or undeclared owners. This is not current-code proof.
2. **Successor implementation contracts:** turn relevant scenarios into public conformance gates.
   Phases claim only their stated coverage. Item 16 belongs to transition, item 19 to open
   interoperability, item 20 to an explicit scale profile; these do not block smaller prerequisites.

Unified expectations:

1. **Coordinate closure:** after multiple rotations/translations, any Element projects into
   any ancestor frame. Root has no LocalPose; external polar input creates no second truth.
2. **No Range misses:** a large Entity centered outside the query sphere still matches the
   linear oracle when the spheres intersect or touch.
3. **Parent–child coverage:** direct containment throughout a random deep tree implies every
   descendant is within ancestor Range. Out-of-bounds tuples under bounded parents reject
   atomically; valid finite direct children under `UNBOUNDED_ROOT` are accepted by type
   without reading root radius.
4. **Numeric boundaries:** optimized queries agree with conservative oracles on deep chains,
   extremes, and contact boundaries. Uncertainty gives extra candidates or unavailable, not misses.
5. **Readable structure:** after clearing children/path/index caches, Core records, owner-local
   ListDirect, and required mount delegation/acceptance reconstruct Structure/StructuralAddress.
6. **Explicit scope:** Start + UpDepth selects Scope; independent deep basis does not truncate
   its closure. Ancestor-direct Entities and higher sibling descendants may match. Changed depth
   requires a new query; cross-Core meaning is unchanged. Return actual Scope and rereadable
   path, not a new boundary inferred from interrupted execution.
7. **Concurrent source graphs:** new/revoked/changed mount/acceptance during query permits completion,
   invalidation, or partial only under bound Structure/delegation/acceptance cuts, never false
   `COMPLETE`.
8. **Discovery authorization:** closure discovery Grants without target UUID Grants permit unknown
   discovery. New recursive owners independently establish local PrincipalRef/views from the same
   caller evidence, without root-principal reuse. Denied branches leak no details. Descendants
   excluded because a necessary Anchor chain is undisclosable are classified undisclosable in
   that view, not first counted allowed then silently lost.
9. **Optional/open Facet and acceptance:** Core-only Elements are complete without Facet.
   An independent implementation without business code can enumerate opaque TypeRefs and read
   exact payload without understanding its format. Core cannot activate from `NONE` without
   matching acceptance; target cannot serve before Core `ACTIVE`.
10. **Facet lifecycle:** reanchor preserves FacetKey/target. Cross-Core transition does not move
    records, but the same Facet must reaccept new Core attachment or remain `DETACHED`/unavailable.
    Reads established after deletion's cut return no payload; confirmed pre-deletion responses
    retain original evidence. Core unavailable is not absence.
11. **Independent owners:** one Realm/Cluster may host several FactAuthorityRefs, and an authority
    may change Cluster/credentials, while queries distinguish continuity from contact changes.
12. **Rootless carrier:** carrier has no local root and copies no mount Anchor. Queries from the
    parent boundary still traverse it with independent cuts.
13. **Invalid contact:** unreachable/mismatched NextHop yields only `UNAVAILABLE/PARTIAL`, not
    changed Element, Facet, permissions, delegation, or owner. One direction's address update
    cannot implicitly change the reverse relationship.
14. **Revocation/mount deletion:** revocation neither removes contents nor returns them to parent.
    Old local writes cease being accepted through the relationship; fencing is proved separately.
    Unfinished transition remains detached/unavailable. Mount deletion fails for unreachable,
    revoked, or detached carriers, never treating unknown coverage as empty.
15. **Observation recovery/privacy:** disconnects after snapshot, duplicate delivery, expired cursors,
    and new sources permit deduplication or GAP. Public cursors/source changes include only
    disclosable owners; hidden branches use opaque state leaking no owner/count/topology/change
    frequency. Neither silent missed events nor privacy leaks are permitted.
16. **Handoff fault injection:** interruption at each durable boundary yields only `OLD_ACTIVE`,
    `UNAVAILABLE`, or `NEW_ACTIVE`, never dual writes/visibility or lost acknowledged writes.
17. **Multiple roots:** without projection evidence, reject shared-basis fabrication and cross-root
    `COMPLETE` claims.
18. **Lost derived layers:** removing indexes, connection/query caches, Client projections, and
    event transport leaves facts/contacts intact and permits entry, rebuilding, and observation.
19. **Open implementations:** at least two implementations sharing no internal Go types, Stores,
    or gateway code pass the same contract corpus.
20. **Pathological scale:** deep chains, high fan-out, heavy Range overlap, and unreachable owners
    cause bounded cost, continuation, partial, or unavailable, not false empty results.

Concept acceptance freezes expected outcomes, not runtime proof. A successor/implementation
claims a capability complete only after passing its phase-relevant gates.

## 8. Explicitly excluded unnecessary truth and mechanisms

- Current-baseline Metadata/displayName fields, APIs, Stores, and Revision semantics. Introduce
  optional capabilities through an independent contract only after real consumers prove common
  minimum need; never make them prerequisites for identity, structure, query, addressing,
  routing, or authorization.
- Text-name resolution, aliases, a DNS project, or name-registration service.
- Bare-UUID universe-wide locators; global Element/owner/Cluster directories.
- Global root, World, WorldRevision, clock, transaction, or cross-owner atomic snapshot.
- Merging Anchor structure, authority graph, Realm trust/permissions, contacts/network, and
  Cluster topology into one tree.
- WorldPose, StructuralAddress, SubtreeEnvelope, indexes, query results, events, or caches as
  a second truth.
- Polar/geodetic coordinates, scale, Collider, asset bounds, LOD, rendering, or high-frequency
  physics state in Core.
- Execution truncation, forwarding hops, timeouts, or page limits silently changing semantic
  universe. UpDepth is an explicit semantic Scope selector and is not prohibited here.
- Exact UUID Grants as authorization for unknown discovery.
- Requiring central schemas, type artifacts, or type contracts for Facet content storage.
- NextHop, connection success, or underlying routing controlling ownership, permission, or
  writer activation.
- Shadow Elements, dual writes, delayed deletion, or unreachability as fencing proof.
- `/1`, `/2`, `/3`, old Store readers, converters, or compatibility branches for deleted prototypes.
- Go packages, module counts, databases, messaging buses, Kubernetes, or current Composition
  fixtures treated as spatial axioms.

## 9. Projection and implementation-contract boundaries

- UCUPA supplies no Console product or built-in visualization responsibility/future task.
  External clients implement browsing/management through public owner operations, explicit
  permissions, and result semantics, not direct Store access or a second truth. Programmable
  management, authorization, observation, and necessary diagnostics retain their existing
  responsibilities; removing panels neither removes them nor creates Operator/new-module UI work.
- Platform product/minimum-truth and plan capability/stage views are non-normative projections.
  They cannot independently add/change owner, truth, outcomes, completeness, or §6.2 proof
  prerequisites. This document prevails on semantic conflicts. The plan maintains phase numbering,
  grouping, and default sequence, not a frozen whole-phase chain. Older same-named documents
  are frozen predecessors, not synchronized projections.
- The [authority manifest](README.md) indexes specifications/implemented contracts without
  duplicating semantics; the [development plan](development-plan.md) maintains progress/gates only.
- Bounded contracts declare implemented and unimplemented D1–D12 clauses and corresponding
  black-box gates. One complete slice is not all uncovered semantics implemented.
- Greenfield defines one current form directly, without predecessor readers, old-data migration,
  converters, version routing, or compatibility branches. Implementation contracts define
  private Store formats.

## 10. Accepted content and successor boundaries

Whole-document acceptance freezes:

- The twelve decisions and cross-decision invariants.
- Target Core Element truth `Structure + Pose + Range`.
- Optional Facet opaque 32-byte TypeRef, exact Payload, and type enumeration.
- FactAuthorityRef and sole-owner entries for Core mounts/per-Element FacetAttachment.
- RealmAttachment using existing RealmID/specified NextHop without changing D7 permission semantics.
- Closure-bounded discovery authorization.
- Separate SpaceReference/SpaceHandle/StructuralAddress responsibilities.
- Complete/partial/unavailable/denied, invalid requests, per-owner cuts, resume/gap.
- Separately confirmed query semantics: upward-depth Scope selection including -1, full descendant
  search, optional SnapshotRetention, CurrentVerified/Retained modes, and no delayed reliable
  writes. P3's target contract is finalized; only the plan records implementation authorization
  and delivery.
- Owner transition permitting unavailability while forbidding dual writes.

Whole-document acceptance does not freeze:

- Go APIs, protobuf/HTTP, or exact error-enum spelling.
- RealmAttachment record granularity, placement, interfaces, maintenance, or recovery contracts.
- Store schemas, FormatID literals, or databases.
- Octree/BVH/cache/query planners.
- Credentials, signatures, TLS, lease clocks, or security products.
- Operator, Archive, Partition, StoreGroup, or Cluster deployment profiles.
- Polar/geodetic projection APIs, product Facet contracts, or upper-layer application behavior.

The user-confirmed [local query contract](core-local-query.md) and
[mount contract §5](core-mount-contract.md#5-reads-paths-and-combined-cuts) already finalize P3 zero/omission/
root behavior, structural paths, optional retention, capacity/release, and current-connection
gates; these are not pending design items above. Current implementation, verification, and
acceptance follow the plan/current authority manifest, not duplicated execution state here.

This document is accepted as a whole. A new counterexample overturning any D1–D12 decision
requires revising dependencies and rerunning every counterexample gate, not transferring
contradictions to implementation. This document maintains no current action or code authorization.
