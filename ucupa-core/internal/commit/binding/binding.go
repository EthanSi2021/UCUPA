// Package binding defines the retained Element operation input used by live
// admission, retry comparison and replay. It owns no facts or receipts.
package binding

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

// Binding is the one internal retained-input representation. A parser result
// is not admitted: the owner must bind caller, boundary and expected revision
// at the original authorization/commit point before validating it for execution.
type Binding struct {
	Boundary      values.ElementID
	PublicRequest string
	Actor         values.PrincipalRef
	Kind          elementfact.OperationKind
	Element       values.ElementID
	Final         contract.SpatialTuple
	Expected      contract.Revision
}

func ValidOperationID(id contract.OperationID) bool {
	length := len([]byte(id))
	return length >= 1 && length <= 128
}

func ValidFinalTuple(final contract.SpatialTuple) bool {
	return final.Parent.Valid() && geometry.ValidPose(final.LocalPose) && geometry.ValidSphere(final.Range)
}

func ValidBinding(binding Binding) bool {
	if (binding.Boundary == "") != (binding.PublicRequest == "") || binding.Boundary != "" && !binding.Boundary.Valid() {
		return false
	}
	if !binding.Actor.Valid() || !binding.Element.Valid() {
		return false
	}
	switch binding.Kind {
	case elementfact.OperationCreateRoot:
		return binding.Final == (contract.SpatialTuple{}) && binding.Expected == (contract.Revision{})
	case elementfact.OperationCreateAnchor, elementfact.OperationCreateEntity:
		return ValidFinalTuple(binding.Final) && binding.Expected == (contract.Revision{})
	case elementfact.OperationUpdateAnchor, elementfact.OperationUpdateEntity:
		return ValidFinalTuple(binding.Final) && token.ValidRevision(binding.Expected)
	case elementfact.OperationDeleteAnchor, elementfact.OperationDeleteEntity:
		return binding.Final == (contract.SpatialTuple{}) && token.ValidRevision(binding.Expected)
	default:
		return false
	}
}

func CanonicalFinalTuple(final contract.SpatialTuple) (contract.SpatialTuple, bool) {
	if !final.Parent.Valid() || !geometry.ValidSphere(final.Range) {
		return contract.SpatialTuple{}, false
	}
	pose, err := contract.NewPose(final.LocalPose.Translation, final.LocalPose.Rotation)
	if err != nil {
		return contract.SpatialTuple{}, false
	}
	return contract.SpatialTuple{Parent: final.Parent, LocalPose: pose, Range: final.Range}, true
}

func EqualBindings(left, right Binding) bool {
	if left.Boundary != right.Boundary || left.PublicRequest != right.PublicRequest {
		return false
	}
	if left.Actor != right.Actor || left.Kind != right.Kind || left.Element != right.Element ||
		left.Expected != right.Expected || left.Final.Parent != right.Final.Parent {
		return false
	}
	if left.Kind == elementfact.OperationCreateRoot ||
		left.Kind == elementfact.OperationDeleteAnchor || left.Kind == elementfact.OperationDeleteEntity {
		return true
	}
	return geometry.EqualPose(left.Final.LocalPose, right.Final.LocalPose) &&
		geometry.EqualSphere(left.Final.Range, right.Final.Range)
}

func CanonicalBindingForStore(binding Binding) bool {
	if !ValidBinding(binding) {
		return false
	}
	switch binding.Kind {
	case elementfact.OperationCreateAnchor, elementfact.OperationCreateEntity, elementfact.OperationUpdateAnchor, elementfact.OperationUpdateEntity:
		return geometry.CanonicalPose(binding.Final.LocalPose) && geometry.ValidSphere(binding.Final.Range)
	default:
		return true
	}
}
