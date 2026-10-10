package facet

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// Owner is the narrow Facet-control port consumed by this RPC group. The
// transport never receives the owner's state, locks, registries, or Store.
type Owner interface {
	InspectFacetControl(context.Context, values.CallerContext, values.ElementID) (corecontract.PublicFacetInspection, error)
	ApplyFacetControl(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicFacetCommand) (corecontract.MutationStatus, corecontract.PublicFacetInspection, error)
	BeginFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicFacetReplacementCommand) (corecontract.PublicFacetReplacementStatus, error)
	InspectFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID) (corecontract.PublicFacetReplacementStatus, error)
	CancelFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID, values.PublicToken) (corecontract.PublicFacetReplacementStatus, error)
	RecoverFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID, replacement.FacetPeers) (corecontract.PublicFacetReplacementStatus, error)
}

// Service owns only Facet-control request parsing and response encoding.
// Unsupported owners are retained as an RPC-time Unimplemented result rather
// than becoming a Server.New validation failure. The replacement recovery
// peers are trusted host wiring shared by every recovery call, never request
// input.
type Service struct {
	pb.UnimplementedFacetControlServiceServer
	owner Owner
	peers replacement.FacetPeers
}

func New(owner any, peers replacement.FacetPeers) *Service {
	o, _ := owner.(Owner)
	return &Service{owner: o, peers: peers}
}

func (s *Service) facetController(ctx context.Context) (Owner, values.CallerContext, error) {
	if s.owner == nil {
		return nil, values.CallerContext{}, status.Error(codes.Unimplemented, "Facet control unavailable")
	}
	c, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, c, status.Error(codes.Unauthenticated, "session required")
	}
	if _, delegated := grpcsession.Grant(ctx); delegated {
		return nil, c, status.Error(codes.PermissionDenied, "read delegation cannot write relationships")
	}
	return s.owner, c, nil
}

func facetControlFailure(err error) (netpb.Failure, error) {
	switch {
	case errors.Is(err, corecontract.ErrAttachmentIndeterminate):
		return netpb.Failure_INDETERMINATE, nil
	case errors.Is(err, corecontract.ErrAttachmentRevisionConflict), errors.Is(err, corecontract.ErrAttachmentStateConflict), errors.Is(err, corecontract.ErrProposalNotCurrent), errors.Is(err, corecontract.ErrAttachmentChanged), errors.Is(err, corecontract.ErrFacetContactRevisionConflict):
		return netpb.Failure_CONFLICT, nil
	case errors.Is(err, corecontract.ErrAcceptanceNotFound), errors.Is(err, corecontract.ErrDispositionNotFound):
		return netpb.Failure_NOT_FOUND, nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 0, status.FromContextError(err).Err()
	default:
		return rpcfailure.MountPeer(err)
	}
}

func publicFacetCommandWire(w *pb.PublicFacetCommand) (corecontract.PublicFacetCommand, error) {
	if w == nil || w.Expected == nil || len(w.Expected.Value) != 32 {
		return corecontract.PublicFacetCommand{}, corecontract.ErrInvalid
	}
	c := corecontract.PublicFacetCommand{Kind: w.GetKind(), Element: values.ElementID(w.GetElement()), Target: values.FactAuthorityRef(w.GetTarget()), NextHop: values.NextHop(w.GetNextHop())}
	kind, wireKind := "attachment-revision", netpb.TokenKind_ATTACHMENT_REVISION
	if c.Kind == "contact" {
		kind, wireKind = "contact-revision", netpb.TokenKind_CONTACT_REVISION
	}
	// Release shares propose's routing comparisons: both name the target Facet
	// and its address so the disposition evidence read stays routable.
	if c.Kind != "propose" && c.Kind != "release" && w.Target != nil || c.Kind != "propose" && c.Kind != "contact" && c.Kind != "release" && w.NextHop != nil || w.Expected.GetKind() != wireKind {
		return c, corecontract.ErrInvalid
	}
	var err error
	c.Expected, err = values.PublicTokenFromBytes(values.FactAuthorityRef(w.Expected.GetOwner()), kind, [32]byte(w.Expected.Value))
	return c, err
}

