package transition

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) BeginCarrierTransition(ctx context.Context, r *pb.BeginCarrierTransitionRequest) (*pb.CarrierTransitionResponse, error) {
	c, err := wire.CarrierCommand(r, s.expected)
	if err != nil {
		return nil, err
	}
	return s.carrierControl(ctx, r.OperationId, c.Mount, &c, values.PublicToken{})
}

func (s *Service) InspectCarrierTransition(ctx context.Context, r *pb.InspectCarrierTransitionRequest) (*pb.CarrierTransitionResponse, error) {
	return s.carrierControl(ctx, r.GetOperationId(), values.ElementID(r.GetMount()), nil, values.PublicToken{})
}

func (s *Service) CancelCarrierTransition(ctx context.Context, r *pb.CancelCarrierTransitionRequest) (*pb.CarrierTransitionResponse, error) {
	expected, err := decodeToken(r.GetExpected(), netpb.TokenKind_CARRIER_REVISION, "carrier-revision", s.expected)
	if err != nil {
		return nil, err
	}
	return s.carrierControl(ctx, r.GetOperationId(), values.ElementID(r.GetMount()), nil, expected)
}

// RecoverCarrierTransition performs one bounded parent-side advance over the
// already retained source and target approval tokens. It reloads the parent's
// durable original input and never re-issues or widens a peer approval.
func (s *Service) RecoverCarrierTransition(ctx context.Context, r *pb.RecoverCarrierTransitionRequest) (*pb.CarrierTransitionResponse, error) {
	if r == nil || len(r.SourceToken) != 32 || len(r.TargetToken) != 32 || proto.Size(r) > 8192 {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier recovery request")
	}
	if s.carriers == nil {
		return nil, status.Error(codes.Unimplemented, "designated carrier connections unavailable")
	}
	b, err := wire.CarrierBinding(r.Binding)
	if err != nil || b.Parent != s.expected {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier recovery binding")
	}
	transport, err := s.carriers.Carrier(b, r.SourceToken, r.TargetToken)
	if err != nil {
		return nil, err
	}
	ctx, caller, entry, cancel, err := s.controlContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	out, err := s.owner.RecoverCarrierTransition(ctx, caller, b, transport)
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if s.sessions.CheckEntrySession(entry) != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	return carrierResponse(out, err, s.expected, b.Operation, b.Command.Mount, nil, false)
}

func (s *Service) carrierControl(ctx context.Context, id []byte, mount values.ElementID, command *contract.CarrierCommand, expected values.PublicToken) (*pb.CarrierTransitionResponse, error) {
	ctx, caller, entry, cancel, err := s.controlContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if len(id) == 0 || len(id) > 128 || !mount.Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier key")
	}
	operation := contract.OperationID(id)
	var out contract.CarrierStatus
	switch {
	case command != nil:
		out, err = s.owner.BeginCarrierTransition(ctx, caller, operation, *command)
	case expected != (values.PublicToken{}):
		out, err = s.owner.CancelCarrierTransition(ctx, caller, operation, mount, expected)
	default:
		out, err = s.owner.InspectCarrierTransition(ctx, caller, operation, mount)
	}
	if ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if s.sessions.CheckEntrySession(entry) != nil {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	return carrierResponse(out, err, s.expected, operation, mount, command, expected != (values.PublicToken{}))
}

func carrierResponse(r contract.CarrierStatus, err error, parent values.FactAuthorityRef, id contract.OperationID, mount values.ElementID, command *contract.CarrierCommand, cancel bool) (*pb.CarrierTransitionResponse, error) {
	if err != nil {
		failure, err := rpcfailure.Transition(err, command != nil || cancel)
		if err != nil {
			return nil, err
		}
		return &pb.CarrierTransitionResponse{Outcome: &pb.CarrierTransitionResponse_Error{Error: failure}}, nil
	}
	if r.Parent != parent || r.Operation != id || r.Mount != mount || !r.Source.Valid() || !r.Target.Valid() || r.Source == parent || r.Target == parent || r.Source == r.Target ||
		r.Progress < contract.CarrierReserved || r.Progress > contract.CarrierSettled || !r.Revision.Valid() || r.Revision.Owner != parent || r.Revision.Kind != "carrier-revision" ||
		(command != nil && (r.Source != command.Source || r.Target != command.Target || r.Progress != contract.CarrierReserved)) || (cancel && r.Progress != contract.CarrierCancelled) {
		return nil, status.Error(codes.Internal, "invalid carrier result")
	}
	out := &pb.CarrierStatus{Parent: proto.String(string(parent)), Source: proto.String(string(r.Source)), Target: proto.String(string(r.Target)),
		OperationId: []byte(id), Mount: proto.String(string(mount)), Progress: pb.CarrierProgress(r.Progress).Enum(), Revision: wire.ControlToken(r.Revision, netpb.TokenKind_CARRIER_REVISION)}
	return &pb.CarrierTransitionResponse{Outcome: &pb.CarrierTransitionResponse_Result{Result: out}}, nil
}
