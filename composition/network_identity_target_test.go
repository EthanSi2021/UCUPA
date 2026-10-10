package composition

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// recordingEstablishment records the exact contact one proof-shaped
// establishment attempt used and answers from the fixture.
type recordingEstablishment struct {
	address values.NextHop
	realm   values.RealmID
	actor   values.FactAuthorityRef
	payload []byte
	calls   int
	caller  values.CallerContext
	expires time.Time
	err     error
}

func (s *recordingEstablishment) TrustLocalCaller(address values.NextHop, expected values.RealmID, subject values.PrincipalRef) (values.CallerContext, error) {
	s.calls++
	s.address, s.realm = address, expected
	return values.CallerContext{}, realm.ErrUnauthenticated
}

func (s *recordingEstablishment) EstablishForeignCaller(_ context.Context, address values.NextHop, expected values.RealmID, actor values.FactAuthorityRef, payload, signature []byte, challenge [32]byte) (values.CallerContext, time.Time, error) {
	s.calls++
	s.address, s.realm, s.payload = address, expected, payload
	s.actor = actor
	return s.caller, s.expires, s.err
}

// recordingMinter records the contact one delegated challenge mint used.
type recordingMinter struct {
	address   values.NextHop
	realm     values.RealmID
	calls     int
	challenge [32]byte
	expires   time.Time
	err       error
}

func (s *recordingMinter) BeginChallenge(_ context.Context, address values.NextHop, expected values.RealmID) ([32]byte, time.Time, error) {
	s.calls++
	s.address, s.realm = address, expected
	return s.challenge, s.expires, s.err
}

type stubLiveness struct{ err error }

func (s stubLiveness) Check(context.Context, []byte, []byte) error { return s.err }

func externalIdentityFixture(t *testing.T, connections RealmIdentityEstablishment, owner realmContactSource) *RealmExternalIdentity {
	t.Helper()
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{"issuer-realm": public}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	x, err := NewRealmExternalIdentity(connections, owner, verifier, map[values.RealmID]IdentityLiveness{"issuer-realm": stubLiveness{}})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// A network contact routes the whole foreign chain to the serving Realm named
// by the owner's persisted contact, once, and returns the minted handle
// without this process re-verifying or re-assembling the caller.
func TestRealmExternalIdentityNetworkContactRoutesToServingRealm(t *testing.T) {
	const address values.NextHop = "https://home.example:443"
	establishment := &recordingEstablishment{caller: values.CallerContext{Subject: values.PrincipalRef{Realm: "issuer-realm", Principal: "operator"}, Establishment: [32]byte{5}, Proof: [32]byte{5}}, expires: time.Now().Add(time.Minute)}
	x := externalIdentityFixture(t, establishment, staticContactOwner{realm: "home-realm", address: address})
	payload := []byte("payload")
	caller, expires, err := x.Establish(context.Background(), payload, make([]byte, 64), "core-a", [32]byte{7})
	if err != nil || caller != establishment.caller || !expires.Equal(establishment.expires) {
		t.Fatalf("network establishment = %+v, %v, %v", caller, expires, err)
	}
	if establishment.calls != 1 || establishment.address != address || establishment.realm != "home-realm" || establishment.actor != "core-a" {
		t.Fatalf("establishment attempts = %d at %v, %v", establishment.calls, establishment.address, establishment.realm)
	}
}

// A remote Realm's decision is terminal: an establishment failure is mapped
// to the public failure values and never retried against the trusted local
// face, another address or a local assembly path.
func TestRealmExternalIdentityNetworkFailureNeverRetriesLocally(t *testing.T) {
	const address values.NextHop = "https://home.example:443"
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{"unauthenticated", realm.ErrUnauthenticated, values.PublicDenied},
		{"store failure", realm.ErrStore, values.PublicUnavailable},
		{"invalid", realm.ErrInvalid, values.PublicInvalid},
	} {
		establishment := &recordingEstablishment{err: test.err}
		x := externalIdentityFixture(t, establishment, staticContactOwner{realm: "home-realm", address: address})
		if _, _, err := x.Establish(context.Background(), []byte("payload"), make([]byte, 64), "core-a", [32]byte{7}); !errors.Is(err, test.want) {
			t.Fatalf("%s mapped = %v; want %v", test.name, err, test.want)
		}
		if establishment.calls != 1 {
			t.Fatalf("%s establishment attempts = %d", test.name, establishment.calls)
		}
	}
}

// A local contact keeps the trusted in-process chain: the proof-shaped face is
// never reached and an unverifiable proof is denied without any network wait.
func TestRealmExternalIdentityLocalContactStaysOnTrustedFace(t *testing.T) {
	establishment := &recordingEstablishment{}
	x := externalIdentityFixture(t, establishment, staticContactOwner{realm: "home-realm", address: "local:realm-home"})
	if _, _, err := x.Establish(context.Background(), []byte("payload"), make([]byte, 64), "core-a", [32]byte{7}); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("local unverified proof = %v; want PublicDenied", err)
	}
	if establishment.calls != 0 {
		t.Fatalf("local contact reached the proof-shaped face %d times", establishment.calls)
	}
}

// The delegated challenge mint fails closed on an unwired source, a failing
// owner, a canceled context, a Realm failure, a zero or already-expired mint,
// and passes only a live mint bound to the owner's persisted contact.
func TestRealmChallengeSourceFailsClosedAndPassesContact(t *testing.T) {
	const address values.NextHop = "https://home.example:443"
	owner := staticContactOwner{realm: "home-realm", address: address}
	if _, _, err := (RealmChallengeSource{}).BeginChallenge(context.Background()); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("unwired source = %v", err)
	}
	if _, _, err := (RealmChallengeSource{Owner: owner}).BeginChallenge(context.Background()); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("missing connections = %v", err)
	}
	minter := &recordingMinter{challenge: [32]byte{9}, expires: time.Now().Add(time.Minute)}
	if _, _, err := (RealmChallengeSource{Connections: minter, Owner: staticContactOwner{err: errors.New("owner unavailable")}}).BeginChallenge(context.Background()); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("failing owner = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := (RealmChallengeSource{Connections: minter, Owner: owner}).BeginChallenge(canceled); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("canceled context = %v", err)
	}
	if minter.calls != 0 {
		t.Fatalf("a failing owner or canceled context reached the Realm %d times", minter.calls)
	}
	broken := []struct {
		name   string
		minter *recordingMinter
	}{
		{"Realm mint failure", &recordingMinter{err: realm.ErrStore}},
		{"expired mint", &recordingMinter{challenge: [32]byte{9}, expires: time.Now().Add(-time.Second)}},
		{"zero challenge", &recordingMinter{challenge: [32]byte{}, expires: time.Now().Add(time.Minute)}},
	}
	for _, test := range broken {
		if _, _, err := (RealmChallengeSource{Connections: test.minter, Owner: owner}).BeginChallenge(context.Background()); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("%s = %v", test.name, err)
		}
	}
	live := &recordingMinter{challenge: [32]byte{9}, expires: time.Now().Add(time.Minute)}
	challenge, expires, err := (RealmChallengeSource{Connections: live, Owner: owner}).BeginChallenge(context.Background())
	if err != nil || challenge != ([32]byte{9}) || !expires.Equal(live.expires) {
		t.Fatalf("live mint = %v, %v, %v", challenge, expires, err)
	}
	if live.calls != 1 || live.address != address || live.realm != "home-realm" {
		t.Fatalf("mint contact = %d at %v, %v", live.calls, live.address, live.realm)
	}
}
