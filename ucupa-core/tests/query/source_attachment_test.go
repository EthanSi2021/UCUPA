package query_test

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	core "ucupa/ucupa-core/instance"
)

// sourceAttachmentPolicy is the same fixed trusted test port used for both
// local writes and the public source receipt. It is installed at construction;
// the test never replaces an operation's authorizer after assembly. Its view
// evidence stays valid exactly as it was before the contact port existed.
type sourceAttachmentPolicy struct{}

func (sourceAttachmentPolicy) Check(context.Context, values.NextHop, values.RealmID) error {
	return nil
}
func (sourceAttachmentPolicy) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (sourceAttachmentPolicy) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, values.AuthorizationView{1}
}

func (sourceAttachmentPolicy) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (sourceAttachmentPolicy) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (sourceAttachmentPolicy) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

type sourceAttachmentAcceptance func(context.Context, values.NextHop, values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error)

func (f sourceAttachmentAcceptance) ReadFacetAcceptance(ctx context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
	return f(ctx, hop, key)
}

func sourceAttachmentAccepted(key values.FacetAcceptanceKey) corecontract.FacetAcceptanceEvidence {
	var raw [40]byte
	raw[0] = 7
	binary.BigEndian.PutUint64(raw[32:], 2)
	cut, _ := values.NewFacetReadCut(raw[:])
	return corecontract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}
}

func TestFederationSourceAttachmentRemainsAtSameCut(t *testing.T) {
	// Preserves the former root lifetime test through the public instance
	// handles: the source receipt must retain attachment truth from its spatial
	// cut while the current Element read immediately sees the repaired contact.
	policy := &sourceAttachmentPolicy{}
	retention := time.Hour
	c, err := core.New(policy, queryContact, corecontract.Config{
		SnapshotRetention: &retention,
		PublicAccessPolicy: &values.PublicAccessPolicy{
			ContextTTL: time.Hour, ObservationTTL: time.Hour,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	target, err := core.New(policy, queryContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	actor := values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}
	root := id(93000)
	for _, owner := range []*core.Core{c, target} {
		if created, err := owner.Elements().CreateRoot(actor, root, "federation-root"); err != nil || created.Record == nil || created.Record.Anchor == nil || created.Record.Anchor.ID != root {
			t.Fatalf("create root: %#v %v", created, err)
		}
	}
	if err := c.Facets().SetFacetAcceptanceSource(sourceAttachmentAcceptance(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
		return sourceAttachmentAccepted(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	element := id(93017)
	if created, err := c.Elements().CreateEntity(actor, element, tuple(t, root, 0, 0, 0, sphere(t, 1)), "attachment-entity"); err != nil || created.Record == nil || created.Record.Entity == nil || created.Record.Entity.ID != element {
		t.Fatalf("create entity: %#v %v", created, err)
	}
	read, err := c.Elements().ReadElement(actor, element, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := c.Facets().ProposeFacet(actor, element, target.AuthorityRef(), "local:facet-old", read.Attachment.Revision, "part-propose")
	if err != nil {
		t.Fatal(err)
	}
	active, err := c.Facets().ActivateFacet(context.Background(), actor, element, proposal.Attachment.Revision, "part-activate")
	if err != nil {
		t.Fatal(err)
	}

	caller := values.CallerContext{Subject: actor}
	part, err := c.Federation().OpenFederationReadPart(context.Background(), caller, c.AuthorityRef(), federationcontract.FederationReadRequest{
		Boundary: root, Intent: "read", PrepareObserve: true, Purpose: [32]byte{9},
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := federationcontract.FederationProjectionRequest{Kind: "read", Element: element}
	if _, err := c.Federation().ProjectFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, projection, nil, 256); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Facets().UpdateFacetNextHop(actor, "part-facet-contact", active.Attachment.Key, active.NextHopRevision, "local:facet-new"); err != nil {
		t.Fatal(err)
	}

	page, err := c.Federation().ProjectFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, projection, nil, 256)
	if err != nil || len(page.Records) != 1 || page.Records[0].Attachment == nil || page.Records[0].Attachment.NextHop != "local:facet-old" {
		t.Fatalf("mixed latest attachment into old spatial cut: %#v %v", page, err)
	}
	if page.Records[0].Attachment.PendingTarget != "" || page.Records[0].Attachment.PendingProposal.Valid() {
		t.Fatal("pending control leaked through record read")
	}
	current, err := c.Elements().ReadElement(actor, element, nil)
	if err != nil || current.Attachment.NextHop != "local:facet-new" {
		t.Fatal("latest facts delayed for snapshot", err)
	}
}
