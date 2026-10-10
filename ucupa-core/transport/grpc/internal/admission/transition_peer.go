package admission

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"ucupa/shared/values"
)

// CheckTransitionPeer admits a fixed service identity, not an end-user grant.
// The receiving transfer operation separately checks its exact durable binding.
func (s *Service) CheckTransitionPeer(ctx context.Context, actor values.FactAuthorityRef) error {
	if ctx == nil || ctx.Err() != nil {
		return status.Error(codes.Unavailable, "request unavailable")
	}
	if s.config.ReadDelegation == nil {
		return status.Error(codes.Unimplemented, "transition peer disabled")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || !actor.Valid() || actor == s.config.Expected || !s.authenticatedActor(ctx, actor) {
		return status.Error(codes.Unauthenticated, "authenticated transition peer required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return status.Error(codes.Unavailable, "service stopped")
	}
	return nil
}
