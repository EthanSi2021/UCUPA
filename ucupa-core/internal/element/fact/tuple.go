package fact

import (
	"errors"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
)

func TupleContainedBy(parent Anchor, child contract.SpatialTuple) (bool, error) {
	if parent.Root() {
		return true, nil
	}
	parentSphere, ok := parent.Range.BoundedSphere()
	if !ok {
		return false, contract.ErrStore
	}
	inside, err := geometry.Contains(parentSphere, child.LocalPose, child.Range)
	if err != nil {
		if errors.Is(err, contract.ErrInvalid) {
			return false, contract.ErrStore
		}
		return false, err
	}
	return inside, nil
}

func TupleContainedByRange(parent contract.BoundedSphere, child contract.SpatialTuple) (bool, error) {
	inside, err := geometry.Contains(parent, child.LocalPose, child.Range)
	if errors.Is(err, contract.ErrInvalid) {
		return false, contract.ErrStore
	}
	return inside, err
}

func AnchorTupleEqual(anchor Anchor, final contract.SpatialTuple) bool {
	sphere, ok := anchor.Range.BoundedSphere()
	return ok && anchor.Parent == final.Parent &&
		geometry.EqualPose(anchor.LocalPose, final.LocalPose) && geometry.EqualSphere(sphere, final.Range)
}

func EntityTupleEqual(entity Entity, final contract.SpatialTuple) bool {
	return entity.Parent == final.Parent &&
		geometry.EqualPose(entity.LocalPose, final.LocalPose) && geometry.EqualSphere(entity.Range, final.Range)
}
