package transition

import (
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

func decodeToken(t *netpb.Token, kind netpb.TokenKind, literal string, owner values.FactAuthorityRef) (values.PublicToken, error) {
	if t == nil || t.GetKind() != kind || t.GetOwner() != string(owner) || len(t.Value) != 32 {
		return values.PublicToken{}, status.Error(codes.InvalidArgument, "invalid transition comparison")
	}
	return values.PublicToken{Owner: owner, Kind: literal, Value: values.EncodePublicBytes(t.Value)}, nil
}

func decodeCommand(r *pb.BeginTransitionRequest, owner values.FactAuthorityRef) (contract.TransitionCommand, error) {
	return wire.TransitionCommand(r, owner)
}

func response(r contract.TransitionStatus, err error, owner values.FactAuthorityRef, id contract.OperationID, boundary values.ElementID, command *contract.TransitionCommand, cancel, recover bool) (*pb.TransitionResponse, error) {
	if err != nil {
		failure, err := rpcfailure.Transition(err, command != nil || cancel || recover)
		if err != nil {
			return nil, err
		}
		return &pb.TransitionResponse{Outcome: &pb.TransitionResponse_Error{Error: failure}}, nil
	}
	if r.Source != owner || r.Operation != id || r.Boundary != boundary || !r.Target.Valid() || r.Target == owner ||
		(r.Kind != contract.TransitionSlice && r.Kind != contract.TransitionRoot) || r.Progress < contract.TransitionIntent || r.Progress > contract.TransitionSettled ||
		!r.Revision.Valid() || r.Revision.Owner != owner || r.Revision.Kind != "transition-revision" ||
		(command != nil && (r.Kind != command.Kind || r.Target != command.Target)) || (cancel && r.Progress != contract.TransitionCancelled) {
		return nil, status.Error(codes.Internal, "invalid transition result")
	}
	out := &pb.TransitionStatus{Source: proto.String(string(r.Source)), OperationId: []byte(r.Operation), Kind: pb.TransitionKind(r.Kind).Enum(),
		Target: proto.String(string(r.Target)), Boundary: proto.String(string(r.Boundary)), Progress: pb.TransitionProgress(r.Progress).Enum(), Revision: wire.ControlToken(r.Revision, netpb.TokenKind_TRANSITION_REVISION)}
	return &pb.TransitionResponse{Outcome: &pb.TransitionResponse_Result{Result: out}}, nil
}
