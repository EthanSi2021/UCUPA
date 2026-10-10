package mount

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func publicMountContactWire(r corecontract.PublicMountContact) *pb.PublicMountContact {
	return &pb.PublicMountContact{Element: proto.String(string(r.Element)), Incoming: proto.Bool(r.Incoming), PeerCore: proto.String(string(r.Peer)), NextHop: proto.String(string(r.NextHop)), Revision: wire.ControlToken(r.Revision, netpb.TokenKind_CONTACT_REVISION)}
}
func (s *Service) InspectMountContact(ctx context.Context, in *pb.InspectMountContactRequest) (*pb.InspectMountContactResponse, error) {
	o, caller, err := s.mountController(ctx)
	if err != nil {
		return nil, err
	}
	r, err := o.InspectMountContact(ctx, caller, values.ElementID(in.GetElement()), in.GetIncoming())
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.InspectMountContactResponse{Outcome: &pb.InspectMountContactResponse_Failure{Failure: f}}, e
	}
	return &pb.InspectMountContactResponse{Outcome: &pb.InspectMountContactResponse_Result{Result: publicMountContactWire(r)}}, nil
}
func (s *Service) RepairMountContact(ctx context.Context, in *pb.RepairMountContactRequest) (*pb.RepairMountContactResponse, error) {
	o, caller, err := s.mountController(ctx)
	if err != nil {
		return nil, err
	}
	w := in.GetExpected()
	if w == nil || w.GetKind() != netpb.TokenKind_CONTACT_REVISION || len(w.Value) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid contact comparison")
	}
	expected, err := values.PublicTokenFromBytes(values.FactAuthorityRef(w.GetOwner()), "contact-revision", [32]byte(w.Value))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid contact comparison")
	}
	r, err := o.RepairMountContact(ctx, caller, corecontract.OperationID(in.OperationId), values.ElementID(in.GetElement()), in.GetIncoming(), expected, values.NextHop(in.GetNextHop()))
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.RepairMountContactResponse{Outcome: &pb.RepairMountContactResponse_Failure{Failure: f}}, e
	}
	return &pb.RepairMountContactResponse{Outcome: &pb.RepairMountContactResponse_Result{Result: publicMountContactWire(r)}}, nil
}
