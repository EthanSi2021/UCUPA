package main

import (
	"crypto/x509"
	"errors"
	"time"

	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	"ucupa/ucupa-core/transport/grpc/transitionconn"
)

type transitionConnectionConfig struct {
	Peers              []transitionPeerConfig `json:"peers"`
	MaxConcurrentPulls int                    `json:"max_concurrent_pulls"`
	PullDeadline       string                 `json:"pull_deadline"`
}

type transitionPeerConfig struct {
	NextHop  values.NextHop          `json:"next_hop"`
	Expected values.FactAuthorityRef `json:"expected_core"`
	CAFile   string                  `json:"ca_file"`
	SPKI     string                  `json:"spki_sha256"`
}

// Both directions use the same finite, address-keyed contact set. An entry is
// not a Ref resolver, an approval, or permission to choose another successor.
func (c *transitionConnectionConfig) open(host coregrpc.Config) (*transitionconn.Connections, func(), error) {
	bad := errors.New("invalid transition connection configuration")
	if c == nil || host.ReadDelegation == nil || len(c.Peers) == 0 || len(c.Peers) > 1024 || c.MaxConcurrentPulls <= 0 || c.MaxConcurrentPulls > 1024 {
		return nil, nil, bad
	}
	duration, err := time.ParseDuration(c.PullDeadline)
	if err != nil || duration <= 0 {
		return nil, nil, bad
	}
	var clients []*coresdk.Client
	closeClients := func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}
	ok := false
	defer func() {
		if !ok {
			closeClients()
		}
	}()
	for _, entry := range c.Peers {
		pin, err := values.DecodePublicBytes(entry.SPKI)
		if err != nil || len(pin) != 32 {
			return nil, nil, bad
		}
		trusted, exists := host.ReadDelegation.Peers[entry.Expected]
		if !exists || trusted != [32]byte(pin) {
			return nil, nil, bad
		}
		data, err := readIdentityFile(entry.CAFile, 1<<20)
		if err != nil {
			return nil, nil, bad
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, nil, bad
		}
		client, err := coresdk.New(coresdk.Config{NextHop: entry.NextHop, Expected: entry.Expected, Roots: roots, SPKISHA256: [32]byte(pin),
			RPCDeadline: host.RPCDeadline, ConnectTimeout: host.HandshakeTimeout, MaxResponseBytes: host.MaxResponseBytes, MaxMetadataBytes: host.MaxMetadataBytes,
			Actor: host.Expected, ActorCertificate: &host.Certificate})
		if err != nil {
			return nil, nil, bad
		}
		clients = append(clients, client)
	}
	connections, err := transitionconn.NewConnections(host.Expected, clients, c.MaxConcurrentPulls, duration)
	if err != nil {
		return nil, nil, bad
	}
	ok = true
	return connections, closeClients, nil
}
