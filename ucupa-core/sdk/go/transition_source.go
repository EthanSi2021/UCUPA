package coresdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func validateTransitionSourceRequest(in any, source, actor values.FactAuthorityRef) error {
	var binding *pb.TransitionApprovalBinding
	switch q := in.(type) {
	case *pb.ReadTransitionChunkRequest:
		binding = q.GetBinding()
		if q.Offset == nil || q.MaxBytes == nil || q.GetMaxBytes() == 0 || q.GetMaxBytes() > 64<<10 || q.GetOffset() >= 1<<20 {
			return status.Error(codes.InvalidArgument, "invalid transition chunk bounds")
		}
	case *pb.ReadTransitionDecisionRequest:
		binding = q.GetBinding()
	default:
		return nil
	}
	if !actor.Valid() || binding.GetSource() != string(source) || binding.GetCommand().GetTarget() != string(actor) || source == actor {
		return status.Error(codes.Unauthenticated, "wrong transition peer")
	}
	return nil
}

// These are structural response checks only; the receiving owner must still
// validate the assembled manifest and its durable transfer relationship.
func validateTransitionSourceExchange(in, out any) error {
	bad := func() error { return status.Error(codes.DataLoss, "transition evidence does not match request") }
	switch q := in.(type) {
	case *pb.ReadTransitionChunkRequest:
		r, ok := out.(*pb.ReadTransitionChunkResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil {
			if !proto.Equal(v.Binding, q.Binding) || v.GetTotalBytes() == 0 || v.GetTotalBytes() > 1<<20 ||
				v.GetOffset() != q.GetOffset() || v.GetOffset() >= v.GetTotalBytes() || q.GetMaxBytes() == 0 || q.GetMaxBytes() > 64<<10 ||
				uint64(len(v.Data)) != min(uint64(q.GetMaxBytes()), v.GetTotalBytes()-v.GetOffset()) || !validSourceFence(v.Fence) {
				return bad()
			}
		}
	case *pb.ReadTransitionDecisionRequest:
		r, ok := out.(*pb.ReadTransitionDecisionResponse)
		if !ok || r == nil {
			return bad()
		}
		if v := r.GetResult(); v != nil && (!proto.Equal(v.Binding, q.Binding) || !validSourceFence(v.Fence) || v.GetAcceptanceRevision() == 0 || v.GetDecisionRevision() == 0) {
			return bad()
		}
	}
	return nil
}

func validSourceFence(v *pb.ServiceTransitionFence) bool {
	if v == nil || len(v.ManifestDigest) != 32 || len(v.GetExpected().GetIncarnation()) != 32 || v.GetExpected().GetSequence() == 0 ||
		len(v.GetFinalCut().GetIncarnation()) != 32 || v.GetFinalCut().GetSequence() == 0 {
		return false
	}
	a := v.GetActor()
	return len(a.GetRealm()) > 0 && len(a.GetPrincipal()) > 0 && len(a.GetRealm())+len(a.GetPrincipal()) <= 4096
}
