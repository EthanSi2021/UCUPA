package transitionwire

import (
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func BindingWire(b contract.TransitionApprovalBinding) (*pb.TransitionApprovalBinding, error) {
	c := b.Command
	raw, err := values.DecodePublicBytes(c.Expected.Value)
	if err != nil || c.Expected.Kind != "record-revision" {
		return nil, contract.ErrInvalid
	}
	w := &pb.TransitionApprovalBinding{Source: proto.String(string(b.Source)), Command: &pb.BeginTransitionRequest{
		OperationId: []byte(b.Operation), Kind: pb.TransitionKind(c.Kind).Enum(), Target: proto.String(string(c.Target)), Boundary: proto.String(string(c.Boundary)),
		Expected:      &netpb.Token{Owner: proto.String(string(c.Expected.Owner)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: raw},
		SourceAddress: proto.String(string(c.SourceAddress)), TargetAddress: proto.String(string(c.TargetAddress)),
	}}
	if c.Kind == contract.TransitionSlice {
		w.Command.Final = wire.MountTupleWire(c.Final)
	} else if c.Final != (contract.SpatialTuple{}) {
		return nil, contract.ErrInvalid
	}
	actual, err := wire.TransitionBinding(w, c.Target)
	if err != nil || actual != b || proto.Size(w) > 4096 {
		return nil, contract.ErrInvalid
	}
	return w, nil
}
