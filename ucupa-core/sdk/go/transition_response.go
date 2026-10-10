package coresdk

import (
	"bytes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func validateTransitionRecoveryRequest(in any, owner values.FactAuthorityRef) error {
	r, ok := in.(*pb.RecoverTransitionRequest)
	if !ok {
		return nil
	}
	b := r.GetBinding()
	command := b.GetCommand()
	if b.GetSource() != string(owner) || len(r.GetApprovalToken()) != 32 || command.GetExpected().GetOwner() != string(owner) ||
		len(command.GetOperationId()) == 0 || len(command.GetOperationId()) > 128 || command.GetTarget() == string(owner) ||
		!values.FactAuthorityRef(command.GetTarget()).Valid() || !values.ElementID(command.GetBoundary()).Valid() {
		return status.Error(codes.InvalidArgument, "invalid recovery binding")
	}
	return nil
}

// This validates disclosure binding, not authorization or transfer decisions.
// The common interceptor already checks required fields, enums and token kinds.
func validateTransitionExchange(in, out any, owner values.FactAuthorityRef) error {
	r, ok := out.(*pb.TransitionResponse)
	if !ok {
		return nil
	}
	var id []byte
	var boundary string
	var begin *pb.BeginTransitionRequest
	cancel := false
	switch q := in.(type) {
	case *pb.BeginTransitionRequest:
		id, boundary, begin = q.GetOperationId(), q.GetBoundary(), q
	case *pb.InspectTransitionRequest:
		id, boundary = q.GetOperationId(), q.GetBoundary()
	case *pb.CancelTransitionRequest:
		id, boundary, cancel = q.GetOperationId(), q.GetBoundary(), true
	case *pb.RecoverTransitionRequest:
		begin = q.GetBinding().GetCommand()
		id, boundary = begin.GetOperationId(), begin.GetBoundary()
		if q.GetBinding().GetSource() != string(owner) || len(q.ApprovalToken) != 32 {
			return status.Error(codes.DataLoss, "invalid recovery binding")
		}
	default:
		return status.Error(codes.DataLoss, "unexpected transition response")
	}
	if r.GetResult() == nil {
		return nil
	} // Error vocabulary checked by the common validator.
	v := r.GetResult()
	if len(id) == 0 || len(id) > 128 || !values.ElementID(boundary).Valid() || v.GetSource() != string(owner) ||
		!bytes.Equal(v.OperationId, id) || v.GetBoundary() != boundary || !values.FactAuthorityRef(v.GetTarget()).Valid() || v.GetTarget() == string(owner) ||
		(begin != nil && (v.GetKind() != begin.GetKind() || v.GetTarget() != begin.GetTarget())) ||
		(cancel && v.GetProgress() != pb.TransitionProgress_TRANSITION_CANCELLED) {
		return status.Error(codes.DataLoss, "transition response does not match request")
	}
	return nil
}
