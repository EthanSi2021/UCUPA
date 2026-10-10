package geometry

import (
	"math"
	"testing"
)

func TestEqualityPreservesBitsWithoutNormalizing(t *testing.T) {
	pose := IdentityPose()
	negativeZero := pose
	negativeZero.Translation.X = math.Copysign(0, -1)
	if !EqualPose(pose, pose) || EqualPose(pose, negativeZero) {
		t.Fatal("pose equality must distinguish negative zero")
	}
	invalid := Pose{}
	if !EqualPose(invalid, invalid) || CanonicalPose(invalid) {
		t.Fatal("representation equality is not validity")
	}
	if !CanonicalPose(pose) || CanonicalPose(negativeZero) {
		t.Fatal("canonical pose validation changed")
	}
	for _, value := range []float64{math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN()} {
		if CanonicalFinite(value) {
			t.Fatalf("accepted noncanonical scalar %v", value)
		}
	}
	for _, value := range []float64{0, 1, -1, math.SmallestNonzeroFloat64, math.MaxFloat64} {
		if !CanonicalFinite(value) {
			t.Fatalf("rejected canonical scalar %v", value)
		}
	}
	sphere, err := NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewBoundedSphere(math.Nextafter(1, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !EqualSphere(sphere, sphere) || EqualSphere(sphere, next) || EqualSphere(BoundedSphere{}, BoundedSphere{}) {
		t.Fatal("sphere equality must retain validity and exact radius")
	}
}
