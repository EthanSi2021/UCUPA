package transition

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) RecoverTransition(ctx context.Context, r *pb.RecoverTransitionRequest) (*pb.TransitionResponse, error) {
	if r == nil || len(r.ApprovalToken) != 32 || proto.Size(r) > 4096 {
		return nil, status.Error(codes.InvalidArgument, "invalid recovery request")
	}
	if s.targets == nil {
		return nil, status.Error(codes.Unimplemented, "designated transition connections unavailable")
	}
	b, err := wire.TransitionBinding(r.Binding, values.FactAuthorityRef(r.GetBinding().GetCommand().GetTarget()))
	if err != nil || b.Source != s.expected {
		return nil, status.Error(codes.InvalidArgument, "invalid recovery binding")
	}
	return s.control(ctx, []byte(b.Operation), b.Command.Boundary, &b.Command, values.PublicToken{}, r)
}

func (s *Service) recover(ctx context.Context, caller values.CallerContext, r *pb.RecoverTransitionRequest) (contract.TransitionStatus, error) {
	b, err := wire.TransitionBinding(r.Binding, values.FactAuthorityRef(r.GetBinding().GetCommand().GetTarget()))
	if err != nil {
		return contract.TransitionStatus{}, values.PublicInvalid
	}
	target, err := s.targets.Target(b, r.ApprovalToken)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	// No workflow or network wait is introduced here: the original operations
	// owner performs one bounded advance and reloads its durable original input.
	return s.owner.RecoverTransition(ctx, caller, b, target)
}
