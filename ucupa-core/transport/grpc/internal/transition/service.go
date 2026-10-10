// Package transition adapts source and parent control RPCs. It owns neither transfer facts
// nor peer approval; the authenticated caller is supplied by session admission.
package transition

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	"ucupa/ucupa-core/transport/grpc/internal/session"
)

type Owner interface {
	AuthorityRef() values.FactAuthorityRef
	BeginTransition(context.Context, values.CallerContext, contract.OperationID, contract.TransitionCommand) (contract.TransitionStatus, error)
	InspectTransition(context.Context, values.CallerContext, contract.OperationID, values.ElementID) (contract.TransitionStatus, error)
	CancelTransition(context.Context, values.CallerContext, contract.OperationID, values.ElementID, values.PublicToken) (contract.TransitionStatus, error)
	RecoverTransition(context.Context, values.CallerContext, contract.TransitionApprovalBinding, transition.TargetTransport) (contract.TransitionStatus, error)
	BeginCarrierTransition(context.Context, values.CallerContext, contract.OperationID, contract.CarrierCommand) (contract.CarrierStatus, error)
	InspectCarrierTransition(context.Context, values.CallerContext, contract.OperationID, values.ElementID) (contract.CarrierStatus, error)
	CancelCarrierTransition(context.Context, values.CallerContext, contract.OperationID, values.ElementID, values.PublicToken) (contract.CarrierStatus, error)
	RecoverCarrierTransition(context.Context, values.CallerContext, contract.CarrierApprovalBinding, transition.CarrierTransport) (contract.CarrierStatus, error)
}

type Sessions interface {
	BindEntrySession(context.Context, values.CallerContext) (admission.EntrySession, error)
	CheckEntrySession(admission.EntrySession) error
	EntryContext(context.Context, admission.EntrySession) (context.Context, context.CancelFunc, error)
}

type Targets interface {
	Target(contract.TransitionApprovalBinding, []byte) (transition.TargetTransport, error)
}

type Service struct {
	pb.UnimplementedTransitionControlServiceServer
	owner    Owner
	expected values.FactAuthorityRef
	sessions Sessions
	targets  Targets
	carriers Carriers
}

func New(owner Owner, expected values.FactAuthorityRef, sessions Sessions, targets Targets, carriers Carriers) *Service {
	return &Service{owner: owner, expected: expected, sessions: sessions, targets: targets, carriers: carriers}
}

func (s *Service) BeginTransition(ctx context.Context, r *pb.BeginTransitionRequest) (*pb.TransitionResponse, error) {
	command, err := decodeCommand(r, s.expected)
	if err != nil {
		return nil, err
	}
	return s.control(ctx, r.OperationId, command.Boundary, &command, values.PublicToken{}, nil)
}

func (s *Service) InspectTransition(ctx context.Context, r *pb.InspectTransitionRequest) (*pb.TransitionResponse, error) {
	return s.control(ctx, r.GetOperationId(), values.ElementID(r.GetBoundary()), nil, values.PublicToken{}, nil)
}

func (s *Service) CancelTransition(ctx context.Context, r *pb.CancelTransitionRequest) (*pb.TransitionResponse, error) {
	expected, err := decodeToken(r.GetExpected(), netpb.TokenKind_TRANSITION_REVISION, "transition-revision", s.expected)
	if err != nil {
		return nil, err
	}
	return s.control(ctx, r.GetOperationId(), values.ElementID(r.GetBoundary()), nil, expected, nil)
}

func (s *Service) control(ctx context.Context, id []byte, boundary values.ElementID, command *contract.TransitionCommand, expected values.PublicToken, recover *pb.RecoverTransitionRequest) (*pb.TransitionResponse, error) {
	ctx, caller, entry, cancel, err := s.controlContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if len(id) == 0 || len(id) > 128 || !boundary.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid transition key")
	}
	var out contract.TransitionStatus
	operation := contract.OperationID(id)
	switch {
	case recover != nil:
		out, err = s.recover(ctx, caller, recover)
	case command != nil:
		out, err = s.owner.BeginTransition(ctx, caller, operation, *command)
	case expected != (values.PublicToken{}):
		out, err = s.owner.CancelTransition(ctx, caller, operation, boundary, expected)
	default:
		out, err = s.owner.InspectTransition(ctx, caller, operation, boundary)
	}
	// Losing the original session or connection after dispatch is not evidence
	// that a mutation failed. No result is disclosed and no automatic retry runs.
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if s.sessions.CheckEntrySession(entry) != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	return response(out, err, s.expected, operation, boundary, command, expected != (values.PublicToken{}), recover != nil)
}

// Source and parent controls use the original session lifetime, including
// while waiting for an owner lock. Cleanup ends only this request's context.
func (s *Service) controlContext(ctx context.Context) (context.Context, values.CallerContext, admission.EntrySession, context.CancelFunc, error) {
	bad := func(code codes.Code, message string) (context.Context, values.CallerContext, admission.EntrySession, context.CancelFunc, error) {
		return nil, values.CallerContext{}, admission.EntrySession{}, nil, status.Error(code, message)
	}
	if s.owner == nil {
		return bad(codes.Unimplemented, "transition control unavailable")
	}
	if s.owner.AuthorityRef() != s.expected {
		return bad(codes.Unavailable, "owner changed")
	}
	if _, ok := session.Grant(ctx); ok {
		return bad(codes.PermissionDenied, "read delegation is not transition approval")
	}
	caller, ok := session.Caller(ctx)
	if !ok {
		return bad(codes.Unauthenticated, "session required")
	}
	entry, err := s.sessions.BindEntrySession(ctx, caller)
	if err != nil {
		return bad(codes.Unauthenticated, "session unavailable")
	}
	bound, cancel, err := s.sessions.EntryContext(ctx, entry)
	if err != nil {
		return bad(codes.Unauthenticated, "session unavailable")
	}
	return bound, caller, entry, cancel, nil
}
