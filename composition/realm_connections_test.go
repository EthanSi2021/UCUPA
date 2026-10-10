package composition

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func TestLocalRealmConnectionsUseOnlySpecifiedAddressAndRealm(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	actor := values.PrincipalRef{Realm: "authorization-a", Principal: "operator"}
	requirement := values.Requirement{Permission: "ucupa.core.realm-contact-read", Scope: values.AuthorityScope(owner)}
	a, err := realminstance.New(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []realm.PermissionBinding{{Subject: actor, Permission: requirement.Permission, Scope: requirement.Scope}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := realminstance.New(realm.Seed{ID: "authorization-b"})
	if err != nil {
		t.Fatal(err)
	}
	var connections LocalRealmConnections
	const first values.NextHop = "local:realm-a"
	const second values.NextHop = "local:realm-b"
	if err := connections.BindEndpoint(first, a); err != nil {
		t.Fatal(err)
	}
	if err := connections.BindEndpoint(second, b); err != nil {
		t.Fatal(err)
	}
	if err := connections.Check(context.Background(), first, actor.Realm); err != nil {
		t.Fatal(err)
	}
	if got := connections.AuthorizePrincipal(first, actor.Realm, actor, requirement); got != values.AuthorizationValid {
		t.Fatalf("specified Realm decision = %v", got)
	}
	if got := connections.AuthorizePrincipal(first, actor.Realm, actor, values.Requirement{Permission: "ucupa.core.realm-contact-update", Scope: requirement.Scope}); got != values.AuthorizationDenied {
		t.Fatalf("ungranted exact permission = %v", got)
	}
	if err := connections.Check(context.Background(), second, actor.Realm); !errors.Is(err, ErrRealmTargetMismatch) {
		t.Fatalf("wrong Realm identity = %v", err)
	}
	if got := connections.AuthorizePrincipal(second, actor.Realm, actor, requirement); got != values.AuthorizationUnavailable {
		t.Fatalf("wrong Realm must not become denial or allow: %v", got)
	}
	if got := connections.AuthorizePrincipal("local:missing", actor.Realm, actor, requirement); got != values.AuthorizationUnavailable {
		t.Fatalf("no address discovery or fallback: %v", got)
	}
	if err := connections.UnbindEndpoint(first); err != nil {
		t.Fatal(err)
	}
	if got := connections.AuthorizePrincipal(first, actor.Realm, actor, requirement); got != values.AuthorizationUnavailable {
		t.Fatalf("removed endpoint reused: %v", got)
	}
	if err := connections.BindEndpoint(first, b); err != nil {
		t.Fatal(err)
	}
	if got := connections.AuthorizePrincipal(first, actor.Realm, actor, requirement); got != values.AuthorizationUnavailable {
		t.Fatalf("replacement with wrong Realm reused: %v", got)
	}
}

func TestLocalRealmConnectionsKeepViewContinuityOnSameRealm(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	actor := values.PrincipalRef{Realm: "authorization-a", Principal: "operator"}
	requirement := values.Requirement{Permission: "ucupa.core.realm-contact-read", Scope: values.AuthorityScope(owner)}
	a, err := realminstance.New(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Bindings: []realm.PermissionBinding{{Subject: actor, Permission: requirement.Permission, Scope: requirement.Scope}}})
	if err != nil {
		t.Fatal(err)
	}
	var connections LocalRealmConnections
	const first values.NextHop = "local:realm-first"
	const second values.NextHop = "local:realm-second"
	if err := connections.BindEndpoint(first, a); err != nil {
		t.Fatal(err)
	}
	captured, err := connections.selectEndpoint(first, actor.Realm)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := connections.TrustLocalCaller(first, actor.Realm, actor)
	if err != nil {
		t.Fatal(err)
	}
	decision, view := connections.AuthorizeView(context.Background(), first, actor.Realm, caller, requirement)
	if decision != values.AuthorizationValid || view == (values.AuthorizationView{}) {
		t.Fatalf("initial view = %v, %v", decision, view)
	}
	if err := connections.BindEndpoint(second, a); err != nil {
		t.Fatal(err)
	}
	if got := connections.ValidateView(context.Background(), second, actor.Realm, caller, view, requirement); got != values.AuthorizationValid {
		t.Fatalf("same Realm through specified new address lost view: %v", got)
	}
	if err := connections.UnbindEndpoint(second); err != nil {
		t.Fatal(err)
	}
	if err := connections.UnbindEndpoint(first); err != nil {
		t.Fatal(err)
	}
	if connections.stillBound(first, actor.Realm, captured) {
		t.Fatal("a captured endpoint must not survive unbinding")
	}
	if err := connections.BindEndpoint(first, a); err != nil {
		t.Fatal(err)
	}
	if connections.stillBound(first, actor.Realm, captured) {
		t.Fatal("rebinding the same Realm must not revive a captured endpoint")
	}
	if got := connections.ValidateView(context.Background(), second, actor.Realm, caller, view, requirement); got != values.AuthorizationUnavailable {
		t.Fatalf("lost address must not be denied or valid: %v", got)
	}
}

func TestLocalRealmConnectionsDoNotReuseEndpointSerial(t *testing.T) {
	a, err := realminstance.New(realm.Seed{ID: "authorization-a"})
	if err != nil {
		t.Fatal(err)
	}
	connections := LocalRealmConnections{serial: math.MaxUint64}
	if err := connections.BindEndpoint("local:realm", a); !errors.Is(err, ErrInvalidRealmEndpoint) {
		t.Fatalf("exhausted endpoint serial = %v", err)
	}
	if err := connections.Check(context.Background(), "local:realm", a.ID()); !errors.Is(err, ErrRealmEndpointUnavailable) {
		t.Fatalf("exhausted serial must not create a binding: %v", err)
	}
}

// The local profile has no proof-shaped foreign establishment: its foreign
// identity chain is verified by trusted assembly and established through
// TrustLocalCaller. The proof-shaped face fails closed.
func TestLocalRealmConnectionsForeignEstablishmentFailsClosed(t *testing.T) {
	a, err := realminstance.New(realm.Seed{ID: "authorization-a"})
	if err != nil {
		t.Fatal(err)
	}
	var connections LocalRealmConnections
	const address values.NextHop = "local:realm"
	if err := connections.BindEndpoint(address, a); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connections.EstablishForeignCaller(context.Background(), address, a.ID(), values.FactAuthorityRef(""), []byte("payload"), make([]byte, 64), [32]byte{1}); !errors.Is(err, realm.ErrUnauthenticated) {
		t.Fatalf("local foreign establishment = %v", err)
	}
	if _, _, err := connections.EstablishForeignCaller(context.Background(), "local:missing", a.ID(), values.FactAuthorityRef(""), []byte("payload"), make([]byte, 64), [32]byte{1}); !errors.Is(err, realm.ErrUnauthenticated) {
		t.Fatalf("unbound local foreign establishment = %v", err)
	}
}

func TestLocalRealmConnectionsRejectRetiredRealm(t *testing.T) {
	a, err := realminstance.CreateDurable(realm.Seed{ID: "authorization-a"}, filepath.Join(t.TempDir(), "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	var connections LocalRealmConnections
	const address values.NextHop = "local:realm"
	if err := connections.BindEndpoint(address, a); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := connections.Check(context.Background(), address, a.ID()); !errors.Is(err, ErrRealmEndpointUnavailable) {
		t.Fatalf("retired Realm passed startup check: %v", err)
	}
	if got := connections.AuthorizePrincipal(address, a.ID(), values.PrincipalRef{Realm: a.ID(), Principal: "operator"}, values.Requirement{}); got != values.AuthorizationUnavailable {
		t.Fatalf("retired Realm became a denial: %v", got)
	}
}
