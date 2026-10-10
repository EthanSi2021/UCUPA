package connected_test

import (
	"context"
	"encoding/binary"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

type pairAttachmentSource func(context.Context, values.NextHop, values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error)

func (source pairAttachmentSource) ReadFacetAcceptance(ctx context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
	return source(ctx, hop, key)
}

func pairAcceptedAttachment(key values.FacetAcceptanceKey) corecontract.FacetAcceptanceEvidence {
	var raw [40]byte
	raw[0] = 7
	binary.BigEndian.PutUint64(raw[32:], 2)
	cut, _ := values.NewFacetReadCut(raw[:])
	return corecontract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}
}

func pairAttachmentProposal(t *testing.T, c *localCore, id values.ElementID, op corecontract.OperationID) corecontract.FacetAttachmentResult {
	t.Helper()
	read, err := c.Elements().ReadElement(localActor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Facets().ProposeFacet(localActor, id, ref, "local:facet", read.Attachment.Revision, op)
	if err != nil || result.Status != corecontract.MutationCommitted {
		t.Fatalf("propose: %#v %v", result, err)
	}
	return result
}

func TestConnectedExactReadUsesSameCutAttachment(t *testing.T) {
	f, branch, callers, _, _ := newPairFixture(t, true)
	ctx := context.Background()
	query := pairRootQuery(f.root)
	page, err := f.parent.Connected().QueryRegionPath(ctx, callers, query, nil, 256, nil)
	if err != nil {
		t.Fatal(err)
	}
	var path corecontract.Path
	for _, match := range page.Matches {
		if match.ElementID == branch {
			path = pairClonePath(page.ResolvedScopePath)
			path.Down = append(path.Down, match.Path...)
			break
		}
	}
	// The mounted branch is an Anchor; resolve its path even if the selector
	// only returned Entity matches. The fixture uses root -> mount -> branch.
	if path.OriginAnchorID == "" {
		path = corecontract.Path{OriginAnchorID: f.root, Down: []values.ElementID{f.mount, branch}}
	}
	old, err := f.parent.Connected().ResolvePath(ctx, callers, path, &page.Cut)
	if err != nil {
		t.Fatal(err)
	}
	p := pairAttachmentProposal(t, f.carrier, branch, "connected-proposal")
	if err := f.carrier.Facets().SetFacetAcceptanceSource(pairAttachmentSource(func(_ context.Context, _ values.NextHop, k values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
		return pairAcceptedAttachment(k), nil
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := f.carrier.Facets().ActivateFacet(ctx, localActor, branch, p.Attachment.Revision, "connected-activate"); err != nil {
		t.Fatal(err)
	}
	historical, err := f.parent.Connected().ResolvePath(ctx, callers, path, &page.Cut)
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.parent.Connected().ResolvePath(ctx, callers, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Attachment != old.Attachment || historical.Source != old.Source || current.Attachment.State != corecontract.FacetActive || current.Source.Authority != f.carrier.AuthorityRef() {
		t.Fatalf("cross-Core attachment mismatch: old=%#v historical=%#v current=%#v", old, historical, current)
	}
}
