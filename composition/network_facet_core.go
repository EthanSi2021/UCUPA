package composition

import (
	"bytes"
	"context"
	"encoding/binary"

	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
	facetcontract "ucupa/ucupa-facet/contract"
)

// FacetCoreEndpoint is installed by trusted host configuration. Client owns a
// target-local fixed service session, never the content request's credentials.
type FacetCoreEndpoint struct {
	Address     values.NextHop
	Core        values.FactAuthorityRef
	Client      *coresdk.Client
	Credentials CoreServiceCredentials
}

// CoreServiceCredentials supplies a fixed service's local secret material.
// Implementations belong to trusted host configuration, not content requests.
type CoreServiceCredentials interface {
	Evidence() (values.RealmID, [16]byte, [32]byte, error)
}

type NetworkFacetCoreSource struct {
	facet values.FactAuthorityRef
	peers map[values.NextHop]FacetCoreEndpoint
}

func NewNetworkFacetCoreSource(owner values.FactAuthorityRef, endpoints []FacetCoreEndpoint) (*NetworkFacetCoreSource, error) {
	if !owner.Valid() {
		return nil, facetcontract.ErrInvalid
	}
	s := &NetworkFacetCoreSource{facet: owner, peers: make(map[values.NextHop]FacetCoreEndpoint, len(endpoints))}
	for _, e := range endpoints {
		_, network := e.Address.NetworkTarget()
		if !network || !e.Core.Valid() || e.Client == nil || s.peers[e.Address].Client != nil {
			return nil, facetcontract.ErrInvalid
		}
		actualAddress, actualCore := e.Client.Target()
		if actualAddress != e.Address || actualCore != e.Core {
			return nil, facetcontract.ErrInvalid
		}
		s.peers[e.Address] = e
	}
	return s, nil
}

func (s *NetworkFacetCoreSource) peer(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (*coresdk.Client, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || !key.Valid() || key.Facet != s.facet {
		return nil, values.PublicUnavailable
	}
	return s.corePeer(ctx, address, key.Core)
}

func (s *NetworkFacetCoreSource) corePeer(ctx context.Context, address values.NextHop, owner values.FactAuthorityRef) (*coresdk.Client, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || !owner.Valid() {
		return nil, values.PublicUnavailable
	}
	e := s.peers[address]
	if e.Client == nil || e.Core != owner {
		return nil, values.PublicUnavailable
	}
	if e.Credentials != nil {
		audience, id, secret, err := e.Credentials.Evidence()
		if err != nil {
			return nil, values.PublicUnavailable
		}
		err = e.Client.EnsureSession(ctx, audience, id, secret)
		clear(secret[:])
		if err != nil {
			return nil, values.PublicUnavailable
		}
	}
	return e.Client, nil
}

func networkServiceKey(k values.FacetAcceptanceKey) *corepb.ServiceAttachmentKey {
	b := k.Proposal.Bytes()
	return &corepb.ServiceAttachmentKey{Core: proto.String(string(k.Core)), Facet: proto.String(string(k.Facet)), Element: proto.String(string(k.Element)), Proposal: &corepb.ServiceRevision{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[68:]))}}
}

func localServiceRevision(r *corepb.ServiceRevision, element values.ElementID) (values.AttachmentRevision, error) {
	if r == nil || len(r.Incarnation) != 32 || r.GetSequence() == 0 || !element.Valid() {
		return values.AttachmentRevision{}, values.PublicUnavailable
	}
	b := make([]byte, 76)
	copy(b, r.Incarnation)
	copy(b[32:68], element)
	binary.BigEndian.PutUint64(b[68:], r.GetSequence())
	return values.NewAttachmentRevision(b)
}

func localServiceKey(k *corepb.ServiceAttachmentKey) (values.FacetAcceptanceKey, error) {
	if k == nil {
		return values.FacetAcceptanceKey{}, nil
	}
	r, err := localServiceRevision(k.Proposal, values.ElementID(k.GetElement()))
	out := values.FacetAcceptanceKey{Core: values.FactAuthorityRef(k.GetCore()), Facet: values.FactAuthorityRef(k.GetFacet()), Element: values.ElementID(k.GetElement()), Proposal: r}
	if err != nil || !out.Valid() {
		return values.FacetAcceptanceKey{}, values.PublicUnavailable
	}
	return out, nil
}