func (s *Service) InspectFacet(ctx context.Context, req *pb.InspectFacetRequest) (*pb.InspectFacetResponse, error) {
	o, c, err := s.facetController(ctx)
	if err != nil {
		return nil, err
	}
	r, err := o.InspectFacetControl(ctx, c, values.ElementID(req.GetElement()))
	if err != nil {
		f, e := facetControlFailure(err)
		return &pb.InspectFacetResponse{Outcome: &pb.InspectFacetResponse_Failure{Failure: f}}, e
	}
	return &pb.InspectFacetResponse{Outcome: &pb.InspectFacetResponse_Result{Result: wire.PublicFacetInspectionWire(r)}}, nil
}

func (s *Service) ApplyFacet(ctx context.Context, req *pb.ApplyFacetRequest) (*pb.ApplyFacetResponse, error) {
	o, c, err := s.facetController(ctx)
	if err != nil {
		return nil, err
	}
	command, err := publicFacetCommandWire(req.GetCommand())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid Facet command")
	}
	state, r, err := o.ApplyFacetControl(ctx, c, corecontract.OperationID(req.OperationId), command)
	if err != nil {
		f, e := facetControlFailure(err)
		return &pb.ApplyFacetResponse{Outcome: &pb.ApplyFacetResponse_Failure{Failure: f}}, e
	}
	return &pb.ApplyFacetResponse{Outcome: &pb.ApplyFacetResponse_Result{Result: &pb.PublicFacetOutcome{OperationId: append([]byte(nil), req.OperationId...), Status: proto.Uint32(uint32(state)), State: wire.PublicFacetInspectionWire(r)}}}, nil
}

// publicReplacementCommandWire accepts only the caller-visible comparisons of
// one reservation. The element's current content owner is disclosed by the
// retained record, never taken from this command.
func publicReplacementCommandWire(w *pb.PublicFacetReplacementCommand) (corecontract.PublicFacetReplacementCommand, error) {
	if w == nil || w.Expected == nil || w.Expected.GetKind() != netpb.TokenKind_ATTACHMENT_REVISION || len(w.Expected.Value) != 32 {
		return corecontract.PublicFacetReplacementCommand{}, corecontract.ErrInvalid
	}
	expected, err := values.PublicTokenFromBytes(values.FactAuthorityRef(w.Expected.GetOwner()), "attachment-revision", [32]byte(w.Expected.Value))
	command := corecontract.PublicFacetReplacementCommand{Element: values.ElementID(w.GetElement()), Target: values.FactAuthorityRef(w.GetTarget()), TargetAddress: values.NextHop(w.GetTargetAddress()), SourceAddress: values.NextHop(w.GetSourceAddress())}
	if err != nil || !command.Element.Valid() || !command.Target.Valid() || !command.TargetAddress.Valid() || !command.SourceAddress.Valid() {
		return corecontract.PublicFacetReplacementCommand{}, corecontract.ErrInvalid
	}
	command.Expected = expected
	return command, nil
}

// replacementRevisionWire decodes the stable public revision comparison of one
// retained replacement reservation.
func replacementRevisionWire(t *netpb.Token) (values.PublicToken, error) {
	if t == nil || t.GetKind() != netpb.TokenKind_FACET_REPLACEMENT_REVISION || len(t.Value) != 32 {
		return values.PublicToken{}, corecontract.ErrInvalid
	}
	return values.PublicTokenFromBytes(values.FactAuthorityRef(t.GetOwner()), "facet-replacement-revision", [32]byte(t.Value))
}

