package policy

import (
	"path/filepath"
	"testing"

	"ucupa/shared/values"
)

func TestAuthorityScopePersistsExactOwnerAndABA(t *testing.T) {
	seed, actor, target := viewSeed()
	target.Permission = "writer-handoff"
	target.Scope = values.AuthorityScope(realmCoreRef)
	seed.Bindings = append(seed.Bindings, target)
	path := filepath.Join(t.TempDir(), "realm.db")
	r, err := CreateDurable(seed, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	caller, err := r.TrustLocalCaller(actor)
	if err != nil {
		t.Fatal(err)
	}
	req := values.Requirement{Permission: target.Permission, Scope: target.Scope}
	decision, view := r.AuthorizeView(caller, req)
	if decision != values.AuthorizationValid {
		t.Fatal(decision)
	}
	for _, scope := range []values.OperationScope{
		values.AuthorityScope("00000000-0000-4000-8000-000000009999"),
		values.CoreElementScope(realmCoreRef, realmSiteID),
		values.AnchorClosureScope(realmCoreRef, realmSiteID),
		values.FacetElementScope(realmCoreRef, realmSiteID),
	} {
		if r.Authorize(actor, values.Requirement{Permission: req.Permission, Scope: scope}) {
			t.Fatal("authority scope leaked into another target")
		}
	}
	if _, err = r.DeleteBinding(actor, target, r.BindingSetRevision(), "remove"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.PutBinding(actor, target, r.BindingSetRevision(), "restore"); err != nil {
		t.Fatal(err)
	}
	if d := r.ValidateView(caller, view, req); d != values.AuthorizationChanged {
		t.Fatal("ABA accepted", d)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = OpenDurable(path)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Authorize(actor, req) {
		t.Fatal("authority binding lost on reopen")
	}
	if d := r.ValidateView(caller, view, req); d != values.AuthorizationDenied {
		t.Fatal("old establishment survived reopen", d)
	}
}
