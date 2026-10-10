package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"testing"
	"time"

	"ucupa/shared/values"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

func federationHostFixture(t *testing.T) (coregrpc.Config, federationConfig) {
	t.Helper()
	c, peer := facetConfigFixture(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	host := c.networkConfig
	host.Expected, host.MaxMetadataBytes, host.MaxResponseBytes = c.Expected, c.MaxMetadataBytes, c.MaxResponseBytes
	host.Certificate = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return host, federationConfig{TTL: "20s", MaxParts: 16, Peers: []mountConnectionConfig{{NextHop: peer.NextHop, Expected: peer.Expected, CAFile: peer.CAFile, SPKI: peer.SPKI}}}
}

func TestFederationHostExplicitConfiguration(t *testing.T) {
	host, configured := federationHostFixture(t)
	// This unreachable peer is intentional: startup validates trust material,
	// but does not contact or discover any space before an authorized request.
	connections, err := configured.open(host)
	if err != nil || connections == nil {
		t.Fatal("explicit configuration rejected", err)
	}
	connections.Close()
	var absent *federationConfig
	if result, err := absent.open(host); err == nil || result != nil {
		t.Fatal("missing configuration silently enabled federation")
	}
	for name, mutate := range map[string]func(*federationConfig, *coregrpc.Config){
		"no peers":          func(c *federationConfig, _ *coregrpc.Config) { c.Peers = nil },
		"too many peers":    func(c *federationConfig, _ *coregrpc.Config) { c.Peers = make([]mountConnectionConfig, 257) },
		"duplicate address": func(c *federationConfig, _ *coregrpc.Config) { c.Peers = append(c.Peers, c.Peers[0]) },
		"missing ttl":       func(c *federationConfig, _ *coregrpc.Config) { c.TTL = "" },
		"zero ttl":          func(c *federationConfig, _ *coregrpc.Config) { c.TTL = "0s" },
		"fractional ttl":    func(c *federationConfig, _ *coregrpc.Config) { c.TTL = "1500ms" },
		"missing capacity":  func(c *federationConfig, _ *coregrpc.Config) { c.MaxParts = 0 },
		"negative capacity": func(c *federationConfig, _ *coregrpc.Config) { c.MaxParts = -1 },
		"self":              func(c *federationConfig, h *coregrpc.Config) { c.Peers[0].Expected = h.Expected },
		"local address":     func(c *federationConfig, _ *coregrpc.Config) { c.Peers[0].NextHop = "local:core" },
		"bad owner":         func(c *federationConfig, _ *coregrpc.Config) { c.Peers[0].Expected = "invalid" },
		"missing ca":        func(c *federationConfig, _ *coregrpc.Config) { c.Peers[0].CAFile += ".missing" },
		"invalid ca":        func(c *federationConfig, _ *coregrpc.Config) { c.Peers[0].CAFile = "federation_test.go" },
		"short pin":         func(c *federationConfig, _ *coregrpc.Config) { c.Peers[0].SPKI = "AQ" },
		"zero pin": func(c *federationConfig, _ *coregrpc.Config) {
			c.Peers[0].SPKI = values.EncodePublicBytes(make([]byte, 32))
		},
		"no actor key":         func(_ *federationConfig, h *coregrpc.Config) { h.Certificate.PrivateKey = nil },
		"no actor certificate": func(_ *federationConfig, h *coregrpc.Config) { h.Certificate.Certificate = nil },
		"no deadline":          func(_ *federationConfig, h *coregrpc.Config) { h.RPCDeadline = 0 },
		"no response limit":    func(_ *federationConfig, h *coregrpc.Config) { h.MaxResponseBytes = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			c, h := configured, host
			c.Peers = append([]mountConnectionConfig(nil), configured.Peers...)
			mutate(&c, &h)
			result, err := c.open(h)
			if result != nil {
				result.Close()
			}
			if err == nil || result != nil {
				t.Fatal("invalid configuration accepted", err)
			}
		})
	}
}
