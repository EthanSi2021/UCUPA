package identity

import (
	"crypto/ed25519"
	"encoding/json"
	"time"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
)

// IdentityVerifier binds an exact issuer namespace to a locally configured key.
// Successful Verify is only a signature/target-request check: a network receiver
// must also Check at the issuer, bind the TLS actor and independently authorize.
// It deliberately cannot return a trusted CallerContext or grant permissions.
type IdentityVerifier struct {
	keys   map[values.RealmID]ed25519.PublicKey
	maxTTL time.Duration
}

func NewIdentityVerifier(keys map[values.RealmID]ed25519.PublicKey, maxTTL time.Duration) (*IdentityVerifier, error) {
	if len(keys) == 0 || !identityTTL(maxTTL) {
		return nil, contract.ErrInvalid
	}
	v := &IdentityVerifier{keys: make(map[values.RealmID]ed25519.PublicKey), maxTTL: maxTTL}
	for issuer, key := range keys {
		if issuer == "" || len(issuer) > 1024 || len(key) != ed25519.PublicKeySize {
			return nil, contract.ErrInvalid
		}
		v.keys[issuer] = append(ed25519.PublicKey(nil), key...)
	}
	return v, nil
}

func (v *IdentityVerifier) Verify(proof contract.IdentityProof, actor values.FactAuthorityRef, challenge [32]byte, now time.Time) (contract.IdentityClaim, error) {
	bad := func() (contract.IdentityClaim, error) { return contract.IdentityClaim{}, contract.ErrUnauthenticated }
	if v == nil || len(proof.Payload) > 4096 || len(proof.Signature) != ed25519.SignatureSize || !actor.Valid() || challenge == ([32]byte{}) {
		return bad()
	}
	// Strict canonical parsing rejects duplicate/unknown fields, noncanonical
	// numbers/encoding and trailing material before any identity is returned.
	value, err := values.ParseCanonicalJSON(proof.Payload, values.JSONLimits{Bytes: 4096, Depth: 2, Nodes: 16, ContainerEntries: 7, StringBytes: 2048})
	if err != nil {
		return bad()
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 7 {
		return bad()
	}
	for _, name := range []string{"issuer", "subject", "actor", "challenge", "nonce", "issued", "expires"} {
		if _, ok := object[name]; !ok {
			return bad()
		}
	}
	var c contract.IdentityClaim
	if json.Unmarshal(proof.Payload, &c) != nil || !(values.PrincipalRef{Realm: c.Issuer, Principal: c.Subject}).Valid() || c.Actor != actor || c.Challenge != values.EncodePublicBytes(challenge[:]) {
		return bad()
	}
	nonce, err := values.DecodePublicBytes(c.Nonce)
	if err != nil || len(nonce) != 32 || [32]byte(nonce) == ([32]byte{}) {
		return bad()
	}
	if c.Issued < 0 || c.Issued > now.Unix() || c.Expires <= now.Unix() || c.Expires <= c.Issued || c.Expires-c.Issued > int64(v.maxTTL/time.Second) {
		return bad()
	}
	key, ok := v.keys[c.Issuer]
	if !ok || !ed25519.Verify(key, append([]byte(identityDomain), proof.Payload...), proof.Signature) {
		return bad()
	}
	return c, nil
}
