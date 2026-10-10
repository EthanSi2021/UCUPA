package acceptance_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
)

// Reuses the public mounted query/read/observation/restart scenario. Both
// original requester establishment and target foreign authorization now reach
// separate pinned Realm products, rather than trusted-local Realm objects.
func TestNetworkFederationRealmProductsRestart(t *testing.T) {
	runFederationRealmProductsRestart(t, false, true)
}

func prepareFederationNetworkRealms(t *testing.T, a, b *networkFixture, extraA, extraB map[string]any, verification string, issuer map[string]any) func() {
	t.Helper()
	realmA, startA := prepareCombinationCoreRealm(t, a, "", "", nil, false)
	realmB, startB := prepareCombinationCoreRealm(t, b, "", "", nil, false)
	for key, value := range realmA {
		extraA[key] = value
	}
	for key, value := range realmB {
		extraB[key] = value
	}
	// The receiver's Core service identity is also the explicitly configured
	// verifier at the issuer. This test deployment shares that controlled TLS
	// certificate with its Realm liveness client; grants and Stores stay separate.
	// This is not discovery, an extra principal, or automatic issuer trust.
	endpoint := realmB["realm_endpoints"].([]any)[0].(map[string]any)
	endpoint["ca_file"] = issuerRootForRealm(t, b)
	endpoint["spki_sha256"] = values.EncodePublicBytes(b.clientConfig.SPKISHA256[:])
	path := filepath.Join(b.dir, "realm-host", "service.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var configuration map[string]any
	if err := json.Unmarshal(raw, &configuration); err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(b.dir, "realm-verifier.pem"), b.certPEM)
	writeNetworkFile(t, filepath.Join(b.dir, "realm-verifier.key"), b.keyPEM)
	configuration["certificate_file"] = filepath.Join(b.dir, "realm-verifier.pem")
	configuration["private_key_file"] = filepath.Join(b.dir, "realm-verifier.key")
	pin, err := values.DecodePublicBytes(issuer["spki_sha256"].(string))
	if err != nil {
		t.Fatal(err)
	}
	configuration["foreign"] = map[string]any{"max_proof_ttl": "30s", "issuers": map[string]any{string(a.actor.Realm): map[string]any{
		"signing_public_key_file": verification, "next_hop": issuer["next_hop"], "expected_core": issuer["expected_core"],
		"ca_file": issuer["ca_file"], "spki_sha256": hex.EncodeToString(pin), "verifier": string(b.config.Expected),
	}}}
	writeServiceConfig(t, filepath.Join(b.dir, "realm-host"), "ucupa-realm", configuration)
	return func() { startA(); startB() }
}

func issuerRootForRealm(t *testing.T, b *networkFixture) string {
	t.Helper()
	// networkCertificate returns leaf plus CA in certPEM, accepted as a pool.
	path := filepath.Join(b.dir, "realm-verifier-ca.pem")
	writeNetworkFile(t, path, b.certPEM)
	return path
}
