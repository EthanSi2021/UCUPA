package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ucupa/shared/values"
	realmgrpc "ucupa/ucupa-realm/transport/grpc"
)

// testCA issues short-lived certificates for product configuration tests.
type testCA struct {
	certificate *x509.Certificate
	key         ed25519.PrivateKey
}

func newTestCA(t *testing.T, commonName string) *testCA {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: commonName}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{certificate: parsed, key: key}
}

func (c *testCA) issue(t *testing.T, commonName string, server bool) (tls.Certificate, [32]byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: commonName}, NotBefore: c.certificate.NotBefore, NotAfter: c.certificate.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature}
	if server {
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, c.certificate, pub, c.key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cert, sha256.Sum256(parsed.RawSubjectPublicKeyInfo)
}

func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const testOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const testElement values.ElementID = "00000000-0000-4000-8000-000000000020"

const validBootstrap = `{
	"realm_id": "realm-a",
	"principals": ["admin", "user"],
	"bindings": [
		{"subject": {"realm": "realm-a", "principal": "admin"}, "permission": "ucupa.realm.binding.put", "scope": {"kind": "realm", "realm": "realm-a"}},
		{"subject": {"realm": "realm-a", "principal": "user"}, "permission": "ucupa.test.read", "scope": {"kind": "core_element", "authority": "00000000-0000-4000-8000-000000000001", "element": "00000000-0000-4000-8000-000000000020"}},
		{"subject": {"realm": "realm-b", "principal": "other"}, "permission": "ucupa.test.read", "scope": {"kind": "facet_key", "authority": "00000000-0000-4000-8000-000000000001", "element": "00000000-0000-4000-8000-000000000020", "facet": "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}},
		{"subject": {"realm": "realm-a", "principal": "admin"}, "permission": "ucupa.test.manage", "scope": {"kind": "authority", "authority": "00000000-0000-4000-8000-000000000001"}},
		{"subject": {"realm": "realm-a", "principal": "admin"}, "permission": "ucupa.test.attach", "scope": {"kind": "facet_element", "authority": "00000000-0000-4000-8000-000000000001", "element": "00000000-0000-4000-8000-000000000020"}},
		{"subject": {"realm": "realm-a", "principal": "admin"}, "permission": "ucupa.test.close", "scope": {"kind": "anchor_closure", "authority": "00000000-0000-4000-8000-000000000001", "element": "00000000-0000-4000-8000-000000000020"}}
	],
	"credentials": [
		{"subject": "user", "audience": "realm-a", "id": "00112233445566778899aabbccddeeff", "secret": "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"}
	]
}`

func TestReadBootstrapParsesExplicitState(t *testing.T) {
	path := writeFile(t, t.TempDir(), "bootstrap.json", []byte(validBootstrap))
	seed, err := readBootstrap(path)
	if err != nil {
		t.Fatal(err)
	}
	if seed.ID != "realm-a" || len(seed.LocalPrincipals) != 2 {
		t.Fatalf("seed header = %v %v", seed.ID, seed.LocalPrincipals)
	}
	if len(seed.Bindings) != 6 {
		t.Fatalf("bindings = %d", len(seed.Bindings))
	}
	if _, _, ok := seed.Bindings[1].Scope.CoreElementTarget(); !ok {
		t.Fatalf("core element binding = %v", seed.Bindings[1].Scope)
	}
	if _, _, ok := seed.Bindings[2].Scope.FacetKeyTarget(); !ok {
		t.Fatalf("facet key binding = %v", seed.Bindings[2].Scope)
	}
	if seed.Credentials[0].Evidence.Audience != "realm-a" || seed.Credentials[0].Evidence.ID[0] != 0x00 {
		t.Fatalf("credential = %v", seed.Credentials[0])
	}
}

