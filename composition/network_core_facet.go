package composition

import (
	"context"
	"encoding/binary"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
	service "ucupa/ucupa-facet/sdk/go"
)

type CoreFacetEndpoint struct {
	Address     values.NextHop
	Facet       values.FactAuthorityRef
	Client      *service.Client
	Credentials CoreServiceCredentials
}

// NetworkCoreFacetSource contacts only the current relationship's exact address
// and independently authenticates the fixed Core service in the target Realm.
type NetworkCoreFacetSource struct {
	core  values.FactAuthorityRef
	peers map[values.NextHop]CoreFacetEndpoint
}

func NewNetworkCoreFacetSource(owner values.FactAuthorityRef, endpoints []CoreFacetEndpoint) (*NetworkCoreFacetSource, error) {
	if !owner.Valid() {
		return nil, values.PublicInvalid
	}
	s := &NetworkCoreFacetSource{core: owner, peers: make(map[values.NextHop]CoreFacetEndpoint, len(endpoints))}
	for _, e := range endpoints {
		_, network := e.Address.NetworkTarget()
		if !network || !e.Facet.Valid() || e.Client == nil || s.peers[e.Address].Client != nil {
			return nil, values.PublicInvalid
		}
		hop, ref := e.Client.Target()
		if hop != e.Address || ref != e.Facet {
			return nil, values.PublicInvalid
		}
		s.peers[e.Address] = e
	}
	return s, nil
}
func (s *NetworkCoreFacetSource) ReadFacetAcceptance(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (corecontract.FacetAcceptanceEvidence, error) {
	bad := values.PublicUnavailable
	if s == nil || ctx == nil || ctx.Err() != nil || !key.Valid() || key.Core != s.core {
		return corecontract.FacetAcceptanceEvidence{}, bad
	}
	e := s.peers[address]
	if e.Client == nil || e.Facet != key.Facet {
		return corecontract.FacetAcceptanceEvidence{}, bad
	}
	if e.Credentials != nil {
		a, id, secret, err := e.Credentials.Evidence()
		if err != nil {
			return corecontract.FacetAcceptanceEvidence{}, bad
		}
		err = e.Client.EnsureSession(ctx, a, id, secret)
		clear(secret[:])
		if err != nil {
			return corecontract.FacetAcceptanceEvidence{}, bad
		}
	}
	b := key.Proposal.Bytes()
	w := &pb.ServiceAttachmentKey{Core: proto.String(string(key.Core)), Facet: proto.String(string(key.Facet)), Element: proto.String(string(key.Element)), Proposal: &pb.ServiceRevision{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[68:]))}}
	r, err := e.Client.Acceptance().ReadAcceptance(ctx, &pb.ReadAcceptanceRequest{Key: w})
	if err != nil || r == nil {
		return corecontract.FacetAcceptanceEvidence{}, bad
	}
	if r.GetFailure() != 0 {
		return corecontract.FacetAcceptanceEvidence{}, values.PublicError(r.GetFailure().String())
	}
	x := r.GetResult()
	if x == nil || !proto.Equal(x.Key, w) || x.Cut == nil || len(x.Cut.Incarnation) != 32 {
		return corecontract.FacetAcceptanceEvidence{}, bad
	}
	if !x.GetFound() {
		if x.AcceptedAt != nil {
			return corecontract.FacetAcceptanceEvidence{}, bad
		}
		return corecontract.FacetAcceptanceEvidence{}, nil
	}
	if x.AcceptedAt == nil || len(x.AcceptedAt.Incarnation) != 32 || x.AcceptedAt.GetSequence() == 0 || string(x.Cut.Incarnation) != string(x.AcceptedAt.Incarnation) || x.AcceptedAt.GetSequence() > x.Cut.GetSequence() {
		return corecontract.FacetAcceptanceEvidence{}, bad
	}
	cutBytes := make([]byte, 40)
	copy(cutBytes, x.AcceptedAt.Incarnation)
	binary.BigEndian.PutUint64(cutBytes[32:], x.AcceptedAt.GetSequence())
	cut, err := values.NewFacetReadCut(cutBytes)
	if err != nil {
		return corecontract.FacetAcceptanceEvidence{}, bad
	}
	return corecontract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}, nil
}

