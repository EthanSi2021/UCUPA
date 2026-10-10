package contract

import "ucupa/ucupa-core/internal/geometry"

// Spatial values have one representation. The private geometry package owns
// canonical construction and exact computation; these aliases expose only the
// existing value fields and read-only methods, not internal reconstruction.
type Vec3 = geometry.Vec3
type RotationPivot = geometry.RotationPivot
type Rotation = geometry.Rotation
type Pose = geometry.Pose
type RangeKind = geometry.RangeKind
type BoundedSphere = geometry.BoundedSphere
type Range = geometry.Range

const (
	PivotW             = geometry.PivotW
	PivotX             = geometry.PivotX
	PivotY             = geometry.PivotY
	PivotZ             = geometry.PivotZ
	RangeBoundedSphere = geometry.RangeBoundedSphere
	RangeUnboundedRoot = geometry.RangeUnboundedRoot
)

// These are the existing public construction operations, now at the value
// contract boundary. Store/wire reconstruction remains private and must not
// replace these canonicalizing constructors for local callers.
func NewRotation(w, x, y, z float64) (Rotation, error) {
	return geometry.NewRotation(w, x, y, z)
}

func NewPose(t Vec3, r Rotation) (Pose, error) {
	return geometry.NewPose(t, r)
}

func NewBoundedSphere(radius float64) (BoundedSphere, error) {
	return geometry.NewBoundedSphere(radius)
}

// Shared error identities keep direct equality and errors.Is unchanged across
// spatial construction, owner operations, and the public error projection.
var (
	ErrInvalid         = geometry.ErrInvalid
	ErrUnrepresentable = geometry.ErrUnrepresentable
)
