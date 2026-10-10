package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

const identityDomain = "ucupa.identity.assertion\x00"

type retainedIdentity struct {
	caller  values.CallerContext
	expires time.Time
}

// IdentityIssuer is a bounded, run-local adjunct of one Realm's authentication.
// It does not create principals, bindings, permission or persistent state, and
// it holds no Realm object: every proof and liveness check re-validates the
// caller through the caller source's current establishment.
type IdentityIssuer struct {
	mu     sync.Mutex
	source contract.CallerSource
	config contract.IdentityIssuerConfig
	proofs map[[32]byte]retainedIdentity
	closed bool
	now    func() time.Time
}

func NewIdentityIssuer(source contract.CallerSource, c contract.IdentityIssuerConfig) (*IdentityIssuer, error) {
	if source == nil || len(c.Key) != ed25519.PrivateKeySize || !identityTTL(c.MaxTTL) || c.MaxProofs <= 0 || c.MaxPerCaller <= 0 || c.MaxPerCaller > c.MaxProofs {
		return nil, contract.ErrInvalid
	}
	// Reject malformed private keys rather than signing with inconsistent halves.
	derived := ed25519.NewKeyFromSeed(c.Key.Seed())
	if !bytes.Equal(derived, c.Key) {
		return nil, contract.ErrInvalid
	}
	c.Key = append(ed25519.PrivateKey(nil), c.Key...)
	return &IdentityIssuer{source: source, config: c, proofs: make(map[[32]byte]retainedIdentity), now: time.Now}, nil
}

func identityTTL(d time.Duration) bool {
	return d > 0 && d%time.Second == 0
}

// Issue requires an authenticated credential, not just a self-declared subject
// or a trusted-local caller. The target's challenge is opaque to the issuer.
// The caller re-validation runs outside the registry lock — under a network
// source it is a real remote wait — and the registry is re-read under the lock
// afterwards: a close, expiry or capacity change during the wait decides the
// request, never a value captured before it.
func (s *IdentityIssuer) Issue(ctx context.Context, caller values.CallerContext, actor values.FactAuthorityRef, challenge [32]byte, ttl time.Duration) (contract.IdentityProof, error) {
	if s == nil || !actor.Valid() || challenge == ([32]byte{}) || !identityTTL(ttl) || ttl > s.config.MaxTTL {
		return contract.IdentityProof{}, contract.ErrInvalid
	}
	if ctx == nil {
		return contract.IdentityProof{}, contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return contract.IdentityProof{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contract.IdentityProof{}, contract.ErrStore
	}
	s.mu.Unlock()
	// Fork performs the current Realm credential/proof check without retaining
	// the secret. It also rejects trusted-local and unproved foreign identities.
	bound, err := s.source.ForkCaller(ctx, caller)
	if err != nil {
		return contract.IdentityProof{}, err
	}
	retained := false
	defer func() {
		if !retained {
			s.releaseCaller(ctx, bound)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contract.IdentityProof{}, contract.ErrStore
	}
	if err := ctx.Err(); err != nil {
		return contract.IdentityProof{}, err
	}
	now := s.now()
	s.expireLocked(now)
	count := 0
	for _, p := range s.proofs {
		if p.caller.Subject == bound.Subject {
			count++
		}
	}
	if len(s.proofs) >= s.config.MaxProofs || count >= s.config.MaxPerCaller {
		return contract.IdentityProof{}, values.PublicResourceExhausted
	}
	var nonce [32]byte
	_, _ = rand.Read(nonce[:])
	claim := contract.IdentityClaim{Issuer: bound.Subject.Realm, Subject: bound.Subject.Principal, Actor: actor,
		Challenge: values.EncodePublicBytes(challenge[:]), Nonce: values.EncodePublicBytes(nonce[:]),
		Issued: now.Unix(), Expires: now.Unix() + int64(ttl/time.Second)}
	// Whole-second expiry is rounded down, never extending the requested lease.
	payload, err := identityPayload(claim)
	if err != nil || len(payload) > 4096 {
		return contract.IdentityProof{}, contract.ErrInvalid
	}
	signature := ed25519.Sign(s.config.Key, append([]byte(identityDomain), payload...))
	s.proofs[sha256.Sum256(payload)] = retainedIdentity{caller: bound, expires: now.Add(ttl - time.Duration(now.Nanosecond()))}
	retained = true
	return contract.IdentityProof{Payload: payload, Signature: signature}, nil
}

// Check proves identity liveness at this issuer's current Realm read. It does
// not authorize an operation and must be reached over authenticated transport.
// Unknown, expired, forged, and revoked statements have the same rejection.
// The caller re-validation runs outside the registry lock — under a network
// source it is a real remote wait — and the retained record is re-read under
// the lock afterwards: the wait never extends a proof's life, and a close,
// expiry or removal during the wait rejects the statement.
func (s *IdentityIssuer) Check(ctx context.Context, proof contract.IdentityProof) error {
	if s == nil {
		return contract.ErrStore
	}
	if ctx == nil {
		return contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key := sha256.Sum256(proof.Payload)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contract.ErrStore
	}
	if len(proof.Payload) > 4096 || len(proof.Signature) != ed25519.SignatureSize {
		s.mu.Unlock()
		return contract.ErrUnauthenticated
	}
	s.expireLocked(s.now())
	p, ok := s.proofs[key]
	if !ok || !ed25519.Verify(s.config.Key.Public().(ed25519.PublicKey), append([]byte(identityDomain), proof.Payload...), proof.Signature) {
		s.mu.Unlock()
		return contract.ErrUnauthenticated
	}
	caller := p.caller
	s.mu.Unlock()
	bound, err := s.source.ForkCaller(ctx, caller)
	if err == nil {
		s.releaseCaller(ctx, bound)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return contract.ErrStore
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.expireLocked(s.now())
	if _, ok := s.proofs[key]; !ok {
		return contract.ErrUnauthenticated
	}
	if errors.Is(err, contract.ErrUnauthenticated) {
		delete(s.proofs, key)
	}
	return err
}

func (s *IdentityIssuer) releaseCaller(ctx context.Context, caller values.CallerContext) {
	if releaser, ok := s.source.(interface {
		ReleaseCaller(context.Context, values.CallerContext)
	}); ok {
		releaser.ReleaseCaller(ctx, caller)
	}
}

func (s *IdentityIssuer) expireLocked(now time.Time) {
	for key, p := range s.proofs {
		if !now.Before(p.expires) {
			delete(s.proofs, key)
		}
	}
}

func (s *IdentityIssuer) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	clear(s.proofs)
	clear(s.config.Key)
}

func identityPayload(c contract.IdentityClaim) ([]byte, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	return values.CanonicalJSONValue(object)
}
