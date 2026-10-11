# UCUPA Core local spatial-truth implementation contract

**Status: G1 and the explicit Range-update amendment are accepted; authority classification follows the [architecture authority manifest](README.md)**

This contract reduces D1–D4 of the accepted mainline to an implementable, falsifiable Core
slice. It defines one owner-local, rooted form without federation, not versions, compatible
inputs, or historical Store readers. If it conflicts with the axioms or mainline, revise this
contract rather than reinterpret governing semantics through implementation.

Original G1 passed all black-box gates and independent review and received user acceptance.
The [authority manifest](README.md) lists this as a current Core implementation contract.
The explicit Range-update amendment was separately verified and accepted, without a parallel
contract. The [development plan](development-plan.md) alone maintains current gates and next work.
This document continues to own basic spatial values, writes, and ordinary local reads. The
accepted [local query contract](core-local-query.md) supersedes `Config` startup read settings,
`Close` read-resource lifecycle, and added query behavior. The
[mount contract](core-mount-contract.md) supersedes its declared external-parent, mount-control,
and combined-read scope. Original G1 limits are not the limits of the entire current Core;
the responsible owner contract defines each clause.

The accepted [P4 local Core/Facet contract](facet-local-truth-and-core-lifecycle.md) supersedes
or extends only Core self-authority profile, exact-read envelope, atomic attachment effects
of create/delete, attachment-only commits, owner-qualified scopes, attachment/contact control,
typed evidence ports, and corresponding Store/commit clause families. This document continues
to own D1–D4 `ElementRecord`, spatial create/update/reanchor/reparent/delete, ElementRevision,
ListDirect, spatial CAS/errors/convergence, and geometry oracle. It does not wholly leave
current authority or retain old write entries lacking attachment effects.

## 1. What this contract freezes

Only this slice may be claimed:

```text
D1  right-handed Cartesian SE(3) coordinate algebra
D2  Element Range and root semantics
D3  one canonical numeric representation and safe geometry decisions
D4  owner-local Core records, reads/writes, convergence, and persistence
+ owner-local ListDirect
+ non-public linear geometry conformance oracle
```

Within this scope, Core alone owns:

- Core existence of ElementUUID, closed kind, current parent, LocalPose, Range, Element Revision.
- Root, direct containment, acyclicity, non-cascading deletion, and non-reusable UUID invariants.
- Owner-local Core read cuts, OperationID bindings, and retained mutation outcomes.
- In-memory and durable Store representations of those facts.

Realm still decides caller identity and permissions; Facet independently owns non-spatial facts.
Core may submit exact requirements to Authorizer, not persist Principals, Grants, or policies.
Names, displayName, text paths, business objects, Facet payload, Route, Cluster, deployment,
and indexes are not Core truth.

## 2. Sole record form

### 2.1 Identifiers and kinds

- `ElementID` remains canonical lowercase RFC 9562 UUIDv4.
- `ANCHOR` and `ENTITY` are closed kinds; no custom kinds or unknown-kind fallback.
- A UUID declares its kind only once; deletion leaves a tombstone permanently claiming it.
- Rename, Pose/Range updates, reparent, process changes, and future proven authority transition
  do not change UUID.

### 2.2 Spatial values

```text
Vec3     { X Scalar, Y Scalar, Z Scalar }
Rotation { Pivot W|X|Y|Z, Ratios [3]Scalar }
Pose     { Translation Vec3, Rotation Rotation }

Range = BOUNDED_SPHERE { Radius Scalar }
      | UNBOUNDED_ROOT

SpatialTuple = ParentAnchorID + LocalPose + BOUNDED_SPHERE
```

`Rotation` is a canonical projective quaternion, not an “approximately unit quaternion.”
`Pivot` identifies the implicit `+1` component; the other three follow W, X, Y, Z order
with Pivot omitted. Nonzero scalar multiples represent the same rotation, eliminating
`q`/`-q` as competing truth. §4 freezes the sole canonicalization.

### 2.3 Element record

```text
AnchorRecord {
  ID
  ParentAnchorID?   # absent only for root
  LocalPose?        # present if and only if a parent exists
  Range             # root uses UNBOUNDED_ROOT; otherwise BOUNDED_SPHERE
  Revision
}

EntityRecord {
  ID
  ContainingAnchorID
  LocalPose
  Range             # BOUNDED_SPHERE only
  Revision
}

ElementRecord = AnchorRecord | EntityRecord
```

Absence means an absent field, not zero UUID, identity Pose, negative number, NaN, or Infinity
sentinels. Root has no parent/LocalPose; identity in its own basis is derived only.

A new Core may be empty. First successful `CreateRoot` establishes its sole root; afterward
exactly one remains, and ordinary operations cannot update, reparent, or delete it. This
contract permits neither independent multiple roots nor a rootless carrier in one Core.
Federation successors must prove those forms rather than reserve them in local records.

### 2.4 Owner binding, Revision, and read cut

```text
Revision      # scoped to one Core owner and Element; opaque equality token
CoreReadCut   # scoped to one Core owner's complete local fact cut; opaque equality token
```

G1 owner is the concrete Core instance/durable Store bound to the call, without a new
transferable public owner ID. Revision/CoreReadCut internally carry unforgeable private scope
created with the instance and preserved on reopening the same Store. Reject tokens from other
instances/Stores. Neither is endpoint, Cluster, root UUID, timestamp, or `FactAuthorityRef`.
The call boundary satisfies D4's local attribution. D6 `FactAuthorityRef` alone defines final
cross-process disclosable/routable identity; G1 creates no transitional identity.

