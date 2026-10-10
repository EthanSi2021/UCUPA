package coresdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func validateTransitionApprovalExchange(in, out any, target values.FactAuthorityRef) error {
	bad := func() error { return status.Error(codes.DataLoss, "transition approval does not match request") }
	switch q := in.(type) {
	case *pb.CreateTransitionApprovalRequest:
		r, ok := out.(*pb.CreateTransitionApprovalResponse)
		if !ok {
			return bad()
		}
		if v := r.GetResult(); v != nil && (v.GetTarget() != string(target) || !proto.Equal(v.Binding, q.Binding) || len(v.Token) != 32 || v.GetExpiresInSeconds() == 0 || v.GetExpiresInSeconds() > q.GetTtlSeconds()) {
			return bad()
		}
	case *pb.CheckTransitionApprovalRequest:
		r, ok := out.(*pb.CheckTransitionApprovalResponse)
		if !ok {
			return bad()
		}
		if v := r.GetResult(); v != nil {
			caller := values.PrincipalRef{Realm: values.RealmID(v.GetRealm()), Principal: values.PrincipalID(v.GetPrincipal())}
			if v.GetTarget() != string(target) || !proto.Equal(v.Binding, q.Binding) || !caller.Valid() {
				return bad()
			}
		}
	}
	return nil
}

// This is the echoed source comparison, not a target-signed token. Its owner
// must be the explicitly bound source; all other responses retain the normal
// same-owner rule. The exchange check also compares the entire echoed input.
func validateTransitionApprovalOwners(m protoreflect.Message, target values.FactAuthorityRef) (bool, error) {
	var binding *pb.TransitionApprovalBinding
	var actual string
	switch v := m.Interface().(type) {
	case *pb.TransitionApproval:
		binding, actual = v.Binding, v.GetTarget()
	case *pb.TransitionApprovalCheck:
		binding, actual = v.Binding, v.GetTarget()
	case *pb.TransitionAcceptance:
		binding, actual = v.Binding, v.GetBinding().GetCommand().GetTarget()
	case *pb.TransitionCompletion:
		binding = v.GetDecision().GetBinding()
		actual = binding.GetCommand().GetTarget()
	default:
		return false, nil
	}
	source := values.FactAuthorityRef(binding.GetSource())
	command := binding.GetCommand()
	if actual != string(target) || !source.Valid() || source == target || command.GetTarget() != actual || command.GetExpected() == nil || len(command.GetOperationId()) == 0 || len(command.GetOperationId()) > 128 {
		return true, status.Error(codes.DataLoss, "invalid approval source or target")
	}
	return true, validateTokenOwners(command.ProtoReflect(), source)
}
