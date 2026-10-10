package admission

import (
	"context"
	"crypto/sha256"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	wire "ucupa/ucupa-realm/transport/grpc/internal/wire"
)

type ownerKey struct{}

// intercept enforces the pinned target, admission capacity and the declared
// wire shape before any handler runs. Protected methods additionally require
// the admitted mTLS service binding; the read-only target check stays open.
func (s *Registry) Intercept(ctx context.Context, in any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	target := md.Get("ucupa-realm-target")
	if len(target) != 1 || target[0] == "" {
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
	s.gateMu.Lock()
	stopped := s.stopped
	s.gateMu.Unlock()
	if stopped {
		return nil, status.Error(codes.Unavailable, "service stopped")
	}
	message, ok := in.(proto.Message)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid message")
	}
	if _, err := wire.Representation.ToValue(message.ProtoReflect(), 0); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid message")
	}
	if info.FullMethod != realmpb.RealmTargetService_CheckTarget_FullMethodName {
		owner, err := s.admittedOwner(ctx)
		if err != nil {
			return nil, err
		}
		ctx = WithOwner(ctx, owner)
	}
	return handler(ctx, in)
}

// admittedOwner resolves the mTLS service binding to the one fact owner this
// client may act for. Service admission and user permission stay separate
// decisions; a missing or unknown binding is rejected without detail.
func (s *Registry) admittedOwner(ctx context.Context) (values.FactAuthorityRef, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p == nil || p.AuthInfo == nil {
		return values.FactAuthorityRef(""), status.Error(codes.PermissionDenied, "service binding required")
	}
	info, isTLS := p.AuthInfo.(credentials.TLSInfo)
	if !isTLS || len(info.State.PeerCertificates) == 0 {
		return values.FactAuthorityRef(""), status.Error(codes.PermissionDenied, "service binding required")
	}
	spki := sha256.Sum256(info.State.PeerCertificates[0].RawSubjectPublicKeyInfo)
	owner, admitted := s.config.AdmittedServices[spki]
	if !admitted {
		return values.FactAuthorityRef(""), status.Error(codes.PermissionDenied, "service binding required")
	}
	return owner, nil
}

func OwnerFromContext(ctx context.Context) (values.FactAuthorityRef, error) {
	owner, ok := ctx.Value(ownerKey{}).(values.FactAuthorityRef)
	if !ok || !owner.Valid() {
		return values.FactAuthorityRef(""), status.Error(codes.PermissionDenied, "service binding required")
	}
	return owner, nil
}
func WithOwner(ctx context.Context, owner values.FactAuthorityRef) context.Context {
	return context.WithValue(ctx, ownerKey{}, owner)
}
