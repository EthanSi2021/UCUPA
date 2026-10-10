package facetgrpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
)

type relationshipOwner interface {
	AcceptAttachmentControl(context.Context, values.CallerContext, facetcontract.PublicAcceptanceRef, values.NextHop, facetcontract.OperationID) (facetcontract.PublicAcceptanceOutcome, error)
	InspectAcceptanceControl(context.Context, values.CallerContext, facetcontract.PublicAcceptanceRef) (facetcontract.PublicAcceptanceInspection, error)
	UpdateCoreContactControl(context.Context, values.CallerContext, facetcontract.OperationID, facetcontract.PublicAcceptanceRef, values.PublicToken, values.NextHop) (facetcontract.PublicAcceptanceOutcome, error)
	RetireContentControl(context.Context, values.CallerContext, facetcontract.PublicAcceptanceRef, facetcontract.OperationID) (facetcontract.PublicAcceptanceOutcome, error)
}

func (s *Server) relationshipCaller(ctx context.Context) (relationshipOwner, values.CallerContext, error) {
	owner, ok := s.owner.(relationshipOwner)
	if !ok {
		return nil, values.CallerContext{}, status.Error(codes.Unimplemented, "relationship control unavailable")
	}
	caller, err := s.Caller(ctx)
	if err != nil || !caller.Subject.Valid() {
		return nil, values.CallerContext{}, status.Error(codes.Unauthenticated, "original session required")
	}
	return owner, caller, nil
}

func decodeRelationshipToken(w *netpb.Token, owner values.FactAuthorityRef, kind netpb.TokenKind, name string) (values.PublicToken, error) {
	if w == nil || w.GetOwner() != string(owner) || w.GetKind() != kind || len(w.Value) != 32 {
		return values.PublicToken{}, status.Error(codes.InvalidArgument, "invalid relationship comparison")
	}
	t, err := values.PublicTokenFromBytes(owner, name, [32]byte(w.Value))
	if err != nil {
		return values.PublicToken{}, status.Error(codes.InvalidArgument, "invalid relationship comparison")
	}
	return t, nil
}

func decodePublicAcceptanceRef(w *pb.PublicAcceptanceRef) (facetcontract.PublicAcceptanceRef, error) {
	if w == nil || !values.FactAuthorityRef(w.GetCore()).Valid() || !values.ElementID(w.GetElement()).Valid() {
		return facetcontract.PublicAcceptanceRef{}, status.Error(codes.InvalidArgument, "invalid relationship selector")
	}
	core := values.FactAuthorityRef(w.GetCore())
	token, err := decodeRelationshipToken(w.Proposal, core, netpb.TokenKind_ATTACHMENT_REVISION, "attachment-revision")
	return facetcontract.PublicAcceptanceRef{Core: core, Element: values.ElementID(w.GetElement()), Proposal: token}, err
}

func relationshipToken(t values.PublicToken, kind netpb.TokenKind) *netpb.Token {
	if !t.Valid() {
		return nil
	}
	raw, _ := values.DecodePublicBytes(t.Value)
	return &netpb.Token{Owner: proto.String(string(t.Owner)), Kind: kind.Enum(), Value: raw}
}

func encodePublicAcceptance(r facetcontract.PublicAcceptanceInspection) *pb.PublicAcceptanceInspection {
	ref := &pb.PublicAcceptanceRef{Core: proto.String(string(r.Ref.Core)), Element: proto.String(string(r.Ref.Element)), Proposal: relationshipToken(r.Ref.Proposal, netpb.TokenKind_ATTACHMENT_REVISION)}
	w := &pb.PublicAcceptanceInspection{Ref: ref, Found: proto.Bool(r.Found)}
	if r.Found {
		w.CoreNextHop = proto.String(string(r.CoreNextHop))
		w.ContactRevision = relationshipToken(r.ContactRevision, netpb.TokenKind_CONTACT_REVISION)
	}
	return w
}

func encodeAcceptanceOutcome(r facetcontract.PublicAcceptanceOutcome) (*pb.PublicAcceptanceOutcome, error) {
	var state pb.CommitStatus
	switch r.Status {
	case facetcontract.Committed:
		state = pb.CommitStatus_COMMITTED
	case facetcontract.Unchanged:
		state = pb.CommitStatus_UNCHANGED
	default:
		return nil, status.Error(codes.Internal, "relationship result unavailable")
	}
	if !r.Inspection.Found || !r.Inspection.CoreNextHop.Valid() || !r.Inspection.ContactRevision.Valid() {
		return nil, status.Error(codes.Internal, "relationship result unavailable")
	}
	return &pb.PublicAcceptanceOutcome{Status: state.Enum(), Inspection: encodePublicAcceptance(r.Inspection)}, nil
}

