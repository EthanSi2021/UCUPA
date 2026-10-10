package composition

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// staticContactOwner discloses one fixed current contact, the way the
// receiving owner's durable Realm contact would.
type staticContactOwner struct {
	realm   values.RealmID
	address values.NextHop
	err     error
}

func (o staticContactOwner) CurrentRealmContact() (values.RealmID, values.NextHop, error) {
	return o.realm, o.address, o.err
}

func authenticationFixture(t *testing.T) (*LocalRealmConnections, values.RealmID, values.CallerContext) {
	t.Helper()
	evidence := realm.CredentialEvidence{Audience: "authorization-a", ID: [16]byte{1}, Secret: [32]byte{2}}
	r, err := realminstance.New(realm.Seed{
		ID:              "authorization-a",
		LocalPrincipals: []values.PrincipalID{"operator"},
		Credentials:     []realm.CredentialBootstrap{{Subject: "operator", Evidence: evidence}},
	})
	if err != nil {
		t.Fatal(err)
	}
	connections := &LocalRealmConnections{}
	if err := connections.BindEndpoint("local:realm-a", r); err != nil {
		t.Fatal(err)
	}
	caller, err := connections.EstablishCaller(context.Background(), "local:realm-a", r.ID(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	return connections, r.ID(), caller
}

// ForkCaller and Establish must read the receiving owner's current durable
// contact instead of retaining a Realm object: a contact that moved away from
// the bound address, a network address beyond the local binding, a failing
// owner, or a missing owner never reaches the still-bound Realm, while the
// live contact still does.
func TestRealmNetworkAuthenticationUsesOwnerContact(t *testing.T) {
	connections, realmID, caller := authenticationFixture(t)
	live := RealmNetworkAuthentication{Connections: connections, Owner: staticContactOwner{realm: realmID, address: "local:realm-a"}}
	if fork, err := live.ForkCaller(context.Background(), caller); err != nil || fork.Subject == (values.PrincipalRef{}) {
		t.Fatalf("ForkCaller over live contact = %+v, %v", fork, err)
	}
	if _, err := live.Establish(context.Background(), "some-audience", [16]byte{1}, [32]byte{2}); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("Establish over live contact = %v; want PublicDenied for an unregistered credential", err)
	}
	moved := RealmNetworkAuthentication{Connections: connections, Owner: staticContactOwner{realm: realmID, address: "local:moved"}}
	if _, err := moved.ForkCaller(context.Background(), caller); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("ForkCaller over a contact that left the bound address = %v; want PublicUnavailable", err)
	}
	if _, err := moved.Establish(context.Background(), "some-audience", [16]byte{1}, [32]byte{2}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("Establish over a contact that left the bound address = %v; want PublicUnavailable", err)
	}
	network := RealmNetworkAuthentication{Connections: connections, Owner: staticContactOwner{realm: realmID, address: "https://elsewhere.example:443"}}
	if _, err := network.ForkCaller(context.Background(), caller); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("ForkCaller over a network address = %v; want PublicUnavailable", err)
	}
	failing := RealmNetworkAuthentication{Connections: connections, Owner: staticContactOwner{err: errors.New("owner unavailable")}}
	if _, err := failing.ForkCaller(context.Background(), caller); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("ForkCaller over a failing owner = %v; want PublicUnavailable", err)
	}
	missing := RealmNetworkAuthentication{Connections: connections}
	if _, err := missing.ForkCaller(context.Background(), caller); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("ForkCaller without an owner = %v; want PublicUnavailable", err)
	}
}