func localControlToken(t *networkpb.Token, owner values.FactAuthorityRef, kind string) (values.PublicToken, error) {
	if t == nil {
		return values.PublicToken{}, nil
	}
	if t.GetOwner() != string(owner) || len(t.Value) != 32 {
		return values.PublicToken{}, values.PublicUnavailable
	}
	return values.PublicTokenFromBytes(owner, kind, [32]byte(t.Value))
}

func (s *NetworkFacetCoreSource) read(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, prepare, proposal bool) (facetcontract.CoreControlRead, error) {
	client, err := s.peer(ctx, address, key)
	if err != nil {
		return facetcontract.CoreControlRead{}, err
	}
	request := &corepb.ReadControlRequest{Key: networkServiceKey(key), Prepare: proto.Bool(prepare)}
	var reply *corepb.ReadControlResponse
	if proposal {
		reply, err = client.AttachmentControl().ReadProposalControl(ctx, request)
	} else {
		reply, err = client.AttachmentControl().ReadControl(ctx, request)
	}
	if err != nil || reply == nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	return decodeNetworkCoreControl(reply, key, prepare, proposal)
}

func decodeNetworkCoreControl(reply *corepb.ReadControlResponse, key values.FacetAcceptanceKey, prepare, proposal bool) (facetcontract.CoreControlRead, error) {
	if reply.GetFailure() != 0 {
		return facetcontract.CoreControlRead{}, values.PublicError(reply.GetFailure().String())
	}
	r := reply.GetResult()
	if r == nil || r.GetCore() != string(key.Core) || r.GetElement() != string(key.Element) || r.Cut == nil || len(r.Cut.Incarnation) != 32 || (r.Preparation != nil) != prepare || !proposal && r.Pending != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	var err error
	b := make([]byte, 40)
	copy(b, r.Cut.Incarnation)
	binary.BigEndian.PutUint64(b[32:], r.Cut.GetSequence())
	cut, err := values.NewCoreCutEvidence(b)
	if err != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	out := facetcontract.CoreControlRead{Evidence: facetcontract.CoreEvidence{Authority: key.Core, Element: key.Element, Cut: cut}}
	e := &out.Evidence
	if r.GetPresence() == corepb.Presence_NOT_FOUND {
		e.State = facetcontract.CoreMissing
	} else {
		e.AttachmentRevision, err = localServiceRevision(r.Revision, key.Element)
		if err != nil {
			return facetcontract.CoreControlRead{}, values.PublicUnavailable
		}
		switch r.GetState() {
		case corepb.AttachmentState_NONE:
			e.State = facetcontract.CoreNone
		case corepb.AttachmentState_ACTIVE:
			e.State = facetcontract.CoreActive
		case corepb.AttachmentState_DETACHED:
			e.State = facetcontract.CoreDetached
		default:
			return facetcontract.CoreControlRead{}, values.PublicUnavailable
		}
	}
	e.Active, err = localServiceKey(r.Active)
	if err != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	e.Pending, err = localServiceKey(r.Pending)
	if err != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	// A transferred element's new Core discloses the previous Core's predecessor
	// key next to its own pending proposal. Only that proven DETACHED predecessor
	// shape may carry a foreign Active key; every other shape keeps the strict
	// single-Core rule for both disclosed keys.
	predecessor := proposal && e.State == facetcontract.CoreDetached && e.Pending == key &&
		e.Active != (values.FacetAcceptanceKey{}) && e.Active.Core != key.Core &&
		e.Active.Facet == key.Facet && e.Active.Element == key.Element
	for _, k := range []values.FacetAcceptanceKey{e.Active, e.Pending} {
		if k == (values.FacetAcceptanceKey{}) {
			continue
		}
		if k.Core != key.Core || k.Element != key.Element {
			if predecessor && k == e.Active {
				continue
			}
			return facetcontract.CoreControlRead{}, values.PublicUnavailable
		}
	}
	out.Proposal, err = localControlToken(r.Proposal, key.Core, "attachment-revision")
	if err != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	out.Preparation, err = localControlToken(r.Preparation, key.Core, "observation-preparation")
	if err != nil {
		return facetcontract.CoreControlRead{}, values.PublicUnavailable
	}
	return out, nil
}

