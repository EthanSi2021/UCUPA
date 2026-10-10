package scan

import (
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// TransferGap checks pending source envelopes against this query's actual
// scope and sphere. It never returns inactive members as matches, expands scope
// or follows an unverified successor. A projection limit cannot prove exclusion.
func TransferGap(elements elementfact.View, transfers transferfact.Visibility, query contract.RegionQuery) (bool, error) {
	if transfers.Inactive(query.StartAnchorID) || transfers.Inactive(query.BasisAnchorID) {
		return false, contract.ErrUnavailable
	}
	_, count, _ := transfers.Counts()
	if count == 0 {
		return false, nil
	}
	memo := NewProjectionMemo(elements, query.StartAnchorID, nil)
	basis, err := memo.Anchor(query.BasisAnchorID)
	if err != nil {
		return false, queryGeometryError(err)
	}
	work := &geometry.Work{}
	inverse := work.Inverse(basis.Pose)
	if work.Failed() || !geometry.PoseWithinWorkLimit(inverse) {
		return false, contract.ErrUnrepresentable
	}
	return TransferGapProjected(elements, transfers, query, inverse)
}

// TransferGapProjected uses the already validated scope-to-query-basis chain
// supplied by a federation projection. Both local and remote discovery share
// exactly the same exclusion rule; a timestamp alone grants no source effect.
func TransferGapProjected(elements elementfact.View, transfers transferfact.Visibility, query contract.RegionQuery, toBasis geometry.ExactPose) (bool, error) {
	if transfers.Inactive(query.StartAnchorID) {
		return false, contract.ErrUnavailable
	}
	memo := NewProjectionMemo(elements, query.StartAnchorID, nil)
	queryRange, err := contract.NewBoundedSphere(query.Sphere.Radius)
	if err != nil {
		return false, err
	}
	for fence := range transfers.Fences() {
		if !elements.DescendsFrom(fence.Origin.Parent, query.StartAnchorID) {
			continue
		}
		origin := fence.Origin
		projection, err := memo.Entity(elementfact.Entity{ID: fence.Boundary, Parent: origin.Parent, LocalPose: origin.LocalPose, Range: origin.Range})
		if err != nil {
			return true, nil
		}
		work := &geometry.Work{}
		relative := work.Compose(toBasis, projection.Pose)
		if work.Failed() || !geometry.PoseWithinWorkLimit(relative) {
			return true, nil
		}
		hit, err := geometry.CenterIntersects(relative, origin.Range, query.Sphere.Center, queryRange)
		if err != nil || hit {
			return true, nil
		}
	}
	return false, nil
}
