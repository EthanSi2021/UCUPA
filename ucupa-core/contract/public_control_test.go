package contract

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

type publicFieldSpec struct {
	name string
	typ  reflect.Type
}

func assertPublicStructFields(t *testing.T, name string, value any, want ...publicFieldSpec) {
	t.Helper()
	typ := reflect.TypeOf(value)
	if typ.Kind() != reflect.Struct || typ.NumField() != len(want) {
		t.Fatalf("%s shape = %v, want %d fields", name, typ, len(want))
	}
	for index, expected := range want {
		field := typ.Field(index)
		if field.Name != expected.name || field.Type != expected.typ || !field.IsExported() {
			t.Fatalf("%s field %d = %s %v exported=%v, want %s %v exported", name, index, field.Name, field.Type, field.IsExported(), expected.name, expected.typ)
		}
	}
}

func assertNoExportedMethods(t *testing.T, name string, value any) {
	t.Helper()
	for _, typ := range []reflect.Type{reflect.TypeOf(value), reflect.PointerTo(reflect.TypeOf(value))} {
		if typ.NumMethod() != 0 {
			t.Fatalf("%s exported method count = %d, want zero", name, typ.NumMethod())
		}
	}
}

func TestPublicControlValueSurface(t *testing.T) {
	assertPublicStructFields(t, "PublicMountCommand", PublicMountCommand{},
		publicFieldSpec{"Kind", reflect.TypeOf(MountCommandKind(0))},
		publicFieldSpec{"Element", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"ExpectedElement", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"ExpectedMount", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"Carrier", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"CarrierNextHop", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"ParentNextHop", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"Final", reflect.TypeOf(SpatialTuple{})},
	)
	assertPublicStructFields(t, "PublicMountInspection", PublicMountInspection{},
		publicFieldSpec{"Element", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"ElementRevision", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"MountRevision", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"State", reflect.TypeOf(MountState(0))},
		publicFieldSpec{"Carrier", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"NextHop", reflect.TypeOf(values.NextHop(""))},
	)
	assertPublicStructFields(t, "PublicMountContact", PublicMountContact{},
		publicFieldSpec{"Element", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"Incoming", reflect.TypeOf(false)},
		publicFieldSpec{"Peer", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"NextHop", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"Revision", reflect.TypeOf(values.PublicToken{})},
	)
	assertPublicStructFields(t, "PublicFacetCommand", PublicFacetCommand{},
		publicFieldSpec{"Kind", reflect.TypeOf("")},
		publicFieldSpec{"Element", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"Expected", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"Target", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"NextHop", reflect.TypeOf(values.NextHop(""))},
	)
	assertPublicStructFields(t, "PublicFacetInspection", PublicFacetInspection{},
		publicFieldSpec{"Element", reflect.TypeOf(values.ElementID(""))},
		publicFieldSpec{"State", reflect.TypeOf(FacetAttachmentState(0))},
		publicFieldSpec{"Revision", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"Target", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"KeyCore", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"Proposal", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"PendingTarget", reflect.TypeOf(values.FactAuthorityRef(""))},
		publicFieldSpec{"PendingProposal", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"NextHop", reflect.TypeOf(values.NextHop(""))},
		publicFieldSpec{"ContactRevision", reflect.TypeOf(values.PublicToken{})},
	)
	assertPublicStructFields(t, "AttachmentControlRead", AttachmentControlRead{},
		publicFieldSpec{"Read", reflect.TypeOf(FacetAttachmentRead{})},
		publicFieldSpec{"Proposal", reflect.TypeOf(values.PublicToken{})},
		publicFieldSpec{"Preparation", reflect.TypeOf(values.PublicToken{})},
	)

	assertNoExportedMethods(t, "PublicMountCommand", PublicMountCommand{})
	assertNoExportedMethods(t, "PublicMountInspection", PublicMountInspection{})
	assertNoExportedMethods(t, "PublicMountContact", PublicMountContact{})
	assertNoExportedMethods(t, "PublicFacetCommand", PublicFacetCommand{})
	assertNoExportedMethods(t, "PublicFacetInspection", PublicFacetInspection{})
	assertNoExportedMethods(t, "AttachmentControlRead", AttachmentControlRead{})

	typ := reflect.TypeOf((*MountControlTransport)(nil)).Elem()
	wantMethods := map[string]bool{"CheckApproval": true, "Finish": true, "Prepare": true, "ReadDecision": true}
	if typ.NumMethod() != len(wantMethods) {
		t.Fatalf("MountControlTransport method count = %d, want %d", typ.NumMethod(), len(wantMethods))
	}
	for index := 0; index < typ.NumMethod(); index++ {
		if !wantMethods[typ.Method(index).Name] {
			t.Fatalf("unexpected MountControlTransport method %q", typ.Method(index).Name)
		}
	}
}

type mountControlTransportProbe struct{}

func (mountControlTransportProbe) CheckApproval(context.Context, values.NextHop, values.FactAuthorityRef, OperationID, MountCommand) (values.PrincipalRef, error) {
	return values.PrincipalRef{}, nil
}

func (mountControlTransportProbe) Prepare(context.Context, values.NextHop, values.FactAuthorityRef, MountPrepareRequest) (MountPrepared, error) {
	return MountPrepared{}, nil
}

func (mountControlTransportProbe) Finish(context.Context, values.NextHop, values.FactAuthorityRef, MountFinishRequest) (MountClosed, error) {
	return MountClosed{}, nil
}

func (mountControlTransportProbe) ReadDecision(context.Context, values.NextHop, values.FactAuthorityRef, MountAttemptKey) (MountDecisionEvidence, error) {
	return MountDecisionEvidence{}, nil
}

var _ MountControlTransport = mountControlTransportProbe{}

func TestCoreReadCutFromEvidenceKeepsOpaqueBytes(t *testing.T) {
	raw := make([]byte, 40)
	raw[0] = 0xa5
	raw[31] = 0x5a
	binary.BigEndian.PutUint64(raw[32:], 29)
	evidence, err := values.NewCoreCutEvidence(raw)
	if err != nil {
		t.Fatalf("construct evidence: %v", err)
	}
	cut, err := CoreReadCutFromEvidence(evidence)
	if err != nil {
		t.Fatalf("parse valid evidence: %v", err)
	}
	if cut.Equal(CoreReadCut{}) || !cut.Evidence().Equal(evidence) || !bytes.Equal(cut.Evidence().Bytes(), raw) {
		t.Fatal("CoreReadCutFromEvidence changed the exact evidence bytes")
	}
	// The arbitrary nonzero scope above is intentionally not tied to an owner;
	// this function parses shape only and does not authenticate its source.
}

func TestCoreReadCutFromEvidenceUsesErrInvalid(t *testing.T) {
	cut, err := CoreReadCutFromEvidence(values.CoreCutEvidence{})
	if !cut.Equal(CoreReadCut{}) {
		t.Fatal("invalid evidence returned a nonzero cut")
	}
	if err != ErrInvalid {
		t.Fatalf("invalid evidence error = %v, want the contract ErrInvalid sentinel", err)
	}
}
