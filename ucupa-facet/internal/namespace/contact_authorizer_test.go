package namespace

import (
	"context"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// moveContact commits one same-Realm contact change on the owner and
// republishes the lock-free snapshot, the way a committed update does.
func moveContact(f *Owner, address values.NextHop) {
	f.mu.Lock()
	defer f.mu.Unlock()
	moved := f.state.RealmContact.clone()
	moved.Current = address
	moved.Serial++
	f.state.RealmContact = &moved
	f.refreshContactLocked()
}

func testContactAuthorizer() (*Owner, *contactPort, values.CallerContext, values.PrincipalRef) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		panic(err)
	}
	contact, err := newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:old"})
	if err != nil {
		panic(err)
	}
	facts := emptyState()
	facts.PublicKey = [32]byte{5}
	facts.RealmContact = &contact
	port := &contactPort{
		live:     map[values.NextHop]bool{"local:old": true, "local:new": true},
		realm:    map[values.NextHop]values.RealmID{"local:old": "realm-a", "local:new": "realm-a"},
		decision: map[values.NextHop]values.AuthorizationDecision{"local:old": values.AuthorizationValid, "local:new": values.AuthorizationValid},
	}
	f := assemble(port, nil, owner, [32]byte{12})
	f.state = facts
	f.mu.Lock()
	f.refreshContactLocked()
	f.mu.Unlock()
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-a", Principal: "operator"}}
	return f, port, caller, caller.Subject
}

func TestContactAuthorizerSupersedesInFlightEvidence(t *testing.T) {
	f, port, caller, actor := testContactAuthorizer()
	requirement := values.Requirement{Permission: contract.PermissionPut, Scope: values.AuthorityScope(f.authority)}

	decision, view := f.authorizer.AuthorizeView(context.Background(), caller, requirement)
	if decision != values.AuthorizationValid || view != (values.AuthorizationView{1}) {
		t.Fatalf("stable view authorization = %v, %v", decision, view)
	}
	port.hook = func() { moveContact(f, "local:new") }
	if decision, view = f.authorizer.AuthorizeView(context.Background(), caller, requirement); decision != values.AuthorizationUnavailable || view != (values.AuthorizationView{}) {
		t.Fatalf("view evidence crossed contact generations: %v, %v", decision, view)
	}
	port.hook = func() { moveContact(f, "local:new") }
	if decision := f.authorizer.AuthorizePrincipal(actor, requirement); decision != values.AuthorizationUnavailable {
		t.Fatalf("principal evidence crossed contact generations: %v", decision)
	}
	port.hook = func() { moveContact(f, "local:new") }
	if decision := f.authorizer.ValidateView(context.Background(), caller, values.AuthorizationView{1}, requirement); decision != values.AuthorizationUnavailable {
		t.Fatalf("validation evidence crossed contact generations: %v", decision)
	}
}

func TestContactAuthorizerKeepsViewsAcrossSameRealmMove(t *testing.T) {
	f, port, caller, actor := testContactAuthorizer()
	requirement := values.Requirement{Permission: contract.PermissionPut, Scope: values.AuthorityScope(f.authority)}

	_, view := f.authorizer.AuthorizeView(context.Background(), caller, requirement)
	moveContact(f, "local:new")
	if decision := f.authorizer.ValidateView(context.Background(), caller, view, requirement); decision != values.AuthorizationValid {
		t.Fatalf("view did not survive a same-Realm contact move: %v", decision)
	}
	if decision := f.authorizer.AuthorizePrincipal(actor, requirement); decision != values.AuthorizationValid || port.used[len(port.used)-1] != "local:new" {
		t.Fatalf("authorization did not follow the moved contact: %v, used=%v", decision, port.used)
	}
}

func TestContactAuthorizerWithoutContactFailsUnavailable(t *testing.T) {
	f, port, caller, actor := testContactAuthorizer()
	requirement := values.Requirement{Permission: contract.PermissionPut, Scope: values.AuthorityScope(f.authority)}
	f.mu.Lock()
	f.state.RealmContact = nil
	f.refreshContactLocked()
	f.mu.Unlock()
	if decision := f.authorizer.AuthorizePrincipal(actor, requirement); decision != values.AuthorizationUnavailable {
		t.Fatalf("missing contact must stay unavailable, not denied: %v", decision)
	}
	if decision, view := f.authorizer.AuthorizeView(context.Background(), caller, requirement); decision != values.AuthorizationUnavailable || view != (values.AuthorizationView{}) {
		t.Fatalf("missing contact view = %v, %v", decision, view)
	}
	if decision := f.authorizer.ValidateView(context.Background(), caller, values.AuthorizationView{1}, requirement); decision != values.AuthorizationUnavailable {
		t.Fatalf("missing contact validation = %v", decision)
	}
	if len(port.used) != 0 {
		t.Fatalf("missing contact must not reach the Realm port: %v", port.used)
	}
}
