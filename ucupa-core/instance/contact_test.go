package instance_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/instance"
)

// movingPort authorizes every request but can move the Core's durable contact
// while one authorization is in flight.
type movingPort struct {
	hook func()
}

func (*movingPort) Check(context.Context, values.NextHop, values.RealmID) error { return nil }
func (p *movingPort) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	if p.hook != nil {
		hook := p.hook
		p.hook = nil
		hook()
	}
	return values.AuthorizationValid
}
func (*movingPort) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{}
}
func (*movingPort) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (*movingPort) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationDenied, values.AuthorizationDenialView{}
}
func (*movingPort) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

// TestInFlightAuthorizationStopsOnContactMove proves the final check between
// capture and use: an approval captured against one contact generation is
// never consumed after the contact moved, and the caller's retry reaches the
// new current address without restart.
func TestInFlightAuthorizationStopsOnContactMove(t *testing.T) {
	port := &movingPort{}
	initial := corecontract.RealmInitialization{Realm: "realm-m", NextHop: "local:old"}
	core, err := instance.New(port, initial, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	contact, ok := core.CurrentRealmContact()
	if !ok || contact.NextHop != "local:old" {
		t.Fatalf("initial contact = %+v, %v", contact, ok)
	}
	actor := values.PrincipalRef{Realm: "realm-m", Principal: "operator"}
	maintainer := values.CallerContext{Subject: actor, Credential: [16]byte{1}, Establishment: [32]byte{2}, Proof: [32]byte{3}}
	rootID := values.ElementID("00000000-0000-4000-8000-000000003001")

	// During the CreateRoot authorization the durable contact moves. The
	// in-flight approval belongs to the superseded generation and must stop,
	// not succeed and not become a denial.
	port.hook = func() {
		move := corecontract.RealmContactUpdate{OperationID: "in-flight-move", Owner: contact.Owner,
			Realm: contact.Realm, Expected: contact.Revision, NextHop: "local:new"}
		if _, err := core.RealmContact().Update(context.Background(), maintainer, move); err != nil {
			t.Errorf("contact move inside authorization: %v", err)
		}
	}
	if _, err := core.Elements().CreateRoot(actor, rootID, "crossed-generation"); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("stale approval crossed contact generation: %v", err)
	}
	moved, ok := core.CurrentRealmContact()
	if !ok || moved.NextHop != "local:new" {
		t.Fatalf("contact move did not commit: %+v, %v", moved, ok)
	}

	// The retry derives fresh evidence from the current contact.
	created, err := core.Elements().CreateRoot(actor, rootID, "crossed-generation-retry")
	if err != nil || created.Record == nil {
		t.Fatalf("retry through the moved contact = %+v, %v", created, err)
	}
}

// TestConstructionBindsOnlyTrustedVerifiableContact proves the public-chain
// creation gate: no constructor accepts an invalid or unverified Realm
// contact, and an open-time assertion can never rescue a mismatch.
func TestConstructionBindsOnlyTrustedVerifiableContact(t *testing.T) {
	port := maintenanceAssembly()
	initial := corecontract.RealmInitialization{Realm: "realm-c", NextHop: "local:realm-c"}
	for _, invalid := range []corecontract.RealmInitialization{
		{Realm: "realm-c"},
		{NextHop: "local:realm-c"},
		{Realm: "realm-c", NextHop: "remote:realm-c"},
	} {
		if c, err := instance.New(port, invalid, corecontract.Config{}); c != nil || !errors.Is(err, corecontract.ErrInvalid) {
			t.Fatalf("invalid initial %#v created Core: %v, %v", invalid, c, err)
		}
		if c, err := instance.CreateDurable(port, invalid, filepath.Join(t.TempDir(), "core.db"), corecontract.Config{}); c != nil || !errors.Is(err, corecontract.ErrInvalid) {
			t.Fatalf("invalid initial %#v created Store: %v, %v", invalid, c, err)
		}
	}
	if c, err := instance.CreateDurable(port, initial, filepath.Join(t.TempDir(), "core.db"), corecontract.Config{ExpectedRealm: "other-realm"}); c != nil || !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("mismatched creation assertion: %v, %v", c, err)
	}
	offline := maintenanceAssembly()
	offline.live["local:realm-c"] = false
	if c, err := instance.New(offline, initial, corecontract.Config{}); c != nil || !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("unverifiable contact created Core: %v, %v", c, err)
	}
	if c, err := instance.CreateDurable(offline, initial, filepath.Join(t.TempDir(), "core.db"), corecontract.Config{}); c != nil || !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("unverifiable contact created Store: %v, %v", c, err)
	}

	path := filepath.Join(t.TempDir(), "core.db")
	created, err := instance.CreateDurable(port, initial, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := instance.OpenDurable(port, path, corecontract.Config{ExpectedRealm: "other-realm"}); reopened != nil || !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("wrong assertion opened Store: %v, %v", reopened, err)
	}
	reopened, err := instance.OpenDurable(port, path, corecontract.Config{ExpectedRealm: "realm-c"})
	if err != nil {
		t.Fatalf("matching assertion rejected: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}