Even an empty Core receives a nonzero valid returnable initial cut; Go zero is always invalid.
Every commit really changing Core facts atomically creates a new CoreReadCut and subject
Revision, leaving other Revisions unchanged. These distinct tokens support equality only,
not exposed counters, ordering, or inferred relationships. A canonical final tuple identical
to current record returns retained `UNCHANGED` without advancing Revision/cut. Retained
negative outcomes, replay, and reads do not advance fact cuts. Cut is not Realm-global revision
or cross-owner total order and requires no subspace request to a “total Realm.”

## 3. Spatial and structural invariants

### 3.1 Coordinate algebra

- `LocalPose` is a right-handed Cartesian rigid transform from Element local frame to direct
  parent Anchor frame; linear units are metres.
- Pose has no scale, shear, projection parameters, or coordinate-system labels.
- Polar, spherical, geodetic, astronomical partitioned, and rendering coordinates project
  outside the Core boundary.
- `WorldPose` is no separate API or persistent field; ancestor-frame Pose derives from the
  current parent chain.
- Independent root bases have no implicit transform. This contract has one rooted space and
  claims no cross-root projection.

### 3.2 Range semantics

- Entity Range is domain-neutral occurrence coverage, not Collider, LOD, visibility distance,
  permission region, or business influence.
- Non-root Anchor Range conservatively covers its full structural closure; extra matches are
  allowed, missed coverage is not.
- Root alone uses `UNBOUNDED_ROOT`; every parented Element uses finite, nonnegative
  `BOUNDED_SPHERE`.
- Sibling Anchors may overlap; spatial organization is not mandatory non-overlapping partitioning.

A bounded direct parent requires exact final-tuple containment:

```text
distance(parent.origin, child.origin) + child.radius <= parent.radius
```

Use §4.4's exact predicate without square roots. Tangency is valid; rotation does not alter
radius. A root parent uses type containment without a nonexistent root radius.

Non-root Anchor/Entity updates may preserve, enlarge, or shrink radius if final parent
containment holds. An Anchor's final Range must also contain all direct child Anchors/Entities
in the same committed state. Children keep their local tuples; do not automatically move/shrink
them to make requests valid. Any failed containment rejects atomically.

Range is explicitly declared conservative coverage, not necessarily minimized. Child creation,
change, or deletion does not adjust ancestor Range/records/Revisions. Updating an Anchor does
not rewrite descendants. Root/mount retirement is not an ordinary operation.

### 3.3 Final-tuple atomicity

Every create/update validates final `parent + LocalPose + Range` together and commits once.
This is an invariant set, not observable error precedence; §7 fixes decision order:

1. Parent exists and is Anchor.
2. UUID, numeric representation, and Range kind are valid.
3. Anchor update creates no cycle.
4. Subject fits its direct parent; an Anchor's final Range also contains every direct child.
5. Expected Revision matches current subject record.

Failure leaves no intermediate parent, Pose, Range, Revision, cut, or operation success.
Anchor reparent rewrites only its own final tuple, not descendant records/Revisions;
projections derive again.

Entity deletion is direct and non-cascading. Anchor deletion requires non-root and a proved
empty direct-child set in the same commit domain, without cascade. G1 has no delegation
state. A successor allowing deletion of formerly remote-content-bearing mount Anchors must
first add independent retirement proof, not reuse local empty-delete rules.

## 4. Sole numeric profile

### 4.1 Scalar

Public authoritative `Scalar` is a 64-bit IEEE 754 binary64 value:

- Accept all finite values, including subnormal and maximum finite; reject NaN and either Infinity.
- Store/return every zero as `+0`; `-0` is not competing truth.
- Absolute position/radius limit is maximum finite binary64 `0x1.fffffffffffffp+1023`.
- Radius is additionally nonnegative; `+0` is a valid point sphere.
- Store/tests compare exact canonical bit patterns, not decimal text or epsilon.

Core sets no empirical metre ceiling unrelated to UCUPA's scale goals. A derived value not
correctly roundable to finite binary64 makes public projection `ErrUnrepresentable`, never
saturated, truncated, or silently reduced in precision.

### 4.2 Rotation canonicalization

Records accept only canonical `Rotation`. Boundary constructors may accept four finite,
not-all-zero binary64 quaternion components and produce it by this sole algorithm:

1. Interpret bit patterns as exact dyadic rationals.
2. Choose maximum absolute component as Pivot; ties prefer W, X, Y, Z in that order.
3. Change the overall sign so Pivot is positive.
4. Do not store Pivot; define it as exact `+1`. Divide remaining components by Pivot and
   correctly round to binary64, round-to-nearest, ties-to-even.
5. If an exact pre-Pivot ratio has `abs(r) < 1` but rounding gives `abs(round(r)) == 1`,
   retain sign and choose the adjacent binary64 toward zero to preserve “first maximum.”
   Canonicalize all signed-zero bits to `+0` again.

Pivot allows W/X/Y/Z only. Ratios must be finite with magnitude at most `1`; those before
Pivot have magnitude strictly below `1`; every zero is `+0`. This is mechanically verifiable
and idempotent. Exactly proportional inputs, `q`, and `-q` produce identical bits.
Core stores no unit quaternion, Euler angle, or matrix as parallel rotation truth.

Internal Hamilton products with exact rational components use the same Pivot/sign/rounding
rules at public Rotation output. SDK unit-quaternion/Euler/matrix projections are not records.

### 4.3 Exact SE(3) conventions

Quaternion order is `q=(qw,qx,qy,qz)`; Hamilton product `a ⊗ b` is:

```text
w = aw*bw - ax*bx - ay*by - az*bz
x = aw*bx + ax*bw + ay*bz - az*by
y = aw*by - ax*bz + ay*bw + az*bx
z = aw*bz + ax*by - ay*bx + az*bw
```

`conj(q)=(qw,-qx,-qy,-qz)`, `norm2(q)=qw²+qx²+qy²+qz²`. Core uses active right-handed
rotation; local vector to parent frame means exactly:

```text
R_q(v) = vector_part(q ⊗ (0,v) ⊗ conj(q)) / norm2(q)
Pose(t,q)(v) = R_q(v) + t
```