func TestReadBootstrapFailsClosed(t *testing.T) {
	base := func(mutate func(map[string]any)) map[string]any {
		value := map[string]any{
			"realm_id":    "realm-a",
			"principals":  []any{"user"},
			"bindings":    []any{},
			"credentials": []any{},
		}
		mutate(value)
		return value
	}
	cases := []struct {
		name     string
		document map[string]any
	}{
		{"unknown kind", base(func(v map[string]any) {
			v["bindings"] = []any{map[string]any{"subject": map[string]any{"realm": "realm-a", "principal": "user"}, "permission": "p", "scope": map[string]any{"kind": "universe", "realm": "realm-a"}}}
		})},
		{"kind field outside shape", base(func(v map[string]any) {
			v["bindings"] = []any{map[string]any{"subject": map[string]any{"realm": "realm-a", "principal": "user"}, "permission": "p", "scope": map[string]any{"kind": "realm", "realm": "realm-a", "element": string(testElement)}}}
		})},
		{"bad facet hex", base(func(v map[string]any) {
			v["bindings"] = []any{map[string]any{"subject": map[string]any{"realm": "realm-b", "principal": "user"}, "permission": "p", "scope": map[string]any{"kind": "facet_key", "authority": string(testOwner), "element": string(testElement), "facet": "zz"}}}
		})},
		{"short credential id", base(func(v map[string]any) {
			v["credentials"] = []any{map[string]any{"subject": "user", "audience": "realm-a", "id": "0011", "secret": strings.Repeat("00", 32)}}
		})},
		{"short credential secret", base(func(v map[string]any) {
			v["credentials"] = []any{map[string]any{"subject": "user", "audience": "realm-a", "id": strings.Repeat("00", 16), "secret": "00"}}
		})},
		{"missing realm identity", base(func(v map[string]any) { v["realm_id"] = "" })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, err := json.Marshal(tc.document)
			if err != nil {
				t.Fatal(err)
			}
			path := writeFile(t, t.TempDir(), "bootstrap.json", document)
			if _, err := readBootstrap(path); err == nil {
				t.Fatal("invalid bootstrap accepted")
			}
		})
	}
	// Unknown JSON fields are rejected even when the declared state is valid.
	path := writeFile(t, t.TempDir(), "bootstrap.json", []byte(`{"realm_id":"r","extra":true}`))
	if _, err := readBootstrap(path); err == nil {
		t.Fatal("unknown bootstrap field accepted")
	}
	// A missing file is an error, not an empty bootstrap.
	if _, err := readBootstrap(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("missing bootstrap file accepted")
	}
}

func (c *testCA) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.certificate)
	return pool
}

// serviceFiles writes one consistent certificate set for readConfig.
func serviceFiles(t *testing.T, dir string) (certPath, keyPath, rootsPath string) {
	t.Helper()
	ca := newTestCA(t, "UCUPA realm product CA")
	server, _ := ca.issue(t, "realm-a", true)
	keyDER, err := x509.MarshalPKCS8PrivateKey(server.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath = writeFile(t, dir, "server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate[0]}))
	keyPath = writeFile(t, dir, "server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	rootsPath = writeFile(t, dir, "roots.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.certificate.Raw}))
	return certPath, keyPath, rootsPath
}

func writeServiceConfig(t *testing.T, dir string, mutate func(map[string]any)) string {
	t.Helper()
	ca := newTestCA(t, "UCUPA realm product client CA")
	_, spki := ca.issue(t, "admitted", false)
	certPath, keyPath, rootsPath := serviceFiles(t, dir)
	document := map[string]any{
		"realm_store":              filepath.Join(dir, "realm.db"),
		"realm_id":                 "realm-a",
		"listen":                   "127.0.0.1:0",
		"next_hop":                 "https://realm-a.invalid:8443",
		"certificate_file":         certPath,
		"private_key_file":         keyPath,
		"client_roots_file":        rootsPath,
		"admitted_services":        []any{map[string]any{"spki_sha256": hex.EncodeToString(spki[:]), "owner": string(testOwner)}},
		"handle_ttl":               "600s",
		"challenge_ttl":            "600s",
		"rpc_deadline":             "10s",
		"handshake_timeout":        "5s",
		"connection_lifetime":      "60s",
		"shutdown_timeout":         "10s",
		"max_handles":              64,
		"max_handles_per_caller":   8,
		"max_challenges":           16,
		"max_challenges_per_owner": 8,
		"max_requirements":         16,
		"max_connections":          16,
		"max_concurrent_rpcs":      8,
		"max_response_bytes":       1048576,
		"max_streams":              64,
		"max_metadata_bytes":       8192,
	}
	if mutate != nil {
		mutate(document)
	}
	b, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, dir, "service.json", b)
}

func TestReadConfigParsesServiceConfiguration(t *testing.T) {
	path := writeServiceConfig(t, t.TempDir(), nil)
	c, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.network.Expected != "realm-a" || c.network.HandleTTL != 600*time.Second ||
		len(c.network.AdmittedServices) != 1 || c.network.ClientRoots == nil ||
		len(c.network.Certificate.Certificate) == 0 {
		t.Fatalf("network configuration = %+v", c.network)
	}
	for spki, owner := range c.network.AdmittedServices {
		if spki == ([32]byte{}) || owner != testOwner {
			t.Fatalf("admitted service = %x %v", spki, owner)
		}
	}
}

func TestReadConfigFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing realm store", func(v map[string]any) { v["realm_store"] = "" }},
		{"missing realm identity", func(v map[string]any) { v["realm_id"] = "" }},
		{"invalid listen", func(v map[string]any) { v["listen"] = "127.0.0.1" }},
		{"invalid advertised address", func(v map[string]any) { v["next_hop"] = "local:realm" }},
		{"unspecified advertised address", func(v map[string]any) { v["next_hop"] = "https://0.0.0.0:8443" }},
		{"non-positive handle ttl", func(v map[string]any) { v["handle_ttl"] = "0s" }},
		{"subsecond handle ttl", func(v map[string]any) { v["handle_ttl"] = "500ms" }},
		{"lifetime below deadline", func(v map[string]any) { v["connection_lifetime"] = "1s" }},
		{"zero handle capacity", func(v map[string]any) { v["max_handles"] = 0 }},
		{"metadata below floor", func(v map[string]any) { v["max_metadata_bytes"] = 255 }},
		{"bad spki hex", func(v map[string]any) {
			v["admitted_services"] = []any{map[string]any{"spki_sha256": "zz", "owner": string(testOwner)}}
		}},
		{"invalid owner", func(v map[string]any) {
			v["admitted_services"] = []any{map[string]any{"spki_sha256": strings.Repeat("00", 32), "owner": "not-an-owner"}}
		}},
		{"duplicate spki", func(v map[string]any) {
			v["admitted_services"] = []any{
				map[string]any{"spki_sha256": strings.Repeat("00", 32), "owner": string(testOwner)},
				map[string]any{"spki_sha256": strings.Repeat("00", 32), "owner": string(testOwner)},
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeServiceConfig(t, t.TempDir(), tc.mutate)
			if _, err := readConfig(path); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	t.Run("unknown field", func(t *testing.T) {
		path := writeServiceConfig(t, t.TempDir(), func(v map[string]any) { v["unexpected"] = true })
		if _, err := readConfig(path); err == nil {
			t.Fatal("unknown configuration field accepted")
		}
	})
	t.Run("unparsable roots", func(t *testing.T) {
		dir := t.TempDir()
		path := writeServiceConfig(t, dir, func(v map[string]any) {
			v["client_roots_file"] = writeFile(t, dir, "bad.pem", []byte("not a certificate"))
		})
		if _, err := readConfig(path); err == nil {
			t.Fatal("invalid client roots accepted")
		}
	})
	t.Run("missing store is not a reopen", func(t *testing.T) {
		path := writeServiceConfig(t, t.TempDir(), nil)
		c, err := readConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if c.RealmStore == "" || !strings.HasSuffix(c.RealmStore, "realm.db") {
			t.Fatalf("realm store = %v", c.RealmStore)
		}
		if _, err := os.Stat(c.RealmStore); !os.IsNotExist(err) {
			t.Fatalf("test premise: unexpected existing Store %v", err)
		}
	})
}

// foreignSection writes one consistent issuer trust set and returns the
// configuration section naming it.
func foreignSection(t *testing.T, dir string) map[string]any {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	verification := writeFile(t, dir, "issuer.pub.pem", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}))
	ca := newTestCA(t, "UCUPA realm foreign issuer CA")
	caPath := writeFile(t, dir, "issuer-ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.certificate.Raw}))
	return map[string]any{
		"max_proof_ttl": "30s",
		"issuers": map[string]any{
			"issuer-realm": map[string]any{
				"signing_public_key_file": verification,
				"next_hop":                "https://issuer.invalid:8443",
				"expected_core":           string(testOwner),
				"ca_file":                 caPath,
				"spki_sha256":             strings.Repeat("11", 32),
				"verifier":                string(testOwner),
			},
		},
	}
}

// readForeignServiceConfig parses a complete service configuration with a
// foreign section and applies one mutation to the section before returning it
// with the assembled network configuration.
func readForeignServiceConfig(t *testing.T, mutateSection func(*foreignConfig)) (*foreignConfig, realmgrpc.Config) {
	t.Helper()
	dir := t.TempDir()
	path := writeServiceConfig(t, dir, func(v map[string]any) {
		v["foreign"] = foreignSection(t, dir)
	})
	c, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if mutateSection != nil {
		mutateSection(c.Foreign)
	}
	return c.Foreign, c.network
}

func TestReadConfigParsesForeignSection(t *testing.T) {
	foreign, network := readForeignServiceConfig(t, nil)
	if foreign == nil || foreign.MaxTTL != "30s" || len(foreign.Issuers) != 1 {
		t.Fatalf("foreign section = %+v", foreign)
	}
	issuer := foreign.Issuers["issuer-realm"]
	if issuer.ExpectedCore != testOwner || issuer.Verifier != testOwner || issuer.SPKI != strings.Repeat("11", 32) {
		t.Fatalf("foreign issuer = %+v", issuer)
	}
	// Parsing alone assembles nothing: issuer trust exists only after open.
	if network.Foreign != nil || network.ForeignLiveness != nil {
		t.Fatal("parsing configured foreign issuer trust")
	}
}

// withIssuer replaces the configured issuer entry after one modification:
// map entries cannot be addressed in place.
func withIssuer(c *foreignConfig, change func(*foreignIssuer)) {
	issuer := c.Issuers["issuer-realm"]
	change(&issuer)
	c.Issuers["issuer-realm"] = issuer
}

func TestForeignOpenFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*foreignConfig)
	}{
		{"missing issuers", func(c *foreignConfig) { c.Issuers = nil }},
		{"zero proof ttl", func(c *foreignConfig) { c.MaxTTL = "0s" }},
		{"bad proof ttl", func(c *foreignConfig) { c.MaxTTL = "later" }},
		{"empty issuer realm", func(c *foreignConfig) {
			issuer := c.Issuers["issuer-realm"]
			delete(c.Issuers, "issuer-realm")
			c.Issuers[""] = issuer
		}},
		{"invalid verifier", func(c *foreignConfig) { withIssuer(c, func(i *foreignIssuer) { i.Verifier = "not-an-owner" }) }},
		{"invalid expected core", func(c *foreignConfig) { withIssuer(c, func(i *foreignIssuer) { i.ExpectedCore = "not-an-owner" }) }},
		{"missing signing key file", func(c *foreignConfig) { withIssuer(c, func(i *foreignIssuer) { i.SigningPublicKeyFile = "" }) }},
		{"missing ca file", func(c *foreignConfig) { withIssuer(c, func(i *foreignIssuer) { i.CAFile = "" }) }},
		{"bad spki hex", func(c *foreignConfig) { withIssuer(c, func(i *foreignIssuer) { i.SPKI = "zz" }) }},
		{"local next hop", func(c *foreignConfig) { withIssuer(c, func(i *foreignIssuer) { i.NextHop = "local:core" }) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			foreign, network := readForeignServiceConfig(t, tc.mutate)
			assembled, _, err := foreign.open(network)
			if err == nil {
				t.Fatal("invalid foreign section accepted")
			}
			if assembled.Foreign != nil || assembled.ForeignLiveness != nil {
				t.Fatal("failed open configured foreign trust")
			}
		})
	}
	t.Run("unparsable signing key", func(t *testing.T) {
		foreign, network := readForeignServiceConfig(t, nil)
		withIssuer(foreign, func(i *foreignIssuer) {
			i.SigningPublicKeyFile = writeFile(t, t.TempDir(), "bad.pub.pem", []byte("not a key"))
		})
		if _, _, err := foreign.open(network); err == nil {
			t.Fatal("invalid signing key accepted")
		}
	})
	t.Run("unparsable ca", func(t *testing.T) {
		foreign, network := readForeignServiceConfig(t, nil)
		withIssuer(foreign, func(i *foreignIssuer) {
			i.CAFile = writeFile(t, t.TempDir(), "bad-ca.pem", []byte("not a certificate"))
		})
		if _, _, err := foreign.open(network); err == nil {
			t.Fatal("invalid issuer CA accepted")
		}
	})
}

func TestForeignOpenAssemblesIssuerTrust(t *testing.T) {
	foreign, network := readForeignServiceConfig(t, nil)
	assembled, closeClients, err := foreign.open(network)
	if err != nil {
		t.Fatal(err)
	}
	defer closeClients()
	if assembled.Foreign == nil || len(assembled.ForeignLiveness) != 1 {
		t.Fatalf("assembled foreign trust = %v %v", assembled.Foreign, assembled.ForeignLiveness)
	}
	if _, ok := assembled.ForeignLiveness["issuer-realm"]; !ok {
		t.Fatal("issuer liveness missing after open")
	}
}

// An absent foreign section is not an error: the product starts with the
// foreign port disabled and reports it as unavailable.
func TestForeignOpenWithoutSectionStaysClosed(t *testing.T) {
	path := writeServiceConfig(t, t.TempDir(), nil)
	c, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Foreign != nil {
		t.Fatal("foreign section present without configuration")
	}
	network, closeForeign, err := c.Foreign.open(c.network)
	if err != nil {
		t.Fatal(err)
	}
	closeForeign()
	if network.Foreign != nil || network.ForeignLiveness != nil {
		t.Fatal("foreign trust assembled without a section")
	}
}