// replacementStatusWire encodes only the separately authorized projection:
// milestones, refusal and the stable revision comparison, never manifest
// bytes or private cuts.
func replacementStatusWire(r corecontract.PublicFacetReplacementStatus) (*pb.FacetReplacementStatus, error) {
	if !r.Authority.Valid() || !r.Element.Valid() || r.Operation == "" || !r.Source.Valid() || !r.Target.Valid() ||
		r.Progress < corecontract.FacetReplacementReserved || r.Progress > corecontract.FacetReplacementCancelled ||
		!r.Revision.Valid() || r.Revision.Kind != "facet-replacement-revision" {
		return nil, status.Error(codes.Internal, "replacement result unavailable")
	}
	return &pb.FacetReplacementStatus{Element: proto.String(string(r.Element)), OperationId: append([]byte(nil), r.Operation...), Source: proto.String(string(r.Source)), Target: proto.String(string(r.Target)), Progress: proto.Uint32(uint32(r.Progress)), Refused: proto.Bool(r.Refused), Revision: wire.ControlToken(r.Revision, netpb.TokenKind_FACET_REPLACEMENT_REVISION)}, nil
}

func (s *Service) replacementOutcome(r corecontract.PublicFacetReplacementStatus, err error) (*pb.FacetReplacementStatusResponse, error) {
	if err != nil {
		f, e := facetControlFailure(err)
		return &pb.FacetReplacementStatusResponse{Outcome: &pb.FacetReplacementStatusResponse_Failure{Failure: f}}, e
	}
	w, err := replacementStatusWire(r)
	if err != nil {
		return nil, err
	}
	return &pb.FacetReplacementStatusResponse{Outcome: &pb.FacetReplacementStatusResponse_Result{Result: w}}, nil
}

func (s *Service) BeginFacetReplacement(ctx context.Context, req *pb.BeginFacetReplacementRequest) (*pb.FacetReplacementStatusResponse, error) {
	o, c, err := s.facetController(ctx)
	if err != nil {
		return nil, err
	}
	command, err := publicReplacementCommandWire(req.GetCommand())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid Facet replacement command")
	}
	r, err := o.BeginFacetReplacement(ctx, c, corecontract.OperationID(req.GetOperationId()), command)
	return s.replacementOutcome(r, err)
}

func (s *Service) InspectFacetReplacement(ctx context.Context, req *pb.ReplacementControlRequest) (*pb.FacetReplacementStatusResponse, error) {
	o, c, err := s.facetController(ctx)
	if err != nil {
		return nil, err
	}
	r, err := o.InspectFacetReplacement(ctx, c, corecontract.OperationID(req.GetOperationId()), values.ElementID(req.GetElement()))
	return s.replacementOutcome(r, err)
}

func (s *Service) CancelFacetReplacement(ctx context.Context, req *pb.CancelFacetReplacementRequest) (*pb.FacetReplacementStatusResponse, error) {
	o, c, err := s.facetController(ctx)
	if err != nil {
		return nil, err
	}
	expected, err := replacementRevisionWire(req.GetExpected())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid replacement revision")
	}
	r, err := o.CancelFacetReplacement(ctx, c, corecontract.OperationID(req.GetOperationId()), values.ElementID(req.GetElement()), expected)
	return s.replacementOutcome(r, err)
}

// RecoverFacetReplacement advances one exact reservation to its committed
// outcome. The designated peer contacts come from trusted host wiring; a host
// that wired no peers keeps this RPC-time Unimplemented result.
func (s *Service) RecoverFacetReplacement(ctx context.Context, req *pb.ReplacementControlRequest) (*pb.FacetReplacementStatusResponse, error) {
	o, c, err := s.facetController(ctx)
	if err != nil {
		return nil, err
	}
	if s.peers.Source == nil || s.peers.Target == nil {
		return nil, status.Error(codes.Unimplemented, "Facet replacement recovery unavailable")
	}
	r, err := o.RecoverFacetReplacement(ctx, c, corecontract.OperationID(req.GetOperationId()), values.ElementID(req.GetElement()), s.peers)
	return s.replacementOutcome(r, err)
}