Thus q need not be unit length, while rotation remains rigid. For inner local→parent
`I=(ti,qi)` and outer parent→ancestor `O=(to,qo)`:

```text
O compose I = (R_qo(ti) + to, qo ⊗ qi)
inverse(t,q) = (-R_conj(q)(t), conj(q))
```

Interpret canonical binary64 bits as exact rationals first, evaluating by parentheses and
W/X/Y/Z order. FMA, compiler reordering, and intermediate binary64 rounding must not decide
authoritative results. Round Hamilton output only at §4.2's Rotation boundary, and translation
only at public Pose output, round-to-nearest, ties-to-even.

Minimum golden corpus:

```text
Rotation(Pivot=W, Ratios[X,Y,Z]=[0,0,1]) maps +X to +Y
that rotation compose itself maps +X to -X
outer=(translation [0,2,0], +90deg Z), inner=(translation [1,0,0], identity)
  => composed translation [0,3,0] and rotation +90deg Z
inverse(+90deg Z) maps +X to -Y
```

Reverse rotation, `qi ⊗ qo`, or `[1,2,0]` violates this contract.

### 4.4 Authoritative containment predicate

Interpret finite binary64 as exact dyadic rationals. For bounded parent/child, translation
`(x,y,z)`, child radius `rc`, parent radius `rp`, accept only if both hold exactly:

```text
rc <= rp
x*x + y*y + z*z <= (rp - rc) * (rp - rc)
```

Semantics are arbitrary-precision integer/rational comparison, not a particular library,
binary64 squaring, square root, or empirical epsilon. Exact tangency accepts, the adjacent
outside tuple rejects, and extreme calculations do not overflow. Incomplete proof rejects
mutation, not guesses inside.

### 4.5 Derived Pose, intersection, and exact-work bounds

Parent-chain composition applies §4.3 `outer compose inner` along current parents from subject.
To project a sphere center into query basis, compute `T(boundary<-element)` and
`T(boundary<-basis)`, then
`inverse(T(boundary<-basis)) compose T(boundary<-element)`. Use exact rationals throughout,
rounding only at public Pose output to canonical binary64/Rotation. Broken chains, cycles,
and noncanonical records are `ErrStore`; finite truth not representable as output Scalar is
`ErrUnrepresentable`.

Sphere intersection compares exact squared distance with
`(query.radius + element.radius)^2`. Root unbounded Range intersects by type. Center-only
tests cannot replace sphere intersection; uncertainty cannot cause complete misses.

For physically executable exact semantics, the contract also fixes work bounds:

```text
MaxExactProjectionEdges = 1024
MaxExactRationalBits     = 65536
```

These are contract work-bound names, not §5 public Go APIs.

Edge count is the maximum parent edges from any subject to the required common boundary in
one Projection/oracle call. After reducing each primitive rational operation, numerator and
denominator magnitudes' bit lengths must not exceed `MaxExactRationalBits`. Direct containment
for every finite binary64 input is naturally below this bound, so writes cannot evade decisions
through work limits.

Exceeding either bound makes the entire Projection/oracle `ErrUnrepresentable`, without
truncated Pose, partial matches, or complete flag. Deeper valid Structure still permits exact
reads/ListDirect. Bounds limit the derived operation, not query universe or persisted query
depth. Future implementations may increase resource limits, but not change in-bound bit-exact
results or disguise out-of-bound cases as success.

## 5. Owner-local API

This contract freezes the following public Go surface for its slice. Numeric/token fields
stay private; DTO fields export as specified, but Core validates every input and returns only
discriminant-consistent values. Operations, parameter order, result discriminants, and error
mapping must not gain parallel entries:

```go
type ElementKind uint8       // closed: Anchor, Entity
type RotationPivot uint8     // closed: W, X, Y, Z
type RangeKind uint8         // closed: BoundedSphere, UnboundedRoot
type MutationStatus uint8    // closed: Committed, Unchanged, Rejected
type ExistenceState uint8    // closed: Present, NotFound
type OperationID string

const (
    ElementAnchor ElementKind = iota + 1
    ElementEntity
)
const (
    PivotW RotationPivot = iota + 1
    PivotX
    PivotY
    PivotZ
)
const (
    RangeBoundedSphere RangeKind = iota + 1
    RangeUnboundedRoot
)
const (
    MutationCommitted MutationStatus = iota + 1
    MutationUnchanged
    MutationRejected
)
const (
    ExistencePresent ExistenceState = iota + 1
    ExistenceNotFound
)

type Rotation struct { /* private canonical Pivot + ratio bits */ }
type BoundedSphere struct { /* private canonical radius bits */ }
type Range struct { /* private closed variant */ }
type Revision struct { /* private owner/Element-scoped token */ }
type CoreReadCut struct { /* private owner-scoped token */ }
type ListContinuation struct { /* private Core/parent/cut/position binding */ }

type Authorizer interface {
    Authorize(values.PrincipalRef, ...values.Requirement) bool
}

type Vec3 struct { X, Y, Z float64 }
type Pose struct { Translation Vec3; Rotation Rotation }
type SpatialTuple struct {
    Parent values.ElementID
    LocalPose Pose
    Range BoundedSphere
}

type AnchorRecord struct {
    ID values.ElementID
    ParentAnchorID *values.ElementID
    LocalPose *Pose
    Range Range
    Revision Revision
}
type EntityRecord struct {
    ID values.ElementID
    ContainingAnchorID values.ElementID
    LocalPose Pose
    Range BoundedSphere
    Revision Revision
}
type ElementRecord struct {
    Kind ElementKind
    Anchor *AnchorRecord
    Entity *EntityRecord
}

type MutationResult struct {
    Status MutationStatus
    Record *ElementRecord
    TombstoneRevision Revision
    OutcomeCut CoreReadCut
}
type ElementRead struct {
    State ExistenceState
    Record *ElementRecord
    Cut CoreReadCut
}
type DirectPage struct {
    Records []ElementRecord
    Cut CoreReadCut
    Continuation *ListContinuation
}
type Projection struct {
    Pose Pose
    SubjectRevision Revision
    Cut CoreReadCut
}
func New(authorizer Authorizer, config Config) (*Core, error)
func CreateDurable(authorizer Authorizer, path string, config Config) (*Core, error)
func OpenDurable(authorizer Authorizer, path string, config Config) (*Core, error)
func (core *Core) Close() error

func NewRotation(w, x, y, z float64) (Rotation, error)
func NewPose(translation Vec3, rotation Rotation) (Pose, error)
func NewBoundedSphere(radius float64) (BoundedSphere, error)
func (rotation Rotation) Pivot() RotationPivot
func (rotation Rotation) Ratios() [3]float64
func (sphere BoundedSphere) Radius() float64
func (value Range) Kind() RangeKind
func (value Range) BoundedSphere() (BoundedSphere, bool)
func (left Revision) Equal(right Revision) bool
func (left CoreReadCut) Equal(right CoreReadCut) bool

func (core *Core) CreateRoot(
    actor values.PrincipalRef,
    id values.ElementID,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) CreateAnchor(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) CreateEntity(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) UpdateAnchor(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) UpdateEntity(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) DeleteAnchor(
    actor values.PrincipalRef,
    id values.ElementID,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) DeleteEntity(
    actor values.PrincipalRef,
    id values.ElementID,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) ReadElement(
    actor values.PrincipalRef,
    id values.ElementID,
    at *CoreReadCut,
) (ElementRead, error)

func (core *Core) ListDirect(
    actor values.PrincipalRef,
    parent values.ElementID,
    limit uint16,
    continuation *ListContinuation,
) (DirectPage, error)

func (core *Core) ProjectToAncestor(
    actor values.PrincipalRef,
    subject values.ElementID,
    ancestor values.ElementID,
    at *CoreReadCut,
) (Projection, error)
```

