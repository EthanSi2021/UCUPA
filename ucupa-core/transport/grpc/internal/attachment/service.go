package attachment

import (
	"context"
	"encoding/binary"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// Owner is the narrow attachment-control port consumed by this RPC group.
// It is intentionally independent of the transport Server and its registries.
type Owner interface {
	ResolveProposalControl(context.Context, values.CallerContext, values.FactAuthorityRef, values.ElementID, values.PublicToken) (corecontract.FacetProposalRead, error)
	ReadProposalControl(context.Context, values.CallerContext, values.FacetAcceptanceKey) (corecontract.FacetProposalRead, error)
	ReadAttachmentControl(context.Context, values.CallerContext, values.FacetAcceptanceKey, bool) (corecontract.AttachmentControlRead, error)
	CheckAttachmentControl(context.Context, values.CallerContext, values.FacetAcceptanceKey, values.PublicToken) error
	ReadFacetReplacementRecord(corecontract.OperationID, values.ElementID) (corecontract.FacetReplacementRecord, error)
	ReadSuccessorControl(context.Context, values.CallerContext, values.ElementID) (corecontract.FacetSuccessorRead, error)
}

// Service owns only the fixed Facet service relation and attachment RPC
// encoding. Session/grant admission and lifecycle remain in the parent
// transport server and arrive through the private context boundary.
type Service struct {
	corepb.UnimplementedAttachmentControlServiceServer
	owner    Owner
	expected values.FactAuthorityRef
	services map[values.PrincipalRef]values.FactAuthorityRef
}

func New(owner Owner, expected values.FactAuthorityRef, services map[values.PrincipalRef]values.FactAuthorityRef) *Service {
	fixed := make(map[values.PrincipalRef]values.FactAuthorityRef, len(services))
	for principal, facet := range services {
		fixed[principal] = facet
	}
	return &Service{owner: owner, expected: expected, services: fixed}
}

func (s *Service) ResolveProposalControl(ctx context.Context, req *corepb.ResolveProposalControlRequest) (*corepb.ReadControlResponse, error) {
	caller, ok := grpcsession.Caller(ctx)
	facet := s.services[caller.Subject]
	if !ok || !facet.Valid() {
		return nil, status.Error(codes.PermissionDenied, "fixed Facet service required")
	}
	if _, delegated := grpcsession.Grant(ctx); delegated {
		return nil, status.Error(codes.PermissionDenied, "original service session required")
	}
	w := req.GetProposal()
	if w == nil || w.GetOwner() != string(s.expected) || w.GetKind() != networkpb.TokenKind_ATTACHMENT_REVISION || len(w.Value) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid proposal comparison")
	}
	p, err := values.PublicTokenFromBytes(s.expected, "attachment-revision", [32]byte(w.Value))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid proposal comparison")
	}
	r, err := s.owner.ResolveProposalControl(ctx, caller, facet, values.ElementID(req.GetElement()), p)
	if err != nil {
		f, e := rpcfailure.Control(err)
		return &corepb.ReadControlResponse{Outcome: &corepb.ReadControlResponse_Failure{Failure: f}}, e
	}
	read := corecontract.AttachmentControlRead{Read: corecontract.FacetAttachmentRead{Authority: r.Authority, Element: r.Element, State: r.State, Attachment: r.Attachment, Cut: r.Cut}, Proposal: p}
	return encodeControlRead(r.Pending, read, r.Pending)
}

func (s *Service) controlCaller(ctx context.Context, w *corepb.ServiceAttachmentKey) (values.CallerContext, values.FacetAcceptanceKey, error) {
	caller, ok := grpcsession.Caller(ctx)
	if !ok || s.services[caller.Subject] == "" {
		return values.CallerContext{}, values.FacetAcceptanceKey{}, status.Error(codes.PermissionDenied, "fixed service required")
	}
	key, err := decodeControlKey(w)
	if err != nil || key.Core != s.expected || key.Facet != s.services[caller.Subject] {
		return values.CallerContext{}, values.FacetAcceptanceKey{}, status.Error(codes.InvalidArgument, "invalid service relationship")
	}
	return caller, key, nil
}

func decodeControlKey(w *corepb.ServiceAttachmentKey) (values.FacetAcceptanceKey, error) {
	if w == nil || w.Proposal == nil || len(w.Proposal.Incarnation) != 32 || w.Proposal.GetSequence() == 0 {
		return values.FacetAcceptanceKey{}, values.PublicInvalid
	}
	raw := make([]byte, 76)
	copy(raw, w.Proposal.Incarnation)
	copy(raw[32:68], w.GetElement())
	binary.BigEndian.PutUint64(raw[68:], w.Proposal.GetSequence())
	revision, err := values.NewAttachmentRevision(raw)
	key := values.FacetAcceptanceKey{Core: values.FactAuthorityRef(w.GetCore()), Facet: values.FactAuthorityRef(w.GetFacet()), Element: values.ElementID(w.GetElement()), Proposal: revision}
	if err != nil || !key.Valid() {
		return values.FacetAcceptanceKey{}, values.PublicInvalid
	}
	return key, nil
}

