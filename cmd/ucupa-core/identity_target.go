package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realminstance "ucupa/ucupa-realm/instance"
)

type externalIdentityConfig struct {
	MaxTTL  string                                  `json:"max_ttl"`
	Issuers map[values.RealmID]identitySourceConfig `json:"issuers"`
}

type identitySourceConfig struct {
	SigningPublicKeyFile string                  `json:"signing_public_key_file"`
	NextHop              values.NextHop          `json:"next_hop"`
	Expected             values.FactAuthorityRef `json:"expected_core"`
	CAFile               string                  `json:"ca_file"`
	SPKI                 string                  `json:"spki_sha256"`
}

// identityTargetOwner is the receiving owner's current durable contact port
// used for target-local caller establishment.
type identityTargetOwner interface {
	CurrentRealmContact() (values.RealmID, values.NextHop, error)
}

// open wires external identity establishment to the receiving owner's Realm
// connections face: the local binding or the pinned network contact.
func (c *externalIdentityConfig) open(connections composition.RealmIdentityEstablishment, receiver identityTargetOwner, host coregrpc.Config) (*composition.RealmExternalIdentity, func(), error) {
	bad := errors.New("invalid external identity configuration")
	var clients []*coresdk.Client
	closeClients := func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}
	ok := false
	defer func() {
		if !ok {
			closeClients()
		}
	}()
	ttl, err := time.ParseDuration(c.MaxTTL)
	if err != nil || len(c.Issuers) == 0 {
		return nil, nil, bad
	}
	keys := make(map[values.RealmID]ed25519.PublicKey)
	sources := make(map[values.RealmID]composition.IdentityLiveness)
	for id, source := range c.Issuers {
		data, err := readIdentityFile(source.SigningPublicKeyFile, 16384)
		if err != nil {
			return nil, nil, bad
		}
		block, rest := pem.Decode(data)
		if block == nil || len(rest) != 0 || block.Type != "PUBLIC KEY" || len(block.Headers) != 0 {
			return nil, nil, bad
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, nil, bad
		}
		key, valid := parsed.(ed25519.PublicKey)
		if !valid {
			return nil, nil, bad
		}
		keys[id] = key
		ca, err := readIdentityFile(source.CAFile, 1<<20)
		if err != nil {
			return nil, nil, bad
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			return nil, nil, bad
		}
		pin, err := values.DecodePublicBytes(source.SPKI)
		if err != nil || len(pin) != 32 {
			return nil, nil, bad
		}
		client, err := coresdk.New(coresdk.Config{NextHop: source.NextHop, Expected: source.Expected, Roots: roots, SPKISHA256: [32]byte(pin), RPCDeadline: host.RPCDeadline, ConnectTimeout: host.HandshakeTimeout, MaxResponseBytes: host.MaxResponseBytes, MaxMetadataBytes: host.MaxMetadataBytes, Actor: host.Expected, ActorCertificate: &host.Certificate})
		if err != nil {
			return nil, nil, bad
		}
		clients = append(clients, client)
		sources[id] = composition.NetworkIdentityCheck{Client: client, Verifier: host.Expected}
	}
	verifier, err := realminstance.NewIdentityVerifier(keys, ttl)
	if err != nil {
		return nil, nil, bad
	}
	result, err := composition.NewRealmExternalIdentity(connections, receiver, verifier, sources)
	if err != nil {
		return nil, nil, bad
	}
	ok = true
	return result, closeClients, nil
}

func readIdentityFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("identity file unavailable")
	}
	return data, nil
}
