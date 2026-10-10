package contract

import (
	"context"
	"crypto/ed25519"
	"time"

	"ucupa/shared/values"
)

// IdentityProof is a signed, short-lived identity statement, not permission.
// The payload has an exact canonical representation; it is not a CallerContext.
type IdentityProof struct {
	Payload   []byte
	Signature []byte
}

// IdentityClaim deliberately contains no target address or authorization view.
// The receiving target must independently bind Challenge to its pending request.
type IdentityClaim struct {
	Issuer    values.RealmID          `json:"issuer"`
	Subject   values.PrincipalID      `json:"subject"`
	Actor     values.FactAuthorityRef `json:"actor"`
	Challenge string                  `json:"challenge"`
	Nonce     string                  `json:"nonce"`
	Issued    int64                   `json:"issued"`
	Expires   int64                   `json:"expires"`
}

// IdentityIssuerConfig has no implicit limits. Signing keys are process
// configuration, not stored credentials or facts. A restart drops all proofs.
type IdentityIssuerConfig struct {
	Key          ed25519.PrivateKey
	MaxTTL       time.Duration
	MaxProofs    int
	MaxPerCaller int
}

// CallerSource re-validates one caller and derives its current establishment.
// The local Realm kernel and a pinned network connection face are the two
// wired sources; a proof is never minted from a self-declared subject. A
// network source is a real remote wait, so every call carries the original
// request context for cancellation, and no caller may hold the issuer's
// registry lock while the fork is in flight.
type CallerSource interface {
	ForkCaller(context.Context, values.CallerContext) (values.CallerContext, error)
}
