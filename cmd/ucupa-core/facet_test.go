package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ucupa/shared/values"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

func facetConfigFixture(t *testing.T) (config, facetConnectionConfig) {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(dir, "service.json")
	b := fmt.Sprintf(`{"audience":"fixture","credential_id":%q,"secret":%q}`, values.EncodePublicBytes(make([]byte, 16)), values.EncodePublicBytes(make([]byte, 32)))
	if err := os.WriteFile(credentials, []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	c := config{Expected: "00000000-0000-4000-8000-000000000001", MaxResponseBytes: 4 << 20, MaxMetadataBytes: 8192,
		networkConfig: coregrpc.Config{RPCDeadline: time.Second, HandshakeTimeout: time.Second}}
	e := facetConnectionConfig{NextHop: "https://127.0.0.1:1", Expected: "00000000-0000-4000-8000-000000000002", CAFile: ca, SPKI: values.EncodePublicBytes(make([]byte, 32)), CredentialFile: credentials}
	// A nonzero pin is required independently of CA validation.
	pin := [32]byte{1}
	e.SPKI = values.EncodePublicBytes(pin[:])
	return c, e
}

func TestFacetSourceConfigurationBoundaries(t *testing.T) {
	c, e := facetConfigFixture(t)
	c.FacetConnections = []facetConnectionConfig{e}
	source, closeClients, err := c.openFacetSource()
	if err != nil || source == nil || closeClients == nil {
		t.Fatalf("valid explicit source: %v", err)
	}
	closeClients()
	for name, mutate := range map[string]func(*config){
		"missing":            func(c *config) { c.FacetConnections = nil },
		"empty":              func(c *config) { c.FacetConnections = []facetConnectionConfig{} },
		"capacity":           func(c *config) { c.FacetConnections = make([]facetConnectionConfig, 257) },
		"duplicate address":  func(c *config) { c.FacetConnections = append(c.FacetConnections, e) },
		"local address":      func(c *config) { c.FacetConnections[0].NextHop = "local:facet" },
		"invalid owner":      func(c *config) { c.FacetConnections[0].Expected = "other" },
		"missing CA":         func(c *config) { c.FacetConnections[0].CAFile += ".missing" },
		"wrong CA":           func(c *config) { c.FacetConnections[0].CAFile = e.CredentialFile },
		"short pin":          func(c *config) { c.FacetConnections[0].SPKI = "AQ" },
		"zero pin":           func(c *config) { c.FacetConnections[0].SPKI = values.EncodePublicBytes(make([]byte, 32)) },
		"missing credential": func(c *config) { c.FacetConnections[0].CredentialFile += ".missing" },
		"empty credential":   func(c *config) { c.FacetConnections[0].CredentialFile = "" },
	} {
		t.Run(name, func(t *testing.T) {
			next := c
			next.FacetConnections = append([]facetConnectionConfig(nil), c.FacetConnections...)
			mutate(&next)
			if source, cleanup, err := next.openFacetSource(); err == nil || source != nil || cleanup != nil {
				if cleanup != nil {
					cleanup()
				}
				t.Fatalf("bad configuration accepted: %v", err)
			}
		})
	}
}

func TestFacetCredentialFileClosedShapeAndBounds(t *testing.T) {
	_, e := facetConfigFixture(t)
	original, err := os.ReadFile(e.CredentialFile)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"unknown":   strings.TrimSuffix(string(original), "}") + `,"extra":true}`,
		"duplicate": strings.TrimSuffix(string(original), "}") + `,"audience":"other"}`,
		"null":      `{"audience":"fixture","credential_id":null,"secret":null}`,
		"array":     `[]`, "short": `{"audience":"fixture","credential_id":"AQ","secret":"AQ"}`,
		"oversized": strings.Repeat("sensitive", 600),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(e.CredentialFile, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			_, _, secret, err := facetCredentialFile(e.CredentialFile).Evidence()
			if err == nil || secret != ([32]byte{}) || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("credential rejection/redaction failed: %v", err)
			}
		})
	}
	// Subtest order is deliberately unspecified. Test byte limits against a
	// fixed file, not whichever malformed credential a map iteration left last.
	if err := os.WriteFile(e.CredentialFile, []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readFacetConnectionFile(e.CredentialFile, 5); err != nil || string(data) != "12345" {
		t.Fatal("exact file limit rejected", err)
	}
	if _, err := readFacetConnectionFile(e.CredentialFile, 4); err == nil {
		t.Fatal("file limit silently truncated")
	}
}
