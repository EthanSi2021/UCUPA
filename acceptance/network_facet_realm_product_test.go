package acceptance_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// The same public relationship/content/restart chain runs against a separate
// Realm product. The fixture does not replace an RPC with an internal call.
func TestFacetRelationshipNetworkRealmProducts(t *testing.T) {
	runFacetRelationshipProducts(t, true)
}

func TestFacetLifecycleNetworkRealmProducts(t *testing.T) {
	runFacetLifecycleProducts(t, true)
}

func TestFacetReplacementNetworkRealmProducts(t *testing.T) {
	runFacetReplacementProducts(t, true)
}

func prepareFacetNetworkRealm(t *testing.T, dir string, policy *realminstance.Realm, owner *facet.Instance, actor values.PrincipalRef) (map[string]any, func() *serviceProductProcess) {
	t.Helper()
	endpoint, configuration, host := prepareOwnerRealmProduct(t, dir, filepath.Join(dir, "realm.db"), actor.Realm, owner.AuthorityRef())
	hop := endpoint["next_hop"].(values.NextHop)
	if _, err := policy.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: facetcontract.PermissionRealmContactUpdate, Scope: values.AuthorityScope(owner.AuthorityRef())}, policy.BindingSetRevision(), "network-facet-realm-maintenance"); err != nil {
		t.Fatal(err)
	}
	if _, err := policy.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: facetcontract.PermissionRealmContactRead, Scope: values.AuthorityScope(owner.AuthorityRef())}, policy.BindingSetRevision(), "network-facet-realm-inspect"); err != nil {
		t.Fatal(err)
	}
	caller, err := policy.TrustLocalCaller(actor)
	if err != nil {
		t.Fatal(err)
	}
	contact, err := owner.Namespace.RealmContact()
	if err != nil {
		t.Fatal(err)
	}
	moved, err := owner.Namespace.UpdateRealmContact(context.Background(), caller, facetcontract.RealmContactUpdate{OperationID: "network-facet-realm-contact", Owner: contact.Owner, Realm: contact.Realm, Expected: contact.Revision, NextHop: hop})
	if err != nil || moved.Decision != facetcontract.RealmContactCommitted {
		t.Fatalf("persist network Realm contact: %+v %v", moved, err)
	}
	return endpoint, prepareServiceProductControl(t, host, "ucupa-realm", configuration)
}

