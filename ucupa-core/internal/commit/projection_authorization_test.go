package commit_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

type projectionPathAuthorizer struct {
	authority values.FactAuthorityRef
	mu        sync.Mutex
	calls     [][]values.Requirement
	deny      values.ElementID
	blockAt   int
	entered   chan struct{}
	release   chan struct{}
}

type validatingProjectionAuthorizer struct {
	authority values.FactAuthorityRef
	calls     [][]values.Requirement
	deny      values.ElementID
}

func (authorizer *validatingProjectionAuthorizer) decision(requirements ...values.Requirement) values.AuthorizationDecision {
	for _, requirement := range requirements {
		if !requirement.Valid() {
			panic("Core submitted an invalid projection Requirement")
		}
		owner, target, ok := requirement.Scope.CoreElementTarget()
		if !ok || owner != authorizer.authority || target == authorizer.deny {
			return values.AuthorizationDenied
		}
	}
	return values.AuthorizationValid
}

func (authorizer *validatingProjectionAuthorizer) AuthorizePrincipal(
	_ values.PrincipalRef,
	requirements ...values.Requirement,
) values.AuthorizationDecision {
	authorizer.calls = append(
		authorizer.calls,
		append([]values.Requirement(nil), requirements...),
	)
	return authorizer.decision(requirements...)
}