// ResolveProposal starts with the caller-visible proposal and the relationship's
// specified address. The receiving Core derives the private key for this fixed
// Facet service; no public token is decoded into service evidence locally.
func (s *NetworkFacetCoreSource) ResolveProposal(ctx context.Context, address values.NextHop, ref facetcontract.PublicAcceptanceRef) (facetcontract.CoreProposalRead, error) {
	client, err := s.corePeer(ctx, address, ref.Core)
	if err != nil {
		return facetcontract.CoreProposalRead{}, err
	}
	b, err := values.DecodePublicBytes(ref.Proposal.Value)
	if err != nil || len(b) != 32 || !ref.Element.Valid() || ref.Proposal.Owner != ref.Core || ref.Proposal.Kind != "attachment-revision" {
		return facetcontract.CoreProposalRead{}, values.PublicInvalid
	}
	r, err := client.AttachmentControl().ResolveProposalControl(ctx, &corepb.ResolveProposalControlRequest{Element: proto.String(string(ref.Element)), Proposal: &networkpb.Token{Owner: proto.String(string(ref.Core)), Kind: networkpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: b}})
	if err != nil || r == nil {
		return facetcontract.CoreProposalRead{}, values.PublicUnavailable
	}
	if r.GetFailure() != 0 {
		return facetcontract.CoreProposalRead{}, values.PublicError(r.GetFailure().String())
	}
	k, err := localServiceKey(r.GetResult().GetPending())
	if err != nil || !k.Valid() || k.Core != ref.Core || k.Facet != s.facet || k.Element != ref.Element {
		return facetcontract.CoreProposalRead{}, values.PublicUnavailable
	}
	read, err := decodeNetworkCoreControl(r, k, false, true)
	if err != nil || read.Proposal != ref.Proposal {
		return facetcontract.CoreProposalRead{}, values.PublicUnavailable
	}
	return facetcontract.CoreProposalRead{Key: k, Evidence: read.Evidence, Proposal: read.Proposal}, nil
}

func (s *NetworkFacetCoreSource) ReadProposal(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	r, err := s.read(ctx, address, key, false, true)
	return r.Evidence, err
}
func (s *NetworkFacetCoreSource) ReadAttachment(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	r, err := s.read(ctx, address, key, false, false)
	return r.Evidence, err
}
func (s *NetworkFacetCoreSource) ReadControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, prepare bool) (facetcontract.CoreControlRead, error) {
	return s.read(ctx, address, key, prepare, false)
}
func (s *NetworkFacetCoreSource) CheckControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, preparation values.PublicToken) error {
	c, err := s.peer(ctx, address, key)
	if err != nil {
		return err
	}
	b, err := values.DecodePublicBytes(preparation.Value)
	if err != nil || len(b) != 32 || preparation.Kind != "observation-preparation" || preparation.Owner != key.Core {
		return values.PublicInvalid
	}
	r, err := c.AttachmentControl().CheckControl(ctx, &corepb.CheckControlRequest{Key: networkServiceKey(key), Preparation: &networkpb.Token{Owner: proto.String(string(key.Core)), Kind: networkpb.TokenKind_OBSERVATION_PREPARATION.Enum(), Value: b}})
	if err != nil || r == nil {
		return values.PublicUnavailable
	}
	if r.GetFailure() != 0 {
		return values.PublicError(r.GetFailure().String())
	}
	if r.GetResult() == nil || !r.GetResult().GetValid() {
		return values.PublicUnavailable
	}
	return nil
}

