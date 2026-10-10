package commit_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	querypkg "ucupa/ucupa-core/internal/query"
)

var mountGuardActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

type mountGuardPolicy struct {
	mu     sync.RWMutex
	denied bool
}

func (policy *mountGuardPolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	policy.mu.RLock()
	defer policy.mu.RUnlock()
	if policy.denied {
		return values.AuthorizationDenied
	}
	return values.AuthorizationValid
}

func (policy *mountGuardPolicy) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if policy.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (policy *mountGuardPolicy) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := policy.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (policy *mountGuardPolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if policy.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, authorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (policy *mountGuardPolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := policy.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}
func (policy *mountGuardPolicy) setDenied(denied bool) {
	policy.mu.Lock()
	policy.denied = denied
	policy.mu.Unlock()
}

type mountGuardOwner struct {
	owner    *commit.Owner
	policy   *mountGuardPolicy
	elements *element.Operations
	queries  *querypkg.Operations
}

type mountGuardFixture struct {
	parent, carrier *mountGuardOwner
	root, mount     values.ElementID
	callers         contract.LocalCallers
}

func mountGuardID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", serial))
}

func mountGuardTuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue}
}

func newMountGuardOwner(t *testing.T, authority values.FactAuthorityRef, scope, run [32]byte) *mountGuardOwner {
	t.Helper()
	policy := &mountGuardPolicy{}
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	queries := querypkg.New(owner, policy, run, querypkg.Settings{Handles: 128, Bytes: 64 << 20, Concurrent: 64})
	t.Cleanup(func() { queries.Close(); _ = owner.CloseStore() })
	return &mountGuardOwner{owner: owner, policy: policy, elements: element.New(owner, policy), queries: queries}
}

func newMountGuardFixture(t *testing.T, activate bool) mountGuardFixture {
	t.Helper()
	parent := newMountGuardOwner(t, values.FactAuthorityRef(mountGuardID(9301)), [32]byte{1}, [32]byte{11})
	carrier := newMountGuardOwner(t, values.FactAuthorityRef(mountGuardID(9302)), [32]byte{2}, [32]byte{12})
	f := mountGuardFixture{parent: parent, carrier: carrier, root: mountGuardID(9100), mount: mountGuardID(9101)}
	if _, err := parent.elements.CreateRoot(mountGuardActor, f.root, "guard-root"); err != nil {
		t.Fatal(err)
	}
	m, err := parent.elements.CreateAnchor(mountGuardActor, f.mount, mountGuardTuple(t, f.root, 0, 10), "guard-mount")
	if err != nil {
		t.Fatal(err)
	}
	f.callers = contract.LocalCallers{parent.owner.AuthorityRef(): mountGuardActor, carrier.owner.AuthorityRef(): mountGuardActor}
	f.prepare(t, "guard-establish", contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: f.mount, ExpectedElementRevision: m.Record.Anchor.Revision,
		CarrierAuthority: carrier.owner.AuthorityRef(), CarrierNextHop: "local:guard-b", ParentNextHop: "local:guard-a",
	})
	if activate {
		f.finish(t, "guard-establish", true)
	}
	return f
}

func (f mountGuardFixture) operation(t *testing.T, owner *commit.Owner, id contract.OperationID) commit.MountOperation {
	t.Helper()
	owner.RLock()
	operation, ok := owner.MountOperationLocked(id)
	owner.RUnlock()
	if !ok {
		t.Fatalf("missing Mount operation %q", id)
	}
	return operation
}

func (f mountGuardFixture) attempt(t *testing.T, owner *commit.Owner, key contract.MountAttemptKey) commit.PeerAttempt {
	t.Helper()
	owner.RLock()
	attempt, ok := owner.PeerAttemptLocked(key)
	owner.RUnlock()
	if !ok {
		t.Fatalf("missing Mount attempt %#v", key)
	}
	return attempt
}

func mountGuardApplyEvent(t *testing.T, owner *commit.Owner, event commit.MountEvent) {
	t.Helper()
	owner.Lock()
	err := owner.ApplyMountEventLocked(event)
	owner.Unlock()
	if err != nil {
		t.Fatalf("control fixture transition %d: %v", event.Kind, err)
	}
	mountGuardValidate(t, owner)
}

