package admission

import (
	"context"
	"crypto/sha256"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) mountPeerActor(ctx context.Context, actor values.FactAuthorityRef) (MountOwner, error) {
	o := s.config.MountControl
	if o == nil || s.config.ReadDelegation == nil {
		return nil, status.Error(codes.Unimplemented, "mount peer disabled")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || !actor.Valid() || actor == s.config.Expected || !s.authenticatedActor(ctx, actor) {
		return nil, status.Error(codes.Unauthenticated, "authenticated mount peer required")
	}
	return o, nil
}

func (s *Service) PrepareMount(ctx context.Context, in *pb.PrepareMountRequest) (*pb.PrepareMountResponse, error) {
	if in == nil || in.Request == nil || in.Approval == nil {
		return nil, status.Error(codes.InvalidArgument, "mount request required")
	}
	r, err := wire.MountRequest(in.Request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid mount request")
	}
	o, err := s.mountPeerActor(ctx, r.Key.ParentAuthority)
	if err != nil {
		return nil, err
	}
	a, err := s.checkedMountApproval(ctx, in.Approval)
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.PrepareMountResponse{Outcome: &pb.PrepareMountResponse_Failure{Failure: f}}, e
	}
	digest, err := o.CommandCommitment(r.Key.ParentAuthority, r.Key.CarrierAuthority, r.OperationID, r.Command)
	if err != nil || r.Key.CarrierAuthority != s.config.Expected || r.CarrierActor != a.caller.Subject || string(r.OperationID) != string(a.binding.OperationId) || string(r.Key.MountAnchorUUID) != a.binding.GetElement() || string(r.Key.ParentAuthority) != a.binding.GetActor() || digest != [32]byte(a.binding.CommandDigest) {
		return nil, status.Error(codes.InvalidArgument, "approval does not bind mount request")
	}
	// Serialize local approval revocation with the bounded owner commit. No
	// network call occurs under this lock; the owner validates the original
	// credential and authorization view again under its own commit lock.
	s.mu.Lock()
	current, ok := s.approvals[sha256.Sum256(in.Approval.Token)]
	if s.stopped || !ok || current.caller != a.caller || current.view != a.view || !time.Now().Before(a.expires) {
		s.mu.Unlock()
		return nil, status.Error(codes.Unauthenticated, "approval unavailable")
	}
	bounded, cancel := context.WithDeadline(ctx, a.expires)
	prepared, err := o.PrepareApprovedMountControl(bounded, a.caller, r.Key.ParentAuthority, r, a.view, digest)
	cancel()
	s.mu.Unlock()
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.PrepareMountResponse{Outcome: &pb.PrepareMountResponse_Failure{Failure: f}}, e
	}
	if prepared.Request != r {
		return nil, status.Error(codes.Internal, "mount request mismatch")
	}
	return &pb.PrepareMountResponse{Outcome: &pb.PrepareMountResponse_Result{Result: &pb.ServiceMountPrepared{Request: wire.MountRequestWire(prepared.Request), Prepared: proto.Bool(prepared.Prepared), Rejection: pb.MountRejection(prepared.Rejection + 1).Enum(), Cut: wire.MountCutWire(prepared.Cut)}}}, nil
}

func (s *Service) FinishMount(ctx context.Context, in *pb.FinishMountRequest) (*pb.FinishMountResponse, error) {
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "decision required")
	}
	evidence, err := wire.MountDecision(in.Evidence)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid decision")
	}
	o, err := s.mountPeerActor(ctx, evidence.Request.Key.ParentAuthority)
	if err != nil {
		return nil, err
	}
	r, err := o.MountPeer().Finish(ctx, evidence.Request.Key.ParentAuthority, corecontract.MountFinishRequest{Evidence: evidence})
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.FinishMountResponse{Outcome: &pb.FinishMountResponse_Failure{Failure: f}}, e
	}
	return &pb.FinishMountResponse{Outcome: &pb.FinishMountResponse_Result{Result: &pb.ServiceMountClosed{Evidence: wire.MountDecisionWire(r.Evidence), Cut: wire.MountCutWire(r.Cut)}}}, nil
}

func (s *Service) ReadMountDecision(ctx context.Context, in *pb.ReadMountDecisionRequest) (*pb.ReadMountDecisionResponse, error) {
	if in == nil || in.GetParentCore() != string(s.config.Expected) || len(in.OperationId) == 0 || len(in.OperationId) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid mount attempt")
	}
	// The selector must match the pinned TLS key; it is not itself proof.
	actor := values.FactAuthorityRef(in.GetActor())
	o, err := s.mountPeerActor(ctx, actor)
	if err != nil {
		return nil, err
	}
	r, err := o.MountPeer().ReadDecision(ctx, actor, corecontract.MountAttemptKey{ParentAuthority: s.config.Expected, OperationID: corecontract.OperationID(in.OperationId)})
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.ReadMountDecisionResponse{Outcome: &pb.ReadMountDecisionResponse_Failure{Failure: f}}, e
	}
	return &pb.ReadMountDecisionResponse{Outcome: &pb.ReadMountDecisionResponse_Result{Result: wire.MountDecisionWire(r)}}, nil
}
