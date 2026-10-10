package binding

import (
	"errors"
	"math"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

const (
	publicTestElement values.ElementID        = "00000000-0000-4000-8000-000000000003"
	publicTestParent  values.ElementID        = "00000000-0000-4000-8000-000000000001"
	publicTestOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000004"
)

func publicTestOperationID() (contract.OperationID, string) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}
	return contract.OperationID(string(raw)), values.EncodePublicBytes(raw)
}

func publicTestRequest(kind string) map[string]any {
	_, encoded := publicTestOperationID()
	return map[string]any{
		"op":          kind,
		"element":     string(publicTestElement),
		"operationId": encoded,
	}
}

func publicTestFinal(translation []any) map[string]any {
	return map[string]any{
		"parent": string(publicTestParent),
		"pose": map[string]any{
			"translation": translation,
			"rotation": map[string]any{
				"pivot":  float64(1),
				"ratios": []any{float64(0), float64(0), float64(0)},
			},
		},
		"range": map[string]any{
			"kind":   "bounded-sphere",
			"radius": float64(2),
		},
	}
}

func publicTestExpected() map[string]any {
	raw := make([]byte, 32)
	raw[0] = 9
	return map[string]any{
		"owner": string(publicTestOwner),
		"kind":  "record-revision",
		"value": values.EncodePublicBytes(raw),
	}
}

func TestParsePublicMutationKeepsCanonicalRequestAndUnresolvedFields(t *testing.T) {
	request := publicTestRequest("create-anchor")
	request["final"] = publicTestFinal([]any{float64(0), float64(1), float64(2)})

	binding, gotID, gotExpected, err := ParsePublicMutation(request)
	if err != nil {
		t.Fatal(err)
	}
	wantID, _ := publicTestOperationID()
	if gotID != wantID {
		t.Fatalf("operation ID = %q, want decoded raw bytes", gotID)
	}
	canonical, err := values.CanonicalJSONValue(request)
	if err != nil {
		t.Fatal(err)
	}
	if binding.PublicRequest != string(canonical) {
		t.Fatalf("PublicRequest = %q, want canonical %q", binding.PublicRequest, canonical)
	}
	if binding.Kind != elementfact.OperationCreateAnchor || binding.Element != publicTestElement {
		t.Fatalf("unexpected parsed binding: %#v", binding)
	}
	if binding.Actor != (values.PrincipalRef{}) || binding.Boundary != "" || binding.Expected != (contract.Revision{}) {
		t.Fatalf("parser assigned admission state: %#v", binding)
	}
	if gotExpected != (values.PublicToken{}) {
		t.Fatalf("create parser returned unexpected expected token: %#v", gotExpected)
	}
}

func TestParsePublicMutationLeavesExpectedTokenForAdmission(t *testing.T) {
	request := publicTestRequest("update-anchor")
	request["final"] = publicTestFinal([]any{float64(0), float64(0), float64(0)})
	request["expected"] = publicTestExpected()

	binding, _, gotExpected, err := ParsePublicMutation(request)
	if err != nil {
		t.Fatal(err)
	}
	if gotExpected.Owner != publicTestOwner || gotExpected.Kind != "record-revision" || gotExpected.Value == "" {
		t.Fatalf("unexpected parsed public token: %#v", gotExpected)
	}
	if binding.Expected != (contract.Revision{}) || binding.Actor != (values.PrincipalRef{}) || binding.Boundary != "" {
		t.Fatalf("parser resolved admission state: %#v", binding)
	}
}

func TestParsePublicMutationShapeAndKindRules(t *testing.T) {
	createWithExpected := publicTestRequest("create-anchor")
	createWithExpected["final"] = publicTestFinal([]any{float64(0), float64(0), float64(0)})
	createWithExpected["expected"] = publicTestExpected()

	deleteWithFinal := publicTestRequest("delete-anchor")
	deleteWithFinal["final"] = publicTestFinal([]any{float64(0), float64(0), float64(0)})
	deleteWithFinal["expected"] = publicTestExpected()

	deleteWithoutExpected := publicTestRequest("delete-anchor")

	for _, test := range []struct {
		name    string
		request map[string]any
	}{
		{name: "create-root is not public", request: publicTestRequest("create-root")},
		{name: "create rejects expected", request: createWithExpected},
		{name: "delete rejects final", request: deleteWithFinal},
		{name: "delete requires expected", request: deleteWithoutExpected},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := ParsePublicMutation(test.request)
			if err != values.PublicInvalid {
				t.Fatalf("ParsePublicMutation error = %v, want PublicInvalid", err)
			}
		})
	}
}

func TestParsePublicMutationRejectsMalformedPublicFields(t *testing.T) {
	unknown := publicTestRequest("create-anchor")
	unknown["final"] = publicTestFinal([]any{float64(0), float64(0), float64(0)})
	unknown["unknown"] = true

	nilField := publicTestRequest("create-anchor")
	nilField["final"] = nil

	badOperationID := publicTestRequest("create-anchor")
	badOperationID["operationId"] = "!"

	badPose := publicTestRequest("create-anchor")
	badPose["final"] = publicTestFinal([]any{float64(0), float64(0)})

	missingOp := publicTestRequest("create-anchor")
	delete(missingOp, "op")
	missingElement := publicTestRequest("create-anchor")
	delete(missingElement, "element")
	missingOperationID := publicTestRequest("create-anchor")
	delete(missingOperationID, "operationId")

	for _, test := range []struct {
		name    string
		request map[string]any
	}{
		{name: "unknown field", request: unknown},
		{name: "nil field", request: nilField},
		{name: "operation id", request: badOperationID},
		{name: "pose vector", request: badPose},
		{name: "missing op", request: missingOp},
		{name: "missing element", request: missingElement},
		{name: "missing operation id", request: missingOperationID},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := ParsePublicMutation(test.request)
			if err != values.PublicInvalid {
				t.Fatalf("ParsePublicMutation error = %v, want PublicInvalid", err)
			}
		})
	}
}

func TestParsePublicMutationPreservesInvalidSentinelForNonCanonicalPose(t *testing.T) {
	request := publicTestRequest("create-anchor")
	request["final"] = publicTestFinal([]any{math.Copysign(0, -1), float64(0), float64(0)})

	_, _, _, err := ParsePublicMutation(request)
	if !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("non-canonical pose error = %v, want ErrInvalid", err)
	}
	if err == values.PublicInvalid {
		t.Fatal("non-canonical pose was collapsed into PublicInvalid")
	}
}
