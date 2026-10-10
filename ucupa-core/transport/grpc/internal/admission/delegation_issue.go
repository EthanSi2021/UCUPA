package admission

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func (s *Service) CreateReadDelegation(ctx context.Context, in *corepb.CreateReadDelegationRequest) (*corepb.CreateReadDelegationResponse, error) {
	c := s.config.ReadDelegation
	if c == nil {
		return nil, status.Error(codes.Unimplemented, "read delegation disabled")
	}
	actor := values.FactAuthorityRef(in.GetActor())
	if !actor.Valid() || in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(c.MaxTTL/time.Second) || len(in.Intents) == 0 || len(in.Intents) > 3 || proto.Size(in) > 4096 {
		return nil, status.Error(codes.InvalidArgument, "invalid read delegation")
	}
	if _, ok := c.Peers[actor]; !ok {
		return nil, status.Error(codes.Unauthenticated, "delegation unavailable")
	}
	intents := make(map[corepb.Intent]bool)
	for _, intent := range in.Intents {
		if (intent != corepb.Intent_READ && intent != corepb.Intent_LIST && intent != corepb.Intent_QUERY) || intents[intent] {
			return nil, status.Error(codes.InvalidArgument, "invalid read intents")
		}
		intents[intent] = true
	}
	if in.GetReference().GetNextHop() != string(s.config.NextHop) {
		return nil, status.Error(codes.InvalidArgument, "wrong entry")
	}
	original, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	parentKey, err := sessionKey(md)
	if err != nil {
		return nil, err
	}
	expires, err := s.reserveGrant(original, parentKey, time.Duration(in.GetTtlSeconds())*time.Second)
	if err != nil {
		return nil, err
	}
	reserved := true
	defer func() {
		if reserved {
			s.mu.Lock()
			s.releaseGrantReservationLocked(original.Subject)
			s.mu.Unlock()
		}
	}()
	fork, err := s.auth.(callerForker).ForkCaller(ctx, original)
	if err != nil {
		return delegationFailure(err)
	}
	var handle *networkpb.Token
	retained := false
	defer func() {
		if !retained && handle != nil {
			s.closeGrantHandle(ctx, fork, handle)
		}
	}()
	// The owner derives each permission from its existing Enter operation.
	// Transport never invents a grant binding or reproduces closure membership.
	for _, intent := range in.Intents {
		next, err := s.enterGrant(ctx, fork, in.Reference, intent)
		if err != nil {
			return delegationFailure(err)
		}
		if handle == nil {
			handle = next
		} else {
			s.closeGrantHandle(ctx, fork, next)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	var token [32]byte
	_, _ = rand.Read(token[:])
	key := sha256.Sum256(token[:])
	s.mu.Lock()
	parent, exists := s.sessions[parentKey]
	remaining := time.Until(expires)
	if s.stopped || !exists || parent.caller != original || !time.Now().Before(parent.expires) || remaining <= 0 {
		s.mu.Unlock()
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	s.grants[key] = readGrant{caller: fork, actor: actor, handle: proto.Clone(handle).(*networkpb.Token), intents: intents, observe: in.GetObserve(), expires: expires}
	s.releaseGrantReservationLocked(original.Subject)
	reserved = false
	retained = true
	s.mu.Unlock()
	seconds := uint64(remaining / time.Second)
	if remaining%time.Second != 0 {
		seconds++
	}
	return &corepb.CreateReadDelegationResponse{Outcome: &corepb.CreateReadDelegationResponse_Result{Result: &corepb.ReadDelegation{
		Target: proto.String(string(s.config.Expected)), Actor: proto.String(string(actor)), Token: token[:], Handle: handle, ExpiresInSeconds: proto.Uint64(seconds),
	}}}, nil
}

func (s *Service) releaseGrantReservationLocked(subject values.PrincipalRef) {
	s.pending[subject]--
	if s.pending[subject] == 0 {
		delete(s.pending, subject)
	}
}

func (s *Service) reserveGrant(caller values.CallerContext, parentKey [32]byte, ttl time.Duration) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.expireGrantsLocked(now)
	parent, ok := s.sessions[parentKey]
	if s.stopped || !ok || parent.caller != caller || !now.Before(parent.expires) {
		return time.Time{}, status.Error(codes.Unauthenticated, "session unavailable")
	}
	total, perCaller := len(s.grants), s.pending[caller.Subject]
	for _, count := range s.pending {
		total += count
	}
	for _, g := range s.grants {
		if g.caller.Subject == caller.Subject {
			perCaller++
		}
	}
	if total >= s.config.ReadDelegation.MaxGrants || perCaller >= s.config.ReadDelegation.MaxPerCaller {
		return time.Time{}, status.Error(codes.ResourceExhausted, "delegation capacity")
	}
	expires := now.Add(ttl)
	if parent.expires.Before(expires) {
		expires = parent.expires
	}
	if time.Until(expires) <= 0 {
		return time.Time{}, status.Error(codes.Unauthenticated, "session unavailable")
	}
	s.pending[caller.Subject]++
	return expires, nil
}

func (s *Service) enterGrant(ctx context.Context, caller values.CallerContext, reference *networkpb.Reference, intent corepb.Intent) (*networkpb.Token, error) {
	v, err := wire.ToValue((&corepb.EnterRequest{Reference: reference, Intent: intent.Enum()}).ProtoReflect(), 0)
	if err != nil {
		return nil, values.PublicInvalid
	}
	m := v.(map[string]any)
	m["op"] = "enter"
	raw, err := values.CanonicalJSONValue(m)
	if err != nil {
		return nil, values.PublicInvalid
	}
	out, err := s.owner.PublicCall(ctx, caller, raw)
	if err != nil {
		return nil, err
	}
	v, err = values.ParseCanonicalJSON(out, values.PublicJSONLimits())
	if err != nil {
		return nil, values.PublicUnavailable
	}
	var entered corepb.EnterResult
	if err := wire.FromValue(entered.ProtoReflect(), v); err != nil {
		return nil, values.PublicUnavailable
	}
	return entered.Handle, nil
}

func (s *Service) closeGrantHandle(ctx context.Context, caller values.CallerContext, handle *networkpb.Token) {
	v, err := wire.ToValue((&corepb.CloseRequest{Handle: handle}).ProtoReflect(), 0)
	if err != nil {
		return
	}
	m := v.(map[string]any)
	m["op"] = "close"
	raw, err := values.CanonicalJSONValue(m)
	if err == nil {
		_, _ = s.owner.PublicCall(ctx, caller, raw)
	}
}

func delegationFailure(err error) (*corepb.CreateReadDelegationResponse, error) {
	var public values.PublicError
	if errors.As(err, &public) {
		if n, ok := networkpb.Failure_value[string(public)]; ok && n != 0 {
			return &corepb.CreateReadDelegationResponse{Outcome: &corepb.CreateReadDelegationResponse_Failure{Failure: networkpb.Failure(n)}}, nil
		}
	}
	return nil, status.Error(codes.Unavailable, "delegation unavailable")
}

func (s *Service) RevokeReadDelegation(ctx context.Context, in *corepb.RevokeReadDelegationRequest) (*corepb.RevokeReadDelegationResponse, error) {
	if s.config.ReadDelegation == nil {
		return nil, status.Error(codes.Unimplemented, "read delegation disabled")
	}
	if len(in.Token) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid delegation token")
	}
	caller, ok := grpcsession.Caller(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "session required")
	}
	// Revalidate the authenticated caller, including current credential status,
	// even if this method does not need a new domain permission.
	if err := s.checkCaller(ctx, caller); err != nil {
		if errors.Is(err, values.PublicUnavailable) {
			return nil, status.Error(codes.Unavailable, "authentication unavailable")
		}
		return nil, status.Error(codes.Unauthenticated, "session unavailable")
	}
	key := sha256.Sum256(in.Token)
	s.mu.Lock()
	g, exists := s.grants[key]
	if exists && g.federation == nil && g.caller.Subject == caller.Subject {
		delete(s.grants, key)
	}
	s.mu.Unlock()
	if exists && g.federation == nil && g.caller.Subject == caller.Subject {
		s.closeGrantHandle(ctx, g.caller, g.handle)
	}
	return &corepb.RevokeReadDelegationResponse{Released: proto.Bool(true)}, nil
}
