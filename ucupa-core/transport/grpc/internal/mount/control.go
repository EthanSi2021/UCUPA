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
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) mountController(ctx context.Context) (Owner, values.CallerContext, error) {
	o := s.owner
	if o == nil || s.connections == nil {
		return nil, values.CallerContext{}, status.Error(codes.Unimplemented, "mount control disabled")
	}
	c, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, c, status.Error(codes.Unauthenticated, "session required")
	}
	if _, delegated := grpcsession.Grant(ctx); delegated {
		return nil, c, status.Error(codes.PermissionDenied, "read delegation cannot control mounts")
	}
	return o, c, nil
}

func publicMountCommand(w *pb.PublicMountCommand) (corecontract.PublicMountCommand, error) {
	if w == nil {
		return corecontract.PublicMountCommand{}, corecontract.ErrInvalid
	}
	c := corecontract.PublicMountCommand{Kind: corecontract.MountCommandKind(w.GetKind()), Element: values.ElementID(w.GetElement()), Carrier: values.FactAuthorityRef(w.GetCarrierCore()), CarrierNextHop: values.NextHop(w.GetCarrierNextHop()), ParentNextHop: values.NextHop(w.GetParentNextHop())}
	token := func(w *netpb.Token, kind netpb.TokenKind, literal string) (values.PublicToken, error) {
		if w == nil {
			return values.PublicToken{}, nil
		}
		if w.GetKind() != kind || len(w.Value) != 32 {
			return values.PublicToken{}, corecontract.ErrInvalid
		}
		return values.PublicTokenFromBytes(values.FactAuthorityRef(w.GetOwner()), literal, [32]byte(w.Value))
	}
	var err error
	if c.ExpectedElement, err = token(w.ExpectedElement, netpb.TokenKind_RECORD_REVISION, "record-revision"); err != nil {
		return c, err
	}
	if c.ExpectedMount, err = token(w.ExpectedMount, netpb.TokenKind_MOUNT_REVISION, "mount-revision"); err != nil {
		return c, err
	}
	if c.Kind != corecontract.MountEstablish && (w.CarrierCore != nil || w.CarrierNextHop != nil || w.ParentNextHop != nil) {
		return c, corecontract.ErrInvalid
	}
	if c.Kind != corecontract.MountUpdate && w.Final != nil {
		return c, corecontract.ErrInvalid
	}
	if w.Final != nil {
		c.Final, err = wire.MountTuple(w.Final)
	}
	return c, err
}

func (s *Service) InspectMount(ctx context.Context, in *pb.InspectMountRequest) (*pb.InspectMountResponse, error) {
	o, caller, err := s.mountController(ctx)
	if err != nil {
		return nil, err
	}
	r, err := o.InspectMountControl(ctx, caller, values.ElementID(in.GetElement()))
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.InspectMountResponse{Outcome: &pb.InspectMountResponse_Failure{Failure: f}}, e
	}
	w := &pb.PublicMountInspection{Element: proto.String(string(r.Element)), ElementRevision: wire.ControlToken(r.ElementRevision, netpb.TokenKind_RECORD_REVISION), MountRevision: wire.ControlToken(r.MountRevision, netpb.TokenKind_MOUNT_REVISION), State: proto.Uint32(uint32(r.State))}
	if r.Carrier.Valid() {
		w.CarrierCore = proto.String(string(r.Carrier))
	}
	if r.NextHop.Valid() {
		w.NextHop = proto.String(string(r.NextHop))
	}
	return &pb.InspectMountResponse{Outcome: &pb.InspectMountResponse_Result{Result: w}}, nil
}

func (s *Service) DescribeMount(ctx context.Context, in *pb.DescribeMountRequest) (*pb.DescribeMountResponse, error) {
	o, caller, err := s.mountController(ctx)
	if err != nil {
		return nil, err
	}
	c, err := publicMountCommand(in.GetCommand())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid mount command")
	}
	target, digest, required, err := o.DescribeMountApproval(ctx, caller, corecontract.OperationID(in.OperationId), c)
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.DescribeMountResponse{Outcome: &pb.DescribeMountResponse_Failure{Failure: f}}, e
	}
	w := &pb.MountApprovalDescription{Required: proto.Bool(required)}
	if required {
		w.Target = proto.String(string(target))
		w.Binding = &pb.MountApprovalBinding{Actor: proto.String(string(s.expected)), Element: proto.String(string(c.Element)), OperationId: append([]byte(nil), in.OperationId...), CommandDigest: digest[:]}
	}
	return &pb.DescribeMountResponse{Outcome: &pb.DescribeMountResponse_Result{Result: w}}, nil
}

func (s *Service) ApplyMount(ctx context.Context, in *pb.ApplyMountRequest) (*pb.ApplyMountResponse, error) {
	o, caller, err := s.mountController(ctx)
	if err != nil {
		return nil, err
	}
	c, err := publicMountCommand(in.GetCommand())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid mount command")
	}
	command, view, release, err := o.ResolveMountControl(ctx, caller, corecontract.OperationID(in.OperationId), c)
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.ApplyMountResponse{Outcome: &pb.ApplyMountResponse_Failure{Failure: f}}, e
	}
	defer release()
	connection := s.connections
	if in.Approval != nil {
		connection, err = connection.WithApproval(in.Approval)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid mount approval")
		}
	}
	r, err := o.ApplyMountControl(ctx, caller, view, corecontract.OperationID(in.OperationId), command, connection)
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.ApplyMountResponse{Outcome: &pb.ApplyMountResponse_Failure{Failure: f}}, e
	}
	w := &pb.PublicMountOutcome{OperationId: []byte(r.OperationID), Element: proto.String(string(r.MountAnchorUUID)), Decision: pb.MountDecisionKind(r.Decision).Enum(), Settled: proto.Bool(r.Settled), Changed: proto.Bool(r.Changed), Rejection: pb.MountRejection(r.AbortCause + 1).Enum(), BlockedBy: proto.Uint32(uint32(r.BlockedBy))}
	return &pb.ApplyMountResponse{Outcome: &pb.ApplyMountResponse_Result{Result: w}}, nil
}
