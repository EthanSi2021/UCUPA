package instance_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/instance"
)

type allow struct{}

// The port methods never dereference the receiver, so a typed nil flows
// through the instance checks into namespace's nilDependency guard unchanged.
func (*allow) Check(context.Context, values.NextHop, values.RealmID) error { return nil }
func (*allow) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (*allow) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{}
}
func (*allow) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}
func (*allow) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationDenied, values.AuthorizationDenialView{}
}
func (*allow) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

var allowInitial = contract.RealmInitialization{Realm: "test-realm", NextHop: "local:instance-test"}

// Construction and local reference validation must not contact other owners.
type offline struct{}

func (offline) ReadProposal(context.Context, values.NextHop, values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	panic("unexpected proposal read")
}
func (offline) ReadAttachment(context.Context, values.NextHop, values.FacetAcceptanceKey) (contract.CoreEvidence, error) {
	panic("unexpected attachment read")
}

func (offline) ReadSuccessor(context.Context, values.NextHop, values.FactAuthorityRef, values.ElementID) (contract.CoreSuccessorRead, error) {
	panic("unexpected successor read")
}

func TestInstanceSharesOneOwnerAndClosesTheDurableResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facet.db")
	i, err := instance.CreateDurable(&allow{}, offline{}, allowInitial, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i.Close() })
	id := i.AuthorityRef()
	copy := *i
	if !id.Valid() || copy.Namespace != i.Namespace || copy.AuthorityRef() != id {
		t.Fatal("instance copy changed owner")
	}
	actor := values.PrincipalRef{Realm: "test-realm", Principal: "reader"}
	if _, err := copy.Namespace.ReadAcceptance(actor, values.FacetAcceptanceKey{}); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("direct namespace operation changed validation: %v", err)
	}
	if err := copy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := instance.OpenDurable(&allow{}, offline{}, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if reopened.AuthorityRef() != id || reopened.Namespace == i.Namespace {
		t.Fatal("reopen changed persistent identity or reused retired runtime")
	}
}

func TestInstanceKeepsConfigurationValidationAndNoDefaultPolicy(t *testing.T) {
	var nilAuth *allow
	if i, err := instance.New(nilAuth, offline{}, allowInitial, instance.Config{}); i != nil || !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("typed nil dependency: %v %v", i, err)
	}
	bad := instance.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: -time.Second}}
	if i, err := instance.New(&allow{}, offline{}, allowInitial, bad); i != nil || !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("invalid policy: %v %v", i, err)
	}
	i, err := instance.New(&allow{}, offline{}, allowInitial, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i.Close() })
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "test-realm", Principal: "reader"}}
	if data, err := i.Namespace.PublicCall(context.Background(), caller, []byte(`{}`)); data != nil || !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("missing policy silently enabled public access: %q %v", data, err)
	}
}

func TestInstanceExposesLifecycleNotBusinessForwarders(t *testing.T) {
	typ := reflect.TypeOf((*instance.Instance)(nil))
	if typ.NumMethod() != 2 {
		t.Fatalf("unexpected instance methods: %d", typ.NumMethod())
	}
	for _, name := range []string{"AuthorityRef", "Close"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("missing lifecycle method %s", name)
		}
	}
	owner := reflect.TypeOf(instance.Instance{}.Namespace)
	if _, ok := owner.MethodByName("Close"); ok {
		t.Fatal("namespace exposes a duplicate lifecycle entry")
	}
	for _, name := range []string{"Get", "Put", "Delete", "ListTypes", "AcceptAttachment", "ReadAcceptance", "UpdateCoreNextHop", "PublicCall"} {
		if _, ok := owner.MethodByName(name); !ok {
			t.Errorf("missing direct namespace operation %s", name)
		}
	}
	var nilInstance *instance.Instance
	if nilInstance.AuthorityRef() != "" || nilInstance.Close() != nil {
		t.Fatal("nil lifecycle handle changed behavior")
	}
}

// TestInstanceBindsOnlyTrustedVerifiableContact proves the public-chain
// creation gate: no constructor accepts an invalid or unverified Realm
// contact, and an open-time assertion can never rescue a mismatch.
func TestInstanceBindsOnlyTrustedVerifiableContact(t *testing.T) {
	port := maintenanceAssembly()
	initial := contract.RealmInitialization{Realm: "realm-x", NextHop: "local:realm-x"}
	for _, invalid := range []contract.RealmInitialization{
		{Realm: "realm-x"},
		{NextHop: "local:realm-x"},
		{Realm: "realm-x", NextHop: "remote:realm-x"},
	} {
		if i, err := instance.New(port, offline{}, invalid, instance.Config{}); i != nil || !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("invalid initial %#v created instance: %v, %v", invalid, i, err)
		}
		if i, err := instance.CreateDurable(port, offline{}, invalid, filepath.Join(t.TempDir(), "facet.db"), instance.Config{}); i != nil || !errors.Is(err, contract.ErrInvalid) {
			t.Fatalf("invalid initial %#v created Store: %v, %v", invalid, i, err)
		}
	}
	if i, err := instance.CreateDurable(port, offline{}, initial, filepath.Join(t.TempDir(), "facet.db"), instance.Config{ExpectedRealm: "other-realm"}); i != nil || !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("mismatched creation assertion: %v, %v", i, err)
	}
	offlinePort := maintenanceAssembly()
	offlinePort.live["local:realm-x"] = false
	if i, err := instance.New(offlinePort, offline{}, initial, instance.Config{}); i != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("unverifiable contact created instance: %v, %v", i, err)
	}
	if i, err := instance.CreateDurable(offlinePort, offline{}, initial, filepath.Join(t.TempDir(), "facet.db"), instance.Config{}); i != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("unverifiable contact created Store: %v, %v", i, err)
	}

	path := filepath.Join(t.TempDir(), "facet.db")
	created, err := instance.CreateDurable(port, offline{}, initial, path, instance.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := instance.OpenDurable(port, offline{}, path, instance.Config{ExpectedRealm: "other-realm"}); reopened != nil || !errors.Is(err, contract.ErrStore) {
		t.Fatalf("wrong assertion opened Store: %v, %v", reopened, err)
	}
	reopened, err := instance.OpenDurable(port, offline{}, path, instance.Config{ExpectedRealm: "realm-x"})
	if err != nil {
		t.Fatalf("matching assertion rejected: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}
