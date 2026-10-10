package read_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	elementread "ucupa/ucupa-core/internal/element/read"
)

type adjacencyAllowed struct{}

func adjacencyAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func adjacencyAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (adjacencyAllowed) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (adjacencyAllowed) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, adjacencyAuthorizationViewDigest(caller.Subject, requirements)
}

func (adjacencyAllowed) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if adjacencyAuthorizationViewDigest(caller.Subject, requirements) != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (adjacencyAllowed) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (adjacencyAllowed) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := (adjacencyAllowed{}).CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func adjacencyID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", serial))
}

func TestDirectAdjacencyMatchesPublicListAcrossPages(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	owner, err := commit.New(authority, scope, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	elements := element.New(owner, adjacencyAllowed{})
	actor := values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}
	root := adjacencyID(93000)
	if created, err := elements.CreateRoot(actor, root, "federation-root"); err != nil || created.Record == nil {
		t.Fatal(created, err)
	}
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 259; i++ {
		id := adjacencyID(uint64(95000 + i))
		created, err := elements.CreateEntity(actor, id, contract.SpatialTuple{Parent: root, LocalPose: pose, Range: sphere}, contract.OperationID(id))
		if err != nil || created.Record == nil {
			t.Fatal(created, err)
		}
	}
	owner.RLock()
	adjacency, err := elementread.DirectAdjacency(context.Background(), owner.FactsLocked().Elements, map[values.ElementID]struct{}{root: {}})
	owner.RUnlock()
	if err != nil {
		t.Fatal(err)
	}
	var got []values.ElementID
	var continuation *contract.ListContinuation
	for {
		page, err := elements.ListDirect(actor, root, 37, continuation)
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range page.Records {
			got = append(got, record.Entity.ID)
		}
		continuation = page.Continuation
		if continuation == nil {
			break
		}
	}
	if len(got) != 259 || !reflect.DeepEqual(got, adjacency[root]) {
		t.Fatal("internal adjacency differs from complete public list", len(got))
	}
}
