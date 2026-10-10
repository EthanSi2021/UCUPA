package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
	realmgrpc "ucupa/ucupa-realm/transport/grpc"
)

// foreignConfig declares the explicit issuer trust set for foreign identity
// establishment: one signing public key per issuer Realm plus the pinned Core
// target that serves that issuer's online proof check under the network
// access contract. No issuer is learned from a proof, and there is no local
// or offline fallback.
type foreignConfig struct {
	MaxTTL  string                           `json:"max_proof_ttl"`
	Issuers map[values.RealmID]foreignIssuer `json:"issuers"`
}

// foreignIssuer names one trusted issuer. Verifier is the receiving service's
// own delegation identity at that issuer's Core: the pinned certificate pair
// presented by the online check client.
type foreignIssuer struct {
	SigningPublicKeyFile string                  `json:"signing_public_key_file"`
	NextHop              values.NextHop          `json:"next_hop"`
	ExpectedCore         values.FactAuthorityRef `json:"expected_core"`
	CAFile               string                  `json:"ca_file"`
	SPKI                 string                  `json:"spki_sha256"`
	Verifier             values.FactAuthorityRef `json:"verifier"`
}

// open assembles the verifier and one pinned online check client per issuer
// onto the network configuration. A nil section leaves the foreign port
// disabled: the service then answers foreign establishment with Unavailable
// instead of accepting any proof. A present section must be complete, or the
// product refuses to start.
func (c *foreignConfig) open(network realmgrpc.Config) (realmgrpc.Config, func(), error) {
	var clients []*coresdk.Client
	closeClients := func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}
	if c == nil {
		return network, closeClients, nil
	}
	bad := errors.New("invalid foreign identity configuration")
	ok := false
	defer func() {
		if !ok {
			closeClients()
		}
	}()
	ttl, err := time.ParseDuration(c.MaxTTL)
	if err != nil || ttl <= 0 || len(c.Issuers) == 0 || len(c.Issuers) > 256 {
		return network, closeClients, bad
	}
	keys := make(map[values.RealmID]ed25519.PublicKey, len(c.Issuers))
	liveness := make(map[values.RealmID]realmgrpc.Liveness, len(c.Issuers))
	for id, issuer := range c.Issuers {
		if id == "" || !issuer.ExpectedCore.Valid() || !issuer.Verifier.Valid() {
			return network, closeClients, bad
		}
		key, err := readSigningPublicKey(issuer.SigningPublicKeyFile)
		if err != nil {
			return network, closeClients, bad
		}
		keys[id] = key
		ca, err := readLimited(issuer.CAFile, 1<<20)
		if err != nil {
			return network, closeClients, bad
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(ca) {
			return network, closeClients, bad
		}
		raw, err := decodeFixedHex(issuer.SPKI, 32)
		if err != nil {
			return network, closeClients, bad
		}
		var pin [32]byte
		copy(pin[:], raw)
		client, err := coresdk.New(coresdk.Config{
			NextHop:          issuer.NextHop,
			Expected:         issuer.ExpectedCore,
			Roots:            roots,
			SPKISHA256:       pin,
			RPCDeadline:      network.RPCDeadline,
			ConnectTimeout:   network.HandshakeTimeout,
			MaxResponseBytes: network.MaxResponseBytes,
			MaxMetadataBytes: network.MaxMetadataBytes,
			Actor:            issuer.Verifier,
			ActorCertificate: &network.Certificate,
		})
		if err != nil {
			return network, closeClients, bad
		}
		clients = append(clients, client)
		liveness[id] = issuerLiveness{check: composition.NetworkIdentityCheck{Client: client, Verifier: issuer.Verifier}}
	}
	verifier, err := realminstance.NewIdentityVerifier(keys, ttl)
	if err != nil {
		return network, closeClients, bad
	}
	network.Foreign = verifier
	network.ForeignLiveness = liveness
	ok = true
	return network, closeClients, nil
}

// issuerLiveness presents the pinned P6 network check as the transport's
// online liveness port. The denial outcome is the issuer's own rejection of
// the proof; every other outcome is unavailability, never an offline pass.
type issuerLiveness struct {
	check composition.IdentityLiveness
}

func (l issuerLiveness) Check(ctx context.Context, payload, signature []byte) error {
	err := l.check.Check(ctx, payload, signature)
	if err != nil {
		if errors.Is(err, values.PublicDenied) {
			return realm.ErrUnauthenticated
		}
		return err
	}
	return nil
}

// readSigningPublicKey loads exactly one PEM PKIX ed25519 public key; any
// other shape is a configuration error, never a weaker key.
func readSigningPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readLimited(path, 16384)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || len(rest) != 0 || block.Type != "PUBLIC KEY" || len(block.Headers) != 0 {
		return nil, errors.New("invalid signing public key file")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, valid := parsed.(ed25519.PublicKey)
	if !valid {
		return nil, errors.New("invalid signing public key file")
	}
	return key, nil
}
