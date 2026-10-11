# Local Core/Facet fact and lifecycle implementation contract

**Status: P4 local implementation and full verification are complete, and delivery was accepted under the user's acceptance-and-commit authorization. This is the local Core/Facet increment contract; the architecture authority manifest maintains its exact current scope, and the development plan records delivery/commits.**

**P5 notice:** The accepted [public access and fact-observation contract](public-access-and-observation.md) directly supersedes this contract's TypeRef/resolver/type-validation and mirror clauses, public Revision keys, and original-request-binding persistence clauses, adding public access/observation.
**P7.3a notice:** [Public-profile section 8](public-access-and-observation.md) revises type semantics to arbitrary opaque 32-byte TypeRefs and bounded byte-preserving opaque Payload. Facet neither fetches/parses type artifacts nor validates schemas; pin/mirror/witness clauses are removed entirely. Sections 3.2, 8, and 9.3 reflect the sole current type/persistence form. Other P4 attachment, contact, spatial/content ownership, and independent-commit rules remain. No old type reader or dual public interfaces.

**P7/Stage 2 successor notice:** [P7 sections 8–10](owner-transition-contract.md#8-facet-namespace-replacement-and-reacceptance) supersede this contract's exclusions of namespace replacement, same-Facet reacceptance, detached-relation recovery, and retirement, limited to delivered initial profiles. F0 and the sole content commit domain remain. The [RealmAttachment contract](realm-attachment-contract.md) supersedes local-authorization assembly limits: owners each persist their contact; local/network calls authorize independently each time. Exact current scope follows the manifest, not a rewritten P4 delivery history.

This contract makes D5's minimum exact Facet truth, D6's non-deferrable per-Element FacetAttachment, and D7's authority-qualified scopes an implementable, falsifiable local slice. It establishes no public Enter, Discover, remote trust, mount federation, authority transition, or production deployment.

G1 here is only a historical shorthand for accepted Core local truth, not a current scheduling number. The [development plan](development-plan.md) alone controls execution.

"Relationship-specified address + stable identity verification" replaces old Ref-resolution clauses directly, without dual contact mechanisms or compatibility interfaces. Address, reverse-validation, retry, and persistence alignment preserves the then-payload/independent-commit boundaries. The new interfaces and concurrency rules were implemented, verified, and accepted with P4; they are not retroactively declared earlier capabilities. Phase 1 already supplied Core self, owner-qualified scopes, and shared commits, reused here rather than rebuilt. The user explicitly started P4 after P3, then authorized acceptance/commit; its attachment/type lifecycle was implemented under this contract. The [authority manifest](README.md) and affected notices switched together at P4 commit, only for the listed increment.

This is an exact increment, not a copy of G1. G1's D1–D4 `ElementRecord`, coordinates, Range, numbers, Structure, public create/read/spatial mutation/delete, ElementRevision, `ListDirect`, error order, read-cut, and geometry-oracle invariants still govern. Supersede/extend only clause families necessarily changed by new control facts:

- Core self owner/ref profile and exact-read envelope.
- Atomic attachment effects of Element create/delete, and ElementRevision vs attachment-only commits.
- Attachment mutations, permissions, convergence, and Facet-consumed typed evidence ports.
- Core private Store/commit form, FormatID, and corresponding validation.

No unlisted G1 behavior may change. This contract wholly replaces Facet's free-string type/lifecycle predecessor; Realm replaces only section 7's scope profile/codec.

## 1. Sole deliverable claims

```text
Core-only Element
  = spatial record + FacetAttachment.NONE
  = a fully valid, persistable UCUPA Element independent of Facet

Optional Facet
  = per-Element proposal -> acceptance -> ACTIVE attachment
  + exact typed records guarded by PRESENT + ACTIVE(self)

Local composition
  = contact a restricted typed port through the relation's specified NextHop, then verify expected FactAuthorityRef
  != bare-Ref resolver, global directory, or authority-wide fixed peer
```

Prove all of the following together:

- Core and Facet are independent fact owners, each with stable FactAuthorityRef, Store, ReadCut, and commit domain.
- Core retains each Element's FacetAttachment. Effective attachment contacts include Facet identity/specified address; actual typed payload stays only in Facet, not copied into attachment.
- Facet may be entirely absent without failing ordinary Core behavior.
- With Facet enabled, only the `ACTIVE(self)` owner established by matching proposal/acceptance may serve.
- One Core authority may attach different Elements to different Facet authorities; one Facet authority may accept Elements from different Cores.
- Realm mechanically carries D7's authority-qualified scopes, adding neither domain semantics nor fact ownership.
- Element spatial records/Facet payload retain no Endpoint, Cluster, Store path, or authority-wide peer pair. Cross-service addresses belong only to exact attachment-control relationships; unattached Elements prefill no address.

## 2. Sole owner boundaries

| Fact/control fact | Sole owner | Explicitly not owned |
|---|---|---|
| Element existence, Kind, Structure, Pose, Range, ElementRevision | Core | Facet payload, upper-layer type interpretation, permissions, Route |
| Per-Element FacetAttachment, pending proposal, AttachmentRevision, forward NextHop/contact revision | Core | Facet acceptance/record, deployment resources |
| Proposal-bound immutable acceptance | Facet | Core attachment decision/existence |
| Reverse Core NextHop/contact revision by AcceptanceKey | Facet | Core existence, current-owner selection, rewriting original acceptance |
| `FacetKey -> opaque payload + FacetRevision`, FacetReadCut | Facet | Element spatial truth, business type meaning, TypeRef interpretation |
| Caller identity, PermissionBinding, Allow/Deny | Realm | Core/Facet facts, Route |
| Local owner ports assembled by specified address | Composition | Authoritative contacts, synthetic cuts, owner selection |

Composition may host Core, Facet, and Realm in one process without merging owners, Revisions, Stores, or transactions.

## 3. Local value profile

### 3.1 `FactAuthorityRef`

```text
FactAuthorityRef = canonical lowercase UUIDv4, a distinct named type
```

- Use the [Core mount identity profile](core-mount-contract.md#21-one-stable-core-identity-without-an-authority-object-layer), without the old 256-bit FactAuthorityRef branch; opaque 32-byte FacetTypeRef is unaffected.
- Each logical Core/Facet has one stable self Ref, not an internal Authority object layer. New/CreateDurable securely generates/persists it once; caller-selected old Refs are forbidden and generation failure fails creation. OpenDurable restores only the original value; missing, invalid, or inconsistent reads cannot generate a replacement.
- Use exact equality only. Ref encodes no kind, path, Realm, Cluster, Endpoint, credential, time, or ordering.
- Typed owner ports/local authority evidence distinguish Core-kind from Facet-kind without two underlying ID families.
- A Store has one self Ref. Peer identities are retained only with exact mount/attachment relationships; no `LocalPeerCoreAuthorityRef`, `LocalPeerFacetAuthorityRef`, or globally fixed authority pair.
- Normal reopening/machine/address changes preserve self Ref. Private token scope remains independent and unexported. Local profile freezes no public wire/authentication evidence; equal Ref does not disprove writable clones, and writer handoff is not guaranteed by this slice.
- Ref bytes prove neither writer, permission, nor reachability. This contract trusts explicitly injected local typed ports.

### 3.2 `FacetTypeRef` and opaque Payload

```text
FacetTypeRef = any opaque value of exactly 32 bytes
FacetKey     = (ElementUUID, FacetTypeRef)

FacetAccessContext = AcceptanceKey {
  FacetAuthorityRef
  CoreAuthorityRef
  ElementUUID
  ProposalRevision
}
```

- Ref supports only exact equality/raw-byte ordering, with no displayName, slot, version suffix, URL, registry number, or digest meaning; it is no address/name/version.
- Go zero value/empty bytes are invalid; constructors/accessors copy bytes.
- [Public-profile section 8](public-access-and-observation.md#8-public-values-opaque-content-and-canonical-encoding), revised in P7.3a, directly supersedes old type encoding/validation entries. Facet neither fetches/parses artifacts nor validates schemas. Put retains bounded, byte-preserving opaque Payload by (Element, TypeRef); Get/ListTypes return current-key content/type sets only.
- Payload remains at most 1 MiB; each Element permits at most 256 current FacetTypeRefs. Oversized writes return `CAPACITY` before any fact mutation.
- `FacetAccessContext` selects a relationship obtained from Core exact read, not a persistent FacetKey/global locator/caller-supplied existence proof. It contains only AcceptanceKey; address is neither key nor identity. The caller obtains FacetNextHop from that ACTIVE attachment. Facet verifies self, loads its retained CoreContact by exact AcceptanceKey, fresh-reads expected Core at the specified address, and verifies current attachment. CoreContact contacts only the originally confirmed Core, never infers/reselects current owner for the caller or resolves a port by CoreRef alone.

## 4. Core's sole exact snapshot

Core exact read returns at one CoreReadCut:

```text
CoreElementRead =
  PRESENT {
    CoreAuthorityRef
    CoreReadCut
    ElementRecord {
      UUID / Kind / Structure / Pose / Range / ElementRevision
    }
    FacetAttachment
  }
| NOT_FOUND {
    CoreAuthorityRef
    CoreReadCut
    ElementUUID
  }
```

There is no second `exists` bool, lifecycle Store, or cache. A minimum-disclosure Facet port may project `CoreAuthorityRef + ElementUUID + existence + FacetAttachment + CoreReadCut` from this same snapshot, without independently storing/evolving it. That port uses an explicit PermissionID distinct from full Element Read and discloses no `ElementRecord` to the Facet service Principal.

### 4.1 FacetAttachment

```text
FacetAttachment =
  NONE {
    AttachmentRevision
  }
| ACTIVE {
    AttachmentRevision
    FacetAuthorityRef
    ProposalRevision
    FacetNextHop
    NextHopRevision
  }
| DETACHED {
    AttachmentRevision
    FormerFacetAuthorityRef
    FormerProposalRevision
  }
```

Within Element/Core self context, `ACTIVE` target identity and ProposalRevision uniquely reconstruct the following key; address/contact revision do not enter key equality:

```text
AcceptanceKey =
  (FacetAuthorityRef, CoreAuthorityRef, ElementUUID, ProposalRevision)
```

Core copies no Facet `AcceptedAt`. Former AcceptanceKey plus current Core cut in `DETACHED` already proves "formerly ACTIVE, now detached." Successor continuity evidence belongs to D12 transitions, not new attachment fields here.

- Element creation and initial `NONE` share one Core commit; no observable PRESENT without initialized attachment.
- AttachmentRevision is an independent, non-reused equality token under `(CoreAuthorityRef, ElementUUID)`.
- Propose/cancel/activate/detach advance AttachmentRevision/CoreReadCut, not ElementRevision.
- Structure/Pose/Range updates advance ElementRevision/CoreReadCut, not AttachmentRevision, target, or accepted proposal.
- Delete terminates current existence/attachment effectiveness in one Core commit without deleting Facet bytes across Stores.
- ACTIVE returns the relationship's authoritative FacetNextHop/contact revision, not payload, Cluster, Facet-record existence, connection cache, or extra RouteRegistration.
- Pending and ACTIVE share one exact forward contact. Activate does not rebuild its address from an old Propose receipt. Cancel clears pending contact; Detach/Delete ends forward-contact effectiveness. Historical receipts revive no relationship.

## 5. Minimum attachment handshake

### 5.1 Proposal and acceptance

```text
FacetAttachmentProposal {
  CoreAuthorityRef
  ElementUUID
  TargetFacetAuthorityRef
  ProposalRevision       # AttachmentRevision after the propose commit
}

FacetAttachmentAcceptance {
  FacetAuthorityRef      # responding self
  CoreAuthorityRef
  ElementUUID
  ProposalRevision
  AcceptedAt             # FacetReadCut
}

Core FacetContact[AcceptanceKey] = FacetNextHop + NextHopRevision  # pending/ACTIVE
Facet CoreContact[AcceptanceKey] = CoreNextHop + NextHopRevision # maintained independently of acceptance

AcceptAttachment(exact proposal, InitialCoreNextHop, actor, OperationID)
```

Propose explicitly takes target FacetRef and initial FacetNextHop, persisting proposal/forward contact in one Core commit. The authorized first Accept caller supplies InitialCoreNextHop explicitly: a bootstrap address for known Core before Facet has a relationship, not part of AcceptanceKey. After verifying expected Core at that address and reading exact current proposal, Facet creates CoreContact/contact token in the same first-acceptance commit. Missing, wrong, or unreachable addresses imply no Core absence.

`AcceptanceKey` uniquely identifies acceptance, immutable and durably retained. This contract permits no deletion, overwrite, or compression of a committed acceptance. `AcceptedAt` already supplies the local fact cut; no duplicate AcceptanceRevision. `ProposalRevision` is the semantic name of post-propose `AttachmentRevision`, not a second token. Propose returns its creation CoreReadCut in the mutation result; no duplicated, uninformative `ProposedAt`.

Repeated Accept converges as follows:

- First acceptance of an exact still-current proposal: `COMMITTED`, one FacetReadCut advance, one `AcceptedAt`.
- Existing exact AcceptanceKey: different OperationID returns `UNCHANGED` with original acceptance/no cut advance; original-ID replay returns original `COMMITTED`.
- No acceptance, and fresh read finds proposal canceled/replaced: `REJECTED`, no commit.
- An in-flight Accept that read valid PENDING may commit historical acceptance after Core Cancel. Independent Stores do not forbid that interleaving. Core Activate rechecks its current proposal and cannot revive cancellation.

The sole decision order is authorization → retained OperationID lookup → exact AcceptanceKey lookup. Existing acceptance returns as above without requiring current proposal. Only absence releases the Facet lock for expected Core fresh read at exact InitialCoreNextHop; after relocking, recheck local key absence, lineage, and evidence bindings before first commit. Concurrent first acceptance returns that same acceptance, overwriting no CoreContact. Repeated acceptance never changes addresses: InitialCoreNextHop initializes once, then section 5.4 maintenance applies. Original-ID binding includes original bootstrap address; replay reads no new port and changes neither inputs nor contacts. Historical acceptance's `UNCHANGED` proves no current proposal/attachment effectiveness; Activate still verifies its pending proposal.

### 5.2 Sole state machine

```text
PRESENT + NONE + no pending proposal
  -- Propose(target, initial FacetNextHop, expected AttachmentRevision) --> NONE + PENDING(exact proposal)

PENDING(proposal)
  -- target Facet Accept(exact proposal) -----------> immutable acceptance
  -- Core Activate(exact acceptance) --------------> ACTIVE(target, proposal revision)
  -- Core Cancel(expected proposal revision) -------> NONE + no pending proposal

ACTIVE(target)
  -- Detach(expected AttachmentRevision) -----------> DETACHED(former target)

any PRESENT state
  -- DeleteElement ---------------------------------> NOT_FOUND
```

Rules:

1. Pending proposal is Core-owned control fact, not a fourth ordinary attachment state; only authorized handshake callers read it.
2. Propose may target any valid FacetAuthorityRef, not necessarily a Store-global peer.
3. First Facet Accept checks expected Core at explicit bootstrap address, rereads exact current proposal, and verifies target=self. Existing AcceptanceKey follows section 5.1 convergence.
4. Core Activate reads exact acceptance at current pending FacetContact's address and verifies Facet identity. Calls run outside Core locks/transactions; after relocking, recheck expected proposal revision, commit, and retain current forward contact. Address changes replace no proposal and do not automatically invalidate legitimate already-sent requests.
5. Acceptance is not activation. Facet serves only after observing Core `PRESENT + ACTIVE(self, same AcceptanceKey)`.
6. Post-Cancel acceptance may remain invisible history but revive no proposal. ProposalRevision is never reused.
7. DETACHED is terminal within this contract; reattachment, owner replacement, content adoption, and fencing belong to D12 successors.
8. ACTIVE namespaces may have no records; ListTypes returns definite complete empty.
9. Within this contract, one ElementUUID's acceptance lineage binds one CoreAuthorityRef; another Core's proposal yields `SUBJECT_AUTHORITY_CONFLICT`. Different ProposalRevisions from one Core may leave immutable history, but only current ACTIVE AcceptanceKey serves. D12 successors alone enable cross-Core rebinding.

### 5.3 Relationship-specified addresses, not bare-Ref resolution

Follow the [Core mount contact boundary](core-mount-contract.md#22-mount-records-specify-the-next-hop-connections-only-deliver). Deployment supplies stable Core/Facet entries reachable by intended callers; relationship owners explicitly choose one. Local NextHop shares `local:<name>`; hosts assemble restricted ports by address with BindEndpoint/UnbindEndpoint:

```text
ConnectCore(NextHop, expected CoreAuthorityRef)   -> typed local Core port | UNAVAILABLE
ConnectFacet(NextHop, expected FacetAuthorityRef) -> typed local Facet port | UNAVAILABLE
```

Address is contact input; Ref is target verification only. No bare-Ref resolver, global directory, UUID-based owner guessing, target switching after failure, or automatic fallback address. The caller explicitly holds its first Core connection. First Accept's authorized caller supplies/verifies the Core bootstrap address; established relationships use their current contacts.

The normal spatial-discovery-to-Facet chain is:

```text
Read Element from the original entry along a verified path (use specified addresses at Core mounts)
→ ACTIVE FacetAttachment at the same Core cut: target Ref + FacetNextHop
→ connect to Facet at the specified address, verify expected identity, authorize independently
→ Facet reads its local CoreContact by AcceptanceKey
→ fresh-read the original Core at specified CoreNextHop; verify existence + ACTIVE + matching acceptance
→ return independent Core/Facet cuts and typed payload or a valid absence result
```

Facet's fresh Core read need not reuse discovery's old Core cut and must report actual evidence cuts; old discovery/new Facet content is not a global atomic snapshot. Facet interprets neither TypeRef nor Payload and retains no type-resolution port. That is a separate boundary from removed fact-Ref service-address resolution.

Only the record-owning side authorizes address changes. Directional addressing is authoritative, not an ignorable hint. Different relationships may use different entries for one authority; Refs need no attached address or sole global address. Unreachability is never NOT_FOUND. Connection objects/reachability caches rebuild; persistent contacts do not disappear. This remains trusted local typed-port assembly with per-call kind/identity/exact-relationship verification, not network authentication through string equality. Remote transport/authentication is separately verified; unified RouteRegistration/referral/lease is no mandatory successor prerequisite.

### 5.4 Local maintenance of bidirectional contacts

NextHopRevision is a private local equality token binding issuing scope, relationship kind, exact AcceptanceKey, and issuance cut. It follows Core mount contact rules but mount tokens cannot authorize attachments; no new random ID. Relationships share no mutable CoreRef-to-address table or authority-wide peer topology.

```text
Core.UpdateFacetNextHop(actor, OperationID, AcceptanceKey, expected NextHopRevision, NextHop)
Facet.UpdateCoreNextHop(actor, OperationID, AcceptanceKey, expected NextHopRevision, NextHop)
  -> status + original NextHop + NextHopRevision + owner ReadCut
```

- Existing Core pending reads and ACTIVE exact reads expose current forward contact/token. Facet.ReadAcceptance's independently authorized envelope additionally exposes CoreContact/current FacetReadCut. This is not immutable acceptance; Activate retains only original acceptance/AcceptedAt as immutable evidence.
- Updates require local authorization, exact relationship/contact CAS, and one local commit. No online peer, other-Store read, mount ControlSlot, Range coordination, or Element write-gate constraint.
- Core maintains exact pending/ACTIVE contacts only; Cancel clears pending, Detach/Delete ends effectiveness. As long as Facet retains acceptance it retains/allows authorized maintenance of CoreContact for checking original Core invalidation. Updating a historical reverse address activates no namespace; business calls still check current Core.
- Real address changes advance only local NextHopRevision/ReadCut, not AttachmentRevision, ElementRevision, FacetRevision, ProposalRevision, AcceptanceKey, AcceptedAt, owner, or payload.
- Propose/Accept initial addresses initialize once. Late messages, old success receipts, duplicate Accept, or Activate cannot overwrite current addresses with them. Original-ID input/results stay fixed after contact repair.
- Valid same-address updates are UNCHANGED, with no token/cut. Exact replay returns original receipt without changing current address. Only a new unbound retry uses new input; a bound operation cannot change address while claiming the same intent.
- New control sends use current relationship address; legitimate old-address in-flight requests may finish under identity/relationship/operation rules. Peer source address is no identity credential; responses never update local contacts automatically. Address changes neither ban old connections nor fence writers.
- Restart restores existing contacts from each Store; hosts rebind corresponding transport objects only. Directions commit independently without automatic network-wide refresh, multi-address failover, migration, or disconnected pathfinding.

## 6. Minimum Facet truth and behavior

```text
FacetKey(ElementUUID, FacetTypeRef)
  -> opaque Payload + FacetRevision

Facet-private control:
  (no per-Type control facts; content, tombstones, and receipts persist in one transaction)
```

Facet persists no per-Type pin, artifact mirror, or witness. TypeRef is only part of FacetKey, neither a second type identity nor a contract-drift constraint. Putting the same Ref after deleting its last record follows ordinary key rules without a retained pin.

Add only these owner behaviors:

| Operation | Minimum result |
|---|---|
| `AcceptAttachment` | Exact immutable acceptance + AcceptedAt FacetReadCut |
| `ReadAcceptance` | Exact PRESENT/NOT_FOUND at FacetReadCut; PRESENT envelope independently carries current CoreContact without changing acceptance |
| `UpdateCoreNextHop` | Local contact CAS, COMMITTED/UNCHANGED, original token/cut; no payload/acceptance change |
| `Get` | FacetAccessContext + exact TypeRef; PRESENT/KEY_NOT_FOUND with FacetRevision, FacetReadCut, Core attachment evidence |
| `ListTypes` | All current FacetTypeRefs in context's ACTIVE namespace, raw-byte ordered; complete empty allowed |
| `Put` | FacetAccessContext + bounded opaque payload + absent/revision CAS + OperationID |
| `Delete` | FacetAccessContext + exact-key expected FacetRevision + OperationID; does not delete Element |

Every fresh Get/ListTypes/Put/Delete obtains retained acceptance/CoreContact by context's exact AcceptanceKey, contacts its fixed expected Core at CoreNextHop, obtains explicit CoreReadCut, and verifies:

```text
Element == PRESENT
FacetAttachment == ACTIVE(self FacetAuthorityRef, same AcceptanceKey)
```

Facet independently verifies that exact immutable acceptance exists/matches in its own Store. Missing, corrupt, or mismatched evidence yields only `UNAVAILABLE`. Get/ListTypes carry `AcceptedAt` evidence; Put/Delete bindings/results also bind it by value, never serving on Core target Ref alone.

Fresh Get/ListTypes accept no caller-supplied old Core cut, lifecycle evidence, or arbitrary substitute address. Read current Core snapshot through the saved exact CoreContact. Facet neither scans acceptances/guesses owner by UUID nor treats contact as a pointer for selecting current owner. To avoid mixed fact/address cuts, local reads use one bounded validation:

```text
1. Complete independent authorization; under the Facet lock capture F0, exact immutable acceptance, and CoreContact/NextHopRevision, then unlock
2. Without verifiable acceptance/contact evidence return UNAVAILABLE, not guessed Core existence; contact expected Core at F0's specified address and obtain @ C
3. Stage the Core existence/attachment decision and evidence; disclose no state-dependent result yet
4. Reenter the Facet lock; if current FacetReadCut != F0, return FACET_SNAPSHOT_CHANGED
5. Verify exact acceptance in F0; if Core is not matching ACTIVE, return the corresponding Core-owned outcome; otherwise return record/KEY_NOT_FOUND/complete TypeRef set
```

Steps 4–5 share one Facet snapshot/lock boundary. Address changes advance FacetReadCut, so an F0 capture cannot switch to latest address. No automatic endpoint switch, cut refresh, or unbounded retry. An in-flight read completing step 2 before detach/delete may still return old Core evidence, but no row created by a later Put after step 2.

Distinguish results:

- `ELEMENT_NOT_FOUND`: Core proves absence at exact cut; Core evidence only, no invented Facet cut.
- `NAMESPACE_NONE`: Core proves NONE; Core evidence only, no invented Facet cut.
- `KEY_NOT_FOUND`: exact key absent from ACTIVE(self); requires F0/matching acceptance.
- `NOT_CURRENT_OWNER`: ACTIVE target is not responding Facet.
- `NAMESPACE_DETACHED`: Core proves DETACHED at exact cut; publicly unavailable with evidence.
- `UNAVAILABLE`: specified connection, expected identity, Core/Store/current-attachment evidence, or retained contact unverifiable.
- `FACET_SNAPSHOT_CHANGED`: Facet cut changed during Core check; no key-absence claim.
- `DENIED`: authorization failed before fact disclosure.

`ACTIVE(other)` returns `NOT_CURRENT_OWNER + Core evidence`; `DETACHED` returns `NAMESPACE_DETACHED + Core evidence`; inability to establish Core cut returns only `UNAVAILABLE`. ACTIVE complete-empty/`KEY_NOT_FOUND` requires F0/matching acceptance, not Core evidence alone.

Put/Delete likewise capture exact acceptance/current CoreContact from one source before Core calls, never another key's address. Unlike reads, later address changes alone invalidate no already obtained legitimate ACTIVE evidence: relock/recheck exact local relationship/original mutation preconditions, commit against actually obtained Core cut, and rewrite no contact. Mutation acceptance binds the exact read attachment revision and returns its evidence. There is no shared Revision/cross-Store linearization point. After Put verifies `ACTIVE@C`, Core may detach/delete before Facet commits `COMMITTED + C`; that confirmed write is not retroactively altered but ceases to be current connected-space payload. Every fresh Get establishing current Core read after detach/delete hides it. In-flight pre-detach/delete Get returns only through F0 stability. Physical orphan cleanup is excluded; hidden bytes cannot reappear through UUID reuse.

## 7. Authority-qualified permissions

The exact resource scopes are:

```text
CoreElementScope(CoreAuthorityRef, ElementUUID)
FacetElementScope(FacetAuthorityRef, ElementUUID)
FacetKeyScope(FacetAuthorityRef, ElementUUID, FacetTypeRef)
RealmAdministrationScope(RealmID)
```

Owner qualification prevents a new owner automatically accepting old Grants after an actual authority transition; Realm does not become a fact owner. Attachment handshake, namespace enumeration, and exact-key mutation use each owner's explicit PermissionIDs.

Contact maintenance independently authorizes on the local side, inheriting no peer user/service identity:

| Operation | PermissionID | Scope |
|---|---|---|
| Core.UpdateFacetNextHop | `ucupa.core.element.facet-contact-update` | CoreElementScope(self, ElementUUID) |
| Facet.UpdateCoreNextHop | `ucupa.facet.element.core-contact-update` | FacetElementScope(self, ElementUUID) |

Read contacts only through that side's existing exact attachment/handshake permissions; possessing an address/Ref grants no payload access. First Accept's bootstrap address still requires independent Core caller establishment and proposal-read permission.

Phase 1 already supplied `CoreElementScope(CoreAuthorityRef, ElementUUID)`; reuse it. The remaining Facet increment directly replaces string `FacetScope`, corresponding Realm scope codec, persisted PermissionBinding/retained-outcome encoding, and Realm Store FormatID in one Greenfield compilation slice. Realm Principal, credentials, binding aggregate CAS, authorization order, and owner-local revision semantics remain. Restore no ownerless Core scope, retain no ownerless Facet constructor, alternate reader/marker, or migration. Do not introduce an interim permission model known to inherit Grants incorrectly after transitions.

Every owner authorizes before state disclosure, OperationID lookup/replay, Core-port calls, and Store mutation. Composition calls another owner with an explicitly configured, authorized service PrincipalRef, not an implicitly reused end-caller permission.

## 8. Revisions, OperationID, and durability

- Core ElementRevision/AttachmentRevision/CoreReadCut, Facet FacetRevision/FacetReadCut, and directional NextHopRevisions are opaque owner-scoped equality tokens. No cross-type/owner/cut ordering.
- One local commit advances its ReadCut once and only the lineage tokens actually changed.
- Mutations use caller-proposed 1..128-byte OperationIDs. Retained binding includes at least actor, owner Ref, kind, subject, canonical inputs, precondition, and payload identity.
- Exact retries currently authorize first, then replay original status, Revision, ReadCut, and evidence without peer calls or another mutation. Initial Propose/Accept addresses and address-update inputs belong to their bindings. A different binding returns `OPERATION_CONFLICT`, never rewrites intent using a new address.
- Activate outcomes retain by value the entire immutable acceptance actually observed, including `AcceptedAt`, for lost-response replay; this copies no Facet cut into current FacetAttachment truth.
- Each owner has one serialized state-commit boundary, with identical memory/bbolt observable semantics. Ordinary bool Authorizer/peer calls run outside owner locks/transactions. P5's trusted local view final comparison participates in locked commit/disclosure ordering, fetches no remote Realm data, and never reenters the owner. The public contract defines independent identity/views.
- Core Store retains self CoreAuthorityRef, CoreReadCut high-water, Elements, attachments/proposals, forward contacts/revisions, tombstones, and outcomes. Facet Store retains self FacetAuthorityRef, FacetReadCut high-water, immutable acceptances, reverse contacts/revisions by AcceptanceKey, records, tombstones, and outcomes. P5 adds stable public-Revision HMAC keys/exact public-request bindings inside both existing Stores. Realm retains qualified PermissionBindings/outcomes. Each has its own new opaque FormatID.
- At initial publication, every commit, and reopening, Core validates that ACTIVE traces to a matching retained Activate outcome containing the same acceptance/AcceptedAt, and DETACHED traces to matching former ACTIVE/Detach. Facet validates AcceptedAt belongs to its authority's cut lineage. These causal proofs are not compressed within this contract. Fabricated ACTIVE/DETACHED, dangling keys, or impossible cuts fail closed.
- Contact initialization/update/cleanup persists with facts, tokens, cut, and original receipt in one local transaction. Reverse initialization and acceptance are simultaneous. Acceptance without valid contact cannot be reported serviceable or "relationship absent"; ReadAcceptance cannot hide this corruption.
- Core shares mount's sole local history rules, adding attachment/contact events. Shared commit/validation machinery does not require full mounts first. Facet also replays acceptance, contacts, and content history. Commit/reopen use the same deterministic rules for relationship keys, historical address results, tokens, and cleanup moments, not initial-address-only checks permitting arbitrary reopened addresses. Non-advancing rejection/unchanged receipts are validated too. No competing CoreReadCut or cross-Store event sequence.
- Each Store recognizes one exact FormatID; no second marker reader, converter, migration branch, or parallel API facade. Missing/unrecognized/corrupt structures uniformly fail closed with owner `ErrStore`, regardless of era.
- Lost responses, reopening, commit faults, and concurrent retries produce no duplicate commits, regressed tokens, dual ownership, or lost confirmed writes.

Core creation and Facet activation/Put are not atomic. Earliest observable order is:

```text
Core PRESENT + NONE
-> Core pending proposal
-> Facet acceptance
-> Core ACTIVE
-> optional Facet Put
```

Failure stops at already owner-committed state. OperationID converges retries, not rollback of another owner.

### 8.1 Closed failure semantics and decision order

All mutations share the prefix: pure syntax/shape/bounds/canonical-container validation requiring no port/fact state → known exact-input-scope authorization → OperationID lookup. Stop on `INVALID`, `DENIED`, original replay, or `OPERATION_CONFLICT` without fact-state reads. Thereafter return only the first applicable outcome in this order:

| Operation | Sole post-prefix decision order |
|---|---|
| Core `ProposeAttachment` | Element absent → `ELEMENT_NOT_FOUND`; expected AttachmentRevision mismatch → `ATTACHMENT_REVISION_CONFLICT`; pending/ACTIVE/DETACHED → `STATE_CONFLICT`; else commit PENDING |
| Core `CancelAttachment` | Element absent → `ELEMENT_NOT_FOUND`; expected ProposalRevision mismatch → `ATTACHMENT_REVISION_CONFLICT`; no exact pending → `PROPOSAL_NOT_CURRENT`; else commit NONE |
| Facet `AcceptAttachment` | Existing exact acceptance with valid contact → `UNCHANGED`, corruption → `UNAVAILABLE`; otherwise unverifiable bootstrap address/Core → `UNAVAILABLE`; read finds missing/canceled/mismatched Element/proposal → `PROPOSAL_NOT_CURRENT`; same UUID accepted from another Core → `SUBJECT_AUTHORITY_CONFLICT`; exact acceptance exists after relock → `UNCHANGED`; other local-lineage conflict → `SUBJECT_AUTHORITY_CONFLICT`; else atomic acceptance + CoreContact |
| Core `ActivateAttachment` | Element absent → `ELEMENT_NOT_FOUND`; expected ProposalRevision mismatch → `ATTACHMENT_REVISION_CONFLICT`; no exact pending → `PROPOSAL_NOT_CURRENT`; unverifiable address/Facet → `UNAVAILABLE`; exact acceptance absent → `ACCEPTANCE_NOT_FOUND`; proposal changes before relock → `ATTACHMENT_REVISION_CONFLICT`; else commit ACTIVE retaining current forward contact |
| Core `DetachAttachment` | Element absent → `ELEMENT_NOT_FOUND`; expected AttachmentRevision mismatch → `ATTACHMENT_REVISION_CONFLICT`; not ACTIVE → `STATE_CONFLICT`; else commit DETACHED |
| Core `UpdateFacetNextHop` | Element absent → `ELEMENT_NOT_FOUND`; no exact pending/ACTIVE relationship → `ATTACHMENT_CHANGED`; corrupt contact → `UNAVAILABLE`; contact CAS mismatch → `CONTACT_REVISION_CONFLICT`; same value → `UNCHANGED`; else local contact commit |
| Facet `UpdateCoreNextHop` | Exact acceptance absent → `ACCEPTANCE_NOT_FOUND`; corrupt contact → `UNAVAILABLE`; contact CAS mismatch → `CONTACT_REVISION_CONFLICT`; same value → `UNCHANGED`; else local contact commit |
| Facet `Put` | Unverifiable local acceptance/CoreContact → `UNAVAILABLE`; connection/identity/Core-evidence failure → `UNAVAILABLE`; map Core result below; record CAS mismatch → `FACET_REVISION_CONFLICT`; capacity exceeded → `CAPACITY`; exact equality → `UNCHANGED`; else commit |
| Facet `Delete` | Unverifiable local acceptance/CoreContact → `UNAVAILABLE`; connection/identity/Core-evidence failure → `UNAVAILABLE`; map Core result below; key absent → `KEY_NOT_FOUND`; expected FacetRevision mismatch → `FACET_REVISION_CONFLICT`; else commit |

Invalid context shape or `FacetAuthorityRef != self` is prefix `INVALID`. Missing exact acceptance/CoreContact or unverifiable address/identity/evidence yields only `UNAVAILABLE`, never invented Core absence. Verified fresh Core results map exactly:

- `NOT_FOUND -> ELEMENT_NOT_FOUND`.
- `NONE -> NAMESPACE_NONE`.
- `DETACHED -> NAMESPACE_DETACHED`.
- `ACTIVE(other FacetAuthorityRef) -> NOT_CURRENT_OWNER`.
- `ACTIVE(self)` with different ProposalRevision → `ATTACHMENT_CHANGED`.
- Core port/Store cannot establish cut → `UNAVAILABLE`.

Get/ListTypes stage these decisions, recheck F0 per section 6, and return only `FACET_SNAPSHOT_CHANGED` on change. Otherwise disclose Core-owned outcomes or PRESENT/`KEY_NOT_FOUND`/complete empty.

Retention is also closed:

- Mutations retain original status, DecisionCut/CommitCut, and obtained cross-owner evidence for `COMMITTED`, `UNCHANGED`, `ELEMENT_NOT_FOUND`, `ATTACHMENT_REVISION_CONFLICT`, `STATE_CONFLICT`, `PROPOSAL_NOT_CURRENT`, `ACCEPTANCE_NOT_FOUND`, `SUBJECT_AUTHORITY_CONFLICT`, `NAMESPACE_NONE`, `NAMESPACE_DETACHED`, `NOT_CURRENT_OWNER`, `ATTACHMENT_CHANGED`, `KEY_NOT_FOUND`, `FACET_REVISION_CONFLICT`, `CONTACT_REVISION_CONFLICT`, and `CAPACITY`.
- `INVALID`, `DENIED`, `OPERATION_CONFLICT`, `UNAVAILABLE`, and read-only `FACET_SNAPSHOT_CHANGED` create no new binding.
- A confirmed precommit local Store failure returns `UNAVAILABLE` without binding. Lost response/uncertain commit boundary returns unretained `INDETERMINATE`; identical-ID retries must converge to either unexecuted or a retained outcome.

New failure classes, fallback mappings for the same condition, or unauthorized-state-based precedence changes violate the contract. Go errors, wire codes, and DTO spelling may be decided later without merging observable distinctions.

## 9. Black-box completion gate

### 9.1 G1 regression and Core-only closure

- Without Facet, all G1 root/Anchor/Entity create/read/spatial-update/reanchor/reparent/delete/ListDirect/oracle/reopen/error/convergence tests pass unchanged.
- `ElementRecord`/`ListDirect` DTOs keep G1 form; ListDirect exposes no FacetAttachment, which only exact Element read returns.
- Attachment commits advance AttachmentRevision/CoreReadCut, not ElementRevision. Contact-only commits advance NextHopRevision/CoreReadCut, not AttachmentRevision. Spatial updates advance ElementRevision/CoreReadCut without changing AttachmentRevision/active target.
- Core-only creation returns `PRESENT + NONE` at one cut; no PRESENT view without initialized attachment.

### 9.2 AuthorityRef and attachment

- Core/Facet self Refs are nonzero, different, and stable after durable reopening; copying Ref bytes activates no second owner.
- Propose → accept → activate; Facet cannot serve between acceptance and activation.
- Wrong target/CoreRef, stale AttachmentRevision, or unavailable port produces no new effective attachment. If absent acceptance and fresh read prove canceled/replaced proposal, Accept makes no commit and returns exact rejection. Legitimate in-flight old reads/existing acceptance replay follow distinct rules below, not blanket rejection.
- Any mismatch in context self/CoreRef/ElementUUID/ProposalRevision permits no scanning/owner guessing. Another CoreAuthorityRef accepting the same UUID returns `SUBJECT_AUTHORITY_CONFLICT`.
- One-Core/many-Facets, one-Facet/many-Cores, and Core-only assemblies pass without global Store peer pairs.
- Pose/Range/Structure updates preserve ACTIVE target; fresh Facet reads after detach/delete reveal no payload.
- Exact acceptance commits once; duplicate Accept with another ID returns the same acceptance. Absent acceptance cannot be created after cancel without a valid old PENDING read. Such an in-flight read may commit historical acceptance, but never activate a canceled proposal. Test PENDING read → Cancel → Accept commit explicitly.
- Completely remove Ref-to-port lookup and still complete multi-Core/Facet assembly, first Accept, Activate, business recheck, and restart. Missing contacts permit no same-UUID/other-acceptance address guessing; wrong service yields unverifiable result, not owner reselection.
- Forward update after Propose then Activate, and reverse update after Accept then duplicate Accept preserve new addresses/original intent. Offline peers allow independent CAS; concurrent maintenance changes no payload, acceptance, target, or mount gate. Old-address retries after cancel restore no relationship.
- Inject lost responses, restart, and corruption at every durable contact-event boundary. Reject old tokens, wrong relationship/owner, and unauthorized maintenance.

### 9.3 Facet exact truth

- Arbitrary unfamiliar TypeRefs, non-JSON payload bytes, and capacity boundaries follow structure/capacity rules. Unrecognized bytes alone never reject payload; reads/writes/exact replay require no new external source.
- Get/ListTypes/exact replay return original bytes, with no parsing, normalization, or reencoding.
- ACTIVE empty ListTypes is complete empty; multiple types return complete raw-byte order without duplicate/silent truncation.
- Put absent/revision CAS, Get, Delete, replay, lost responses, and durable reopening preserve local cuts.
- Get/ListTypes return at F0 if Facet stays unchanged during Core checks. Late Put during checks returns `FACET_SNAPSHOT_CHANGED`, never new row plus old ACTIVE evidence.
- Reverse-address changes after F0 also return `FACET_SNAPSHOT_CHANGED`, not new-address continuation. Test missing contact/wrong identity/disconnect separately from proven absence; none becomes normal emptiness.
- Put reading `ACTIVE@C` before concurrent detach/delete may commit with old evidence; fresh Get then hides it. In-flight Get follows F0 stability or fails.
- ELEMENT_NOT_FOUND, NAMESPACE_NONE, NAMESPACE_DETACHED, KEY_NOT_FOUND, NOT_CURRENT_OWNER, ATTACHMENT_CHANGED, FACET_SNAPSHOT_CHANGED, UNAVAILABLE, and DENIED remain distinct.
- Facet serializes concurrent mutations. Race tests are mandatory; unavailable tooling means incomplete verification, not a skip reported passing.

### 9.4 Greenfield removal evidence

Static/black-box tests jointly establish absence of the following in this Facet increment. Scope exclusions here do not remove already delivered Core capabilities:

- `FacetType string`, second FacetKey/scope profile, extra FormatID reader, converter, or migration branch.
- `LocalPeer*AuthorityRef`, authority pairs, global Facet service, or network-wide UUID lookup.
- Metadata/displayName fields, APIs, Stores, permissions, or Revision semantics added by this increment.
- Addresses in Element spatial records/Facet payload, or Cluster/Store paths in facts.
- Bare FactAuthorityRef-to-address/port resolution, mutable authority-wide peers, or fallback-owner search.
- Public BatchGet, change log/Observe, Reconcile, mount, federation, transition, or production mechanisms.
- Core importing Facet, Facet importing Core, or Composition becoming a fact owner.

Realm gates also cover validity, canonical codecs, binding administration, replay, durable reopen, and corruption for three qualified scopes; no unqualified scope or second Realm FormatID reader.

Delivery follows the [development plan's full verification requirements](development-plan.md#7-verification-and-change-rules) through:

```text
go run ./cmd/verify
```

## 10. Explicit exclusions

- Metadata, displayName, or generic descriptive fields/read-write APIs.
- Public FactAuthorityRef/FacetTypeRef wire encoding, AuthorityEvidence, or contract distribution.
- SpaceReference, SpaceHandle, StructuralAddress, Enter, Discover, Observe, or public Propose.
- Network address encoding, cross-machine transport, TLS/Noise/security handshakes, multiple addresses, or automatic contact publication.
- Mount delegation/acceptance, recursive query, partial/complete source proofs.
- Reattachment, Core/Facet owner replacement, writer fencing, or authority transition.
- Batch, change retention, GAP, physical Reconcile, indexes, workers, backups, or production deployment.

Successors separately prove non-federated public behavior, public contact encoding/target checks, and cross-organization transport/authentication/mount federation; the plan schedules actual dependencies. Former Ref-to-RouteRegistration/referral/lease is not mandatory later work and cannot silently restore another resolver behind local ports.

## 11. Acceptance and effect

The original contract, shared contact principles, and clauses were finalized before explicit Start authorization. On 2026-09-07, the user requested acceptance and commit; scope review and a repeated passing full gate completed P4 acceptance. It and the separate commit take effect only for:

1. This current local Core/Facet increment contract. G1 still owns D1–D4 `ElementRecord`, create/spatial-update/reanchor/reparent/delete, ElementRevision, ListDirect, spatial CAS/errors/convergence, and geometry oracle, not a wholesale move to historical. Supersede/extend only Core self profile, exact-read envelope, atomic create/delete attachment effects, attachment-only commits, qualified scopes, attachment/typed ports, and Store/commit families. G1 notices align these exact boundaries individually.
2. Manifest/affected notices: Facet predecessors become historical. Realm retains identity, credentials, bindings, and delivered Core-qualified scopes; notices state only replacement of remaining Facet scope profile/codec.
3. Update the development plan to mark only this local Facet lifecycle accepted. The sole plan chooses next action from other capability status, not automatic Stage 1/Baseline completion.
4. Retain one explicit contract owner per current fact, no parallel API, second Store reader, migration, or compatibility authority.

## 12. Accepted P4 implementation mapping

At P4 delivery, Core attachment control was located in `ucupa-core/attachment_*.go`. Facet public values/dependencies were in `facet.go`, acceptance/contact in `attachment.go`, content in `content.go`, deterministic decisions in `mutation.go`, state/evidence in `state.go`, and persistence/transactions/history validation in `store*.go`. These describe the historical owner-kernel organization, not a new owner or compatibility facade.

Core's actual cut projects through `CoreReadCut.Evidence()` to a shared local opaque value carried by Composition on Facet's narrow consumer evidence port. It is no independently authenticating source credential. P4 assembly explicitly injected a TypeContract resolver/validator; P5 replaced it with candidate artifact sources/owner validation; P7.3a removed it entirely. Facet injects/calls no type source now. Actual delivery follows the plan, not dual type readers.

Only specified behavior is allowed. Private Store markers recognize current format only. Committed business rejection/unchanged retains original outcomes without advancing cut; corrupt projections/history are never automatically repaired. Only the development plan maintains delivery, final evidence, and next action. This acceptance authorizes no later Phase.
