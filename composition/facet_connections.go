package composition

import (
	"context"
	"sync"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
)

// LocalFacetConnections is an address-only local assembly. It owns no contacts.
type LocalFacetConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]*facet.Instance
}

func (c *LocalFacetConnections) BindEndpoint(address values.NextHop, peer *facet.Instance) error {
	if c == nil || !address.IsLocal() || peer == nil || !peer.AuthorityRef().Valid() {
		return facetcontract.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.endpoints == nil {
		c.endpoints = map[values.NextHop]*facet.Instance{}
	}
	c.endpoints[address] = peer
	return nil
}
func (c *LocalFacetConnections) UnbindEndpoint(address values.NextHop) error {
	if c == nil || !address.IsLocal() {
		return facetcontract.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.endpoints, address)
	return nil
}

// Connect captures the specified local owner; no fallback or Ref lookup exists.
// FactAuthorityRef is immutable for a Facets instance, including after Close.
func (c *LocalFacetConnections) Connect(address values.NextHop, expected values.FactAuthorityRef) (*facet.Instance, error) {
	if c == nil || !address.IsLocal() || !expected.Valid() {
		return nil, facetcontract.ErrInvalid
	}
	c.mu.RLock()
	peer := c.endpoints[address]
	c.mu.RUnlock()
	if peer == nil || peer.AuthorityRef() != expected {
		return nil, facetcontract.ErrUnavailable
	}
	return peer, nil
}

type facetAcceptanceSource struct {
	connections *LocalFacetConnections
	actor       values.PrincipalRef
}

func (c *LocalFacetConnections) ForCore(actor values.PrincipalRef) (corecontract.FacetAcceptanceSource, error) {
	if c == nil || !actor.Valid() {
		return nil, facetcontract.ErrInvalid
	}
	return facetAcceptanceSource{c, actor}, nil
}
func (s facetAcceptanceSource) ReadFacetAcceptance(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
	if ctx.Err() != nil {
		return corecontract.FacetAcceptanceEvidence{}, corecontract.ErrUnavailable
	}
	peer, err := s.connections.Connect(address, key.Facet)
	if err != nil {
		return corecontract.FacetAcceptanceEvidence{}, corecontract.ErrUnavailable
	}
	r, err := peer.Namespace.ReadAcceptance(s.actor, key)
	if err != nil || ctx.Err() != nil || !r.Cut.Valid() {
		return corecontract.FacetAcceptanceEvidence{}, corecontract.ErrUnavailable
	}
	if !r.Found {
		return corecontract.FacetAcceptanceEvidence{}, nil
	}
	if r.Acceptance.Key != key || !r.Acceptance.AcceptedAt.Valid() {
		return corecontract.FacetAcceptanceEvidence{}, corecontract.ErrUnavailable
	}
	return corecontract.FacetAcceptanceEvidence{Key: key, AcceptedAt: r.Acceptance.AcceptedAt}, nil
}

type facetDispositionSource struct {
	connections *LocalFacetConnections
	caller      values.CallerContext
}

// ForCoreCaller fixes the Core's independently established service caller for
// final-disposition reads; client requests can never replace it.
func (c *LocalFacetConnections) ForCoreCaller(caller values.CallerContext) (corecontract.FacetDispositionSource, error) {
	if c == nil || !caller.Subject.Valid() || caller.Proof == ([32]byte{}) {
		return nil, corecontract.ErrInvalid
	}
	return facetDispositionSource{c, caller}, nil
}

func (s facetDispositionSource) ReadFacetDisposition(ctx context.Context, address values.NextHop, facet values.FactAuthorityRef, element values.ElementID) (corecontract.FacetDispositionRead, error) {
	if ctx == nil || ctx.Err() != nil {
		return corecontract.FacetDispositionRead{}, corecontract.ErrUnavailable
	}
	if !facet.Valid() || !element.Valid() {
		return corecontract.FacetDispositionRead{}, corecontract.ErrInvalid
	}
	peer, err := s.connections.Connect(address, facet)
	if err != nil {
		return corecontract.FacetDispositionRead{}, corecontract.ErrUnavailable
	}
	r, err := peer.Namespace.ReadDispositionControl(ctx, s.caller, element)
	if err != nil || ctx.Err() != nil {
		return corecontract.FacetDispositionRead{}, corecontract.ErrUnavailable
	}
	if !r.Found {
		// A proven exact absence is the zero read; the Core's release decision
		// treats any non-zero shape without a found disposition as malformed.
		return corecontract.FacetDispositionRead{}, nil
	}
	if !r.Key.Valid() || !r.Cut.Valid() || r.Key.Facet != facet || r.Key.Element != element {
		return corecontract.FacetDispositionRead{}, corecontract.ErrUnavailable
	}
	return corecontract.FacetDispositionRead{Found: true, Facet: facet, Element: element, Key: r.Key, Cut: r.Cut}, nil
}
