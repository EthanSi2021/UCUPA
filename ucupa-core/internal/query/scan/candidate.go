package scan

import (
	"errors"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
)

func (projections *Memo) Candidate(
	id values.ElementID,
	query contract.RegionQuery,
	queryRange contract.BoundedSphere,
	basisInverse geometry.ExactPose,
) (contract.QueryMatch, bool, error) {
	match := contract.QueryMatch{ElementID: id}
	var radius contract.BoundedSphere
	var projection Projection
	var err error
	if anchor, exists := projections.elements.Anchor(id); exists {
		match.Kind, match.Revision = contract.ElementAnchor, anchor.Revision
		if anchor.Root() {
			return match, true, nil
		}
		var bounded bool
		radius, bounded = anchor.Range.BoundedSphere()
		if !bounded {
			return contract.QueryMatch{}, false, contract.ErrStore
		}
		projection, err = projections.Anchor(id)
	} else if entity, exists := projections.elements.Entity(id); exists {
		match.Kind, match.Revision = contract.ElementEntity, entity.Revision
		radius = entity.Range
		projection, err = projections.Entity(entity)
	} else {
		return contract.QueryMatch{}, false, contract.ErrStore
	}
	if err != nil {
		return contract.QueryMatch{}, false, err
	}
	work := &geometry.Work{}
	relative := work.Compose(basisInverse, projection.Pose)
	if work.Failed() || !geometry.PoseWithinWorkLimit(relative) {
		return contract.QueryMatch{}, false, contract.ErrUnrepresentable
	}
	intersects, err := geometry.CenterIntersects(relative, radius, query.Sphere.Center, queryRange)
	if errors.Is(err, contract.ErrInvalid) {
		return match, false, contract.ErrStore
	}
	return match, intersects, err
}

func Path(elements elementfact.View, id, scope values.ElementID) ([]values.ElementID, error) {
	path := make([]values.ElementID, 0)
	for id != scope {
		if len(path) > elements.Counts().Anchors {
			return nil, contract.ErrStore
		}
		path = append(path, id)
		if anchor, exists := elements.Anchor(id); exists {
			id = anchor.Parent
		} else if entity, exists := elements.Entity(id); exists {
			id = entity.Parent
		} else {
			return nil, contract.ErrStore
		}
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path, nil
}
