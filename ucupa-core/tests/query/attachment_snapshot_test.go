package query_test

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var attachmentSnapshotActor = values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}

type attachmentSnapshotSource func(context.Context, values.NextHop, values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error)

func (f attachmentSnapshotSource) ReadFacetAcceptance(ctx context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
	return f(ctx, hop, key)
}

func attachmentSnapshotEvidence(key values.FacetAcceptanceKey) corecontract.FacetAcceptanceEvidence {
	var raw [40]byte
	raw[0] = 7
	binary.BigEndian.PutUint64(raw[32:], 2)
	cut, _ := values.NewFacetReadCut(raw[:])
	return corecontract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}
}

func TestAttachmentSnapshotKeepsOriginalAttachment(t *testing.T) {
	ttl := time.Minute
	c, err := core.New(queryPolicyContact{allowAll{}}, queryContact, corecontract.Config{SnapshotRetention: &ttl})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	root := id(91000)
	if result, err := c.Elements().CreateRoot(attachmentSnapshotActor, root, "snapshot-root"); err != nil ||
		result.Record == nil || result.Record.Anchor == nil || result.Record.Anchor.ID != root {
		t.Fatalf("snapshot root: %#v %v", result, err)
	}
	ids := []values.ElementID{id(91001), id(91002), id(91003)}
	for _, elementID := range ids {
		result, err := c.Elements().CreateEntity(attachmentSnapshotActor, elementID,
			tuple(t, root, 0, 0, 0, sphere(t, 0)), corecontract.OperationID("snapshot-"+string(elementID)))
		if err != nil || result.Status != corecontract.MutationCommitted || result.Record == nil ||
			result.Record.Entity == nil || result.Record.Entity.ID != elementID {
			t.Fatalf("snapshot entity: %#v %v", result, err)
		}
	}
	q := corecontract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: corecontract.QueryEntities}
	page, err := c.Queries().QueryRegion(attachmentSnapshotActor, q, nil, 256, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := ids[0]
	old, err := c.Queries().ReadElementSnapshot(attachmentSnapshotActor, id, *page.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	read, err := c.Elements().ReadElement(attachmentSnapshotActor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Facets().ProposeFacet(attachmentSnapshotActor, id, ref, "local:facet", read.Attachment.Revision, "snapshot-propose")
	if err != nil || p.Status != corecontract.MutationCommitted {
		t.Fatalf("propose: %#v %v", p, err)
	}
	if err := c.Facets().SetFacetAcceptanceSource(attachmentSnapshotSource(func(_ context.Context, _ values.NextHop, k values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
		return attachmentSnapshotEvidence(k), nil
	})); err != nil {
		t.Fatal(err)
	}
	_, err = c.Facets().ActivateFacet(context.Background(), attachmentSnapshotActor, id, p.Attachment.Revision, "snapshot-active")
	if err != nil {
		t.Fatal(err)
	}
	history, err := c.Queries().ReadElementSnapshot(attachmentSnapshotActor, id, *page.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	current, err := c.Elements().ReadElement(attachmentSnapshotActor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if history.Attachment != old.Attachment || history.Cut != old.Cut || current.Attachment.State != corecontract.FacetActive {
		t.Fatal("snapshot mixed current attachment")
	}
	if current.Record.Entity.Revision != history.Record.Entity.Revision {
		t.Fatal("attachment changed spatial revision")
	}
}
