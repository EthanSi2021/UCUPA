package wire

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
)

func CarrierCommand(r *pb.BeginCarrierTransitionRequest, parent values.FactAuthorityRef) (contract.CarrierCommand, error) {
	bad := func() (contract.CarrierCommand, error) {
		return contract.CarrierCommand{}, status.Error(codes.InvalidArgument, "invalid carrier command")
	}
	if r == nil || proto.Size(r) > 4096 || !parent.Valid() || len(r.OperationId) == 0 || len(r.OperationId) > 128 {
		return bad()
	}
	c := contract.CarrierCommand{Source: values.FactAuthorityRef(r.GetSource()), Target: values.FactAuthorityRef(r.GetTarget()), Mount: values.ElementID(r.GetMount()),
		ParentAddress: values.NextHop(r.GetParentAddress()), SourceAddress: values.NextHop(r.GetSourceAddress()), TargetAddress: values.NextHop(r.GetTargetAddress())}
	if !c.Source.Valid() || !c.Target.Valid() || !c.Mount.Valid() || c.Source == c.Target || c.Source == parent || c.Target == parent ||
		!c.ParentAddress.Valid() || !c.SourceAddress.Valid() || !c.TargetAddress.Valid() {
		return bad()
	}
	e, m := r.ExpectedElement, r.ExpectedMount
	if e == nil || m == nil || e.GetOwner() != string(parent) || m.GetOwner() != string(parent) ||
		e.GetKind() != netpb.TokenKind_RECORD_REVISION || m.GetKind() != netpb.TokenKind_MOUNT_REVISION || len(e.Value) != 32 || len(m.Value) != 32 {
		return bad()
	}
	c.ExpectedElement = values.PublicToken{Owner: parent, Kind: "record-revision", Value: values.EncodePublicBytes(e.Value)}
	c.ExpectedMount = values.PublicToken{Owner: parent, Kind: "mount-revision", Value: values.EncodePublicBytes(m.Value)}
	return c, nil
}

func CarrierBinding(w *pb.CarrierApprovalBinding) (contract.CarrierApprovalBinding, error) {
	if w == nil || proto.Size(w) > 4096 {
		return contract.CarrierApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid carrier binding")
	}
	parent := values.FactAuthorityRef(w.GetParent())
	c, err := CarrierCommand(w.Command, parent)
	if err != nil {
		return contract.CarrierApprovalBinding{}, err
	}
	return contract.CarrierApprovalBinding{Parent: parent, Operation: contract.OperationID(w.Command.OperationId), Command: c}, nil
}
