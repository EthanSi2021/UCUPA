# UCUPA foundational axioms

**Status: accepted, stable governing axioms; constrain detailed semantics; carry no current status or plan; do not authorize implementation**

This document consolidates UCUPA's original purpose, capability reassessment, and feasibility
assessment for an open spatial system into accepted governing constraints. It is not a new
module, API inventory, implementation contract, or development plan. All detailed semantics
must follow these axioms. The [architecture authority manifest](README.md) alone indexes current
authority and implementation contracts; the [development plan](development-plan.md) alone
maintains current work.

Historical problem evidence is in the original UCUPA functionality and design map; itemized
judgments are in the capability reassessment ledger; the feasibility assessment explains UCUPA
as infrastructure for an open three-dimensional spatial network and an unbounded cyber
universe. The Core Range and spatial-root decision remains a focused evidence draft. Accepted
target semantics for Range and roots are defined by D2, D3, D4, and D10 of the
[concept and mainline baseline](ucupa-concept-mainline-baseline.md).

## 1. Definition

> **UCUPA is a logically unbounded network of stable spatial facts maintained by independent
> authorities. Its minimum public spatial semantics are Element structure, pose, and
> domain-neutral discovery coverage; Facet carries typed stable extensions attached to
> Elements. Evidence-bearing entry, discovery, reading, observation, and change contracts
> connect the space.**

“Logically unbounded” means that new finite spaces and autonomous authorities can continue
joining without changing public semantics. It does not mean infinite precision, infinite
capacity, permanent availability, or an operation that covers the entire system.

```text
External resources and arbitrary higher-level systems
Applications / robotics / other consumers
                  │
      Enter / Discover / Read / Observe / Propose
                  │
════════════ UCUPA public spatial narrow waist ════════════
 Core: Element existence and spatial facts
 Facet: typed stable extension records attached to Elements
───────────────────────────────────────────────────────────
 Fact authority / federation: sole fact owner, delegation, acceptance
 Access control / trust: caller establishment, authorization relation
 Reachability: authority-specified contacts, connections, independent freshness
───────────────────────────────────────────────────────────
 Replaceable Store / index / gateway / transport / deployment
```

## 2. Ten foundational axioms

### A1. Open autonomy

UCUPA consists of independently operated spatial-fact authorities. Growth does not depend on
one central world database, a central organization, or every participant upgrading a common
internal implementation.

### A2. Element identity

Anchor and Entity are the two closed Element kinds. ElementUUID is a stable, non-reusable
identity; rename, Pose update, reparent, and route changes do not change it. A fact-owner
transition claiming continuity of the same Element must also preserve its UUID. D12 of the
[concept and mainline baseline](ucupa-concept-mainline-baseline.md) defines that continuity in
detail. A UUID does not identify position, path, permission, or endpoint, and there is no
system-wide directory that finds any Element throughout the open system from its bare UUID.

### A3. Spatial completeness

In the target Core, an ordinary Element's stable spatial facts consist of
`Structure + parent-relative Pose + domain-neutral Range`. Pose answers origin and orientation;
Structure answers relative to which Anchor; Range answers where that spatial existence may
extend. Together they support general spatial discovery.

Range must have deterministic, composable, conservative intersection semantics without false
negatives. These axioms do not freeze its shape, Anchor aggregation, parent–child containment,
spatial-root representation, or numeric encoding. D1–D4 and D10 of the mainline alone define
the current detailed target.

### A4. Spatial structure

An Anchor is a local coordinate frame and structural node. An Entity is a spatial leaf
occurrence within an Anchor. Every structural parent is unique and structure is acyclic;
business relationships do not enter Core. D1, D2, D4, and D9 of the mainline alone define the
detailed targets for roots, Pose, Range, and spatial boundaries.

### A5. Derived values are not authoritative truth

WorldPose, structural ancestry/address, SubtreeEnvelope, children indexes, spatial indexes,
query results, derived events, endpoints resolved through specified contacts, actual
reachability, and connection caches derive from authoritative facts and the current runtime
context. They may be persisted for performance or recovery. Their loss may require index
rebuilding, a new snapshot, or reestablishing observation, but must not change authoritative
facts or create competing Element truth or a second writer.