// encodeRetirementOutcome reports the proven absence of the disposed
// relationship: the committed retire removed the acceptance, so the result's
// inspection is exactly the not-found shape of the request's selector.
func encodeRetirementOutcome(r facetcontract.PublicAcceptanceOutcome) (*pb.PublicAcceptanceOutcome, error) {
	var state pb.CommitStatus
	switch r.Status {
	case facetcontract.Committed:
		state = pb.CommitStatus_COMMITTED
	case facetcontract.Unchanged:
		state = pb.CommitStatus_UNCHANGED
	default:
		return nil, status.Error(codes.Internal, "relationship result unavailable")
	}
	if r.Inspection.Found || r.Inspection.CoreNextHop.Valid() || r.Inspection.ContactRevision.Valid() {
		return nil, status.Error(codes.Internal, "relationship result unavailable")
	}
	return &pb.PublicAcceptanceOutcome{Status: state.Enum(), Inspection: encodePublicAcceptance(r.Inspection)}, nil
}

func relationshipFailure(err error) (netpb.Failure, error) {
	var public values.PublicError
	if errors.As(err, &public) {
		if code, ok := netpb.Failure_value[string(public)]; ok && code != 0 {
			return netpb.Failure(code), nil
		}
	}
	return 0, status.Error(codes.Internal, "relationship result unavailable")
}

func (s *Server) AcceptAttachment(ctx context.Context, in *pb.AcceptAttachmentRequest) (*pb.AcceptAttachmentResponse, error) {
	o, caller, err := s.relationshipCaller(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := decodePublicAcceptanceRef(in.GetRef())
	if err != nil {
		return nil, err
	}
	r, err := o.AcceptAttachmentControl(ctx, caller, ref, values.NextHop(in.GetCoreNextHop()), facetcontract.OperationID(in.OperationId))
	if err != nil {
		f, e := relationshipFailure(err)
		return &pb.AcceptAttachmentResponse{Outcome: &pb.AcceptAttachmentResponse_Failure{Failure: f}}, e
	}
	w, err := encodeAcceptanceOutcome(r)
	return &pb.AcceptAttachmentResponse{Outcome: &pb.AcceptAttachmentResponse_Result{Result: w}}, err
}

func (s *Server) InspectAcceptance(ctx context.Context, in *pb.InspectAcceptanceRequest) (*pb.InspectAcceptanceResponse, error) {
	o, caller, err := s.relationshipCaller(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := decodePublicAcceptanceRef(in.GetRef())
	if err != nil {
		return nil, err
	}
	r, err := o.InspectAcceptanceControl(ctx, caller, ref)
	if err != nil {
		f, e := relationshipFailure(err)
		return &pb.InspectAcceptanceResponse{Outcome: &pb.InspectAcceptanceResponse_Failure{Failure: f}}, e
	}
	return &pb.InspectAcceptanceResponse{Outcome: &pb.InspectAcceptanceResponse_Result{Result: encodePublicAcceptance(r)}}, nil
}

func (s *Server) UpdateCoreContact(ctx context.Context, in *pb.UpdateCoreContactRequest) (*pb.UpdateCoreContactResponse, error) {
	o, caller, err := s.relationshipCaller(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := decodePublicAcceptanceRef(in.GetRef())
	if err != nil {
		return nil, err
	}
	expected, err := decodeRelationshipToken(in.Expected, s.config.Expected, netpb.TokenKind_CONTACT_REVISION, "contact-revision")
	if err != nil {
		return nil, err
	}
	r, err := o.UpdateCoreContactControl(ctx, caller, facetcontract.OperationID(in.OperationId), ref, expected, values.NextHop(in.GetCoreNextHop()))
	if err != nil {
		f, e := relationshipFailure(err)
		return &pb.UpdateCoreContactResponse{Outcome: &pb.UpdateCoreContactResponse_Failure{Failure: f}}, e
	}
	w, err := encodeAcceptanceOutcome(r)
	return &pb.UpdateCoreContactResponse{Outcome: &pb.UpdateCoreContactResponse_Result{Result: w}}, err
}

func (s *Server) RetireContent(ctx context.Context, in *pb.RetireContentRequest) (*pb.RetireContentResponse, error) {
	o, caller, err := s.relationshipCaller(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := decodePublicAcceptanceRef(in.GetRef())
	if err != nil {
		return nil, err
	}
	r, err := o.RetireContentControl(ctx, caller, ref, facetcontract.OperationID(in.OperationId))
	if err != nil {
		f, e := relationshipFailure(err)
		return &pb.RetireContentResponse{Outcome: &pb.RetireContentResponse_Failure{Failure: f}}, e
	}
	w, err := encodeRetirementOutcome(r)
	return &pb.RetireContentResponse{Outcome: &pb.RetireContentResponse_Result{Result: w}}, err
}