For methods taking `at`, `at == nil` selects current cut; a non-nil Go zero token is
`ErrInvalid`. Internal fields of `Revision`, `CoreReadCut`, `Rotation`, `BoundedSphere`,
`Range`, and `ListContinuation` cannot be constructed/parsed. Their Go zero values are
invalid; constructors/Core create them and accessors/`Equal` consume them, without ordering,
time, or wire-serialization promises. `ElementKind`, `RotationPivot`, `RangeKind`,
`MutationStatus`, and `ExistenceState` export only their closed constants. Unknown numeric
values yield `ErrInvalid` or `ErrStore`, not fallback.

Returned records/slices are value snapshots, never aliases to mutable Core state.
`ElementRecord` has exactly one non-nil variant matching Kind; root Anchor optional pointers
are both nil, non-root both non-nil. `MutationResult` and `ElementRead` optional/zero fields
follow discriminants below; callers do not infer state from whichever field is nonzero.

Public DTOs are closed discriminated values:

```text
ElementRecord = ANCHOR(AnchorRecord) | ENTITY(EntityRecord)

MutationResult {
  Status = COMMITTED | UNCHANGED | REJECTED
  Record?               # COMMITTED create/update, or UNCHANGED update
  TombstoneRevision?    # COMMITTED delete
  OutcomeCut             # retained outcome's CommitCut or DecisionCut
}

ElementRead = PRESENT { ElementRecord, CoreReadCut }
            | NOT_FOUND { CoreReadCut }

DirectPage = { ElementRecord[], CoreReadCut, continuation? }

Projection = { Pose, SubjectRevision, CoreReadCut }
```

Read the closed discriminant first, not nil Record, zero Revision, or zero cut.
`MutationResult` with COMMITTED returns the post-commit cut. Only Update can be UNCHANGED, returning unchanged record/
Revision and decision-time cut. Retained failures return `REJECTED`, original DecisionCut,
and corresponding sentinel error. Non-retained errors return zero `MutationResult`.
Exact replay always returns the original result/error, not a new cut presented as current truth.

COMMITTED create/update has only Record; COMMITTED delete only TombstoneRevision; UNCHANGED
only Update Record; REJECTED neither, but valid OutcomeCut. PRESENT ElementRead has exactly
one Record; NOT_FOUND none. Occupied-UUID Create returns retained `ErrConflict`, not UNCHANGED.
Every other combination is a Core bug or `ErrStore`, never a caller response.

`ReadElement` is the sole ordinary exact-record read; retain no typed `Anchor`/`Entity`
parallel readers. Authorized absence is `NOT_FOUND` with current cut, making absence attributable.
An unestablishable consistent Store read returns zero `ElementRead`/`ErrStore`, publicly
`UNAVAILABLE`, not a fabricated cut. `ErrDenied`/`ErrReadCutExpired` also return no existence
outcome.

Core no longer exports actor-free `ConfirmElement` or an authorization-free existence oracle.
In original G1, Composition maintained the existing Facet owner boundary by explicitly binding
an established service `PrincipalRef`; a non-authoritative adapter called sole
`ReadElement(actor, id, nil)` and projected to Facet's then-consumed `(exists, available)`.
The actor required exact `ucupa.core.element.read`, never inferred/default-granted by
Core/Composition. DENIED/ErrStore mapped to `available=false`, not invented absence.
That bool belonged only to the pre-typed-lifecycle Facet-owned consumer port, not Core public API,
a compatibility facade, or a new evidence contract. The independent Facet local contract defines
typed existence/cut lifecycle.

### 5.1 ListDirect

`ListDirect` is necessary complete Structure access, not a caller shadow directory:

- Parent is current Anchor; pages contain complete records of both direct child kinds.
- Order by ElementUUID's 16 canonical bytes ascending; `1 <= limit <= 256`.
- Without continuation, select current cut at one consistent transaction/lock boundary and
  return first page.
- Non-nil continuation alone supplies later cut/position; no second `at` argument.
- G1 serves only current snapshots. Non-current token cut yields `ErrReadCutExpired` and no facts.
- Opaque continuation binds at least private Core scope, parent, first cut, last UUID.
- Never silently switch later pages to current. Unservable original cut returns
  `ErrReadCutExpired` without items; discard all prior pages and restart from page one.
- Malformed token is `ErrInvalid` before authorization. Well-formed Core-scope/parent binding
  is checked only after exact-parent authorization; mismatch then yields `ErrInvalid`.
  No continuation on final page proves complete direct adjacency at that cut.
- No silent per-child permission filtering. Require the entire parent's `ListDirect` permission or return `ErrDenied`.

This owner-local all-fact cut is the minimum correct baseline, not centralized Realm revision.
Future snapshots, MVCC, or finer cuts may improve concurrency/serve old cuts without changing
API/result semantics. G1 does not guarantee multipage completion under continuous writes:
either one cut's complete set or explicit failure/full restart.

### 5.2 ProjectToAncestor

Read subject, intermediate Anchors, and target ancestor at one cut. Return deterministically
rounded subject-to-ancestor Pose, subject Revision, and read evidence. Same subject/ancestor
Anchor gives identity; an Entity as its own ancestor is still `ErrConflict` for invalid kind.

Do not persist results or disclose intermediate UUIDs. Caller requires exact read permission
for every internal path Element; any missing authorization is `ErrDenied`. Missing ancestor
is `ErrNotFound`; non-Anchor or non-ancestor is `ErrConflict`. This API finds no other root,
crosses no authority, and interprets no text path as address.

Public Projection's binary64 translation is presentation, not a safe query-pruning envelope.
Combining rounded center with original radius may lose conservative coverage; therefore this
DTO returns no projected Range. `ReadElement(at=cut)` provides same-cut original Range, but
spatial negatives still need exact oracle or future D10 error-envelope evidence. Rounded Pose
must not manufacture complete misses.

Caller-facing `ReadElement`/`ProjectToAncestor` share `at` rules: nil=current; Go zero is
pre-authorization `ErrInvalid`; well-formed foreign-Core token is `ErrInvalid` after current
exact-scope authorization; own unservable cut is `ErrReadCutExpired`. `ListDirect` creates cut
on first page; later continuation alone supplies that same cut. Exact read/projection may bind
page cut without fabricating a mixed snapshot.

## 6. Permissions and disclosure order

Fixed PermissionIDs:

| Operation | PermissionID | Exact Element scope |
|---|---|---|
| Create root | `ucupa.core.anchor.create-root` | Candidate root |
| Create Anchor | `ucupa.core.anchor.create` | Final parent |
| Create Entity | `ucupa.core.entity.create` | Final parent |
| Exact read | `ucupa.core.element.read` | Subject/every projection path item |
| `ListDirect` | `ucupa.core.anchor.list-direct` | Parent |
| Update tuple | `ucupa.core.element.update` | Subject |
| Place in zone | `ucupa.core.element.place` | Final parent |
| Delete Anchor | `ucupa.core.anchor.delete` | Subject |
| Delete Entity | `ucupa.core.entity.delete` | Subject |

Even unchanged-parent updates require subject update and final-parent place as one all-of
decision. No independent old-parent permission is required. Kind-specific parent create
permission includes first placement.

All calls follow: state-independent syntax/canonical validation → input-known exact-scope
authorization → OperationID replay lookup or state read → needed path authorization →
CAS/structure/geometry checks → atomic commit. Existence, kind, parent, stale continuation,
or conflict details cannot disclose state before authorization. Even historical successful
OperationID replay needs current authorization after revocation; `DENIED` never binds an
OperationID.

External Authorizer never runs within Core state lock/Store transaction. After initial
input-known authorization, `ProjectToAncestor` privately resolves the path at a provisional
cut, unlocks for all-of complete-path requirements, then evaluates at exactly that cut.
Any intervening fact mutation yields whole-call `ErrReadCutExpired`, not mixed paths.
G1 proves required current authorization precedes disclosure/final evaluation/commit in this
order, not atomic Realm/Core cuts across owners.

G1 submits exact requirements only. Descendant closure scopes, views, hidden branches, and
remote trust belong to public-discovery/open-federation successors, not records or PermissionIDs.

G1 `Authorizer` has one bool decision. `false` always means fail-closed `ErrDenied` to Core,
whether explicit Realm refusal or a backend unable to permit. It does not infer unavailable
causes from bool or turn Realm failure into Core existence evidence.

## 7. Errors, CAS, and OperationID convergence

Sole error set:

```text
ErrInvalid
ErrDenied
ErrNotFound
ErrConflict
ErrRevisionConflict
ErrOperationConflict
ErrReadCutExpired
ErrUnrepresentable
ErrStore
```

- `ErrInvalid`: ID, OperationID, limit, continuation, or numeric representation is invalid
  before state access.
- `ErrDenied`: not all current exact requirements approved; no fact-state disclosure.
- `ErrNotFound`: authorized mutation/list/projection target absent. `ReadElement` uses
  `NOT_FOUND` outcome.
- `ErrConflict`: wrong kind, occupied UUID, second root, root mutation, cycle, nonempty deletion,
  final-parent containment or Anchor child coverage failure, or invalid ancestry. Shrinking
  Range itself is not a conflict.
- `ErrRevisionConflict`: expected Revision is foreign to owner/Element or not equal to current record.
- `ErrOperationConflict`: OperationID already bound to a different actor, operation kind, ID,
  canonical final tuple, or CAS.
