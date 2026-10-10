package coresdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func validateTransitionTargetRequest(in any, target, actor values.FactAuthorityRef) error {
	var b *pb.TransitionApprovalBinding
	switch q := in.(type) {
	case *pb.AcceptTransitionRequest:
		b = q.GetBinding()
		if len(q.GetApprovalToken()) != 32 {
			return status.Error(codes.InvalidArgument, "invalid target approval token")
		}
	case *pb.InstallTransitionRequest:
		b = q.GetBinding()
	default:
		return nil
	}
	if !actor.Valid() || b.GetSource() != string(actor) || b.GetCommand().GetTarget() != string(target) || actor == target {
		return status.Error(codes.Unauthenticated, "wrong transition peer")
	}
	return nil
}

func validateTransitionTargetExchange(in, out any) error {
	bad := func() error { return status.Error(codes.DataLoss, "transition result does not match request") }
	switch q := in.(type) {
	case *pb.AcceptTransitionRequest:
		r, ok := out.(*pb.AcceptTransitionResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && (!proto.Equal(v.Binding, q.Binding) || !validSourceFence(v.Fence) || v.GetAcceptanceRevision() == 0) {
			return bad()
		}
	case *pb.InstallTransitionRequest:
		r, ok := out.(*pb.InstallTransitionResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil {
			d := v.Decision
			if d == nil || !proto.Equal(d.Binding, q.Binding) || !validSourceFence(d.Fence) || d.GetAcceptanceRevision() == 0 || d.GetDecisionRevision() == 0 ||
				v.GetInstallationRevision() == 0 || v.GetCompletionRevision() == 0 {
				return bad()
			}
		}
	}
	return nil
}
