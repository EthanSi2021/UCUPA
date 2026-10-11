# P7 ownership continuity and lifecycle contract

**Status: accepted P7 domain contract. The first supported P7.1–P7.4 profiles have been implemented, verified, accepted, and committed. The architecture authority manifest declares the exact current scope. Positive continuation of outgoing external edges remains a conditional, on-demand contract and is not implied by the first delivery. No parallel design is retained.**

In the original development history, this contract was accepted at `12502b35`,
with its complete text restored from the version retained at historical commit `b9a1a96f`;
the abbreviated rewrite was withdrawn.
The 2026-09-23 scope revision distinguishes first-delivery from on-demand support without removing
command responsibilities, durable evidence, or failure semantics of supported profiles.
Positive continuation of external edges remains a condition for future enablement, not a first-delivery P7 acceptance gate.
Code was reimplemented from the completed refactor at `35f9e6f3`,
without restoring old P7 source or inheriting its implementation state/verification conclusions.
The development plan maintains the current baseline and implementation order.
FacetAttachment, AttachmentRevision, ProposeAttachment, and similar terms here describe relationships, revisions, and actions,
not requirements to restore old public symbols.
Core relationships use the accepted "Facet" terminology; actual declarations are synchronized with the corresponding slice.

This contract implements [mainline D12](ucupa-concept-mainline-baseline.md#d12-owner-transition-preserves-stable-uuids-but-permits-explicit-unavailable-windows),
continuing D4's commits/original-operation recovery, D7's independent authorization,
D10's completeness, and D11's observation continuity.
Networking reuses the [P6 contract](network-access-contract.md);
phase status appears only in the [development plan](development-plan.md).
This contract defines all P7 business boundaries, command roles, transfer evidence, failure semantics, and acceptance gates.
Go declarations/Protobuf field numbers map to it, rather than defining separate semantics.

Effective-scope clarification from the 2026-10-02 documentation reconciliation:
section 6's same-original-Store writer profile, section 7's first four Core transfer profiles,
sections 8–9's bounded complete namespace replacement/reacceptance/relationship recovery/retirement,
and section 10's corresponding read/terminal-observation loop were delivered in P7.1–P7.4.
Common command, authorization, persistence, and acceptance clauses apply within those supported profiles.
Positive continuation of external edges not enabled under section 1.2 remains outside current scope.
Evidence is in the sole development plan.

## 1. Purpose and scope

P7 preserves ElementUUIDs, confirmed facts, and recoverability of original write outcomes
when facts change writer instance or responsible owner.
Explicit, potentially prolonged unavailability is allowed.
Zero downtime, automatic takeover after disconnection, arbitrary-replica recovery, and global atomic migration are not promised.

| Transition | Preserved | Must change or be reestablished | Sole owner of the switching fact |
|---|---|---|---|
| Same-owner writer handoff | AuthorityRef, UUIDs, spatial/content facts, original operation history | Writer epoch and run credentials; address may remain unchanged | That owner's original durable commit domain |
| Core Element/coverage transfer | ElementUUID, confirmed facts; spatial changes only through an explicit final tuple | CoreAuthorityRef, placement, target authorization, and read basis | Original Core release/successor |
| Carrier replacement | Parent mount-Anchor UUID and transferred content UUIDs; structure may remain unchanged | Carrier, matching acceptance, and contacts | Parent Core DelegationBinding |
| Facet namespace replacement | ElementUUID, complete current key set, and exact payload | FacetAuthorityRef, matching acceptance, and contacts | Element's current Core FacetAttachment |
| Reacceptance by the same Facet | FacetAuthorityRef and existing namespace content | AcceptanceKey for the new Core/proposal | Element's current Core FacetAttachment |

Endpoint changes, ordinary reanchoring, mount revocation, and Element deletion remain independent and do not implicitly trigger transfer.
No global transition service, overarching Realm, independent Ref resolver, or cross-owner shared database is added.
Completing the first writer slice does not complete P7; section 13 lists all delivery gates.

### 1.1 Supersession of existing contracts

Delivered first profiles supersede only these explicit exclusions.
Unenabled on-demand profiles cannot claim support through them:

- The mount contract's exclusions of writer handoff/replacement and reattachment/disposition after revocation;
  original establishment, range-change, and revocation semantics remain.
- The local Facet contract's exclusions of content adoption/reacceptance/authority replacement;
  original F0 and ordinary-write convergence remain.
  Type responsibilities follow P7.3a's delivered opaque TypeRef/Payload contract.
- Transfer-related source invalidation and terminal-successor increments in the public read contract;
  ordinary post-write snapshot reads do not become unavailable.
- Core/Facet Store writer/transition control records and the sole current encoding;
  normal Element/Facet truth models remain.

Existing Realm identity, credential, and grant decisions and P6 network authentication are not replaced.
The network only carries this contract's added commands.

### 1.2 First supported profiles and on-demand extensions

The first P7.2 Core ownership-transfer profiles support a single Entity,
an owner-local Anchor closure without external mount edges,
root successor, and carrier replacement whose content contains no uncontinued external mount edges.
If the source or target is itself in an existing incoming mount relationship,
its parent relationship, permissions, and placement must still be checked under this contract;
it is not treated as an unrelated independent space.
These first Anchor/root/carrier profiles exclude outgoing mount edges within selected content
that would need cross-owner reestablishment.
They do not exclude verified incoming source/target parent relationships
or the carrier's own parent-controlled mount relationship.
These are public support commitments, not private-transaction or one-sided-RPC demonstrations.
Every supported profile completes source fence/final cut, target acceptance/installation,
unique successor/binding, original-operation recovery, read closure, and necessary fault verification.

If selected content contains external mount edges whose continuation is not complete,
the first public entry points must explicitly reject before withdrawing source content.
Partial membership, old relationships, or unfinished acceptance cannot be reported as a successful transfer.
Positive support for closing each external relationship and establishing a new-parent continuation
is a later profile started separately for a concrete scenario.
Existing private implementations/tests may remain, but do not expand first-delivery public commitments.
Section 7.3 still constrains correctness if that profile is enabled;
"on demand" cannot justify bypassing external edges.
This layering does not reduce identity, authorization, single-writer, durable-recovery, or error-disclosure requirements
for supported profiles.

## 2. Responsibilities and transfer identity

- An authorized external caller selects the resource/target and submits exact intent; UCUPA does not choose migration policy.
  Each owner authenticates/authorizes independently, and an entry service identity cannot substitute for the user.
- The old owner stores source facts, closes original commit eligibility, fixes final cut,
  and retains original outcomes and irreversible continuity records.
- The target owner/writer receives and validates facts and explicitly accepts.
  It neither publishes them nor accepts ordinary writes before the switch is established.
- The control owner makes the sole decision for the existing control fact in section 1's table;
  it does not own a second copy of another party's facts.
- Execution services transport bytes, inspect decisions, and complete existing obligations;
  they do not select a target or initiate different authorization.

Roles may coincide, but evidence is not interchangeable.
A writer is run-time commit eligibility, not a new Element container.
An epoch is a commit generation, not an API version.

### 2.1 Exact operation binding

A transfer is uniquely identified by (control owner, OperationID).
OperationID retains the nonempty, at-most-128-byte convention.
The control owner saves canonical input: kind, original caller identity, source/target,
exact resource scope, expected Revision/epoch, final placement, and target-bound approvals.
The first contact dependency binds direction and that contact's Revision; later address repair is recorded separately.
Same ID/same input returns original progress/outcome; same ID/different input conflicts.
The target deduplicates by original transfer identity rather than treating retransmission as a new import.
After timeout, inspect/recover the original ID; do not guess a retry by changing ID, CAS, or tuple.
Manifests/public evidence contain no long-lived credentials, sessions, delegation secrets, or original Store signing keys.

## 3. Public states and irreversible decisions

    OLD_ACTIVE → source durable fence + final cut → UNAVAILABLE
    UNAVAILABLE → unique successor/binding decision + matching acceptance + target installation → NEW_ACTIVE

These three states describe availability while changing writer/fact carrier.
OLD_ACTIVE permits only the old party; UNAVAILABLE permits neither as an active source of that fact;
NEW_ACTIVE permits only the new party.
The old party may retain protected transfer/recovery evidence,
but does not present old placement as a current Element.
No temporary dual writing followed by deletion, shadow Element, or timeout-based inference of successful switching is allowed.

The following durable milestones apply by transfer type; they do not require six states, records, or calls for every operation.
Milestones that can be atomic within one owner/transaction may share records/calls,
provided exact input, decision, and original-result evidence remain.
A control decision and target installation in different transaction domains cannot be merged into a completed operation;
omitting states cannot hide unfinished obligations.

| Internal durable progress | Meaning |
|---|---|
| INTENT | Control owner fixes input and permits target preflight; old facts remain valid |
| FENCED | Source closes the write gate and fixes final cut in its sole commit transaction; old facts are no longer an active source |
| ACCEPTED | Target durably holds complete candidate data, exact acceptance, and a placement reservation; not yet public |
| COMMITTED | Control owner irreversibly commits the unique successor/binding decision; target need not yet be online |
| INSTALLED | Target checks COMMITTED and enables facts in its own transaction, without reselecting input |
| SETTLED | Necessary parties record final effects and complete bounded cleanup; this does not authorize erasing sole continuity records |

COMMITTED without installation remains unavailable, not prematurely NEW_ACTIVE.
Acceptance is a durable commitment to an exact transfer, not ordinary content-write permission.
Later grant revocation does not erase accepted obligations.
New changes, target changes, and other transfers still require current authorization;
ordinary new-owner reads/writes remain independently authorized on each call.

| Type | Required flow | Not additionally introduced |
|---|---|---|
| Same-domain writer handoff | Close old epoch, fix final cut/designated installation; a new run checks the same Store and installs a higher epoch | Export/chunked import, new-owner placement, or cross-Store acceptance; see section 6 |
| Core slice/carrier/root transfer | Source fence → complete-content and target-placement acceptance → sole control decision → installation/outcome convergence; first profiles reject uncontinued external edges, while future enabled profiles continue each edge | Global atomic migration; section 1 determines the control owner |
| Facet namespace replacement | Source fence → complete-namespace acceptance → Element Core switch → new Facet installation/outcome convergence | Spatial-placement adjudication or moving only some keys; see section 8 |
| Reacceptance/relationship recovery with the same content owner | Prove old relationship invalidity, verify retained content/current acceptance conditions, and establish new proposal/acceptance and one valid relationship | Retransporting content, reviving old approvals, or repeating an owner transfer; see sections 8.1 and 9 |
| Content retirement/control release | Explicit disposition, final proof for a closed scope, then release after checking outstanding obligations | A fictional target owner, target installation, or NEW_ACTIVE; see section 9 |

Reacceptance/recovery continues an existing invalid relationship.
Retirement ends with completed disposition, not the full handoff state chain.
Fence/Accept/Commit rules below govern only decisions actually borne by that type.
Inapplicability does not waive current relationship, content, or permission checks.

### 3.1 Cancellation, recovery, and authorization timing

1. Before fence, the originally authorized caller may cancel and the old party remains valid.
   After fence there is no generic Abort, restoration of the old writer, or automatic target change.
2. Before permitting the source to fence, the controller closes its own cancellation window.
   Later cancellation requires the source's durable result: not yet fenced, and permanently refusing to fence for this transfer.
   Source cancellation/fence serialize in one transaction domain; disconnection cannot establish successful cancellation.
3. Before fence, source and target must provide target-bound preflight approvals; preflight is not final content acceptance.
   Each party revalidates original caller/view before and after external waits and before its Fence/Accept/Commit transaction.
   Revoke/regrant cannot continue an old undecided action.
4. Source Fence, target Accept, and control Commit are each party's irreversible commit point under its exact authorization.
   Confirmed obligations can subsequently be recovered.
   Expiry of run-time approval neither deletes the obligation nor expands its scope.
5. An ACCEPTED target does not unilaterally expire its reservation.
   Only authoritative cancellation or final completion/disposition evidence discharges it.
6. After restart, undecided user actions require exact retry by the reauthenticated original caller;
   a persisted Principal string is not a credential.
   Decided installation/settlement may be completed by a bounded execution identity, not disguised as fresh user authorization.
7. If durability is uncertain, first inspect that owner's durable result.
   Disconnection, cancellation of a wait, and RPC timeout are not rollback evidence.
8. If the original target cannot continue, remain unavailable; changing an address cannot change the target.
   Replacing an already accepted target would require a separate transfer proving that the original target can never enable facts.
   This increment provides no automatic redirection.

## 4. Command evidence and permissions

Caller control and restricted peer cooperation are distinct.
Core/Facet provide only semantic entry points required by that type, not a generic transition-management module.
The caller submits intent, inspects progress, exactly retries, or requests recovery;
it need not orchestrate chunks, fence, acceptance, and installation step by step.
Each owner advances and durably records its own steps through restricted peer commands.
Workflow state is not delegated to the SDK or a new coordinating owner.

Caller control:

| Command role | Input/result | Handler |
|---|---|---|
| InspectTransition | Exact resource/transfer → disclosable progress, comparison Revision, outstanding obligations | Each record owner |
| BeginTransition | Original ID, kind, source/target, scope, expected, final tuple → INTENT | Control owner |
| CancelTransition | Original ID, expected → authoritative cancellation only before fence | Control/source serialize under section 3.1 |
| RecoverTransition | Original transfer → one bounded advance of decided obligations, without creation or target change | Corresponding owner/authorized execution service |
| UpdateTransitionContact | Original transfer, direction, contact CAS, new address → independent contact Revision | Owner storing that contact |
| RestoreRelation | Former relationship, content lineage, new proposal, expected → exact reacceptance/recovery decision | Relationship control owner and content owner |
| RetireContent | Exact closed scope, explicit deletion intent/transferred evidence, expected → final disposition evidence | Content owner |
| ReleaseControl | Matching disposition, expected, no outstanding obligations → release control occupancy | Mount/attachment control owner |

Restricted peer cooperation; same-domain steps may occur directly within the owner, without RPCs added for symmetry:

| Command role | Input/result | Handler |
|---|---|---|
| FenceSource | Exact intent, source approval, expected → fence, final cut, applicable content basis | Source owner |
| ReadTransferChunk/StageTransferChunk | Transfer, manifest, position, exact bytes → bounded transport/deduplication result | Source/target service, only for actual content transport |
| AcceptTarget | Complete content basis, target approval, type-specific conditions → durable acceptance | Target owner |
| CommitTransition | Original transfer, fence, matching acceptance, expected → immutable decision | Control owner |
| InstallTransition | Exact COMMITTED decision, accepted object → original installation result | Target owner |

These are not entry points through which a caller issues its own fence, acceptance, or installation permission.
Undecided actions still require the original caller's valid approval under section 3.1.
Cross-machine operations retain P6 gRPC/TLS, designated addresses, expected-target authentication,
original-requester target-bound approval, and independent receiver authorization.
Fixed peer identities execute only exact transfer ports, not arbitrary user modifications.
Hashes, Refs, OperationIDs, and successor bytes are not authentication, access credentials, or offline permission.

Realm retains PrincipalRef + PermissionID + owner-qualified scope, separating at least source export/release,
target import/accept, control binding changes, status reads, and fixed-peer execution.
Writer operations use authority scope, Core slices exact boundary/closure, and Facet exact namespace.
Ordinary CRUD, Query, MountRead, or Facet Get does not authorize export, handoff, or retirement.
Concrete PermissionIDs/codecs are registered in existing shared values/Realm without another permission owner.
Public Inspect/recovery evidence follows original-operation/resource permissions;
other callers' transfer tasks and hidden targets cannot be enumerated.

### 4.1 Results and errors

Transfer progress, business availability, and request outcome are returned separately.
Successful Begin is not completed transfer, and COMMITTED is not installed target.
Inspected state includes issuing owner, transfer identity, comparison Revision,
completed steps, and recovery responsibilities that may be disclosed.
INVALID means invalid input; DENIED means permission not established;
CONFLICT means original input/CAS/lineage or reservation conflict.
Capacity exhaustion uses existing RESOURCE_EXHAUSTED;
unestablished evidence uses UNAVAILABLE, and lost run-time read continuity uses GAP.
State names here describe contract semantics.
Wire errors reuse each owner's existing categories with exact mapping,
not a generic migration failure that hides distinctions.
Disconnection after a write may have been dispatched leaves its outcome uncertain;
only that owner's retained outcome can determine whether it committed.
Recovering an original successful operation after current facts transferred
does not make its old outcome current write/disclosure eligibility.

## 5. Final cut, manifests, and original outcomes

When handing off a writer or transferring content, source fence and final cut are fixed in the same commit lock/transaction.
A write that succeeds first is included in final cut; later writes are rejected.
Data final cut is distinct from fence/successor control Revision;
control records do not pretend to be ordinary data changes.
A cross-Store manifest binds transfer, source, scope, final cut, complete ordered entries, and canonical content digest.
The digest covers entry kind, ID, length, and exact bytes.
A file hash without a membership manifest cannot prove scope completeness.
Same-domain writer handoff directly checks the original Store/handoff record rather than generating a new whole-database manifest.
Reacceptance by the same content owner does not reimport existing content.

| Type | Objects that must be retained/verified |
|---|---|
| Same-owner writer | All original Store facts, claimed UUIDs/tombstones, control logs, acknowledged and retained non-changing outcomes, scope/issuance material; no transport through public export encoding |
| Core slice | Members, UUID/kind, Parent/Pose/Range, coverage dependencies, source Revision/cut, attachment pre-state, and continuity; only this owner's facts transfer |
| Facet namespace | Complete live TypeRef set, exact payload, relevant tombstones/operation lineage, original namespace acceptance/cut |

Cross-owner transfer does not copy source signing keys, Realm grants, run tokens, or unrelated resource history.
The target uses its own owner/Revisions.
The source remains recovery authority for old OperationID→original outcome.
The target stores transfer/source-operation lineage associations for deduplication and provenance,
not source CAS/outcomes as new-owner write credentials.
A source operation spanning multiple slices is still recovered by the source under original permissions,
without disclosing unrelated entries.
Preserving acknowledged writes requires verifiable final data/original outcomes,
not availability of every old outcome at the target while the source is disconnected.
Sole original-outcome recovery evidence cannot be deleted because content transfers.
New writes use new owner context/CAS without automatic replay of source commands.

Total transfer size, entry count, chunk size, concurrency, and staging quotas are explicitly bounded;
unknown total size cannot justify unlimited reception.
Same position/same bytes is retryable.
Different bytes, gaps, overlaps, duplicate UUIDs/keys, wrong digests, or extra entries are rejected.
Unaccepted temporary chunks may be reclaimed after a bounded lifetime;
data bearing durable obligations is not erased with session TTL.
Insufficient capacity explicitly rejects/pauses without truncating manifests,
calling a partial namespace complete, or reverting the old party.
Facet acceptance verifies the complete key set, transport digest of exact payload, capacity, source, and lineage.
It does not parse payload or execute type contracts.
Removing artifact/pin rules does not waive completeness, duplicate-key, tombstone,
original-request/result, or content-digest checks.
This retains accepted opaque-payload semantics;
prior clauses requiring type artifacts for Facet writes/persistence were superseded
by mainline D5 and the public contract's opaque-content rules.
Slice implementation/acceptance status appears only in the development plan.

## 6. Same-owner writer handoff

### 6.1 Supported profile

Core persist/Facet save keep their respective sole transactions/history validation.
This profile persists writer epoch and handoff decisions within the same Store.
A bbolt exclusive lock protects only one database file; it cannot fence a copied file.

The first profile is **explicit sequential handoff within the same original durable commit domain**:
no Store copying, rollback, or substitution.
The old run closes its current epoch in the Store, fixes final cut and exact target-installation binding,
rejects all later business writes, and stops new disclosure.
It cancels in-flight requests before exit.
The new run obtains exclusive access to the same Store, validates records/authorization assembly,
and atomically installs a higher epoch.
Target-installation binding is eligibility for controlled startup, not a new AuthorityRef/address,
and cannot be obtained through self-reported identity alone.

Ordinary Open encountering a closed but uninstalled handoff record may only perform controlled recovery checks;
it does not automatically become writer.
Every commit transaction checks the run's epoch, not only startup or RPC entry.
Spatial facts, attachment/mount control, Facet content/acceptance,
contact updates, and recovery writes all obey this gate.
Installation control transactions are a narrow exception and cannot carry ordinary writes.
Installation occurs at most once; epoch overflow is rejected,
and old installation eligibility cannot obtain the next generation.

Normal restart retains a valid epoch and rebuilds run context without creating a handoff.
Explicit handoff irreversibly closes the gate and designates installation;
it is not the same automatic-takeover branch.
Same-owner handoff does not silently change Realm/authorization ownership.
The first profile explicitly checks original dependencies through controlled assembly.
Comparing RealmID alone is neither protection from malicious assembly nor the Stage 2 contact mechanism.

### 6.2 Unsupported replica takeover

Two Store copies with identical Ref/epoch and separate file locks cannot fence each other.
Another copy with the same identity cannot be enabled without an original durable fence constraining all commit paths.
Cross-independent-Store data movement in P7 follows section 7's different-owner transfer,
not copied identity called writer handoff.
Backup rollback, automatic disaster takeover, and replica consistency need concrete operational contracts;
this profile does not prove them.

## 7. Core Element/coverage transfer

### 7.1 Membership and source closure

An Entity is the smallest slice.
An Anchor slice lists all owner-local descendants and direct boundaries,
leaving no local child pointing to an old parent.
External mount content belongs to another owner and is not copied as local entries.
Slices with external boundaries must continue each matching relationship.
Without legitimate new-parent/carrier evidence, they cannot be enabled;
omitting boundaries cannot establish complete subtree transfer.
First public support is limited to closures without external mount edges.
Inputs with external edges follow section 1.2's safe rejection.
Positive per-edge continuation enters delivery acceptance only after that later profile is explicitly started.

Fence also prevents changes to membership, ancestor transform composition,
external relationships, or related operations that would invalidate final cut.
Unrelated slices may continue writing.
Unfinished mount/attachment control must first converge or make transfer initiation fail;
an Element does not silently carry away undecided obligations.
Source parent adjacency and release change consistently in the source transaction.
The source slice leaves active queries while successor/progress evidence remains;
exit at fence is not completed departure.
New queries distinguish unfinished transfer, carrier change within the original space,
and completed departure under section 10.
Historical continuity records are not a permanent query gap,
and their public disclosure remains authorized.
Old lineage of claimed UUIDs is not cleared to "never existed";
ordinary Create cannot bypass a successor to reuse identity.

### 7.2 Placement, reservations, and installation

The initiator submits final Parent, LocalPose, and Range.
UCUPA validates; it does not automatically adjust pose, expand Range, or preserve world pose.
An already active target UUID or permanently deleted lineage is rejected,
without overwriting or revival.
Previously transferred-out inactive lineage is not permanent deletion.
A return A→B→A, including after multiple transfers, requires verification of a continuous successor chain
from known predecessor to current source, current source fence, and this new operation
before a legitimate new placement generation can be established.
Old claimed/successor records are not erased.
Retry of the same transfer returns only its original result, not a return import;
ordinary Create still cannot reuse the UUID.
Against explicit parent and ancestor Structure/Range cut,
the target checks parent existence/kind, acyclicity, complete enclosure, and direct-content authority.
A mount parent additionally requires exact DelegationBinding/MountAcceptance/contact/write-gate checks;
parent Core cannot be bypassed.

Durable acceptance establishes a persistent control reservation for UUID, placement, and relevant relationships.
Related deletion, Range shrinkage, reparenting, revocation, or another transfer cannot invalidate promised placement;
conflicts are explicitly rejected.
No mutex is held across a network wait.
Local records and checks on every relevant commit path enforce the reservation without globally pausing unrelated writes.
Restart restores it; timeout cannot release it unilaterally.
Completion or valid cancellation evidence discharges it.

Original Core commits a unique successor from fence/acceptance; only that permits target installation.
Original transfer, target acceptance, and the sole decision must match issuer/role before installation.
A network receiver reads the decision through authenticated contact with the designated controller;
client-provided hashes, self-reported success, and stale contact caches do not enable facts.
An exact result already durably installed at the target may replay locally.
Later relationship-validity checks still follow original mount/Facet contracts;
replay creates no new commit eligibility.
Target Revisions describe new placement; source tokens cannot serve as cross-owner CAS.
Structure may change through the explicit final tuple or remain unchanged.
Both ownership continuity and spatial validity require evidence.

### 7.3 Carrier, external edges, and root

Parent mount control serializes carrier replacement.
Both old-carrier final cut/fence and new-carrier acceptance must hold before binding switches.
The mount Anchor remains owned by the parent, not moved with content.
Range update, revoke, and replacement of one relationship cannot commit independently/concurrently.
If slice transfer with external mounts is later enabled,
it also requires permanent invalidation of the old-parent relationship,
new-parent exact delegation, and reacceptance by the original carrier.
Carrier content is not copied.
Any unconnected edge leaves the whole operation incomplete/unavailable;
new structural cycles, duplicate acceptance, and out-of-range placement are rejected.
The carrier's own control records must stop writing/valid querying of that branch under its old-parent relationship
and fix matching acceptance with the new parent.
Changing a pointer at the moving parent does not implicitly authorize the carrier
or permit continued use of the old relationship.

Root replacement requires an old-root successor,
preserving root UUID and the no-Parent/unbounded-root rules.
The target is a controlled inactive receiver under a new authority;
it cannot overwrite an existing active root or splice independent spaces together.
Inactive data is private transfer state.
Installation atomically establishes the target's sole root and complete accepted space.
A disconnected old root without evidence means unavailable;
creating another space does not inherit the old root.

## 8. Facet namespace replacement and reacceptance

A namespace is an Element's complete TypeRef key space.
Moving only some keys cannot switch its entire Attachment.
Source Facet closes the write gate/fixes final cut in one transaction.
Put, Delete, acceptance changes, and late F0 rechecks can no longer commit.
The target validates the complete manifest, exact-payload integrity, and lineage;
before acceptance it cannot ListTypes/Get/Put.
Acceptance binds source namespace, final cut, ElementUUID, target Facet,
and the current Core's exact new proposal.

Element Core commits the sole switch only when old fence, target acceptance,
and expected AttachmentRevision match.
The replacement's new proposal is stored in the exact transfer control record,
not an ordinary ProposeAttachment overwriting ACTIVE/DETACHED.
The old relationship remains valid before fence and explicitly unavailable afterward;
only the unique Commit changes current binding.
Delete, Detach, other proposals, and Core transfer serialize with it,
without reviving a deleted Element.
After installation, new Facet still performs F0 capture/fresh-check/recheck;
old final cut is not current attachment validity.
A switched Core with an uninstalled target is unavailable;
old Facet is permanently noncurrent.

### 8.1 Same-Facet reacceptance after Core transfer

Core transfer does not move Facet payload by default.
The new Core's initial attachment is DETACHED with predecessor evidence.
Only an actual NONE source becomes NONE;
unrecovered content is not interpreted as no content.
New Core creates a new proposal.
The same Facet verifies old Core successor/existence final cut,
new Core existence/proposal, and namespace lineage before issuing new matching acceptance.
New Core then commits ACTIVE without reusing the old AcceptanceKey.
In its own transaction, Facet uniquely associates the same namespace lineage with the new acceptance,
not a second writable content copy.
Matching UUID alone cannot adopt existing content.
Core transfer may finish first; Facet failure leaves its attachment detached,
without rolling back Core ownership.
While recovery proposal obligations remain unfinished,
ordinary CancelProposal, Element Delete, or another Core transfer cannot erase them.
Facet may already have accepted with its response still in transit;
Core still being DETACHED alone does not prove cancellation safe.
Continue exact recovery/activation, or discharge through explicit final disposition,
rather than ordinary deletion in place of disposition evidence.

## 9. Recovery and retirement after revocation

Revoke/detach does not prove that content was deleted, returned, or transferred.

- Restore the original content relationship using a new proposal/acceptance and original content lineage,
  checking original revocation, absence of replacement/retirement, current scope, permissions, and contacts.
  Revocation history is not cleared to reuse old approval/write gates/query bases.
- Content retirement requires explicit deletion intent, not a recovery timeout.
  Within a closed scope, the content owner confirms authorized deletion of all content
  or handoff to a target with a complete successor, then provides exact final disposition evidence.
- Control occupancy is released only after the control owner verifies disposition,
  no unfinished obligations, and external child relationships.
  Nonempty content, disconnection, or unknown disposition causes rejection.
  Ordinary empty-Anchor deletion cannot bypass undisposed content of a detached carrier.
- Content-byte cleanup is separate from permanent deduplication/continuity records.
  P7 does not reclaim sole tombstones, successors, or original operation outcomes,
  or add generic historical GC. Temporary chunks and terminated run credentials may be reclaimed within bounds.

Retirement does not make UUIDs reusable.
Permanent content-owner disconnection without disposition evidence remains detached/unavailable;
completion is not fabricated.

## 10. Reads, discovery, old snapshots, and observation

After fence, old facts are no longer a valid current source.
Retained preserves data but does not freeze owner/writer validity.
Relevant receipts, pagination, and ReadAt cannot continue across fence:
they explicitly report invalid context, GAP, or unavailability, not a false empty set.
In-flight reads check fence at the final disclosure gate.
Old responses already handed to the network stack cannot be recalled,
and ordering by network arrival time is not promised.
Old evidence still grants no new commit or continuation eligibility.
Unrelated resource reads are not stopped without cause.

A new query determines completeness from its resolved Scope, query sphere, and verified cuts,
not from where an Element previously belonged:

| State this query depends on | Read rule |
|---|---|
| Transfer unfinished; relevant branch may still intersect and cannot be safely pruned | With valid root context, return PARTIAL under D10; without an established root basis, UNAVAILABLE; source adjacency exit alone does not establish an empty set |
| Carrier changes within the original semantic scope | Verify the new source through confirmed current relationship/successor and designated contact; COMPLETE requires closed acceptance, installation, structure, permission, and read evidence |
| Transfer complete; Element outside this Scope or not intersecting its query sphere | Current structural/spatial evidence may safely exclude it; retained old lineage creates no gap, and other necessary sources closing permits COMPLETE |

Clearing a departure gap requires persistent source completion evidence linking the exact control decision,
authenticated target-installation result, and source-adjacency change,
bound to the control Revision/evidence verified by this query.
COMMITTED, self-reported success, timeout, or source-adjacency change alone is insufficient.
If new placement still belongs to this Scope and may intersect,
the completed transfer's matching source must be included once through the new path.
Once departure from this Scope is proved, later target disconnection does not recreate that old-Scope gap;
a query including target Scope still independently checks target availability.
If placement remains within Scope but lies outside the query sphere,
safe pruning requires valid coverage evidence at this query's cuts.
Position at transfer time does not permanently exempt the target from checks.
Only valid structure/contacts can continue discovery of the current Scope.
A successor is continuity evidence, not a pass to expand Scope or bypass permissions.
Transfer invalidating Start, Basis, or root evidence requires reestablishing query context,
not silently changing scope or returning an empty result.

Old continuation does not insert successors or change Scope/cuts.
The new-query rules do not restore old receipt, ReadAt, or pagination eligibility.
Disclosable successor evidence provides source/target/final cut/decision binding/designated contact.
The target independently verifies identity, placement/acceptance, and current permissions.
Without disclosure permission, return only opaque failure,
not hidden owner identities, addresses, scopes, or counts.

After final cut, source observation provides terminal transition/successor evidence
without mixing in ordinary changes from that old scope.
Termination evidence remains readable only within original valid authorization and retention.
Lost retention or unproven continuity yields GAP, not a false no-change result.
New owner observation starts from its own explicit cut, accepts no old cursor,
and does not promise inherently consecutive sequence numbers across the two streams.
Same-owner writer restart also invalidates run-local cursors.
Durable handoff records explain termination; they are not unlimited historical replay.

## 11. Persistence, contacts, resources, and engineering boundaries

Within its own transaction, each owner saves only the intent, fence, acceptance, decision,
installation, and original input/outcome that its type actually bears.
It does not copy other owners' records or add empty steps to complete a universal state chain.
Data/control history has verifiable causal relationships.
Load rejects missing final cuts, installation without provenance,
duplicate targets, and regressing epochs.
Same-owner handoff may have an independent control journal,
without rewriting business history or treating replay as a new commit.
No write lock is held during external waits.
After returning and before the final transaction, recheck original record, role, expected values, and evidence.

Task capacity, concurrency, and transfer deadlines are explicit and bounded.
Durable obligations do not vanish with session TTL.
A full task budget rejects new work rather than discarding old tasks for space;
resource exhaustion/disconnection does not automatically release fence.
The owner storing each direction validates identity, permissions, and contact CAS for contact repair,
without using the broken old address or waiting for the control slot to unlock.
An address change changes neither target, owner, nor manifest, and revives no old credential.
Late old updates cannot overwrite newer contacts.

Core/Facet maintain their own gates/state without importing each other's kernel.
Composition/transport/SDK only wire ports.
Responsibilities follow the [engineering rules](../../ENGINEERING.md),
without a shared writer manager, generic workflow, or second fact index.
Current placement puts Core writer/control records in the sole persistent owner
`ucupa-core/internal/commit`,
and Facet records in the sole content owner `ucupa-facet/internal/namespace`.
Store does not decide placement or authorization.
New implementation follows real responsibilities,
without business proxies piled into `instance` or empty transfer/chunk/handoff directories created in advance.
Placement does not narrow this contract or permit a second commit/recovery mechanism.
Protobuf has one generated representation, with public schemas synchronized.
Local/network paths share owner decisions without old-interface aliases or old Store readers.
P7 transfers business-fact ownership; it is not compatibility migration between historical software versions.

## 12. Required positive and negative cases

Verify these cases for each declared supported profile.
For positive continuation of external edges not yet enabled,
first delivery verifies safe rejection.
An internal isolated test is neither public support
nor grounds to remove correctness requirements for future enablement.

| Counterexample | Required result |
|---|---|
| Write races with fence | Write enters final cut or is rejected; no lost acknowledged commit |
| INTENT cancellation interleaves with source fence | Only one receives durable permission; unknown source state cannot establish successful cancellation |
| Old instance/clone comes online after fence | Old epoch rejected in original domain; cross-copy takeover without real fence is unsupported, not self-reported success |
| Crash/response loss before and after every durable boundary | Only old-valid, unavailable, or new-valid; original-ID inspection/recovery, without dual writes/disclosure |
| Parent shrink/delete/reparent/revoke after acceptance | Relevant reservation prevents invalidating placement; no installation from stale spatial evidence |
| Slice omits members, leaves children, or misses cross-owner edge continuation | No acceptance/installation; partial copying is not a complete subtree |
| Target UUID conflict, duplicate namespace, same ID/different input | Conflict, without overwrite or second active lineage |
| Return A→B→A, old successor replayed as return, deleted identity reimported | Only a new transfer with complete continuity and current source fence; reject old evidence and revival after permanent deletion |
| Revoke/regrant, approval changed to another target, service borrows user permissions | Undecided action rejected; decided recovery follows original binding only |
| Payload tampered, keys missing, final chunk lost, quota full | No partial acceptance, binding switch, or rollback of old party |
| Core Delete races with Facet replacement/reacceptance | Serialized constraints or conflict; no renewed ACTIVE for a deleted Element |
| Old acceptance/CAS/cursor used at target | Not new write/observation credentials; original-outcome recovery remains separate from new writes |
| Only source adjacency exit/COMMITTED, target installation unfinished | Relevant unclosed query cannot claim COMPLETE empty set; old pagination cannot switch sources to continue |
| Query old Scope after completed departure, then target disconnects | Persistent completion/current-scope evidence proving exclusion removes a permanent departure gap; other closed sources permit COMPLETE |
| New and old placement both within one query Scope after completion | Evaluate new-placement intersection at matching cuts and count at most once; neither omission nor duplicate disclosure |
| Outside query sphere at installation, then move/expand Range within the same Scope | Use current valid coverage, not permanent pruning by installation-time position; missing required evidence prevents COMPLETE |
| Carrier changes without spatial change, or successor points outside Scope | First case requires the new source to close or a gap; second cannot expand query Scope |
| Same-domain writer/same-Facet reacceptance forced through export/import | Check original Store/content and applicable decisions directly, without useless transport; still verify handoff/relationship acceptance |
| Caller directly requests internal installation, or cross-owner Commit/Install merged | Reject without restricted execution eligibility/exact decision; decided control cannot be reported as enabled target |
| Ordinary mount-Anchor deletion or same-UUID recreation after detach | Reject without disposition evidence; old identity cannot be reused |
| Disconnection/contact repair meets unfinished transfer | Preserve correct state; permit authorized independent repair, without automatic target change or routing fallback |
| Hidden-branch transfer/unauthorized successor query | No identity/address/count disclosure; unknown is not absent |
| New Core inherits merely because old root is disconnected | Reject continuity claim; a new space is not recovery of the old one |

## 13. Overall P7 acceptance gates

Acceptance is incremental for declared supported profiles.
Partial private results cannot substitute for complete first-delivery public capabilities.
Existing internal code does not make later on-demand profiles mandatory for the P7 Baseline:

1. Core/Facet writer handoff within the same original durable domain,
   covering every commit path, fence, restart, and installation.
2. Public transfer/recovery for Core Entity, owner-local Anchor closure without external mount edges,
   root successor, and simple carrier replacement.
   Inputs with uncontinued external edges safely reject before source content is withdrawn.
   Positive per-relationship continuation for external-boundary slices is accepted only after that later profile is explicitly started.
   Implicit atomic migration across multiple owners is always rejected.
3. Complete Facet namespace replacement, same-Facet reacceptance after Core transfer,
   revoked-relationship recovery, and content retirement.
4. Corresponding authorization, public control commands, original-result recovery,
   designated-contact maintenance, query invalidation, terminal observation, and reentry loop.
5. For first supported profiles: unit tests, deterministic concurrent interleavings,
   faults at every irreversible durable boundary, independent Stores/real product processes,
   P6 network retry/permission matrices, and public positive/negative cases.
   No exhaustive Cartesian product of unenabled profiles and every fault condition is required.
   Protocol changes must preserve the declared independent-implementation corpus.
6. The complete repository go run ./cmd/verify passes with evidence for engineering,
   generation, race, dependencies, and examples, explicitly listing supported profiles.

If positive external-edge continuation is later started,
add its public-call, authorization, durable-recovery, and per-edge fault acceptance under sections 7.3 and 12.
Current private code or first P7 acceptance does not establish support for that profile.

Independent processes on one machine do not prove real multi-machine networking or replicated-Store safety.
P6's real multi-machine verification boundary remains explicit.
Same-identity cross-copy takeover, the P8 production profile, Stage 2 Realm contacts,
and downstream products are not P7-delivered capabilities.
Contract confirmation/implementation authorization does not establish implementation or acceptance,
automatically commit subsequent code, or start a later phase.
