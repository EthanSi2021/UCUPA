package record

import (
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

func LoadElement(elements *elementfact.State, scope [32]byte, id values.ElementID, stored StoredElement) error {
	if stored.RevisionSerial == 0 {
		return errors.New("Element revision is zero")
	}
	if stored.Kind != "departure" && (len(stored.Departure) != 0 || stored.DepartureController != "" || stored.RootDeparture) {
		return errors.New("active or deleted Element carries departure")
	}
	revision := token.RevisionFromParts(scope, id, stored.RevisionSerial)
	switch stored.Kind {
	case "anchor":
		if stored.RangeKind == "unbounded-root" {
			if stored.Parent != "" || stored.Pose != nil || stored.RadiusBits != "" {
				return errors.New("root Anchor record shape is invalid")
			}
			return elements.RestoreAnchor(elementfact.Anchor{ID: id, Range: geometry.UnboundedRootRange(), Revision: revision})
		} else {
			parent, pose, sphere, err := stored.childValues()
			if err != nil {
				return err
			}
			return elements.RestoreAnchor(elementfact.Anchor{ID: id, Parent: parent, LocalPose: pose, Range: geometry.BoundedRange(sphere), Revision: revision})
		}
	case "entity":
		parent, pose, sphere, err := stored.childValues()
		if err != nil {
			return err
		}
		return elements.RestoreEntity(elementfact.Entity{ID: id, Parent: parent, LocalPose: pose, Range: sphere, Revision: revision})
	case "tombstone":
		if stored.Parent != "" || stored.Pose != nil || stored.RangeKind != "" || stored.RadiusBits != "" {
			return errors.New("tombstone record shape is invalid")
		}
		return elements.RestoreTombstone(id, revision)
	case "departure":
		if stored.Parent != "" || stored.Pose != nil || stored.RangeKind != "" || stored.RadiusBits != "" || !stored.DepartureController.Valid() || len(stored.Departure) == 0 || len(stored.Departure) > 128 {
			return errors.New("departure record shape is invalid")
		}
		return elements.RestoreDeparture(id, elementfact.Departure{Controller: stored.DepartureController, Revision: revision, Operation: contract.OperationID(string(stored.Departure)), Root: stored.RootDeparture})
	default:
		return errors.New("Element record kind is unrecognized")
	}
}
