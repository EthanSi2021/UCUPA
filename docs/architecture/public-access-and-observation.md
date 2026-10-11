# UCUPA public access and fact-observation contract

**Status: the P5 design contract was accepted and committed as `3c00b0c1` in the original development history. Its code and schemas were implemented, fully verified, accepted under the user's authorization to begin acceptance and commit, and committed with the S1P5 delivery. This is a current incremental contract within the scope declared by the architecture authority manifest. Evidence and delivery records are in the development plan.**

This contract implements [mainline D5, D7, and D9–D11](ucupa-concept-mainline-baseline.md) as an increment to public behavior.
It does not redo D1–D4 or introduce a generic CRUD owner, independent routing layer, second world, or new authorization model.
The five categories describe user outcomes; they do not require five identically named methods or modules.
The [authority manifest](README.md) declares the current implementation scope; only the
[development plan](development-plan.md) maintains the implementation sequence. As the accepted P5 increment,
this contract supersedes only the explicitly identified public-value, caller/view, public read/write/observation,
and type/content clauses. Under the [P7 contract](owner-transition-contract.md), P7.3a replaced section 8's
type-artifact, pin/mirror, and schema-validation rules with opaque Payload semantics.
Local P1–P4 behavior not explicitly superseded remains effective. These are not two parallel versions;
design acceptance does not prove implementation completion.