// ReadReplacement discloses exactly the named durable replacement control
// record of the expected Core. It is the trusted fixed-service read behind the
// Facet's own service identity; client requests never reach it, and the
// controlling Core's transport confines the record to a named party.
func (s *NetworkFacetCoreSource) ReadReplacement(ctx context.Context, address values.NextHop, core values.FactAuthorityRef, element values.ElementID, op facetcontract.OperationID) (facetcontract.CoreReplacementRead, error) {
	if ctx == nil || ctx.Err() != nil || !core.Valid() || !element.Valid() || len(op) == 0 || len(op) > 128 {
		return facetcontract.CoreReplacementRead{}, values.PublicInvalid
	}
	client, err := s.corePeer(ctx, address, core)
	if err != nil {
		return facetcontract.CoreReplacementRead{}, err
	}
	reply, err := client.AttachmentControl().ReadReplacementControl(ctx, &corepb.ReadReplacementControlRequest{OperationId: []byte(op), Element: proto.String(string(element))})
	if err != nil || reply == nil {
		return facetcontract.CoreReplacementRead{}, values.PublicUnavailable
	}
	if reply.GetFailure() != 0 {
		return facetcontract.CoreReplacementRead{}, values.PublicError(reply.GetFailure().String())
	}
	r := reply.GetResult()
	if r == nil || r.GetElement() != string(element) || !bytes.Equal(r.GetOperationId(), []byte(op)) {
		return facetcontract.CoreReplacementRead{}, values.PublicUnavailable
	}
	proposal, err := localServiceRevision(r.Proposal, element)
	if err != nil {
		return facetcontract.CoreReplacementRead{}, values.PublicUnavailable
	}
	out := facetcontract.CoreReplacementRead{
		Authority: core, Element: element, Operation: op,
		Source: values.FactAuthorityRef(r.GetSource()), Target: values.FactAuthorityRef(r.GetTarget()),
		Proposal: proposal, FinalCut: r.GetFinalCut(),
		Permitted: r.GetPermitted(), Fenced: r.GetFenced(), Accepted: r.GetAccepted(),
		Committed: r.GetCommitted(), Settled: r.GetSettled(), Cancelled: r.GetCancelled(),
	}
	if !out.Source.Valid() || !out.Target.Valid() || (len(r.GetManifestDigest()) != 0 && len(r.GetManifestDigest()) != 32) {
		return facetcontract.CoreReplacementRead{}, values.PublicUnavailable
	}
	if len(r.GetManifestDigest()) == 32 {
		copy(out.ManifestDigest[:], r.GetManifestDigest())
	}
	return out, nil
}

// ReadSuccessor discloses the expected Core's durable successor statement for
// one departed element over the fixed service session. It is the trusted
// Facet-side read behind the Facet's own service identity; a proven absence
// is returned as a value, never as an error.
func (s *NetworkFacetCoreSource) ReadSuccessor(ctx context.Context, address values.NextHop, core values.FactAuthorityRef, element values.ElementID) (facetcontract.CoreSuccessorRead, error) {
	if ctx == nil || ctx.Err() != nil || !core.Valid() || !element.Valid() {
		return facetcontract.CoreSuccessorRead{}, values.PublicInvalid
	}
	client, err := s.corePeer(ctx, address, core)
	if err != nil {
		return facetcontract.CoreSuccessorRead{}, err
	}
	reply, err := client.AttachmentControl().ReadSuccessorControl(ctx, &corepb.ReadSuccessorControlRequest{Element: proto.String(string(element))})
	if err != nil || reply == nil {
		return facetcontract.CoreSuccessorRead{}, values.PublicUnavailable
	}
	if reply.GetFailure() != 0 {
		return facetcontract.CoreSuccessorRead{}, values.PublicError(reply.GetFailure().String())
	}
	r := reply.GetResult()
	if r == nil || r.GetElement() != string(element) {
		return facetcontract.CoreSuccessorRead{}, values.PublicUnavailable
	}
	out := facetcontract.CoreSuccessorRead{Found: r.GetFound(), Authority: core, Element: element, Settled: r.GetSettled(),
		Source: values.FactAuthorityRef(r.GetSource()), Target: values.FactAuthorityRef(r.GetTarget()), FinalCut: r.GetFinalCut()}
	if out.Found {
		out.Operation = facetcontract.OperationID(r.GetOperationId())
		if len(out.Operation) == 0 || len(out.Operation) > 128 || !out.Source.Valid() || !out.Target.Valid() || out.FinalCut == 0 {
			return facetcontract.CoreSuccessorRead{}, values.PublicUnavailable
		}
	}
	return out, nil
}
