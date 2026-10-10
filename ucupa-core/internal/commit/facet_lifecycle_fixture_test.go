package commit_test

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	corefacet "ucupa/ucupa-core/internal/facet"
)

var attachmentActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

type attachmentPolicy struct{ denied atomic.Bool }

func (p *attachmentPolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	if p.denied.Load() {
		return values.AuthorizationDenied
	}
	return values.AuthorizationValid
}

func (p *attachmentPolicy) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if p.AuthorizePrincipal(caller.Subject, requirements...) != values.AuthorizationValid {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (p *attachmentPolicy) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	decision, current := p.AuthorizeView(ctx, caller, requirements...)
	if decision != values.AuthorizationValid {
		return decision
	}
	if current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (p *attachmentPolicy) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if p.AuthorizePrincipal(caller.Subject, requirement) == values.AuthorizationDenied {
		return values.AuthorizationDenied, authorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (p *attachmentPolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := p.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type attachmentFixture struct {
	owner    *commit.Owner
	elements *element.Operations
	facets   *corefacet.Operations
	policy   *attachmentPolicy
}

func assembleAttachmentOwner(owner *commit.Owner) *attachmentFixture {
	policy := &attachmentPolicy{}
	return &attachmentFixture{owner: owner, policy: policy,
		elements: element.New(owner, policy),
		facets:   corefacet.New(owner, policy, access.New(policy, owner, values.PublicLimits{}), nil)}
}

func reopenAttachmentOwner(path string) (*attachmentFixture, error) {
	owner, err := commit.OpenDurable(path, nil)
	if err != nil {
		return nil, err
	}
	return assembleAttachmentOwner(owner), nil
}

func attachmentID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", serial))
}

func attachmentOwner(t *testing.T, durable bool) (*attachmentFixture, values.ElementID, string) {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var scope [32]byte
	if _, err := rand.Read(scope[:]); err != nil {
		t.Fatal(err)
	}
	var owner *commit.Owner
	var path string
	if durable {
		path = filepath.Join(t.TempDir(), "core.db")
		owner, err = commit.CreateDurable(path, authority, scope, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	} else {
		owner, err = commit.New(authority, scope, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	c := assembleAttachmentOwner(owner)
	root := attachmentID(811000)
	if _, err := c.elements.CreateRoot(attachmentActor, root, "attach-root"); err != nil {
		t.Fatal(err)
	}
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	radius, err := corecontract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	id := attachmentID(811001)
	result, err := c.elements.CreateEntity(attachmentActor, id,
		corecontract.SpatialTuple{Parent: root, LocalPose: pose, Range: radius}, "attach-entity")
	if err != nil || result.Status != corecontract.MutationCommitted || result.Record == nil ||
		result.Record.Entity == nil || result.Record.Entity.ID != id {
		t.Fatalf("create attachment entity: %#v %v", result, err)
	}
	return c, id, path
}

type attachmentSourceFunc func(context.Context, values.NextHop, values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error)

func (f attachmentSourceFunc) ReadFacetAcceptance(ctx context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
	return f(ctx, hop, key)
}

func acceptedAttachment(key values.FacetAcceptanceKey) corecontract.FacetAcceptanceEvidence {
	var raw [40]byte
	raw[0] = 7
	binary.BigEndian.PutUint64(raw[32:], 2)
	cut, _ := values.NewFacetReadCut(raw[:])
	return corecontract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}
}

func attachmentProposal(t *testing.T, c *attachmentFixture, id values.ElementID, op corecontract.OperationID) corecontract.FacetAttachmentResult {
	t.Helper()
	read, err := c.elements.ReadElement(attachmentActor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.facets.ProposeFacet(attachmentActor, id, ref, "local:facet", read.Attachment.Revision, op)
	if err != nil || result.Status != corecontract.MutationCommitted {
		t.Fatalf("propose: %#v %v", result, err)
	}
	return result
}
