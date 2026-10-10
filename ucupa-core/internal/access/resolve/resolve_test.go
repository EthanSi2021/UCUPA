package resolve

import (
	"errors"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	"ucupa/ucupa-core/internal/commit"
)

func TestResolvePreservesOwnerShapeCallerAndRetirementOrder(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{71}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	limits := values.PublicLimits{ContextTTL: time.Hour, ObservationTTL: time.Hour, Contexts: 32, PerCaller: 32, Groups: 4, Bytes: 1 << 20}
	resources := registry.New(authority, limits, owner, nil)
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "resolve", Principal: "one"}}
	boundary := values.ElementID("54000000-0000-4000-8000-000000000001")
	owner.Lock()
	defer owner.Unlock()
	defer resources.CloseLocked()
	space, err := resources.IssueSpaceLocked(caller, boundary)
	if err != nil {
		t.Fatal(err)
	}
	read, err := resources.IssueLocalReadLocked(caller, boundary, space.Ref, contract.CoreReadCut{}, values.AuthorizationView{1}, nil, "list", "conditions")
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := resources.IssueContinuationLocked(read.Ref, boundary, "")
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := resources.PrepareLocalLocked(space.Ref, nil, values.AuthorizationView{1}, contract.CoreReadCut{}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := resources.BeginLocalLocked(preparation.Ref)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		token   values.PublicToken
		call    func(values.CallerContext, any) error
		retired error
	}{
		{"space", space.Token, func(c values.CallerContext, r any) error { _, e := SpaceLocked(owner, resources, c, r); return e }, values.PublicInvalidContext},
		{"read", read.Token, func(c values.CallerContext, r any) error { _, e := ReadLocked(owner, resources, c, r); return e }, values.PublicInvalidContext},
		{"continuation", continuation.Token, func(c values.CallerContext, r any) error {
			_, e := ContinuationLocked(owner, resources, c, r)
			return e
		}, values.PublicInvalidContext},
		{"preparation", preparation.Token, func(c values.CallerContext, r any) error { _, e := PreparationLocked(owner, resources, c, r); return e }, values.PublicGap},
		{"cursor", cursor, func(c values.CallerContext, r any) error { _, e := CursorLocked(owner, resources, c, r); return e }, values.PublicGap},
	}
	other := caller
	other.Subject.Principal = "two"
	for _, c := range cases {
		if err := c.call(caller, accessvalue.TokenJSON(c.token)); err != nil {
			t.Fatalf("%s valid: %v", c.name, err)
		}
		if err := c.call(other, accessvalue.TokenJSON(c.token)); !errors.Is(err, values.PublicDenied) {
			t.Fatalf("%s caller: %v", c.name, err)
		}
		wrong := c.token
		wrong.Owner = "54000000-0000-4000-8000-000000000099"
		if err := c.call(other, accessvalue.TokenJSON(wrong)); !errors.Is(err, values.PublicInvalid) {
			t.Fatalf("%s owner before caller: %v", c.name, err)
		}
		if err := c.call(caller, map[string]any{"kind": c.token.Kind}); !errors.Is(err, values.PublicInvalid) {
			t.Fatalf("%s shape: %v", c.name, err)
		}
	}
	resources.RemoveLocked(space.Ref)
	for _, c := range cases {
		if err := c.call(caller, accessvalue.TokenJSON(c.token)); !errors.Is(err, c.retired) {
			t.Fatalf("%s retired: %v", c.name, err)
		}
	}
	owner.BeginCloseLocked()
	for _, c := range cases {
		if err := c.call(other, nil); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("%s closed before shape/caller: %v", c.name, err)
		}
	}
}