Contact information specified by an authorized party in a relationship may be authoritative
control facts of that relationship's owner; it is not Element spatial truth or Facet payload.
Distinguish “how this relationship's authority specifies contacting its target” from “whether
the caller can connect now.” The former is not a cache that may be discarded arbitrarily; the
latter cannot create ownership or permissions. D6 and D8 define the contact mechanism. This
axiom requires no Ref address directory, route registration, or particular deployment system.

### A6. Facet is a controlled extension surface

Facet holds stable typed records attached to existing Elements. The Facet truth owner owns
the key, record existence, exact payload, owner-local revision, and final mutation; the type
definer owns the payload's domain meaning and validation. The access-control owner establishes
a caller as PrincipalRef and holds authorization relationships such as PermissionBinding.
Core and Facet define the PermissionID and Scope needed for their operations and enforce
authorization results at the final read or mutation boundary. Realm is the current reference
implementation of this access-control role, not an owner or copy of Core/Facet facts. Type
identity must be stable and unambiguous; its encoding and resolution mechanism are decided
separately.

Facet does not own Element existence, copy Core spatial facts, revive deleted Elements, or
contain “all non-spatial data.” Asset bytes, accounts, workflows, real-time physics, and private
system processes remain owned by their respective systems.

### A7. Sole authority and orthogonal topologies

Every authoritative fact has exactly one verifiable semantic owner at each valid cut.
Replication, caching, routing, and deployment cannot create a second writer. None of these
five relationships implies another:

```text
Spatial structure                 = where an Element is and relative to which frame
Fact-authority / delegation graph = who owns, commits, and proves Core/Facet and delegation facts
Access-control / trust graph      = who establishes callers and permits observation or proposed operations
Reachability graph                = how the relevant fact owner can currently be reached
Deployment topology               = where Stores, processes, and Clusters run
```

An Anchor is not a Realm, a Realm is not a Cluster, and a Route is not a fact owner.
Deployment migration and route changes must not change Element identity or spatial meaning.
Core/Facet and delegation/acceptance fact owners, access-control/trust owners, reachability
owners, and deployment owners each have explicit control facts. No relationship automatically
grants authority in another or expands Core/Facet world semantics.

### A8. Entry, addressing, and discovery are separate

Consumers obtain an explicit spatial entry, caller/trust context, and current reachability
context independently, then combine them to resolve known structural addresses or discover
unknown Elements. Their owners may differ; none implies either of the others. D9 freezes the
responsibilities of `SpaceReference`, `SpaceHandle`, and `StructuralAddress`; specific encodings
are not part of this axiom.

Structural addresses must derive from current Anchor structure and may change after
reparenting. UUID only confirms identity. Known-ID point CRUD is not a spatial-infrastructure
baseline, and callers must not be required to know the target UUID beforehand or guess a
common ancestor.

### A9. Queries and observation must be honest

Spatial discovery must not silently omit an Element because its center lies outside
QueryRegion, an index is stale, a relevant fact owner is unreachable, or an execution budget
is exhausted. Completeness must explicitly bind this query's semantic universe, selector,
authorization view, and each contributing fact owner's read cut. Preserve frame, fact owner,
freshness, coverage, missing-fact-owner evidence, and route/endpoint reachability evidence
separately. This axiom requires a strict distinction between “confirmed absent” and “currently
cannot confirm”; D10 and D11 define the concrete result semantics.

The semantic query universe and execution budget must be separate. A caller may explicitly
select a structural or spatial universe. If hop, depth, fan-out, deadline, or page size are
only budgets, exhaustion may produce continuation or an incomplete result, not redefine
“complete” within that universe.

Stable fact changes must support observation from every contributing fact owner's explicit
read cut, gap detection, and recovery. Multi-owner results preserve their separate cuts,
cursors, missing-fact-owner evidence, and independent route/endpoint evidence; they do not
synthesize a global atomic starting point. D11 freezes observable semantics; specific streams
and transports remain implementation mechanisms. Events are derived observations of
authoritative facts, not a second source of truth.

### A10. Federation and higher-level runtimes do not merge truth