func encodeControlRevision(r values.AttachmentRevision) *corepb.ServiceRevision {
	b := r.Bytes()
	return &corepb.ServiceRevision{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[68:]))}
}

func encodeControlKey(k values.FacetAcceptanceKey) *corepb.ServiceAttachmentKey {
	if !k.Valid() {
		return nil
	}
	return &corepb.ServiceAttachmentKey{Core: proto.String(string(k.Core)), Facet: proto.String(string(k.Facet)), Element: proto.String(string(k.Element)), Proposal: encodeControlRevision(k.Proposal)}
}

func (s *Service) ReadControl(ctx context.Context, req *corepb.ReadControlRequest) (*corepb.ReadControlResponse, error) {
	caller, key, err := s.controlCaller(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	r, err := s.owner.ReadAttachmentControl(ctx, caller, key, req.GetPrepare())
	if err != nil {
		failure, transport := rpcfailure.Control(err)
		return &corepb.ReadControlResponse{Outcome: &corepb.ReadControlResponse_Failure{Failure: failure}}, transport
	}
	return encodeControlRead(key, r, values.FacetAcceptanceKey{})
}

func (s *Service) ReadProposalControl(ctx context.Context, req *corepb.ReadControlRequest) (*corepb.ReadControlResponse, error) {
	caller, key, err := s.controlCaller(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	if req.GetPrepare() {
		return nil, status.Error(codes.InvalidArgument, "proposal observation is not supported")
	}
	r, err := s.owner.ReadProposalControl(ctx, caller, key)
	if err != nil {
		failure, transport := rpcfailure.Control(err)
		return &corepb.ReadControlResponse{Outcome: &corepb.ReadControlResponse_Failure{Failure: failure}}, transport
	}
	read := corecontract.AttachmentControlRead{Read: corecontract.FacetAttachmentRead{Authority: r.Authority, Element: r.Element, State: r.State, Attachment: r.Attachment, Cut: r.Cut}}
	return encodeControlRead(key, read, r.Pending)
}

func encodeControlRead(key values.FacetAcceptanceKey, r corecontract.AttachmentControlRead, pending values.FacetAcceptanceKey) (*corepb.ReadControlResponse, error) {
	e := r.Read
	cut := e.Cut.Evidence()
	if e.Authority != key.Core || e.Element != key.Element || !cut.Valid() {
		return nil, status.Error(codes.Internal, "control result unavailable")
	}
	b := cut.Bytes()
	out := &corepb.ControlRead{Core: proto.String(string(e.Authority)), Element: proto.String(string(e.Element)), Cut: &corepb.ServiceCoreCut{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[32:]))}, Proposal: wire.ControlToken(r.Proposal, networkpb.TokenKind_ATTACHMENT_REVISION), Preparation: wire.ControlToken(r.Preparation, networkpb.TokenKind_OBSERVATION_PREPARATION)}
	out.Pending = encodeControlKey(pending)
	switch e.State {
	case corecontract.ExistenceNotFound:
		out.Presence = corepb.Presence_NOT_FOUND.Enum()
	case corecontract.ExistencePresent:
		if !e.Attachment.Revision.Valid() {
			return nil, status.Error(codes.Internal, "control result unavailable")
		}
		out.Presence = corepb.Presence_PRESENT.Enum()
		out.Revision = encodeControlRevision(e.Attachment.Revision)
		out.Active = encodeControlKey(e.Attachment.Key)
		switch e.Attachment.State {
		case corecontract.FacetNone:
			out.State = corepb.AttachmentState_NONE.Enum()
		case corecontract.FacetActive:
			out.State = corepb.AttachmentState_ACTIVE.Enum()
		case corecontract.FacetDetached:
			out.State = corepb.AttachmentState_DETACHED.Enum()
		case corecontract.FacetRetired:
			// The released slot keeps its former key as the permanent lineage
			// record; a retired relationship can never become live again.
			out.State = corepb.AttachmentState_RETIRED.Enum()
		default:
			return nil, status.Error(codes.Internal, "control result unavailable")
		}
	default:
		return nil, status.Error(codes.Internal, "control result unavailable")
	}
	return &corepb.ReadControlResponse{Outcome: &corepb.ReadControlResponse_Result{Result: out}}, nil
}

func (s *Service) CheckControl(ctx context.Context, req *corepb.CheckControlRequest) (*corepb.CheckControlResponse, error) {
	caller, key, err := s.controlCaller(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	if req.Preparation == nil || req.Preparation.GetOwner() != string(key.Core) || req.Preparation.GetKind() != networkpb.TokenKind_OBSERVATION_PREPARATION || len(req.Preparation.Value) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid preparation")
	}
	token, err := values.PublicTokenFromBytes(key.Core, "observation-preparation", [32]byte(req.Preparation.Value))
	if err == nil {
		err = s.owner.CheckAttachmentControl(ctx, caller, key, token)
	}
	if err != nil {
		failure, transport := rpcfailure.Control(err)
		return &corepb.CheckControlResponse{Outcome: &corepb.CheckControlResponse_Failure{Failure: failure}}, transport
	}
	return &corepb.CheckControlResponse{Outcome: &corepb.CheckControlResponse_Result{Result: &corepb.CheckControlResult{Valid: proto.Bool(true)}}}, nil
}

