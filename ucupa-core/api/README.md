# Core public representations

This page describes the local representation still in use and the typed gRPC representation
in [core.proto](core.proto). `cmd/protogen` generates the sole protocol bindings.
The [network host and SDK](../../cmd/ucupa-core/README.md) do not accept a whole JSON envelope.
Both representations use the same owner rules and operation history; they are not protocol
versions or an old HTTP compatibility entry.

## Transfer control and evidence

P7.2 `TransitionControlService` provides source Begin/Inspect/Cancel/Recover through an
authenticated ordinary-user session, with independent export/commit/inspect checks at the
relevant steps. No original-caller string or user-forgeable peer evidence field is accepted.
operation_id is raw bytes. Begin expected is the original source Element's `record-revision`;
Cancel expected is original progress's `transition-revision`. Root has no final tuple;
slice requires the full final tuple. INTENT records original input, not preflight, fence,
installation, or completed transfer. This is not a local JSON command: local calls use
`instance.Transfers()` public values and the same commit/recovery results.

`TransitionApprovalService` provides target Core run-local preflight approval, not durable
Accept. The original requester establishes a target user session and submits source, raw
operation bytes, and the complete original command. The target independently checks
`ucupa.core.transition.import` at the exact target boundary and, for a slice, final-parent
`place-element`. Approval binds original caller/view, source identity, comparisons, final
tuple, and both specified addresses. A finite token does not disclose the view.
Check permits only the explicitly configured, certificate-authenticated original source service;
ordinary sessions, read delegation, and bare Ref do not substitute.

Expiry, session release, restart, or authorization change invalidates approval.
Revoke/regrant does not revive an old view. Successful Check proves neither final placement
nor offline installation permission. Later Accept still requires valid original-caller
authorization and full source evidence.

`TransitionSourceService` exposes read-only `ReadTransitionChunk`/
`ReadTransitionDecision`. Requests carry original binding and chunk position/`max_bytes`,
not caller-created fences or installation proofs. The fixed target uses mTLS; the source
checks its durable operation, target, and full original input, not an acting user session.
Positions start at zero. Chunks are at most 64 KiB; the complete manifest at most 1 MiB.
Responses bind original input, source final cut, digest, total length, and exact position.
The receiver collects all bytes and verifies the digest before owner acceptance.
Decision responses retain original target acceptance cut/revision and source decision
cut/revision. Transport success is not installation completion.

`TransitionTargetService` accepts/installs only original bindings; first acceptance additionally
takes a finite `approval_token`. The target reads actual evidence through its specified source
connection, not a caller-uploaded fence, manifest, or decision.
After pulling and before persistence, first acceptance rechecks target caller/view, approval,
session, and deadline. Revocation is not blocked by the pull lock.
Missing approval cannot create a new promise, but the fixed source may recover an already
durable exact acceptance. Installation does not revive old approval.
Acceptance retains original fence and target cut/revision; installation retains decision,
installation, and completion cut/revision. Cross-owner comparison remains the source's,
not a re-signed target token. The SDK checks complete response binding.

Store errors after write dispatch are INDETERMINATE; evidence-fetch failures are UNAVAILABLE.
Recover supplies the full original binding and target approval token. The source checks its
durable original input even after completion; a changed binding is not a retry.
The ordinary session bounds this advance: release/expiry during a wait stops uncommitted
steps without discarding durable obligations. Callers do not orchestrate fence/chunks/accept/install
or upload their evidence. Each advance rechecks disclosure permission. Undecided export or
binding changes separately check original-user permission; fulfilling decided obligations
does not require reviving old approvals.

The accepted first P7.2 delivery is bounded: single Entity, owner-local Anchor closure without
uncontinued outgoing external edges, root successor, and simple carrier replacement.
See the [current scope](../../docs/architecture/development-plan.md) and
[lifecycle contract](../../docs/architecture/owner-transition-contract.md), not older partial
adapter tests, for overall completion. Nested carrier transfer, positive continuation of outgoing
external edges, and implicit multi-owner atomic migration are outside this delivery.

## Parent-side carrier control

Parent `c.Transfers()` and the same `TransitionControlService` provide
`BeginCarrierTransition`, `InspectCarrierTransition`, and `CancelCarrierTransition`,
using [CarrierCommand/CarrierStatus](../contract/carrier.go).
Parent is the control owner; source is the old carrier. The mount Anchor does not move.
Begin/Cancel independently require parent+mount `transition.commit`; Inspect requires
`transition.inspect`. Both recheck actual caller/view. Ordinary mount permission or source
export does not grant parent authorization.

Begin fixes comparisons, all three identities, and specified contacts. Exact retry returns
original reservation; Inspect returns current parent progress. `carrier-revision` is a
bounded operation comparison, not source fence, target approval, or installation evidence.
Cancel compares the original reservation. Once the parent permits fence dispatch, lack of a
network reply does not justify local cancellation.

These methods use gRPC/`client.Transitions()`, original sessions, and request cancellation
without a new service, switch, client recovery orchestrator, or private state.
Requests contain no self-reported caller, fence, or target acceptance proof.
Wire responses distinguish parent/source. The SDK checks parent, raw operation bytes, mount,
comparison kind, and progress. Begin preserves RESERVED; Cancel returns CANCELLED; Inspect
returns current state. Cancellation advances mount comparison revision, invalidating old CAS
for a new operation, but changes no spatial Element, carrier, or contact.
Uncertain Store writes remain INDETERMINATE, failed Store reads UNAVAILABLE, and session loss
prevents disclosure.

