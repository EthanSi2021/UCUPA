package wire

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

// TransitionCommand decodes the same exact input at source and target. Its
// source-owned comparison is caller-supplied evidence, not target authority.
func TransitionCommand(r *pb.BeginTransitionRequest, source values.FactAuthorityRef) (contract.TransitionCommand, error) {
	bad := func() (contract.TransitionCommand, error) {
		return contract.TransitionCommand{}, status.Error(codes.InvalidArgument, "invalid transition command")
	}
	if r == nil || !source.Valid() || len(r.OperationId) == 0 || len(r.OperationId) > 128 {
		return bad()
	}
	c := contract.TransitionCommand{Kind: contract.TransitionKind(r.GetKind()), Target: values.FactAuthorityRef(r.GetTarget()), Boundary: values.ElementID(r.GetBoundary()), SourceAddress: values.NextHop(r.GetSourceAddress()), TargetAddress: values.NextHop(r.GetTargetAddress())}
	if !c.Target.Valid() || c.Target == source || !c.Boundary.Valid() || !c.SourceAddress.Valid() || !c.TargetAddress.Valid() {
		return bad()
	}
	t := r.Expected
	if t == nil || t.GetKind() != netpb.TokenKind_RECORD_REVISION || t.GetOwner() != string(source) || len(t.Value) != 32 {
		return bad()
	}
	c.Expected = values.PublicToken{Owner: source, Kind: "record-revision", Value: values.EncodePublicBytes(t.Value)}
	switch r.GetKind() {
	case pb.TransitionKind_TRANSITION_SLICE:
		var err error
		c.Final, err = MountTuple(r.Final)
		if err != nil {
			return bad()
		}
	case pb.TransitionKind_TRANSITION_ROOT:
		if r.Final != nil {
			return bad()
		}
	default:
		return bad()
	}
	return c, nil
}

func TransitionBinding(b *pb.TransitionApprovalBinding, target values.FactAuthorityRef) (contract.TransitionApprovalBinding, error) {
	if b == nil || b.GetCommand().GetTarget() != string(target) {
		return contract.TransitionApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid transition target")
	}
	source := values.FactAuthorityRef(b.GetSource())
	c, err := TransitionCommand(b.Command, source)
	if err != nil {
		return contract.TransitionApprovalBinding{}, err
	}
	return contract.TransitionApprovalBinding{Source: source, Operation: contract.OperationID(b.Command.OperationId), Command: c}, nil
}