// ReadReplacementControl discloses exactly one named replacement control
// record to the fixed Facet service it names as source or target. The service
// approval table and the record's own parties confine the read; no public
// caller authorization exists on this path.
func (s *Service) ReadReplacementControl(ctx context.Context, req *corepb.ReadReplacementControlRequest) (*corepb.ReadReplacementControlResponse, error) {
	caller, ok := grpcsession.Caller(ctx)
	facet := s.services[caller.Subject]
	if !ok || !facet.Valid() {
		return nil, status.Error(codes.PermissionDenied, "fixed Facet service required")
	}
	if _, delegated := grpcsession.Grant(ctx); delegated {
		return nil, status.Error(codes.PermissionDenied, "original service session required")
	}
	id := corecontract.OperationID(req.GetOperationId())
	if len(id) == 0 || len(id) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid replacement operation")
	}
	element := values.ElementID(req.GetElement())
	if !element.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid replacement element")
	}
	r, err := s.owner.ReadFacetReplacementRecord(id, element)
	if err != nil {
		failure, transport := rpcfailure.MountPeer(err)
		return &corepb.ReadReplacementControlResponse{Outcome: &corepb.ReadReplacementControlResponse_Failure{Failure: failure}}, transport
	}
	if r.Authority != s.expected || r.Element != element || r.Operation != id || !r.Proposal.Valid() || (r.Source != facet && r.Target != facet) {
		return &corepb.ReadReplacementControlResponse{Outcome: &corepb.ReadReplacementControlResponse_Failure{Failure: networkpb.Failure_DENIED}}, nil
	}
	w := &corepb.ReplacementControlRecord{Element: proto.String(string(r.Element)), OperationId: append([]byte(nil), r.Operation...), Source: proto.String(string(r.Source)), Target: proto.String(string(r.Target)), Proposal: encodeControlRevision(r.Proposal), Permitted: proto.Bool(r.Permitted), Fenced: proto.Bool(r.Fenced), Accepted: proto.Bool(r.Accepted), Committed: proto.Bool(r.Committed), Settled: proto.Bool(r.Settled), Cancelled: proto.Bool(r.Cancelled)}
	if r.FinalCut != 0 {
		w.FinalCut = proto.Uint64(r.FinalCut)
	}
	if r.ManifestDigest != ([32]byte{}) {
		w.ManifestDigest = append([]byte(nil), r.ManifestDigest[:]...)
	}
	return &corepb.ReadReplacementControlResponse{Outcome: &corepb.ReadReplacementControlResponse_Result{Result: w}}, nil
}

// ReadSuccessorControl discloses one departed element's durable successor
// statement to the fixed Facet service. The service session and the element's
// attachment-read authorization confine the read; no public caller
// authorization exists on this path, and a proven absence is a normal result.
func (s *Service) ReadSuccessorControl(ctx context.Context, req *corepb.ReadSuccessorControlRequest) (*corepb.ReadSuccessorControlResponse, error) {
	caller, ok := grpcsession.Caller(ctx)
	facet := s.services[caller.Subject]
	if !ok || !facet.Valid() {
		return nil, status.Error(codes.PermissionDenied, "fixed Facet service required")
	}
	if _, delegated := grpcsession.Grant(ctx); delegated {
		return nil, status.Error(codes.PermissionDenied, "original service session required")
	}
	element := values.ElementID(req.GetElement())
	if !element.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid successor element")
	}
	r, err := s.owner.ReadSuccessorControl(ctx, caller, element)
	if err != nil {
		failure, transport := rpcfailure.Control(err)
		return &corepb.ReadSuccessorControlResponse{Outcome: &corepb.ReadSuccessorControlResponse_Failure{Failure: failure}}, transport
	}
	if r.Authority != s.expected || r.Element != element {
		return &corepb.ReadSuccessorControlResponse{Outcome: &corepb.ReadSuccessorControlResponse_Failure{Failure: networkpb.Failure_UNAVAILABLE}}, nil
	}
	w := &corepb.SuccessorControlRecord{Element: proto.String(string(r.Element)), Found: proto.Bool(r.State == corecontract.ExistencePresent), Settled: proto.Bool(r.Settled)}
	if r.State == corecontract.ExistencePresent {
		w.OperationId = append([]byte(nil), r.Operation...)
		w.Source = proto.String(string(r.Source))
		w.Target = proto.String(string(r.Target))
		if r.FinalCut != 0 {
			w.FinalCut = proto.Uint64(r.FinalCut)
		}
	}
	return &corepb.ReadSuccessorControlResponse{Outcome: &corepb.ReadSuccessorControlResponse_Result{Result: w}}, nil
}
