package admission

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

// IdentityProofIssuer is trusted composition, not a fact-owner operation.
// It verifies credentials at issue/check and never returns permission or caller
// proofs on the wire. The transport must not manufacture a subject for it.
// Issue and Check carry the original request's context: under a network caller
// source the issuer's re-validation is a real remote wait.
type IdentityProofIssuer interface {
	Issue(context.Context, values.CallerContext, values.FactAuthorityRef, [32]byte, time.Duration) ([]byte, []byte, error)
	Check(context.Context, []byte, []byte) error
}

func identityFailure(err error) error {
	switch {
	case errors.Is(err, values.PublicUnavailable):
		return status.Error(codes.Unavailable, "identity unavailable")
	case errors.Is(err, values.PublicResourceExhausted):
		return status.Error(codes.ResourceExhausted, "identity capacity")
	case errors.Is(err, values.PublicInvalid):
		return status.Error(codes.InvalidArgument, "invalid identity request")
	default:
		return status.Error(codes.Unauthenticated, "identity unavailable")
	}
}

func (s *Service) IssueIdentityProof(ctx context.Context, in *networkpb.IssueIdentityProofRequest) (*networkpb.IdentityProof, error) {
	if s.config.IdentityProofs == nil {
		return nil, status.Error(codes.Unimplemented, "identity issuance disabled")
	}
	actor := values.FactAuthorityRef(in.GetActor())
	if !actor.Valid() || len(in.Challenge) != 32 || [32]byte(in.Challenge) == ([32]byte{}) || in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64((1<<63-1)/int64(time.Second)) {
		return nil, status.Error(codes.InvalidArgument, "invalid identity request")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	// A proof never outlives the original user session. Reconnecting does not
	// extend this upper bound, and delegated callers cannot reach this method.
	md, _ := metadata.FromIncomingContext(ctx)
	key, err := sessionKey(md)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	entry, exists := s.sessions[key]
	s.mu.Unlock()
	ttl := time.Duration(in.GetTtlSeconds()) * time.Second
	remaining := time.Until(entry.expires).Truncate(time.Second)
	if !exists || entry.caller != caller || remaining <= 0 {
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	if ttl > remaining {
		ttl = remaining
	}
	payload, signature, err := s.config.IdentityProofs.Issue(ctx, caller, actor, [32]byte(in.Challenge), ttl)
	if err != nil {
		return nil, identityFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return &networkpb.IdentityProof{Payload: payload, Signature: signature}, nil
}

func (s *Service) CheckIdentityProof(ctx context.Context, in *networkpb.CheckIdentityProofRequest) (*networkpb.CheckIdentityProofResponse, error) {
	if s.config.IdentityProofs == nil {
		return nil, status.Error(codes.Unimplemented, "identity issuance disabled")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	// This is an authenticated service check, not a user or delegation request.
	if len(md.Get("authorization")) != 0 || !s.authenticatedActor(ctx, values.FactAuthorityRef(in.GetVerifier())) {
		return nil, status.Error(codes.Unauthenticated, "service identity required")
	}
	p := in.GetProof()
	if p == nil || len(p.Payload) > 4096 || len(p.Signature) != 64 {
		return nil, identityFailure(values.PublicDenied)
	}
	if err := s.config.IdentityProofs.Check(ctx, p.Payload, p.Signature); err != nil {
		return nil, identityFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return &networkpb.CheckIdentityProofResponse{Valid: proto.Bool(true)}, nil
}
