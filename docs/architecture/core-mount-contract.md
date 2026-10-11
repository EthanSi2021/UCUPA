# Minimal two-Core mount implementation contract

**Status: P2 mount control and P3 paths/combined queries are implemented, fully verified, and accepted by the user. The architecture authority manifest declares their bounded, delivered local scope current. The development plan records code delivery and commits; acceptance does not expand this into a network, nested-mount, or multi-writer contract.**

**Successor notice:** The [P6 network contract](network-access-contract.md) owns public network federation. The [P7 contract](owner-transition-contract.md) owns same-original-Store writer handoff, the first supported Core transfers, and simple carrier replacement within their stated limits; uncontinued outgoing external edges must still be safely rejected before source withdrawal. The [RealmAttachment contract](realm-attachment-contract.md) owns each current owner's persistent authorization contact and network verification. Spatial, mount, query, and permission clauses not expressly superseded remain effective. These increments must not be recorded retroactively as capabilities already delivered in P2/P3.

The user accepted structural access through mounts, with mount records specifying both stable Core identity and the next-hop address rather than requiring separate Ref-to-address resolution. Changing a mount Anchor's Range may pause all new Element fact writes in the carrier's space. Disconnection may prolong that pause until recovery; no fixed completion time is promised. The user first authorized contract revision, then separately authorized Phase 1's shared minimum foundation. After verification and confirmation that it met commit conditions, that foundation was committed separately. The user then authorized P2: sections 1–4 mount control and section 5's necessary omission-prevention restrictions on existing local methods. P2 implementation and full verification were reviewed and separately committed with authorization covering only that scope. P3 paths/combined queries were subsequently implemented and fully verified under separate authorization, accepted under the agreed design, and separately committed. Section 5's bounded two-Core reads thereby took effect. Only the [development plan](development-plan.md) maintains current execution status.

This contract inherits ownership and authorization principles from [mainline D6–D10](ucupa-concept-mainline-baseline.md). Shared contact rules are aligned with D8 and the [Facet attachment contract](facet-local-truth-and-core-lifecycle.md). Section 6 explains the implementation increment; the new direction is not presented as unchanged predecessor Ref routing. Geometry follows [local spatial truth](core-local-spatial-truth.md) and [local queries](core-local-query.md). This contract changes no foundational axiom and does not equate a same-machine experiment with complete open federation.

## 1. Two information layers, scope, and sole ownership

Distinguish Element spatial facts from mount contact information; do not turn Elements into deployment objects:

- **Spatial facts** identify an Element and its spatial existence/organization: identity, Structure, Pose, and Range. Root Anchors retain their existing special representation, without invented Parent or LocalPose.
- **Mount information** identifies the carrier below a cross-Core boundary and the entry used to access it. Mount-control records associate the target Core's stable identity with a specified next-hop address. Ordinary Elements do not repeat addresses. A "mount Anchor" is a role of an ordinary Anchor, not a new Element kind or duplicate spatial truth.

Structure determines the branch to access; the mount record supplies the cross-Core entry; the host/deployment layer makes it reachable. Machines, Pods, replicas, and Cluster topology stay outside the Element model; the spatial tree need not map to deployment hierarchy. Moving the same logical Core to another machine while preserving its entry requires no spatial or mount-address change. Changing the entry address for an existing target changes neither spatial facts nor carrier identity; spatial changes do not automatically change addresses. This separation does not remove coordination: changing the mount Anchor's Range still requires the two-sided containment checks and write coordination below.

The first acceptance scenario has two independent Cores, independent Stores, one mount edge, no nested mounts, and trusted, explicit in-process connections.

```text
Core A owns R, L, M
R (sole unbounded root)
├─ L ─ local Element
└─ M (ordinary Anchor; its Element record exists only in A)
   ├─ child Anchor ─ …   owned by Core B
   └─ Entity            owned by Core B
```

- A owns M's Element record. B carries its content, not M itself. B's direct content uses external M as Parent and local coordinate reference.
- B neither owns, copies, nor changes M and creates no replacement local root. Two existing independent roots cannot be joined by an assumed identity transform.
- A retains M's spatial facts and delegation control. B independently retains accepted content, acceptance control, its write gate, and local operation outcomes.
- Initial establishment permits only an empty ordinary Anchor in A and a B without Element claims.
- The parties share no Store, transaction, operation-result namespace, or cross-Core lock; each decides at its local commit boundary.
- "Two instances, one edge" bounds this slice, not UCUPA's permanent capacity. Reject unsupported nesting, replacement, and existing-content migration rather than accept a request and process only part.
- This slice adds no Facet capability, Observe, network transport, automatic discovery, Route module, global directory, replication, or writer handoff.

This slice provides neither retirement nor reattachment of mount content. Content remains after revocation and M cannot be ordinarily deleted; the complete mount lifecycle is not claimed.

## 2. Stable identity, connections, and permissions

### 2.1 One stable Core identity, without an Authority object layer

One Core in this slice corresponds to one logical fact owner responsible for its stored Elements. `FactAuthorityRef` is that Core's stable identity, not a set of Authority objects inside it. Retain this type name without parallel CoreID/CarrierID types. The local Go/Store profile uses canonical lowercase UUIDv4 in a distinct named type.

- `New`/`CreateDurable` generates it once with secure randomness. A caller cannot supply an old Ref; generation failure fails creation.
- `OpenDurable` restores only the stored Ref. Missing, malformed, or inconsistent reads are rejected, never repaired by generation.
- `Core.AuthorityRef()` exposes identity to trusted local assembly; A and B must differ.
- Normal reopening, connection-object replacement, or address changes preserve Ref. Identity is not a permission Principal, address, root UUID, or ElementID.
- Private token scope still independently limits Revision/CoreReadCut issuance and is not exported as Ref.
- P2 adds no durable writer epoch. P3 adds only run-local snapshot lifetime binding, not writer fencing. Equal Ref or scope does not prove the absence of cloned writers. Restored backups, writable clones, permanent Store loss, and cross-instance writer transfer are not guaranteed here.
- This encoding freezes only the local profile, not public network encoding or cryptographic AuthorityEvidence.

### 2.2 Mount records specify the next hop; connections only deliver

```text
A's mount record for M: CarrierAuthority = B's stable Ref, NextHop = specified B address
B's incoming record: ParentAuthority = A's stable Ref, NextHop = specified A address
```

Every serving Core that participates in mounts requires a stable connectable entry supplied by its host/deployment layer and usable within its intended callers' network reach. UCUPA neither allocates that address nor builds a Core-Ref address-resolution service. A next hop may be a stable service entry rather than an instance's temporary IP; this slice uses local addresses in the same role. "Stable" does not imply permanent availability, worldwide reachability, or immutable configuration; the target behind it must still match the expected Core identity.

Each record is authoritative for its relationship direction, not an optional hint: A owns the forward record and B the reverse record. Authority comes from the locally authorized mount-control write, not trust proven by an IP/string. Addresses exist only at cross-Core relationships, not on every ordinary Element. A alone still owns M's spatial record; its mount record specifies B as the carrier below it. There is no extra global Ref-to-address table or search for alternative targets by bare Ref after failure.

Local `NextHop` is a persistable address with profile `local:<name>`: name has 1–63 lowercase ASCII letters, digits, or hyphens, beginning with a letter or digit. No implicit alias, case conversion, or redirection; names are not derived from Ref. This same-machine format neither freezes future network addresses nor implements network transport. A trusted host uses `BindEndpoint(address NextHop, peer PeerPort)`/`UnbindEndpoint(address)` to assemble a specified address to a restricted local port. This rebuildable transport adapter calls by address only and accepts no bare-Ref addressing. PeerPort exposes no raw Store, unauthorized snapshot, or arbitrary write entry to ordinary callers.

