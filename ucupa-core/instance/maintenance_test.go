package instance_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/instance"
)

// maintenancePort controls which local addresses this test assembly trusts.
type maintenancePort struct {
	live     map[values.NextHop]bool
	realm    map[values.NextHop]values.RealmID
	decision map[values.NextHop]values.AuthorizationDecision
	used     []values.NextHop
}

func (p *maintenancePort) Check(ctx context.Context, address values.NextHop, expected values.RealmID) error {
	if !p.live[address] || p.realm[address] != expected {
		return errors.New("endpoint offline")
	}
	return nil
}
func (*maintenancePort) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func (p *maintenancePort) AuthorizeView(_ context.Context, address values.NextHop, _ values.RealmID, _ values.CallerContext, _ ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	p.used = append(p.used, address)
	return p.decision[address], values.AuthorizationView{}
}
func (*maintenancePort) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func (*maintenancePort) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}
func (*maintenancePort) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func maintenanceAssembly() *maintenancePort {
	return &maintenancePort{
		live:     map[values.NextHop]bool{"local:realm-c": true, "local:realm-d": true},
		realm:    map[values.NextHop]values.RealmID{"local:realm-c": "realm-c", "local:realm-d": "realm-c"},
		decision: map[values.NextHop]values.AuthorizationDecision{"local:realm-c": values.AuthorizationValid, "local:realm-d": values.AuthorizationValid},
	}
}

func TestMaintenanceIsTheMinimalExclusiveHandle(t *testing.T) {
	typ := reflect.TypeOf((*instance.RealmMaintenance)(nil))
	if typ.NumMethod() != 4 {
		t.Fatalf("maintenance handle must stay minimal: %d methods", typ.NumMethod())
	}
	for _, name := range []string{"Inspect", "Update", "Recover", "Close"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("missing maintenance method %s", name)
		}
	}
}

func TestMaintenanceOpenRepairsOfflineContactThroughPublicChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core.db")
	port := maintenanceAssembly()
	initial := corecontract.RealmInitialization{Realm: "realm-c", NextHop: "local:realm-c"}
	created, err := instance.CreateDurable(port, initial, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := created.CurrentRealmContact(); !ok {
		t.Fatal("created Core has no durable contact")
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}

	// The persisted address went offline: the ordinary open refuses to verify
	// it, the controlled maintenance open does not.
	port.live["local:realm-c"] = false
	if reopened, err := instance.OpenDurable(port, path, corecontract.Config{}); reopened != nil || !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("ordinary open accepted an unverifiable contact: %v, %v", reopened, err)
	}
	if _, err := instance.OpenRealmMaintenance(port, path, corecontract.Config{ExpectedRealm: "other-realm"}); !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("wrong expected Realm opened maintenance: %v", err)
	}
	if _, err := instance.OpenRealmMaintenance(port, filepath.Join(t.TempDir(), "missing.db"), corecontract.Config{}); !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("missing Store opened maintenance: %v", err)
	}
	m, err := instance.OpenRealmMaintenance(port, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// The current address cannot authorize, so disclosure needs the candidate.
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-c", Principal: "operator"},
		Credential: [16]byte{1}, Establishment: [32]byte{2}, Proof: [32]byte{3}}
	if _, err := m.Inspect(context.Background(), caller, ""); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("offline current address disclosed contact without candidate: %v", err)
	}
	contact, err := m.Inspect(context.Background(), caller, "local:realm-d")
	if err != nil || contact.NextHop != "local:realm-c" || contact.Realm != "realm-c" {
		t.Fatalf("candidate inspect = %+v, %v", contact, err)
	}
	request := corecontract.RealmContactUpdate{OperationID: "repair", Owner: contact.Owner, Realm: contact.Realm,
		Expected: contact.Revision, NextHop: "local:realm-d"}
	result, err := m.Update(context.Background(), caller, request)
	if err != nil || result.Decision != corecontract.RealmContactCommitted || result.Contact.NextHop != "local:realm-d" {
		t.Fatalf("candidate repair = %+v, %v", result, err)
	}
	if port.used[len(port.used)-1] != "local:realm-d" {
		t.Fatalf("repair authorized through %v", port.used)
	}
	recovered, err := m.Recover(context.Background(), caller, "repair", "")
	if err != nil || recovered != result {
		t.Fatalf("recover after repair = %+v, %v", recovered, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	// The repaired contact now opens through the ordinary entry.
	reopened, err := instance.OpenDurable(port, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	current, ok := reopened.CurrentRealmContact()
	if !ok || current.NextHop != "local:realm-d" {
		t.Fatalf("ordinary reopen after repair = %+v, %v", current, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceOpenKeepsStoreExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core.db")
	port := maintenanceAssembly()
	initial := corecontract.RealmInitialization{Realm: "realm-c", NextHop: "local:realm-c"}
	created, err := instance.CreateDurable(port, initial, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := instance.OpenRealmMaintenance(port, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if second, err := instance.OpenDurable(port, path, corecontract.Config{}); second != nil || !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("ordinary open bypassed exclusive maintenance lock: %v, %v", second, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := instance.OpenDurable(port, path, corecontract.Config{})
	if err != nil {
		t.Fatalf("store stayed locked after maintenance close: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

type cancelMaintenancePort struct {
	*maintenancePort
	cancel context.CancelFunc
}

func (p *cancelMaintenancePort) AuthorizeView(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	decision, view := p.maintenancePort.AuthorizeView(ctx, address, expected, caller, requirements...)
	if p.cancel != nil {
		p.cancel()
	}
	return decision, view
}

func TestMaintenanceCanceledUpdateHasNoDurableOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "core.db")
	port := &cancelMaintenancePort{maintenancePort: maintenanceAssembly()}
	created, err := instance.CreateDurable(port, corecontract.RealmInitialization{Realm: "realm-c", NextHop: "local:realm-c"}, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	open := func() *instance.RealmMaintenance {
		t.Helper()
		m, err := instance.OpenRealmMaintenance(port, path, corecontract.Config{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = m.Close() })
		return m
	}
	m := open()
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-c", Principal: "operator"}, Credential: [16]byte{1}, Establishment: [32]byte{2}, Proof: [32]byte{3}}
	contact, err := m.Inspect(context.Background(), caller, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port.cancel = cancel
	request := corecontract.RealmContactUpdate{OperationID: "canceled", Owner: contact.Owner, Realm: contact.Realm, Expected: contact.Revision, NextHop: "local:realm-d"}
	if _, err := m.Update(ctx, caller, request); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("canceled update = %v", err)
	}
	port.cancel = nil
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m = open()
	if _, err := m.Recover(context.Background(), caller, request.OperationID, ""); !errors.Is(err, corecontract.ErrNotFound) {
		t.Fatalf("canceled request was durably retained: %v", err)
	}
	if current, err := m.Inspect(context.Background(), caller, ""); err != nil || current != contact {
		t.Fatalf("canceled update changed contact: %+v %v", current, err)
	}
	request.OperationID = "committed"
	live, cancelAfterCommit := context.WithCancel(context.Background())
	committed, err := m.Update(live, caller, request)
	cancelAfterCommit()
	if err != nil || committed.Decision != corecontract.RealmContactCommitted {
		t.Fatalf("fresh update = %+v %v", committed, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m = open()
	if recovered, err := m.Recover(context.Background(), caller, request.OperationID, ""); err != nil || recovered != committed {
		t.Fatalf("cancellation after commit lost original result: %+v %v", recovered, err)
	}
}
