# UCUPA Core

Core is the sole owner of Element spatial truth: stable UUIDs, Kind, local poses, Range,
Anchor structure, Element Revision, and owner-local read cuts. Application properties,
identity establishment, and permission decisions do not belong to Core.

The implementation follows the [local spatial truth contract](../docs/architecture/core-local-spatial-truth.md)
and [local query contract](../docs/architecture/core-local-query.md). G1, explicit Range updates,
and P3 were verified and accepted. Stage 1's foundation includes stable Core identity,
owner-qualified scopes, and commit history; see the [foundation tests](tests/instance/foundation_test.go).
The P2 [mount contract](../docs/architecture/core-mount-contract.md) defines trusted local
two-Core control; P3 adds startup configuration, temporary read contexts, and cross-Core reads.
Those local profiles alone do not establish network, nested, or open federation support.
The [authority manifest](../docs/architecture/README.md) classifies current contracts;
the [development plan](../docs/architecture/development-plan.md) records accepted implementation
and validation scope, including later network and lifecycle increments.

## Code organization

Core and Facet follow the same [engineering rules](../ENGINEERING.md).
The module root is navigation only, without an old Go package, type aliases, or per-method
forwarders. Read the plan for migration and fixed-Store evidence and delivery status;
this README is not a second implementation journal.

| Entry/responsibility | Location |
|---|---|
| Construct, open, close, and obtain local operations | [instance](instance/instance.go) |
| Public inputs, results, opaque values; connected/federation contracts | [contract](contract/spatial.go), [connected](contract/connected/connected.go), [federation](contract/federation/federation.go) |
| `c.Elements()`: creation, reads, writes, projection, public mutations | [element](internal/element/operation.go) |
| `c.Queries()`: local queries, snapshots, pagination, release | [query](internal/query/operation.go) |
| `c.Connected()`: trusted local cross-Core queries | [connected](internal/query/connected/operation.go) |
| `c.Mounts()`: mount control, peer acceptance, contact maintenance, recovery | [mount](internal/mount/operation.go) |
| `c.Facets()`: Core-side Facet contacts and control | [facet](internal/facet/operation.go) |
| `c.Writer()`: single-writer handoff on the original Store | [handoff](internal/handoff/operation.go) |
| `c.Transfers()`: source control/recovery and parent-side carrier control/recovery within supported profiles | [source](internal/transfer/operation.go), [carrier](internal/transfer/carrier.go), [carrier recovery](internal/transfer/carrier_recover.go) |
| `c.Access()`: public request envelope | [access/entry](internal/access/entry/request.go) |
| `c.Federation()`: bounded source reads and disclosure | [federation/source](internal/query/federation/source/operation.go) |
| One fact aggregate, candidate commits, history, publication, uncertain-result recovery | [commit](internal/commit/owner.go) |
| Database byte format, write preparation, resource lifecycle | [store](internal/store/transaction.go) |
| Bounded Element/relation storage-value codecs, without database or authoritative state | [store/record](internal/store/record/element.go) |
| Canonical transfer journal, manifest, and continuity evidence codecs, without commits | [transfer/encoding](internal/transfer/encoding/journal.go) |
| Public context, observation, and retirement accounting, without duplicate facts | [registry](internal/access/registry/registry.go) |
| gRPC configuration, registration, host; actual RPC groups in transport-private packages | [transport/grpc](transport/grpc/server.go), [entry](transport/grpc/internal/entry/service.go), [admission](transport/grpc/internal/admission/service.go) |
| Specified-address outbound mount/federation connections; consumer clients | [mountconn](transport/grpc/mountconn/mount_connections.go), [federationconn](transport/grpc/federationconn/federation_connections.go), [SDK](sdk/go/client.go) |

The instance returns actual operations rather than duplicating their methods. Spatial, mount,
and Facet-contact operations share one commit owner. Snapshots, access resources, admission,
and client connections retain their own lifecycles without becoming second spatial truth.
`c.Access().PublicCall(...)` is the local public envelope; network adapters independently
check identity, caller, contacts, and responses.

Protocol sources and field constraints are in [api](api/README.md); generated representations
are in `bindings/go/protobuf`. The independent host is [cmd/ucupa-core](../cmd/ucupa-core/README.md).
Tests follow the actual owner. [Public composition tests](tests/instance/foundation_test.go)
use real instances/contracts, and fault injection exists only in the relevant owner's test
build. A passing package or empty module root is not full acceptance.

## Local truth model

- A Core may be empty. CreateRoot establishes one local root that cannot be updated, moved,
  or deleted. An empty Core may instead accept an external-parent mount under P2 and then
  must not create a local root.
- A root has no parent or LocalPose and exclusively uses UNBOUNDED_ROOT.
- Every non-root Anchor/Entity atomically stores parent Anchor, LocalPose, and
  BOUNDED_SPHERE Range.
- Anchors form an acyclic tree; an Entity has one direct parent Anchor.
- UUID claims survive deletion permanently and cannot be reused across Kind or lifecycle.
- Callers may explicitly preserve, enlarge, or shrink non-root Anchor/Entity Range.
  The direct parent must exactly contain the final tuple. An Anchor must also cover all
  direct children; violations are atomically rejected.