Delegation, acceptance, reachability, access-control permission, and write authority are
independent. Federation adds traversability without merging owners, control planes, clocks,
or transactions. An open system requires no global root, global spatial index,
`WorldRevision`, cross-owner atomic snapshot, or fact-authority, access-control, trust,
organizational, or deployment hierarchy matching the Anchor tree.

UCUPA preserves stable spatial facts that remain valid across Runtime stops and restarts.
Higher-level systems may persist their own assets, authoring data, business state, checkpoints,
and product projections, and may own real-time authority within finite regions/epochs. They
must not mirror UCUPA Element existence, Structure, Pose, or Range as a separate independent
authority. Only changes explicitly mapped to stable Core/Facet changes, independently validated
and committed by the relevant Core/Facet fact owner, become UCUPA facts.

## 3. Behavioral consequences of the public narrow waist

The axioms require at least these five product-neutral behaviors. Their names do not freeze
Go APIs, wire formats, modules, or process boundaries:

1. **Enter**: combine independently obtained spatial entry, caller/trust, and current
   reachability contexts.
2. **Discover**: discover unknown Elements by Structure or spatial region.
3. **Read**: read Core/Facet with provenance, authoritative cuts, and completeness evidence.
4. **Observe**: observe committed facts, detect gaps after interruptions, and recover.
5. **Propose**: submit identified, preconditioned changes with convergent retries to the
   relevant Core/Facet fact owner confirmed by fact-authority evidence. New facts arise only
   when that owner accepts and commits them.

An adapter may convert encodings, transport, and caller context or cache evidence-bearing
projections. It must not fabricate missing discovery or observation, mirror a permanent world,
synthesize a global Revision, manufacture cross-owner atomicity, or disguise `UNAVAILABLE`
as an empty result.

## 4. Explicit exclusions

- A global `World` object, central database, bare-UUID locator, global Revision, or global transaction.
- Merging the Anchor tree, fact-authority/delegation graph, access-control/trust graph,
  organizational structure, and Cluster topology into one tree.
- Making displayName, text paths, or name resolution prerequisites for identity, routing,
  authorization, or correct discovery.
- Promoting WorldPose, structural addresses, indexes, SubtreeEnvelope, events, or query caches
  into a second source of truth.
- Built-in Asset, Mesh, Collider, LOD, rendering, physics, or product business semantics.
- Automatically interpreting external asset bounds as Core Range.
- A central bridge or adapter privately constructing a second world index.
- Mirrored truth, shadow Elements, or cross-fact-owner atomic promises for migration.
- Treating Store, database, Transport, Carrier, Operator, Console, or module counts as world axioms.
- Version suffixes, old Store readers, converters, or compatibility branches for deleted prototypes.

Facet or consumers may still provide `displayName` as an optional presentation feature, but it
is not a prerequisite for the UCUPA baseline.

## 5. Falsifiability gate

A successor unable to demonstrate these outcomes must return to the axioms for review:

1. After all higher-level Runtimes stop, stable Elements and Facets remain with their
   respective fact owners and can be obtained again.
2. An explicit spatial context permits discovery of unknown Elements, not only reads and
   writes by known UUID.
3. A large Element whose center is outside the query region can still match when its coverage
   intersects; inability to prove completeness is explicitly reported as incomplete.
4. An unavailable route/endpoint or required fact owner is not interpreted as Element absence.
5. After derived address-resolution results, indexes, connection caches, and Client projections
   are cleared, retained authoritative facts and contact-control records allow reconstruction.
   This does not authorize deleting persistent contacts or commit history.
6. Replacing Store, transport, gateway, language, or deployment topology does not change public
   spatial semantics.
7. Removing any particular consumer leaves UCUPA a complete, general-purpose spatial system.

## 6. Authority boundary

- This document freezes only A1–A10. The
  [concept and mainline baseline](ucupa-concept-mainline-baseline.md) is the sole detailed target
  semantics subordinate to these axioms.
- This document does not maintain implementation status, next work, Store, API, module, or
  phase. Such material cannot establish that a concept belongs in the axioms.
- A projection or implementation contract needing semantics absent from these axioms and the
  mainline must first revise the relevant authority and pass its counterexample gate again;
  downstream documents must not invent those semantics.
- Implementation still requires an independent contract and explicit start authorization.
  Acceptance of the axioms or mainline does not itself authorize code.
