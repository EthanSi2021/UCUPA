package composition

import (
	"errors"
	"testing"
	"ucupa/shared/values"
)

func TestLocalAdaptersCannotBindNetworkAddresses(t *testing.T) {
	// Nil peers cannot establish bindings, regardless of addresses. Connect also
	// rejects network input before lookup, never treating it as a local alias.
	hop := values.NextHop("https://localhost:443")
	owner := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001")
	if !hop.Valid() {
		t.Fatal("fixture must be a valid network address")
	}
	if _, err := (&LocalCoreConnections{}).ReadElement(hop, owner, values.PrincipalRef{}, values.ElementID(owner), nil); !errors.Is(err, ErrInvalidCoreEndpoint) {
		t.Fatal("Core network input accepted locally")
	}
	if _, err := (&LocalFacetConnections{}).Connect(hop, owner); err == nil {
		t.Fatal("Facet network input accepted locally")
	}
}
