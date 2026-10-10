package admission

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func federationPartMethod(method string) bool {
	if method == pb.FederationReadService_ObserveFederationReadPart_FullMethodName {
		return true
	}
	return method == pb.FederationReadService_BeginFederationReadAtChallenge_FullMethodName || method == pb.FederationReadService_ProjectFederationReadPart_FullMethodName || method == pb.FederationReadService_CheckFederationReadPart_FullMethodName || method == pb.FederationReadService_ReleaseFederationReadPart_FullMethodName
}
func (s *Service) federationCaller(ctx context.Context, basis *netpb.Token) (FederationSource, readGrant, [32]byte, error) {
	o := s.config.Federation
	if o == nil {
		return nil, readGrant{}, [32]byte{}, status.Error(codes.Unimplemented, "federation reads disabled")
	}
	key, ok := grpcsession.Grant(ctx)
	caller, hasCaller := grpcsession.Caller(ctx)
	s.mu.Lock()
	g, found := s.grants[key]
	live := !s.stopped && time.Now().Before(g.expires)
	s.mu.Unlock()
	if !ok || !hasCaller || !found || !live || g.caller != caller || g.federation == nil || !proto.Equal(basis, g.federation.Basis) {
		return nil, readGrant{}, key, status.Error(codes.Unauthenticated, "federation grant required")
	}
	if err := s.checkGrantIdentity(ctx, key); err != nil {
		return nil, readGrant{}, key, err
	}
	return o, g, key, nil
}
func (s *Service) finishFederationCall(ctx context.Context, g readGrant, key [32]byte, out proto.Message) error {
	if err := s.checkGrantIdentity(ctx, key); err != nil {
		return err
	}
	if !s.grantAlive(key, g.caller) {
		return status.Error(codes.Unauthenticated, "federation grant unavailable")
	}
	if err := ctx.Err(); err != nil {
		return status.FromContextError(err).Err()
	}
	if proto.Size(out) > s.config.MaxResponseBytes {
		return status.Error(codes.ResourceExhausted, "response capacity")
	}
	return nil
}
func (s *Service) ProjectFederationReadPart(ctx context.Context, in *pb.ProjectFederationReadPartRequest) (*pb.ProjectFederationReadPartResponse, error) {
	o, g, key, err := s.federationCaller(ctx, in.GetBasis())
	if err != nil {
		return nil, err
	}
	basis, err := wire.FederationToken(in.Basis, netpb.TokenKind_READ_RECEIPT, "read-receipt")
	if err != nil || in.GetLimit() == 0 || in.GetLimit() > 256 {
		return nil, status.Error(codes.InvalidArgument, "invalid read page")
	}
	request, err := wire.FederationProjectionRequest(in.Request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid projection")
	}
	readIdentity, err := s.consumeFederationReadAtProof(ctx, g, key, in)
	if err != nil {
		return nil, err
	}
	if readIdentity != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, readIdentity.deadline)
		defer cancel()
	}
	var continuation *values.PublicToken
	if in.Continuation != nil {
		t, e := wire.FederationToken(in.Continuation, netpb.TokenKind_QUERY_CONTINUATION, "query-continuation")
		if e != nil || t.Owner != s.config.Expected {
			return nil, status.Error(codes.InvalidArgument, "invalid continuation")
		}
		continuation = &t
	}
	r, err := o.ProjectFederationReadPart(ctx, g.caller, g.actor, basis, request, continuation, uint16(in.GetLimit()))
	var out *pb.ProjectFederationReadPartResponse
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		if e != nil {
			return nil, e
		}
		out = &pb.ProjectFederationReadPartResponse{Outcome: &pb.ProjectFederationReadPartResponse_Failure{Failure: f}}
	} else {
		out = &pb.ProjectFederationReadPartResponse{Outcome: &pb.ProjectFederationReadPartResponse_Result{Result: wire.FederationPageWire(r)}}
	}
	if err := s.finishFederationCall(ctx, g, key, out); err != nil {
		return nil, err
	}
	if readIdentity != nil {
		if err := s.config.ExternalIdentity.Check(ctx, readIdentity.proof.Payload, readIdentity.proof.Signature, readIdentity.actor, readIdentity.challenge); err != nil {
			return nil, identityFailure(err)
		}
		if ctx.Err() != nil || !s.grantAlive(key, g.caller) {
			return nil, status.Error(codes.Unauthenticated, "read basis unavailable")
		}
	}
	return out, nil
}
func (s *Service) CheckFederationReadPart(ctx context.Context, in *pb.CheckFederationReadPartRequest) (*pb.CheckFederationReadPartResponse, error) {
	o, g, key, err := s.federationCaller(ctx, in.GetBasis())
	if err != nil {
		return nil, err
	}
	basis, err := wire.FederationToken(in.Basis, netpb.TokenKind_READ_RECEIPT, "read-receipt")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid basis")
	}
	err = o.CheckFederationReadPart(ctx, g.caller, g.actor, basis, in.GetRequireCurrent())
	var out *pb.CheckFederationReadPartResponse
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		if e != nil {
			return nil, e
		}
		out = &pb.CheckFederationReadPartResponse{Outcome: &pb.CheckFederationReadPartResponse_Failure{Failure: f}}
	} else {
		out = &pb.CheckFederationReadPartResponse{Outcome: &pb.CheckFederationReadPartResponse_Result{Result: &pb.FederationReadPartCheck{Valid: proto.Bool(true)}}}
	}
	if err := s.finishFederationCall(ctx, g, key, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *Service) ReleaseFederationReadPart(ctx context.Context, in *pb.FederationReadPartRequest) (*pb.ReleaseFederationReadPartResponse, error) {
	o, g, key, err := s.federationCaller(ctx, in.GetBasis())
	if err != nil {
		return nil, err
	}
	basis, err := wire.FederationToken(in.Basis, netpb.TokenKind_READ_RECEIPT, "read-receipt")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid basis")
	}
	err = o.ReleaseFederationReadPart(ctx, g.caller, g.actor, basis)
	var out *pb.ReleaseFederationReadPartResponse
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		if e != nil {
			return nil, e
		}
		out = &pb.ReleaseFederationReadPartResponse{Outcome: &pb.ReleaseFederationReadPartResponse_Failure{Failure: f}}
	} else {
		out = &pb.ReleaseFederationReadPartResponse{Outcome: &pb.ReleaseFederationReadPartResponse_Result{Result: &pb.FederationReadPartRelease{Released: proto.Bool(true)}}}
	}
	if err := s.finishFederationCall(ctx, g, key, out); err != nil {
		return nil, err
	}
	if err == nil {
		s.mu.Lock()
		delete(s.grants, key)
		s.mu.Unlock()
	}
	return out, nil
}
