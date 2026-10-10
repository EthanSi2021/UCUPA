package realmgrpc

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"net"
	"path/filepath"
	"testing"
	"time"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
	wire "ucupa/ucupa-realm/transport/grpc/internal/wire"
)

func baseTestConfig() Config {
	return Config{
		Expected:              "realm-a",
		Certificate:           tls.Certificate{Certificate: [][]byte{{1}}, PrivateKey: ed25519.PrivateKey(make([]byte, 64))},
		ClientRoots:           x509.NewCertPool(),
		AdmittedServices:      map[[32]byte]values.FactAuthorityRef{},
		HandleTTL:             10 * time.Second,
		ChallengeTTL:          10 * time.Second,
		RPCDeadline:           5 * time.Second,
		HandshakeTimeout:      time.Second,
		ConnectionLifetime:    10 * time.Second,
		ShutdownTimeout:       time.Second,
		MaxHandles:            8,
		MaxHandlesPerCaller:   4,
		MaxChallenges:         8,
		MaxChallengesPerOwner: 4,
		MaxRequirements:       8,
		MaxConnections:        4,
		MaxConcurrentRPCs:     4,
		MaxResponseBytes:      1 << 20,
		MaxStreams:            4,
		MaxMetadataBytes:      8192,
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	kernel, err := realminstance.New(realm.Seed{ID: "realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing certificate", func(c *Config) { c.Certificate = tls.Certificate{} }},
		{"missing private key", func(c *Config) { c.Certificate = tls.Certificate{Certificate: [][]byte{{1}}} }},
		{"missing roots", func(c *Config) { c.ClientRoots = nil }},
		{"identity mismatch", func(c *Config) { c.Expected = "other-realm" }},
		{"zero handle ttl", func(c *Config) { c.HandleTTL = 0 }},
		{"sub-second handle ttl", func(c *Config) { c.HandleTTL = 500 * time.Millisecond }},
		{"zero challenge ttl", func(c *Config) { c.ChallengeTTL = 0 }},
		{"sub-second challenge ttl", func(c *Config) { c.ChallengeTTL = 500 * time.Millisecond }},
		{"zero rpc deadline", func(c *Config) { c.RPCDeadline = 0 }},
		{"zero handshake timeout", func(c *Config) { c.HandshakeTimeout = 0 }},
		{"lifetime below deadline", func(c *Config) { c.ConnectionLifetime = c.RPCDeadline - time.Second }},
		{"zero shutdown timeout", func(c *Config) { c.ShutdownTimeout = 0 }},
		{"zero max handles", func(c *Config) { c.MaxHandles = 0 }},
		{"per caller above total", func(c *Config) { c.MaxHandlesPerCaller = c.MaxHandles + 1 }},
		{"zero max challenges", func(c *Config) { c.MaxChallenges = 0 }},
		{"challenges per owner above total", func(c *Config) { c.MaxChallengesPerOwner = c.MaxChallenges + 1 }},
		{"zero max requirements", func(c *Config) { c.MaxRequirements = 0 }},
		{"requirements above protocol ceiling", func(c *Config) { c.MaxRequirements = wire.MaxRequirements + 1 }},
		{"zero max connections", func(c *Config) { c.MaxConnections = 0 }},
		{"zero max concurrent", func(c *Config) { c.MaxConcurrentRPCs = 0 }},
		{"zero max response bytes", func(c *Config) { c.MaxResponseBytes = 0 }},
		{"zero max streams", func(c *Config) { c.MaxStreams = 0 }},
		{"metadata below floor", func(c *Config) { c.MaxMetadataBytes = 255 }},
		{"zero spki service", func(c *Config) {
			c.AdmittedServices = map[[32]byte]values.FactAuthorityRef{{}: handleTestOwner}
		}},
		{"invalid service owner", func(c *Config) {
			c.AdmittedServices = map[[32]byte]values.FactAuthorityRef{{1}: "not-an-uuid"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := baseTestConfig()
			tc.mutate(&config)
			if server, err := New(kernel, config); err == nil {
				server.Stop()
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestNewRejectsInvalidKernel(t *testing.T) {
	if _, err := New(nil, baseTestConfig()); err == nil {
		t.Fatal("nil kernel accepted")
	}
	kernel, err := realminstance.New(realm.Seed{ID: "realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(kernel, baseTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	server.Stop()
	// Only a durable owner can be retired; an in-memory kernel stays live.
	durable, err := realminstance.CreateDurable(realm.Seed{ID: "realm-a"}, filepath.Join(t.TempDir(), "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.Close(); err != nil {
		t.Fatal(err)
	}
	// A retired owner must never be exposed as a live authorization service.
	if server, err := New(durable, baseTestConfig()); err == nil {
		server.Stop()
		t.Fatal("retired kernel accepted")
	}
}

func TestTypedNilVerifierRemainsAbsent(t *testing.T) {
	kernel, err := realminstance.New(realm.Seed{ID: "realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	var verifier *realminstance.IdentityVerifier
	c := baseTestConfig()
	c.Foreign = verifier
	s, err := New(kernel, c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if s.config.Foreign != nil {
		t.Fatal("typed nil enabled foreign identity")
	}
}

func TestNewRejectsForeignWithoutLivenessSources(t *testing.T) {
	kernel, err := realminstance.New(realm.Seed{ID: "realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	verifier := foreignVerifierForTest(t)
	config := baseTestConfig()
	config.Foreign = verifier
	if server, err := New(kernel, config); err == nil {
		server.Stop()
		t.Fatal("issuer trust without liveness sources accepted")
	}
	config = baseTestConfig()
	config.ForeignLiveness = map[values.RealmID]Liveness{"issuer-realm": nil}
	if server, err := New(kernel, config); err == nil {
		server.Stop()
		t.Fatal("liveness sources without issuer trust accepted")
	}
	config.Foreign = verifier
	server, err := New(kernel, config)
	if err != nil {
		t.Fatal(err)
	}
	server.Stop()
}

func TestServeRequiresListenerAndStopsOnce(t *testing.T) {
	kernel, err := realminstance.New(realm.Seed{ID: "realm-a"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(kernel, baseTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(nil); err == nil {
		t.Fatal("nil listener accepted")
	}
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	serve := make(chan error, 1)
	go func() { serve <- server.Serve(first) }()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(second); err == nil {
		t.Fatal("second Serve accepted")
	}
	server.Stop()
	server.Stop()
	select {
	case err := <-serve:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Stop")
	}
	third, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if err := server.Serve(third); err == nil {
		t.Fatal("serving a stopped service accepted")
	}
}
