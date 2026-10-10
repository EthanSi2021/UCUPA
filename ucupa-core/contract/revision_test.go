package contract

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestRevisionContractRetainsOnlyOriginalMethodsAndFields(t *testing.T) {
	for _, tc := range []struct {
		typ     reflect.Type
		methods []string
	}{
		{reflect.TypeOf(Revision{}), []string{"Equal", "ServiceEvidence"}},
		{reflect.TypeOf(MountRevision{}), []string{"Equal", "ServiceEvidence"}},
		{reflect.TypeOf(FacetContactRevision{}), []string{"Equal"}},
		{reflect.TypeOf(ListContinuation{}), nil},
	} {
		if !tc.typ.Comparable() || tc.typ.NumMethod() != len(tc.methods) {
			t.Fatalf("method or equality surface changed: %s", tc.typ)
		}
		for i, want := range tc.methods {
			if tc.typ.Method(i).Name != want {
				t.Fatalf("%s exposed %s", tc.typ, tc.typ.Method(i).Name)
			}
		}
		for i := 0; i < tc.typ.NumField(); i++ {
			if tc.typ.Field(i).IsExported() {
				t.Fatalf("%s exposes %s", tc.typ, tc.typ.Field(i).Name)
			}
		}
	}
	s := reflect.TypeOf(ServiceRevision{})
	want := []struct {
		name string
		typ  reflect.Type
	}{{"Incarnation", reflect.TypeOf([32]byte{})}, {"Element", reflect.TypeOf(values.ElementID(""))}, {"Sequence", reflect.TypeOf(uint64(0))}}
	if s.NumField() != len(want) || s.NumMethod() != 2 {
		t.Fatal("service evidence shape changed")
	}
	for i, field := range want {
		if s.Field(i).Name != field.name || s.Field(i).Type != field.typ || !s.Field(i).IsExported() {
			t.Fatal("service evidence field changed")
		}
	}
	if s.Method(0).Name != "ElementRevision" || s.Method(1).Name != "MountRevision" {
		t.Fatal("service evidence methods changed")
	}
}

func TestServiceRevisionKeepsTheSharedInputErrorIdentity(t *testing.T) {
	_, geometryError := NewBoundedSphere(-1)
	_, elementError := (ServiceRevision{}).ElementRevision()
	_, mountError := (ServiceRevision{}).MountRevision()
	for _, err := range []error{geometryError, elementError, mountError} {
		if err != ErrInvalid || !errors.Is(fmt.Errorf("context: %w", err), ErrInvalid) {
			t.Fatalf("split input error identity: %v", err)
		}
	}
	if ErrInvalid.Error() != "invalid Core input" || ErrUnrepresentable.Error() != "Core result is unrepresentable" {
		t.Fatal("error text changed")
	}
}
