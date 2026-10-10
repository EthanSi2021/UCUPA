package denial

import (
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"
)

func TestEvidenceIsOpaqueAndHasNoBehavior(t *testing.T) {
	typ := reflect.TypeOf(Evidence{})
	if typ.Kind() != reflect.Struct {
		t.Fatalf("evidence kind: got %v, want struct", typ.Kind())
	}
	for index := 0; index < typ.NumField(); index++ {
		if field := typ.Field(index); field.IsExported() || field.Anonymous {
			t.Fatalf("evidence field %d is externally constructible: %#v", index, field)
		}
	}
	if typ.NumMethod() != 0 || reflect.PointerTo(typ).NumMethod() != 0 {
		t.Fatal("evidence gained public methods")
	}
}

func TestEvidenceBindingRejectsZeroAndEveryWrongBinding(t *testing.T) {
	owner := values.FactAuthorityRef("owner")
	source := values.FactAuthorityRef("source")
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "principal"}, Establishment: [32]byte{1}}
	request := federationvalue.FederationReadRequest{Boundary: values.ElementID("boundary"), Intent: "query", Purpose: [32]byte{2}}
	requirement := values.Requirement{Permission: values.PermissionID("query")}
	view := values.AuthorizationDenialView{3}
	incoming := mountfact.Incoming{Detached: true}
	expires := time.Unix(100, 0)
	evidence := New(owner, source, caller, request, requirement, view, incoming, expires)

	if MatchesBinding(Evidence{}, owner, source, caller, request) {
		t.Fatal("zero evidence matched a binding")
	}
	if !MatchesBinding(evidence, owner, source, caller, request) {
		t.Fatal("captured evidence lost its exact binding")
	}
	tests := []struct {
		name    string
		owner   values.FactAuthorityRef
		source  values.FactAuthorityRef
		caller  values.CallerContext
		request federationvalue.FederationReadRequest
	}{
		{"owner", "other", source, caller, request},
		{"source", owner, "other", caller, request},
		{"caller", owner, source, values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "other"}}, request},
		{"request", owner, source, caller, federationvalue.FederationReadRequest{Boundary: request.Boundary, Intent: request.Intent, Purpose: [32]byte{4}}},
	}
	for _, test := range tests {
		if MatchesBinding(evidence, test.owner, test.source, test.caller, test.request) {
			t.Fatalf("%s mismatch borrowed denial evidence", test.name)
		}
	}

	gotView, gotRequirement := Authorization(evidence)
	if gotView != view || gotRequirement != requirement {
		t.Fatalf("authorization extraction: got %#v %#v", gotView, gotRequirement)
	}
	if !MatchesIncoming(evidence, incoming) {
		t.Fatal("captured incoming relation did not match")
	}
	if got := Incoming(evidence); got != incoming {
		t.Fatalf("incoming extraction: got %#v, want %#v", got, incoming)
	}
	changedIncoming := incoming
	changedIncoming.Gate = mountfact.GateOpen
	if MatchesIncoming(evidence, changedIncoming) {
		t.Fatal("changed incoming relation retained denial")
	}
	if Expired(evidence, expires.Add(-time.Nanosecond)) {
		t.Fatal("denial expired before its absolute deadline")
	}
	if !Expired(evidence, expires) || !Expired(evidence, expires.Add(time.Nanosecond)) {
		t.Fatal("denial survived its absolute deadline")
	}
}
