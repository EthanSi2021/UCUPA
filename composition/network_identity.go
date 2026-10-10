package composition

import (
	"context"
	"errors"
	"time"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// RealmIdentityProofs adapts the issuer without teaching Core about Realm.
// This service port is separate from the consumer's fact authorization.
type RealmIdentityProofs struct{ Issuer *realminstance.IdentityIssuer }

func identityError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, realm.ErrStore):
		return values.PublicUnavailable
	case errors.Is(err, realm.ErrInvalid):
		return values.PublicInvalid
	case errors.Is(err, values.PublicResourceExhausted):
		return values.PublicResourceExhausted
	default:
		return values.PublicDenied
	}
}

// Issue and Check carry the original request's context: under a network
// caller source the issuer's re-validation is a real remote wait.
func (p RealmIdentityProofs) Issue(ctx context.Context, c values.CallerContext, actor values.FactAuthorityRef, challenge [32]byte, ttl time.Duration) ([]byte, []byte, error) {
	if p.Issuer == nil {
		return nil, nil, values.PublicUnavailable
	}
	proof, err := p.Issuer.Issue(ctx, c, actor, challenge, ttl)
	return proof.Payload, proof.Signature, identityError(err)
}

func (p RealmIdentityProofs) Check(ctx context.Context, payload, signature []byte) error {
	if p.Issuer == nil {
		return values.PublicUnavailable
	}
	return identityError(p.Issuer.Check(ctx, realm.IdentityProof{Payload: payload, Signature: signature}))
}
