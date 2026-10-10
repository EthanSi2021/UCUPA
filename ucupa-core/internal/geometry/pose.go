// Package geometry owns Core's pure spatial values and exact arithmetic.
//
// The package intentionally has no dependency on Core state, Store, contracts,
// or transport. Spatial values keep their representation private so callers
// must use the constructors or the explicitly documented raw Store rebuild
// paths.
package geometry

import (
	"math"
	"math/big"

	"ucupa/ucupa-core/internal/failure"
)

var (
	ErrInvalid         = failure.ErrInvalid
	ErrUnrepresentable = failure.ErrUnrepresentable
)

type Vec3 struct{ X, Y, Z float64 }
type RotationPivot uint8

const (
	PivotW RotationPivot = iota + 1
	PivotX
	PivotY
	PivotZ
)

type Rotation struct {
	pivot  RotationPivot
	ratios [3]float64
}

type Pose struct {
	Translation Vec3
	Rotation    Rotation
}

type RangeKind uint8

const (
	RangeBoundedSphere RangeKind = iota + 1
	RangeUnboundedRoot
)

type BoundedSphere struct {
	radius     float64
	validValue bool
}

type Range struct {
	kind   RangeKind
	sphere BoundedSphere
}

func NewRotation(w, x, y, z float64) (Rotation, error) {
	c := [4]float64{w, x, y, z}
	for i := range c {
		if !Finite(c[i]) {
			return Rotation{}, ErrInvalid
		}
		c[i] = CanonicalZero(c[i])
	}
	return rotationFromExact([4]*big.Rat{exactFloat(c[0]), exactFloat(c[1]), exactFloat(c[2]), exactFloat(c[3])})
}

func NewPose(t Vec3, r Rotation) (Pose, error) {
	if !Finite(t.X) || !Finite(t.Y) || !Finite(t.Z) || !ValidRotation(r) {
		return Pose{}, ErrInvalid
	}
	t.X = CanonicalZero(t.X)
	t.Y = CanonicalZero(t.Y)
	t.Z = CanonicalZero(t.Z)
	return Pose{Translation: t, Rotation: r}, nil
}

func NewBoundedSphere(radius float64) (BoundedSphere, error) {
	if !Finite(radius) || radius < 0 {
		return BoundedSphere{}, ErrInvalid
	}
	return BoundedSphere{radius: CanonicalZero(radius), validValue: true}, nil
}

func (r Rotation) Pivot() RotationPivot { return r.pivot }
func (r Rotation) Ratios() [3]float64   { return r.ratios }
func (s BoundedSphere) Radius() float64 { return s.radius }
func (r Range) Kind() RangeKind         { return r.kind }
func (r Range) BoundedSphere() (BoundedSphere, bool) {
	return r.sphere, r.kind == RangeBoundedSphere
}

// IdentityPose returns the canonical identity orientation at the origin.
func IdentityPose() Pose { return Pose{Rotation: Rotation{pivot: PivotW}} }

// BoundedRange returns an invalid zero Range when the sphere is not valid.
func BoundedRange(s BoundedSphere) Range {
	if !ValidSphere(s) {
		return Range{}
	}
	return Range{kind: RangeBoundedSphere, sphere: s}
}

// UnboundedRootRange returns the only unbounded range form.
func UnboundedRootRange() Range { return Range{kind: RangeUnboundedRoot} }

// RotationFromComponents rebuilds the private representation exactly as
// stored. It deliberately does not normalize or canonicalize signed zero;
// callers loading Store bytes must validate the result separately.
func RotationFromComponents(pivot RotationPivot, ratios [3]float64) Rotation {
	return Rotation{pivot: pivot, ratios: ratios}
}

// SphereFromStored rebuilds a persisted sphere without normalizing its radius.
// In particular, a negative zero remains observable to ValidSphere.
func SphereFromStored(radius float64) BoundedSphere {
	return BoundedSphere{radius: radius, validValue: true}
}

func ValidRotation(r Rotation) bool {
	if r.pivot < PivotW || r.pivot > PivotZ {
		return false
	}
	p := int(r.pivot) - 1
	for component, ri := 0, 0; component < 4; component++ {
		if component == p {
			continue
		}
		v := r.ratios[ri]
		if !Finite(v) || (v == 0 && math.Signbit(v)) || math.Abs(v) > 1 || (component < p && math.Abs(v) >= 1) {
			return false
		}
		ri++
	}
	return true
}

func ValidSphere(s BoundedSphere) bool {
	return s.validValue && Finite(s.radius) && s.radius >= 0 && !(s.radius == 0 && math.Signbit(s.radius))
}

func ValidRange(r Range) bool {
	return (r.kind == RangeUnboundedRoot && r.sphere == (BoundedSphere{})) ||
		(r.kind == RangeBoundedSphere && ValidSphere(r.sphere))
}

func ValidPose(p Pose) bool {
	_, err := NewPose(p.Translation, p.Rotation)
	return err == nil
}

func Finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func CanonicalZero(v float64) float64 {
	if v == 0 {
		return 0
	}
	return v
}
