package composition

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// IdentityLiveness is the exact configured issuer's authenticated service port.
// It is not an address resolver and never accepts a URL from the proof.
type IdentityLiveness interface {
	Check(context.Context, []byte, []byte) error
}

type NetworkIdentityCheck struct {
	Client   *coresdk.Client
	Verifier values.FactAuthorityRef
}

func (c NetworkIdentityCheck) Check(ctx context.Context, payload, signature []byte) error {
	if c.Client == nil || !c.Verifier.Valid() {
		return values.PublicUnavailable
	}
	out, err := c.Client.IdentityProofs().CheckIdentityProof(ctx, &networkpb.CheckIdentityProofRequest{Verifier: proto.String(string(c.Verifier)), Proof: &networkpb.IdentityProof{Payload: payload, Signature: signature}})
	if status.Code(err) == codes.Unauthenticated {
		return values.PublicDenied
	}
	if err != nil || !out.GetValid() {
		return values.PublicUnavailable
	}
	return nil
}

// RealmIdentityEstablishment is the connections-port face for the P6 identity
// target. Every call names the owner's persisted address and expected Realm;
// each profile implements exactly the establishment its contact can verify.
// EstablishForeignCaller is a real remote wait under a network contact and
// carries the original request's context.
type RealmIdentityEstablishment interface {
	TrustLocalCaller(values.NextHop, values.RealmID, values.PrincipalRef) (values.CallerContext, error)
	EstablishForeignCaller(context.Context, values.NextHop, values.RealmID, values.FactAuthorityRef, []byte, []byte, [32]byte) (values.CallerContext, time.Time, error)
}

// RealmExternalIdentity creates a target-local establishment only after the
// issuer signature AND current authenticated issuer check pass. Keys and
// connections are supplied explicitly at startup, never learned from a mount.
// A network contact routes the whole foreign chain to the serving Realm —
// proof, challenge, issuer trust and liveness are verified there — while the
// local contact keeps verifying in this process and assembling the caller
// through the trusted local face. Grant maintenance (Check) always verifies
// the retained proof at the pinned issuer liveness source in this process.
type RealmExternalIdentity struct {
	connections RealmIdentityEstablishment
	owner       realmContactSource
	verifier    *realminstance.IdentityVerifier
	issuers     map[values.RealmID]IdentityLiveness
}

func NewRealmExternalIdentity(connections RealmIdentityEstablishment, owner realmContactSource, verifier *realminstance.IdentityVerifier, issuers map[values.RealmID]IdentityLiveness) (*RealmExternalIdentity, error) {
	if connections == nil || owner == nil || verifier == nil || len(issuers) == 0 {
		return nil, values.PublicInvalid
	}
	x := &RealmExternalIdentity{connections: connections, owner: owner, verifier: verifier, issuers: make(map[values.RealmID]IdentityLiveness)}
	for id, source := range issuers {
		// Preserve the signed namespace exactly. A configured issuer may serve
		// the same logical Realm; there is no foreign-to-local subject mapping.
		if id == "" || source == nil {
			return nil, values.PublicInvalid
		}
		x.issuers[id] = source
	}
	return x, nil
}

func (x *RealmExternalIdentity) verified(ctx context.Context, payload, signature []byte, actor values.FactAuthorityRef, challenge [32]byte) (realm.IdentityClaim, error) {
	if x == nil {
		return realm.IdentityClaim{}, values.PublicUnavailable
	}
	c, err := x.verifier.Verify(realm.IdentityProof{Payload: payload, Signature: signature}, actor, challenge, time.Now())
	if err != nil {
		return realm.IdentityClaim{}, values.PublicDenied
	}
	source, ok := x.issuers[c.Issuer]
	if !ok {
		return realm.IdentityClaim{}, values.PublicDenied
	}
	if err := source.Check(ctx, payload, signature); err != nil {
		return realm.IdentityClaim{}, err
	}
	// The network wait cannot extend the signed validity interval.
	if !time.Now().Before(time.Unix(c.Expires, 0)) {
		return realm.IdentityClaim{}, values.PublicDenied
	}
	return c, nil
}

func (x *RealmExternalIdentity) Establish(ctx context.Context, payload, signature []byte, actor values.FactAuthorityRef, challenge [32]byte) (values.CallerContext, time.Time, error) {
	realmID, address, err := x.owner.CurrentRealmContact()
	if err != nil || realmID == "" || !address.Valid() {
		return values.CallerContext{}, time.Time{}, values.PublicUnavailable
	}
	if _, ok := address.NetworkTarget(); ok {
		// The remote Realm runs the full foreign chain itself and mints the
		// run-local handle; a failure there is never retried against another
		// address, Realm or a local assembly path.
		caller, expires, err := x.connections.EstablishForeignCaller(ctx, address, realmID, actor, payload, signature, challenge)
		if err != nil {
			return values.CallerContext{}, time.Time{}, identityError(err)
		}
		return caller, expires, nil
	}
	c, err := x.verified(ctx, payload, signature, actor, challenge)
	if err != nil {
		return values.CallerContext{}, time.Time{}, err
	}
	caller, err := x.connections.TrustLocalCaller(address, realmID, values.PrincipalRef{Realm: c.Issuer, Principal: c.Subject})
	if err != nil {
		return values.CallerContext{}, time.Time{}, identityError(err)
	}
	return caller, time.Unix(c.Expires, 0), nil
}

func (x *RealmExternalIdentity) Check(ctx context.Context, payload, signature []byte, actor values.FactAuthorityRef, challenge [32]byte) error {
	_, err := x.verified(ctx, payload, signature, actor, challenge)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return values.PublicUnavailable
	}
	return err
}

// RealmChallengeSource mints identity challenges at the serving Core's
// persistent Realm contact, so one proof's challenge is single-use both in
// this Core's request-bound registry and at the Realm that will verify the
// proof. The local profile keeps its in-process mint and never wires this.
type RealmChallengeSource struct {
	Connections RealmChallengeMinter
	Owner       realmContactSource
}

// RealmChallengeMinter is the connections-port face that reaches the serving
// Realm's own challenge registry bound to this Core's service owner. Under a
// network contact the mint is a real remote wait and carries the original
// request's context.
type RealmChallengeMinter interface {
	BeginChallenge(context.Context, values.NextHop, values.RealmID) ([32]byte, time.Time, error)
}

func (s RealmChallengeSource) BeginChallenge(ctx context.Context) ([32]byte, time.Time, error) {
	if s.Owner == nil || s.Connections == nil {
		return [32]byte{}, time.Time{}, values.PublicUnavailable
	}
	realmID, address, err := s.Owner.CurrentRealmContact()
	if err != nil || realmID == "" || !address.Valid() {
		return [32]byte{}, time.Time{}, values.PublicUnavailable
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, time.Time{}, values.PublicUnavailable
	}
	challenge, expires, err := s.Connections.BeginChallenge(ctx, address, realmID)
	if err != nil {
		return [32]byte{}, time.Time{}, values.PublicUnavailable
	}
	if challenge == ([32]byte{}) || !time.Now().Before(expires) {
		return [32]byte{}, time.Time{}, values.PublicUnavailable
	}
	return challenge, expires, nil
}
