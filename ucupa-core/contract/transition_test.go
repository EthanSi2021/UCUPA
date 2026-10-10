package contract

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestTransitionPublicValuesDoNotExposePrivateEvidence(t *testing.T) {
	assertPublicStructFields(t, "TransitionApprovalBinding", TransitionApprovalBinding{},
		publicFieldSpec{"Source", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Operation", reflect.TypeOf(OperationID(""))},
		publicFieldSpec{"Command", reflect.TypeOf(TransitionCommand{})},
	)
	assertPublicStructFields(t, "TransitionCommand", TransitionCommand{},
		publicFieldSpec{"Kind", reflect.TypeOf(TransitionKind(0))},
		publicFieldSpec{"Target", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Boundary", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"Expected", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"Final", reflect.TypeOf(SpatialTuple{})},
		publicFieldSpec{"SourceAddress", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"TargetAddress", reflect.TypeOf(values.NextHop(""))},
	)
	assertPublicStructFields(t, "TransitionStatus", TransitionStatus{},
		publicFieldSpec{"Source", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Operation", reflect.TypeOf(OperationID(""))},
		publicFieldSpec{"Kind", reflect.TypeOf(TransitionKind(0))},
		publicFieldSpec{"Target", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Boundary", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"Progress", reflect.TypeOf(TransitionProgress(0))},
		publicFieldSpec{"Revision", reflect.TypeOf(values.PublicToken{})},
	)
	assertNoExportedMethods(t, "TransitionCommand", TransitionCommand{})
	assertNoExportedMethods(t, "TransitionStatus", TransitionStatus{})
}

func TestCarrierPublicValuesKeepParentDistinctAndEvidencePrivate(t *testing.T) {
	assertPublicStructFields(t, "CarrierCommand", CarrierCommand{},
		publicFieldSpec{"Source", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Target", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Mount", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"ExpectedElement", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"ExpectedMount", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"ParentAddress", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"SourceAddress", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"TargetAddress", reflect.TypeOf(values.NextHop(""))},
	)
	assertPublicStructFields(t, "CarrierStatus", CarrierStatus{},
		publicFieldSpec{"Parent", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Source", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Target", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Operation", reflect.TypeOf(OperationID(""))},
		publicFieldSpec{"Mount", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"Progress", reflect.TypeOf(CarrierProgress(0))},
		publicFieldSpec{"Revision", reflect.TypeOf(values.PublicToken{})},
	)
	assertNoExportedMethods(t, "CarrierCommand", CarrierCommand{})
	assertNoExportedMethods(t, "CarrierStatus", CarrierStatus{})
}
