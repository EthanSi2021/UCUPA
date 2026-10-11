# RealmAttachment authorization-contact contract

**Status: the S2P1 design was implemented by S2P2's local loop and S2P3a–d's network consumers/bounded compositions, verified, accepted under authorization, and committed. This is a current incremental contract only within that explicit scope. The architecture authority manifest declares exact effect; the development plan records environmental boundaries and overall Baseline acceptance. This contract authorizes no new code, commit, or deployment change.**

This implements [mainline D7/D8](ucupa-concept-mainline-baseline.md#realm-authorization-access-chain-supplement),
adding only how Core/Facet maintain contact with their authorization Realm.
Identity, credentials, and exact PermissionBinding remain owned by
[Realm contracts](README.md#4-realm-contracts);
fact commits/lifecycles remain with their respective owners.
Phase, progress, and verification results appear only in
[development-plan section 6](development-plan.md#6-stage-2realm-授权访问链).

**Supersession:** this contract replaces P4/P5/P6 local authorization-dependency assembly limits
and arbitrary run-time injection.
Original identity/credential/PermissionBinding decisions and P6 original-caller relay remain.
Contacts belong to original Core/Facet Stores;
Realm networking carries only the existing authorization kernel.
S2P3 evidence is independent product processes on one machine/real gRPC/TLS.
P6 two-host fact-service evidence and S2P2 two-host toolchain revalidation do not establish
real multi-host Realm or production acceptance.

## 1. Minimum loop and explicit exclusions

The first profile is one Core/Facet logical owner using one explicit authorization Realm.
Different fact owners may use different Realms or explicitly share one.
Shared Realm does not mean shared permissions or commit transactions.

```text
Caller → Core/Facet
           ├─ Read RealmID + NextHop from its own RealmAttachment
           ├─ Verify target, establish caller identity, and check exact requirements with that Realm
           └─ Check local facts/current execution eligibility, then disclose/commit
```

A contact is not authorization.
Realm neither proxies spatial requests nor decides Core/Facet business validity.
No global Realm, Ref-address resolution, authorization copying,
offline Allow cache, automatic Realm change, or replica takeover.
No per-Element multi-Realm configuration, proposal/acceptance state machine,
RealmAuthorityRef, K8s, or unified administration module.
The first profile permits maintenance only of **the same Realm's address**.
Changing authorization Realm, merging permissions, or migrating the authorization database is excluded.

## 2. Record ownership and uniqueness

Each fact owner stores exactly one effective contact.
Its original Core/Facet Store is the sole persistent contact owner.
In-memory owners follow the same rules without process-exit recovery guarantees.
Elements, content, type keys, and incoming transfer packages do not duplicate this record.

| Semantic field | Constraint |
|---|---|
| Owner | Current Core/Facet FactAuthorityRef, exactly matching actual owner of the containing Store |
| Realm | Existing RealmID responsible for this owner's authorization decisions; unchanged after establishment in this profile |
| NextHop | One explicit address using current local/network profiles; neither derived nor resolved from RealmID |
| ContactRevision | Opaque comparison basis issued by the containing owner; equality comparison only, bound to Owner/contact generation, not interchangeable with Element/content Revision |

These are necessary semantic fields, not a requirement to redundantly encode an Owner already present in the Store header.
Initial establishment, address changes, and retained outcomes join the original owner's sole transaction/history system,
without another contact database/commit engine.
A→B→A addresses do not reuse contact generation.
Original comparison evidence retains defined effect after restart, rather than resetting an in-memory counter.

RealmAttachment contains no credential secret, PermissionBinding, Realm authorization private key,
authorization view, session, connection object, DNS result, or Realm Store path.
Controlled deployment supplies trust material/run-resource configuration;
it cannot override recorded RealmID.

Authorization Realm differs from `PrincipalRef.Realm`, the subject's identity namespace.
Receivers cannot select an authorization source from the request subject's Realm.
Foreign subjects still require P6 explicit identity trust/liveness checks,
then independent authorization by the Realm designated by this contact.
A contact does not establish cross-Realm trust.

## 3. Establishment, opening, and trusted connections

### 3.1 Explicit initialization, not repair during ordinary startup

Controlled fact-owner initialization explicitly supplies expected RealmID, NextHop, and adapter trust configuration.
Prove connection to that Realm first, then atomically save contact with initial owner state.
Failure cannot leave an externally serviceable owner without contact.
Initialization uses existing trusted bootstrap,
not a Realm self-reported by an ordinary request or automatic subject/admin-permission creation.
Existing Realm bootstrap/administration explicitly configures initial administration permissions;
creating an attachment grants none automatically.

Opening an existing Store first validates Store identity/integrity, writer eligibility,
and persistent contact, then connects using that record.
Configuration may assert expected Realm to detect wrong assembly,
not override persistent records; mismatch rejects enablement.
Missing record, wrong Owner, invalid address, or corrupt history is a Store/configuration error,
not repaired with default Realm, arbitrary injected Authorizer, or reseeding.
Ordinary service requires successful authorization-dependency verification before enablement.
Offline inspection/section 4 controlled repair cannot expose ordinary reads/writes.

### 3.2 Local and network use the same contact semantics

- Local adapters obtain a trusted assembly object at the designated `local:` address and check RealmID,
  without scanning instances for a matching ID.
  This proves controlled assembly, not trust in a malicious object merely returning that ID.
- Network uses current gRPC/Protobuf/TLS principles:
  explicit NextHop, standard certificate/hostname validation,
  and controlled `expected RealmID → server SPKI` binding before credentials/authorization bodies are sent.
  RealmID and FactAuthorityRef are not converted into each other.
  A target field accepting only fact Refs cannot impersonate Realm identity.
  Realm service also compares expected request Realm against its original opened Realm Store identity.
- Trusted Core/Facet-to-Realm service access binds explicit service identity and allowed fact owner;
  network uses mTLS/controlled binding.
  Knowing a Realm address does not let a client self-report users or query every authorization relationship.
  Service admission eligibility differs from user fact permissions, and both are checked.
- Do not follow peer-returned addresses, downgrade, or switch to local Realm after unreachability.
  DNS resolves only the designated host.
  Process/IP changes behind a stable NextHop do not automatically update contacts;
  reconnect still verifies identity.

Ordinary requests cannot replace trust, certificates, or credentials.
Trust changes require controlled reassembly, not old connections bypassing revocation.
This profile does not defend malicious writable clones possessing the same Realm Store/service private key,
or implement Realm single-writer handoff/HA.

## 4. Contact maintenance: reading and same-target address updates

Only two owner-control action semantics are added, not generic Realm CRUD:

| Action | Input/result | Required permission |
|---|---|---|
| Read contact | Current Owner → RealmID, NextHop, ContactRevision | `ucupa.core.realm-contact-read` or `ucupa.facet.realm-contact-read` + exact AuthorityScope(Owner) |
| Update address/recover original outcome | Original OperationID, Owner, expected ContactRevision, original RealmID, new NextHop → original decision/contact comparison basis | `ucupa.core.realm-contact-update` or `ucupa.facet.realm-contact-update` + exact AuthorityScope(Owner) |

The owner defines permissions; Realm still compares only exact requirements.
Ordinary Element read/write, writer administration, or Realm-binding administration
does not imply contact-maintenance permission.
Ordinary Element/Payload reads do not disclose contact data.

Capture original contact and validate new NextHop shape,
then acquire current maintenance authorization from the **original Realm** through one of two paths:

- **Normal maintenance:** if current contact remains verifiable,
  authenticate original Realm at current address and check the maintainer's exact permission.
  New address need not be online; saving it does not prove availability.
  First use authenticates target under section 3.2.
  Failure remains unavailable without sending credentials to a wrong target or falling back to the old address.
- **Disconnected repair:** if current contact cannot be verified,
  the explicitly submitted new address may contact original Realm.
  First authenticate the candidate as the persistently recorded Realm;
  only then send identity evidence, establish maintainer identity, and obtain current authorization.
  Same RealmID alone, self-reported Allow, or cached old authorization cannot repair.
  An unverifiable candidate cannot change contact.

Both paths use the same update command/local commit rules,
without alternate-address lists or automatic business routing.
An established explicit denial cannot switch paths to search for Allow.
Candidate contact is used only to authorize explicit repair;
it is not an ordinary authorization source before commit.
RealmID still cannot change.
After authorization, the original local transaction checks writer gate,
Owner, RealmID, and contact CAS, atomically committing contact/outcome.
Success proves persistent contact update, not new-address reachability.
A not-yet-online endpoint cannot turn committed success into not-executed.

Repair does not need old-connection approval but does not bypass original Realm authorization.
If ordinary service cannot enable, a controlled maintenance entry exclusively opens the **original** Store
and executes only the same update rules, without a second concurrent writer.
An expired old business session cannot repair.
An unauthenticatable original Realm or unauthorized maintainer leaves repair failed/unavailable,
without emergency Allow, a substituted authorization database, or product database-editing backdoor.

Same actor/OperationID/canonical request retries return original decision;
different binding conflicts without automatic ID/CAS change.
Retained-outcome reads still require original Realm current maintenance permission,
through current contact normally or an authenticated candidate during offline repair.
After authorization, inspect original outcome first.
Historical addresses superseded by later updates need not remain reachable,
and old address updates are not reexecuted.
Only a new undecided update proceeds through necessary path checks, contact CAS, and write.
After crash, contact/outcome are both pre-state or both post-state.
Lost responses recover by original request.
Outcomes record their historical result, not subsequent current contact.
A valid same-address update may be unchanged without advancing generation.
Malformed inputs, unauthenticatable repair candidates, or unavailable authorization dependencies
do not fabricate persistent DENIED.
Admitted decisions converge under original owner retained-result rules.
History validation uses committed decisions/bindings,
not reconnection to Realm to reassess yesterday's commit with today's permission.
Capacity/generation overflow explicitly rejects;
outcomes cannot be deleted or generations regressed to manufacture success.

Pure address update changes no RealmID, AuthorityRef,
spatial/content Revision, Facet relationship, or writer epoch.
It advances contact generation without directly revoking caller, view, or session;
completed durable business results remain.
Continuation after contact change must prove continuity,
not merely a new endpoint self-reporting the same RealmID:

- Reauthenticate original Realm at current designated address.
  Validate **original caller establishment/original view**, receiving owner,
  exact requirement set, local fact basis, and related permission generations.
  Only then may original context/continuation/observation continue under current contact generation.
  Do not put a new caller/new Allow into old tokens, extend expiry, or save another authorization copy.
- Temporary unverifiability returns UNAVAILABLE without disclosure/commit.
  One disconnection does not automatically destroy original context,
  but its existing expiry, capacity reclamation, and release still apply.
  Continuation requires successful revalidation.
- Confirmed caller invalidity, Realm restart, relevant revoke/regrant, or invalid basis
  follows original rejection/INVALID_CONTEXT/GAP rules.
  Reestablish identity/read basis as required; continuity cannot ignore invalid evidence.

In-flight execution guards still check contact generation.
If it changed at final local checking, old verification cannot be consumed directly.
Within original request deadline/bounded attempts,
the request may revalidate as above and recheck local dependencies;
otherwise stop rather than forcibly rewriting old comparison evidence.
This establishes only Realm-contact authorization continuity,
without loosening Facet, spatial-snapshot, or other existing-token invalidation rules.

## 5. Identity, authorization, and final business decisions

1. A request enters the designated Core/Facet.
   That owner's verified Realm contact establishes caller identity.
   Ordinary requests cannot select another Realm/address as authorization source;
   a subject declaration is not identity evidence.
2. Owner derives the complete PermissionID/Scope set from current operation/fact boundary
   and submits it to designated Realm.
   Realm atomically checks relevant authorization and returns existing decision/opaque view.
   Unrelated bindings do not invalidate this view.
3. At final read/write boundary, owner checks authorization continuity,
   contact generation, caller validity, and local facts before disclosure/commit.
   Realm does not decide ancestry, mount validity, Facet lineage, or payload,
   or store a second set of spatial/content facts.

Consumers cannot accept only `bool Authorize` and collapse remote errors into false.
Public chains preserve VALID, CHANGED, DENIED, and UNAVAILABLE.
Confirmed invalid identity differs from unverifiable identity.
Denial discloses no object existence.
Federation denial views establish only that an exact requirement remains denied;
successful denial validation is not Allow.
Revoke/regrant invalidates old views even when final Allow matches.
Disconnection extends no view, session, delegation, or query-basis lifetime.

### 5.1 Local ports are not directly remote protocols

Existing CallerContext and AuthorizationView/DenialView are Realm run-local trusted-port values,
not memory fields serialized as remote identity proof.
S2P2 first implements local contacts.
S2P3 bridges Realm caller/views through bounded remote handles
bound to service, caller, and receiving owner,
then local adaptation supplies them to fact owners.
Handles are not offline authorization or final-client disclosures.
Realm view keys/bindings are not copied to Core/Facet,
and client-reported Principal does not justify TrustLocalCaller.

P6 terminal sessions, restricted delegations, issuer challenges,
identity liveness, and independent target authorization remain.
Direct same-Realm credential access retains original audience.
The receiving service is an explicitly trusted credential-handling boundary:
it may forward credentials to the authenticated designated Realm for identity establishment,
without storing secrets or forwarding them to another Core/Facet, logs, or outcomes.
Cross-Realm identity still uses existing explicit issuer trust,
not attempts to authenticate a foreign Principal's secret at a local authorization Realm.
New Realm service communication identities protect service calls,
not service permission substituted for user permission.

Remote-handle counts/lifetimes, request deadlines, and message capacity are explicit and bounded.
P6 resource/disconnection rules remain, without automatic renewal/offline grace.
Realm restart invalidates old run evidence.
Reconnect restores neither old sessions nor continuation proof.

### 5.2 Network waits and decision timing

Current Core `internal/access/admission.go` requires lock-held ValidateView to be bounded local work.
Facet retains capture/unlock/recheck boundaries.
S2P3 cannot insert network RPCs into those lock-held calls.

Remote composition uses bounded capture/verification/local recheck:

1. Locally capture this exact command/read scope, caller, contact generation,
   writer epoch, and required fact basis.
2. Hold no fact commit lock while waiting for Realm.
   Establish/check view through authenticated contact and complete current authorization verification for this request.
3. Return to the sole local decision boundary for local-only checks.
   Verify this result belongs to the same request/requirement set,
   current verified contact generation/run, and still-valid business dependencies.
   Contact changes revalidate under section 4.
   Changed facts/permission requirements fail or reverify;
   old Allow cannot be applied to new scope.
4. Only this in-flight request may consume the result.
   It is not a persistent permit or reusable across a next page, retry, or another operation.
   Holding it does not nullify deadline, cancellation, contact change, or writer-gate closure.

This is not a distributed atomic Core/Facet/Realm transaction.
Authorization linearizes at Realm's final verification decision for this request.
Revocation completed before verification must reject.
Concurrent revocation after verification does not undo this operation already entering execution;
later requests reverify.
Continuous read/observation views still detect relevant generation changes,
rather than new Allow continuing old bases.
A lost response after commit is not noncommit evidence.
Original-outcome recovery still requires current permission,
and later Realm unreachability does not roll back facts.

S2P3 implements this through call-local guards, remote handles, and typed RPCs
across existing mutation, outcome-recovery, pagination, observation,
denial-view, and P6 identity-relay consumers.
Local proof is still not directly a wire value.
Later changes cannot claim the full authorization surface from one passing Get.
If these semantics cannot be retained, stop/review the boundary,
rather than unbounded lock-held waits, periodically cached Allow,
global revocation locks, or a new authorization model.

### 5.3 S2P3 network responsibilities and bounded handles

Realm service carries the original Realm Store/kernel,
not a second grants/credentials mirror.
This network surface covers existing identity establishment/authorization consumption,
not remote subject, credential, or grant administration.
Protocol uses typed Protobuf.
The table groups responsibilities, not separate packages or a fixed RPC count:

| Responsibility | Input/output boundary |
|---|---|
| Target check | Establish actual Realm using configured RealmID, service-certificate binding, and designated NextHop before user credentials are sent |
| Identity establishment/derivation | Accept credentials under original audience or existing P6 issuer evidence/challenge/explicit trust; return verified subject/bounded caller handle. Derivation retains original eligibility, not self-reported Principal |
| Current authorization/continuous view | Verify original caller handle/exact Requirement set; return original four-state result and required bounded view handle. New Allow cannot replace old-view validation |
| Denial evidence | Establish/validate denial-view handles, retaining still-denied versus currently-allowed; successful denial validation is not authorization |
| Release | Explicitly release unused run handles, with expiry for final reclamation. Failed release creates no offline permission or unbounded retry obligation |

Protected service requests bind the TLS-authenticated, admitted receiving owner.
Target RealmID comes from its current persistent contact,
not a terminal request choosing authorization source.
RealmID/FactAuthorityRef remain distinct types;
target authentication cannot reuse them by cast.
Existing bounded TLS/gRPC transport may be shared,
without Ref routing/automatic source changes.
At first creation, target check establishes only Realm identity:
no caller/view, grant reads, or new owner registration.
Later identity/authorization consumption requires explicit trusted generated-owner/service-certificate binding.
Missing binding rejects admission; passing target check does not automatically admit services.
Trusted bootstrap differs from ordinary requests and does not expand remote administration.

P6 foreign-relay Actor is the initiating Core identity
already verified by receiving Core through service TLS,
not receiving Core's own owner.
`EstablishForeignCaller` explicitly carries Actor,
and the original signature must match it exactly.
Realm mTLS admission still determines receiving service owner;
challenge/returned handles still bind that owner.
Receiving owner cannot replace signed Actor,
and request Actor/Principal alone establishes no identity.
Issuer trust, online liveness, one-use challenges,
and expiry recheck after waiting retain original rules.

Handles are unguessable run-local references.
The sole registry is at Realm service admission,
associating RealmID, Realm run, receiving owner,
original caller, and corresponding view requirements.
They are not final-client disclosures and cannot cross owner/caller/run.
Local proof fields of caller/view/denial view are not wire encodings;
Core/Facet adapters obtain no Realm signing keys.
Handles reference kernel evidence, not cached cross-request Allow.
Every operation still completes section 5.2 current online verification.

Handle TTL cannot outlive parent identity/P6 evidence.
Derivation does not refresh original evidence lifetime.
Actual interfaces bound count, TTL, RPC deadline,
message size, and requirement-set capacity,
testing boundaries, expiry, and capacity rejection.
Zero cannot mean unlimited.
Temporary same-Realm disconnection does not itself deregister still-valid original handles.
After contact repair, only the same original caller/view may be validated.
Restart, invalidation, or revoke/regrant cannot be disguised as old continuity by reestablishing caller.

Cross-Realm identity still verifies original issuer evidence,
challenge, explicit issuer trust, and online liveness.
Core/Facet claiming to have verified foreign Principal does not justify remote Realm TrustLocalCaller.
Issuer network rechecks also hold no Realm state/commit lock;
after return, local dependencies are rechecked without another distributed transaction.
Service identity cannot replace original user identity.
P6 original-caller relay/independent target authorization remain unchanged.

Local/network are explicitly configured contact profiles,
not mutual failure fallbacks.
Network coverage includes creation, reopen, online maintenance,
original-Realm offline repair, and original-outcome recovery.
Adding network addresses requires per-rule review of historical decision replay.
Rules changing old durable rejection/results require a new sole Store marker and old-format rejection;
unchanged field layout alone cannot justify retaining a marker.

## 6. Recovery, handoff, and transfer

| Situation | Sole behavior |
|---|---|
| Normal Core/Facet restart | Recover original contact/outcomes and verify original Realm at its address; rebuild run caller/connections without automatic reseeding |
| Temporarily unreachable Realm | Retain contact/facts; protected calls explicitly unavailable; recovery of the same Realm requires revalidation, not Denied/NotFound |
| Realm restart | Stable RealmID remains, but old caller/view/remote handles invalidate; rebuild basis rather than continuing old observation |
| Same-owner carrier change/P7.1 handoff | Original Store contact follows owner; check original Realm/writer eligibility without machine-config substitution. Closed epoch cannot also update contact |
| NextHop maintenance | Independent section 4 CAS commit; fact ownership/permissions unchanged. Original context may continue after continuity revalidation through current contact; old in-flight results cannot cross new generation without revalidation |
| P7.2 Element/closure transfer to new Core | Target uses its own RealmAttachment; package imports no source contact/grants/caller; target checks new owner-qualified scopes |
| P7.3 content transfer to new Facet | Independent target contact/authorization; old Realm Allow does not replace target approval or move source owner's contact |
| Store copy/old-backup restoration | No new same-identity replica eligibility; original single-writer, anti-rollback, and recovery limits remain |

S2P2 removed arbitrary writer `AuthorizationBinding` labels.
Writer authorization identity derives from original Store RealmAttachment.
S2P3 retains that sole basis, without old labels or startup-config overrides.
Address change is not an authorization-owner change.
Necessary contact repair during handoff occurs only in writable stages allowed by the original contract,
not repair bypassing closed epoch.

An old Store missing required contact or unable to uniquely replay its historical rules
requires the implementation slice to update the corresponding private FormatMarker and explicitly reject old format.
No injected fields, converters, compatibility readers, or dual-rule replay.
Realm Store-marker changes depend on actual Realm persistent semantics,
not mechanical changes merely because Core/Facet gained contacts.

## 7. Failures and privacy

| Established result | External meaning |
|---|---|
| Authenticated Realm; current caller/grant disallowed | Original-boundary UNAUTHENTICATED/DENIED, without resource-existence disclosure |
| Currently allowed but old relevant view changed | Original INVALID_CONTEXT/GAP or other continuity failure, without silently substituting a new view |
| Missing contact, wrong Realm/certificate/pin, disconnection, or unverifiable evidence | Startup failure or UNAVAILABLE, not confirmed denial, empty result, or NOT_FOUND |
| Contact CAS/OperationID binding conflict | Corresponding original-owner conflict, without automatic address substitution |
| Capacity/deadline/cancellation | Preserve original boundary errors; absence of a receipt does not decide business commit status |

Contact addresses, internal Realm revisions, credentials, and authorization details
do not enter ordinary business errors.
Controlled diagnostics may identify configuration/trust/connection/authorization/fact-decision failure layers,
without secrets, redeemable handles, or hidden relationship graphs.

## 8. Implementation responsibilities and acceptance

S2P2 implements the local contact loop only.
S2P3 implements independent Realm service/network compositions for existing Core/Facet/P6 consumers.
Both use one persistent-contact semantics,
not a disposable local model later replaced by another network model.
Realm imports no Core/Facet private packages.
Connection adapters own neither grants nor facts; SDKs do not adjudicate for owners.
Contacts enter existing Core commit/Facet namespace domains.
Public values/ports belong to corresponding contracts;
actual network placement follows ENGINEERING.
This contract creates no speculative directories or authorization for wholesale Realm-directory normalization.

| Minimum counterexample/evidence | Gate |
|---|---|
| Normal local identity, authorization, Core/Facet reads/writes; identical requirement outcomes for memory/durable profiles | S2P2 |
| Wrong Realm/owner, missing/corrupt records, config override, request-selected Realm never permit access | S2P2 |
| Restart retains contact; concurrent address CAS, A→B→A, original-ID retry/reordering, crashes/response loss converge | S2P2 |
| Authorized current contact permits committing a not-yet-online new address; outcome/restart retain it. No credentials/service before new-target authentication, or old-address fallback | S2P2; S2P3 adds TLS cases |
| With old address offline, only legitimate original-Realm maintainers repair through new address; unverifiable paths/impersonating candidate leave records unchanged, and definite denial cannot search another path for Allow | S2P2; S2P3 adds TLS cases |
| Pure address change leaves fact Revision/epoch; same original caller/view may continue pages/observation after continuity revalidation, and old in-flight results cannot commit/disclose without it | S2P2; S2P3 adds network interleavings |
| Address change combined with Realm restart, revoke/regrant, or expiry cannot continue; temporary disconnection is neither permanent-context invalidation nor grace/offline Allow | S2P2; S2P3 adds network cases |
| Same-owner handoff retains Realm and rejects old-writer writes; cross-owner transfer imports no contact/permissions | S2P2 |
| Independent Realm process/Store and correct gRPC/TLS target; wrong pin sends no credentials; wrong audience/service binding rejects | S2P3 |
| No local caller/view/denial-view fields on wire; remote handles cannot cross caller/service-owner/run; current single-use verification is not reusable across requests | S2P3 |
| Explicit barriers verify revocation timing, revoke/regrant ABA, and Realm restart; bounded sessions cannot allow offline access | S2P3 |
| Slow/disconnected Realm holds no fact commit lock; changed local facts/contact/epoch after relocking cannot commit/disclose old result directly, only revalidate/stop under original rules | S2P3 |
| Real service chains with Core/Facet using separate Realms, plus explicit shared Realm; P6 original-caller relay/independent authorization do not regress | S2P3 |
| Mid-operation disconnection/cancellation, committed response loss, reconnect/original-outcome recovery; old outcomes not disclosed without permission revalidation | S2P3 |

This is a bounded matrix, not exhaustive topology/fault combinations
or resumed scenario-free P6 environment validation.
Process-boundary acceptance uses real gRPC/TLS/independent Stores;
same-machine evidence does not claim real multi-host/production capability.
Every code slice still runs full `go run ./cmd/verify` on its final file set,
separately recording implementation, verification, user acceptance, and commit.
S2P1 performs only contract/reference/engineering-consistency checks;
documentation checks do not establish runtime behavior.
