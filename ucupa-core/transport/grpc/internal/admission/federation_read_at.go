package admission

import (
	"context"
	"crypto/sha256"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

type federationReadAtChallenge struct {
	grant   [32]byte
	actor   values.FactAuthorityRef
	basis   *netpb.Token
	element string
}

type federationReadAtIdentity struct {
	identityGrant
	deadline time.Time
}

// The old grant proves access to the retained basis, not a new Read request.
// A single-use challenge binds the additional proof to this grant and Element.
func (s *Service) BeginFederationReadAtChallenge(ctx context.Context, in *pb.BeginFederationReadAtChallengeRequest) (*pb.IdentityChallenge, error) {
	_, g, key, err := s.federationCaller(ctx, in.GetBasis())
	if err != nil {
		return nil, err
	}
	if !values.ElementID(in.GetElement()).Valid() || in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(s.config.ReadDelegation.MaxTTL/time.Second) {
		return nil, status.Error(codes.InvalidArgument, "invalid read-at challenge")
	}
	// A delegated mint waits on the serving Realm outside every registry lock.
	// A grant that expires or changes while waiting is rechecked below; the
	// minted challenge is bounded by the grant and simply self-expires.
	now := time.Now()
	nonce, expires, err := s.mintChallenge(ctx, now, time.Duration(in.GetTtlSeconds())*time.Second)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lockedNow := time.Now()
	current, exists := s.grants[key]
	if !exists || current.caller != g.caller || s.stopped || !lockedNow.Before(g.expires) {
		return nil, status.Error(codes.Unauthenticated, "read basis unavailable")
	}
	if g.expires.Sub(lockedNow).Truncate(time.Second) <= 0 {
		return nil, status.Error(codes.Unauthenticated, "read basis expiring")
	}
	if bound := g.expires; expires.After(bound) {
		expires = bound
	}
	if !lockedNow.Before(expires) {
		return nil, status.Error(codes.Unavailable, "challenge unavailable")
	}
	count := 0
	for k, c := range s.challenges {
		if !lockedNow.Before(c.expires) {
			delete(s.challenges, k)
		} else if c.actor() == string(g.actor) {
			count++
		}
	}
	if len(s.challenges) >= s.config.ReadDelegation.MaxGrants || count >= s.config.ReadDelegation.MaxPerCaller {
		return nil, status.Error(codes.ResourceExhausted, "challenge capacity")
	}
	s.challenges[sha256.Sum256(nonce[:])] = identityChallenge{readAt: &federationReadAtChallenge{grant: key, actor: g.actor, basis: proto.Clone(in.Basis).(*netpb.Token), element: in.GetElement()}, expires: expires}
	return &pb.IdentityChallenge{Challenge: nonce[:], ExpiresInSeconds: proto.Uint64(uint64(expires.Sub(lockedNow) / time.Second))}, nil
}

func (s *Service) consumeFederationReadAtProof(ctx context.Context, g readGrant, key [32]byte, in *pb.ProjectFederationReadPartRequest) (*federationReadAtIdentity, error) {
	if in.GetRequest().GetKind() != "read-at" {
		if in.ReadChallenge != nil || in.ReadProof != nil {
			return nil, status.Error(codes.InvalidArgument, "unexpected read-at proof")
		}
		return nil, nil
	}
	bad := func() (*federationReadAtIdentity, error) {
		return nil, status.Error(codes.Unauthenticated, "independent read-at proof required")
	}
	if len(in.GetReadChallenge()) != 32 || in.ReadProof == nil || len(in.ReadProof.Payload) == 0 || len(in.ReadProof.Payload) > 4096 || len(in.ReadProof.Signature) != 64 || in.Continuation != nil {
		return bad()
	}
	s.mu.Lock()
	nonceKey := sha256.Sum256(in.ReadChallenge)
	c, exists := s.challenges[nonceKey]
	if exists && c.readAt != nil && c.readAt.grant == key && c.readAt.actor == g.actor {
		delete(s.challenges, nonceKey)
	} else {
		exists = false
	}
	s.mu.Unlock()
	if !exists || !time.Now().Before(c.expires) || !proto.Equal(c.readAt.basis, in.Basis) || c.readAt.element != in.GetRequest().GetElement() {
		return bad()
	}
	nonce := [32]byte(in.ReadChallenge)
	caller, expires, err := s.config.ExternalIdentity.Establish(ctx, in.ReadProof.Payload, in.ReadProof.Signature, g.actor, nonce)
	if err != nil {
		return nil, identityFailure(err)
	}
	if caller.Subject != g.caller.Subject || !time.Now().Before(expires) || !time.Now().Before(c.expires) || !s.grantAlive(key, g.caller) {
		return bad()
	}
	// Keep the original established caller and its exact authorization views.
	// Fresh proof cannot replace those views, renew the grant or the snapshot.
	if expires.Before(c.expires) {
		c.expires = expires
	}
	return &federationReadAtIdentity{identityGrant: identityGrant{proof: proto.Clone(in.ReadProof).(*netpb.IdentityProof), actor: g.actor, challenge: nonce}, deadline: c.expires}, nil
}