// ReadFacetDisposition discloses one element's durable final disposition at the
// current relationship's exact Facet address. A proven absence is returned as a
// value, never as an error; only a failed or malformed read is unavailable.
func (s *NetworkCoreFacetSource) ReadFacetDisposition(ctx context.Context, address values.NextHop, facet values.FactAuthorityRef, element values.ElementID) (corecontract.FacetDispositionRead, error) {
	bad := values.PublicUnavailable
	if s == nil || ctx == nil || ctx.Err() != nil || !facet.Valid() || !element.Valid() {
		return corecontract.FacetDispositionRead{}, bad
	}
	e := s.peers[address]
	if e.Client == nil || e.Facet != facet {
		return corecontract.FacetDispositionRead{}, bad
	}
	if e.Credentials != nil {
		a, id, secret, err := e.Credentials.Evidence()
		if err != nil {
			return corecontract.FacetDispositionRead{}, bad
		}
		err = e.Client.EnsureSession(ctx, a, id, secret)
		clear(secret[:])
		if err != nil {
			return corecontract.FacetDispositionRead{}, bad
		}
	}
	r, err := e.Client.Acceptance().ReadDisposition(ctx, &pb.ReadDispositionRequest{Element: proto.String(string(element))})
	if err != nil || r == nil {
		return corecontract.FacetDispositionRead{}, bad
	}
	if r.GetFailure() != 0 {
		return corecontract.FacetDispositionRead{}, values.PublicError(r.GetFailure().String())
	}
	x := r.GetResult()
	if x == nil {
		return corecontract.FacetDispositionRead{}, bad
	}
	if !x.GetFound() {
		// A proven exact absence is the zero read; the Core's release decision
		// treats any non-zero shape without a found disposition as malformed.
		if x.Key != nil || x.Cut != nil {
			return corecontract.FacetDispositionRead{}, bad
		}
		return corecontract.FacetDispositionRead{}, nil
	}
	key, keyErr := facetServiceKey(x.Key)
	cut, cutErr := networkFacetCut(x.Cut)
	if keyErr != nil || cutErr != nil || !key.Valid() || key.Facet != facet || key.Element != element {
		return corecontract.FacetDispositionRead{}, bad
	}
	return corecontract.FacetDispositionRead{Found: true, Facet: facet, Element: element, Key: key, Cut: cut}, nil
}

func facetServiceKey(k *pb.ServiceAttachmentKey) (values.FacetAcceptanceKey, error) {
	if k == nil {
		return values.FacetAcceptanceKey{}, nil
	}
	if k.Proposal == nil || len(k.Proposal.Incarnation) != 32 || k.Proposal.GetSequence() == 0 || !values.ElementID(k.GetElement()).Valid() {
		return values.FacetAcceptanceKey{}, values.PublicUnavailable
	}
	b := make([]byte, 76)
	copy(b, k.Proposal.Incarnation)
	copy(b[32:68], k.GetElement())
	binary.BigEndian.PutUint64(b[68:], k.Proposal.GetSequence())
	r, err := values.NewAttachmentRevision(b)
	out := values.FacetAcceptanceKey{Core: values.FactAuthorityRef(k.GetCore()), Facet: values.FactAuthorityRef(k.GetFacet()), Element: values.ElementID(k.GetElement()), Proposal: r}
	if err != nil || !out.Valid() {
		return values.FacetAcceptanceKey{}, values.PublicUnavailable
	}
	return out, nil
}

func networkFacetCut(c *pb.ServiceFacetCut) (values.FacetReadCut, error) {
	if c == nil || len(c.Incarnation) != 32 || c.GetSequence() == 0 {
		return values.FacetReadCut{}, values.PublicUnavailable
	}
	b := make([]byte, 40)
	copy(b, c.Incarnation)
	binary.BigEndian.PutUint64(b[32:], c.GetSequence())
	return values.NewFacetReadCut(b)
}
