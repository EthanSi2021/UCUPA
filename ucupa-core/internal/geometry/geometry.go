package geometry

import (
	"math"
	"math/big"
)

const (
	MaxExactProjectionEdges = 1024
	MaxExactRationalBits    = 65536
)

type exactVec3 struct{ x, y, z *big.Rat }
type exactQuaternion struct{ w, x, y, z *big.Rat }

// ExactPose is the private exact-arithmetic pose representation. Its rational
// coordinates remain inaccessible; callers pass values back to this package's
// exact operations instead of obtaining rational getters.
type ExactPose struct {
	t exactVec3
	q exactQuaternion
}

func newRat() *big.Rat { return new(big.Rat) }

func exactFloat(v float64) *big.Rat {
	r := newRat()
	if r.SetFloat64(v) == nil {
		panic("non-finite exact float")
	}
	return r
}

func ratAbsLessThanOne(v *big.Rat) bool { return newRat().Abs(v).Cmp(big.NewRat(1, 1)) < 0 }

func rotationFromExact(c [4]*big.Rat) (Rotation, error) {
	return rotationFromExactProfile(c, false)
}

func rotationFromExactWithinWorkLimit(c [4]*big.Rat) (Rotation, error) {
	return rotationFromExactProfile(c, true)
}

func rotationFromExactProfile(c [4]*big.Rat, enforceWorkLimit bool) (Rotation, error) {
	if enforceWorkLimit {
		for _, component := range c {
			if !rationalWithinWorkLimit(component) {
				return Rotation{}, ErrUnrepresentable
			}
		}
	}
	p := 0
	for i := 1; i < 4; i++ {
		if newRat().Abs(c[i]).Cmp(newRat().Abs(c[p])) > 0 {
			p = i
		}
	}
	if c[p].Sign() == 0 {
		return Rotation{}, ErrInvalid
	}
	sign := int64(1)
	if c[p].Sign() < 0 {
		sign = -1
	}
	pivot := newRat().Mul(c[p], big.NewRat(sign, 1))
	var ratios [3]float64
	for component, ri := 0, 0; component < 4; component++ {
		if component == p {
			continue
		}
		numerator := newRat().Mul(c[component], big.NewRat(sign, 1))
		ratio := newRat().Quo(numerator, pivot)
		if enforceWorkLimit && !rationalWithinWorkLimit(ratio) {
			return Rotation{}, ErrUnrepresentable
		}
		rounded, _ := ratio.Float64()
		if !Finite(rounded) {
			return Rotation{}, ErrUnrepresentable
		}
		if component < p && ratAbsLessThanOne(ratio) && math.Abs(rounded) == 1 {
			rounded = math.Nextafter(rounded, 0)
		}
		ratios[ri] = CanonicalZero(rounded)
		ri++
	}
	return RotationFromComponents(RotationPivot(p+1), ratios), nil
}

func exactRotation(r Rotation) exactQuaternion {
	p := int(r.pivot) - 1
	v := [4]*big.Rat{newRat(), newRat(), newRat(), newRat()}
	v[p].SetInt64(1)
	for c, ri := 0, 0; c < 4; c++ {
		if c == p {
			continue
		}
		v[c] = exactFloat(r.ratios[ri])
		ri++
	}
	return exactQuaternion{v[0], v[1], v[2], v[3]}
}

// ExactFromPose converts a validated public pose to exact arithmetic. It
// preserves the existing conversion contract and performs no public rounding.
func ExactFromPose(p Pose) ExactPose {
	return ExactPose{exactVec3{exactFloat(p.Translation.X), exactFloat(p.Translation.Y), exactFloat(p.Translation.Z)}, exactRotation(p.Rotation)}
}

func mulRat(a, b *big.Rat) *big.Rat { return newRat().Mul(a, b) }

func hamilton(a, b exactQuaternion) exactQuaternion {
	add := func(a, b *big.Rat) *big.Rat { return newRat().Add(a, b) }
	sub := func(a, b *big.Rat) *big.Rat { return newRat().Sub(a, b) }
	return exactQuaternion{
		sub(sub(sub(mulRat(a.w, b.w), mulRat(a.x, b.x)), mulRat(a.y, b.y)), mulRat(a.z, b.z)),
		sub(add(add(mulRat(a.w, b.x), mulRat(a.x, b.w)), mulRat(a.y, b.z)), mulRat(a.z, b.y)),
		add(add(sub(mulRat(a.w, b.y), mulRat(a.x, b.z)), mulRat(a.y, b.w)), mulRat(a.z, b.x)),
		add(sub(add(mulRat(a.w, b.z), mulRat(a.x, b.y)), mulRat(a.y, b.x)), mulRat(a.z, b.w)),
	}
}

