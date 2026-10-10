package contract

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"ucupa/ucupa-core/internal/geometry"
)

func TestSpatialAliasesAreGeometryTypes(t *testing.T) {
	var (
		contractVec    Vec3
		geometryVec    geometry.Vec3
		contractPivot  RotationPivot
		geometryPivot  geometry.RotationPivot
		contractRot    Rotation
		geometryRot    geometry.Rotation
		contractPose   Pose
		geometryPose   geometry.Pose
		contractKind   RangeKind
		geometryKind   geometry.RangeKind
		contractSphere BoundedSphere
		geometrySphere geometry.BoundedSphere
		contractRange  Range
		geometryRange  geometry.Range
	)
	geometryVec, contractVec = contractVec, geometryVec
	geometryPivot, contractPivot = contractPivot, geometryPivot
	geometryRot, contractRot = contractRot, geometryRot
	geometryPose, contractPose = contractPose, geometryPose
	geometryKind, contractKind = contractKind, geometryKind
	geometrySphere, contractSphere = contractSphere, geometrySphere
	geometryRange, contractRange = contractRange, geometryRange
}

func TestSpatialFieldsGettersAndConstants(t *testing.T) {
	vec := Vec3{X: 1, Y: 2, Z: 3}
	if vec.X != 1 || vec.Y != 2 || vec.Z != 3 {
		t.Fatalf("Vec3 fields changed: %#v", vec)
	}
	rotation, err := NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rotation.Pivot() != PivotW || rotation.Ratios() != [3]float64{} {
		t.Fatalf("Rotation getters/constants changed: pivot=%v ratios=%v", rotation.Pivot(), rotation.Ratios())
	}
	pose, err := NewPose(vec, rotation)
	if err != nil || pose.Translation != vec || pose.Rotation != rotation {
		t.Fatalf("Pose fields changed: %#v err=%v", pose, err)
	}
	sphere, err := NewBoundedSphere(2)
	if err != nil || sphere.Radius() != 2 {
		t.Fatalf("BoundedSphere getter changed: %v err=%v", sphere.Radius(), err)
	}
	if RangeBoundedSphere != geometry.RangeBoundedSphere || RangeUnboundedRoot != geometry.RangeUnboundedRoot ||
		PivotX != geometry.PivotX || PivotY != geometry.PivotY || PivotZ != geometry.PivotZ {
		t.Fatal("spatial constants are not aliases")
	}
	var empty Range
	if empty.Kind() != RangeKind(0) {
		t.Fatalf("Range.Kind changed: %v", empty.Kind())
	}
	if bounded, ok := empty.BoundedSphere(); ok || bounded != (BoundedSphere{}) {
		t.Fatalf("zero Range bounded view changed: %#v %v", bounded, ok)
	}
}

func TestSpatialConstructorsAndSentinels(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	rotation, err := NewRotation(1, negativeZero, 0, 0)
	if err != nil || math.Signbit(rotation.Ratios()[0]) {
		t.Fatalf("NewRotation did not canonicalize negative zero: %#v err=%v", rotation, err)
	}
	if _, err := NewRotation(0, 0, 0, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero rotation error = %v, want %v", err, ErrInvalid)
	}
	if _, err := NewRotation(math.NaN(), 0, 0, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NaN rotation error = %v, want %v", err, ErrInvalid)
	}
	pose, err := NewPose(Vec3{X: negativeZero}, rotation)
	if err != nil || math.Signbit(pose.Translation.X) {
		t.Fatalf("NewPose did not canonicalize negative zero: %#v err=%v", pose, err)
	}
	if _, err := NewPose(Vec3{}, Rotation{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero Rotation accepted by NewPose: %v", err)
	}
	if _, err := NewPose(Vec3{X: math.Inf(1)}, rotation); !errors.Is(err, ErrInvalid) {
		t.Fatalf("infinite translation error = %v, want %v", err, ErrInvalid)
	}
	sphere, err := NewBoundedSphere(negativeZero)
	if err != nil || math.Signbit(sphere.Radius()) || sphere.Radius() != 0 {
		t.Fatalf("NewBoundedSphere did not canonicalize negative zero: %x err=%v", math.Float64bits(sphere.Radius()), err)
	}
	if _, err := NewBoundedSphere(-1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative sphere error = %v, want %v", err, ErrInvalid)
	}
	if _, err := NewBoundedSphere(math.Inf(1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("infinite sphere error = %v, want %v", err, ErrInvalid)
	}
	if ErrInvalid != geometry.ErrInvalid || ErrUnrepresentable != geometry.ErrUnrepresentable {
		t.Fatal("contract errors are not the geometry sentinels")
	}
	if ErrInvalid.Error() != "invalid Core input" || ErrUnrepresentable.Error() != "Core result is unrepresentable" {
		t.Fatal("geometry sentinel text changed")
	}
	if !errors.Is(errors.Join(ErrInvalid, errors.New("context")), geometry.ErrInvalid) ||
		!errors.Is(errors.Join(ErrUnrepresentable, errors.New("context")), geometry.ErrUnrepresentable) {
		t.Fatal("errors.Is no longer recognizes geometry sentinels")
	}
}

func TestSpatialTypesExposeOnlyExistingGetters(t *testing.T) {
	for typ, want := range map[reflect.Type]map[string]bool{
		reflect.TypeOf(Rotation{}):      {"Pivot": true, "Ratios": true},
		reflect.TypeOf(BoundedSphere{}): {"Radius": true},
		reflect.TypeOf(Range{}):         {"Kind": true, "BoundedSphere": true},
	} {
		if typ.NumMethod() != len(want) {
			t.Fatalf("%v exported method count = %d, want %d", typ, typ.NumMethod(), len(want))
		}
		for index := 0; index < typ.NumMethod(); index++ {
			if !want[typ.Method(index).Name] {
				t.Fatalf("%v unexpected method %q", typ, typ.Method(index).Name)
			}
		}
	}
}