Those three controls alone perform only reservation/inspection/permitted cancellation.
`RecoverCarrierTransition` supplies the explicit supported three-party advance using source
and target approvals; it does not infer approval from a reservation. Real independent TLS-host
evidence for the bounded public chain is in
[three-party network tests](../tests/transfer/network_carrier_three_party_test.go).
It is not proof of real multi-machine or production operation.

## Parent-side evidence

`TransitionParentService` exposes `ReadCarrierPlan`/`ReadCarrierDecision` through
`client.TransitionParent()`. Only binding-matched source/target services with actual TLS
certificates may check the parent. Client-supplied identity, ordinary sessions, and self-reported
completion cannot replace service authentication.

Requests contain only the complete original binding. The parent reads its own reservation,
permit, cancellation, and unique binding, then rechecks the same actual participant before
disclosure. No decision means explicitly unavailable, not treating a reservation as a decision.
Responses preserve raw OperationID/actor bytes, per-party cuts/revisions, mount comparisons,
and specified addresses. A binding is at most 4 KiB and evidence at most 16 KiB.
Receivers check complete bindings and each owner's bounded evidence.

After Store restart, reads return original durable results, not new approvals. Ref routing or
caches do not replace an unreachable parent. Codecs convert bounded values only, not facts,
locks, or Stores; each original owner independently decides its state.
Parent-only network tests cover that boundary; the complete supported public carrier chain
has separate three-party evidence referenced above.

## Federation representations

P6 aggregate Read/List/Query returns `closure`: a canonical base64url digest in JSON,
32 bytes in Protobuf. It binds the original source combination and is unchanged across pages.
Local non-aggregate calls omit it. It is neither permission nor offline completeness proof
and cannot turn coverage gaps into COMPLETE.

Aggregate results also return `sourceGraph`, defined by the
[source-evidence schema](read-source-evidence.schema.json): identities and cut/authorization-view
commitments of disclosed authorized sources, parent-child mount evidence, and Query coverage
commitments. It includes no addresses, private basis, credentials, or hidden-source placeholders.
Query `parents` disclose `parent`/`adjacency` commitments for permitted Anchors in Scope
and hit paths, binding the full direct-adjacency page chain without child lists, page counts,
or private continuation. Pages of one query preserve the same set.
This is the authenticated entry's declaration of the original source combination, not a
geometric proof a consumer can independently recompute. See network contract §8.5.

The proto also defines `ReadDelegationService`: authenticated users may issue finite,
target-limited, client-certificate-bound read delegation. This changes network authorization
use, not local JSON commands or spatial decisions. See network contract §8.1.

## Local entry and encoding

Call `core.Access().PublicCall(ctx, caller, canonicalJSON)`, with `core` constructed/opened
by `ucupa-core/instance`, or use Composition's specified-address adapter.
Realm `EstablishCaller` (credentials) or explicit trusted local `TrustLocalCaller` establishes
`caller`; it is not in JSON. This local entry is not HTTP/TLS, and local trust is not remote
authentication.

- [Request schema](request.schema.json), [response schema](response.schema.json),
  [structural types](types.schema.json).
- [Shared encoding](../../shared/api/values.schema.json),
  [independently consumable conformance cases](conformance.json).
- The [public contract](../../docs/architecture/public-access-and-observation.md) owns
  permission, timing, and failure semantics.

JSON uses exact UTF-8 RFC 8785: reject duplicate keys, noncanonical number/byte encodings,
and extra fields. Schema shape checks do not replace canonical-byte, integer-range,
owner/caller/role, spatial-relation, or authorization checks.
Allowed JSON numbers must also be finite binary64; depth strings respect signed/unsigned
64-bit bounds. `upDepth=-1` is the string `"-1"`; `limit` is a small JSON integer.
Example tokens demonstrate shape, not usable runtime credentials.

## Call sequence

1. `enter` supplies `{boundary:{core,anchor},nextHop}` and actual `intent` to obtain a handle.
2. `query`/`list` returns paths/read receipts. `read` may use that receipt as `basis`
   for same-basis Element/attachment reads.
3. For observation, set `prepareObserve:true` on the initial read, then use
   `begin-observe`/`poll`. Commits before first Begin are already in the preparation queue;
   an ordinary receipt cannot retroactively prepare observation.
4. `create-*`, `update-*`, and `delete-*` use handle-relative Element, final
   parent/local pose/Range, and conditions. Update and place must both be authorized;
   operations share the original owner commit and OperationID.
5. `recover-outcome` supplies original reference, OperationID, and original canonical
   request without expired handle; it recovers only the original receipt.
6. `release-observe` releases observation; `close` releases handle/dependencies, not facts.

Public Revision is an HMAC under the owner's persistent key, not a private sequence or
history-query entry. In the original local profile, relevant mounted closure coverage reports
PARTIAL/gaps; that profile alone does not provide cross-Core public federation.
The separately specified network profile supplies aggregate behavior.

## Configuration and resources

Omitted `Config.PublicAccessPolicy` disables the public entry.
ContextTTL and ObservationTTL require explicit positive durations, without default seconds.
Snapshot retention is independent; observation works with retention disabled.
Other defaults are in public contract §6.5.

Accounting uses 2 KiB per context plus retained-condition bytes and 512 logical bytes per
notification group. Execution additionally reserves `8×request bytes + 1 MiB`
for parse structures and maximum batches. These are conservative admission units, not exact RSS.
P3 independently accounts for snapshot projections.
All admission checks precede allocation; slow-consumer overflow becomes GAP without blocking
ordinary writes.

`go run ./cmd/ucupa` includes actual public calls.
Cross-owner cases are in [acceptance](../../acceptance/public_facet_test.go);
an independent Draft 2020-12 test consumer validates schemas.
