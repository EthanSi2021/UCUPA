package composition

import (
	"context"
	"sync"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
)

type FacetCorePort interface {
	AuthorityRef() values.FactAuthorityRef
	ReadFacetProposal(values.PrincipalRef, values.ElementID) (corecontract.FacetProposalRead, error)
	ReadFacetAttachment(values.PrincipalRef, values.ElementID) (corecontract.FacetAttachmentRead, error)
	ReadFacetReplacementRecord(corecontract.OperationID, values.ElementID) (corecontract.FacetReplacementRecord, error)
	ReadFacetSuccessor(values.PrincipalRef, values.ElementID) (corecontract.FacetSuccessorRead, error)
}

// LocalFacetCoreEndpoint captures the relationship read and optional
// observation-control/public-access entries of one Core. Control and Access
// are one service pair: both are present or both are absent. Rebinding replaces
// the whole endpoint atomically.
type LocalFacetCoreEndpoint struct {
	Read    FacetCorePort
	Control FacetCoreControlPort
	Access  FacetCoreAccessPort
}

type LocalFacetCoreConnections struct {
	mu        sync.RWMutex
	endpoints map[values.NextHop]LocalFacetCoreEndpoint
}

func (c *LocalFacetCoreConnections) BindEndpoint(address values.NextHop, endpoint LocalFacetCoreEndpoint) error {
	if c == nil || !address.IsLocal() || nilCorePort(endpoint.Read) || !endpoint.Read.AuthorityRef().Valid() {
		return facetcontract.ErrInvalid
	}
	if endpoint.Control != nil || endpoint.Access != nil {
		if endpoint.Control == nil || endpoint.Access == nil || nilCorePort(endpoint.Control) || nilCorePort(endpoint.Access) ||
			endpoint.Control.AuthorityRef() != endpoint.Read.AuthorityRef() || endpoint.Access.AuthorityRef() != endpoint.Read.AuthorityRef() {
			return facetcontract.ErrInvalid
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.endpoints == nil {
		c.endpoints = map[values.NextHop]LocalFacetCoreEndpoint{}
	}
	c.endpoints[address] = endpoint
	return nil
}
func (c *LocalFacetCoreConnections) UnbindEndpoint(address values.NextHop) error {
	if c == nil || !address.IsLocal() {
		return facetcontract.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.endpoints, address)
	return nil
}
func (c *LocalFacetCoreConnections) ForFacet(actor values.PrincipalRef) (facetcontract.CoreSource, error) {
	if c == nil || !actor.Valid() {
		return nil, facetcontract.ErrInvalid
	}
	return facetCoreSource{connections: c, actor: actor}, nil
}

// ForFacetCaller fixes the independently established service caller. Client
// requests can never replace it with their own or another principal.
func (c *LocalFacetCoreConnections) ForFacetCaller(caller values.CallerContext) (facetcontract.CoreSource, error) {
	if c == nil || !caller.Subject.Valid() || caller.Proof == ([32]byte{}) {
		return nil, facetcontract.ErrInvalid
	}
	return facetCoreSource{connections: c, actor: caller.Subject, caller: caller}, nil
}

type facetCoreSource struct {
	connections *LocalFacetCoreConnections
	actor       values.PrincipalRef
	caller      values.CallerContext
}

func (s facetCoreSource) peer(address values.NextHop, key values.FacetAcceptanceKey) (FacetCorePort, error) {
	if !address.IsLocal() || !key.Valid() {
		return nil, facetcontract.ErrInvalid
	}
	s.connections.mu.RLock()
	peer := s.connections.endpoints[address].Read
	s.connections.mu.RUnlock()
	if peer == nil || peer.AuthorityRef() != key.Core {
		return nil, facetcontract.ErrUnavailable
	}
	return peer, nil
}
func (s facetCoreSource) ReadProposal(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	if ctx.Err() != nil {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	peer, err := s.peer(address, key)
	if err != nil {
		return facetcontract.CoreEvidence{}, err
	}
	r, err := peer.ReadFacetProposal(s.actor, key.Element)
	if err != nil || ctx.Err() != nil || peer.AuthorityRef() != key.Core || r.Authority != key.Core || r.Element != key.Element {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	e, err := projectFacetCore(r.Authority, r.Element, r.State, r.Attachment, r.Cut)
	if err != nil {
		return facetcontract.CoreEvidence{}, err
	}
	e.Pending = r.Pending
	return e, nil
}
func (s facetCoreSource) ReadAttachment(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	if ctx.Err() != nil {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	peer, err := s.peer(address, key)
	if err != nil {
		return facetcontract.CoreEvidence{}, err
	}
	r, err := peer.ReadFacetAttachment(s.actor, key.Element)
	if err != nil || ctx.Err() != nil || peer.AuthorityRef() != key.Core || r.Authority != key.Core || r.Element != key.Element {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	return projectFacetCore(r.Authority, r.Element, r.State, r.Attachment, r.Cut)
}

// ReadReplacement discloses exactly the named durable replacement control
// record of the expected Core. It is the trusted local service read behind the
// Facet's fixed service identity; client requests never reach it directly.
func (s facetCoreSource) ReadReplacement(ctx context.Context, address values.NextHop, core values.FactAuthorityRef, element values.ElementID, op facetcontract.OperationID) (facetcontract.CoreReplacementRead, error) {
	if ctx == nil || ctx.Err() != nil {
		return facetcontract.CoreReplacementRead{}, facetcontract.ErrUnavailable
	}
	if !address.IsLocal() || !core.Valid() || !element.Valid() || len(op) == 0 || len(op) > 128 {
		return facetcontract.CoreReplacementRead{}, facetcontract.ErrInvalid
	}
	s.connections.mu.RLock()
	peer := s.connections.endpoints[address].Read
	s.connections.mu.RUnlock()
	if peer == nil || peer.AuthorityRef() != core {
		return facetcontract.CoreReplacementRead{}, facetcontract.ErrUnavailable
	}
	r, err := peer.ReadFacetReplacementRecord(corecontract.OperationID(op), element)
	if err != nil || ctx.Err() != nil || peer.AuthorityRef() != core || r.Authority != core || r.Element != element || r.Operation != corecontract.OperationID(op) {
		return facetcontract.CoreReplacementRead{}, facetcontract.ErrUnavailable
	}
	return facetcontract.CoreReplacementRead{
		Authority: r.Authority, Element: r.Element, Operation: op,
		Source: r.Source, Target: r.Target, Proposal: r.Proposal,
		FinalCut: r.FinalCut, ManifestDigest: r.ManifestDigest,
		Permitted: r.Permitted, Fenced: r.Fenced, Accepted: r.Accepted,
		Committed: r.Committed, Settled: r.Settled, Cancelled: r.Cancelled,
	}, nil
}

// ReadSuccessor discloses the expected Core's durable successor statement for
// one departed element. It is the trusted local service read behind the
// Facet's fixed service identity; the proven absence is returned as a value.
func (s facetCoreSource) ReadSuccessor(ctx context.Context, address values.NextHop, core values.FactAuthorityRef, element values.ElementID) (facetcontract.CoreSuccessorRead, error) {
	if ctx == nil || ctx.Err() != nil {
		return facetcontract.CoreSuccessorRead{}, facetcontract.ErrUnavailable
	}
	if !address.IsLocal() || !core.Valid() || !element.Valid() {
		return facetcontract.CoreSuccessorRead{}, facetcontract.ErrInvalid
	}
	s.connections.mu.RLock()
	peer := s.connections.endpoints[address].Read
	s.connections.mu.RUnlock()
	if peer == nil || peer.AuthorityRef() != core {
		return facetcontract.CoreSuccessorRead{}, facetcontract.ErrUnavailable
	}
	r, err := peer.ReadFacetSuccessor(s.actor, element)
	if err != nil || ctx.Err() != nil || peer.AuthorityRef() != core || r.Authority != core || r.Element != element {
		return facetcontract.CoreSuccessorRead{}, facetcontract.ErrUnavailable
	}
	out := facetcontract.CoreSuccessorRead{Found: r.State == corecontract.ExistencePresent, Authority: r.Authority, Element: r.Element,
		Source: r.Source, Target: r.Target, FinalCut: r.FinalCut, Settled: r.Settled}
	if out.Found {
		out.Operation = facetcontract.OperationID(r.Operation)
	}
	return out, nil
}

// projectFacetCore maps the Core's relationship disclosure into the Facet's
// evidence shapes. A RETIRED slot is outside every Facet decision shape: the
// Facet disposed of that namespace first, so no evidence-based transition can
// act on it and the read stays safely refused.
func projectFacetCore(owner values.FactAuthorityRef, id values.ElementID, state corecontract.ExistenceState, a corecontract.FacetAttachment, cut corecontract.CoreReadCut) (facetcontract.CoreEvidence, error) {
	e := facetcontract.CoreEvidence{Authority: owner, Element: id, Cut: cut.Evidence()}
	if state == corecontract.ExistenceNotFound {
		e.State = facetcontract.CoreMissing
		return e, nil
	}
	if state != corecontract.ExistencePresent {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	e.AttachmentRevision = a.Revision
	e.Active = a.Key
	switch a.State {
	case corecontract.FacetNone:
		e.State = facetcontract.CoreNone
	case corecontract.FacetActive:
		e.State = facetcontract.CoreActive
	case corecontract.FacetDetached:
		e.State = facetcontract.CoreDetached
	case corecontract.FacetRetired:
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	default:
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	return e, nil
}