func conjugate(q exactQuaternion) exactQuaternion {
	return exactQuaternion{newRat().Set(q.w), newRat().Neg(q.x), newRat().Neg(q.y), newRat().Neg(q.z)}
}

func square(v *big.Rat) *big.Rat { return newRat().Mul(v, v) }

func addRats(v ...*big.Rat) *big.Rat {
	r := newRat()
	for _, x := range v {
		r.Add(r, x)
	}
	return r
}

func rotateExact(q exactQuaternion, v exactVec3) exactVec3 {
	z := newRat()
	p := hamilton(hamilton(q, exactQuaternion{z, v.x, v.y, v.z}), conjugate(q))
	n := addRats(square(q.w), square(q.x), square(q.y), square(q.z))
	return exactVec3{newRat().Quo(p.x, n), newRat().Quo(p.y, n), newRat().Quo(p.z, n)}
}

func inverseExact(p ExactPose) ExactPose {
	q := conjugate(p.q)
	t := rotateExact(q, p.t)
	return ExactPose{exactVec3{newRat().Neg(t.x), newRat().Neg(t.y), newRat().Neg(t.z)}, q}
}

func publicPose(p ExactPose) (Pose, error) {
	if !PoseWithinWorkLimit(p) {
		return Pose{}, ErrUnrepresentable
	}
	v := [3]*big.Rat{p.t.x, p.t.y, p.t.z}
	out := Vec3{}
	dst := []*float64{&out.X, &out.Y, &out.Z}
	for i := range v {
		x, _ := v[i].Float64()
		if !Finite(x) {
			return Pose{}, ErrUnrepresentable
		}
		*dst[i] = CanonicalZero(x)
	}
	r, err := rotationFromExactWithinWorkLimit([4]*big.Rat{p.q.w, p.q.x, p.q.y, p.q.z})
	if err != nil {
		return Pose{}, ErrUnrepresentable
	}
	return NewPose(out, r)
}

// composePoseExact composes outer and inner public poses using exact arithmetic.
func composePoseExact(o, i Pose) (Pose, error) {
	if !ValidPose(o) || !ValidPose(i) {
		return Pose{}, ErrInvalid
	}
	w := &Work{}
	result := w.Compose(ExactFromPose(o), ExactFromPose(i))
	if w.Failed() {
		return Pose{}, ErrUnrepresentable
	}
	return publicPose(result)
}

// ComposePath composes subject-up edges from the identity pose.
func ComposePath(subjectUp []Pose) (Pose, error) {
	if len(subjectUp) > MaxExactProjectionEdges {
		return Pose{}, ErrUnrepresentable
	}
	p, err := composePathInternal(subjectUp)
	if err != nil {
		return Pose{}, err
	}
	return publicPose(p)
}

func composePathInternal(subjectUp []Pose) (ExactPose, error) {
	a := ExactFromPose(IdentityPose())
	for _, edge := range subjectUp {
		if !ValidPose(edge) {
			return ExactPose{}, ErrInvalid
		}
		w := &Work{}
		a = w.Compose(ExactFromPose(edge), a)
		if w.Failed() || !PoseWithinWorkLimit(a) {
			return ExactPose{}, ErrUnrepresentable
		}
	}
	return a, nil
}

// PoseWithinWorkLimit reports whether every exact rational primitive is within
// the configured numerator/denominator bit profile.
func PoseWithinWorkLimit(p ExactPose) bool {
	return rationalWithinWorkLimit(p.t.x) && rationalWithinWorkLimit(p.t.y) && rationalWithinWorkLimit(p.t.z) && rationalWithinWorkLimit(p.q.w) && rationalWithinWorkLimit(p.q.x) && rationalWithinWorkLimit(p.q.y) && rationalWithinWorkLimit(p.q.z)
}

// Contains reports exact sphere containment at a public pose.
func Contains(parent BoundedSphere, pose Pose, child BoundedSphere) (bool, error) {
	if !ValidPose(pose) || !ValidSphere(parent) || !ValidSphere(child) {
		return false, ErrInvalid
	}
	rp, rc := exactFloat(parent.radius), exactFloat(child.radius)
	if rc.Cmp(rp) > 0 {
		return false, nil
	}
	d := newRat().Sub(rp, rc)
	v := pose.Translation
	ds := addRats(square(exactFloat(v.X)), square(exactFloat(v.Y)), square(exactFloat(v.Z)))
	return ds.Cmp(square(d)) <= 0, nil
}

