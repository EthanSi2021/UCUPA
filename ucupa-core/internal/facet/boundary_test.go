package facet_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/facet"
)

// boundaryDecision is a local hook-capable Authorizer: only the principal
// decision participates in the facet read boundary under test.
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
// captured generation no longer matches and the proposal read becomes
// unavailable instead of disclosing the pending relationship.
func TestFacetBoundaryStopsOnContactMoveAfterAuthorization(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, [32]byte{7}, contract.RealmInitialization{Realm: "facet-fixture", NextHop: "local:fixture"})
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
	elements := element.New(owner, policy)
	facets := facet.New(owner, policy, access.New(policy, owner, publicLimits(8)), nil)
	if _, err := elements.CreateRoot(facetActor, facetRoot, "boundary-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := elements.CreateEntity(facetActor, facetEntity, tuple(t, facetRoot), "boundary-entity"); err != nil {
		t.Fatal(err)
	}
	read, err := elements.ReadElement(facetActor, facetEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := facets.ProposeFacet(facetActor, facetEntity, target, "local:facet", read.Attachment.Revision, "boundary-propose")
	if err != nil || proposed.Status != contract.MutationCommitted {
		t.Fatalf("propose = %#v %v", proposed, err)
	}
	moves := 0
	movesContact := func(values.PrincipalRef) {
		moves++
		current, ok := owner.CurrentContact()
		if !ok {
			t.Fatal("missing durable contact")
		}
		request := contract.RealmContactUpdate{OperationID: contract.OperationID(fmt.Sprintf("boundary-move-%d", moves)),
			Owner: authority, Realm: "facet-fixture", Expected: current.Revision,
			NextHop: values.NextHop(fmt.Sprintf("local:fixture-moved-%d", moves))}
		owner.Lock()
		defer owner.Unlock()
		if _, err := owner.ApplyRealmContactLocked(facetActor, request); err != nil {
			t.Fatalf("durable contact move = %v", err)
		}
	}
	armed = movesContact
	if _, err := facets.ReadFacetProposal(facetActor, facetEntity); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("proposal read after contact move = %v", err)
	}
	// Contrast: with the moved contact now current, the same authorized actor
	// reads the pending proposal through the new generation without any special
	// handling.
	after, err := facets.ReadFacetProposal(facetActor, facetEntity)
	if err != nil || after.State != contract.ExistencePresent || after.Pending != proposed.Pending || after.NextHop != "local:facet" {
		t.Fatalf("proposal read on current contact = %#v %v", after, err)
	}
}
