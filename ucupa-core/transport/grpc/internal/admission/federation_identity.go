package admission

import (
	federationcontract "ucupa/ucupa-core/contract/federation"

	"context"
	"crypto/rand"
	"crypto/sha256"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// FederationSource is the transport's complete source-read port. It is
// configured independently from the public JSON owner and must identify the
// same fact owner; transport never rediscovers it by asserting Owner.
type FederationSource interface {
	AuthorityRef() values.FactAuthorityRef
	OpenFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, federationcontract.FederationReadRequest) (federationcontract.FederationReadPart, error)
	ProjectFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, federationcontract.FederationProjectionRequest, *values.PublicToken, uint16) (federationcontract.FederationProjectionPage, error)
	CheckFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, bool) error
	ReleaseFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken) error
	ObserveFederationReadPart(context.Context, values.CallerContext, values.FactAuthorityRef, values.PublicToken, federationcontract.FederationObserveRequest) (federationcontract.FederationObservationPage, error)
}

func (s *Service) BeginFederationReadChallenge(ctx context.Context, in *pb.BeginFederationReadChallengeRequest) (*pb.IdentityChallenge, error) {
	actor := values.FactAuthorityRef(in.GetActor())
	if err := s.requireIdentityActor(ctx, actor); err != nil {
		return nil, err
	}
	if s.config.Federation == nil {
		return nil, status.Error(codes.Unimplemented, "federation reads disabled")
	}
	r, err := wire.FederationRequest(in.GetRequest())
	if err != nil || in.GetTtlSeconds() == 0 || in.GetTtlSeconds() > uint64(s.config.ReadDelegation.MaxTTL/time.Second) || in.GetNextHop() != string(s.config.NextHop) || proto.Size(in) > 4096 || (r.Incoming.Valid() && r.Incoming.CarrierAuthority != s.config.Expected) {
		return nil, status.Error(codes.InvalidArgument, "invalid federation challenge")
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
	s.challenges[sha256.Sum256(challenge[:])] = identityChallenge{federation: proto.Clone(in).(*pb.BeginFederationReadChallengeRequest), expires: expires}
	return &pb.IdentityChallenge{Challenge: challenge[:], ExpiresInSeconds: proto.Uint64(uint64(expires.Sub(now) / time.Second))}, nil
}

func (s *Service) ExchangeFederationReadIdentity(ctx context.Context, in *pb.ExchangeIdentityRequest) (*pb.ExchangeFederationReadIdentityResponse, error) {
	actor := values.FactAuthorityRef(in.GetActor())
	if err := s.requireIdentityActor(ctx, actor); err != nil {
		return nil, err
	}
	o := s.config.Federation
	if o == nil {
		return nil, status.Error(codes.Unimplemented, "federation reads disabled")
	}
	if len(in.GetChallenge()) != 32 || in.Proof == nil || len(in.Proof.Payload) == 0 || len(in.Proof.Payload) > 4096 || len(in.Proof.Signature) != 64 {
		return nil, status.Error(codes.Unauthenticated, "identity unavailable")
	}
	s.mu.Lock()
	key := sha256.Sum256(in.Challenge)
	c, ok := s.challenges[key]
	if ok && c.federation != nil && c.actor() == string(actor) {
		delete(s.challenges, key)
	} else {
		ok = false
	}
	s.mu.Unlock()
	if !ok || !time.Now().Before(c.expires) {
		return nil, status.Error(codes.Unauthenticated, "identity unavailable")
	}
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
	request, err := wire.FederationRequest(c.federation.Request)
	if err != nil {
		return nil, status.Error(codes.Internal, "challenge unavailable")
	}
	part, err := o.OpenFederationReadPart(ctx, caller, actor, request)
	if err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.ExchangeFederationReadIdentityResponse{Outcome: &pb.ExchangeFederationReadIdentityResponse_Failure{Failure: f}}, e
	}
	keep := false
	defer func() {
		if !keep {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.config.RPCDeadline)
			defer cancel()
			_ = o.ReleaseFederationReadPart(cleanup, caller, actor, part.Basis)
		}
	}()
	i := &identityGrant{proof: proto.Clone(in.Proof).(*netpb.IdentityProof), actor: actor, challenge: [32]byte(in.Challenge)}
	if err := s.config.ExternalIdentity.Check(ctx, i.proof.Payload, i.proof.Signature, actor, i.challenge); err != nil {
		return nil, identityFailure(err)
	}
	if err := o.CheckFederationReadPart(ctx, caller, actor, part.Basis, false); err != nil {
		f, e := rpcfailure.MountPeer(err)
		return &pb.ExchangeFederationReadIdentityResponse{Outcome: &pb.ExchangeFederationReadIdentityResponse_Failure{Failure: f}}, e
	}
	if err := s.config.ExternalIdentity.Check(ctx, i.proof.Payload, i.proof.Signature, actor, i.challenge); err != nil {
		return nil, identityFailure(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, status.Error(codes.Internal, "grant unavailable")
	}
	wire := wire.FederationPartWire(part)
	s.mu.Lock()
	defer s.mu.Unlock()
	remaining := time.Until(expires)
	if s.stopped || remaining <= 0 {
		return nil, status.Error(codes.Unavailable, "identity unavailable")
	}
	result := &pb.ExchangeFederationReadIdentityResponse{Outcome: &pb.ExchangeFederationReadIdentityResponse_Result{Result: &pb.FederationReadGrant{Target: proto.String(string(s.config.Expected)), Actor: proto.String(string(actor)), Token: token[:], ExpiresInSeconds: proto.Uint64(uint64((remaining + time.Second - 1) / time.Second)), Part: wire}}}
	if proto.Size(result) > s.config.MaxResponseBytes {
		return nil, status.Error(codes.ResourceExhausted, "response capacity")
	}
	s.grants[sha256.Sum256(token[:])] = readGrant{caller: caller, actor: actor, expires: expires, identity: i, federation: proto.Clone(wire).(*pb.FederationReadPart)}
	keep = true
	return result, nil
}