// spheresIntersect reports exact closed-sphere intersection.
func spheresIntersect(a Vec3, ar BoundedSphere, b Vec3, br BoundedSphere) (bool, error) {
	if !Finite(a.X) || !Finite(a.Y) || !Finite(a.Z) || !Finite(b.X) || !Finite(b.Y) || !Finite(b.Z) || !ValidSphere(ar) || !ValidSphere(br) {
		return false, ErrInvalid
	}
	dx := newRat().Sub(exactFloat(a.X), exactFloat(b.X))
	dy := newRat().Sub(exactFloat(a.Y), exactFloat(b.Y))
	dz := newRat().Sub(exactFloat(a.Z), exactFloat(b.Z))
	r := newRat().Add(exactFloat(ar.radius), exactFloat(br.radius))
	return addRats(square(dx), square(dy), square(dz)).Cmp(square(r)) <= 0, nil
}

func rationalWithinWorkLimit(v *big.Rat) bool {
	return v != nil && v.Num().BitLen() <= MaxExactRationalBits && v.Denom().BitLen() <= MaxExactRationalBits
}

// Work enforces the exact-arithmetic profile after every reduced rational
// primitive, not merely after a composed result in which oversized
// intermediates may cancel.
type Work struct{ failed bool }

// Failed reports whether any primitive exceeded the exact work profile.
func (w *Work) Failed() bool { return w.failed }

func (w *Work) keep(v *big.Rat) *big.Rat {
	if !rationalWithinWorkLimit(v) {
		w.failed = true
	}
	return v
}

func (w *Work) add(a, b *big.Rat) *big.Rat { return w.keep(newRat().Add(a, b)) }
func (w *Work) sub(a, b *big.Rat) *big.Rat { return w.keep(newRat().Sub(a, b)) }
func (w *Work) mul(a, b *big.Rat) *big.Rat { return w.keep(newRat().Mul(a, b)) }
func (w *Work) quo(a, b *big.Rat) *big.Rat { return w.keep(newRat().Quo(a, b)) }

func (w *Work) hamilton(a, b exactQuaternion) exactQuaternion {
	return exactQuaternion{
		w.sub(w.sub(w.sub(w.mul(a.w, b.w), w.mul(a.x, b.x)), w.mul(a.y, b.y)), w.mul(a.z, b.z)),
		w.sub(w.add(w.add(w.mul(a.w, b.x), w.mul(a.x, b.w)), w.mul(a.y, b.z)), w.mul(a.z, b.y)),
		w.add(w.add(w.sub(w.mul(a.w, b.y), w.mul(a.x, b.z)), w.mul(a.y, b.w)), w.mul(a.z, b.x)),
		w.add(w.sub(w.add(w.mul(a.w, b.z), w.mul(a.x, b.y)), w.mul(a.y, b.x)), w.mul(a.z, b.w)),
	}
}

func (w *Work) rotate(q exactQuaternion, v exactVec3) exactVec3 {
	z := newRat()
	p := w.hamilton(w.hamilton(q, exactQuaternion{z, v.x, v.y, v.z}), conjugate(q))
	n := w.add(w.add(w.add(w.mul(q.w, q.w), w.mul(q.x, q.x)), w.mul(q.y, q.y)), w.mul(q.z, q.z))
	return exactVec3{w.quo(p.x, n), w.quo(p.y, n), w.quo(p.z, n)}
}

// Compose applies the exact outer-then-inner composition used by projection.
func (w *Work) Compose(o, i ExactPose) ExactPose {
	r := w.rotate(o.q, i.t)
	return ExactPose{exactVec3{w.add(r.x, o.t.x), w.add(r.y, o.t.y), w.add(r.z, o.t.z)}, w.hamilton(o.q, i.q)}
}

// Inverse returns the exact inverse of a pose.
func (w *Work) Inverse(p ExactPose) ExactPose {
	q := conjugate(p.q)
	t := w.rotate(q, p.t)
	return ExactPose{exactVec3{w.keep(newRat().Neg(t.x)), w.keep(newRat().Neg(t.y)), w.keep(newRat().Neg(t.z))}, q}
}
