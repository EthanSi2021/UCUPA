package admission

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func (s *Service) ExchangeIdentity(ctx context.Context, in *corepb.ExchangeIdentityRequest) (*corepb.CreateReadDelegationResponse, error) {
	actor := values.FactAuthorityRef(in.GetActor())
	if err := s.requireIdentityActor(ctx, actor); err != nil {
		return nil, err
	}
	if len(in.Challenge) != 32 || in.Proof == nil || len(in.Proof.Payload) > 4096 || len(in.Proof.Signature) != 64 {
		return nil, status.Error(codes.Unauthenticated, "identity unavailable")
	}
	key := sha256.Sum256(in.Challenge)
	s.mu.Lock()
	c, ok := s.challenges[key]
	if ok && c.federation == nil && c.request.GetActor() == string(actor) {
		delete(s.challenges, key)
	} else {
		ok = false
	}
	s.mu.Unlock()
	if !ok || !time.Now().Before(c.expires) {
		return nil, status.Error(codes.Unauthenticated, "identity unavailable")
	}
	// The one-use challenge is consumed even if verification fails. Repeating an
	// exchange never creates another establishment or revives a lost result.
	caller, expires, err := s.config.ExternalIdentity.Establish(ctx, in.Proof.Payload, in.Proof.Signature, actor, [32]byte(in.Challenge))
	if err != nil {
		return nil, identityFailure(err)
	}
	if c.expires.Before(expires) {
		expires = c.expires
	}
	if err := s.reserveExternalGrant(caller, expires); err != nil {
		return nil, err
	}
	defer func() { s.mu.Lock(); s.releaseGrantReservationLocked(caller.Subject); s.mu.Unlock() }()
	var handle *networkpb.Token
	retained := false
	defer func() {
		if !retained && handle != nil {
			s.closeGrantHandle(ctx, caller, handle)
		}
	}()
	intents := make(map[corepb.Intent]bool)
	for _, intent := range c.request.Intents {
		next, err := s.enterGrant(ctx, caller, c.request.Reference, intent)
		if err != nil {
			return delegationFailure(err)
		}
		intents[intent] = true
		if handle == nil {
			handle = next
		} else {
			s.closeGrantHandle(ctx, caller, next)
		}
	}
	i := &identityGrant{proof: proto.Clone(in.Proof).(*networkpb.IdentityProof), actor: actor, challenge: [32]byte(in.Challenge)}
	if err := s.config.ExternalIdentity.Check(ctx, i.proof.Payload, i.proof.Signature, actor, i.challenge); err != nil {
		return nil, identityFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	var token [32]byte
	_, _ = rand.Read(token[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := time.Until(expires)
	if s.stopped || remaining <= 0 {
		return nil, status.Error(codes.Unavailable, "identity unavailable")
	}
	s.grants[sha256.Sum256(token[:])] = readGrant{caller: caller, actor: actor, handle: proto.Clone(handle).(*networkpb.Token), intents: intents, observe: c.request.GetObserve(), expires: expires, identity: i}
	retained = true
	return &corepb.CreateReadDelegationResponse{Outcome: &corepb.CreateReadDelegationResponse_Result{Result: &corepb.ReadDelegation{Target: proto.String(string(s.config.Expected)), Actor: proto.String(string(actor)), Token: token[:], Handle: handle, ExpiresInSeconds: proto.Uint64(uint64((remaining + time.Second - 1) / time.Second))}}}, nil
}

func (s *Service) reserveExternalGrant(caller values.CallerContext, expires time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.expireGrantsLocked(now)
	if s.stopped || !now.Before(expires) {
		return status.Error(codes.Unavailable, "identity unavailable")
	}
	total, count := len(s.grants), s.pending[caller.Subject]
	for _, n := range s.pending {
		total += n
	}
	for _, g := range s.grants {
		if g.caller.Subject == caller.Subject {
			count++
		}
	}
	if total >= s.config.ReadDelegation.MaxGrants || count >= s.config.ReadDelegation.MaxPerCaller {
		return status.Error(codes.ResourceExhausted, "delegation capacity")
	}
	s.pending[caller.Subject]++
	return nil
}
