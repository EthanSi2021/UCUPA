package mount

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

// boundaryDecision is a local hook-capable Authorizer: only the principal
// decision participates in the mount read boundary under test.
type boundaryDecision func(values.PrincipalRef, ...values.Requirement) bool

func (f boundaryDecision) AuthorizePrincipal(actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if f(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (f boundaryDecision) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if f.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, values.AuthorizationView{1}
}

func (f boundaryDecision) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := f.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (f boundaryDecision) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if f.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, values.AuthorizationDenialView{1}
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (f boundaryDecision) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	decision, current := f.CaptureDenialView(ctx, caller, requirement)
	if decision != values.AuthorizationDenied || current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

// A stale authorization approval must never reach the final read boundary: when
// the durable Realm contact moves between authorize and the owner read lock, the
// captured generation no longer matches and the mount read becomes unavailable
// instead of disclosing mount state.
func TestMountBoundaryStopsOnContactMoveAfterAuthorization(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{7}, corecontract.RealmInitialization{Realm: "mount-fixture", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var armed func(values.PrincipalRef)
	policy := boundaryDecision(func(actor values.PrincipalRef, requirements ...values.Requirement) bool {
		if armed != nil {
			move := armed
			armed = nil
			move(actor)
		}
		return true
	})
	limits := values.PublicLimits{ContextTTL: time.Minute, ObservationTTL: time.Minute, ConcurrentReads: 4}
	elements := element.New(owner, policy)
	mounts := New(owner, policy, access.New(policy, owner, limits))
	root, anchor := mountTestID(61), mountTestID(62)
	if _, err := elements.CreateRoot(mountTestActor, root, "boundary-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := elements.CreateAnchor(mountTestActor, anchor, mountTestTuple(t, root, 0, 10), "boundary-anchor"); err != nil {
		t.Fatal(err)
	}
	moves := 0
	movesContact := func(values.PrincipalRef) {
		moves++
		current, ok := owner.CurrentContact()
		if !ok {
			t.Fatal("missing durable contact")
		}
		request := corecontract.RealmContactUpdate{OperationID: corecontract.OperationID(fmt.Sprintf("boundary-move-%d", moves)),
			Owner: authority, Realm: "mount-fixture", Expected: current.Revision,
			NextHop: values.NextHop(fmt.Sprintf("local:fixture-moved-%d", moves))}
		owner.Lock()
		defer owner.Unlock()
		if _, err := owner.ApplyRealmContactLocked(mountTestActor, request); err != nil {
			t.Fatalf("durable contact move = %v", err)
		}
	}
	armed = movesContact
	if _, err := mounts.ReadMount(mountTestActor, anchor); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("read after contact move = %v", err)
	}
	// Contrast: with the moved contact now current, the same authorized actor
	// reads the anchor through the new generation without any special handling.
	read, err := mounts.ReadMount(mountTestActor, anchor)
	if err != nil || read.State != corecontract.MountNone || read.Incoming {
		t.Fatalf("read on current contact = %#v %v", read, err)
	}
}