- `ErrReadCutExpired`: valid owner cut no longer servable; no partial facts at that cut.
- `ErrUnrepresentable`: finite valid truth cannot form this profile's public derived output.
- `ErrStore`: Store, transaction, lock, codec, random owner establishment, or durable-history
  validation failed; outcome may be unconfirmed.

Mutation OperationID is a nonempty opaque `1..128`-byte value. Binding covers actor, operation
kind, subject, all canonical input, expected Revision. Exact replay returns original receipt
or retained negative outcome; different binding is `ErrOperationConflict`.

Atomically retain `COMMITTED`, `UNCHANGED`, `ErrNotFound`, `ErrConflict`, and
`ErrRevisionConflict`, each with original `CommitCut`/`DecisionCut`. `ErrInvalid`,
`ErrDenied`, `ErrOperationConflict`, `ErrReadCutExpired`, `ErrUnrepresentable`, and `ErrStore`
are not retained outcomes. Clearly precommit `ErrStore` creates no binding, but response loss/
commit-boundary failure may return `ErrStore` after the transaction created binding/outcome.
When indistinguishable, retry identical OperationID/binding: execute if unbound, replay if bound,
never commit twice.

After authorization and retained lookup, mutation precedence is:

```text
update: subject existence/kind -> expected Revision -> root immutability
        -> unchanged tuple -> final parent existence/kind -> cycle
        -> exact parent containment -> Anchor exact direct-child coverage -> commit

delete: subject existence/kind -> expected Revision -> root/empty rule -> commit

child create: final parent existence/kind -> candidate UUID unclaimed
              -> exact containment -> commit

root create: candidate UUID unclaimed -> no root exists -> commit
```

CAS precedes destination/geometry to prevent stale callers probing destinations through error
types. Quaternion, zero, and tuple canonicalization precede OperationID binding; mathematically
equivalent retries cannot become `ErrOperationConflict`.

Memory/durable Core serialize overlapping read/mutation critical sections. CAS, child
empty/coverage proof, containment, cycles, advancing fact cuts, records/tombstones, and outcomes
are one logical commit. Concurrency must not create duplicate roots, out-of-bounds children,
orphans, cycles, missed deletions, or mixed cuts.

## 8. Non-public linear geometry conformance oracle

The implementation must provide a package-private or test-only oracle with the following logical inputs:

```text
ScanIntersecting(
  boundaryAnchorID,
  queryBasisAnchorID,
  at CoreReadCut?,
  QuerySphere { Center, Radius },
  selector = ANCHOR | ENTITY | BOTH,
)
  -> complete ordered Range-intersection matches at one CoreReadCut
```

This is not `Discover` and must not be exported as a public product API. Its sole purpose is to provide a correctness reference that future indexes and discovery implementations cannot weaken:

1. `boundaryAnchorID` defines the complete Structure closure, including itself. `queryBasisAnchorID` defines only the coordinate basis of the query sphere; these are separate parameters. Both must be current Anchors, and the query basis must be the boundary itself or one of its descendants.
2. The query basis may be an arbitrarily deep descendant of the boundary. The oracle still traverses the boundary's entire closure, so it can match sibling branches of the query basis's ancestors and their descendants. A common ancestor must not be misinterpreted as restricting traversal to one sibling branch.
3. The selector filters only the final output, not Anchor traversal. An empty Anchor still has its own Range; whether it is returned depends on the selector and sphere intersection.
4. Within one consistent memory snapshot or Store read transaction, validate the currently published spatial snapshot in `O(current facts)` and scan the closure linearly. Do not consult a BVH, Octree, child cache, persisted WorldPose, or caller-maintained shadow directory. The oracle does not replay retained OperationID history on every scan; full history validation belongs to the Store boundaries in section 9.
5. Project each Element's Range center into the query basis, then apply the exact sphere-intersection predicate in section 4.5. An Entity whose center lies outside the query sphere must still match if the two spheres intersect or are tangent.
6. Sort the output by ElementUUID and attach one common cut. Return the exact intersection set of authoritative Range spheres, not numerically uncertain candidates presented as matches. If completeness cannot be proven, the entire oracle fails; it must not omit entries while reporting complete.

The oracle may use a temporary parent map and memoized transforms within the same snapshot to keep structural access linear. These temporary structures are not Core truth. Query depth, budgets, continuation, authorization views, SpaceHandle, remote sources, and `PARTIAL` belong to the D9–D11 public Discover contract and are not designed here in advance.

The non-public failure matrix is fixed:

| Condition | Whole-call result |
|---|---|
| Non-canonical ID/cut, non-finite center, negative or non-finite radius, or unknown selector | `ErrInvalid` |
| Boundary or basis is confirmed absent | `ErrNotFound` |
| Boundary/basis is not an Anchor, or basis is outside the boundary closure | `ErrConflict` |
| The requested cut from this Core can no longer be served | `ErrReadCutExpired` |
| Corrupt cut/scope, parent graph, or current record held by the oracle | `ErrStore` |
| Exact-work limit exceeded, or derived value is unrepresentable | `ErrUnrepresentable` |

Every failure returns zero matches and a zero cut. The oracle performs no authorization and must not become a Composition runtime entry point that bypasses Realm. Retained operations and Store history are not inputs to each oracle scan. Corruption in them must be caught by full validation at Store create/open/persist/reload boundaries and prevent publication with `ErrStore`; replaying history within a scan must not violate the linear work bound above.

## 9. Durable Store and Greenfield replacement

Memory and bbolt durable implementations must provide the same observable semantics. The unique durable Store recognition marker for this slice is:

```text
ucupa.core.local@5747950a9ca74cbcae658dde84f87b80
```

The marker identifies an exact Store kind; it is not a public protocol version. Store creation atomically establishes the marker, a random private token scope, and a nonzero initial read cut. An empty Store is valid to reopen. Recognition covers record and retained-operation decision semantics, not merely field layout. The explicit Range-update amendment uses this contract's unique marker.

