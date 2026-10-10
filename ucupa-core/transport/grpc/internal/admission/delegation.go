package admission

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

// ReadDelegationConfig is explicit target-side trust and finite admission.
// Peers binds actor identities to authenticated client keys, never to addresses.
// Omission disables issuance and use, not the ordinary user's public operations.
type ReadDelegationConfig struct {
	ClientRoots  *x509.CertPool
	Peers        map[values.FactAuthorityRef][32]byte
	MaxTTL       time.Duration
	MaxGrants    int
	MaxPerCaller int
}

// callerForker derives one isolated caller from the presented establishment.
// Under a network Realm contact it is a real remote wait, so it carries the
// original request's context for cancellation.
type callerForker interface {
	ForkCaller(context.Context, values.CallerContext) (values.CallerContext, error)
}

// checkCaller consumes a fork only as a current liveness check. Unlike a
// retained grant, this temporary reference has no caller after the check.
func (s *Service) checkCaller(ctx context.Context, caller values.CallerContext) error {
	bound, err := s.auth.(callerForker).ForkCaller(ctx, caller)
	if err == nil {
		if releaser, ok := s.auth.(interface {
			ReleaseCaller(context.Context, values.CallerContext)
		}); ok {
			releaser.ReleaseCaller(ctx, bound)
		}
	}
	return err
}

type readGrant struct {
	caller     values.CallerContext
	actor      values.FactAuthorityRef
	handle     *networkpb.Token
	intents    map[corepb.Intent]bool
	observe    bool
	expires    time.Time
	identity   *identityGrant
	federation *corepb.FederationReadPart
}

func CopyReadDelegationConfig(c *ReadDelegationConfig) (*ReadDelegationConfig, error) {
	if c == nil {
		return nil, nil
	}
	if c.ClientRoots == nil || len(c.Peers) == 0 || c.MaxTTL <= 0 || c.MaxTTL%time.Second != 0 || c.MaxGrants <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxGrants {
		return nil, errors.New("invalid read delegation configuration")
	}
	out := *c
	out.ClientRoots = c.ClientRoots.Clone()
	out.Peers = make(map[values.FactAuthorityRef][32]byte, len(c.Peers))
	for actor, pin := range c.Peers {
		if !actor.Valid() || pin == ([32]byte{}) {
			return nil, errors.New("invalid delegated actor trust")
		}
		out.Peers[actor] = pin
	}
	return &out, nil
}

func (s *Service) expireGrantsLocked(now time.Time) {
	for key, grant := range s.grants {
		if !now.Before(grant.expires) {
			delete(s.grants, key)
		}
	}
}

// Owner contexts have their own finite TTL and count/byte limits. Expiration
// need not call an owner while holding the transport lock or retain tombstones.
func (s *Service) grantAlive(key [32]byte, caller values.CallerContext) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireGrantsLocked(time.Now())
	g, ok := s.grants[key]
	return !s.stopped && ok && g.caller == caller
}

func (s *Service) delegatedCaller(ctx context.Context, md metadata.MD, method string, request proto.Message) (values.CallerContext, [32]byte, error) {
	bad := func() (values.CallerContext, [32]byte, error) {
		return values.CallerContext{}, [32]byte{}, status.Error(codes.Unauthenticated, "delegation unavailable")
	}
	if s.config.ReadDelegation == nil {
		return bad()
	}
	items := md.Get("authorization")
	if len(items) != 1 || !strings.HasPrefix(items[0], "Delegation ") {
		return bad()
	}
	token, err := values.DecodePublicBytes(strings.TrimPrefix(items[0], "Delegation "))
	if err != nil || len(token) != 32 {
		return bad()
	}
	key := sha256.Sum256(token)
	s.mu.Lock()
	s.expireGrantsLocked(time.Now())
	g, ok := s.grants[key]
	s.mu.Unlock()
	if !ok {
		return bad()
	}
	if !s.authenticatedActor(ctx, g.actor) {
		return bad()
	}
	if err := s.checkGrantIdentity(ctx, key); err != nil {
		return values.CallerContext{}, [32]byte{}, err
	}
	if g.federation != nil {
		if !federationPartMethod(method) {
			return bad()
		}
		m := request.ProtoReflect()
		f := m.Descriptor().Fields().ByName("basis")
		if f == nil || !m.Has(f) || !proto.Equal(m.Get(f).Message().Interface(), g.federation.Basis) {
			return bad()
		}
		return g.caller, key, nil
	}
	intent := corepb.Intent_INTENT_UNSPECIFIED
	switch method {
	case corepb.CoreService_ReadElement_FullMethodName:
		intent = corepb.Intent_READ
	case corepb.CoreService_ListDirect_FullMethodName:
		intent = corepb.Intent_LIST
	case corepb.CoreService_QueryRegion_FullMethodName:
		intent = corepb.Intent_QUERY
	case corepb.CoreService_BeginObserve_FullMethodName, corepb.CoreService_PollChanges_FullMethodName, corepb.CoreService_ReleaseObserve_FullMethodName:
		if !g.observe {
			return bad()
		}
	default:
		return bad()
	}
	m := request.ProtoReflect()
	if intent != 0 {
		if !g.intents[intent] {
			return bad()
		}
		field := m.Descriptor().Fields().ByName("handle")
		if field == nil || !m.Has(field) || !proto.Equal(m.Get(field).Message().Interface(), g.handle) {
			return bad()
		}
		if f := m.Descriptor().Fields().ByName(protoreflect.Name("prepare_observe")); f != nil && m.Get(f).Bool() && !g.observe {
			return bad()
		}
	}
	return g.caller, key, nil
}

func (s *Service) authenticatedActor(ctx context.Context, actor values.FactAuthorityRef) bool {
	if s.config.ReadDelegation == nil || !actor.Valid() {
		return false
	}
	pin, known := s.config.ReadDelegation.Peers[actor]
	p, ok := peer.FromContext(ctx)
	if !known || !ok {
		return false
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	return ok && len(tlsInfo.State.VerifiedChains) != 0 && len(tlsInfo.State.PeerCertificates) != 0 &&
		sha256.Sum256(tlsInfo.State.PeerCertificates[0].RawSubjectPublicKeyInfo) == pin
}
