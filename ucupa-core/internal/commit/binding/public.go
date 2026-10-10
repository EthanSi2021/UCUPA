// Public mutation parsing validates only input shape; caller, scope membership and expected-token provenance remain owner decisions.
package binding

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

func ParsePublicMutation(request map[string]any) (Binding, contract.OperationID, values.PublicToken, error) {
	if _, err := accessvalue.Object(request, "op", "element", "final", "expected", "operationId"); err != nil {
		return Binding{}, "", values.PublicToken{}, err
	}
	op, a := request["op"].(string)
	element, b := request["element"].(string)
	idText, c := request["operationId"].(string)
	idBytes, err := values.DecodePublicBytes(idText)
	kind := KindFromName(op)
	if !a || !b || !c || kind < elementfact.OperationCreateAnchor || kind > elementfact.OperationDeleteEntity || !values.ElementID(element).Valid() || err != nil || !ValidOperationID(contract.OperationID(idBytes)) {
		return Binding{}, "", values.PublicToken{}, values.PublicInvalid
	}
	binding := Binding{Kind: kind, Element: values.ElementID(element)}
	if kind == elementfact.OperationCreateAnchor || kind == elementfact.OperationCreateEntity || kind == elementfact.OperationUpdateAnchor || kind == elementfact.OperationUpdateEntity {
		final, err := accessvalue.Object(request["final"], "parent", "pose", "range")
		if err != nil {
			return Binding{}, "", values.PublicToken{}, err
		}
		parent, ok := final["parent"].(string)
		if !ok || !values.ElementID(parent).Valid() {
			return Binding{}, "", values.PublicToken{}, values.PublicInvalid
		}
		pose, err := accessvalue.ParsePose(final["pose"])
		if err != nil {
			return Binding{}, "", values.PublicToken{}, err
		}
		rangeValue, err := accessvalue.Object(final["range"], "kind", "radius")
		if err != nil || rangeValue["kind"] != "bounded-sphere" {
			return Binding{}, "", values.PublicToken{}, values.PublicInvalid
		}
		radius, ok := rangeValue["radius"].(float64)
		if !ok {
			return Binding{}, "", values.PublicToken{}, values.PublicInvalid
		}
		sphere, err := contract.NewBoundedSphere(radius)
		if err != nil {
			return Binding{}, "", values.PublicToken{}, values.PublicInvalid
		}
		binding.Final = contract.SpatialTuple{Parent: values.ElementID(parent), LocalPose: pose, Range: sphere}
	} else if request["final"] != nil {
		return Binding{}, "", values.PublicToken{}, values.PublicInvalid
	}
	expected := values.PublicToken{}
	if kind >= elementfact.OperationUpdateAnchor {
		expected, err = accessvalue.Token(request["expected"], "record-revision")
		if err != nil {
			return Binding{}, "", values.PublicToken{}, err
		}
	} else if request["expected"] != nil {
		return Binding{}, "", values.PublicToken{}, values.PublicInvalid
	}
	canonical, _ := values.CanonicalJSONValue(request)
	binding.PublicRequest = string(canonical)
	return binding, contract.OperationID(idBytes), expected, nil
}
