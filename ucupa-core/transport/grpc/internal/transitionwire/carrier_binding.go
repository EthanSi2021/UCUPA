package transitionwire

import (
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func CarrierBindingWire(b contract.CarrierApprovalBinding) (*pb.CarrierApprovalBinding, error) {
	c := b.Command
	e, err := values.DecodePublicBytes(c.ExpectedElement.Value)
	if err != nil || c.ExpectedElement.Kind != "record-revision" {
		return nil, contract.ErrInvalid
	}
	m, err := values.DecodePublicBytes(c.ExpectedMount.Value)
	if err != nil || c.ExpectedMount.Kind != "mount-revision" {
		return nil, contract.ErrInvalid
	}
	w := &pb.CarrierApprovalBinding{Parent: proto.String(string(b.Parent)), Command: &pb.BeginCarrierTransitionRequest{
		OperationId: []byte(b.Operation), Source: proto.String(string(c.Source)), Target: proto.String(string(c.Target)), Mount: proto.String(string(c.Mount)),
		ExpectedElement: &netpb.Token{Owner: proto.String(string(c.ExpectedElement.Owner)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: e},
		ExpectedMount:   &netpb.Token{Owner: proto.String(string(c.ExpectedMount.Owner)), Kind: netpb.TokenKind_MOUNT_REVISION.Enum(), Value: m},
		ParentAddress:   proto.String(string(c.ParentAddress)), SourceAddress: proto.String(string(c.SourceAddress)), TargetAddress: proto.String(string(c.TargetAddress)),
	}}
	if actual, err := wire.CarrierBinding(w); err != nil || actual != b {
		return nil, contract.ErrInvalid
	}
	return w, nil
}
