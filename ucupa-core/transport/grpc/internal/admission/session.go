package admission

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

type session struct {
	caller   values.CallerContext
	expires  time.Time
	lifetime *sessionLifetime
}

func (s *Service) EstablishSession(ctx context.Context, in *networkpb.EstablishSessionRequest) (*networkpb.EstablishSessionResponse, error) {
	if len(in.CredentialId) != 16 || len(in.Secret) != 32 || proto.Size(in) > 4096 || in.GetAudience() == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication failed")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 {
		return nil, status.Error(codes.InvalidArgument, "unexpected session")
	}
	id, secret := [16]byte(in.CredentialId), [32]byte(in.Secret)
	caller, err := s.auth.Establish(ctx, values.RealmID(in.GetAudience()), id, secret)
	clear(secret[:])
	clear(in.Secret)
	if err != nil {
		if errors.Is(err, values.PublicUnavailable) {
			return nil, status.Error(codes.Unavailable, "authentication unavailable")
		}
		return nil, status.Error(codes.Unauthenticated, "authentication failed")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	var token [32]byte
	_, _ = rand.Read(token[:])
	key := sha256.Sum256(token[:])
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	if s.stopped {
		return nil, status.Error(codes.Unavailable, "service stopped")
	}
	count := 0
	for _, entry := range s.sessions {
		if entry.caller.Subject == caller.Subject {
			count++
		}
	}
	if len(s.sessions) >= s.config.MaxSessions || count >= s.config.MaxSessionsPerCaller {
		return nil, status.Error(codes.ResourceExhausted, "session capacity")
	}
	s.sessions[key] = session{caller: caller, expires: now.Add(s.config.SessionTTL), lifetime: newSessionLifetime()}
	// No goroutine or timer is allocated for every expired token. Admission and
	// lookups reclaim entries, and Stop releases the bounded registry entirely.
	return &networkpb.EstablishSessionResponse{Session: token[:], ExpiresInSeconds: uint64(s.config.SessionTTL / time.Second)}, nil
}

func (s *Service) expireLocked(now time.Time) {
	for k, v := range s.sessions {
		if !now.Before(v.expires) {
			v.lifetime.stop()
			delete(s.sessions, k)
		}
	}
}

func sessionKey(md metadata.MD) ([32]byte, error) {
	var zero [32]byte
	items := md.Get("authorization")
	if len(items) != 1 || !strings.HasPrefix(items[0], "Bearer ") {
		return zero, status.Error(codes.Unauthenticated, "session required")
	}
	b, err := values.DecodePublicBytes(strings.TrimPrefix(items[0], "Bearer "))
	if err != nil || len(b) != 32 {
		return zero, status.Error(codes.Unauthenticated, "session required")
	}
	return sha256.Sum256(b), nil
}

func (s *Service) caller(md metadata.MD) (values.CallerContext, error) {
	k, e := sessionKey(md)
	if e != nil {
		return values.CallerContext{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(time.Now())
	entry, ok := s.sessions[k]
	if !ok {
		return values.CallerContext{}, status.Error(codes.Unauthenticated, "session unavailable")
	}
	return entry.caller, nil
}

func (s *Service) ReleaseSession(ctx context.Context, _ *networkpb.ReleaseSessionRequest) (*networkpb.ReleaseSessionResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	key, err := sessionKey(md)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.sessions[key].lifetime.stop()
	delete(s.sessions, key)
	s.mu.Unlock()
	return &networkpb.ReleaseSessionResponse{}, nil
}
