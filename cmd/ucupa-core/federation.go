package main

import (
	"crypto/x509"
	"errors"
	"time"

	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	federationconn "ucupa/ucupa-core/transport/grpc/federationconn"
)

type federationConfig struct {
	TTL      string                  `json:"ttl"`
	MaxParts int                     `json:"max_parts"`
	Peers    []mountConnectionConfig `json:"peers"`
}

// These are exact-address transport trust pins, not a Core ID resolver. A
// request can contact a peer only after the owner follows a permitted spatial
// relationship to that address. No service credential replaces the requester.
func (c *federationConfig) open(host coregrpc.Config) (*federationconn.FederationConnections, error) {
	bad := errors.New("invalid federation connection configuration")
	if c == nil || len(c.Peers) == 0 || len(c.Peers) > 256 || c.MaxParts <= 0 {
		return nil, bad
	}
	ttl, err := time.ParseDuration(c.TTL)
	if err != nil || ttl <= 0 || ttl%time.Second != 0 {
		return nil, bad
	}
	peers := make([]coresdk.Config, 0, len(c.Peers))
	for _, entry := range c.Peers {
		data, err := readIdentityFile(entry.CAFile, 1<<20)
		if err != nil {
			return nil, bad
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, bad
		}
		pin, err := values.DecodePublicBytes(entry.SPKI)
		if err != nil || len(pin) != 32 {
			return nil, bad
		}
		peers = append(peers, coresdk.Config{NextHop: entry.NextHop, Expected: entry.Expected,
			Roots: roots, SPKISHA256: [32]byte(pin), RPCDeadline: host.RPCDeadline,
			ConnectTimeout: host.HandshakeTimeout, MaxResponseBytes: host.MaxResponseBytes,
			MaxMetadataBytes: host.MaxMetadataBytes, Actor: host.Expected, ActorCertificate: &host.Certificate})
	}
	connections, err := federationconn.NewFederationConnections(federationconn.FederationConnectionConfig{
		Source: host.Expected, Peers: peers, MaxParts: c.MaxParts, TTL: ttl,
	})
	if err != nil {
		return nil, bad
	}
	return connections, nil
}