func (authorizer *validatingProjectionAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if authorizer.decision(requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (authorizer *validatingProjectionAuthorizer) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := authorizer.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (authorizer *validatingProjectionAuthorizer) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if authorizer.decision(requirement) != values.AuthorizationValid {
		return values.AuthorizationDenied, authorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (authorizer *validatingProjectionAuthorizer) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := authorizer.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (authorizer *projectionPathAuthorizer) decision(denied values.ElementID, requirements ...values.Requirement) values.AuthorizationDecision {
	for _, requirement := range requirements {
		owner, target, ok := requirement.Scope.CoreElementTarget()
		if !ok || owner != authorizer.authority || target == denied {
			return values.AuthorizationDenied
		}
	}
	return values.AuthorizationValid
}

func (authorizer *projectionPathAuthorizer) AuthorizePrincipal(
	_ values.PrincipalRef,
	requirements ...values.Requirement,
) values.AuthorizationDecision {
	copied := append([]values.Requirement(nil), requirements...)
	authorizer.mu.Lock()
	authorizer.calls = append(authorizer.calls, copied)
	call := len(authorizer.calls)
	denied := authorizer.deny
	shouldBlock := call == authorizer.blockAt
	entered, release := authorizer.entered, authorizer.release
	if shouldBlock {
		authorizer.blockAt = 0
	}
	authorizer.mu.Unlock()

	if shouldBlock {
		close(entered)
		<-release
	}
	return authorizer.decision(denied, requirements...)
}

func (authorizer *projectionPathAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	authorizer.mu.Lock()
	denied := authorizer.deny
	authorizer.mu.Unlock()
	if authorizer.decision(denied, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (authorizer *projectionPathAuthorizer) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := authorizer.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (authorizer *projectionPathAuthorizer) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if decision, _ := authorizer.AuthorizeView(ctx, caller, requirement); decision == values.AuthorizationDenied {
		return values.AuthorizationDenied, authorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (authorizer *projectionPathAuthorizer) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := authorizer.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (authorizer *projectionPathAuthorizer) reset(deny values.ElementID, blockAt int) {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	authorizer.calls = nil
	authorizer.deny = deny
	authorizer.blockAt = blockAt
	authorizer.entered = make(chan struct{})
	authorizer.release = make(chan struct{})
}

func (authorizer *projectionPathAuthorizer) recordedCalls() [][]values.Requirement {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	calls := make([][]values.Requirement, len(authorizer.calls))
	for index := range authorizer.calls {
		calls[index] = append([]values.Requirement(nil), authorizer.calls[index]...)
	}
	return calls
}

func TestCorruptProjectionPathIsAuthorizedBeforeDisclosure(t *testing.T) {
	for _, test := range []struct {
		name              string
		discoveredMissing values.ElementID
	}{
		{name: "missing parent", discoveredMissing: projectionID(999)},
		{name: "cycle"},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &projectionPathAuthorizer{}
			owner, elements, root, middle, subject := projectionFixture(t, authorizer)
			if err := commit.CorruptProjectionParentForTest(owner, test.name); err != nil {
				t.Fatal(err)
			}

			authorizer.reset(middle, 0)
			result, err := elements.ProjectToAncestor(projectionActor, subject, root, nil)
			if !errors.Is(err, corecontract.ErrDenied) || result != (corecontract.Projection{}) {
				t.Fatalf("hidden corrupt path was disclosed: result=%#v err=%v", result, err)
			}
			calls := authorizer.recordedCalls()
			if len(calls) != 2 {
				t.Fatalf("authorization calls: got %d, want 2", len(calls))
			}
			if got, want := projectionRequirementTargets(owner.AuthorityRef(), calls[0]), []values.ElementID{subject, root}; !reflect.DeepEqual(got, want) {
				t.Fatalf("input authorization targets: got %v want %v", got, want)
			}
			wantPath := []values.ElementID{subject, middle}
			if test.discoveredMissing != "" {
				wantPath = append(wantPath, test.discoveredMissing)
			}
			wantPath = append(wantPath, root)
			if got := projectionRequirementTargets(owner.AuthorityRef(), calls[1]); !reflect.DeepEqual(got, wantPath) {
				t.Fatalf("path authorization targets: got %v want %v", got, wantPath)
			}
			if test.discoveredMissing != "" {
				authorizer.reset(test.discoveredMissing, 0)
				result, err = elements.ProjectToAncestor(projectionActor, subject, root, nil)
				if !errors.Is(err, corecontract.ErrDenied) || result != (corecontract.Projection{}) {
					t.Fatalf("missing referenced scope was disclosed: result=%#v err=%v", result, err)
				}
			}

			authorizer.reset("", 0)
			result, err = elements.ProjectToAncestor(projectionActor, subject, root, nil)
			if !errors.Is(err, corecontract.ErrStore) || result != (corecontract.Projection{}) {
				t.Fatalf("authorized corrupt path: result=%#v err=%v", result, err)
			}
			if calls = authorizer.recordedCalls(); len(calls) != 2 {
				t.Fatalf("authorized corrupt path calls: got %d, want 2", len(calls))
			}
		})
	}
}

func TestNonCanonicalProjectionParentDoesNotReachAuthorizer(t *testing.T) {
	authorizer := &validatingProjectionAuthorizer{}
	owner, elements, root, middle, subject := projectionFixture(t, authorizer)
	if err := commit.CorruptProjectionParentForTest(owner, "non-canonical parent"); err != nil {
		t.Fatal(err)
	}
	authorizer.calls = nil

	var (
		result     corecontract.Projection
		err        error
		panicValue any
	)
	func() {
		defer func() { panicValue = recover() }()
		result, err = elements.ProjectToAncestor(projectionActor, subject, root, nil)
	}()
	if panicValue != nil {
		t.Fatalf("non-canonical parent caused panic: %v", panicValue)
	}
	if !errors.Is(err, corecontract.ErrStore) || result != (corecontract.Projection{}) {
		t.Fatalf("non-canonical parent: result=%#v err=%v", result, err)
	}
	if len(authorizer.calls) != 2 {
		t.Fatalf("authorization calls: got %d, want 2", len(authorizer.calls))
	}
	if got, want := projectionRequirementTargets(owner.AuthorityRef(), authorizer.calls[1]), []values.ElementID{subject, middle, root}; !reflect.DeepEqual(got, want) {
		t.Fatalf("path authorization targets: got %v want %v", got, want)
	}

	authorizer.calls = nil
	authorizer.deny = middle
	result, err = elements.ProjectToAncestor(projectionActor, subject, root, nil)
	if !errors.Is(err, corecontract.ErrDenied) || result != (corecontract.Projection{}) {
		t.Fatalf("non-canonical parent path was disclosed without middle permission: result=%#v err=%v", result, err)
	}
}

func TestCorruptProjectionPathRechecksCutAfterAuthorization(t *testing.T) {
	authorizer := &projectionPathAuthorizer{}
	owner, elements, root, _, subject := projectionFixture(t, authorizer)
	if err := commit.CorruptProjectionParentForTest(owner, "missing parent"); err != nil {
		t.Fatal(err)
	}

	authorizer.reset("", 2)
	type projectionOutcome struct {
		result corecontract.Projection
		err    error
	}
	projected := make(chan projectionOutcome, 1)
	go func() {
		result, err := elements.ProjectToAncestor(projectionActor, subject, root, nil)
		projected <- projectionOutcome{result: result, err: err}
	}()

	select {
	case <-authorizer.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("projection did not reach path authorization")
	}
	released := false
	defer func() {
		if !released {
			close(authorizer.release)
		}
	}()

	concurrentFinal := projectionTuple(t, root, 0)
	mutationDone := make(chan error, 1)
	go func() {
		_, err := elements.CreateEntity(
			projectionActor,
			projectionID(4),
			concurrentFinal,
			"projection-concurrent-cut")

		mutationDone <- err
	}()
	select {
	case err := <-mutationDone:
		if err != nil {
			t.Fatalf("concurrent mutation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("path Authorizer held the Core lock")
	}
	close(authorizer.release)
	released = true

	select {
	case outcome := <-projected:
		if !errors.Is(outcome.err, corecontract.ErrReadCutExpired) || outcome.result != (corecontract.Projection{}) {
			t.Fatalf("corrupt path used a mixed cut: result=%#v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("projection did not finish after authorization release")
	}
}

var projectionActor = values.PrincipalRef{Realm: "projection-realm", Principal: "projection-actor"}

func projectionFixture(
	t *testing.T,
	authorizer corecontract.Authorizer,
) (*commit.Owner, *element.Operations, values.ElementID, values.ElementID, values.ElementID) {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, scope, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	elements := element.New(owner, authorizer)
	switch policy := authorizer.(type) {
	case *projectionPathAuthorizer:
		policy.authority = owner.AuthorityRef()
	case *validatingProjectionAuthorizer:
		policy.authority = owner.AuthorityRef()
	}
	root := projectionID(1)
	if result, createErr := elements.CreateRoot(projectionActor, root, "projection-root"); createErr != nil || result.Record == nil {
		t.Fatalf("create root: result=%#v err=%v", result, createErr)
	}
	middle := projectionID(2)
	if result, createErr := elements.CreateAnchor(
		projectionActor, middle, projectionTuple(t, root, 10), "projection-middle"); createErr != nil || result.Record == nil {
		t.Fatalf("create middle: result=%#v err=%v", result, createErr)
	}
	subject := projectionID(3)
	if result, createErr := elements.CreateEntity(
		projectionActor, subject, projectionTuple(t, middle, 1), "projection-subject"); createErr != nil || result.Record == nil {
		t.Fatalf("create subject: result=%#v err=%v", result, createErr)
	}
	return owner, elements, root, middle, subject
}

func projectionTuple(t *testing.T, parent values.ElementID, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue}
}

func projectionID(value uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("20000000-0000-4000-8000-%012x", value))
}

func projectionRequirementTargets(authority values.FactAuthorityRef, requirements []values.Requirement) []values.ElementID {
	targets := make([]values.ElementID, 0, len(requirements))
	for _, requirement := range requirements {
		owner, target, ok := requirement.Scope.CoreElementTarget()
		if !ok || owner != authority {
			return nil
		}
		targets = append(targets, target)
	}
	return targets
}