func mountGuardValidate(t *testing.T, owner *commit.Owner) {
	t.Helper()
	owner.RLock()
	currentErr := owner.ValidateCurrentLocked()
	owner.RUnlock()
	if currentErr != nil {
		t.Fatalf("control fixture current snapshot: %v", currentErr)
	}
	if err := commit.ValidateCompleteStateForTest(owner); err != nil {
		t.Fatalf("control fixture complete state: %v", err)
	}
}

func (f mountGuardFixture) prepare(t *testing.T, id contract.OperationID, command contract.MountCommand) {
	t.Helper()
	binding := commit.MountBinding{Actor: mountGuardActor, PeerActor: mountGuardActor, Command: command}
	f.parent.owner.Lock()
	err := f.parent.owner.ApplyMountIntentLocked(f.callers, id, binding)
	f.parent.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	mountGuardValidate(t, f.parent.owner)
	request := f.operation(t, f.parent.owner, id).Request
	f.carrier.owner.Lock()
	prepared, err := f.carrier.owner.ApplyMountPrepareLocked(f.parent.owner.AuthorityRef(), request, true)
	f.carrier.owner.Unlock()
	if err != nil {
		t.Fatalf("fixture prepare: %#v %v", prepared, err)
	}
	mountGuardValidate(t, f.carrier.owner)
	if !prepared.Prepared {
		t.Fatalf("fixture prepare was rejected: %#v", prepared)
	}
}

func (f mountGuardFixture) prepareRange(t *testing.T, id contract.OperationID, radius float64) {
	t.Helper()
	read, err := f.parent.elements.ReadElement(mountGuardActor, f.mount, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.parent.owner.RLock()
	facts := f.parent.owner.FactsLocked()
	mount := facts.Mount.Record(f.mount)
	f.parent.owner.RUnlock()
	f.prepare(t, id, contract.MountCommand{Kind: contract.MountUpdate, MountAnchorUUID: f.mount,
		ExpectedElementRevision: read.Record.Anchor.Revision, ExpectedMountRevision: mount.Revision,
		Final: mountGuardTuple(t, f.root, 0, radius)})
}

func (f mountGuardFixture) finish(t *testing.T, id contract.OperationID, decision bool) {
	t.Helper()
	key := contract.MountAttemptKey{ParentAuthority: f.parent.owner.AuthorityRef(), OperationID: id}
	prepared := f.attempt(t, f.carrier.owner, key).Prepared
	mountGuardApplyEvent(t, f.parent.owner, commit.MountEvent{Kind: commit.MountEventDecide, Operation: id, Authorized: decision, Prepared: prepared})
	evidence := f.operation(t, f.parent.owner, id).Evidence()
	f.carrier.owner.Lock()
	closed, err := f.carrier.owner.ApplyMountFinishLocked(f.parent.owner.AuthorityRef(), contract.MountFinishRequest{Evidence: evidence})
	f.carrier.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	mountGuardValidate(t, f.carrier.owner)
	mountGuardApplyEvent(t, f.parent.owner, commit.MountEvent{Kind: commit.MountEventSettle, Operation: id, Closed: closed})
}

func (f mountGuardFixture) detach(t *testing.T) {
	t.Helper()
	const id contract.OperationID = "guard-revoke"
	f.parent.owner.RLock()
	record := f.parent.owner.FactsLocked().Mount.Record(f.mount)
	f.parent.owner.RUnlock()
	binding := commit.MountBinding{Actor: mountGuardActor, PeerActor: mountGuardActor, Command: contract.MountCommand{Kind: contract.MountRevoke, MountAnchorUUID: f.mount, ExpectedMountRevision: record.Revision}}
	f.parent.owner.Lock()
	err := f.parent.owner.ApplyMountIntentLocked(f.callers, id, binding)
	f.parent.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	mountGuardValidate(t, f.parent.owner)
	evidence := f.operation(t, f.parent.owner, id).Evidence()
	f.carrier.owner.Lock()
	closed, err := f.carrier.owner.ApplyMountFinishLocked(f.parent.owner.AuthorityRef(), contract.MountFinishRequest{Evidence: evidence})
	f.carrier.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	mountGuardValidate(t, f.carrier.owner)
	mountGuardApplyEvent(t, f.parent.owner, commit.MountEvent{Kind: commit.MountEventSettle, Operation: id, Closed: closed})
}

func assertMountGuardQueryError(t *testing.T, page contract.QueryPage, got, want error) {
	t.Helper()
	if !errors.Is(got, want) || !reflect.DeepEqual(page, contract.QueryPage{}) {
		t.Fatalf("query: %#v %v; want %v", page, got, want)
	}
}
