package policy

import (
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

func viewSeed() (contract.Seed, values.PrincipalRef, contract.PermissionBinding) {
	actor := values.PrincipalRef{Realm: "views", Principal: "manager"}
	target := contract.PermissionBinding{Subject: actor, Permission: "observe", Scope: values.AnchorClosureScope(realmCoreRef, realmSiteID)}
	return contract.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []contract.PermissionBinding{
		administrationBinding(actor, contract.PermissionPutBinding, actor.Realm),
		administrationBinding(actor, contract.PermissionDeleteBinding, actor.Realm), target,
	}}, actor, target
}

func TestAuthorizationViewRelatedABAAndIndependentChanges(t *testing.T) {
	seed, actor, target := viewSeed()
	policy := mustRealm(t, seed)
	caller, err := policy.TrustLocalCaller(actor)
	if err != nil {
		t.Fatal(err)
	}
	req := values.Requirement{Permission: target.Permission, Scope: target.Scope}
	decision, view := policy.AuthorizeView(caller, req)
	if decision != values.AuthorizationValid {
		t.Fatal(decision)
	}
	_, duplicate := policy.AuthorizeView(caller, req, req)
	if view != duplicate {
		t.Fatal("duplicate requirements changed view")
	}
	unrelated := target
	unrelated.Permission = "unrelated"
	if _, err := policy.PutBinding(actor, unrelated, policy.BindingSetRevision(), "unrelated"); err != nil {
		t.Fatal(err)
	}
	if got := policy.ValidateView(caller, view, req); got != values.AuthorizationValid {
		t.Fatal("unrelated change invalidated view", got)
	}
	if _, err := policy.PutBinding(actor, target, policy.BindingSetRevision(), "noop"); err != nil {
		t.Fatal(err)
	}
	if got := policy.ValidateView(caller, view, req); got != values.AuthorizationValid {
		t.Fatal("noop changed view", got)
	}
	if _, err := policy.DeleteBinding(actor, target, policy.BindingSetRevision(), "revoke"); err != nil {
		t.Fatal(err)
	}
	if got := policy.ValidateView(caller, view, req); got != values.AuthorizationDenied {
		t.Fatal("revocation", got)
	}
	if _, err := policy.PutBinding(actor, target, policy.BindingSetRevision(), "regrant"); err != nil {
		t.Fatal(err)
	}
	if got := policy.ValidateView(caller, view, req); got != values.AuthorizationChanged {
		t.Fatal("ABA escaped", got)
	}
	for _, forged := range []values.CallerContext{
		{Subject: actor}, {Subject: actor, Establishment: caller.Establishment},
		{Subject: values.PrincipalRef{Realm: "evil", Principal: "manager"}, Establishment: caller.Establishment, Proof: caller.Proof},
	} {
		if got, _ := policy.AuthorizeView(forged, req); got != values.AuthorizationDenied {
			t.Fatal("forged caller", got)
		}
	}
	other, _ := policy.TrustLocalCaller(actor)
	_, fresh := policy.AuthorizeView(caller, req)
	if got := policy.ValidateView(other, fresh, req); got != values.AuthorizationChanged {
		t.Fatal("establishment not bound", got)
	}
	if policy.Authorize(actor, values.Requirement{Permission: req.Permission, Scope: values.CoreElementScope(realmCoreRef, realmSiteID)}) {
		t.Fatal("closure implicitly granted exact target")
	}
}

func TestAuthorizationViewRestartRebuildsTombstoneEpochs(t *testing.T) {
	seed, actor, target := viewSeed()
	path := filepath.Join(t.TempDir(), "realm.db")
	policy, err := CreateDurable(seed, path)
	if err != nil {
		t.Fatal(err)
	}
	caller, _ := policy.TrustLocalCaller(actor)
	req := values.Requirement{Permission: target.Permission, Scope: target.Scope}
	_, view := policy.AuthorizeView(caller, req)
	if _, err := policy.DeleteBinding(actor, target, policy.BindingSetRevision(), "delete"); err != nil {
		t.Fatal(err)
	}
	epoch := policy.state.bindingEpochs[bindingKey{actor, req}]
	if err := policy.Close(); err != nil {
		t.Fatal(err)
	}
	policy, err = OpenDurable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer policy.Close()
	if got := policy.state.bindingEpochs[bindingKey{actor, req}]; got != epoch {
		t.Fatal("lost deletion epoch", got, epoch)
	}
	if _, err := policy.PutBinding(actor, target, policy.BindingSetRevision(), "restore"); err != nil {
		t.Fatal(err)
	}
	if got := policy.ValidateView(caller, view, req); got != values.AuthorizationDenied {
		t.Fatal("restart accepted old establishment", got)
	}
	fresh, _ := policy.TrustLocalCaller(actor)
	if got, _ := policy.AuthorizeView(fresh, req); got != values.AuthorizationValid {
		t.Fatal("new establishment", got)
	}
}

func TestAuthorizationViewCredentialRevocationDoesNotAffectTrustedAssembly(t *testing.T) {
	seed, actor, target := viewSeed()
	seed.LocalPrincipals = append(seed.LocalPrincipals, "user")
	managerProof := testCredentialEvidence(seed.ID, 1, 11)
	userProof := testCredentialEvidence(seed.ID, 2, 12)
	seed.Credentials = []contract.CredentialBootstrap{{Subject: actor.Principal, Evidence: managerProof}, {Subject: "user", Evidence: userProof}}
	seed.Bindings = append(seed.Bindings, administrationBinding(actor, contract.PermissionRevokeCredential, seed.ID))
	target.Subject = values.PrincipalRef{Realm: seed.ID, Principal: "user"}
	seed.Bindings = append(seed.Bindings, target)
	policy := mustRealm(t, seed)
	caller, err := policy.EstablishCaller(userProof)
	if err != nil {
		t.Fatal(err)
	}
	trusted, _ := policy.TrustLocalCaller(target.Subject)
	req := values.Requirement{Permission: target.Permission, Scope: target.Scope}
	_, view := policy.AuthorizeView(caller, req)
	_, localView := policy.AuthorizeView(trusted, req)
	if _, err := policy.RevokeCredential(managerProof, userProof.ID, policy.CredentialSetRevision(), "revoke-proof"); err != nil {
		t.Fatal(err)
	}
	if got := policy.ValidateView(caller, view, req); got != values.AuthorizationDenied {
		t.Fatal("revoked credential", got)
	}
	if got := policy.ValidateView(trusted, localView, req); got != values.AuthorizationValid {
		t.Fatal("unrelated establishment", got)
	}
}