The Store must at least persist and cross-validate:

- Private token scope, current opaque read cut, and each Element's current record or tombstone with its opaque Revision.
- Canonical numeric bit patterns, kind, parent, Pose, Range, and opaque Element Revision.
- Complete OperationID bindings, retained outcomes, and the record/tombstone Revision and commit cut in successful receipts.

Each `COMMITTED` mutation writes its record/tombstone, permanent UUID claim, Revision, fact cut, operation binding, and outcome in one bbolt transaction. `UNCHANGED` and retained negative outcomes write only operation facts and their DecisionCut; they do not advance the Element Revision or fact cut.

Initial creation, every persist, reopening, and reloading after a failure must fail closed by validating the complete current state and operation history before publication. Reopening must also validate the marker, scope/cut, canonical codec, UUIDs, root cardinality, closed kind set, parent integrity, acyclicity, Range kind, all direct containment, final coverage for every historical mutation, and Revision provenance. Missing, empty, or any mismatched marker; missing fields; unknown semantic fields; corrupt records; impossible history; Store lock failures; and transaction failures all return `ErrStore`. Partial opening is forbidden.

The implementation does not read, convert, or migrate any predecessor Core Store. It retains no old FormatID fallback and returns no special "old profile" error. Test fixtures create new Stores in this contract's format directly; Git history preserves the past.

## 10. Predecessor surfaces to remove in the same implementation slice

Completion of this contract requires—and has demonstrated—that the following predecessor surfaces are replaced, not retained alongside their successors:

- The root `LocalPose` argument and persisted field.
- Typed `Anchor`/`Entity` readers, a separate `WorldPose`, `ReanchorAnchor`, and `ReanchorEntity`.
- `PermissionReanchorSubject`/`PermissionReanchorDestination`.
- `ErrUnsupportedProfile` and any predecessor Store reader or marker branch.
- The original in-memory `DelegationBinding`, `MountContext`, `MountAcceptance`, `AdmitMount`, `AcceptMount`, `MatchesDelegation`, `MatchesMountAcceptance`, and mount PermissionIDs.
- `composition/routes*`, the original `acceptance/` federation fixture, demo, and then-current acceptance README claims that depended on those mount DTOs.

The old federation proof lacked durable OperationID/CAS, Range cuts, complete parent-chain evidence, and multi-owner contracts. Adapting it to the new record would import the then-G4 capabilities into the original G1 scope and create a second Core profile. It was therefore removed without a compatibility facade. Successor open-federation designs derive federation anew from D6–D11 and independent interoperability contracts.

The bodies of `docs/concept-v0.md`, historical contracts, and frozen evidence are not rewritten by this code removal. The authority switch updates only the authority manifest and the minimum necessary predecessor status notices; Git and the original texts preserve the historical reasoning.

This slice does not reset the current Facet and Realm contracts. They continue to compose through existing, explicit owner boundaries. G1 made only the minimum changes to exposed Facet compile/test dependencies needed to preserve the existing Core-existence port; it did not implement G2 semantics early.

## 11. Black-box completion gate

G1 may be declared complete only when all the following tests pass through public behavior, the package-private linear oracle in section 8, or a package-private exact-numeric conformance helper used solely to establish the algorithms and work bounds in section 4. Numeric helpers must not replace API state, authorization, convergence, or Store gates.

### 11.1 Records and structure

- UUID canonicality, cross-kind conflicts, and permanent non-reuse after deletion and reopening.
- An empty Store, a unique root, and a root without parent/LocalPose whose Range can only be `UNBOUNDED_ROOT`.
- Only valid final tuples for Anchors and Entities; atomic rejection of a second root, root update/delete, orphans, and cycles.
- Non-root Anchor and Entity updates may retain, enlarge, or shrink Range; an empty Anchor may shrink to zero.
- A valid Anchor shrink covers direct child Anchors, Entities, and their descendants. Tangency is accepted; a one-representable-value shortfall that causes a containment violation is rejected.
- Joint Parent, Pose, and Range updates are validated only as the final tuple. Failure to fit within the new parent or to cover any direct child is rejected atomically.
- Neither success nor rejection automatically changes another Element's record/Revision. Moving a child inward or deleting it does not automatically tighten ancestor Ranges.
- Concurrent parent-Anchor shrinking and outward child movement cannot both commit a state that violates containment.
- An identical canonical final tuple with a current Revision returns UNCHANGED without advancing the record, Revision, or cut.
- Reparenting preserves the UUID and changes only the subject Revision; every descendant record/Revision remains byte-for-byte unchanged.
- Entity deletion and empty non-root Anchor deletion succeed; nonempty Anchor deletion has no side effects.

### 11.2 Numbers and geometry

- Reject NaN, Infinity, and negative radii; read `-0` back as `+0`; round-trip subnormal and maximum-finite bit patterns.
- `q`, `-q`, and exact positive or negative scalar multiples produce the same Rotation. Pivot ties and boundary rounding produce a unique, idempotent representation.
- The +90° Z, composition, translation, and inverse golden vectors in section 4.3 produce bit-exact directions; the opposite convention is not accepted.
- Accept boundary tangency and reject the next representable value outward; exact predicates on extremely large or small dyadics do not overflow.
- Projection, inverse, and non-ancestor failure behavior is deterministic for deep parent chains. Values outside public Scalar output return `ErrUnrepresentable` without corrupting the Store.
- Exact-work edge/bit behavior is deterministic below, exactly at, and first beyond each limit. An over-limit Projection/oracle returns zero results without truncating Structure.
- The oracle matches a huge Entity whose center lies outside the query sphere but whose Range intersects it, tangent spheres, and empty Anchors.
- With the query basis in a deep sibling branch, the oracle still matches another sibling and its descendants under the boundary; selectors do not truncate traversal.
- The oracle agrees exactly with exhaustive expected results at the same cut. No future index corpus may report fewer matches.