// Only test deployment material is shared; each owner still stores its own
// contact and each product consumes its own explicitly admitted service key.
func prepareOwnerRealmProduct(t *testing.T, dir, store string, id values.RealmID, owner values.FactAuthorityRef) (map[string]any, map[string]any, string) {
	t.Helper()
	ca, caKey := realmForeignCA(t)
	_, pin, cert, key := realmForeignLeaf(t, ca, caKey, "facet-realm")
	serviceCA, serviceCAKey := realmForeignCA(t)
	_, servicePin, serviceCert, serviceKey := realmForeignLeaf(t, serviceCA, serviceCAKey, "facet-service")
	for name, data := range map[string][]byte{
		"realm-ca.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}),
		"realm.pem":    []byte(cert), "realm.key": []byte(key),
		"realm-service-ca.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serviceCA.Raw}),
		"realm-service.pem":    []byte(serviceCert), "realm-service.key": []byte(serviceKey),
	} {
		writeNetworkFile(t, filepath.Join(dir, name), data)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listen := l.Addr().String()
	_ = l.Close()
	hop := values.NextHop("https://" + listen)
	host := filepath.Join(dir, "realm-host")
	if err := os.MkdirAll(host, 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := map[string]any{
		"realm_store": store, "realm_id": id,
		"listen": listen, "next_hop": hop,
		"certificate_file": filepath.Join(dir, "realm.pem"), "private_key_file": filepath.Join(dir, "realm.key"),
		"client_roots_file": filepath.Join(dir, "realm-service-ca.pem"),
		"admitted_services": []any{map[string]any{"spki_sha256": hex.EncodeToString(servicePin[:]), "owner": owner}},
		"handle_ttl":        "10m", "challenge_ttl": "10m", "rpc_deadline": "4s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
		"max_handles": 128, "max_handles_per_caller": 64, "max_challenges": 64, "max_challenges_per_owner": 16,
		"max_requirements": 64, "max_connections": 16, "max_concurrent_rpcs": 8,
		"max_response_bytes": 4 << 20, "max_streams": 64, "max_metadata_bytes": 8192,
	}
	endpoint := map[string]any{"next_hop": hop, "ca_file": filepath.Join(dir, "realm-ca.pem"), "spki_sha256": values.EncodePublicBytes(pin[:]), "service_certificate_file": filepath.Join(dir, "realm-service.pem"), "service_private_key_file": filepath.Join(dir, "realm-service.key")}
	return endpoint, configuration, host
}

// Exclusive original-Store maintenance uses the same pinned Realm product and
// authenticated caller as normal serving. An offline current contact can only
// be repaired through an explicitly supplied same-Realm candidate.
func verifyFacetNetworkMaintenance(t *testing.T, dir string, endpoint map[string]any, credential realm.CredentialEvidence) {
	t.Helper()
	read := func(field string) []byte {
		b, err := os.ReadFile(endpoint[field].(string))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(read("ca_file")) {
		t.Fatal("invalid test roots")
	}
	service, err := tls.X509KeyPair(read("service_certificate_file"), read("service_private_key_file"))
	if err != nil {
		t.Fatal(err)
	}
	pin, err := values.DecodePublicBytes(endpoint["spki_sha256"].(string))
	if err != nil {
		t.Fatal(err)
	}
	hop := endpoint["next_hop"].(values.NextHop)
	connections := &composition.NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint(hop, credential.Audience, composition.NetworkRealmEndpointConfig{Roots: roots, SPKISHA256: [32]byte(pin), Service: &service, RPCDeadline: 4 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 4 << 20, MaxMetadataBytes: 8192}); err != nil {
		t.Fatal(err)
	}
	defer connections.UnbindNetworkEndpoint(hop)
	ctx := context.Background()
	caller, err := connections.EstablishCaller(ctx, hop, credential.Audience, credential)
	if err != nil {
		t.Fatal(err)
	}
	defer connections.ReleaseRealmEvidence(ctx, hop, credential.Audience, caller.Establishment)
	m, err := facet.OpenRealmMaintenance(connections, filepath.Join(dir, "facet.db"), facet.Config{ExpectedRealm: credential.Audience})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	contact, err := m.InspectRealmContact(ctx, caller, "")
	if err != nil {
		t.Fatal(err)
	}
	move, err := m.UpdateRealmContact(ctx, caller, facetcontract.RealmContactUpdate{OperationID: "offline-network-contact", Owner: contact.Owner, Realm: contact.Realm, Expected: contact.Revision, NextHop: "https://127.0.0.1:1"})
	if err != nil || move.Decision != facetcontract.RealmContactCommitted {
		t.Fatalf("authorized move to offline address: %+v %v", move, err)
	}
	if _, err := m.RecoverRealmContact(ctx, caller, "offline-network-contact", ""); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("offline contact disclosed original result without candidate: %v", err)
	}
	recovered, err := m.RecoverRealmContact(ctx, caller, "offline-network-contact", hop)
	if err != nil || recovered != move {
		t.Fatalf("same-Realm candidate changed original result: %+v %v", recovered, err)
	}
	contact, err = m.InspectRealmContact(ctx, caller, hop)
	if err != nil || contact != move.Contact {
		t.Fatalf("candidate inspection: %+v %v", contact, err)
	}
	repaired, err := m.UpdateRealmContact(ctx, caller, facetcontract.RealmContactUpdate{OperationID: "repair-network-contact", Owner: contact.Owner, Realm: contact.Realm, Expected: contact.Revision, NextHop: hop})
	if err != nil || repaired.Decision != facetcontract.RealmContactCommitted || repaired.Contact.NextHop != hop {
		t.Fatalf("same-Realm repair: %+v %v", repaired, err)
	}
}
