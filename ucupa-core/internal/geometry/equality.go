package geometry

import "math"

// EqualPose compares the canonical representation bit for bit. Validation is
// separate: identical invalid records must not be silently normalized here.
func EqualPose(left, right Pose) bool {
	if left.Rotation.Pivot() != right.Rotation.Pivot() {
		return false
	}
	leftRatios, rightRatios := left.Rotation.Ratios(), right.Rotation.Ratios()
	leftValues := [...]float64{
		left.Translation.X, left.Translation.Y, left.Translation.Z,
		leftRatios[0], leftRatios[1], leftRatios[2],
	}
	rightValues := [...]float64{
		right.Translation.X, right.Translation.Y, right.Translation.Z,
		rightRatios[0], rightRatios[1], rightRatios[2],
	}
	for index := range leftValues {
		if math.Float64bits(leftValues[index]) != math.Float64bits(rightValues[index]) {
			return false
		}
	}
	return true
}

// EqualSphere retains the existing valid-sphere prerequisite and exact bits.
func EqualSphere(left, right BoundedSphere) bool {
	return ValidSphere(left) && ValidSphere(right) &&
		math.Float64bits(left.Radius()) == math.Float64bits(right.Radius())
}

// CanonicalPose accepts only the representation already required at Store and
// public write boundaries; it does not repair a noncanonical input.
func CanonicalPose(pose Pose) bool {
	return ValidPose(pose) && CanonicalFinite(pose.Translation.X) && CanonicalFinite(pose.Translation.Y) && CanonicalFinite(pose.Translation.Z) && ValidRotation(pose.Rotation)
}

func CanonicalFinite(value float64) bool {
	return Finite(value) && !(value == 0 && math.Signbit(value))
}