1. The caller initially holds a known entry connection. Across a mount, obtain the specified address and expected Ref from the relationship's persistent record.
2. Each new control exchange reads the current specified address. Queries use addresses in their fixed cut, with changes handled by section 5. Missing bindings, misconnection, closure, or interruption explicitly fail: no automatic alternative address, global resolution, or empty-content result.
3. Each call checks established peer identity, expected target, source, exact relationship, and operation/read binding. The receiver validates within its own critical section; the sender validates response source and binding. Equal Ref alone is not remote authentication.
4. Peer calls, caller establishment, and Authorizer calls always run outside Core locks and Store transactions.
5. Trusted local assembly supplies initial trust. Cross-machine authentication/transport requires separate design; test ports may inject failure, duplication, and delay.
6. Runtime connection objects stay outside Store. Rebuilding binds explicit addresses only; it does not establish mounts, grant permissions, or restore revoked relationships. Address records are persistent control facts and must not disappear with an adapter/cache.

Control ports include at least Prepare (independent authorization of a new intent), Finish (verify a terminal decision and discharge an exact obligation), and ReadDecision (read only a known peer attempt's terminal decision). ReadDecision cannot replace public mount-read, enumerate other operations, or create recovery obligations from new requests. Read ports perform only section 5's authorized capture/cut checks.

Address answers "where to connect"; Ref answers "is this the expected Core." Neither replaces Realm authorization or spatial legality. Failure of the sole specified address removes reachability through that entry until explicit repair or recovery; automatic rerouting is not promised.

### 2.3 Separate address maintenance from spatial coordination

A initializes the forward address at establishment reservation; B initializes the reverse address at its first Prepare. Both bind the exact MountKey. Thereafter each side locally repairs its own record through `UpdateMountNextHop`, without requiring the peer online, occupying ControlSlot, or obeying B's Element write gate. An absent relationship cannot be created by an address update.

- Change only this direction's NextHop, never target Ref, MountKey, AdmissionKey, Element, write gate, or mount effectiveness. MountRevision/ElementRevision do not advance; an actual address change advances independent NextHopRevision and the owner's CoreReadCut.
- Addresses may be repaired in Establishing, Active, Revoking, Detached, and B's exact Prepared relationship. An unfinished operation cannot require range coordination to finish before contact repair. Updating an address does not reopen a revoked entry.
- Original Establish addresses initialize once. Control intent, OperationID binding, and original terminal outcome remain immutable. Retries/recovery send the original intent through the current specified address; late Prepare, Finish, responses, and exact replays never overwrite a newer address.
- Contact repair does not require repeating Prepare for a legitimately prepared Range. Decisions depend on exact intent, relationship, spatial state, and gate, not the peer's entire CoreReadCut remaining equal forever to its preparation cut. Section 5 separately governs P3 retention and contact invalidation.
- Already sent messages at old addresses may complete after validation against original identity, relationship, operation, and phase. Source address neither defines identity nor rejects legitimate settlement. Address updates are not old-connection bans, writer fencing, replication, migration, or carrier replacement.
- Directions commit independently without global synchronization. Authorized callers explicitly repair records that refer to changed deployment addresses. This slice adds no automatic publication, multiple addresses, failover, or network-wide refresh. If neither disconnected party can supply a new address, reachability is not promised to recover automatically.

### 2.4 Owner-qualified permissions and local callers

Phase 1 replaced Core's bare `ElementScope(UUID)` with the following reused scope:

```text
CoreElementScope(TargetCore FactAuthorityRef, ElementUUID)
```

Core constructs requirements from its own persistent Ref, not a caller-supplied owner. Realm mechanically encodes/decodes and compares the complete scope: no ownerless-grant fallback or permission inheritance.

`TargetCore` is the handling authority, not necessarily the UUID's Element owner. Reading M in A uses (A,M); accepting, listing, or querying content below M in B uses (B,M), without claiming B owns M.

Local call context explicitly supplies each owner's independently established PrincipalRef; Principals may differ. Trusted in-process call boundaries supply this context. It is neither freely self-reported network credentials nor permission for A's Allow to replace B's Authorizer. Missing peer caller context means not permitted, never a default to A's actor.

Realm still owns identity/credential establishment, permission binding, and Allow/Deny. Core defines requirements, check ordering, and consumption of decisions. "Independent caller establishment" describes each trust boundary, not a Core identity service. Persisted PrincipalRefs bind original intent and recovery evidence; Core copies no accounts, credentials, or permission tables. Range violations, CAS conflicts, closed write gates, and unavailable connections belong to Core/connection contracts, not Realm decisions.

Retain ordinary Core PermissionIDs and add:

| Permission | Target scope | Meaning |
|---|---|---|
| `ucupa.core.anchor.mount-control` | M in A | Propose establishment, change mount control/forward address, or revoke |
| `ucupa.core.anchor.mount-accept` | External M in B | Accept initial establishment/new range-coordination intent, or change this side's reverse address |
| `ucupa.core.anchor.mount-read` | M in the Core being read | Read disclosable relationship/control-operation state on that side |

A Range update additionally requires existing subject-update and final-parent-place permissions in A. Prior write permission does not automatically allow state reads. Existing Facet-to-Core grants/adapter tests add owner qualification mechanically, without new FacetAuthorityRef, attachment, or local-Facet lifecycle behavior.

## 3. Persistent facts, revision tokens, and Store validation

### 3.1 Exact keys and states

```text
MountKey = (ParentAuthority, MountAnchorUUID, ProposalRevision, CarrierAuthority)
AttemptKey = (ParentAuthority, OperationID)
AdmissionKey = (MountKey, OperationID that established the currently effective Range)

A:
  State = None | Establishing | Active | Revoking | Detached
  MountRevision
  MountKey? + CurrentAdmissionKey?
  OutgoingContact? = NextHop + NextHopRevision       # target Ref comes from MountKey
  ControlSlot = empty | OperationID
  MountOperations[OperationID] = exact request + decision + closure evidence

B:
  Incoming = empty | exact MountKey + immutable acceptance
  ParentContact? = NextHop + NextHopRevision         # Target Ref comes from MountKey
  Admission = exact AdmissionKey + Range + parent decision evidence
  Gate = Open | Preparing(AttemptKey) | Closed
  PeerAttempts[AttemptKey] = exact request + Prepared/Rejected/Closed + retained evidence
```

- `MountRevision` is a new control token with private issuing scope, M UUID, and local cut serial at issuance; zero is invalid. Changed control facts for the same M issue a new token; it is not Element Revision.
- MountRevision issued at establishment reservation is ProposalRevision, permanently identifying the MountKey generation; it cannot be reused after abort.
- `NextHopRevision` binds local private scope, relationship kind, exact MountKey, and address-issuance cut, without a new random identity. Facet attachments follow the same rule bound to AcceptanceKey. Tokens are not interchangeable; add no contact identity or second CoreReadCut. A valid same-address unchanged update issues no token. Independent address CAS prevents ordinary Element writes/range coordination from blocking maintenance. A current contact must belong to its registered relationship and is cleared with that relationship; old addresses/receipts remain non-revivable history. A cannot clear OutgoingContact upon merely retaining Abort; it must receive B's exact Closed and settle first. Once B persists Closed, it may clear the aborted incoming/ParentContact; late messages only replay the terminal outcome.
- AdmissionKey reuses original operation identity as the admission generation, not a random AdmissionID. Only establishment/Range commits replace it; Pose-only changes, addresses, ordinary B content updates, and aborts do not.
- Immutable acceptance binds MountKey, the initial prepared tuple/Range basis, and B's AcceptedAt cut. Subsequent gates, admission, and current effectiveness are separate facts and do not rewrite it.
- A's control OperationIDs share the ordinary mutation owner-local intent namespace. Reusing an ID with different kind, actor, peer actor, target, CAS, or canonical input returns ErrOperationConflict.
- B's peer attempts use (parent owner, operation ID), separate from B's application OperationIDs.
- Each address-update side uses its ordinary local mutation OperationID namespace, not a separate address-operation ID system.
- Closed/rejected records are permanent, including abort-before-prepare. Absence of pending state does not permit reopening an old attempt.
- A's None may retain historical MountRevision/proposal lineage; an absent relationship is not a fabricated active MountKey.

### 3.2 One CoreReadCut, extended commit history

Every local commit changing observable facts, admission, gates, relationships, or control progress advances the existing sole CoreReadCut. Only commits actually changing an Element record advance that Element's Revision. Exact replay of original success/rejection/UNCHANGED advances no cut; merely retaining a terminal rejection that changes no facts follows existing rules.

Phase 1 established an independent commit sequence, explicit commit positions, and removal of the hard `cutSerial == revisionSerial + 1` constraint. At that foundation stage, history implemented only Element mutations. This contract extends the same basis with control transitions/replay validation; adding Store tables while retaining an Element-only validator is insufficient.

Continue with one owner-local persistent commit sequence, covering:

```text
ElementCommitted
MountReserved
PeerPrepared / PeerRejected
MountDecided
PeerClosed
MountSettled
NextHopUpdated
```

- Every real transition has a unique consecutive cut; its event retains canonical inputs and applicable prior-state/authorization decisions and new-state basis.
- Commit and reopening use the same deterministic transition function. Authorization decisions are retained as external inputs at that time; reopening does not ask today's Realm to reinterpret history or claim the Store authenticates the historical permission service.
- Replay-validate every advancing event and unchanged/rejected outcome retained at that cut, not successful events alone.
- New ElementRevisions have independent counting. A control event may also change M's final tuple but must not issue a Revision for an unchanged Element.
- Each step persists facts, control state, cut, event, and original receipt in one local transaction before publishing memory state.
- Keep existing persist-failure reload rules: accept only complete previous or next state; otherwise the instance cannot continue serving. Indeterminate persistence returns an uncertain Store error, not confirmed abort.
- Replace the incremental Store marker once during implementation. Recognize one current format only: no old reader, compatibility, automatic migration, or fallback.
- Reopening and every persist check owner Ref and private scope together; event replay must reconstruct all current state and retained outcomes.

### 3.3 Local structural forms

| Form | Required conditions |
|---|---|
| Not yet in a space | No root, effective incoming, or Element claim; closed control attempts may remain |
| Rooted A | Sole immutable unbounded root; no incoming |
| External-parent B | No local root; one incoming; every local parent chain ends at that relationship's external M |

- B cannot CreateRoot while preparing acceptance; every local Create rejects UUID=M. M enters none of B's Anchor, Entity, or tombstone tables; reserving a boundary does not copy its record.
- Interpret `Parent=M` only through exact incoming context; every other Parent must be a local Anchor. Reject orphans, cycles, or a second independent space outside this declared external endpoint.
- B checks direct-child containment exactly against current Admission's finite Range; other parent-child pairs use existing local geometry.
- Once A reserves/activates M, it accepts no more local direct children under M.
- Detached B retains the last valid context for Store validation and authorized owner-local reads, but closes its write gate and does not present those records as current connected-space content.
- Local projection follows only proven frame chains; it cannot require a nonexistent M record in B or invent a root identity Pose.

### 3.4 UUID guarantee boundary

ElementUUID remains stable Element identity, not an identity silently changed to "owner + UUID." Producers must neither reissue UUIDs nor appropriate another Element's UUID; each owner permanently rejects UUIDs it has claimed.

Independent Stores perform no network-wide duplicate check and cannot promise precommit global uniqueness against arbitrary malicious caller inputs. If two owners claim the same ElementUUID within an authorized combined scope, return ErrIdentityConflict: do not merge, choose one, or reinterpret them as two valid Elements. Source + UUID in results only identifies the claim's source.

B always rejects collisions with external M locally; no collision check is claimed for spaces not yet read. Guaranteed pre-write rejection of every cross-owner collision would require additional claim coordination or a separately defined allocation model, neither silently implemented here.

## 4. Control calls and convergence

### 4.1 Applications submit intent, not Prepare/Commit steps

Use one tagged intent call rather than expose protocol steps to applications:

```text
ApplyMount(ctx, LocalCallers, OperationID, MountCommand) -> MountControlResult, error

MountCommand =
  Establish { M, ExpectedElementRevision, CarrierAuthority, CarrierNextHop, ParentNextHop }
  Update    { M, ExpectedElementRevision, ExpectedMountRevision, Final SpatialTuple }
  Revoke    { M, ExpectedMountRevision }

ReadMount(actor, M) -> MountRead, error
ReadMountOperation(actor, M, OperationID) -> MountControlResult, error
RecoverMounts(ctx) -> recovery report

UpdateMountNextHop(actor, OperationID, M, MountKey, ExpectedNextHopRevision, NextHop)
  -> NextHopUpdateResult { OperationID, MountKey, NextHop, NextHopRevision, Changed, Cut }, error
```

- Call `ApplyMount` in A. LocalCallers binds each owner's independently established Principal; initial establishment and Range Prepare require B's caller.
- Establishment/actual Range changes without an established B caller return zero result ErrDenied before A's persistent reservation: no OperationID binding or slot. Adding an actor later would change the original request and create unrecoverable Pending. A bound B actor refused by current B policy is instead an explicit rejection for which A can retain Abort, distinct from missing identity.
- Update accepts the complete final Parent/Pose/Range, requiring no potentially invalid intermediate state.
- The initiator explicitly supplies both Establish addresses, usable in A/B transport contexts respectively. B independently authorizes the initial ParentNextHop; it does not learn it automatically from network source or peer messages. A persists its forward address at reservation and B its reverse address at Prepare, not after Active, so mid-protocol restarts retain recovery evidence.
- Exact ApplyMount retries advance the original operation. Each call makes at most one forward phase traversal; unavailability or canceled waiting returns rather than loops indefinitely. Network deadline/wire profiles are outside this contract.
- Both Read calls are read-only, with no implicit protocol execution. MountRead carries State, MountRevision, disclosable MountKey/AdmissionKey, current Range basis, this side's NextHop/NextHopRevision, ControlSlot summary, and Cut; not full Pose/ancestor chain. An unknown operation returns ErrNotFound only after authorization.
- RecoverMounts is a restricted trusted-host entry: traverse existing unfinished operations once, accept no new intent, and bypass no new-Commit authorization. It adds no background daemon; applications may call it explicitly after reconnection/restart. Exact messages may also recover peer-known operations. A sends original Prepare/Finish via OutgoingContact; B reads A's original decision via ParentContact. Missing runtime address binding reports pending recovery, without bare-Ref lookup, discarding persistent addresses, or unlocking the gate.
- There is no second client Cancel. Explicit validation rejection before a durable terminal decision converges initial establishment/update to Abort. Disconnection, ctx completion, or stopping a wait leaves Pending, not automatic abort. A future active-withdrawal need requires a separate contract.

Address updates run locally in A or B, whichever owns the contact, with section 2.4 permissions; A's permission cannot edit B's record. Perform canonical validation, current authorization, and original OperationID lookup, then verify exact relationship and NextHopRevision at the local commit boundary. Denial, invalid shape, and conflicting intent reserve no new ID. Authorized relationship absence/mismatch and revision conflicts retain explicit rejection; success/unchanged retain original receipts. Rejection returns zero result and its error. Exact retries reauthorize, then replay the original result/error without replacing old receipt addresses with current ones. Actual updates atomically persist address, token, cut, and outcome via NextHopUpdated; unchanged results create no cut. Indeterminate local Store outcome returns ErrStore and requires original-ID read/retry, not a claim that no update occurred. ReadMountOperation reads only ApplyMount operations; obtain address-update outcomes by exact retry and current addresses through ReadMount.

### 4.2 Results are not ordinary MutationResult

```text
MountControlResult {
  OperationID, MountAnchorUUID, MountKey?             # exact binding
  Decision: Pending | Committed | Aborted
  Settled: bool
  DecisionCut?                                      # A's original decision cut
  Record?: ElementRecord                            # original update result, not latest
  Changed: bool
  AbortCause?: Denied | NotFound | Conflict | RevisionConflict
  BlockedBy?: Unavailable | OwnerMismatch | PeerStore
}
```

- Pending is unsettled with no DecisionCut; it does not claim no commit is in flight.
- Committed/Aborted terminal decisions are immutable. Settled=false means B's closure and A's settlement are not both confirmed.
- Committed does not mean the relationship remains active; replay cannot revive a later revocation.
- Aborted contains no new Element record; unfinished abort settlement may still hold a gate/control slot.
- A valid Range/tuple-unchanged Update is Committed, Changed=false, Settled=true, returning original record/cut without issuing ElementRevision or pausing B merely because the new API was called.
- Range-unchanged but Pose/Parent-changing Update checks Element/Mount CAS and an empty slot, then commits locally atomically: Committed, Changed=true, Settled=true, advancing ElementRevision without replacing AdmissionKey.
- Invalid shape, current-call denial, and OperationID-binding conflict return zero result plus error without revealing resource state.
- Authorized pre-reservation NotFound, kind/structure/geometry Conflict, or RevisionConflict retains Aborted, Settled=true and the then-cut without messaging B. ErrMountBusy admits nothing and occupies no OperationID.
- If A can confirm its original durable state, return a nonzero result. BlockedBy represents peer unavailability, never invented Abort. Explicit peer Prepare rejection leads A to Abort; establishment/Range change cannot commit without matching Prepared.
- If A cannot confirm its Store outcome, return zero result ErrStore. Recover through same-ID read/retry, not a new ID treated as "uncommitted."

### 4.3 Establishment

| Step | Local atomic change | Required rechecks |
|---|---|---|
| A reserves | None → Establishing; issue ProposalRevision; save intent/control slot | M non-root, CAS, empty direct content, no unresolved relationship; freeze M tuple and local child admission |
| B prepares | Save immutable acceptance and Prepared; close write gate | Independent authorization, exact identity/relationship/request; no root, Element claim, or conflicting incoming |
| A decides | Matching preparation: Active + Commit; otherwise Abort | Reauthorize original actor; M Revision, empty Anchor, reservation phase, and B-confirmed tuple/Range still match |
| B closes | Commit installs Admission/opens gate; Abort clears ineffective incoming | Discharge only exact A terminal decision; save Closed and reject late Prepare |
| A settles | Save closure proof/release slot; Abort returns to None | Peer has durably closed this exact attempt; no reuse of old completion receipts |

A Active while B has not installed Admission is not a complete empty space. A Abort before B's closure confirmation must not restore local child admission under M prematurely.

### 4.4 Updating the mount Anchor's final tuple

| Step | Local atomic change | Required rechecks |
|---|---|---|
| A reserves | Register intent while Active, occupy control slot, freeze M tuple and new revocation | Initial Element/Mount CAS, relationship, and local legality of the final tuple |
| B prepares | Verify direct-content coverage, save Prepared, close all new fact-write gates for this space | Current MountKey, old AdmissionKey, exact request; validation and gate closure in one commit critical section |
| A decides | New M + new AdmissionKey + Commit, or Abort alone | Reauthorization; M CAS, exact reservation state, parent containment, acyclicity, matching B proof |
| B closes | Commit installs new Admission; Abort retains old Admission; record Closed/open gate | Exact attempt/terminal match; a late proof cannot reopen an aborted attempt |
| A settles | Save B completion proof/release slot | Subsequent Range operation or revocation must wait |

Reservation itself advances MountRevision, so decision validates this operation's exact reservation state. Comparing initial ExpectedMountRevision against the token advanced by that same operation would manufacture a conflict.

If Range is unchanged and no operation is pending, Update can handle Pose/Parent atomically in A without B Prepare. A's other branches remain writable subject to their independent structure/containment rules. Ordinary UpdateAnchor handles mounted M only under this condition; Range changes explicitly return ErrMountCoordinationRequired, not evade coordination through A's empty local child set. This error and ErrMountBusy are non-admission errors, reserving no OperationID; callers may reuse that ID for the correct ApplyMount intent. With a pending slot, an ordinary new M Update returns ErrMountBusy; exact historical receipts still replay after authorization. Applications submit every actual spatial change; no automatic enlargement or writeback.

### 4.5 Revocation

Revocation is a unilateral decision, not Range Prepare:

1. A checks permission, Active, relationship CAS, and no occupied slot, then commits irrevocable Commit, Revoking, control occupancy, and closure of its connected-space entry together.
2. B durably closes its write gate and records Closed/Detached from the accepted relationship and exact terminal decision; it does not decide anew whether A may revoke.
3. A receives closure proof, becomes Detached/Settled, and releases the slot.

With B disconnected, A can close its own entry but cannot claim B stopped local writes. An unsettled Range attempt makes revocation ErrMountBusy; recover that attempt first. Revoking/revoked coverage is unavailable, not an ignorable empty set.

### 4.6 Authorization, replay, and recovery order

- New public intent: stateless canonical validation → current A authorization → OperationID lookup → local-state decision. After an external call, recheck exact state in the local critical section.
- B independently authorizes first Prepare. Prepared creates a durable obligation to discharge the exact operation's terminal decision. Subsequent closure requires no new-write permission from the original user; otherwise revocation could permanently prevent settling committed Range changes.
- Before a new Commit, A rechecks original actor permission; failure lets restricted recovery persist Abort. A currently unauthorized user call still returns ErrDenied without disclosing/replacing that durable terminal decision.
- Recovery advances only an existing exact intent; old authorization, connections, or acceptance cannot create new business operations.
- Ordinary B mutation: authorization → exact original receipt lookup → write gate → existing CAS/geometry decision. Closed gate returns ErrMountBusy for new requests without occupying OperationID; old receipts replay after authorization.
- "Ordinary Element updates trigger no range coordination" does not mean writes remain possible during the pause.
- Never call the peer or Realm while holding local locks. Preparation, decisions, gate closure/opening, and outcomes publish together at local commit boundaries.
- No timeout-based B unlock, Abort-to-Commit reinterpretation, lost-response rollback, or old Closed proof closing a new operation.
- B closes only the gate owned by that attempt. Check exact Closed/Rejected history first; a closed attempt only replays without changing current state. Abort for an attempt never Prepared adds a closure record only, clearing no other incoming/gate. A Commit mismatching current Prepared is rejected. Revoke Finish matches the current accepted relationship to close it without inventing Range Prepared.
- A recovery uses its retained original actor, reauthorizing a new Pending Commit; denial produces Abort. B reads A decisions only for attempts it knows and discharges obligations without selecting another caller.
- The bool Authorizer in this profile proves independent check moments, not cross-Store atomic authorization/commit or revoke-and-regrant ABA detection.

## 5. Reads, paths, and combined cuts

**P3 is implemented, verified, and accepted. This section defines upward depth, complete in-scope discovery, optional retention, paths, composition, and failures; implementation evidence is in the development plan.** P2's owner-local omission-prevention rules remain effective. Revising reads does not redo sections 1–4's persistent control state machine. The [local-query contract](core-local-query.md) alone defines local query/snapshot lifecycle; this section adds paths and two-Core composition.

### 5.1 Minimum public read increment

```text
ResolvePath(ctx, LocalCallers, Path, at *ConnectedCut) -> PathRead, error
ListDirectPath(ctx, LocalCallers, Path, at *ConnectedCut, limit, continuation)
  -> ConnectedDirectPage, error
QueryRegionPath(ctx, LocalCallers, PathRegionQuery, at *ConnectedCut, limit, continuation)
  -> ConnectedQueryPage, error
ReleaseConnectedCut(ctx, LocalCallers, ConnectedCut) -> error

Path = { OriginAnchorID, Up: uint64, Down: []ElementID }
PathRegionQuery = { StartPath, UpDepth: int64, BasisPath, Sphere, Selector }
```

Path starts at a known Origin Anchor in the receiving Core, traverses Up actual parent edges, then follows Down UUIDs edge by edge. Up counts **actual** parent edges in an address; crossing the root is invalid, not saturated. Query UpDepth is semantic selection: positive values ascend at most that many levels; -1 reaches the proven true root without a level limit; below -1 is invalid, and 0/omission selects Start. Path.Up and ActualUpDepth remain nonnegative actual counts, never -1. Intermediate nodes must be Anchors; the terminal may be Entity. Empty Down with Up=0 means Origin. No network-wide bare UUID/Ref locator or separate routing; a query's StartPath and BasisPath must share Origin.

Origin normally belongs to the receiving Core. The sole external entry is M accepted by B: first contact A through ParentContact and verify identity and current exact mount effectiveness. Thus empty B or B holding only direct Entities can enter without a replacement root. A B child Anchor's parent edge can reach A's M and its ancestors; service crossings add no level, and A alone owns M. The receiving Core is not a spatial boundary: entry through B cannot forbid required upward paths or turn B into an independent root. This profile crosses neither independent roots nor nested mounts and does not silently erase a future SpaceHandle boundary.

QueryRegionPath resolves Start, selects Scope upward, and searches Scope and all descendants in one selected structure view. Basis only interprets the sphere and must lie in Scope, not necessarily Start's subtree. Return Scope and its actual path; siblings, ancestor-direct Entities, and Scope itself participate under the same rules. Positive depth stops at the proven root; -1 also requires the true root, not the receiving Core boundary. A future explicit SpaceHandle boundary limits -1 to that declared boundary, never across independent roots. Disconnection, denial, and non-acceptance are not roots; failed necessary upward traversal cannot return a truncated successful Scope. Complete concerns only Scope's closure, not absence outside it.

```text
Source = { Authority: FactAuthorityRef, Cut: CoreReadCut }
Match = { ElementID, Kind, Revision, Path, Source }
ConnectedQueryPage = {
  Matches, Cut: ConnectedCut, ReadMode: QueryReadMode, ResolvedScopePath, ActualUpDepth,
  Status, Gap, Continuation?
}
ConnectedDirectPage = { RecordsWithSource, Cut: ConnectedCut, ReadMode: QueryReadMode, Status, Gap, Continuation? }
PathRead = Present { Record, Source, Path, Cut, ReadMode }
        | NotFound { VerifiedPrefix, AbsenceSource, Cut, ReadMode }
```

Match.Path is relative to **resolved Scope**, excluding Scope itself. ResolvedScopePath addresses Scope from original Origin: ascending from StartPath removes the Down suffix first, merging remaining ascent into Up, rather than adding everything to original Up. Verify every input edge. Valid Up then Down may return to the original branch—for example, B's S up to R and down to its Entity. Accept such discovery-derived addresses, normalizing to the shortest tree path within the selected cut, not a new latest read. Traversing the same mount twice does not add a second source. Canonical paths bind/compare requests; check count overflow rather than infer adjacency from strings.

After discovery, use the original entry and **append Match.Path's downward chain to ResolvedScopePath** for ResolvePath at original ConnectedCut. Record ID/Kind/Revision/Source must match; exact-read permission is separate. Do not append to original StartPath or find an address through Source.Authority. M appears once as A's record. Source.Cut is ownership evidence, not a retained-history handle; no shortcut passes it alone to ordinary ReadElement for history.

ListDirectPath returns full direct-child records only; M's direct content comes from B, without listing M again. Only the party responsible for adjacency in the specified view proves NotFound; connection failure is not absence. Query returns summaries without requiring unknown matches to have full-record read grants beforehand.

Even a B-entry query entirely inside B's subtree must verify A's current exact mount and independently obtain (A,M) mount-read, not whole-A query permission. A's revocation must prevent connected-space success while B still has old state. Reads not requiring that guarantee explicitly use owner-local methods, never silently downgrade after failure.

**P2's omission-prevention gate remains:** owner-local ListDirect at remotely carried M, QueryRegion whose actual Scope closure includes unresolved coverage, or depth resolution across an external parent returns zero result ErrMountContextRequired. Active/Establishing/Revoking/Detached are not empty adjacency. B's local ListDirect, QueryRegion, and ProjectToAncestor targeting external M also need mount context. ReadElement(M) proves only that B does not own M, not that M is absent from space. Fully closed ordinary owner-local reads do not establish current effectiveness of the whole connection.

### 5.2 Initial combined capture, fixed mode thereafter, no shared locks

ConnectedCut is private receiving-Core run-local read context. It binds original entry, read scope, each caller, fixed ReadMode, each source's CoreReadCut/run binding, exact relationship/Admission/contact revisions, and coverage. Only Retained additionally binds source snapshot handles. CurrentVerified metadata retains no old Elements, adjacency, geometry, or complete match array. It is not WorldRevision, permission evidence, a public wire token, or a durable query Store.

#### 5.2.1 Select mode before resource admission

Each owner's optional SnapshotRetention follows the local-query contract: absence disables retention; explicit finite positive duration enables it; no default; fixed for the run. Establish actual sources and mode before first disclosure:

- Retained requires all actual sources enabled; any disabled actual source makes the whole context CurrentVerified. Sources needed for ascent, Basis, mount effectiveness, or other proofs count even without output Elements.
- Excluded sources contribute nothing. Unauthorized/unverified sources prove no capability and must not be labeled "unconfigured." Failure to obtain a necessary source's proof fails the request; an ordinary descendant may instead be an explicit coverage gap.
- Capability differs from capacity. Full capacity, refusal, or disconnection at an enabled necessary source cannot silently downgrade mode. If source/configuration proof changes before capture, fail and clean up rather than replace the source set/mode within the call. CurrentVerified uses temporary current views everywhere; even an enabled owner leaves no spatial pin.
- Before mode selection, an unclosed ordinary descendant may be Excluded. Once sources/mode are selected, failure of any selected source in first-page capture fails/cleans up by cause, without removing it, reselecting mode, or looping capture. Insufficient retention/borrow allowance at any selected source is ErrSnapshotCapacity, including nonessential descendants; later pages likewise remove no source for capacity.
- Every page exposes the same ReadMode bound in ConnectedCut/continuation. Retained cannot become CurrentVerified after expiry; CurrentVerified cannot upgrade, re-pin, or add a recovered Excluded source. Changed configuration/run binding requires restarting, not interpreting an old Cut under post-restart configuration.

With no external-effectiveness dependency, apply the same rule to one source. With the sole mount edge, capture as follows. A/B always mean parent/carrier, not entry order. Mode probing obtains only authorized configuration/source candidates, not spatial consistency proof; final capture must match them.

#### 5.2.2 Capture and per-call validation

1. Authorize A outside locks; capture cA's immutable temporary view under a short read lock. Retained also admits/retains it.
2. Contact B at cA's specified address and verify identity. B independently establishes caller/authorizes, then captures cB under a short read lock; admit retention only for Retained. No cross-Core lock or shared database transaction.
3. Exact MountKey, effective AdmissionKey, and Range must match. M's Pose/Parent and ancestors come from cA; B content/local structure comes from cB, never latest mixtures. Admission's old ElementRevision does not force Pose-only updates to stop; check coverage and projection bases separately.
4. For initial Retained capture, recheck A is readable and current=cA before sealing sources. B capture is bracketed by unchanged A, so B need not still be current=cB then. Proven A cut change cleans up and returns ErrReadCutExpired. Unreachable A is ErrUnavailable and unreadable local Store is ErrStore, not invented expiry.
5. Resolve Start/UpDepth/Scope/Basis, compute, and sort in selected views. Any preliminary permission-target resolution must agree with the final authorized exact Scope; otherwise clean up and return zero result ErrReadCutExpired for caller restart. No internal Scope/cut retries or silent authorization expansion/narrowing.
6. Before disclosure, each owner reauthorizes outside locks, then checks local readability, run/context validity, and section 5.3 relationship/contact gates. Retained checks every required pin's lifetime; CurrentVerified checks current=cB then current=cA, or only its own source if no B. Any proven change returns zero result ErrReadCutExpired; disconnection/denial follows section 5.5.

CurrentVerified uses step 6 for first/later pages and same-Cut Resolve/List. Later calls reauthorize original scope, capture temporary current source views, and require exact fixed cA/cB. After computation, recheck B→A; no new cuts or rescue through an enabled owner's old pin. Keep temporary views alive through computation/disclosure checks, then release. B→A checks are independent moments, not a promise all sources remain current at return; non-wrapping cuts within one run establish that the used combination existed together. Source restart invalidates run binding even with the same persisted cut.

Entry through B first independently authorizes incoming/ParentContact candidates, then follows the A/B capture/check protocol. Final B contact/relationship must match the candidate, without automatic redirection. A records obtained for ascent disclose only authorized necessary prefixes, not everything contacted by the coordinator.

This is neither a global timestamp, cross-Store atomic transaction, nor "latest at return." A writes during initial Retained capture may require restart; after sealing, ordinary writes do not expire old pages. CurrentVerified source changes require a new query. A's committed new Range without matching B Admission cannot establish a new combination. Sealed Retained may still use old Range/B content, subject to the current relationship-effectiveness gate.

Fix sources and coverage before the first page. Closed means verified and included in the fixed cut; only Retained keeps its view across calls. Excluded is an unclosed opaque gap. Later pages add no source, replace no cut/pin, and upgrade no Excluded source; use only Closed sources within original Cut scope. Unclosed necessary Scope/Basis/ascent publishes no incomplete ConnectedCut. Expansion requires a new context.

The creating operation defines ConnectedCut's reusable read scope. Owner maps, whether temporary or retained, are not public whole-owner read access:

| Creating operation | Reusable fact-read scope |
|---|---|
| QueryRegionPath | Actual Scope and all descendants; only verified sources readable; Excluded remains a gap |
| ListDirectPath | Target Anchor and direct-child records/adjacency, not arbitrary descendants |
| ResolvePath | Verified path prefix and terminal; NotFound also fixes the parent-adjacency basis of absence |

Extra ancestor/mount prefixes serve interpretation/effectiveness only, without expanding content scope. Out-of-scope at is ErrInvalid; required in-scope Excluded/absent sources yield ErrUnavailable, not NotFound or fresh capture elsewhere. Exact match reads after Query are valid; resolving only M cannot use an existing A pin to add B and list its content at that Cut.

### 5.3 Fixed facts, not frozen permissions or current connection effectiveness

Every page, exact path read, and direct listing rechecks this call's LocalCallers and owner permissions. Snapshots fix spatial facts and their composition basis, not Allow; closed mount entries cannot disclose historical content through an old snapshot.

| Change after first page | Sealed-query behavior |
|---|---|
| Ordinary create/update/reparent/delete; M Pose/Parent; valid Range/Admission update | Retained uses old views/Scope; CurrentVerified returns ErrReadCutExpired on any confirmed source-cut change; new queries read new commits |
| A no longer Active or exact MountKey differs | Current connection gate closes; necessary context fails; ordinary descendants follow gap rules, not B's old Active |
| Confirmed change in a depended-on specified NextHopRevision | ConnectedCut explicitly expires with ErrReadCutExpired; underlying fact snapshot is not rewritten |
| Fixed address unreachable, preventing current contact evidence from its owner | Unavailability/descendant gap; do not guess address change or snapshot expiry |
| Retained pin expiry/release; either mode's run ends or combined metadata is released/lost | Whole-page ErrReadCutExpired; same Ref/cut or latest cannot revive context |
| Permission revocation | No disclosure; necessary Scope/Basis gives ErrDenied; ordinary unknown descendants may terminate Partial |
| Specified address reaches wrong Core | Reject its data; same-named address is not the original source and changes no fact ownership |

**Accepted tradeoff:** only Retained continues old pagination across ordinary writes/Range updates. Revocation closes current connected-space entry; address revision invalidates connected queries depending on that old directional contact. Faults/maintenance may require a new query, not a global Ref resolver. Each page obtains current mount effectiveness from A rather than trusting B's old acceptance. Each required directional contact revision is verified by its owner, not guessed by transport cache. Disclosure checks are independent boundary moments, not atomic Realm/A/B revocation across Stores. Revocation/expiry may occur after final checks; receipt-time permission/unexpired state is not guaranteed and returned old pages are not retroactively rewritten.

Use the whole-disclosable-Scope authorization profile:

| Operation | Local/A requirements | Independent B requirements |
|---|---|---|
| QueryRegionPath | Actual Scope query-region; exact-read for necessary entry/ascent prefixes outside Scope | (B,M) query if Scope includes M content; (B,S) query if Scope is S inside B |
| ResolvePath | Necessary path prefixes/terminal exact-read; mount entry additionally mount-read | B path/terminal exact-read; external M adjacency may use (B,M) list-direct |
| ListDirectPath | Necessary prefix exact-read; target Anchor list-direct; mount entry additionally mount-read | (B,M) list-direct for target M; (B,target) list-direct for a target inside B |

- Ascent does not expand grants. Denied final Scope fails wholly, not by lowering depth to answer another question. Private preliminary resolution constructs authorization only and reveals no ancestors, cut, or existence. Unresolvable paths check known-prefix disclosure permissions; unauthorized prefixes disclose no NotFound/Conflict/mount detail. Local Scope-resolution diagnostics follow exact Start's local-query failure gate; cross-owner necessary prefixes require that owner's exact-read/mount-read first. Uniform ErrDenied prevents topology leaks through error classification.
- For Scope=M, A owns M: query requires (A,M) and (B,M), listing requires each side's corresponding list permission, and ResolvePath(M) also needs A exact-read. A mount-read never replaces content authorization.
- For smaller S inside B, whole-A/B query permission is unnecessary; independently verify A's necessary effectiveness/prefix.
- Scope query permits local closure summaries/internal geometry inputs, not another Core's grants. Internal Basis resolution does not call public ResolvePath with an extra full-record requirement.
- Later same-Cut behavior uses this operation's permissions, without additionally requiring the creating query/list grant. After query revocation, valid necessary exact-read may still read the original record; original query never replaces exact-read. A fixed source contributing no content to this operation is checked only for caller/handle run binding, cut/pin, and current contact effectiveness, disclosing no records/adjacency. This is not content permission and cannot add sources, expand scope, or restore Excluded/terminal state.
- Unauthorized unknown B descendants may be opaque gaps. B required for Start/ascent/Scope/Basis is necessary context, not an ordinary descendant gap. M's authorized A disclosure is not automatically hidden by B denial.
- The bool port proves no revoke/regrant ABA. This slice adds no global privileged coordinator, hidden-branch authorization views, or remote authentication. Old snapshots cannot establish Facet's current existence/attachment authorization; its lifecycle is not rewritten here.

### 5.4 Lifecycle, geometry, and resources

Retained owners keep immutable views for their explicit durations: fixed TTL, no renewal, finite admission/concurrency. Each local monotonic clock decides expiry; expiry of any necessary pin invalidates context, whose lifetime cannot exceed its sources. No global time comparison. Keep snapshots after final page for same-Cut reads; explicit release attempts source-pin cleanup, with disconnected owners' TTL as fallback.

CurrentVerified retains bounded combined metadata only for mode/scope binding and terminal-Partial markers, not spatial pins or an implicit metadata TTL derived from absent SnapshotRetention. Bound count and bytes. Capacity pressure may evict unborrowed CurrentVerified metadata, explicitly expiring old Cuts. Still-insufficient capacity returns zero result ErrSnapshotCapacity, without unbounded growth or evicting promised Retained contexts. Evicted tokens/at cannot reconstruct registration to bypass terminal markers. Unchanged current is necessary for continuation, not indefinite availability. Same-Cut reads after the final page remain possible until run end, release, metadata loss/eviction, or cut change.

Bound combined metadata and concurrent temporary borrows in both modes; source allowance does not permit unlimited contexts. ReleaseConnectedCut is idempotent for the original caller's legitimate binding, including expired/released contexts; zero/wrong bindings fail. Invalidate disclosure first, reclaim after safe borrow exit; CurrentVerified clears only metadata/temporary borrows. Cancellation/failure cleans up undelivered captures; expired-but-borrowed state still counts. No long locked waiting or interference with other queries/fact writes. Neither mode restores run context on restart; disabling retention disables neither resource checks nor per-call consistency.

- Full scan, exact filter, canonical UUID sort; mount-Range pruning is not an initial correctness prerequisite.
- Closed-sphere intersection/tangency; selectors filter output only. Count the M-to-B-direct-child parent edge once.
- Valid UpDepth never limits downward closure. The complete proof chain's 1024 edges/65536 rational bits do not reset across Cores.
- Necessary Scope/Basis context over limit gives ErrUnrepresentable; individual candidates over limit create whole-query ExactWorkLimit.
- Rounded center plus original radius is not a safe exclusion sphere; centers lacking finite output Pose can still match exactly.
- limit=1..256 caps output only, not scan, sort, CPU, or latency. Resource limits provide explicit refusal/bounded retention, not a complete execution-budget system or spatial depth.
- Views retain no full commit/receipt history, snapshot Store, long bbolt transaction, or cross-Core lock. P2 durable journal/recovery validation remains.

### 5.5 Completeness, failures, and continuation

Gap summarizes the whole query/ConnectedCut using ExactWorkLimit and UnresolvedCoverage bits. Numerically unresolved branches and disconnected/unauthorized descendants within selected Scope cannot be treated as empty; outside Scope is no gap. Disclose no unauthorized source identities, counts, or topology.

| Condition | Result |
|---|---|
| Necessary entry/Start ascent/Scope/Basis source or current relationship cannot be verified | Zero result ErrUnavailable; lack of permission ErrDenied; unreadable local Store ErrStore |
| Authorized path proven absent/query terminal not Anchor | ErrNotFound/ErrConflict; no existence disclosure without permission and no absence inferred from disconnect |
| Basis non-Anchor, absent, or outside actual Scope | Uniform ErrInvalid after authorization, without finer probing |
| Ordinary B descendant unready, denied, unverifiable, or unreachable before mode selection | Excluded; other verified entries + Partial/UnresolvedCoverage; use no unauthorized B content |
| Selected source unreachable, denied, or mismatched during first-page capture after mode selection | Whole-call ErrUnavailable/ErrDenied, or ErrReadCutExpired for proven cut change; no removal/mode reselection |
| Initial cut recheck proves change; CurrentVerified cut changed; run context/Retained pin expired; depended-on contact revision changed | Zero result ErrReadCutExpired; never mix latest into original chain |
| Local/selected-source retention admission, metadata, or borrow allowance insufficient | Zero result ErrSnapshotCapacity; no source removal/mode change; promised handles/TTL unchanged |
| Actual source has retention unconfigured but original cut remains verifiable | Normal CurrentVerified query/pagination, no error or Gap |
| Previously contributing nonessential B fails permission/current-effectiveness checks on continuation | Discard all this page's B-dependent entries/evidence; after A checks return terminal Partial without continuation |
| Contributing B Retained pin explicitly expired/lost, or CurrentVerified B cut confirmed changed | Whole-page ErrReadCutExpired; recoverable latest is not the original source |
| Correct source proves missing adjacency in a valid view | PathRead.NotFound; no other-branch search for the UUID |
| UUID conflict in authorized combination | ErrIdentityConflict; no merge or address-based choice |
| Token shape/caller/query/scope binding mismatch | ErrInvalid; resource-dependent checks follow authorization; non-nil error carries no staged data |

ConnectedCut scope cannot expand. First-page at is optional but requires original scope/sources; continuation at must be nil, with fixed context from the continuation. Bind Origin/canonical paths, Start, UpDepth, Basis, resolved Scope/ActualUpDepth, sphere/selector, callers, ReadMode, source cuts/runs/optional handles, exact relationship/contact for current connection gates, coverage, and previous-page last UUID. Limits may change; depth changes start a new chain. -1 and a positive depth reaching the same Scope remain distinct continuation bindings.

- Recovered first-page Excluded descendants never enter the old chain; start anew to complete it, or smaller-UUID new matches would be missed.
- Updates rewrite neither mode's fixed Scope/path/order/Gap: Retained uses old views; CurrentVerified rejects changed cuts. New creates/deletes/reparenting cannot enter the old chain.
- Complete requires no gap and no next page. Further pages mean Partial, possibly with no Gap; numeric/coverage gaps persist on the final page. Complete describes the accumulated union; only an empty Complete first page proves no Scope matches.
- Terminal Partial ends the original chain without claiming completeness; previous pages remain then-valid observations. After sealing, retain original source registration/mode rather than remove B and reselect. Run-local context must retain terminal/unclosed markers so replays, old page Cut, and at cannot bypass them. A formerly Closed B losing disclosure evidence cannot rejoin via at even if pin/current cut remains valid; start a new query. Markers are memory state, not Store facts; concurrent borrows check them before disclosure so terminated sources cannot still support Complete. Invalid root context fails, not Partial.
- Error pages are zero results. Restart discards the original accumulated chain as a complete-set interpretation; never join old/new snapshots. No automatic latest retry, address change, TTL renewal, or durable read-handle recovery.
- Valid, currently permitted token replays are stable and must not be counted twice. Splitting one semantic query across two Cores must preserve result set, Scope, structural depth, and same-view ordering against the single-Core control.

## 6. Explicit current increments and excluded work

| Pre-Phase-1 assumption | Delivered foundation and contract increment |
|---|---|
| Nonempty Core always has local root | Rooted or explicitly accepted external-parent form; still one connected-space root |
| No public owner identity | Phase 1 stable FactAuthorityRef per Core, no object layer/token-scope reuse |
| Core scope only ElementUUID | Phase 1 CoreElementScope(TargetCore, ElementUUID) and Realm codec/comparison |
| Cut corresponds one-to-one with Element commit | Phase 1 independent sequence; P2 control events/replay use the same CoreReadCut |
| Children all in local Store | Local or exactly accepted remote coverage; unreachable is not empty |
| QueryPage contains only current Core cut | P3 revises QueryRegion directly for upward depth/optional finite snapshots; paths combine explicit sources without compatibility entries |
| Update checks only local children | Active mount Range coordinates; ordinary Pose-only remains local |
| No mount contacts | Independent directional NextHop, address CAS, durable events without disrupting spatial coordination |

Retain D6's stable owner, two-sided confirmation, external parent, separate admission/projection, and sole carrier. Mount addressing explicitly becomes "structure → specified address → verify Core," replacing bare-Ref resolution prerequisites.

The user confirmed shared contact rules, aligned with the following mainline clauses. Document alignment alone is not implementation evidence for mount/query code or target APIs:

| Affected clause | Aligned shared contact rule and Core boundary |
|---|---|
| D4 excludes Route/Endpoint | Element spatial records remain address-free; mount controls retain authoritative NextHop, not disposable cache |
| D6 identity/delegation | One stable Ref per Core, no Authority layer; MountKey fixes both owners; directional addresses independent of ownership/tuple |
| Former D8 Ref-resolved RouteRegistration and shared mandatory Core/Facet Route contract | Known relationships supply directional next hops with identity/relationship verification; no bare-Ref resolver/global directory/fallback |
| D9 entry/path | Explicit initial connection; forward mount access/reverse checks use relationship addresses, not global UUID/Ref lookup |
| D10 sources/post-discovery reads | Source labels owner/cut, not addressing; original entry, full path, and ConnectedCut read discoveries |
| D11 reachability separate from fact cuts | Runtime reachability remains separate; relationship-specified address is persistent control fact and changes advance local cut |
| D12 Route to new writer | Successors maintain explicit addresses; changing one is not fencing, owner replacement, or completed migration |
| Former unified Ref Route prerequisite | Not mandatory Core/Facet successor work; cross-machine transport/authentication, nesting, and scale need later contracts |

This replaces the former direction, not postpones all RouteRegistration/referral/lease/multi-Endpoint resolution as retained obligations. Core per-Element attachment stores Facet's forward contact; Facet stores reverse contact by exact AcceptanceKey. Both reuse stable Ref, specified NextHop, independent contact revisions, and single-owner commits without a shared mutable Ref-to-address table. Facet's own contract defines fields, first acceptance, maintenance permissions, and F0 reads; do not duplicate them here or require Facet completion before mount validation. Ref-string equality/in-process object checks cannot impersonate unimplemented network authentication.

D7/D10's whole-local-scope authorization profile does not establish hidden-branch view-vector/opaque-closure protocols. This slice changes no governing target and starts no Facet local lifecycle, complete federation, migration, or network productization. Existing local contracts/Phase 1 still constrain implementation. P2/P3 separately received explicit Start, bounded implementation/full verification, user acceptance, and commit authorization. No parallel contact contract or old-form compatibility layer. P7.3a removed Facet type-resolution ports and does not restore fact-Ref address resolution.

## 7. Implementation decomposition and acceptance

Phase 1's shared minimum foundation was delivered separately. P2 control and P3 combined queries are verified, accepted, and separately committed. Only the [development plan](development-plan.md) maintains scheduling/default order; the following proof scope creates no parallel execution plan:

1. Stable Core identity/scope, bidirectional addresses, unified history: Store validation/local regressions establish the control foundation.
2. Establishment, ordinary B content, Range coordination, revocation: close original-outcome, gate, recovery, and fault-injection behavior.
3. Path resolution, direct listing, cross-Core QueryRegion: verified control relationships with cuts, permissions, pagination, and gaps.
4. Dual-durable-Core black-box example/full-repository verification for user acceptance, not internal helpers replacing public paths.

Minimum falsifiable tests:

| Category | Required behavior |
|---|---|
| First mount | Empty Anchor/B; reject nonempty; no fake root; A rejects local children; B cannot write before activation |
| Activation race | A cannot ordinarily shrink after B prepares old Range and then activate; CAS/exact reservation cannot be bypassed |
| Range race | Shrink vs B create/move cannot jointly violate coverage; tangency accepted; failure never enlarges/rewrites automatically |
| Non-Range update | M Pose/Parent-only does not pause B or alter B local records; new queries use new projection, retained ones original |
| Gate | All new create/update/reparent/delete blocked; old receipts replay; blocked requests consume no OperationID |
| Terminal decisions | A Abort immutable; no B timeout unlock; abort-before-Prepare/late Prepared/Closed never restore old state |
| Restart/persistence | Single-side crash/lost response around every durable step; latest Store restores same state; anything beyond previous/next poisons instance |
| Revocation | Separately prove A entry closure/B stopped writes; no false completion on disconnect; Detached retains content without ordinary delete/reattach |
| Permissions | Independent A/B callers/owner scopes; wrong-owner grants fail; revoked users receive no receipts, existing settlement obligations recover |
| Spatial discovery | A sibling to B/B Basis to A sibling; huge outside-center tangent Entity; M once; depth/exact-work limits not reset |
| Empty carrier entry | Empty/entity-only B enters at external M; A supplies summaries/full M record with independent authorization; no fake root/omitted Scope |
| Query cuts | Initial composition/concurrent writes; stable Retained pages/per-call CurrentVerified; optional config, TTL, capacity, release, restart; no global Revision |
| Gaps/pagination | Correct root failure, B denial/disconnect, real emptiness, numeric gaps; recovered source not added; final gaps persist |
| Identity | Wrong peer/new empty Store/external-M collision/observed owner collision explicitly rejected, never endpoint-chosen truth |
| No implicit routing | Forward query, original-path match read, B reverse checks, both recovery directions work without Ref-to-Peer/address lookup |
| Contact repair | Independent authorization/CAS both ways; possible Pending/gate-closed/offline; no spatial/owner/admission/original-intent change |
| Address replay | Change after Prepare then retry/restart; no intent conflict; late messages never overwrite/restore; replay changes no state |
| Address/query | Depended-on contact change expires Cut; unreachable proves neither expiry nor absence; no address switch/reroute |
| Regression | Existing test/race/vet, Realm scopes, mechanical Facet-to-Core adaptation, dependencies, public examples |

### 7.1 Historical design checks are not implementation verification

The earlier finite Range-coordination model explored 208 reachable states/624 transitions for one shrink and later revocation only; its evidence is limited to that abstraction. Removing timeout prohibition, immutable decisions, or closure records yields counterexamples.

Independent read-only reviews covered control/Store, identity/permissions, and combined queries. Before finite-snapshot revision, enumerating A/B capture, B/A recheck, one A control change, and one B change gave 30 legal orders. All 10 passing rechecks had a source-combination witness; not advancing cut on A control changes admitted an old-relationship counterexample. This old capture model remains historical design evidence, not validation of retained views, initial sealing, or later disclosure.

These checks prove neither real Store faults, arbitrary message sequences, authentication, nor liveness; they are not production tests/full protocol proofs. Implementation status comes from the plan, not models or automatic acceptance of existing QueryRegion. The specified-next-hop revision also received read-only address-lifecycle/full-chain/mainline-difference review; those finite models did not validate its new state machine. New acceptance rows require implementation fault/concurrency tests.

### 7.2 Specified-next-hop counterexample review

| Counterexample | Required outcome |
|---|---|
| Address reaches another Core | Expected Ref mismatch rejects data; address cannot change ownership |
| Same Core restarts/changes address | Preserve Ref; rebuild connection/repair address without changing Element/relationship |
| Old address fails while Range coordination pauses writes | Local address repair then recover original intent; do not require unlocking first |
| Old Prepare/Finish/update retry arrives after repair | Exact intent/receipt only; no overwrite or reopening |
| Address changes during pagination | No endpoint-switch stitching; confirmed contact change expires, unavailable evidence gives unavailability/gap |
| B has no Anchor of its own | Enter through accepted external M; A owns Anchor facts, B content; no forced root |
| Two writable replicas share Ref | Address/Ref cannot identify legitimate writer; unsupported here, requiring fencing/handoff contract |

These examples justify responsibilities, not arbitrary deployment, attack, scale, or message-sequence correctness. Simplicity comes from known-relationship connections and reused transport, not eliminating address maintenance, remote authentication, or deployment constraints.

### 7.3 P3 depth/snapshot counterexample gates

| Counterexample | Required target behavior |
|---|---|
| Start in B ascends to A and matches A sibling | Count real parent edges, return ResolvedScopePath, read at original entry/Cut without Ref locator |
| -1 reaches true root vs parent unreachable | Proven root with actual count vs necessary-context failure; no narrowed Scope; Path.Up rejects -1 |
| Revocation, numeric-context limit, or expiry during B's -1 ascent | Corresponding necessary-context failure; no last-seen ancestor as Scope/latest repair |
| Deeper query introduces smaller-sorted Element | New query, reject old cursor, omit nothing |
| Pin old M in A; A moves M, B creates E, then pin B | Final A recheck fails; no never-coexistent old-M/new-E combination |
| B writes after Retained B pin while A unchanged | Initial combination valid; later pages use old B without extra current=cB requirement |
| Valid Range/reparent/delete after sealing | Retained pins keep old facts; CurrentVerified rejects changed cuts; new query reads new facts |
| A revoked, B retains old Active | Current A gate rejects; historical pin extends no mount access |
| Address change vs unknown-address disconnection | Proven revision change expires; otherwise unavailability/gap, no reroute |
| Full read after final page; expiry/restart | No automatic final-page release; explicit expiry/restart failure, no same-cut handle rebuild |
| Capacity full after selection/one capture fails after another | Whole-call failure without source removal; cleanup undelivered resources, preserve writes/old handles |
| Revocation after admission, expiry during borrow, B disconnect before disclosure | Reauthorize/lifetime-check each page; zero results or terminal Partial, no unverified cache disclosure |
| A/B both disabled or one disabled | CurrentVerified, no cross-call spatial pins; normal pagination if cuts unchanged |
| Proof-only B disabled | Still CurrentVerified; no omission because B outputs nothing |
| B writes after CurrentVerified first page | Continuation/same-Cut Resolve rejected; no old A pin + latest B |
| Both enabled, necessary side full | Explicit capacity failure, not "unconfigured" or downgrade |
| A enabled/B disabled; B computation allowance insufficient under CurrentVerified | Whole-call capacity failure, not remove B and select A-only Retained |
| Retained pin expired but current unchanged | Still expiry; no rebuild/mode switch |
| CurrentVerified metadata released/evicted, then B recovers | Old Cut expired; no source revival/terminal-marker loss |

Historical in-memory enumeration covered 154 rooted trees of at most 6 nodes, 5,039 Origin/Start pairs, and 44,484 depth cases for root saturation, Scope-path composition, and relative-match readback, without real permissions/Store/network. For A pin, B pin, A recheck, and one write per side, 20 legal orders gave 10 passing combinations, each with a coexistence witness; removing A recheck produced 1 impossible combination. This covers only initial Retained capture's finite model, not optional configuration, per-call CurrentVerified, or arbitrary concurrency.

These are design counterexamples/test requirements, not claims of passing code tests.

Optional-retention finalization additionally enumerated A/B capture, B/A recheck, and one write per side: 30 legal orders, with witnesses for all 10 passing CurrentVerified combinations. Four configuration combinations and both sides' retention capacity produced 16 checked branches: disabled is not failure, enabled-but-full never downgrades. This abstraction excludes real authorization, network, Store, reclamation, and arbitrary concurrency and cannot replace implementation tests.