**Successor-increment notice:** the [P6 network contract](network-access-contract.md) covers network public access and federation;
[P7 section 10](owner-transition-contract.md#10-reads-discovery-old-snapshots-and-observation) covers source invalidation,
terminal observation, and reentry for the first supported transfer profiles; the
[RealmAttachment contract](realm-attachment-contract.md) covers persistent authorization contacts and network Realm verification.
Exclusions stated here limit the P5 slice; they do not deny these separately delivered increments.
Public-scope, original-operation, pagination, observation, and permission invariants not explicitly superseded remain effective.
Documentation closure does not make an unenabled profile supported.

## 1. User outcomes and scope of this increment

A caller must be able to complete this sequence:

```text
Explicit space entry + independently established caller context
  → Enter → temporary SpaceHandle
  → Discover → unknown Elements, paths, actual scope, and read basis
  → Read → Core facts and same-cut attachment; independent Facet access as needed
  → Observe → changes since the read basis, continuation, or an explicit gap
  → Propose → owner validation, durable commit, and a recoverable outcome
```

The first complete P5 public profile is **an explicit Anchor closure within one Core, with optional Facets for that closure**,
using explicit, trusted local connections and caller establishment. Each Facet owns its own namespace and stream;
they do not form a global cut. Core and Facet do not share a Store, and no overarching Realm is required.

P2/P3's two-Core local capabilities remain supported and covered by regression tests.
Renaming those entry points does not prove public federation. When an operation in this public profile must cross a mount,
it explicitly reports unsupported coverage: UNAVAILABLE if the entry context cannot be established,
or PARTIAL if relevant descendants of a defined query remain unhandled.
A mount proven irrelevant by safe pruning needs no remote contact. A result cannot report COMPLETE
and only then disclose in a footnote that mounts were ignored.
Extending the public cross-Core source graph still requires the corresponding federation evidence.

This contract does not include network services, remote authentication, nested federation, writer/owner migration,
production deployment, or Stage 2 RealmAttachment. A public encoding is not an Internet transport;
trusted local binding must not be presented as remote identity authentication.

## 2. Pre-implementation baseline and required increment

The table records gaps before P5 implementation, not a second implementation to retain.
Current public entry points and verification artifacts are in the [Core API](../../ucupa-core/api/README.md),
[Facet API](../../ucupa-facet/api/README.md), and the P5 delivery record in the development plan.

| Existing basis | What P5 adds, and what does not prove it |
|---|---|
| Core `ReadElement`, `ListDirect`, `QueryRegion`, path reads, and optional snapshots | Explicit entry/boundary, public addresses, and value encodings; consumers must not need private tokens or advance knowledge of the target UUID |
| Core owner-qualified exact scopes; query authorization against the resolved Scope | D7's explicit AnchorClosureScope, membership at the operation cut, and authorization views; an exact scope alone is not general closure authorization |
| Core `commits`, the sole order for spatial, mount, and attachment changes | An authorized committed projection and cursor; no direct publication of the journal, OperationID, actor, or original commands |
| Facet `events`, recording commands and their outcomes | Changes only from commits that actually advance the Facet cut; rejection, unchanged outcomes, and replay are not new facts |
| P4 TypeRef/witness/injected validator | Arbitrary opaque 32-byte TypeRef and bounded, byte-exact opaque Payload; Facet does not fetch or parse type artifacts or execute schemas, as revised by P7.3a |
| Realm's original boolean `Authorize` and aggregate binding revision | Controlled authorization-view validation; adapter timestamps, cached booleans, or a public whole-database version cannot substitute for a view |

Implementation entry points: [Core](../../ucupa-core/instance/instance.go),
[query](../../ucupa-core/internal/query/operation.go),
[Core commits](../../ucupa-core/internal/commit/owner.go),
[Facet decisions](../../ucupa-facet/internal/namespace/mutation.go),
[Realm authorization](../../ucupa-realm/internal/policy/binding.go), and
[shared scopes](../../shared/values/types.go).

## 3. Enter: an entry point is neither permission nor a new world object

### 3.1 SpaceReference

A shareable description contains at least BoundaryAnchorRef (CoreRef, AnchorUUID), the designated Core NextHop,
and a trust-bootstrap hint recognizable to the caller. A hint may be stale, but contains no secret,
grants no permission, and cannot override the receiver's Realm or authentication configuration.
Publishing a reference does not make its publisher the fact owner.

The local profile obtains its trust context from explicit assembly, independently establishing the target Core and caller.
A PrincipalRef self-reported in JSON is not an authenticated identity. A missing address does not trigger a Ref-directory lookup;
a wrong target or disconnection does not mean that the space is absent.

The fixed local public fields are `boundary:{core,anchor}`, `nextHop`, and optional `trustHint`.
`core` is a FactAuthorityRef and `anchor` an Anchor UUID, encoded under section 8.
`trustHint` is merely an untrusted hint that the caller may ignore; it cannot create an authentication session.
Trusted caller context is a separate call argument, not part of the reference.

### 3.2 Establishing and using SpaceHandle

Enter first validates the input and independently established identity context,
then checks the entry Anchor, current spatial structure, and necessary authorization with the expected Core.
Success returns an owner-issued, caller-bound, temporary run-local handle and the boundary/source evidence that may be disclosed.
The handle does not pre-enumerate a subtree, contain a global Revision, or act as a transferable authorization ticket.

- A root or non-root Anchor can be a published boundary; entering an Entity is not entering a space.
- Later operations recheck current permissions and required relationships; successful Enter does not freeze Allow forever.
- Moving the boundary within the same Core does not change BoundaryAnchorRef; old paths and cuts expire under their own rules.
- If the boundary is deleted or its authority does not match, the old handle is not automatically attached elsewhere; no guessed owner or permanent redirect.
- Handles are not reused after Close, expiry, or instance restart. Reenter from the original reference; normal reentry does not change UUIDs.
- A known but unauthorized target returns DENIED without confirming existence; NOT_FOUND requires authorization and authoritative read evidence.

SpaceHandle governs the boundary and call context. QueryContinuation, read snapshots, and ObservationCursor each govern
their own lifetime; they are not combined into one universal token. Discarding a handle deletes no facts or persistent contacts.

### 3.3 Minimum public call surface

The table fixes request responsibilities and required values, not a generic facade.
Section 8 defines canonical field encodings. Existing facts, final tuples, CAS, and receipt fields refer directly
to their owner contracts rather than restating a second fact model.

| Behavior | Minimum input | Result and constraints |
|---|---|---|
| `Enter` | Independent caller context, SpaceReference, `intent` | Handle, boundary, and minimum authorization evidence for that concrete operation; read, discovery, and create/update/delete can be intents, with no generic Propose intent |
| `CloseSpace` | caller, handle | Idempotent release of locally derived context, not deletion of a space |
| `ReadElement` | caller, handle, structural address, optional read basis, `prepareObserve` | Element/Attachment and read receipt from one Core cut; preparation is also returned when requested |
| `ListDirect` | caller, handle, Anchor address, limit, optional continuation, `prepareObserve` | Pagination in the existing list order; the first page establishes the read basis and optional preparation, and later pages do not recreate it |
| `QueryRegion` | caller, handle, Start/Basis addresses, UpDepth, sphere, selector, limit, optional continuation, `prepareObserve` | Actual Scope, ActualUpDepth, matches, status/gaps, receipt, and continuation basis |
| Owner-specific Core mutation | caller, valid handle, operation name, original final tuple, CAS, OperationID | Original command and exact replay; reference-only access is only for section 7's outcome recovery, not a substitute for the handle required by a new commit |
| Facet content access | caller, designated Facet contact disclosed by Core, AcceptanceKey, original type/payload/preconditions | Independent checks of Facet identity/permissions and the current Core relationship; a Core handle is not Facet authorization |

Structural-address fields are `origin`, `up`, and `down`. Origin is a known Anchor within this handle and may default
to the boundary; up is a nonnegative count of actual parent edges, and down a UUID chain.
At one cut, Core verifies the connection from Origin to the boundary and every path edge.
Caller-reported ancestry is not evidence. Ascending cannot cross the boundary;
an unknown or stale path cannot be silently repaired using another, latest cut.
Discovery returns rereadable paths from verified Origin to Scope/results, not a bare-UUID entry point that bypasses the boundary.
Query UpDepth selects a scope; it is not the address's up field.

Enter checks only the existing permissions needed by its selected intent, without pre-granting other behaviors.
For example, query-region intent does not additionally require element.read and therefore does not disclose Facet contacts merely by entering.
A write-only caller can enter under the corresponding create/update/delete intent,
receiving only boundary validity and a handle, not ElementRead; it is not forced to acquire read permission first.
Section 4 fixes concrete intents and permission combinations; update intent checks both update and place.
Every later operation is independently decided under section 4. Successful entry is not final Parent or membership validation.

## 4. Public permissions: retain exact relationships and add closure scope

`AnchorClosureScope(BoundaryAnchorRef)` is a new exact scope variant. It does not change
`PrincipalRef + PermissionID + OperationScope` or introduce role trees, wildcard paths, or a policy language.
Realm checks only whether that scope is explicitly granted; Core checks membership at its own fact cut.

1. Closure Discover permission allows discovery of previously unknown Elements created within that closure,
   without pre-granting each UUID.
2. Enter checks the actual discovery/read permissions it needs, rather than adding an enter permission with no separate responsibility.
   Discover, spatial Read, Observe, and Propose are independently authorized. Discover grants neither modification
   nor disclosure of Facet entry points through Core exact reads, Facet payload access, or attachment/contact control.
3. Public owner methods check the handle boundary, current authorization boundary, and membership at this operation's fact cut.
   Updates require both the source Element and final placement to satisfy the authorization scope;
   an old path alone cannot authorize moving outside it.
   Existing independent exact-scope administration retains its own permissions and cannot bypass a public handle's boundary.
4. Core may privately traverse local structure within the closure, but returns only records that may be disclosed
   and the paths needed to explain them. The first profile uses a **fully authorized closure**, with no per-node hiding
   or mixed ACLs inside it. Exact grants are not inferred to be closure grants.
   This is a declared capability limit, not permission to treat future hidden branches as absent.
5. Facet applies its own owner-qualified namespace/key permissions; Core closure grants do not cross into Facet.
   Facet's reverse check of Core continues to use a fixed service identity, not the client's identity,
   and must not skip P4's current-relationship validation.

### Per-operation authorization mapping

Core public paths explicitly select `AnchorClosureScope(handle.boundary)`; failure does not fall back to an exact grant.
Membership objects below are checked at the owner's same read/commit cut.
A shared authorization scope does not mean that only one object needs checking.

| Core operation | PermissionID, retaining existing names | Objects that must be within the closure |
|---|---|---|
| ReadElement | `ucupa.core.element.read` | The Element read; this permission still discloses the complete same-cut ElementRead/Attachment envelope |
| ListDirect | `ucupa.core.anchor.list-direct` | The listed Anchor and its disclosed direct contents |
| QueryRegion | `ucupa.core.anchor.query-region` | Start, actual Scope, Basis, returned records, and explanatory paths |
| CreateAnchor / CreateEntity | `ucupa.core.anchor.create` / `ucupa.core.entity.create` | Final Parent; the new Element enters that closure in the same commit |
| UpdateElement | `ucupa.core.element.update` **and** `ucupa.core.element.place` | Source Element **and** final Parent; both being in one closure does not remove the place requirement |
| DeleteAnchor / DeleteEntity | `ucupa.core.anchor.delete` / `ucupa.core.entity.delete` | The original deletion subject; existing constraints such as rejection of nonempty Anchors remain |

Creating a root is not an operation within an entered space; it retains the original independent bootstrap interface.
Mount, attachment, and contact control keep their specific PermissionID/target combinations,
which the closure permissions above do not grant. Their exact administration entry points do not become generic closure control.
Facet Get/Put/Delete use the existing FacetKeyScope, and ListTypes the existing FacetElementScope;
there is no generic Discover/Propose grant.
Public content observation adds `ucupa.core.element.observe` (Core closure or exact CoreElementScope),
`ucupa.facet.element.observe` (FacetElementScope), and `ucupa.facet.observe` (FacetKeyScope).
Observation also requires the specific permission for its underlying read.
Core service-control observation is defined in section 6.6 and does not implicitly broaden client permissions.

### Authorization views and revocation

Public reads, pagination, and observation require a view issued by the authorization boundary,
bound to caller establishment and this operation's exact requirements.
Allow and the view must come from the same authorization state, and the view must be validated before disclosure.
A change during comparison causes rejection/retry; reading true booleans before and after is insufficient to detect revoke/regrant ABA.

**Authorization generations for the relevant requirements** derive from Realm's existing identity, credential, and binding changes.
Deleting/regranting a relevant binding invalidates old views even if final Allow is the same.
Unrelated binding changes and changes to undisclosable facts are not public-view events.
This derived view is not another PermissionBinding and exposes neither Realm's whole-database revision nor hidden subject counts.
An authorization service that cannot establish the result returns UNAVAILABLE, not a false definite Deny.

The minimum consumer ports are `AuthorizeView(caller, requirements) -> decision, opaqueView` and
`ValidateView(caller, requirements, opaqueView) -> VALID / CHANGED / DENIED / UNAVAILABLE`.
requirements are a deduplicated, canonically sorted set of exact PermissionID/Scope pairs,
evaluated in one Realm state. The fact owner always derives them from the operation;
it must not copy caller-reported permissions.
A view is a 32-byte HMAC-SHA-256 over Realm identity, caller establishment, the canonical requirement set,
and relevant generations, using a random key for this Realm run.
Private inputs use unambiguous field encoding and comparison uses constant-time equal.
Validation recomputes current requirements and generations; it needs no extra view registry, TTL, or persistent cache.
The key never leaves Realm and changes on restart.
The token is only for trusted local-port validation, not a remote signature, cross-Realm trust,
or new contact credential, and does not publish binding sequence numbers.
MAC generation/comparison use standard-library facilities; see the [Go HMAC documentation](https://pkg.go.dev/crypto/hmac).
Established current denial is DENIED; permission restored with a different old generation is CHANGED;
unverifiable state is UNAVAILABLE.

Derived generations advance only on effective changes to relevant identity/credential or binding state,
not on rejection, unchanged outcomes, or exact replay.
A deleted binding retains its derived-generation tombstone: removing and reinserting a map entry cannot reset the generation.
Recovery rebuilds generations from existing verified history. Realm restart invalidates run-local views;
the derived map is not a second persistent Grant.
The fact owner first obtains a view, then reads/decides at its own cut and validates the view
at the disclosure or commit authorization checkpoint.
Revocation after that checkpoint does not undo an already linearized operation.
No global atomic transaction across Realm and fact Stores is claimed.
Reads prepared for observation additionally bind Observe requirements in section 6.2's order;
ordinary Read does not automatically request Observe permission.
This increment directly extends the local consumer ports it needs, rather than adapting two booleans into a view
or omitting validation until Stage 2.

## 5. Discover/Read: reuse existing spatial semantics

StructuralAddress contains only explicit Origin, a count of ascending parent edges, and a descending UUID chain.
Every edge is verified at the selected Core cut; an old path that fails to resolve cannot silently become a bare-UUID lookup.
Returned paths must support rereading on the original basis.

Public queries constrain Start, Basis, and actual Scope to the handle boundary.
The first profile **also retains P3's requirement that Basis be the actual Scope or a descendant Anchor of it**.
For example, with sibling A/B beneath boundary R, Start=A, UpDepth=0, Basis=B remains INVALID,
even though B is within R. This is not a new spatial rule; a coordinate basis outside Scope but within the boundary
is not silently added in this increment.
UpDepth is unchanged: omitted/0 stays at Start; a positive value ascends at most that many actual parent edges;
-1 reaches the proven boundary/root; values below -1 are invalid.
A disconnected ascending path is not a boundary. Ascending stops at an explicit boundary, without looking for higher ancestors.

Matching tests closed-sphere intersection in the selected Scope and all its descendants.
selector does not truncate Anchor traversal, and no downward-depth parameter is added.

P3's ReadMode, pagination order, limits, and failure semantics remain effective:

- Omitted SnapshotRetention still disables retention of old views; Enter or Observe does not enable it or supply a default duration.
- CurrentVerified can continue only the original cut while it remains current; Retained reads use the original view within its finite lifetime.
- Changing depth, region, basis, selector, or caller requires a new query rather than reuse of an old cursor.
- Multi-page query status describes only the selected sources and scope; a round is complete only after all pages are obtained and status is COMPLETE.
- Public records carry actual owner, Revision, cut, scope/path, and authorization basis; they do not copy private state or invent a global cut.

Core exact reads retain P4's same-cut spatial record and attachment envelope.
ListDirect/QueryRegion do not additionally disclose Facet contacts.
After reading ACTIVE, use the relationship's designated Facet address and AcceptanceKey directly.
Current Facet content still follows P4 F0/Core fresh-check; it is not promised to be payload from the same instant as an old Core snapshot.
Accessing a current namespace requires rechecking the current attachment.
ACTIVE in an old snapshot is not a long-lived access credential.

## 6. Observe: bounded, resumable notifications of committed changes

### 6.1 Fixed minimum behavior

The initial behavior is explicit pull through `BeginObserve / PollChanges / ReleaseObserve`,
not a message bus, background SDK watcher, or server push.
BeginObserve takes preparation obtained during a read; PollChanges requests the next batch using the preceding cursor.
Only spatial, content, or contact/attachment control changes reliably committed by the owner may produce notifications.

Each notification contains a stable opaque ChangeID, issuing owner, disclosable change categories, and commit basis.
In the first profile, notifications mean **relevant facts changed and must be reread/requeried**.
They contain no raw write command, actor, OperationID, historical payload, or automatically derived Enter/Exit/Move application event.
An Anchor movement can invalidate an entire query projection;
notifying only that its record changed must not imply that derived descendant results remain fresh.

These are ordered, deduplicable invalidation notifications, not a per-frame state stream or full historical-query API.
Multiple relevant effects of one commit form one owner-local change group;
create and initial NONE, or delete and attachment invalidation, are not split.
A public batch cannot mark half a group consumed. Insufficient capacity fails explicitly rather than truncating and acknowledging it.

### 6.2 Connecting ReadCut to a cursor

An initial read intended for observation explicitly sets `prepareObserve=true`.
An ordinary unprepared Read requests no Observe permission,
and an old cut alone cannot be used to manufacture observation eligibility at Begin.
Preparation failure makes that request fail explicitly; it cannot downgrade to an ordinary read while claiming valid continuity.
Existing old snapshots remain usable for ordinary reads.
Starting observation requires a new current read basis, not retrospective access from an arbitrary historical instant.

The order is fixed:

1. In one authorization state, Realm obtains a joint view for this Read/Discover and its corresponding Observe requirements.
2. In the same commit-ordering critical section that selects current cut C, the owner registers preparation.
   Its private contents are caller/view, selected boundary or exact namespace/key, query conditions,
   starting C, a bounded notification queue, and an absolute deadline.
   Registration failure returns RESOURCE_EXHAUSTED first, leaves no partially usable token,
   and does not block subsequent ordinary writes.
3. Complete the read at C. Any retained cross-page snapshot remains independently managed under P3.
   Validate the view before publishing the read result and preparation; on failure, release the undelivered preparation.
   Relevant commits after C already enter that preparation; registration must not wait for Begin.
4. BeginObserve validates the same caller, conditions, and joint view,
   **idempotently activates the preparation in place**, and returns an initial cursor.
   Retry neither restarts a stream nor extends its lifetime; a new Observe view cannot replace the old one.

Pagination preparation belongs to the first page's C and is reused, not recreated from an intermediate page.
A caller can Poll while paging, but the original query's incomplete coverage remains a gap.
Release preparation and requery if the initial query fails or its conditions change.
An authorized, proven empty list, empty spatial result, or absent Facet key is a valid read basis and may return preparation
to observe a later create/first Put. No current result is not registration failure;
lack of permission or evidence cannot establish that empty baseline.
Revoking and regranting Observe while Read remains allowed invalidates the joint view even **before** the first Begin.
Current denial returns DENIED; restored permission with a different generation returns GAP.
Neither fills in changes from the unauthorized interval. Without preparation, a new read is required.

While preparation/authorization remain valid and retention is sufficient,
the first observation covers every relevant `commit > C`, not latest at Begin.
Core still uses its existing sole `commits` order; Facet projects only Committed entries that actually advance its cut.
Rejection, UNCHANGED, exact replay, and queries themselves produce no new fact notification.

A cursor binds owner/issuing run, caller/authorization view, boundary/selector, starting basis, and scan position.
The client cannot edit its position to skip segments. Repeating the same cursor may repeat stable ChangeIDs,
but cannot omit a segment. A new cursor advances only through a prefix the owner has verified as scanned,
not one it merely intends to scan.

Reliable commits do not depend on notification receivers being online.
Failure to project or buffer notifications after a commit may invalidate that read side and require a new basis,
but must not roll back committed facts, falsely report that a write did not happen, or silently skip the missing segment.
Persistent facts/outcomes and derived notifications are not two write transactions:
no outbox owner capable of independently deciding facts is added.

The minimum implementation uses one bounded invalidation queue per preparation/observation,
shared by pending and active states, without a persistent message store.
The owner's commit order also serializes registration, relevance decisions, projection, and failure marking.
A committed group either enters the queue in full or marks the observation GAP.
No window may publish the latest cut and let Poll pass it before a projection gap is marked.
The invalidation marker needs no second fact commit; abnormal exit invalidates the entire run's tokens.
Public queues are not replayed on restart, and old streams return GAP.
Unrelated commits neither enter the queue nor consume its retention budget,
and must not generate empty notifications or inferable sequence changes from database-wide activity.
Reclaiming notifications does not delete the original durable journal.
Bounded observation does not require replaying the whole journal as a public stream.

When transfer ends the old scope, invalidation follows the [P7 contract](owner-transition-contract.md)'s terminal-evidence rules.
Loss after this owner's own transfer decision fences but before it settles is not final GAP evidence:
the queue stays pending, admitting neither ordinary changes nor immediate GAP.
After settlement, the next commit disclosure provides exactly one exclusive `terminal` group bound to that decision's
operation, source/target, final cut, settlement state, and designated contact.
The same cursor can reread it idempotently; exhausted retention returns explicit GAP.
A decision that does not finally complete—because authorization expires, TTL elapses, context is reclaimed,
or the instance restarts—and any loss without sufficient evidence, including a closed parent gate,
external carrier, another owner's decision, or mixed loss, still yields GAP immediately or after retention is lost,
without partial evidence. The terminal group binds no manifest content and grants no access to the successor Core.

### 6.3 Covering newly appearing results

Spatial observation tracks at least membership/structural changes and all coordinate dependencies of the selected scope,
not only the first matching UUIDs or the current candidates after safe pruning.
The first implementation may conservatively observe the whole authorized handle boundary:
changes trigger QueryRegion again. Extra invalidations are allowed; missing later matches from creation, movement into scope,
parent Anchor movement, or Range changes is not.
Relevance is evaluated against each commit's before/after structure,
not against the latest membership table when Poll runs.
A commit moving an Element out of the closure may invalidate the old result;
subsequent changes occurring only outside it cannot keep entering that queue.
Query/List preparation projects only disclosable spatial changes.
Conservative observation of the entire boundary must not expose Facet control activity.
ReadElement preparation may include attachment-envelope changes that the original read could disclose;
Facet payload still does not enter Core notifications.

BeginObserve's selector is bound to this semantic scope.
Changing depth or other query conditions requires a new query and observation,
not insertion into the old query's ordering chain. Observe cursors are not QueryContinuation.
An initial PARTIAL query may establish observation,
but receiving some later changes cannot make its gaps complete; a new query must prove closure.

### 6.4 Permissions, lifetime, and failure

Before each batch is disclosed, revalidate caller, relevant authorization view, and necessary relationships.
Old Allow cannot authorize historical content. A relevant view change terminates the old observation;
restored permission starts from a new read basis, not the old cursor and facts from the unauthorized interval.
Records, owners, counts, and change rates outside the disclosure scope do not enter public notifications.
Tokens have fixed-size opaque representations: neither raw internal commit sequences nor token length/empty-heartbeat counts
may encode hidden activity.
Even an empty Poll must not reveal unauthorized commit counts through changing cursor bytes.
This is a logical disclosure constraint, not a constant-time network guarantee.

Observation has its own bounded cursor/projection retention policy, independent of SnapshotRetention's switch and lifetime.
**Disabling query snapshots does not disable Observe; notifications do not retain a whole old spatial view across requests.**
Insufficient retention, expired context, a lost projection segment, or issuer restart that breaks continuity returns explicit GAP.
GAP requires a new read basis; it is not an empty batch or a silent switch to latest.
Temporary disconnection without proven segment loss returns UNAVAILABLE and preserves the cursor;
after recovery, the owner decides whether continuation remains possible.
Insufficient execution capacity/computation budget returns RESOURCE_EXHAUSTED, not a false claim of lost history.

ReleaseObserve releases only run-local observation resources.
Expiry permits safe reclamation without deleting current Core/Facet facts, outcomes, or Store history.
Slow consumers cannot block commits or require unbounded retention.
Observation capacities and deadlines must be explicit, bounded, and testable, with no assumed ten-second lifetime.

### 6.5 Call fields, resources, and errors

| Call | Required fields | Return/retry |
|---|---|---|
| BeginObserve | caller, preparation | cursor; idempotent activation of the same preparation, not a second queue |
| PollChanges | caller, cursor, limit | Complete groups and nextCursor; limit counts groups, default 64, maximum 256; no changes returns an empty batch and the same cursor |
| ReleaseObserve | caller, preparation or cursor | Repeatable by the same caller; releases the entire observation and its derived tokens, without undoing writes |

Each group's public fields are `changeId`, `owner`, `category`, and `basis`.
category is the set of disclosable `spatial`, `content`, `attachment`, or `contact` categories for that commit.
basis is an opaque commit basis, not preparation from which a stream can be recreated,
and does not promise readable historical payload at that commit.
Categories from one commit are merged into one group.
ChangeID is stable within the same owner run and retention scope.
Cursors are isolated by role/caller/view; public IDs do not encode commit sequence numbers.

Core/Facet startup `PublicAccessPolicy` for the public profile fixes the following values.
Omitting the policy leaves original local owner operations available,
but public-context issuance returns UNAVAILABLE (not configured), rather than silently enabling Observe or SnapshotRetention defaults.
The complete P5 acceptance profile must explicitly configure both owners.
The policy is copied for the run and not hot-modified.

| Setting | Rule |
|---|---|
| ContextTTL | Required positive duration, with no default number of seconds; measured from issuance for handles/ordinary read receipts |
| ObservationTTL | Required positive duration, with no default number of seconds; starts at preparation registration, and Begin/Poll/retry do not extend it |
| MaxContexts | Default 128; bounds the owner's total handles, ordinary read receipts, pagination references, and preparation/observation root contexts |
| MaxContextsPerCaller | Default 32, no greater than MaxContexts; service identities count too and are not exempt |
| MaxGroupsPerObservation | Default 1,024; pending and active observation share that queue budget |
| MaxPublicBytes | Default 64 MiB; conservatively accounts for derived contexts, queues, token indexes, and retained response batches, not just payload |
| MaxConcurrentPublicReads | Default 64; concurrent public reads/polls/preparations are bounded, and queued requests must also be cancellable |

Only omitted count/byte fields select these bounded engineering defaults; explicit zero/negative values are invalid.
Durations are not inferred from those defaults.
P3 SnapshotRetention/snapshot capacity remains separate configuration.
Ordinary read receipts do not retain historical space and cannot substitute for snapshots.
Later receipts/observations of an active handle do not outlive its remaining lifetime.
Expiry of a Facet service-side dependency causes a composite-stream GAP, not automatic extension.
Delivery stops at expiry while new commits persist normally.
A full unconsumed queue makes that observation GAP, releases the queue,
and retains a termination marker within the original token's validity period.
New-context allocation or execution-capacity exhaustion returns RESOURCE_EXHAUSTED;
if memory pressure forces an established observation to be discarded, it explicitly becomes GAP.
Repeated Poll with the same cursor may redeliver retained groups; first delivery does not discard their sole copy.
After the caller uses nextCursor, the owner may reclaim the acknowledged prefix.
An old cursor into that reclaimed prefix returns explicit GAP without affecting a still-contiguous new cursor.
Count/byte/time limits also yield explicit GAP for lost observation continuity;
unbounded retry history is not promised.
A slow consumer affects only its own queue. Fact commits do not wait for consumer acknowledgment,
and reclamation decisions for concurrent duplicate requests use the same observation lock.

| Situation | Public result |
|---|---|
| Invalid fields, token encoding/role, limit, or conditions | INVALID; no cursor or fact change |
| Current caller/permission definitely invalid | DENIED, with no resource-existence information or old notifications |
| Permission restored with a new generation, lost original preparation/observation, or deadline reached | GAP, requiring a new read; no reinterpretation of the old cursor |
| Lost ordinary handle/receipt | INVALID_CONTEXT; Enter/Read again, without misreporting context failure as an absent space |
| Authorized fact whose absence is established | NOT_FOUND, based only on current owner evidence |
| Temporarily disconnected peer or unverifiable current authorization/relationship | UNAVAILABLE; cursor does not advance, and recovery rechecks continuity |
| Insufficient current execution/new-context resources | RESOURCE_EXHAUSTED; neither an excuse for unreadable history nor a false COMPLETE result |

These are public error categories, not renames of internal owner errors or a merger of CAS CONFLICT with write INDETERMINATE.
Mapping occurs at the actual read/commit boundary.
Error encodings contain no private tokens, Store keys, or hidden-source counts.

### 6.6 Core and Facet composition

The **Facet owner** coordinates composition; Composition only adapts ports.
Core evidence in P4 ContentRead is control evidence obtained by a fixed Facet service identity,
**not the client's Core read receipt**. It cannot become a client Core cursor,
and a client with only Facet content permission need not acquire observation permission over the whole Core.

Core provides that fixed service identity the minimum control port
`ReadAttachmentWithPreparation(nextHop, AcceptanceKey)`.
Both the existing `ucupa.core.element.facet-attachment-read` and new
`ucupa.core.element.facet-attachment-observe` must be explicitly granted in exact CoreElementScope.
Under section 6.2, the port atomically obtains current control evidence and private preparation,
tracking only this Element's existence, exact attachment/key, and contact revisions.
It discloses no spatial payload or other Elements.
Handles from `BeginAttachmentObserve / PollAttachmentChanges / ReleaseAttachmentObserve` are for that service identity only.
Service and client authorization failures are independently evaluated;
the service identity is not a privileged coordinator for the entire space.

Facet prepared Get/ListTypes follow this fixed order, without a cross-Store transaction:

1. Obtain the client's joint Facet read/Observe view.
   Under the Facet lock, capture F0, AcceptanceKey, and reverse-contact revision,
   and register pending local preparation so Facet commits during the Core wait cannot be missed.
2. Release the Facet lock. At that designated reverse address, use the fixed service identity
   to obtain current Core control evidence C0 and control preparation.
   No Facet commit lock is held while waiting for another owner.
   Core uses its own view and lock, not shared client authorization.
3. Relock Facet and verify unchanged F0 and original acceptance/contact,
   applicability of Core evidence to the exact key,
   no expired/lost segments in either preparation, and valid current client view.
   Failure cleans up both undelivered preparations and explicitly requires reread or reports unavailability.
4. Only success issues Facet preparation, privately binding F0, the local queue, and the Core control dependency.
   The client receives only its own Facet token, not the service view/cursor.
   Begin activation of both dependencies is idempotently retryable;
   one-sided failure must not issue a half-successful composite.

These bases are verifiable causal dependencies, not a claim that C0 and F0 were captured simultaneously.
Every later Core delete/detach/contact change enters the internal queue;
detecting it does not require a later Facet write.
Poll first checks dependency continuity and the current exact relationship, then discloses Facet content notifications.
An established invalid relationship stops the composite stream; a service dependency GAP causes composite GAP;
temporary unreachability returns UNAVAILABLE.
Without background polling, change detection occurs at the next Poll, not within a promised push latency.
It cannot return an empty batch and falsely claim the dependency remains proven valid.

Every Facet-notification read continues to validate the current exact attachment,
rather than delivering old namespace content/details without checks.
A new attachment, enabled new type, or newly disclosable Facet source first needs its own legitimate read basis,
then its own cursor; it is not treated as observed since the old Core cut.
Unproven continuity yields GAP/PARTIAL;
an invalid required root or exact attachment yields UNAVAILABLE.
An address change does not change fact identity, but an old contact cannot be silently replaced to continue.
Revalidate through the new contact and establish a new basis if continuity cannot be proved.

Service-control changes cause revalidation or composite invalidation, not a fabricated Facet content commit.
A client with its own Core read/Observe permissions may establish and consume its own Core stream separately.
Client-visible composition contains only the owner/cursor vector that client may know.
Without Core observation permission, the service-control stream's sequences, events, view, and cursor remain private;
a valid Facet token cannot borrow its authority.
Release, expiry, or client revocation makes Facet release its own resources and internal derived Core dependencies.
Disconnected peers fall back to their absolute deadlines for reclamation.
There is no unified Core/Facet order or atomic starting point.
Even non-federated multi-source scopes must validate these rules;
a working Core Observe alone does not complete P5.

## 7. Propose: call the owner explicitly without a new transaction layer

Send the request to the actual owner through a verified entry point or attachment contact,
retaining owner-specific final tuple/payload, CAS, exact OperationID, and original outcome.
The corresponding owner still performs entry authorization, fact constraints, and final commit.

- A state change is acknowledged only after validation and reliable persistence; deferred saving must not manufacture query consistency.
- A lost response is not proven rejection. Retry with the same actor, OperationID, and canonical request, not a random new ID and another write.
- INVALID, DENIED, CONFLICT, UNAVAILABLE, and INDETERMINATE remain distinct from confirmed Committed/Unchanged.
- Contact refresh and identity establishment do not automatically retry non-idempotent business actions, alter original preconditions, or repair a conflict outcome into a new request.
- Multiple Core/Facet commits are reported separately; no Propose may pretend to provide cross-Store all-or-nothing behavior.
- Observe reflects only actual commits. A notification does not replace the original mutation outcome, and no notification does not prove write failure.

### Closure writes and outcomes after deletion

New public Core commands record `AccessBinding={mode,boundary?}` in the existing operation binding:
mode is exact or closure, and closure boundary is the original BoundaryAnchorRef.
It persists atomically with original actor, operation kind, canonical input, preconditions, and outcome,
not in another operation database.
Concrete permissions and subjects derive deterministically from operation kind/input.
No second authorization table, ancestry tree, old authorization view, or credentials are saved.
First admission of a new command requires a valid handle, current authorization, and same-cut membership evidence.

When authorization/membership checks first succeed, the same operation records AdmissionCut,
the owner cut before fact adjudication begins.
Store validation checks historical membership at that cut:
the Committed pre-state, or the decision state for Unchanged/recorded Rejected, must match AdmissionCut.
Its value must be cross-checked against the sole commit history; a self-reported marker is not proof.
Pure authorization/scope-membership rejection creates no persistent credential that bypasses authorization.
Retainable fact-decision outcomes still follow the original rules.

An exact retry with the same OperationID first identifies the original binding,
then checks access to its outcome under the concrete permissions of the same current scope.
It must not demand that an already deleted object still belong to the current closure before looking up its outcome:
that would prevent a successful deletion with a lost response from ever converging.
Current-scope authorization discloses only this actor's original operation result,
not a general query over historical data.

For expired handles, restarts, or even deletion of the original boundary, provide read-only
`RecoverOutcome(reference, caller,
operationId, exactOriginalRequest)`.
It checks the original Core identity only at the reference's designated address,
checks current permissions for the original scope,
and returns an existing outcome for the same actor, AccessBinding, and canonical request.
It **never executes a new command that was not found**, creates no new handle, and restores no retired Anchor.
The same Principal established with new credentials may recover;
old credentials/views are not a permanent dependency.
No disclosable outcome for that actor returns OUTCOME_UNKNOWN, not Element NOT_FOUND.
A mismatched request or boundary for the same actor returns CONFLICT without original-command details.
Other actors' outcomes cannot be disclosed. Revoked original permission returns DENIED;
OperationID cannot lend authority.

This extension to the original record directly uses the sole current encoding,
with historical validation and restart tests, not an old-record interpreter or legacy-scope compatibility entry.
Facet exact key/namespace replay order retains P4's rules;
public types and observation do not add parsing or current-existence prerequisites.

## 8. Public values, opaque content, and canonical encoding

The following fixes fields and validation rules for the P5 local public profile; format choices are no longer TBD.
Implementation places machine schemas/conformance files in the owning `api/`;
needed generated bindings follow the engineering rules.
These artifacts implement this contract without new version paths, old readers, or fallback formats.
Empty directories are not created in advance.

### 8.1 Values and evidence

Public request/response envelopes are BOM-free UTF-8 [JCS / RFC 8785](https://www.rfc-editor.org/rfc/rfc8785) bytes.
The receiver strictly parses them, then requires byte equality with JCS re-encoding.
It rejects duplicate/unknown envelope fields, invalid Unicode, non-finite numbers, and noncanonical input.
JCS performs no Unicode normalization; ordinary `json.Marshal` does not prove conformance.
Spatial values retain accepted UUID, finite binary64/projective-quaternion, and explicit root Range rules;
message parsing does not normalize or adjudicate for the owner.
Omitted fields differ from null. A root omits Parent/LocalPose;
zero UUIDs or negative radii cannot substitute for semantic variants.

| Public value | Fixed representation |
|---|---|
| UUID, FactAuthorityRef, RealmID | Existing accepted exact identity text; trust does not derive from addresses, names, or self-reported envelope information |
| Binary fields, `payload` | [RFC 4648 section 5](https://www.rfc-editor.org/rfc/rfc4648#section-5) base64url string, only `A-Z a-z 0-9 - _`; no padding/whitespace, zero unused trailing bits, and byte-for-byte equality after decode/re-encode |
| OperationID | Existing exact 1–128 bytes encoded as that base64url; arbitrary Go string bytes are not forced into Unicode text |
| UpDepth, address up, ActualUpDepth | Canonical decimal strings, without plus signs, leading zeroes, or negative zero; UpDepth allows -1 or nonnegative int64, others nonnegative uint64; no JSON-number rounding |
| limit, resource counts, and other declared small integers | Nonnegative JSON safe integers within field limits; identity/Revision/cut must not be recoded as numbers |
| `typeRef` | Any opaque 32-byte value, decoded to exactly 32 bytes; publicly a 43-character unpadded string in that encoding, with no address or variable name; Facet interprets neither content nor origin |
| Public token | Exactly `{owner,kind,value}`; owner is the issuer's established identity, kind the field's exact purpose, and value a 32-byte opaque value in that encoding |

Token kinds are fixed as `space-handle`, `record-revision`, `attachment-revision`, `contact-revision`,
`read-receipt`, `query-continuation`, `observation-preparation`, `observation-cursor`, `change-id`,
and `change-basis`.
The authorization port's `authorization-view` is only for trusted ports, not directly attached to client responses.
Use sites check owner, kind, and issuance context.
Private Store scopes, cut serials, binding generations, and operation history are not published,
nor are existing CoreCutEvidence.Bytes published unchanged.
TypeRef is part of a content key, not a run token; caller changes and restarts do not change it.

An ordinary read receipt is a random 32-byte, caller/view-bound run-local reference.
The owner privately binds its actual cut and read conditions.
One read chain reuses it; independent reads issue new references even at the same cut,
so reference changes cannot reveal commits outside the closure.
Other run tokens are likewise role-isolated.
Section 6.5 bounds their context counts, bytes, and lifetimes without another TTL registry or default number of seconds.
A receipt retains no second spatial fact copy.
Expiry/loss yields INVALID_CONTEXT or GAP under section 6.5 and requires rereading,
not decoding a sequence number to continue.

Public Revision value is fixed as HMAC-SHA-256:
the owner uses a persistent random 32-byte private key for domain-separated authentication of
owner, kind, and the original private Revision in an unambiguous length-delimited encoding.
The key is an internal owner-Store encoding basis, not a new identity or authorization.
It is stored for the Store's lifetime and is not regenerated on restart;
public equality/CAS semantics remain stable for the same fact Revision.
A missing/corrupt persistent key is a Store-integrity error,
not grounds to generate another key and pretend recovery succeeded.
CAS calculates and compares the current candidate Revision tag;
it neither inverts the MAC nor scans all history.
The original expected tag enters the existing OperationID binding's canonical request unchanged.
Section 7's exact replay is identified before current CAS, without replacing the tag with the current one.
Run-local read receipts are not persistently replayable expected Revisions;
RecoverOutcome remains section 7's read-only outcome recovery.
A local address contains only designated NextHop, without implying network authentication, TLS, or verified remote identity.

### 8.2 Opaque Payload and capacity

Facet stores and returns bounded, byte-exact opaque Payload for `(Element, TypeRef)`.
It does not fetch or parse type artifacts, execute schema validation,
or condition acceptance on payload semantics such as JSON, text, or another format.
Bytes remain exactly unchanged between storage and return: Get returns what the caller supplied,
without Unicode replacement, normalization, reformatting, or default insertion.
The public envelope still undergoes section 8.1's strict JCS decoding,
and its payload field is decoded exactly as base64url.
Retry and original-result recovery do not reinterpret the bytes.

Capacity limits are fixed hard bounds. Violations follow the public error table and produce no content commit:

| Bounded resource | Hard limit |
|---|---|
| Decoded bytes in one payload | 1,048,576 bytes (1 MiB) |
| Current TypeRefs per Element | 256 |

Putting a new TypeRef when the 256 limit is reached, or an oversized payload, fails as a whole.
Overwriting an existing key still obeys the payload limit but does not increase the TypeRef count.
ListTypes returns the complete current set sorted by raw bytes.

### 8.3 Persistence and recovery

Content facts, tombstones, cut, public Revision key, and operation outcomes persist in the same transaction.
There are no per-Type pins, artifact mirrors, or witness records.
Deleting a key follows the original owner rules; there is no orphan-pin concept,
and retaining history does not disclose deleted content.
Restart recovers the sole Store and revalidates history before serving Get/Delete/exact replay,
without an online external type source. Mirror-integrity failure is not an error category.
Facet solely defines its Store format marker.
Old-format Stores are strictly rejected on open, without compatibility reading or conversion.

### 8.4 Errors and independent validation

| Situation | Required result |
|---|---|
| Invalid request encoding, TypeRef length, or public-structure hard bounds | INVALID; zero content commits, with rejection outcomes still following original owner rules |
| No current ListTypes/Get permission, or invalid old view | DENIED or context invalidation under sections 4/6.5, without first confirming key existence |
| The currently authorized namespace/key lacks that TypeRef | NOT_FOUND; the same TypeRef at another Element does not authorize disclosure |
| Payload exceeds 1 MiB, or the Element already has 256 current types | RESOURCE_EXHAUSTED; zero content commits and no effect on existing keys |
| Execution budget/public capacity exhausted | RESOURCE_EXHAUSTED; no content commit |
| Store-integrity failure | Public UNAVAILABLE, without private paths/bytes, automatic repair, or continued service from corrupt state |

An independent corpus covers JCS numbers/signed zero/Unicode ordering/duplicate keys;
base64url padding/unused bits/truncation; tokens across caller/owner/kind and restart;
stable Revision after restart and exact retries with old expected tags;
ListTypes → exact Get opaque round trips by clients without preinstalled business code;
namespace/key overreach, revocation, detach, and concurrent F0;
non-JSON bytes, 1 MiB/256 boundaries and overflow, and every resource boundary;
failure before/after the first Put commit and complete-Store recovery of Get/Delete/exact replay after restart;
and Store corruption preventing invalid loading.
Canonical public envelopes/base64url do not prove any business format for Payload; Facet makes no such judgment.
Implementation directly updates affected record/history validation, tests, and examples,
without legacy TypeRef/witness/public-format readers, mapping directories, or conversion services.
These are constraints of the accepted contract;
existing P4 opaque foundations alone do not establish completion of the public profile.

## 9. Failure-oriented acceptance

The following are implementation acceptance requirements.
Actual test paths, execution results, and delivery state are in the development plan's P5 implementation record.
This table's existence does not prove passing tests.

| Counterexample | Required observation |
|---|---|
| reference self-reports Principal, Realm, or another Core | Trusted assembly cannot be replaced; no existence disclosure without authorization |
| Missing address or designated address reaches the wrong object | Failure, without Ref scanning or automatic owner substitution |
| A child with a previously ungranted UUID is created after Enter | Legitimate closure Discover finds it; not a demo with preinjected IDs |
| UpDepth=-1 or Basis/path attempts to cross the published boundary | Stop at the proven boundary or return INVALID, without exposing the external parent chain |
| Siblings A/B beneath boundary R; Start=A, UpDepth=0, Basis=B | INVALID; being within the boundary does not relax original Basis∈Scope |
| Boundary/target reparented and old path reused | Recheck every edge, without silently repairing via bare UUID |
| Discover only, without spatial Read/Facet permissions | Only that behavior's disclosable content, with no write permission or implicit Facet access |
| Create-only closure permission, or update without place | Create intent can enter and create without reading; update without place is DENIED even when both endpoints are in the closure |
| Grant deleted and regranted, with both endpoint booleans true | Old view/cursor invalidated; no catch-up of history from the unauthorized interval |
| Read remains allowed, but Observe revoked/regranted before first Begin | Joint preparation view invalidated; new Allow at Begin cannot authorize historical delivery |
| Ordinary Read without preparation, then observation from its old cut | Explicitly require a new prepared Read; no invented eligibility at the old instant |
| C+1 committed between initial read C and BeginObserve | First Poll includes C+1 or explicit GAP, not a start from latest |
| Every interleaving of capture C, register preparation, commit C+1 | No window with captured C but missed C+1 registration; failure leaves no half-preparation |
| Initially no match, then create/move into scope/parent movement/Range expansion | Relevant invalidation; requery discovers new results |
| Ancestor moves without EntityRevision changing | Spatial observation still invalidates; unchanged EntityRevision does not mean an unchanged world projection |
| Spatial and attachment effects in one commit | One group; consumption cannot stop between effects |
| Additional Facet rejected/unchanged/replayed operation | No new fact notification or falsely advanced fact order |
| Batch response lost, followed by request with old cursor | Stable ChangeIDs may repeat, but the next segment is not skipped |
| Derived notifications lost after durable commit | Original write remains; observation GAP, not silent continuation |
| Enqueue/failure marking races with Poll | Deliver only a verified continuous prefix or GAP, without crossing a gap |
| read→observe with snapshots disabled and enabled | Both follow observation retention rules; snapshots are not silently enabled |
| Slow consumer, expiry, release, restart | Explicit old-context GAP/invalidation, without losing current facts/outcomes |
| Use nextCursor, reclaim consumed prefix, then reuse original cursor | Old cursor GAP; continuous new cursor remains valid; no changes does not produce another cursor chain |
| PublicAccessPolicy omitted, TTL missing/invalid, or resources exhausted | Explicit public unavailability/INVALID/resource failure; ordinary commits unaffected and no silent snapshot enablement |
| Temporary disconnection with unknown segment loss | UNAVAILABLE; after recovery decide continuation or GAP, without advancing cursor |
| Insufficient scan work while history still exists | RESOURCE_EXHAUSTED, not a fabricated data gap or COMPLETE |
| Core detach/delete without a new Facet write | Composite observation still stops/revalidates; old namespace is not continually disclosed |
| Caller has only Facet permission; Core authorizes only the fixed Facet service identity | Legitimate Facet observation works, but service Core receipt/view/cursor cannot be transferred or borrowed |
| Content/contact changes after F0 while waiting for Core; only one preparation succeeds | Prepared Read fails with cleanup/expiry reclamation, without a half-composite or fabricated common cut |
| New Facet source or newly authorized source | New read basis and independent cursor; no history leaked along an old hidden position |
| Restricted/relevant unsupported mount branch | Preserve PARTIAL/UNAVAILABLE, not a complete empty set |
| Hidden/outside-boundary facts change continuously | No public details, inferable sequence numbers, or notification frequency driven by hidden changes |
| Extreme numbers, malformed tokens, cross-role tokens | Deterministic rejection or numeric gap, without overflow or silent normalization into another request |
| Unfamiliar TypeRef and arbitrary non-JSON bytes | Authorized byte-exact storage/return, without preinstalled mappings, content parsing, or rejection of unrecognized bytes |
| Payload exceeds 1 MiB or Element receives a 257th type | RESOURCE_EXHAUSTED; zero content commits and existing keys unaffected |
| Replay of the same payload and reads after restart | Original bytes every time, without parsing, normalization, or re-encoding |
| Store or public Revision key corruption | Store-integrity failure, without automatic repair/regeneration or misreporting as temporary type unavailability |
| Successful deletion with lost response, then restart or deletion of original boundary | RecoverOutcome recovers only the actor's original outcome; no new command or second fact change |
| Changed boundary, actor, or canonical request when recovering one OperationID | No reinterpretation of original AccessBinding or disclosure of another actor's outcome |
| Derived handle/connection/observation caches cleared | Reentry, reading, and observation remain possible without a client world copy as the source of truth |

## 10. Delivery gates and closure boundary

Review gaps and ordinary engineering choices have been resolved into concrete rules accepted by the user.
Implementation, full verification, and authorized delivery acceptance for this increment are complete.
The following delivered artifacts/evidence are maintenance requirements, not a new open-ended plan
or a request for users to choose libraries, field names, or directories:

1. Produce the owning modules' public schemas and independently consuming examples using sections 3–8's fields/encodings.
   Verify extremes, identity/role isolation, and absence of historical compatibility formats;
   Go aliases or forwarded Store JSON alone are insufficient.
2. Implement exact/closure AccessBinding, concrete PermissionID combinations,
   Realm-view acquisition/validation, and cross-checks against decision history.
   Verify write-only entry, post-deletion outcomes, revoke/regrant, and restart,
   without copying Grants or implementing Stage 2 contact records in this increment.
3. Implement prepared Read, in-place Begin, bounded queues, and GAP rules;
   verify section 9's interleavings and resource boundaries.
   Core/F0 retain independent cuts; Facet owns its internal control-observation composition,
   and Composition does not take ownership of that state.
4. Implement opaque Payload storage, return, capacity limits, and recovery under section 8.
   Verify positive/negative cases, byte invariance, and independent decoding;
   a library's default behavior is neither the contract nor interoperability evidence.

Source placement follows the [engineering rules](../../ENGINEERING.md):
Core entry/closure/observe and Facet observe/content are split by responsibility,
while each owner retains sole state and adjudication.
Facet also owns private Core control-observation dependencies and their composite lifecycle.
Realm retains identity/authorization ownership; Composition only wires ports,
without a second set of composite facts or owner permission decisions.
Only genuinely public values enter shared.
Schemas/conformance belong in the owning `api/`; no empty SDK, transport, or bindings directories are created in advance.
Cross-owner counterexamples belong in acceptance, private commit faults in their owner module;
ordinary consumers cannot access Store or private history.

A partial P5 implementation must state its coverage.
Complete P5 delivery requires the corresponding gates for all five public behaviors,
local opaque-content validation, and declared multi-source Core/Facet observation.
Design confirmation, implementation authorization, technical verification, acceptance, and commit are recorded separately.
Contract closure and design acceptance do not replace implementation delivery/verification;
code-start and commit authorization are separately recorded.
This contract authorizes no remote service, downstream business, later Phase, or automatic commit.
