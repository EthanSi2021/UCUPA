package admission

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) Intercept(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	target := md.Get("ucupa-target")
	if len(target) != 1 || !values.FactAuthorityRef(target[0]).Valid() {
		return nil, status.Error(codes.InvalidArgument, "invalid target")
	}
	if target[0] != string(s.config.Expected) {
		return nil, status.Error(codes.Unavailable, "target unavailable")
	}
	select {
	case s.active <- struct{}{}:
		defer func() { <-s.active }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "request capacity")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	s.mu.Lock()
	stopped := s.stopped
	if !stopped {
		// Add and the stop transition share the lock: Wait cannot race a new
		// admitted call after shutdown has observed an empty group.
		s.running.Add(1)
	}
	s.mu.Unlock()
	if stopped {
		return nil, status.Error(codes.Unavailable, "service stopped")
	}
	defer s.running.Done()
	message, ok := req.(proto.Message)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid message")
	}
	if _, err := wire.ToValue(message.ProtoReflect(), 0); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message")
	}
	if items := md.Get("authorization"); len(items) == 1 && strings.HasPrefix(items[0], "Delegation ") {
		caller, key, err := s.delegatedCaller(ctx, md, info.FullMethod, message)
		if err != nil {
			return nil, err
		}
		ctx = grpcsession.WithCaller(ctx, caller)
		ctx = grpcsession.WithGrant(ctx, key)
	} else if info.FullMethod != networkpb.SessionService_EstablishSession_FullMethodName && info.FullMethod != networkpb.SessionService_ReleaseSession_FullMethodName && !serviceIdentityMethod(info.FullMethod) {
		caller, err := s.caller(md)
		if err != nil {
			return nil, err
		}
		ctx = grpcsession.WithCaller(ctx, caller)
	}
	return handler(ctx, req)
}
