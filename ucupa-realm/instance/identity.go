package instance

import (
	"crypto/ed25519"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-realm/contract"
	"ucupa/ucupa-realm/internal/identity"
)

type IdentityIssuer = identity.IdentityIssuer
type IdentityVerifier = identity.IdentityVerifier

func NewIdentityIssuer(source contract.CallerSource, config contract.IdentityIssuerConfig) (*IdentityIssuer, error) {
	return identity.NewIdentityIssuer(source, config)
}
func NewIdentityVerifier(keys map[values.RealmID]ed25519.PublicKey, maxTTL time.Duration) (*IdentityVerifier, error) {
	return identity.NewIdentityVerifier(keys, maxTTL)
}
