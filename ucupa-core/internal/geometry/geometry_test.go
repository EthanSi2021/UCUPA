package geometry

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
)

func TestProjectiveRotationCanonicalization(t *testing.T) {
	a, e := NewRotation(2, 0, 0, 2)
	if e != nil {
		t.Fatal(e)
	}
	b, e := NewRotation(-4, math.Copysign(0, -1), 0, -4)
	if e != nil {
		t.Fatal(e)
	}
	if a != b || a.Pivot() != PivotW || a.Ratios() != [3]float64{0, 0, 1} {
		t.Fatalf("rotations %#v %#v", a, b)
	}
	if _, e = NewRotation(0, 0, 0, 0); e != ErrInvalid {
		t.Fatalf("zero error %v", e)
	}
}

func TestProjectiveRotationCriticalPivotRounding(t *testing.T) {
	one := big.NewRat(1, 1)
	deltaDenominator := new(big.Int).Lsh(big.NewInt(1), 55)
	delta := new(big.Rat).SetFrac(big.NewInt(1), deltaDenominator)
	justBelowOne := new(big.Rat).Sub(new(big.Rat).Set(one), delta)
	zero := func() *big.Rat { return new(big.Rat) }

	for _, test := range []struct {
		name      string
		component *big.Rat
		want      float64
	}{
		{name: "positive", component: justBelowOne, want: math.Nextafter(1, 0)},
		{name: "negative", component: new(big.Rat).Neg(justBelowOne), want: math.Nextafter(-1, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			rotation, err := rotationFromExact([4]*big.Rat{
				new(big.Rat).Set(test.component), new(big.Rat).Set(one), zero(), zero(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if rotation.Pivot() != PivotX || rotation.Ratios() != [3]float64{test.want, 0, 0} {
				t.Fatalf("critical canonical Rotation: %#v", rotation)
			}
			idempotent, err := NewRotation(test.want, 1, 0, 0)
			if err != nil || idempotent != rotation {
				t.Fatalf("canonical Rotation is not idempotent: got=%#v err=%v want=%#v", idempotent, err, rotation)
			}
		})
	}
}

func TestExactSE3Convention(t *testing.T) {
	q, e := NewRotation(1, 0, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	o, _ := NewPose(Vec3{Y: 2}, q)
	i, _ := NewPose(Vec3{X: 1}, IdentityPose().Rotation)
	p, e := composePoseExact(o, i)
	if e != nil {
		t.Fatal(e)
	}
	if p.Translation != (Vec3{Y: 3}) || p.Rotation != q {
		t.Fatalf("compose %#v", p)
	}
	v := rotateExact(exactRotation(q), exactVec3{exactFloat(1), exactFloat(0), exactFloat(0)})
	if v.x.Sign() != 0 || v.y.Cmp(exactFloat(1)) != 0 || v.z.Sign() != 0 {
		t.Fatalf("rotate %s %s %s", v.x, v.y, v.z)
	}
	twice, e := composePoseExact(Pose{Rotation: q}, Pose{Rotation: q})
	if e != nil {
		t.Fatal(e)
	}
	v = rotateExact(exactRotation(twice.Rotation), exactVec3{exactFloat(1), exactFloat(0), exactFloat(0)})
	if v.x.Cmp(exactFloat(-1)) != 0 || v.y.Sign() != 0 || v.z.Sign() != 0 {
		t.Fatalf("twice-composed rotation %s %s %s", v.x, v.y, v.z)
	}
	inv := inverseExact(ExactFromPose(Pose{Rotation: q}))
	v = rotateExact(inv.q, exactVec3{exactFloat(1), exactFloat(0), exactFloat(0)})
	if v.x.Sign() != 0 || v.y.Cmp(exactFloat(-1)) != 0 || v.z.Sign() != 0 {
		t.Fatalf("inverse %s %s %s", v.x, v.y, v.z)
	}
}

func TestExactContainmentAndIntersection(t *testing.T) {
	p, _ := NewBoundedSphere(2)
	c, _ := NewBoundedSphere(1)
	at, _ := NewPose(Vec3{X: 1}, IdentityPose().Rotation)
	ok, e := Contains(p, at, c)
	if e != nil || !ok {
		t.Fatalf("tangent %v %v", ok, e)
	}
	out, _ := NewPose(Vec3{X: math.Nextafter(1, 2)}, IdentityPose().Rotation)
	ok, e = Contains(p, out, c)
	if e != nil || ok {
		t.Fatalf("outside %v %v", ok, e)
	}
	ok, e = spheresIntersect(Vec3{}, c, Vec3{X: 2}, c)
	if e != nil || !ok {
		t.Fatalf("intersection %v %v", ok, e)
	}
}

func TestExactContainmentAtBinary64Extremes(t *testing.T) {
	parent, err := NewBoundedSphere(math.MaxFloat64)
	if err != nil {
		t.Fatal(err)
	}
	point, err := NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	tangent, err := NewPose(Vec3{X: math.MaxFloat64}, IdentityPose().Rotation)
	if err != nil {
		t.Fatal(err)
	}
	inside, err := Contains(parent, tangent, point)
	if err != nil || !inside {
		t.Fatalf("max-finite tangent: inside=%v err=%v", inside, err)
	}
	outside, err := NewPose(
		Vec3{X: math.MaxFloat64, Y: math.SmallestNonzeroFloat64},
		IdentityPose().Rotation,
	)
	if err != nil {
		t.Fatal(err)
	}
	inside, err = Contains(parent, outside, point)
	if err != nil || inside {
		t.Fatalf("exact extreme outside point: inside=%v err=%v", inside, err)
	}
}

func TestScalarCanonicalizationAndRange(t *testing.T) {
	p, e := NewPose(Vec3{X: math.Copysign(0, -1)}, IdentityPose().Rotation)
	if e != nil || math.Signbit(p.Translation.X) {
		t.Fatalf("zero %#v %v", p, e)
	}
	s, e := NewBoundedSphere(math.SmallestNonzeroFloat64)
	if e != nil || s.Radius() != math.SmallestNonzeroFloat64 {
		t.Fatalf("subnormal %x %v", math.Float64bits(s.Radius()), e)
	}
	if _, e = NewBoundedSphere(-1); e != ErrInvalid {
		t.Fatalf("negative %v", e)
	}
	for _, invalid := range []float64{math.Inf(-1), math.Inf(1), math.NaN()} {
		if _, err := NewBoundedSphere(invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid radius %v: %v", invalid, err)
		}
		if _, err := NewPose(Vec3{X: invalid}, IdentityPose().Rotation); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid translation %v: %v", invalid, err)
		}
		if _, err := NewRotation(1, invalid, 0, 0); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid quaternion component %v: %v", invalid, err)
		}
	}
	zero, e := NewBoundedSphere(0)
	if e != nil || !ValidSphere(zero) || ValidSphere((BoundedSphere{})) {
		t.Fatalf("bounded zero validity: constructed=%v zero-value=%v err=%v", ValidSphere(zero), ValidSphere((BoundedSphere{})), e)
	}
	if !ValidRange(Range{}) && ValidRange(UnboundedRootRange()) && ValidRange(BoundedRange(zero)) {
		return
	}
	t.Fatal("Range zero/variant validity is not closed")
}

func TestPublicRotationCanonicalizationEnforcesPrimitiveWorkLimit(t *testing.T) {
	// Each input component is independently within the 65,536-bit limit, but
	// component/pivot is irreducible and exceeds it. The output boundary must
	// reject the ratio primitive instead of rounding it to binary64.
	const exponent = 32769
	power := new(big.Int).Lsh(big.NewInt(1), exponent)
	powerMinusOne := new(big.Int).Sub(new(big.Int).Set(power), big.NewInt(1))
	powerMinusTwo := new(big.Int).Sub(new(big.Int).Set(power), big.NewInt(2))
	pivot := new(big.Rat).SetFrac(new(big.Int).Set(powerMinusOne), new(big.Int).Set(power))
	component := new(big.Rat).SetFrac(new(big.Int).Set(powerMinusTwo), new(big.Int).Set(powerMinusOne))
	zero := func() *big.Rat { return new(big.Rat) }
	value := ExactPose{
		t: exactVec3{zero(), zero(), zero()},
		q: exactQuaternion{pivot, component, zero(), zero()},
	}
	if !PoseWithinWorkLimit(value) {
		t.Fatal("fixture components unexpectedly exceed the work limit")
	}
	if result, err := publicPose(value); !errors.Is(err, ErrUnrepresentable) || result != (Pose{}) {
		t.Fatalf("oversized ratio primitive: %#v err=%v", result, err)
	}
}

func TestExactRationalBitBoundaryAtPublicPose(t *testing.T) {
	zero := func() *big.Rat { return new(big.Rat) }
	identity := exactQuaternion{w: big.NewRat(1, 1), x: zero(), y: zero(), z: zero()}
	for _, test := range []struct {
		bits    int
		wantErr error
	}{
		{bits: MaxExactRationalBits - 1},
		{bits: MaxExactRationalBits},
		{bits: MaxExactRationalBits + 1, wantErr: ErrUnrepresentable},
	} {
		t.Run(fmt.Sprintf("%d-bits", test.bits), func(t *testing.T) {
			denominator := new(big.Int).Lsh(big.NewInt(1), uint(test.bits-1))
			value := new(big.Rat).SetFrac(big.NewInt(1), denominator)
			if got := rationalWithinWorkLimit(value); got != (test.wantErr == nil) {
				t.Fatalf("rational boundary acceptance = %v", got)
			}
			result, err := publicPose(ExactPose{
				t: exactVec3{x: value, y: zero(), z: zero()},
				q: identity,
			})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || result != (Pose{}) {
					t.Fatalf("out-of-limit public Pose: %#v err=%v", result, err)
				}
				return
			}
			if err != nil || result.Translation != (Vec3{}) || result.Rotation != IdentityPose().Rotation {
				t.Fatalf("in-limit public Pose: %#v err=%v", result, err)
			}
		})
	}
}

func TestStoredReconstructionDoesNotNormalize(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	rotation := RotationFromComponents(PivotW, [3]float64{negativeZero, 0, 0})
	if !math.Signbit(rotation.Ratios()[0]) || ValidRotation(rotation) {
		t.Fatalf("raw rotation was normalized or accepted: %#v valid=%v", rotation, ValidRotation(rotation))
	}
	sphere := SphereFromStored(negativeZero)
	if !math.Signbit(sphere.Radius()) || ValidSphere(sphere) {
		t.Fatalf("raw sphere was normalized or accepted: radius=%x valid=%v", math.Float64bits(sphere.Radius()), ValidSphere(sphere))
	}
}

func TestCenterIntersectsPrimitiveWorkBoundary(t *testing.T) {
	zero, err := NewBoundedSphere(0)
	if err != nil {
		t.Fatal(err)
	}
	identity := ExactFromPose(IdentityPose())
	intersects, err := CenterIntersects(identity, zero, Vec3{}, zero)
	if err != nil || !intersects {
		t.Fatalf("identity center intersection: %v %v", intersects, err)
	}
	if _, err := CenterIntersects(ExactPose{}, zero, Vec3{}, zero); err != ErrInvalid {
		t.Fatalf("invalid exact pose error = %v, want %v", err, ErrInvalid)
	}
	if _, err := CenterIntersects(identity, zero, Vec3{X: math.NaN()}, zero); err != ErrInvalid {
		t.Fatalf("invalid query center error = %v, want %v", err, ErrInvalid)
	}

	denominator := new(big.Int).Lsh(big.NewInt(1), MaxExactRationalBits-1)
	component := new(big.Rat).SetFrac(big.NewInt(1), denominator)
	value := ExactPose{
		t: exactVec3{x: component, y: newRat(), z: newRat()},
		q: exactQuaternion{w: big.NewRat(1, 1), x: newRat(), y: newRat(), z: newRat()},
	}
	if !PoseWithinWorkLimit(value) {
		t.Fatal("primitive-boundary fixture unexpectedly exceeds input limit")
	}
	if _, err := CenterIntersects(value, zero, Vec3{}, zero); err != ErrUnrepresentable {
		t.Fatalf("primitive overflow error = %v, want %v", err, ErrUnrepresentable)
	}
}

func FuzzProjectiveRotationCanonicalization(f *testing.F) {
	for _, seed := range [][4]float64{
		{2, 0, 0, 2},
		{-4, math.Copysign(0, -1), 0, -4},
		{1, 1, 1, 1},
		{math.SmallestNonzeroFloat64, 0, 0, 0},
		{math.MaxFloat64, -math.MaxFloat64, 1, -1},
	} {
		f.Add(
			math.Float64bits(seed[0]), math.Float64bits(seed[1]),
			math.Float64bits(seed[2]), math.Float64bits(seed[3]),
		)
	}

	f.Fuzz(func(t *testing.T, wb, xb, yb, zb uint64) {
		components := [4]float64{
			math.Float64frombits(wb), math.Float64frombits(xb),
			math.Float64frombits(yb), math.Float64frombits(zb),
		}
		allZero := true
		for _, component := range components {
			if !Finite(component) {
				if _, err := NewRotation(components[0], components[1], components[2], components[3]); !errors.Is(err, ErrInvalid) {
					t.Fatalf("non-finite quaternion accepted: %x err=%v", [4]uint64{wb, xb, yb, zb}, err)
				}
				return
			}
			allZero = allZero && component == 0
		}
		if allZero {
			if _, err := NewRotation(components[0], components[1], components[2], components[3]); !errors.Is(err, ErrInvalid) {
				t.Fatalf("zero quaternion accepted: %x err=%v", [4]uint64{wb, xb, yb, zb}, err)
			}
			return
		}

		rotation, err := NewRotation(components[0], components[1], components[2], components[3])
		if err != nil {
			t.Fatal(err)
		}
		negated, err := NewRotation(-components[0], -components[1], -components[2], -components[3])
		if err != nil || negated != rotation {
			t.Fatalf("q and -q diverged: q=%#v -q=%#v err=%v", rotation, negated, err)
		}

		var canonical [4]float64
		pivot := int(rotation.Pivot()) - 1
		canonical[pivot] = 1
		ratios := rotation.Ratios()
		for component, ratio := 0, 0; component < 4; component++ {
			if component == pivot {
				continue
			}
			canonical[component] = ratios[ratio]
			ratio++
		}
		idempotent, err := NewRotation(canonical[0], canonical[1], canonical[2], canonical[3])
		if err != nil || idempotent != rotation {
			t.Fatalf("canonical Rotation is not idempotent: first=%#v second=%#v err=%v", rotation, idempotent, err)
		}
	})
}

func FuzzExactContainmentDifferential(f *testing.F) {
	for _, seed := range [][5]float64{
		{2, 1, 1, 0, 0},
		{2, 1, math.Nextafter(1, 2), 0, 0},
		{math.MaxFloat64, 0, math.MaxFloat64, 0, 0},
		{math.MaxFloat64, 0, math.MaxFloat64, math.SmallestNonzeroFloat64, 0},
		{math.SmallestNonzeroFloat64, 0, math.SmallestNonzeroFloat64, 0, 0},
	} {
		f.Add(
			math.Float64bits(seed[0]), math.Float64bits(seed[1]),
			math.Float64bits(seed[2]), math.Float64bits(seed[3]), math.Float64bits(seed[4]),
		)
	}

	f.Fuzz(func(t *testing.T, parentBits, childBits, xBits, yBits, zBits uint64) {
		parentRadius := math.Float64frombits(parentBits)
		childRadius := math.Float64frombits(childBits)
		translation := Vec3{
			X: math.Float64frombits(xBits),
			Y: math.Float64frombits(yBits),
			Z: math.Float64frombits(zBits),
		}
		if !Finite(parentRadius) || !Finite(childRadius) || parentRadius < 0 || childRadius < 0 ||
			!Finite(translation.X) || !Finite(translation.Y) || !Finite(translation.Z) {
			return
		}
		parent, err := NewBoundedSphere(parentRadius)
		if err != nil {
			t.Fatal(err)
		}
		child, err := NewBoundedSphere(childRadius)
		if err != nil {
			t.Fatal(err)
		}
		pose, err := NewPose(translation, IdentityPose().Rotation)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Contains(parent, pose, child)
		if err != nil {
			t.Fatal(err)
		}
		want := referenceContains(parentRadius, childRadius, translation)
		if got != want {
			t.Fatalf(
				"containment differs from independent exact reference: parent=%016x child=%016x translation=[%016x %016x %016x] got=%v want=%v",
				parentBits, childBits, xBits, yBits, zBits, got, want,
			)
		}
	})
}

func referenceContains(parentRadius, childRadius float64, translation Vec3) bool {
	exact := func(value float64) *big.Rat {
		bits := math.Float64bits(value)
		negative := bits>>63 != 0
		exponent := int((bits >> 52) & 0x7ff)
		mantissa := bits & ((uint64(1) << 52) - 1)
		power := -1074
		if exponent != 0 {
			mantissa |= uint64(1) << 52
			power = exponent - 1023 - 52
		}
		numerator := new(big.Int).SetUint64(mantissa)
		if negative {
			numerator.Neg(numerator)
		}
		denominator := big.NewInt(1)
		if power >= 0 {
			numerator.Lsh(numerator, uint(power))
		} else {
			denominator.Lsh(denominator, uint(-power))
		}
		return new(big.Rat).SetFrac(numerator, denominator)
	}
	parent := exact(parentRadius)
	child := exact(childRadius)
	if child.Cmp(parent) > 0 {
		return false
	}
	difference := new(big.Rat).Sub(parent, child)
	distanceSquared := new(big.Rat)
	for _, coordinate := range []float64{translation.X, translation.Y, translation.Z} {
		value := exact(coordinate)
		distanceSquared.Add(distanceSquared, new(big.Rat).Mul(value, value))
	}
	return distanceSquared.Cmp(new(big.Rat).Mul(difference, difference)) <= 0
}
