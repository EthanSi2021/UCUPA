package acceptance_test

import (
	"context"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
)

func attachFixture(t *testing.T, c *core.Core, f *facet.Instance, actor values.PrincipalRef, id values.ElementID) values.FacetAcceptanceKey {
	t.Helper()
	ctx := context.Background()
	read, err := c.Elements().ReadElement(actor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := c.Facets().ProposeFacet(actor, id, f.AuthorityRef(), "local:facet", read.Attachment.Revision, corecontract.OperationID("propose-"+string(id)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Namespace.AcceptAttachment(ctx, actor, proposed.Pending, "local:core", facetcontract.OperationID("accept-"+string(id))); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Facets().ActivateFacet(ctx, actor, id, proposed.Attachment.Revision, corecontract.OperationID("activate-"+string(id))); err != nil {
		t.Fatal(err)
	}
	return proposed.Pending
}
