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
)

// ExternalIdentity verifies the locally pinned issuer, challenge and actor,
// checks current issuer liveness and establishes a fresh local caller. Only
// trusted composition implements this port; no remote caller/context is copied.
type ExternalIdentity interface {
	Establish(context.Context, []byte, []byte, values.FactAuthorityRef, [32]byte) (values.CallerContext, time.Time, error)
	Check(context.Context, []byte, []byte, values.FactAuthorityRef, [32]byte) error
}

type identityChallenge struct {
	request    *corepb.CreateReadDelegationRequest
	federation *corepb.BeginFederationReadChallengeRequest
	readAt     *federationReadAtChallenge
	expires    time.Time
}

func (c identityChallenge) actor() string {
	if c.readAt != nil {
		return string(c.readAt.actor)
	}
	if c.federation != nil {
		return c.federation.GetActor()
	}
	return c.request.GetActor()
}

type identityGrant struct {
	proof     *networkpb.IdentityProof
	actor     values.FactAuthorityRef
	challenge [32]byte
}

func serviceIdentityMethod(method string) bool {
	if method == corepb.TransitionParentService_ReadCarrierPlan_FullMethodName || method == corepb.TransitionParentService_ReadCarrierDecision_FullMethodName {
		return true
	}
	if method == corepb.TransitionTargetService_AcceptTransition_FullMethodName || method == corepb.TransitionTargetService_InstallTransition_FullMethodName {
		return true
	}
	if method == corepb.TransitionSourceService_ReadTransitionChunk_FullMethodName || method == corepb.TransitionSourceService_ReadTransitionDecision_FullMethodName {
		return true
	}
	if method == corepb.TransitionApprovalService_CheckTransitionApproval_FullMethodName {
		return true
	}
	if method == corepb.CarrierApprovalService_CheckCarrierApproval_FullMethodName ||
		method == corepb.TransitionSourceService_FenceCarrierSource_FullMethodName ||
		method == corepb.TransitionSourceService_ReadCarrierSourceChunk_FullMethodName ||
		method == corepb.TransitionSourceService_SettleCarrierSource_FullMethodName {
		return true
	}
	if method == corepb.TransitionTargetService_CheckCarrierImport_FullMethodName ||
		method == corepb.TransitionTargetService_AcceptCarrierTarget_FullMethodName ||
		method == corepb.TransitionTargetService_InstallCarrierTarget_FullMethodName ||
		method == corepb.TransitionTargetService_ReadCarrierCompletion_FullMethodName {
		return true
	}
	if method == corepb.MountPeerService_PrepareMount_FullMethodName || method == corepb.MountPeerService_FinishMount_FullMethodName || method == corepb.MountPeerService_ReadMountDecision_FullMethodName {
		return true
	}
	return method == corepb.MountApprovalService_CheckMountApproval_FullMethodName || method == networkpb.IdentityProofService_CheckIdentityProof_FullMethodName ||
		method == corepb.FederationReadService_BeginFederationReadChallenge_FullMethodName || method == corepb.FederationReadService_ExchangeFederationReadIdentity_FullMethodName ||
		method == corepb.ReadDelegationService_BeginIdentityChallenge_FullMethodName ||
		method == corepb.ReadDelegationService_ExchangeIdentity_FullMethodName
}

func (s *Service) requireIdentityActor(ctx context.Context, actor values.FactAuthorityRef) error {
	if s.config.ExternalIdentity == nil {
		return status.Error(codes.Unimplemented, "external identity disabled")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || !s.authenticatedActor(ctx, actor) {
		return status.Error(codes.Unauthenticated, "service identity required")
	}
	return nil
}

// mintChallenge produces one challenge value and its registration expiry. A
// configured source waits on the serving Realm outside every registry lock, so
// the admission registry never blocks on the network; nil keeps the
// in-process mint. The expiry never extends the requested bound.
func (s *Service) mintChallenge(ctx context.Context, now time.Time, ttl time.Duration) ([32]byte, time.Time, error) {
	if s.config.ChallengeSource == nil {
		var challenge [32]byte
		if _, err := rand.Read(challenge[:]); err != nil {
			return [32]byte{}, time.Time{}, status.Error(codes.Internal, "challenge unavailable")
		}
		return challenge, now.Add(ttl), nil
	}
	challenge, expires, err := s.config.ChallengeSource.BeginChallenge(ctx)
	if err != nil || challenge == ([32]byte{}) || !now.Before(expires) {
		return [32]byte{}, time.Time{}, status.Error(codes.Unavailable, "challenge unavailable")
	}
	if bound := now.Add(ttl); expires.After(bound) {
		expires = bound
	}
	return challenge, expires, nil
}

func (s *Service) BeginIdentityChallenge(ctx context.Context, in *corepb.CreateReadDelegationRequest) (*corepb.IdentityChallenge, error) {
	actor := values.FactAuthorityRef(in.GetActor())
	if err := s.requireIdentityActor(ctx, actor); err != nil {
		return nil, err
	}
	if in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(s.config.ReadDelegation.MaxTTL/time.Second) || in.GetReference().GetNextHop() != string(s.config.NextHop) || len(in.Intents) == 0 || len(in.Intents) > 3 || proto.Size(in) > 4096 {
		return nil, status.Error(codes.InvalidArgument, "invalid identity challenge")
	}
	seen := map[corepb.Intent]bool{}
	for _, intent := range in.Intents {
		if seen[intent] || (intent != corepb.Intent_READ && intent != corepb.Intent_LIST && intent != corepb.Intent_QUERY) {
			return nil, status.Error(codes.InvalidArgument, "invalid read intents")
		}
		seen[intent] = true
	}
	now := time.Now()
	challenge, expires, err := s.mintChallenge(ctx, now, time.Duration(in.GetTtlSeconds())*time.Second)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for key, c := range s.challenges {
		if !now.Before(c.expires) {
			delete(s.challenges, key)
		} else if c.actor() == string(actor) {
			count++
		}
	}
	if s.stopped {
		return nil, status.Error(codes.Unavailable, "service stopped")
	}
	if len(s.challenges) >= s.config.ReadDelegation.MaxGrants || count >= s.config.ReadDelegation.MaxPerCaller {
		return nil, status.Error(codes.ResourceExhausted, "challenge capacity")
	}
	s.challenges[sha256.Sum256(challenge[:])] = identityChallenge{request: proto.Clone(in).(*corepb.CreateReadDelegationRequest), expires: expires}
	return &corepb.IdentityChallenge{Challenge: challenge[:], ExpiresInSeconds: proto.Uint64(uint64(expires.Sub(now) / time.Second))}, nil
}

func (s *Service) checkGrantIdentity(ctx context.Context, key [32]byte) error {
	s.mu.Lock()
	g, ok := s.grants[key]
	s.mu.Unlock()
	if !ok {
		return status.Error(codes.Unauthenticated, "delegation unavailable")
	}
	if g.identity == nil {
		return nil
	}
	i := g.identity
	err := s.config.ExternalIdentity.Check(ctx, i.proof.Payload, i.proof.Signature, i.actor, i.challenge)
	if err != nil {
		// Loss of reachability is not proof of revocation. A confirmed invalid
		// proof terminates this establishment; a new one cannot reuse its views.
		if !errors.Is(err, values.PublicUnavailable) {
			s.mu.Lock()
			delete(s.grants, key)
			s.mu.Unlock()
		}
		return identityFailure(err)
	}
	return nil
}
