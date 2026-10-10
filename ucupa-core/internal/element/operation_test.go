package element_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

type decide func(values.PrincipalRef, ...values.Requirement) bool

func elementAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func elementAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (f decide) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if f(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (f decide) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if f.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, elementAuthorizationViewDigest(caller.Subject, requirements)
}

func (f decide) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := f.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (f decide) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if f.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, elementAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (f decide) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := f.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

var localActor = values.PrincipalRef{Realm: "element-test", Principal: "writer"}

const localID values.ElementID = "30000000-0000-4000-8000-000000000001"

func TestElementOperationSurfaceHasNoStateOrLifecycleEscape(t *testing.T) {
	typ := reflect.TypeOf((*element.Operations)(nil))
	want := []string{"AuthorityRef", "CreateAnchor", "CreateEntity", "CreateRoot", "DeleteAnchor", "DeleteEntity", "ListDirect", "ProjectToAncestor", "ReadElement", "UpdateAnchor", "UpdateEntity"}
	got := []string{}
	for i := 0; i < typ.NumMethod(); i++ {
		got = append(got, typ.Method(i).Name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("operation methods: %v", got)
	}
	for i := 0; i < typ.Elem().NumField(); i++ {
		if typ.Elem().Field(i).IsExported() {
			t.Fatal("operation exposes its dependencies")
		}
	}
	var absent *element.Operations
	checks := []func() (contract.MutationResult, error){
		func() (contract.MutationResult, error) { return absent.CreateRoot(localActor, localID, "root") },
		func() (contract.MutationResult, error) {
			return absent.CreateAnchor(localActor, localID, contract.SpatialTuple{}, "anchor")
		},
		func() (contract.MutationResult, error) {
			return absent.CreateEntity(localActor, localID, contract.SpatialTuple{}, "entity")
		},
		func() (contract.MutationResult, error) {
			return absent.UpdateAnchor(localActor, localID, contract.SpatialTuple{}, contract.Revision{}, "update-anchor")
		},
		func() (contract.MutationResult, error) {
			return absent.UpdateEntity(localActor, localID, contract.SpatialTuple{}, contract.Revision{}, "update-entity")
		},
		func() (contract.MutationResult, error) {
			return absent.DeleteAnchor(localActor, localID, contract.Revision{}, "delete-anchor")
		},
		func() (contract.MutationResult, error) {
			return absent.DeleteEntity(localActor, localID, contract.Revision{}, "delete-entity")
		},
	}
	for _, call := range checks {
		out, err := call()
		if !errors.Is(err, contract.ErrInvalid) || out != (contract.MutationResult{}) {
			t.Fatalf("nil operation: %#v %v", out, err)
		}
	}
}

func TestElementAuthorizationAndRunRetirementOrder(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{7}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	operations := element.New(owner, decide(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		if actor != localActor || len(requirements) != 1 || requirements[0] != (values.Requirement{Permission: contract.PermissionCreateRoot, Scope: values.CoreElementScope(authority, localID)}) {
			t.Error("authorization scope changed")
		}
		close(entered)
		<-release
		return true
	}))
	result := make(chan error, 1)
	go func() { _, err := operations.CreateRoot(localActor, localID, "retired"); result <- err }()
	<-entered
	// Authorization must run outside the commit lock. This also exercises an
	// already-authorized operation reaching the write gate after run retirement.
	retired := make(chan [2]bool, 1)
	go func() {
		owner.Lock()
		first, again := owner.BeginCloseLocked(), owner.BeginCloseLocked()
		owner.Unlock()
		retired <- [2]bool{first, again}
	}()
	select {
	case state := <-retired:
		close(release)
		if !state[0] || state[1] {
			t.Fatal("run retirement is not idempotent")
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("authorization held the Owner lock")
	}
	if err := <-result; !errors.Is(err, contract.ErrStore) {
		t.Fatalf("retired write: %v", err)
	}
	denied := element.New(owner, decide(func(values.PrincipalRef, ...values.Requirement) bool { return false }))
	if _, err := denied.CreateRoot(localActor, localID, "denied"); !errors.Is(err, contract.ErrDenied) {
		t.Fatalf("retirement leaked before authorization: %v", err)
	}
	owner.RLock()
	defer owner.RUnlock()
	if owner.FactsLocked().Elements.Counts().Anchors != 0 {
		t.Fatal("retired write was published")
	}
}

// A stale authorization approval must never reach the final read or commit
// boundary: when the durable Realm contact moves between authorize and the
// owner lock, the captured generation no longer matches and the whole local
// operation becomes unavailable instead of committing or disclosing data.
func TestElementBoundaryStopsOnContactMoveAfterAuthorization(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{7}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var armed func(values.PrincipalRef)
	operations := element.New(owner, decide(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		if armed != nil {
			move := armed
			armed = nil
			move(actor)
		}
		return true
	}))
	moves := 0
	movesContact := func(actor values.PrincipalRef) {
		moves++
		current, ok := owner.CurrentContact()
		if !ok {
			t.Fatal("missing durable contact")
		}
		request := contract.RealmContactUpdate{OperationID: contract.OperationID(fmt.Sprintf("move-contact-%d", moves)),
			Owner: authority, Realm: "fixture-realm", Expected: current.Revision,
			NextHop: values.NextHop(fmt.Sprintf("local:fixture-moved-%d", moves))}
		owner.Lock()
		defer owner.Unlock()
		if _, err := owner.ApplyRealmContactLocked(actor, request); err != nil {
			t.Fatalf("durable contact move = %v", err)
		}
	}
	armed = movesContact
	if _, err := operations.CreateRoot(localActor, localID, "stale-root"); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("write after contact move = %v", err)
	}
	armed = movesContact
	if _, err := operations.ReadElement(localActor, localID, nil); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("read after contact move = %v", err)
	}
	owner.RLock()
	published := owner.FactsLocked().Elements.Counts().Anchors
	owner.RUnlock()
	if published != 0 {
		t.Fatal("stale write was published")
	}
	// Contrast: with the moved contact now current, the same authorized actor
	// succeeds against the new generation without any special handling.
	if _, err := operations.CreateRoot(localActor, localID, "fresh-root"); err != nil {
		t.Fatalf("write on current contact = %v", err)
	}
	owner.RLock()
	published = owner.FactsLocked().Elements.Counts().Anchors
	owner.RUnlock()
	if published != 1 {
		t.Fatal("current-contact write was not published")
	}
}