- No automatic ancestor/child adjustment occurs. Range is conservative coverage, not
  necessarily the minimum enclosing sphere.

Spatial numbers use one binary64/projective-quaternion profile. The Store holds canonical
representations. Containment, SE(3) composition, QueryRegion, and the private intersection
oracle use exact rational computation. Public ProjectToAncestor rounds only at the final Pose
boundary and does not persist WorldPose.

## Owner-local operations

Element writes are create, update, and non-cascading delete. Update/delete use Element Revision
CAS. Every actual Element commit produces a subject Revision and CoreReadCut; identical
updates return UNCHANGED without advancing tokens.

ReadElement returns the current owner-local exact record. ListDirect paginates all direct
child Anchors/Entities in UUID order at one cut; any fact mutation explicitly expires the old
continuation. ProjectToAncestor reads and authorizes the required parent path at one cut.

QueryRegion selects Scope from StartAnchorID and UpDepth: 0 stays at the start, a positive
value ascends at most that many levels, -1 ascends to a proven root, and values below -1 are
invalid. It searches Scope and all descendants for sphere intersections using an independent
Basis, without a downward depth limit. Selecting a common ancestor covers sibling branches;
Basis does not narrow scope automatically. Results include actual Scope and ascent depth,
UUID, Kind, Revision, and Scope-relative path. Each page performs a full scan and paginates
in UUID order. Numerical work-limit gaps are Partial, not absence. The limit bounds returned
items, not scan cost.

New, CreateDurable, and OpenDurable explicitly receive Config. Omitted SnapshotRetention
disables old-state retention; a positive duration enables it, without a default.
Queries and pagination still work when disabled but require a fresh query after cut changes.
When enabled, subsequent pages retain the same old view and ordinary writes still commit
durably without delay. For complete records, CurrentVerified uses same-cut ReadElement;
Retained uses ReadElementSnapshot. Exact-read permission is checked independently.
Snapshots have count, logical-byte, and concurrent-borrow limits. Expiry, release, or close
ends reuse; no silent switch to current state occurs.

Every mutation has a 1..128-byte OperationID. Retries with the same actor, operation kind,
subject, applicable canonical input, and expected Revision replay the original result/cut.
Different bindings under that ID return `ErrOperationConflict`. Success, UNCHANGED,
`ErrNotFound`, `ErrConflict`, and `ErrRevisionConflict` are retained.

Core constructs exact `CoreElementScope(CoreRef, ElementUUID)` from its stable identity and
submits exact all-of requirements to Authorizer in the contract's disclosure order:

- Create root, Anchor, or Entity.
- Exact read, ListDirect, and QueryRegion on the exact Scope.
- Update subject and place in the final parent.
- Delete Anchor or Entity.

Existing Element writes and exact read/ListDirect each use one all-of decision.
`ProjectToAncestor` first authorizes the request-known subject/ancestor, then the full path
resolved at the same provisional cut, and finally rechecks the cut.
QueryRegion authorizes the exact Scope before and after evaluation, then rechecks the cut.
Query permission does not grant exact-read permission.

Authorization precedes disclosure, OperationID replay, and mutation decisions. External
Authorizer calls never occur under the Core lock or Store transaction. Instance assembly binds
the authorizer to this owner's current persistent Realm contact. Disconnection, wrong Realm,
or unverifiable evidence is unavailable, not denial or absence. Core exports no actor-free
existence probe. Composition uses explicit service actors for ReadFacetProposal/ReadFacetAttachment,
projecting actual Core cut, existence, and attachment evidence without spatial records.

## Local two-Core mount control

The original P2 profile supports one mount: A owns an empty ordinary Anchor M; empty B accepts
contents below it. M remains A's. B retains external-parent context and accepted Range without
copying M, claiming its UUID, or inventing a root. B exactly checks direct-content containment.

Control operations are `ApplyMount` for establishment/change/revocation,
`ReadMount` and `ReadMountOperation` for state/original outcomes,
`RecoverMounts` for unfinished obligations, and `UpdateMountNextHop` for this direction's
address. Recovery advances one explicit round; there is no background loop or timeout unlock.
Trusted assembly uses `ConfigureMountTransport` and restricted `MountPeer`; these local
ports are not substitutes for network authentication.

A Range change first closes B's new-write gate, then A decides, and B reopens only after
confirming that decision. Disconnection retains pending state. Old exact receipts remain
replayable; new requests rejected by the gate do not claim OperationID.
Ordinary M Pose/Parent changes leave B's local facts unchanged; Range changes require control.
Revocation distinguishes A closing entry from B stopping writes. Detached retains B's content;
it is neither deletion nor completed migration.

Both sides use established callers and owner-qualified scopes. New commits recheck original
caller permission; exact existing settlement obligations are not lost after revocation.
Address plus stable Ref identify the target; only the relation's specified NextHop is used.
Address repair may occur during control waits without changing Element, MountKey, or Admission.
Element, mount intent, and address operations share this Core's OperationID namespace and
CoreReadCut. Control-only commits do not increment Element Revision.

