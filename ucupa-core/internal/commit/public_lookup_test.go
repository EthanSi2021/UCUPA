package commit

import (
	"errors"
	"reflect"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
)

func TestPublicRetainedLookupKeepsOriginalDisclosureAndCut(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	state := newCoreState(authority, [32]byte{1})
	state.cutSerial = 7
	cut := token.FromParts(state.scope, 3)
	binding := commitbinding.Binding{Actor: values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}, Boundary: values.ElementID("10000000-0000-4000-8000-000000000001"), PublicRequest: "original-canonical-request"}
	state.operations["original"] = retainedOperation{binding: binding, outcome: rejectedOutcome(elementfact.RevisionConflict, cut)}
	before := state.clone()
	other := binding
	other.Actor.Principal = "another"
	changedBoundary := binding
	changedBoundary.Boundary = values.ElementID("10000000-0000-4000-8000-000000000002")
	changedRequest := binding
	changedRequest.PublicRequest = "different-request"
	for _, test := range []struct {
		name       string
		id         contract.OperationID
		binding    commitbinding.Binding
		recovering bool
		found      bool
		err        error
		outcome    bool
	}{
		{"missing", "absent", binding, true, false, nil, false},
		{"retry", "original", binding, false, true, contract.ErrRevisionConflict, true},
		{"recovery", "original", binding, true, true, contract.ErrRevisionConflict, true},
		{"foreign retry", "original", other, false, true, values.PublicConflict, false},
		{"foreign recovery", "original", other, true, true, values.PublicOutcomeUnknown, false},
		{"changed boundary", "original", changedBoundary, true, true, values.PublicConflict, false},
		{"changed request", "original", changedRequest, true, true, values.PublicConflict, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err, found := state.lookupPublicMutation(test.id, test.binding, test.recovering)
			if found != test.found || !errors.Is(err, test.err) {
				t.Fatalf("lookup: %+v, %v, %t", result, err, found)
			}
			if test.outcome {
				if result.Status != contract.MutationRejected || result.OutcomeCut != cut {
					t.Fatalf("lost original outcome/cut: %+v", result)
				}
			} else if !reflect.DeepEqual(result, contract.MutationResult{}) {
				t.Fatalf("disclosed unrelated outcome: %+v", result)
			}
			if !sameCompleteCoreState(before, state) {
				t.Fatal("lookup changed the aggregate")
			}
		})
	}
}
