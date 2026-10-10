package acceptance_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
)

func TestP4FacetGrantsStayWithExactOwnerAndScopeKind(t *testing.T) {
	w := newFacetWorld(t)
	const id values.ElementID = "00000000-0000-4000-8000-00000000fa91"
	w.root(t, 0, id)
	key := w.attach(t, 0, 0, id)
	if _, err := w.facets[0].Namespace.Put(context.Background(), w.actor, key, fixtureType("content"), fixturePayload("secret"), facetcontract.ExpectAbsent(), "put"); err != nil {
		t.Fatal(err)
	}
	other := key
	other.Facet = w.facets[1].AuthorityRef()
	if _, err := w.facets[1].Namespace.Get(context.Background(), w.actor, other, fixtureType("content")); !errors.Is(err, facetcontract.ErrDenied) {
		t.Fatalf("owner grant transferred: %v", err)
	}
	w.grant(t, w.actor, facetcontract.PermissionGet, values.FacetElementScope(other.Facet, id))
	if _, err := w.facets[1].Namespace.Get(context.Background(), w.actor, other, fixtureType("content")); !errors.Is(err, facetcontract.ErrDenied) {
		t.Fatalf("namespace expanded to key grant: %v", err)
	}
	w.grant(t, w.actor, facetcontract.PermissionGet, values.FacetKeyScope(other.Facet, id, fixtureType("content")))
	if _, err := w.facets[1].Namespace.Get(context.Background(), w.actor, other, fixtureType("content")); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("invented acceptance: %v", err)
	}
}
