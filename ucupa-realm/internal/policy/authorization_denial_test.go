package policy

import (
	"testing"

	"ucupa/shared/values"
)

func TestDenialViewAuthenticatedExactAndABA(t *testing.T) {
	seed, actor, target := viewSeed()
	r := mustRealm(t, seed)
	caller, _ := r.TrustLocalCaller(actor)
	req := values.Requirement{Permission: target.Permission, Scope: target.Scope}
	if d, v := r.CaptureDenialView(caller, req); d != values.AuthorizationValid || v != (values.AuthorizationDenialView{}) {
		t.Fatal("grant captured as denial", d, v)
	}
	if _, err := r.DeleteBinding(actor, target, r.BindingSetRevision(), "denial-delete"); err != nil {
		t.Fatal(err)
	}
	d, view := r.CaptureDenialView(caller, req)
	if d != values.AuthorizationDenied || view == (values.AuthorizationDenialView{}) {
		t.Fatal(d, view)
	}
	if r.ValidateView(caller, values.AuthorizationView(view), req) == values.AuthorizationValid {
		t.Fatal("denial became Allow")
	}
	unrelated := target
	unrelated.Permission = "another"
	if _, err := r.PutBinding(actor, unrelated, r.BindingSetRevision(), "denial-unrelated"); err != nil {
		t.Fatal(err)
	}
	if r.ValidateDenialView(caller, view, req) != values.AuthorizationValid {
		t.Fatal("unrelated change invalidated denial")
	}
	other, _ := r.TrustLocalCaller(actor)
	if r.ValidateDenialView(other, view, req) != values.AuthorizationChanged {
		t.Fatal("establishment not bound")
	}
	if d, v := r.CaptureDenialView(values.CallerContext{Subject: actor}, req); d != values.AuthorizationDenied || v != (values.AuthorizationDenialView{}) {
		t.Fatal("unauthenticated denial proof")
	}
	if _, err := r.PutBinding(actor, target, r.BindingSetRevision(), "denial-grant"); err != nil {
		t.Fatal(err)
	}
	if r.ValidateDenialView(caller, view, req) != values.AuthorizationChanged {
		t.Fatal("grant did not invalidate denial")
	}
	if r.ValidateView(caller, values.AuthorizationView(view), req) == values.AuthorizationValid {
		t.Fatal("denial upgraded after grant")
	}
	if _, err := r.DeleteBinding(actor, target, r.BindingSetRevision(), "denial-revoke"); err != nil {
		t.Fatal(err)
	}
	if r.ValidateDenialView(caller, view, req) != values.AuthorizationChanged {
		t.Fatal("ABA restored old denial")
	}
	changed := req
	changed.Permission = "absent-other"
	if r.ValidateDenialView(caller, view, changed) != values.AuthorizationChanged {
		t.Fatal("requirement not bound")
	}
}
