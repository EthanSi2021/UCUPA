package main

import (
	"testing"
	"ucupa/shared/values"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

func TestTransitionApprovalConfigurationIsExplicit(t *testing.T) {
	if got, err := (*transitionApprovalConfig)(nil).load(false); got != nil || err != nil {
		t.Fatalf("absent option enabled approvals: %v %v", got, err)
	}
	for name, c := range map[string]transitionApprovalConfig{
		"valid":                 {MaxTTL: "5s", MaxApprovals: 3, MaxPerCaller: 2},
		"zero ttl":              {MaxTTL: "0s", MaxApprovals: 3, MaxPerCaller: 2},
		"fractional ttl":        {MaxTTL: "500ms", MaxApprovals: 3, MaxPerCaller: 2},
		"no capacity":           {MaxTTL: "5s", MaxPerCaller: 2},
		"caller exceeds global": {MaxTTL: "5s", MaxApprovals: 1, MaxPerCaller: 2},
		"no caller cap":         {MaxTTL: "5s", MaxApprovals: 3},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := c.load(true); (err == nil) != (name == "valid") {
				t.Fatalf("configuration error: %v", err)
			}
			if _, err := c.load(false); err == nil {
				t.Fatal("accepted without explicit peer trust")
			}
		})
	}
}

func TestTransitionConnectionConfigurationIsExplicit(t *testing.T) {
	host, federation := federationHostFixture(t)
	peer := federation.Peers[0]
	pin, err := values.DecodePublicBytes(peer.SPKI)
	if err != nil {
		t.Fatal(err)
	}
	host.ReadDelegation = &coregrpc.ReadDelegationConfig{Peers: map[values.FactAuthorityRef][32]byte{peer.Expected: [32]byte(pin)}}
	configured := transitionConnectionConfig{Peers: []transitionPeerConfig{{NextHop: peer.NextHop, Expected: peer.Expected, CAFile: peer.CAFile, SPKI: peer.SPKI}}, MaxConcurrentPulls: 2, PullDeadline: "5s"}
	connections, closeClients, err := configured.open(host)
	if err != nil || connections == nil {
		t.Fatalf("explicit configuration: %v", err)
	}
	closeClients()
	if r, _, err := (*transitionConnectionConfig)(nil).open(host); err == nil || r != nil {
		t.Fatal("missing configuration enabled transfer")
	}
	for name, mutate := range map[string]func(*transitionConnectionConfig, *coregrpc.Config){
		"empty":              func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers = nil },
		"too many":           func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers = make([]transitionPeerConfig, 1025) },
		"duplicate":          func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers = append(c.Peers, c.Peers[0]) },
		"missing deadline":   func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.PullDeadline = "" },
		"negative deadline":  func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.PullDeadline = "-1s" },
		"missing capacity":   func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.MaxConcurrentPulls = 0 },
		"excessive capacity": func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.MaxConcurrentPulls = 1025 },
		"local address":      func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers[0].NextHop = "local:peer" },
		"invalid ref":        func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers[0].Expected = "invalid" },
		"self":               func(c *transitionConnectionConfig, h *coregrpc.Config) { c.Peers[0].Expected = h.Expected },
		"no trust":           func(_ *transitionConnectionConfig, h *coregrpc.Config) { h.ReadDelegation = nil },
		"missing ca":         func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers[0].CAFile += ".missing" },
		"wrong pin": func(c *transitionConnectionConfig, _ *coregrpc.Config) {
			c.Peers[0].SPKI = values.EncodePublicBytes(make([]byte, 32))
		},
		"short pin":    func(c *transitionConnectionConfig, _ *coregrpc.Config) { c.Peers[0].SPKI = "AQ" },
		"no actor key": func(_ *transitionConnectionConfig, h *coregrpc.Config) { h.Certificate.PrivateKey = nil },
	} {
		t.Run(name, func(t *testing.T) {
			c, h := configured, host
			c.Peers = append([]transitionPeerConfig(nil), configured.Peers...)
			mutate(&c, &h)
			result, closeClients, err := c.open(h)
			if closeClients != nil {
				closeClients()
			}
			if err == nil || result != nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
