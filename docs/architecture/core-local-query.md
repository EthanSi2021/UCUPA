# UCUPA Core local spatial-query contract

**Status: P3 is implemented, has passed full verification, and has been accepted by the user. This is the sole current local-query implementation contract. The architecture authority manifest maintains its current classification; the development plan records delivery evidence and commits.**

This document revises the existing QueryRegion directly; it does not maintain two query specifications or compatibility entry points. Acceptance covers the complete current query form, including depth and optional snapshots. The [development plan](development-plan.md) preserves the history of the earlier single-Core query commit that had not been independently accepted. It does not retroactively claim that implementation satisfied this contract, and no predecessor query entry point is retained.

Spatial values, containment, commits, and ordinary reads follow the [local-truth contract](core-local-spatial-truth.md). [Mainline D10](ucupa-concept-mainline-baseline.md#d10-structural-discovery-and-spatial-query-use-one-explicit-universe) governs their semantics. Cross-Core paths and snapshot composition are defined only in [mount-contract section 5](core-mount-contract.md#5-reads-paths-and-combined-cuts). This contract does not duplicate the Element model, mutation decisions, or a separate query algorithm. The [authority manifest](README.md) distinguishes target contracts from current ones.

## 1. Scope and invariants

- Create, update, and delete still confirm success only after validation and reliable persistence. They do not wait for queries to finish or artificially delay persistence by ten seconds.
- Ordinary ReadElement reads the latest committed state by default. Pagination remains fixed to the fact cut selected at the first page and never incorporates later state.
- With snapshot retention disabled, each page verifies that the original cut is still current. Only when retention is enabled may a time-limited old view be kept across requests, allowing pagination across writes.
- There is no arbitrary-timestamp query, permanent historical Store, or global timeline. Disabling retention does not disable queries or pagination.
- Queries do not change Elements, Revisions, CoreReadCuts, OperationIDs, or the persistent Store. Bounded temporary read state is allowed.
- Local queries handle only structures that can be closed within this owner. On remote coverage or when traversal needs to cross an external parent, P2's ErrMountContextRequired rule prevents omissions: remote coverage must not be treated as empty, nor an external parent as a root.
- This slice adds no Realm authorization model, separate Route, network service, spatial index, Facet filter, or real-time simulation.

## 2. One depth: select Scope without truncating descendants

The request distinguishes three things: Start is the starting point for upward traversal; Basis is the query sphere's coordinate reference; Scope is the resolved search set.

1. UpDepth is a signed integer. Omitted or 0 selects Start. A positive value specifies **the maximum number of actual parent edges to traverse upward**. **-1 means unrestricted upward traversal to this space's proven true root**. Values below -1 are invalid. A positive value beyond the root also stops at the root. ActualUpDepth is always the actual nonnegative number of levels traversed, never -1. There is no separate downward depth. -1 is a query option, not an infinite Range value or a second spatial truth.
2. Scope is the highest selected Anchor. Search that Anchor and all its descendants for Elements intersecting the query sphere. Entities directly under ancestors, sibling Anchors, and their descendants are in this set; the entry branch does not exclude them.
3. Reaching the depth limit completely defines a smaller semantic scope. Anything outside Scope is not a gap in this query. Complete applies only to the returned Scope. Scope is this space's root closure only when -1 is selected and the root is successfully proven. Unrestricted upward traversal does not imply unlimited execution resources. Failure to establish the necessary root context must fail, not turn an intermediate stopping Anchor into Scope.
4. An unconfirmed preceding parent edge must not be presented as "the root has been reached." Cross-owner traversal requires path context; missing connections and insufficient authorization produce their corresponding failures.
5. Basis must be Scope itself or a descendant Anchor and may lie outside Start's subtree. A deep Basis does not implicitly narrow Scope. Start, the ancestor chain, Scope, and Basis all come from the read view at the same selected cut.
6. Crossing Core boundaries adds no structural edge. Deployment-induced forwarding hops are not UpDepth, and numeric proof-work limits are not semantic depth.
7. Changing Start, UpDepth, Basis, the sphere, or the selector starts a new query; an old cursor cannot continue it.

The choices are fixed: -1 is unrestricted upward traversal, 0 is the default, and positive values saturate at the root. Start and Basis are explicitly distinct. In the current local profile, traversal ends at the true root. If a future SpaceHandle declares a boundary, traversal must not cross it; the mount contract defines the path rules. Neither the receiving Core nor an arbitrary connection entry point constitutes such a boundary.

## 3. Local calls and results

The target public surface is below. It reuses existing value types; concrete Go declarations must be checked against engineering rules during implementation:


```text
RegionQuery = {
  StartAnchorID, UpDepth: int64, BasisAnchorID, Sphere, Selector
}
QueryRegion(actor, query, at *CoreReadCut, limit, continuation)
  -> QueryPage, error

QueryReadMode = CurrentVerified | Retained
QueryPage = {
  Matches, Cut: CoreReadCut, ReadMode: QueryReadMode, Snapshot: CoreReadSnapshot?,
  ScopeAnchorID, ActualUpDepth, Status, Gap, Continuation?
}
Match = { ElementID, Kind, Revision, Path }
ReadElementSnapshot(actor, ElementID, snapshot CoreReadSnapshot)
  -> ElementRead, error
ReleaseReadSnapshot(actor, snapshot CoreReadSnapshot) -> error
```

- Sphere remains a finite Vec3 Center and finite nonnegative Radius, with the existing canonical rules; -0 normalizes to +0. Selector remains Anchor/Entity/Both. The limit remains 1..256 and caps only the current page's output count.
- On the first call, continuation=nil. With at=nil, capture current; an explicit at may be captured only while that cut is still current. A bare cut does not authorize searching the journal for history. This QueryRegion entry point does not expose arbitrary old-cut queries.
- On continuation calls, at must be nil. The read mode, fixed cut/Scope, optional snapshot, query identity, and position come exclusively from the continuation. The query must be canonically equivalent; the limit may change within the valid range.
- The continuation retains the requested UpDepth. -1 and a positive value that happens to reach the root are still different requests and cannot be interchanged merely because their current Scope matches. Validate UpDepth >= -1 before counting; do not cast -1 to a large unsigned integer to simulate traversal. Check accumulation overflow for both actual depth and structural-address upward steps; counters must not wrap into another path.
- CoreReadCut labels only a fact cut. CoreReadSnapshot is a private handle to a retained view in this run, associated with a cut, fixed lifetime, and resource occupancy. It is not a new fact Revision, network bearer token, or authorization proof.
- ElementRead reuses the existing ReadElement result type and carries the same cut. ReadElementSnapshot explicitly selects retained state; it does not silently change ReadElement's at/current semantics. This interface reads only Elements whose membership in the retained Scope can be proven. Unknown IDs and out-of-Scope IDs both return ErrInvalid; a bare UUID without parent context cannot prove historical absence. An internal contradiction in a validated view returns ErrStore. Under Retained, discovering an Element and passing only page.Cut to an ordinary read is insufficient to obtain its old state; use page.Snapshot. Under CurrentVerified, Snapshot must be absent. After discovery, ordinary ReadElement(at=page.Cut) succeeds only while the cut remains current. Do not return a zero-value placeholder handle or interpret an absent Snapshot as a historical-read handle; the Snapshot interfaces reject zero values with ErrInvalid.
- Release frees only a handle with valid shape and bindings belonging to the caller. Repeated release and cleanup after expiry succeed; incorrect Core/caller bindings are rejected. Release discloses no facts and does not require the caller still to hold a query grant. Other handles sharing the underlying view are unaffected.
- Local ListDirect and ProjectToAncestor gain no historical-read interface from this contract. The mount contract defines snapshot rules for cross-Core enumeration.

An existing Element's origin can still act as a probe. First use ordinary ReadElement to obtain Parent, Pose, and cut; use Parent as Basis and Pose.Translation as Center, then call QueryRegion at that cut. A commit between the two steps explicitly expires the read and requires restarting. Only Retained, after successful retention, remains valid across ordinary writes. The caller chooses the search radius; it is not automatically the read Element's Range. The caller also explicitly selects Start.

### 3.1 Optional retention and a fixed read mode

Each Core accepts optional SnapshotRetention duration configuration when created or started. A Go constructor parameter may use *time.Duration to express "not configured." The implementation must copy the value rather than retain a mutable caller-owned pointer that controls runtime policy. This slice fixes the configuration for the run and adds no hot switching. Future configuration files map to the same semantics; this slice does not require a service configuration-file system first.

| Configuration | Read mode and behavior |
|---|---|
| Not configured | CurrentVerified: no old spatial state retained across requests; queries and pagination remain available; each page verifies that the original cut is still current |
| Explicit finite positive duration | Retained: retain this read view for that duration; later writes do not change the original pagination basis |
| Explicit zero, negative, or unrepresentable duration | Creation/startup configuration error; do not interpret it as absent or add a default; reject overflow before text conversion produces an invalid value |

There is no default duration in seconds, no default left to implementation choice, and no infinite retention. These modes are runtime behavior of one API, not compatibility entry points. Retention capability and resource admission are separate: if enabled but capacity is insufficient, explicitly reject; do not succeed by switching to CurrentVerified.

- ReadMode is selected before the first page, and every later page binds the same mode. Retained must carry a valid Snapshot whose cut equals Page.Cut; CurrentVerified must have no Snapshot. Both fix the query, Scope, ActualUpDepth, and ordering position.
- CurrentVerified borrows a temporary consistent view per call and retains neither old records nor computed results between calls. A continuation first authorizes the cursor's fixed Scope, then checks that current equals the original cut. After computation, authorize again and recheck the same cut. Any confirmed mismatch returns zero results with ErrReadCutExpired; do not reselect Scope or continue ordering from a new cut.
- Absence of configuration is not an error, expiry, or completeness gap. With no writes, a valid run, and successful checks, pagination should complete normally. A call still has finite temporary-memory and computation allowances; disabling historical retention removes neither consistency nor resource boundaries.
- CurrentVerified cursors use size-bounded private value metadata and register no historical views. They bind the current run to reject pre-restart cursors. There is no retained handle to release and no historical TTL. Ordinary ReadElement with a bare at still follows the current fact-cut rules; cursor run binding does not change ordinary read semantics after restart.
- The mount contract defines all-source mode selection, combined metadata, and failure handling across Cores. An individual owner must not switch modes unilaterally.

## 4. Matches, ordering, and completeness

- Use exact closed-sphere intersection, including tangency. A center outside the query sphere still matches when its Range intersects. An Anchor uses its own conservative Range; an unbounded root matches when the selector includes Anchors.
- The selector filters output kind only; it must not truncate traversal needed to reach selected descendants.
- Match.Path is the downward UUID chain from resolved Scope: omit Scope itself and include the terminal Element. Scope's own Path is empty. Every edge actually exists in the view at the selected cut; a path is not a stable identity.
- Return ascending order by the 16 bytes of each canonical UUID. Kind, Revision, Path, and Cut share the same source. Do not return full records, Facet, Metadata, or a rounded projected Pose.
- ReadElementSnapshot still checks exact-read permission independently. A query grant does not automatically grant full-record reads. After authorization, the successfully read ID/Kind/Revision must agree with the original match; newly deleted or reparented state must not enter an old read.
- Each page may recompute results in the view at the same selected cut. Retained borrows the old view; CurrentVerified reacquires the original cut while it remains current. Retaining the entire match array or introducing a result Store is not required.

| More verified results remain for another page | Exact-numeric gap exists | Status / Gap / continuation |
|---|---|---|
| No | No | Complete / None / absent |
| Yes | No | Partial / None / present |
| Yes | Yes | Partial / ExactWorkLimit / present |
| No | Yes | Partial / ExactWorkLimit / absent |

Complete means that **the union of the same pagination chain** is complete, not that the final page contains all results. Only a Complete first page with empty Matches proves that the selected Scope has no intersecting Elements at that selected cut. Gap describes the entire query/cut, not the suffix after the preceding page's UUID; an over-limit candidate with an earlier sort position cannot have its gap cleared on the final page. Returned entries are always definite matches. Partial without a continuation is a terminal gap; do not manufacture a cursor that makes no progress.

## 5. Permissions and disclosure

PermissionQueryRegion remains ucupa.core.anchor.query-region, but its target is **CoreElementScope(this Core, the actual ScopeAnchorID)**. Authorization at Start does not automatically expand upward.

- A query grant permits match summaries and necessary paths throughout the Scope closure. It does not grant exact reads, writes, or Facet access. Add no implicit per-Element hiding; silently omitting unauthorized branches and then reporting complete is forbidden.
- Resolve the actual Scope privately on the local side solely to construct the authorization requirement. Before permission is granted, disclose no Scope, ancestor chain, cut, existence, or match. Successful resolution checks only the actual Scope's query grant; it imposes no additional Start grant. If a local Scope cannot be formed—for example, Start is missing or not an Anchor, an external parent is required, or Core is unreadable—first check the exact requested Start's query grant as permission to disclose that failure. Without it, always return ErrDenied; do not reveal state by returning Invalid, MountContextRequired, or Store first. With permission, disclose the corresponding failure only after rechecking the same grant. This is a failure-diagnostic gate, not permission to search ancestors through a Start grant. External parents still follow the path contract; there is no network-wide UUID lookup.
- Admit computation only after authorizing the actual Scope, and allocate a formal retained handle only under Retained. If the resolution view differs from current at capture, return zero results with expiry; do not silently change Scope within the call. Continuations reauthorize the Scope fixed in the cursor directly.
- Each page and snapshot exact read first authorizes outside the lock, computes after borrowing an immutable view, then authorizes again before disclosure. Finally check that Core is readable and the run/read context is valid. Retained checks the handle and expiry; it does not require current to equal the old snapshot cut. CurrentVerified must check current against the original cut again. Perform the owner's final checks after the external authorization returns.
- Snapshot exact reads are restricted to the query's authorized, retained Scope. Anything outside it uniformly returns ErrInvalid; the interface does not become an arbitrary historical-record reader.
- The external Authorizer never runs under a Core lock or Store transaction. A bool false still maps to ErrDenied; do not invent distinctions the permission backend cannot supply or fabricate a Realm authorization version.
- Permissions are not frozen facts. Holding a snapshot does not bypass subsequent revocation. However, the bool port does not prove a revoke-and-regrant ABA distinction or provide atomic authorization across Realm/Core Stores.
- The snapshot fixes the old structure's scope meaning. New authorization checks still target the same exact Scope; current's new parent must not rewrite the original authorization closure. An independently authorized historical read does not claim the object still exists now.

## 6. Bounded snapshot lifecycle

Only Retained uses the following **on-demand retained views in owner memory**. They change neither Store encoding nor the write path. CurrentVerified uses a per-call temporary view as specified in section 3.1; returning, cancellation, or failure releases the borrow, and temporary computation remains subject to finite admission limits.

1. Capture immutable spatial records and required mount/contact state with a cut at the local commit-publication boundary. Immutable data may be shared, but do not retain an entire coreState containing full receipts, operation history, and commit history. Do not hold a Core lock, bbolt read transaction, or write-blocking database snapshot throughout pagination.
2. Bind a handle to this Core, this run, caller, allowed Scope, fixed cut, and expiry. Underlying state at the same cut may be shared, but authorization is not shared; another caller cannot obtain that handle merely by sharing a cut.
3. When enabled, the fixed TTL is the explicit finite positive duration supplied by trusted assembly. Do not accept infinity or hard-code ten seconds as semantics. Without configuration, this lifecycle is not entered. Each handle has an independent expiry; a new handle at the same cut does not refresh an old one. Reclaim a shared view only after all handles end and all borrows exit. Measure from initial capture using the owner's local monotonic clock. This first implementation neither renews leases nor extends them on pagination. Check expiry at every borrow and final disclosure. There is no global wall clock or cross-owner timestamp comparison.
4. Bound active handle count, retained record/path byte accounting, and concurrent borrows/computation together. An expired view with an outstanding computation borrow still counts toward occupancy. Count shared storage only once for the state actually retained. Declare the accounting basis; do not present logical bytes as an exact process-memory ceiling.
5. Check and reserve capacity before allocation. If the limit is exceeded, reject new queries with ErrSnapshotCapacity; do not evict handles within their promised lifetime, enlarge limits, or change write semantics. Admission/capture failures release reservations; cancellation/expiry ultimately reclaims borrows and temporary results. Continuations and snapshot exact reads must also obtain finite borrow/computation allowances. Insufficient allowance likewise returns zero results with ErrSnapshotCapacity: no infinite queue, release of the original handle, re-pin, or TTL change. The same request may be retried before expiry. This is not a general scan-count/CPU-deadline budget system.
6. A successful final page **does not automatically release** its snapshot: full Elements still need to be read at that same snapshot afterward. The caller explicitly releases it when finished; otherwise TTL reclaims it. Release first invalidates the handle for new borrows and final disclosure, then reclaims it after borrows exit safely. Waiting must hold neither Core nor registry locks, extend no readable lifetime, and invalidate no other independent handle.
7. Normal reopening still restores the same AuthorityRef and fact cut, but does not restore run-local handles. After expiry, release, shutdown/restart, or handle loss, old pages/exact reads explicitly fail. Do not replay the journal to recover a handle or switch to latest. A prior-run handle from the same legitimate owner returns ErrReadCutExpired, not a new argument error merely because its run marker differs. Illegal bindings such as the wrong owner/caller still return ErrInvalid in the required authorization order. This run binding governs only private read-state lifetime; it is not a durable writer epoch, replication fence, or Authority identity change. Close also ends an in-memory Core's run: release read resources, return ErrStore for subsequent ordinary operations, and return ErrReadCutExpired for legitimate old read handles/cursors. Close is no longer treated as a no-op for memory Core.
8. Failure or lack of authorization publishes no usable handle; promptly release newly created handles that were never delivered. Failed use of a delivered handle does not implicitly extend its lifetime. Already delivered old pages remain old observations and cannot be joined to a new chain to claim Complete.

A continuation privately binds the Core run, caller, normalized query (including Start/UpDepth/Basis), actual Scope and ancestor basis, fixed cut, ReadMode, optional snapshot, and last UUID of the preceding page. Replay is stable under the same permissions and valid read basis and must not count duplicates toward the set. Ordinary writes do not expire Retained: pagination **already captured and not yet expired** can complete during continuous writes. CurrentVerified continues only while the original cut is current and the run is valid; after a write, start a new query. There is no guarantee that an arbitrarily large query finishes within finite TTL, and short-term retention must not be presented as infinite resources or permanent history.

## 7. Numbers and cost

Continue using a conservative full-scan, exact-filter, UUID-sort implementation with no persistent index or background precomputation. Pagination limits output only; it guarantees neither O(limit), a fixed scan count, path length, CPU cost, nor total latency.

- Exact-work limits remain 1024 edges per relevant projection chain and 65536 bits for a reduced rational numerator/denominator. These bound proof cost, not valid structure depth. Deployment crossings do not reset them, and they are not downward search depth.
- If the necessary Scope/Basis coordinate context cannot be established, fail the entire call with ErrUnrepresentable and return no page.
- If the context is established but an individual selector-relevant candidate exceeds the limit, mark ExactWorkLimit for the whole query and continue with other definite matches.
- An unselected kind may skip its own intersection calculation, but retain the structure and coordinate chains required by selected descendants.
- A corrupt retained view or unreadable Core returns ErrStore for the whole call; do not wrap untrustworthy facts in Partial.
- Every fully computable case must equal the unpruned oracle on the view at the same selected cut. An all-success-or-all-failure oracle is not the public implementation. Inability to output a finite binary64 Pose does not imply inability to prove intersection exactly.

## 8. Failure rules

Any non-nil error returns a zero-value page/read result without staged matches, paths, or cut.

| Condition | Result |
|---|---|
| Invalid query (including UpDepth < -1), limit, token shape/binding, or an at supplied with a continuation | ErrInvalid; resource-binding checks follow the authorization order |
| Local Scope cannot be formed and exact Start failure-diagnostic permission is absent | ErrDenied; do not distinguish absence, kind, external parent, or Store state |
| Failure-diagnostic permission granted, but Start is missing or not an Anchor | ErrInvalid |
| Actual Scope query or exact read is not currently permitted | ErrDenied, with no existence conclusion |
| Basis is absent, is not an Anchor, or is outside the actual Scope | Uniform ErrInvalid after authorization; do not probe another scope |
| First-page specified cut is no longer current; the depended-on cut changes before formal capture; or CurrentVerified's original cut is no longer current before/after the call | ErrReadCutExpired |
| SnapshotRetention not configured and the original cut remains verifiable | Normal CurrentVerified query/pagination; no error or Gap |
| Required Retained handle expired, released, or expired before disclosure; or either mode's old cursor was issued by a run that has ended | ErrReadCutExpired; do not read latest |
| Snapshot admission/resource reservation exceeds capacity, or continuation/exact-read borrow allowance is insufficient | ErrSnapshotCapacity; create no new handle and preserve existing handles and expiry |
| External parent or remote coverage required | ErrMountContextRequired |
| Necessary coordinate context is unrepresentable, or Core is unreadable | ErrUnrepresentable or ErrStore, respectively |

An old Retained handle remains valid after ordinary writes; do not expire it solely because current changed. CurrentVerified must reject a changed cut. Failure priority is stateless validation → trusted bindings and authorization → state-dependent outcomes → pre-disclosure checks. Do not interpret another party's token contents or expose capacity details before checking the caller.

## 9. Required acceptance tests

1. Test depth -1, 0, omitted, 1, multiple levels, beyond the true root, and below -1. -1 reaches the proven root; positive values saturate at the root; ActualUpDepth is the actual count. A -1 request at the root returns actual 0. Invalid negative values fail without state access, and an external parent is not treated as a root. Include rejection of the minimum int64 value, cursor mixing of -1 with a positive value selecting the same Scope, and interrupted/expired upward traversal that must not be truncated into success.
2. Cover Entities directly under ancestors at every level, siblings and deep descendants, and large Entities with centers outside but intersecting the query sphere. Selectors do not truncate traversal.
3. Basis may lie outside Start's subtree while inside actual Scope. Changing depth creates a new chain; an old UUID cursor cannot be reused.
4. Under Retained, change Pose, Range, Parent, create, or delete Start/ancestors/Elements after the first page. Old pages and snapshot reads preserve old state; new queries/ordinary reads see new commits. Do not copy unpersisted state or delay write confirmation.
5. In both modes, the multi-page union has no omissions or duplicates; the limit may change, and replay under a valid context is stable. Retained permits snapshot exact reads after the final page and fails after explicit release. Ordinary reads after a CurrentVerified final page still validate the original cut.
6. A race between the Entity-origin probe's initial read and capture explicitly expires the query. After Retained successfully pins the view, later writes no longer cause failure.
7. Upward traversal does not expand authorization. Without Scope permission, reveal no resolution result. Missing/non-Anchor/external-parent Start without diagnostic permission returns the same ErrDenied as absence of a Scope grant. Query permission without read permission returns summaries only. Revocation during long scans/exact reads is rejected before disclosure; the Authorizer remains outside locks throughout.
8. TTL boundaries, expiry during computation, normal restart, handle loss, and incorrect Core/caller/query never fall back to latest. A genuinely unreadable Store retains its fault semantics; do not disguise every failure as expiry.
9. Deterministically test full capacity, concurrent admission, shared views, release/expiry during borrows, and failure cleanup. Rejecting a new query does not affect fact writes or silently evict an already promised handle.
10. Cover the four pagination forms for numeric gaps, sticky Gap from an over-limit candidate with a smaller UUID, and rejection of continuations that make no progress.
11. With M=MaxFloat64, place an Anchor under root at x=M/range=M and its Entity at local x=M/range=0. A query with root Scope/Basis and Center.x=M/radius=M must match the Entity whose exact center is 2M, without requiring a finite output Pose.
12. Complete cases agree with the same-view oracle. Partial cases use independent exact vectors to prove returned entries are correct and other computable matches were not omitted.
13. Queries do not change Store encoding/content, Revisions, cuts, receipts, or OperationIDs. The observable temporary-snapshot lifecycle must not be presented as new persistent truth.
14. Test absent configuration, explicit positive duration, zero/negative/overflow, and mutation of an external configuration pointer. Without configuration, no spatial pin is retained across requests; pagination can complete without commits, and commits before computation, during computation, or before final validation explicitly expire it. In both modes, Snapshot presence/absence, cut, cursor mode, and run bindings agree. Disabled retention is not misreported as expiry; insufficient capacity does not downgrade the mode.
15. Pass the public-surface allowlist, file-size checks, existing Core/Facet/Realm regressions, and full-repository test/race/vet/dependency/example gates before submitting for user acceptance. Design review does not replace runtime verification.

Confirmation of the target contract and implementation authorization are recorded separately in the development plan. Previously passing current-cut query tests do not establish the snapshot tests above.
