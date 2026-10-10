package instance_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/instance"
)

// maintenancePort controls which local addresses this test assembly trusts.
// It records the addresses maintenance actually used.
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
func (p *maintenancePort) AuthorizePrincipal(address values.NextHop, _ values.RealmID, _ values.PrincipalRef, _ ...values.Requirement) values.AuthorizationDecision {
	p.used = append(p.used, address)
	return p.decision[address]
}
func (p *maintenancePort) AuthorizeView(_ context.Context, address values.NextHop, _ values.RealmID, _ values.CallerContext, _ ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	p.used = append(p.used, address)
	return p.decision[address], values.AuthorizationView{1}
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
		live:     map[values.NextHop]bool{"local:realm-x": true, "local:realm-y": true},
		realm:    map[values.NextHop]values.RealmID{"local:realm-x": "realm-x", "local:realm-y": "realm-x"},
		decision: map[values.NextHop]values.AuthorizationDecision{"local:realm-x": values.AuthorizationValid, "local:realm-y": values.AuthorizationValid},
	}
}

func TestMaintenanceIsTheMinimalExclusiveHandle(t *testing.T) {
	typ := reflect.TypeOf((*instance.Maintenance)(nil))
	if typ.NumMethod() != 4 {
		t.Fatalf("maintenance handle must stay minimal: %d methods", typ.NumMethod())
	}
	for _, name := range []string{"InspectRealmContact", "UpdateRealmContact", "RecoverRealmContact", "Close"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("missing maintenance method %s", name)
		}
	}
}

func TestMaintenanceOpenRepairsOfflineContactThroughPublicChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facet.db")
	port := maintenanceAssembly()
	initial := contract.RealmInitialization{Realm: "realm-x", NextHop: "local:realm-x"}
	created, err := instance.CreateDurable(port, offline{}, initial, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}

	// The persisted address went offline: the ordinary open refuses to verify
	// it, the controlled maintenance open does not.
	port.live["local:realm-x"] = false
	if reopened, err := instance.OpenDurable(port, offline{}, path, instance.Config{}); reopened != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("ordinary open accepted an unverifiable contact: %v, %v", reopened, err)
	}
	if _, err := instance.OpenRealmMaintenance(port, path, instance.Config{ExpectedRealm: "other-realm"}); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("wrong expected Realm opened maintenance: %v", err)
	}
	m, err := instance.OpenRealmMaintenance(port, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// The current address cannot authorize, so disclosure needs the candidate.
	actor := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-x", Principal: "operator"}}
	if _, err := m.InspectRealmContact(context.Background(), actor, ""); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("offline current address disclosed contact without candidate: %v", err)
	}
	contact, err := m.InspectRealmContact(context.Background(), actor, "local:realm-y")
	if err != nil || contact.NextHop != "local:realm-x" || contact.Realm != "realm-x" {
		t.Fatalf("candidate inspect = %+v, %v", contact, err)
	}
	request := contract.RealmContactUpdate{OperationID: "repair", Owner: contact.Owner, Realm: contact.Realm,
		Expected: contact.Revision, NextHop: "local:realm-y"}
	result, err := m.UpdateRealmContact(context.Background(), actor, request)
	if err != nil || result.Decision != contract.RealmContactCommitted || result.Contact.NextHop != "local:realm-y" {
		t.Fatalf("candidate repair = %+v, %v", result, err)
	}
	if port.used[len(port.used)-1] != "local:realm-y" {
		t.Fatalf("repair authorized through %v", port.used)
	}
	recovered, err := m.RecoverRealmContact(context.Background(), actor, "repair", "")
	if err != nil || recovered != result {
		t.Fatalf("recover after repair = %+v, %v", recovered, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	// The repaired contact now opens through the ordinary entry.
	reopened, err := instance.OpenDurable(port, offline{}, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	current, err := reopened.Namespace.RealmContact()
	if err != nil || current.NextHop != "local:realm-y" {
		t.Fatalf("ordinary reopen after repair = %+v, %v", current, err)
	}
}

func TestMaintenanceOpenKeepsStoreExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facet.db")
	port := maintenanceAssembly()
	initial := contract.RealmInitialization{Realm: "realm-x", NextHop: "local:realm-x"}
	created, err := instance.CreateDurable(port, offline{}, initial, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := instance.OpenRealmMaintenance(port, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if second, err := instance.OpenDurable(port, offline{}, path, instance.Config{}); second != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("ordinary open bypassed exclusive maintenance lock: %v, %v", second, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := instance.OpenDurable(port, offline{}, path, instance.Config{})
	if err != nil {
		t.Fatalf("store stayed locked after maintenance close: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}
