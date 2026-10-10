package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"time"

	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

type identityIssuerConfig struct {
	SigningKeyFile string `json:"signing_key_file"`
	MaxTTL         string `json:"max_ttl"`
	MaxProofs      int    `json:"max_proofs"`
	MaxPerCaller   int    `json:"max_per_caller"`
}

// open wires the issuer to its caller source: the in-process Realm kernel in
// the local profile, or the owner's pinned network Realm contact face.
func (c *identityIssuerConfig) open(source realm.CallerSource) (*realminstance.IdentityIssuer, error) {
	bad := errors.New("invalid identity issuer configuration")
	ttl, err := time.ParseDuration(c.MaxTTL)
	if err != nil {
		return nil, bad
	}
	f, err := os.Open(c.SigningKeyFile)
	if err != nil {
		return nil, bad
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(data) > 16384 {
		return nil, bad
	}
	defer clear(data)
	block, rest := pem.Decode(data)
	if block == nil || len(rest) != 0 || block.Type != "PRIVATE KEY" || len(block.Headers) != 0 {
		return nil, bad
	}
	defer clear(block.Bytes)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, bad
	}
	signing, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, bad
	}
	defer clear(signing)
	issuer, err := realminstance.NewIdentityIssuer(source, realm.IdentityIssuerConfig{Key: signing, MaxTTL: ttl, MaxProofs: c.MaxProofs, MaxPerCaller: c.MaxPerCaller})
	if err != nil {
		return nil, bad
	}
	return issuer, nil
}
