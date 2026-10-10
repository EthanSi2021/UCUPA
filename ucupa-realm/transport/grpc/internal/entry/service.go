package entry

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	admission "ucupa/ucupa-realm/transport/grpc/internal/admission"
	wire "ucupa/ucupa-realm/transport/grpc/internal/wire"
)

func identityError(err error) error {
	switch {
	case errors.Is(err, realm.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "caller rejected")
	case errors.Is(err, realm.ErrStore):
		return status.Error(codes.Unavailable, "Realm unavailable")
	default:
		return status.Error(codes.InvalidArgument, "invalid caller evidence")
	}
}

// callerEntry resolves one presented caller handle to its owning service and
// stored kernel context. Unknown, expired and cross-owner references fail
// closed before any kernel work runs.
func (s *Service) callerEntry(ctx context.Context, raw []byte) ([32]byte, admission.Entry, error) {
	owner, err := admission.OwnerFromContext(ctx)
	if err != nil {
		return [32]byte{}, admission.Entry{}, err
	}
	token, err := wire.Exact32(raw)
	if err != nil {
		return [32]byte{}, admission.Entry{}, err
	}
	entry, err := s.registry.Lookup(token)
	if err != nil {
		return [32]byte{}, admission.Entry{}, err
	}
	if entry.Kind != admission.Caller || entry.Owner != owner {
		return [32]byte{}, admission.Entry{}, status.Error(codes.Unauthenticated, "handle unavailable")
	}
	return token, entry, nil
}

// CheckTarget answers the read-only Realm identity check: it establishes no
// caller, view or grant and registers no owner.
func (s *Service) CheckTarget(context.Context, *realmpb.CheckTargetRequest) (*realmpb.CheckTargetResponse, error) {
	return &realmpb.CheckTargetResponse{RealmId: proto.String(string(s.config.Expected))}, nil
}

// EstablishCaller passes credential evidence to the original kernel. The
// secret is used once and never stored, logged or echoed.
func (s *Service) EstablishCaller(ctx context.Context, in *realmpb.EstablishCallerRequest) (*realmpb.EstablishCallerResponse, error) {
	owner, err := admission.OwnerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if in.GetAudience() != string(s.config.Expected) {
		return nil, status.Error(codes.Unauthenticated, "audience mismatch")
	}
	if len(in.GetCredentialId()) != 16 || len(in.GetSecret()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid evidence")
	}
	var id realm.CredentialID
	var secret realm.CredentialSecret
	copy(id[:], in.GetCredentialId())
	copy(secret[:], in.GetSecret())
	caller, err := s.realm.EstablishCaller(realm.CredentialEvidence{Audience: s.config.Expected, ID: id, Secret: secret})
	clear(secret[:])
	if err != nil {
		return nil, identityError(err)
	}
	token, err := s.registry.Issue(admission.Entry{
		Kind:    admission.Caller,
		Owner:   owner,
		Caller:  caller,
		Expires: s.now().Add(s.config.HandleTTL),
	}, s.config.MaxHandles, s.config.MaxHandlesPerCaller)
	if err != nil {
		return nil, err
	}
	return &realmpb.EstablishCallerResponse{
		Caller:           token[:],
		SubjectRealm:     proto.String(string(caller.Subject.Realm)),
		SubjectPrincipal: proto.String(string(caller.Subject.Principal)),
		ExpiresInSeconds: proto.Uint64(uint64(s.config.HandleTTL / time.Second)),
	}, nil
}

// BeginChallenge issues one unguessable single-use challenge bound to the
// admitted service identity, for building a foreign identity proof.
func (s *Service) BeginChallenge(ctx context.Context, _ *realmpb.BeginChallengeRequest) (*realmpb.BeginChallengeResponse, error) {
	owner, err := admission.OwnerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	challenge, err := s.registry.NewChallenge(owner)
	if err != nil {
		return nil, err
	}
	return &realmpb.BeginChallengeResponse{
		Challenge:        challenge[:],
		ExpiresInSeconds: proto.Uint64(uint64(s.config.ChallengeTTL / time.Second)),
	}, nil
}

func livenessError(err error) error {
	if errors.Is(err, realm.ErrUnauthenticated) {
		return status.Error(codes.Unauthenticated, "issuer rejected identity")
	}
	return status.Error(codes.Unavailable, "issuer unavailable")
}

