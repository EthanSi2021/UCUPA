package composition

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
)

// FacetCoreControlPort is the optional current-Core observation service entry.
type FacetCoreControlPort interface {
	AuthorityRef() values.FactAuthorityRef
	ReadAttachmentControl(context.Context, values.CallerContext, values.FacetAcceptanceKey, bool) (corecontract.AttachmentControlRead, error)
	CheckAttachmentControl(context.Context, values.CallerContext, values.FacetAcceptanceKey, values.PublicToken) error
}

// FacetCoreAccessPort is the matching public-envelope entry. It remains
// separate from relationship control even when both belong to one Core.
type FacetCoreAccessPort interface {
	AuthorityRef() values.FactAuthorityRef
	PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error)
}

func (s facetCoreSource) CheckControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, preparation values.PublicToken) error {
	if ctx == nil || ctx.Err() != nil {
		return values.PublicUnavailable
	}
	endpoint, err := s.controlPeer(address, key)
	if err != nil {
		return values.PublicUnavailable
	}
	port := endpoint.Control
	err = port.CheckAttachmentControl(ctx, s.caller, key, preparation)
	if !sameFacetCoreService(endpoint, key.Core) {
		return values.PublicUnavailable
	}
	return err
}

func (s facetCoreSource) ReadControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, prepare bool) (facetcontract.CoreControlRead, error) {
	if ctx == nil || ctx.Err() != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	endpoint, err := s.controlPeer(address, key)
	if err != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	port := endpoint.Control
	read, err := port.ReadAttachmentControl(ctx, s.caller, key, prepare)
	if !sameFacetCoreService(endpoint, key.Core) {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	if err != nil {
		return facetcontract.CoreControlRead{}, err
	}
	if read.Read.Authority != key.Core || read.Read.Element != key.Element {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	e := read.Read
	evidence, err := projectFacetCore(e.Authority, e.Element, e.State, e.Attachment, e.Cut)
	if err != nil {
		return facetcontract.CoreControlRead{}, err
	}
	return facetcontract.CoreControlRead{Evidence: evidence, Proposal: read.Proposal, Preparation: read.Preparation}, nil
}

func (s facetCoreSource) ControlCall(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, request []byte) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	endpoint, err := s.controlPeer(address, key)
	if err != nil {
		return nil, values.PublicUnavailable
	}
	port := endpoint.Access
	raw, err := port.PublicCall(ctx, s.caller, request)
	if !sameFacetCoreService(endpoint, key.Core) {
		return nil, values.PublicUnavailable
	}
	return raw, err
}

func (s facetCoreSource) controlPeer(address values.NextHop, key values.FacetAcceptanceKey) (LocalFacetCoreEndpoint, error) {
	if !address.IsLocal() || !key.Valid() {
		return LocalFacetCoreEndpoint{}, facetcontract.ErrInvalid
	}
	s.connections.mu.RLock()
	endpoint := s.connections.endpoints[address]
	s.connections.mu.RUnlock()
	if !sameFacetCoreService(endpoint, key.Core) {
		return LocalFacetCoreEndpoint{}, values.PublicUnavailable
	}
	return endpoint, nil
}

func sameFacetCoreService(endpoint LocalFacetCoreEndpoint, owner values.FactAuthorityRef) bool {
	return !nilCorePort(endpoint.Read) && endpoint.Read.AuthorityRef() == owner &&
		!nilCorePort(endpoint.Control) && endpoint.Control.AuthorityRef() == owner &&
		!nilCorePort(endpoint.Access) && endpoint.Access.AuthorityRef() == owner
}
