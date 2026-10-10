package contract

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestElementValueDefinitionsRetainTheirExactShape(t *testing.T) {
	type field struct {
		name string
		typ  reflect.Type
	}
	for _, tc := range []struct {
		value  any
		fields []field
	}{
		{SpatialTuple{}, []field{{"Parent", reflect.TypeOf(values.ElementID(""))}, {"LocalPose", reflect.TypeOf(Pose{})}, {"Range", reflect.TypeOf(BoundedSphere{})}}},
		{AnchorRecord{}, []field{{"ID", reflect.TypeOf(values.ElementID(""))}, {"ParentAnchorID", reflect.TypeOf((*values.ElementID)(nil))}, {"LocalPose", reflect.TypeOf((*Pose)(nil))}, {"Range", reflect.TypeOf(Range{})}, {"Revision", reflect.TypeOf(Revision{})}}},
		{EntityRecord{}, []field{{"ID", reflect.TypeOf(values.ElementID(""))}, {"ContainingAnchorID", reflect.TypeOf(values.ElementID(""))}, {"LocalPose", reflect.TypeOf(Pose{})}, {"Range", reflect.TypeOf(BoundedSphere{})}, {"Revision", reflect.TypeOf(Revision{})}}},
		{ElementRecord{}, []field{{"Kind", reflect.TypeOf(ElementKind(0))}, {"Anchor", reflect.TypeOf((*AnchorRecord)(nil))}, {"Entity", reflect.TypeOf((*EntityRecord)(nil))}}},
		{MutationResult{}, []field{{"Status", reflect.TypeOf(MutationStatus(0))}, {"Record", reflect.TypeOf((*ElementRecord)(nil))}, {"TombstoneRevision", reflect.TypeOf(Revision{})}, {"OutcomeCut", reflect.TypeOf(CoreReadCut{})}}},
	} {
		typ := reflect.TypeOf(tc.value)
		if typ.NumField() != len(tc.fields) || typ.NumMethod() != 0 {
			t.Fatalf("%s gained fields or behavior", typ)
		}
		for i, want := range tc.fields {
			got := typ.Field(i)
			if got.Name != want.name || got.Type != want.typ || !got.IsExported() {
				t.Fatalf("%s field %d changed: %v", typ, i, got)
			}
		}
	}
	if ElementAnchor != 1 || ElementEntity != 2 || MutationCommitted != 1 || MutationUnchanged != 2 || MutationRejected != 3 {
		t.Fatal("stored enum values changed")
	}
	if reflect.TypeOf(OperationID("")).Kind() != reflect.String || reflect.TypeOf(OperationID("")).NumMethod() != 0 {
		t.Fatal("OperationID became a validation or issuance API")
	}
}

func TestExistingCoreErrorIdentityAndText(t *testing.T) {
	seen := map[error]bool{}
	for _, tc := range []struct {
		err  error
		text string
	}{
		{ErrDenied, "Core authorization denied"},
		{ErrNotFound, "Core Element not found"},
		{ErrConflict, "Core state conflict"},
		{ErrRevisionConflict, "Core revision conflict"},
		{ErrOperationConflict, "Core operation conflict"},
		{ErrReadCutExpired, "Core read cut expired"},
		{ErrStore, "Core Store failure"},
	} {
		if tc.err == nil || seen[tc.err] || tc.err.Error() != tc.text || !errors.Is(fmt.Errorf("original context: %w", tc.err), tc.err) {
			t.Fatalf("changed Core error %q: %v", tc.text, tc.err)
		}
		seen[tc.err] = true
	}
}
