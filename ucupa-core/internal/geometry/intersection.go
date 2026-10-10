package geometry

// CenterIntersects compares an exact element-center pose against a public
// query sphere. Invalid public values or an unusable exact pose return
// ErrInvalid; oversized exact primitives retain the existing
// ErrUnrepresentable result.
func CenterIntersects(p ExactPose, elementRange BoundedSphere, queryCenter Vec3, queryRange BoundedSphere) (bool, error) {
	if !validExactPose(p) || !ValidSphere(elementRange) || !ValidSphere(queryRange) ||
		!Finite(queryCenter.X) || !Finite(queryCenter.Y) || !Finite(queryCenter.Z) {
		return false, ErrInvalid
	}
	work := &Work{}
	dx := work.sub(p.t.x, exactFloat(queryCenter.X))
	dy := work.sub(p.t.y, exactFloat(queryCenter.Y))
	dz := work.sub(p.t.z, exactFloat(queryCenter.Z))
	distanceSquared := work.add(
		work.add(work.mul(dx, dx), work.mul(dy, dy)),
		work.mul(dz, dz),
	)
	radius := work.add(exactFloat(elementRange.radius), exactFloat(queryRange.radius))
	radiusSquared := work.mul(radius, radius)
	if work.Failed() {
		return false, ErrUnrepresentable
	}
	return distanceSquared.Cmp(radiusSquared) <= 0, nil
}

func validExactPose(p ExactPose) bool {
	return p.t.x != nil && p.t.y != nil && p.t.z != nil &&
		p.q.w != nil && p.q.x != nil && p.q.y != nil && p.q.z != nil
}
