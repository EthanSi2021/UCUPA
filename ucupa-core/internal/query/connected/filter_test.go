package connected

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	querypkg "ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/exchange"
	coretoken "ucupa/ucupa-core/internal/token"
)

type filterAllow struct{}

func connectedAuthorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func connectedAuthorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (filterAllow) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (filterAllow) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, connectedAuthorizationViewDigest(caller.Subject, requirements)
}

func (filterAllow) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if connectedAuthorizationViewDigest(caller.Subject, requirements) != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (filterAllow) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (filterAllow) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := (filterAllow{}).CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type filterFixture struct {
	lease *querypkg.Lease
	root  values.ElementID
	mount values.ElementID
	key   contract.MountKey
}

func newFilterFixture(t *testing.T) filterFixture {
	t.Helper()
	const (
		authority values.FactAuthorityRef = "00000000-0000-4000-8000-000000009301"
		carrier   values.FactAuthorityRef = "00000000-0000-4000-8000-000000009302"
		root      values.ElementID        = "00000000-0000-4000-8000-000000009310"
		mount     values.ElementID        = "00000000-0000-4000-8000-000000009311"
	)
	actor := values.PrincipalRef{Realm: "connected-filter", Principal: "reader"}
	owner, err := commit.New(authority, [32]byte{1}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	if err != nil {
		t.Fatal(err)
	}
	elements := element.New(owner, filterAllow{})
	if _, err := elements.CreateRoot(actor, root, "filter-root"); err != nil {
		t.Fatal(err)
	}
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue, err := contract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := elements.CreateAnchor(actor, mount, contract.SpatialTuple{Parent: root, LocalPose: pose, Range: rangeValue}, "filter-mount"); err != nil {
		t.Fatal(err)
	}
	queries := querypkg.New(owner, filterAllow{}, [32]byte{2}, querypkg.Settings{Handles: 8, Bytes: 1 << 20, Concurrent: 8})
	lease, err := queries.Capture(actor, mount, nil, contract.QueryCurrentVerified)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		lease.Finish()
		queries.Close()
	})
	key := coretoken.MountKey{ParentAuthority: authority, MountAnchorUUID: mount, CarrierAuthority: carrier,
		ProposalRevision: coretoken.MountRevisionFromParts([32]byte{3}, mount, 1)}
	return filterFixture{lease: lease, root: root, mount: mount, key: key}
}

func TestConnectedFiniteFactsProofOnlyIsPresent(t *testing.T) {
	f := newFilterFixture(t)
	proof := filterConnectedFacts(f.lease, exchange.Grant{
		Kind: exchange.Query, Scope: f.mount, Mount: f.key, ProofOnly: true,
	})
	absent := exchange.Facts{}
	if !proof.Present {
		t.Fatal("valid proof-only reply was reported absent")
	}
	if absent.Present {
		t.Fatal("zero connected facts unexpectedly reported present")
	}
	counts := proof.Elements.Counts()
	if counts.Anchors != 0 || counts.Entities != 0 {
		t.Fatalf("proof-only projection disclosed elements: %#v", counts)
	}
	if _, ok := proof.Attachments.Record(f.mount); ok {
		t.Fatal("proof-only projection disclosed an attachment")
	}
}

func TestConnectedFilterCopiesOnlyGrantAndResolveAttachments(t *testing.T) {
	f := newFilterFixture(t)
	for _, tc := range []struct {
		name string
		kind exchange.Kind
	}{
		{name: "query", kind: exchange.Query},
		{name: "list", kind: exchange.List},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := filterConnectedFacts(f.lease, exchange.Grant{Kind: tc.kind, Scope: f.mount})
			if _, ok := facts.Elements.Anchor(f.mount); !ok {
				t.Fatalf("authorized mount anchor missing: %#v", facts.Elements.Counts())
			}
			if _, ok := facts.Elements.Anchor(f.root); ok {
				t.Fatal("element outside grant scope disclosed")
			}
			counts := facts.Elements.Counts()
			if counts.Anchors != 1 || counts.Entities != 0 {
				t.Fatalf("unexpected filtered live facts: %#v", counts)
			}
			if _, ok := facts.Attachments.Record(f.mount); ok {
				t.Fatal("non-resolve projection disclosed an attachment")
			}
		})
	}

	resolved := filterConnectedFacts(f.lease, exchange.Grant{Kind: exchange.Resolve, Scope: f.mount})
	if _, ok := resolved.Elements.Anchor(f.mount); !ok {
		t.Fatal("resolve projection lost its authorized element")
	}
	if _, ok := resolved.Elements.Anchor(f.root); ok {
		t.Fatal("resolve projection disclosed an element outside its grant")
	}
	if _, ok := resolved.Attachments.Record(f.mount); !ok {
		t.Fatal("resolve projection omitted the current attachment record")
	}
	for id := range resolved.Attachments.Records() {
		if id != f.mount {
			t.Fatalf("resolve projection disclosed an unauthorized attachment: %s", id)
		}
	}
}