Numeric vectors involving nontrivial rounding, adjacent boundaries, subnormals, or extreme values must use hexadecimal binary64 bits, precisely named standard-library constants, or exact `Float64frombits`/`Nextafter` constructions, with explicit UUIDs. Structural fixtures may use exactly representable small integers and a deterministic literal UUID helper. No vector may use a test epsilon to accept an incorrect value as equal. Golden corpora, independent exact references, and fuzz/differential tests provide falsifiable evidence together. Passing tests must not be presented as an exhaustive mathematical proof over all binary64 inputs and arbitrary depths; the specified algorithms define the full-domain semantics.

### 11.3 Reads, concurrency, and convergence

- `ReadElement` does not conflate PRESENT, NOT_FOUND, UNAVAILABLE, or their cut evidence.
- An empty Store's initial cut is nonzero and unchanged after reopening. Revisions, cuts, and continuations from another instance/Store are rejected.
- Core source contains no authorization-free `ConfirmElement`. The Composition adapter calls `ReadElement` with an explicit service actor; the Facet bool consumer port used in this slice is not described as G2 typed evidence.
- `ListDirect` lists both kinds together with deterministic ordering, limits, continuation, final completeness, and empty-list behavior.
- Any concurrent fact mutation causes an old continuation to return `ErrReadCutExpired`. Restarting at the first page obtains one new complete cut; different cuts are never stitched together.
- `ReadElement` and `ProjectToAncestor` requested at the same cut either succeed together or report an expired cut. Continuous writes may starve pagination but must not produce false completeness.
- After clearing all rebuildable temporary structures, `ListDirect`, parent chains, and projection still derive the same results solely from owner truth.
- Each of the seven mutation classes covers its codec, applicable COMMITTED results, and exact replay after reopening. Each shared convergence category—UNCHANGED, different-binding conflict, stale/foreign Revision, retained negative outcome, original outcome cut, and lost-response retry—has at least one semantically applicable representative. The corresponding operation covers kind-specific decision order and invariants; a meaningless seven-operations-by-all-categories matrix is not required.
- A stale subject Revision and missing destination together reliably return `ErrRevisionConflict` first. A syntactically valid cursor-binding mismatch is returned only after parent authorization.
- Authorization precedes state disclosure and replay; missing any permission in an update's all-of set causes no side effects.
- While a controllable fake Authorizer blocks one call, another approved call can enter the Core critical section and complete. This proves that `Authorize` runs outside the Core state lock/Store transaction. A false result establishes only fail-closed `ErrDenied`.
- Concurrent create/update/reparent/delete under race-enabled tests preserves the unique root, CAS, acyclicity, empty proof, and cut atomicity.

### 11.4 Store and removal evidence

- Every applicable COMMITTED, UNCHANGED, and retained negative outcome preserves its outcome/cut after process exit and reopening. Fault injection at each commit boundary produces no partial state.
- Valid Anchor shrinking survives reopening. A shrink rejected because of an out-of-bounds child still replays its original rejection after the child moves inward; only a new OperationID obtains a new decision. Precommit rollback and committed-but-lost responses for shrinking both support exact retries.
- Missing, empty, or any mismatched marker; locked Stores; unknown fields; non-canonical numbers; invalid containment graphs; incorrect Revisions/cuts; and forged operation history all fail closed with `ErrStore`.
- Source/static tests establish mechanically closed sets: a public Core method allowlist, one current marker, `storedElement`/`storedPose` schema allowlists, and predecessor identifier/file/permission-prefix denylists.
- Code review together with invalid/predecessor Store fail-closed behavior establishes the absence of alternative open/routes, old readers/indexes, public Discover, and predecessor compatibility paths. A finite keyword scan must not be called an exhaustive proof against every compatibility branch.
- `go test -race ./...` and `go vet ./...` pass, and the acceptance README claims only owner-local Core truth.

## 12. Explicit exclusions

This contract neither implements nor implies:

- The D5 Facet successor or the complete FacetTypeRef/namespace-attachment lifecycle.
- Public `Enter / Discover / Read / Observe / Propose`, SpaceHandle, StructuralAddress, query depth/budgets, Discover/query continuation, or `COMPLETE/PARTIAL`.
- FactAuthorityRef, delegation/acceptance, Route/referral, rootless carriers, nested mounts, or multi-owner cuts.
- Realm closure authorization, authorization views, hidden-branch privacy, or remote trust.
- D12 writer handoff, owner replacement, cross-authority reanchoring, or unavailability windows.
- Mount retirement or root retirement.
- BVH, Octree, child indexes, spatial caches, event streams, network protocols, process topology, recovery orchestration, or production capacity.
- Authoritative polar/geodetic records, displayName, textual paths, Collider, LOD, assets, or upper-layer application business objects.

These are not rejected UCUPA capabilities. They are independent behaviors that later gates must establish on this contract's truth. G1 completion means only that Core's local spatial truth is executable, restartable, convergent, and exhaustively testable. It must not be advertised as open federation or the complete product.

## 13. Acceptance and effect

The original G1 contract and implementation have been accepted. The [architecture authority manifest](README.md) lists this document as the current Core implementation contract for foundational spatial behavior. The [development plan](development-plan.md) retains the original G1 local-spatial completion record; the explicit Range-update amendment was also independently verified and accepted by the user. Subsequent execution status remains governed by that plan.
`core-minimum-model-correction.md`, `core-anchor-structural-mutations.md`, `phase-11-element-id-decision.md`, and `element-id-profile-name-correction.md` leave the current table and remain historical only.

Any test that forces Core to add a second numeric representation, root Pose, shadow directory, predecessor Store fallback, or unproven federation demonstrates an implementation violation of the contract. It is not grounds for adding an exception as compatibility logic.