Owner-local reads do not silently become composed reads. ListDirect encountering mounted
contents, QueryRegion covering remote contents, or a local query bounded by B's external
parent returns `ErrMountContextRequired`, not empty or complete. P3 explicitly requests
connected space through separate path operations.

Evidence: [two-Core control](../acceptance/mount_control_test.go),
[concurrency/recovery](../acceptance/mount_recovery_test.go),
[commit faults](internal/commit/mount_store_atomicity_test.go), and
[history corruption](internal/commit/mount_store_corruption_test.go).

## Reads and queries along mount paths

P3 ResolvePath, ListDirectPath, and QueryRegionPath start from an explicit entry Anchor and
follow actual parent-child relations. Mounts add no structural depth: M's record comes from A,
its contents from B. Connections use the relation's NextHop and verify stable target identity,
without a bare-Ref directory. ConfigureConnectedTransport/ConnectedPeer are trusted local
assembly, not network authentication.

The first request fixes ConnectedCut scope, sources, and read mode. Later pages and same-cut
path reads cannot expand scope or silently add sources. Retained requires all necessary sources
to enable retention; mixed configurations use CurrentVerified throughout. Every disclosure
independently checks each owner's permissions, current mount validity, and contact relation.
A data snapshot is not an authorization snapshot; Query permission does not grant full reads.

Necessary-source failure explicitly fails the request. Unverifiable optional coverage is
Partial/Gap, not an empty set. Expired snapshot/original cuts require a new query. Loss of
optional coverage ends continuation; recovered content cannot patch an old result.
ReleaseConnectedCut releases the context; unreleased retained state expires safely.
This P3 profile is one mount between two trusted local Cores, not real multi-host or arbitrary
nested federation; later network profiles have separate contracts and evidence.

## Single-writer handoff on the original Store

P7.1 implements the same-domain profile in [P7 §6](../docs/architecture/owner-transition-contract.md#6-same-owner-writer-handoff).
Delivery/acceptance and full validation are recorded in the plan.
`c.Writer().InspectWriter/HandoffWriter` returns actual control results; SDK `Writer()`
uses the same gRPC service. Control requires enabled PublicAccessPolicy and a trusted
authorization view. Independently check `ucupa.core.writer.inspect` or
`ucupa.core.writer.handoff` on `AuthorityScope(c.AuthorityRef())`; Element, closure, and
read-delegation permission do not substitute. Fencing does not disable inspection/exact retry.

`Config.Writer` contains only the handoff switch and optional installation qualification.
The initial durable transaction records the switch; ordinary reopen must match.
Handoff closes the current epoch and fixes the final cut in the original transaction.
Subsequent writes and old reads/snapshots/observations stop disclosing facts.
The designated instance exclusively opens the same original Store and verifies private
installation qualification before advancing epoch once. Identity, UUIDs, facts, signing
material, and original business outcomes remain; ordinary restart does not advance epoch.
In-memory creation rejects Writer configuration. A handoff-disabled Store cannot be enabled
later or patched at startup. This is not copied storage, rollback, or outage takeover.
See the [Core host](../cmd/ucupa-core/README.md#single-writer-handoff) for fields, secret handling,
and network use.

## Durable Store

New creates an in-memory Core; CreateDurable/OpenDurable/Close manage the equivalent bbolt
profile. Trusted creation supplies RealmID and local address, verifies the connection, then
binds identity. Reopen restores and checks the contact from the original Store; configuration
cannot override or invent it. The sole exact marker is:

    ucupa.core.local@9c4f2a6e8b1d7350f6a3c9e2b7d4f1a8

New/CreateDurable generate an independent `FactAuthorityRef` once; OpenDurable only restores it.
`AuthorityRef()` supports trusted local identity assembly, not availability proof or remote
authentication.

The Store atomically persists self Ref, private scope, Realm contact, current cut, records,
tombstones, Revisions, OperationID bindings, retained outcomes, and cut-ordered commit history.
Element and control commits share that sequence. Control events retain original input and
external results; the same transition rules reconstruct control state without a writable mirror.
Rejected, unchanged, and replayed operations add no fact commit, but retained outcomes persist.
Reopen validates successful and non-mutating receipts. Persist/reload recheck self Ref, scope,
and Realm contact; missing contact rejects open. There is no old-marker reader, migration,
compatibility branch, or predecessor-profile error.

The private linear scanIntersecting is the QueryRegion/future-index conformance oracle,
not public API. P3 snapshots/connected contexts are run-local, with no Store fields or durable
cursor. Close ends both in-memory and durable runs; reopen revives no old query handles.
P6 public federation supports nested discovery, reads, listing, and finite observation through
specified mount contacts. Transport/SDK and independent hosts adapt networking outside the kernel.
No independent route/referral, Facet-successor lookup, permanent event stream, spatial index,
displayName, text path, or production operating surface is implemented. See the plan for
current network delivery, selected two-host evidence, and remaining validation limits.
