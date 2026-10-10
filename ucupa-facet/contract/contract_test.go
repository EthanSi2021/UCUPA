package contract_test

import (
	"reflect"
	"testing"
	"ucupa/ucupa-facet/contract"
)

func TestOpaquePublicValueMethodSurface(t *testing.T) {
	for _, value := range []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "revision", typeOf: reflect.TypeOf(contract.Revision{})},
		{name: "contact revision", typeOf: reflect.TypeOf(contract.ContactRevision{})},
	} {
		if value.typeOf.NumMethod() != 1 || value.typeOf.Method(0).Name != "Equal" {
			t.Fatalf("%s public methods: %v", value.name, value.typeOf)
		}
	}
}

func TestPublicPreconditionConstructors(t *testing.T) {
	var revision contract.Revision
	_ = contract.ExpectAbsent()
	_ = contract.ExpectRevision(revision)
	if !revision.Equal(revision) {
		t.Fatal("revision equality")
	}
	var contact contract.ContactRevision
	if !contact.Equal(contact) {
		t.Fatal("contact revision equality")
	}
}