// EstablishForeignCaller keeps the P6 chain: signature verification, online
// liveness at the configured issuer, expiry recheck after the network wait,
// then trusted-assembly of exactly the verified subject. The liveness check
// runs while no registry or Realm lock is held; the challenge is consumed in
// one atomic step afterwards, so concurrency and waiting cannot turn one
// challenge into two identities or spend an expired one.
func (s *Service) EstablishForeignCaller(ctx context.Context, in *realmpb.EstablishForeignCallerRequest) (*realmpb.EstablishForeignCallerResponse, error) {
	owner, err := admission.OwnerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.config.Foreign == nil {
		return nil, status.Error(codes.Unavailable, "foreign identity not configured")
	}
	challenge, err := wire.Exact32(in.GetChallenge())
	if err != nil {
		return nil, err
	}
	now := s.now()
	pending := s.registry.HasChallenge(challenge, owner, now)
	if !pending {
		return nil, status.Error(codes.Unauthenticated, "challenge rejected")
	}
	actor := values.FactAuthorityRef(in.GetActor())
	if !actor.Valid() {
		return nil, status.Error(codes.Unauthenticated, "identity actor rejected")
	}
	claim, err := s.config.Foreign.Verify(realm.IdentityProof{Payload: in.GetPayload(), Signature: in.GetSignature()}, actor, challenge, now)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "identity proof rejected")
	}
	source, ok := s.config.ForeignLiveness[claim.Issuer]
	if !ok {
		return nil, status.Error(codes.Unavailable, "issuer liveness unavailable")
	}
	if err := source.Check(ctx, in.GetPayload(), in.GetSignature()); err != nil {
		return nil, livenessError(err)
	}
	if now = s.now(); !now.Before(time.Unix(claim.Expires, 0)) {
		return nil, status.Error(codes.Unauthenticated, "identity proof expired")
	}
	// The single-use constraint is enforced here, after every wait: lookup,
	// owner match, fresh expiry and removal are one step.
	if err := s.registry.ConsumeChallenge(challenge, owner); err != nil {
		return nil, err
	}
	caller, err := s.realm.TrustLocalCaller(values.PrincipalRef{Realm: claim.Issuer, Principal: claim.Subject})
	if err != nil {
		return nil, identityError(err)
	}
	expires := s.now().Add(s.config.HandleTTL)
	if claimExpiry := time.Unix(claim.Expires, 0); claimExpiry.Before(expires) {
		expires = claimExpiry
	}
	seconds := uint64(expires.Sub(s.now()) / time.Second)
	if seconds == 0 {
		return nil, status.Error(codes.Unauthenticated, "identity lease exhausted")
	}
	token, err := s.registry.Issue(admission.Entry{
		Kind:    admission.Caller,
		Owner:   owner,
		Caller:  caller,
		Expires: expires,
	}, s.config.MaxHandles, s.config.MaxHandlesPerCaller)
	if err != nil {
		return nil, err
	}
	return &realmpb.EstablishForeignCallerResponse{
		Caller:           token[:],
		SubjectRealm:     proto.String(string(caller.Subject.Realm)),
		SubjectPrincipal: proto.String(string(caller.Subject.Principal)),
		ExpiresInSeconds: proto.Uint64(seconds),
	}, nil
}

// ForkCaller derives an isolated kernel establishment from the presented
// handle's still-valid context. Derivation never extends the parent lease.
func (s *Service) ForkCaller(ctx context.Context, in *realmpb.ForkCallerRequest) (*realmpb.ForkCallerResponse, error) {
	token, entry, err := s.callerEntry(ctx, in.GetCaller())
	if err != nil {
		return nil, err
	}
	forked, err := s.realm.ForkCaller(ctx, entry.Caller)
	if err != nil {
		return nil, identityError(err)
	}
	now := s.now()
	expires := now.Add(s.config.HandleTTL)
	if entry.Expires.Before(expires) {
		expires = entry.Expires
	}
	seconds := uint64(expires.Sub(now) / time.Second)
	if seconds == 0 {
		return nil, status.Error(codes.Unauthenticated, "caller lease exhausted")
	}
	derived, err := s.registry.Issue(admission.Entry{
		Kind:    admission.Caller,
		Owner:   entry.Owner,
		Caller:  forked,
		Parent:  sha256.Sum256(token[:]),
		Expires: expires,
	}, s.config.MaxHandles, s.config.MaxHandlesPerCaller)
	if err != nil {
		return nil, err
	}
	return &realmpb.ForkCallerResponse{Caller: derived[:], ExpiresInSeconds: proto.Uint64(seconds)}, nil
}

// ReleaseHandle removes one reference. Release is idempotent and never grants
// or preserves any permission; another service's handle is never released.
func (s *Service) ReleaseHandle(ctx context.Context, in *realmpb.ReleaseHandleRequest) (*realmpb.ReleaseHandleResponse, error) {
	owner, err := admission.OwnerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	token, err := wire.Exact32(in.GetHandle())
	if err != nil {
		return nil, err
	}
	if entry, err := s.registry.Lookup(token); err == nil && entry.Owner != owner {
		return nil, status.Error(codes.Unauthenticated, "handle unavailable")
	}
	s.registry.Release(token)
	return &realmpb.ReleaseHandleResponse{}, nil
}
