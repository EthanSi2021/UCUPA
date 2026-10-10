package main

import (
	"context"
	"crypto/sha256"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

func demoType() values.FacetTypeRef {
	digest := sha256.Sum256([]byte("ucupa-facet-demo-type"))
	typ, _ := values.NewFacetTypeRef(digest[:])
	return typ
}
func attachDemo(cluster *composition.Cluster) (values.FacetAcceptanceKey, error) {
	read, err := cluster.Core().Elements().ReadElement(operator, localEntityID, nil)
	if err != nil {
		return values.FacetAcceptanceKey{}, err
	}
	p, err := cluster.Core().Facets().ProposeFacet(operator, localEntityID, cluster.Facets().AuthorityRef(), "local:facet", read.Attachment.Revision, "demo-propose")
	if err != nil {
		return values.FacetAcceptanceKey{}, err
	}
	if _, err = cluster.Facets().Namespace.AcceptAttachment(context.Background(), operator, p.Pending, "local:core", "demo-accept"); err != nil {
		return values.FacetAcceptanceKey{}, err
	}
	if _, err = cluster.Core().Facets().ActivateFacet(context.Background(), operator, localEntityID, p.Attachment.Revision, corecontract.OperationID("demo-activate")); err != nil {
		return values.FacetAcceptanceKey{}, err
	}
	return p.Pending, nil
}
