package admission

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

// EntrySession is a call-local witness, not a second registry or identity token.
// Only this package can construct or inspect its original session binding.
type EntrySession struct {
	key   [32]byte
	entry session
}

func (entry EntrySession) Expires() time.Time { return entry.entry.expires }

func (s *Service) BindEntrySession(ctx context.Context, caller values.CallerContext) (EntrySession, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	key, err := sessionKey(md)
	if err != nil {
		return EntrySession{}, values.PublicDenied
	}
	s.mu.Lock()
	s.expireLocked(time.Now())
	entry, found := s.sessions[key]
	live := found && entry.caller == caller && !s.stopped
	s.mu.Unlock()
	if !live {
		return EntrySession{}, values.PublicDenied
	}
	return EntrySession{key: key, entry: entry}, nil
}

func (s *Service) CheckEntrySession(entry EntrySession) error {
	s.mu.Lock()
	s.expireLocked(time.Now())
	current, found := s.sessions[entry.key]
	live := found && current == entry.entry && !s.stopped
	s.mu.Unlock()
	if !live {
		return values.PublicDenied
	}
	return nil
}

// CheckDelegatedResult is the original post-owner disclosure check. A normal
// caller has no grant and gains no additional permission by this operation.
func (s *Service) CheckDelegatedResult(ctx context.Context, caller values.CallerContext) error {
	if key, ok := grpcsession.Grant(ctx); ok {
		if err := s.checkGrantIdentity(ctx, key); err != nil {
			return err
		}
		if !s.grantAlive(key, caller) {
			return status.Error(codes.Unauthenticated, "delegation unavailable")
		}
	}
	return nil
}
