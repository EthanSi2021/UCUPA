package policy

import (
	"context"
	"testing"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

func TestForkCallerIsIsolatedAndPreservesCredentialRevocation(t *testing.T) {
	seed, actor, target := viewSeed()
	seed.LocalPrincipals = append(seed.LocalPrincipals, "user")
	manager := testCredentialEvidence(seed.ID, 1, 11)
	user := testCredentialEvidence(seed.ID, 2, 12)
	seed.Credentials = []contract.CredentialBootstrap{{Subject: actor.Principal, Evidence: manager}, {Subject: "user", Evidence: user}}
	seed.Bindings = append(seed.Bindings, administrationBinding(actor, contract.PermissionRevokeCredential, seed.ID))
	target.Subject = values.PrincipalRef{Realm: seed.ID, Principal: "user"}
	seed.Bindings = append(seed.Bindings, target)
	r := mustRealm(t, seed)
	original, err := r.EstablishCaller(user)
	if err != nil {
		t.Fatal(err)
	}
	fork, err := r.ForkCaller(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if fork == original || fork.Subject != original.Subject || fork.Credential != original.Credential {
		t.Fatal("fork changed identity or reused establishment")
	}
	req := values.Requirement{Permission: target.Permission, Scope: target.Scope}
	decision, view := r.AuthorizeView(fork, req)
	if decision != values.AuthorizationValid {
		t.Fatal(decision)
	}
	if got := r.ValidateView(original, view, req); got != values.AuthorizationChanged {
		t.Fatalf("fork view was transferable: %v", got)
	}
	forged := original
	forged.Subject.Principal = actor.Principal
	if _, err := r.ForkCaller(context.Background(), forged); err != contract.ErrUnauthenticated {
		t.Fatalf("forged source: %v", err)
	}
	local, err := r.TrustLocalCaller(original.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ForkCaller(context.Background(), local); err != contract.ErrUnauthenticated {
		t.Fatalf("trusted local caller became network evidence: %v", err)
	}
	if _, err := r.RevokeCredential(manager, user.ID, r.CredentialSetRevision(), "revoke-fork-source"); err != nil {
		t.Fatal(err)
	}
	if got := r.ValidateView(fork, view, req); got != values.AuthorizationDenied {
		t.Fatalf("fork survived credential revoke: %v", got)
	}
	if _, err := r.ForkCaller(context.Background(), original); err != contract.ErrUnauthenticated {
		t.Fatalf("revoked source forked: %v", err)
	}
}
