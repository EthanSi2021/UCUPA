package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
	realmsdk "ucupa/ucupa-realm/sdk/go"
)

const realmForeignUser values.FactAuthorityRef = "00000000-0000-4000-8000-0000000000d1"
const realmForeignOtherOwner values.FactAuthorityRef = "00000000-0000-4000-8000-0000000000d4"
const realmForeignVerifier values.FactAuthorityRef = "00000000-0000-4000-8000-0000000000d2"
const realmForeignAuthority values.FactAuthorityRef = "00000000-0000-4000-8000-0000000000d3"
const realmForeignElement values.ElementID = "00000000-0000-4000-8000-0000000000e1"

// realmForeignCA issues the receiver-side certificate material: the product's
// server certificate doubles as its pinned client identity at the issuer, and
// the user client presents its own leaf under the same root.
func realmForeignCA(t *testing.T) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "UCUPA realm foreign test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return parsed, key
}

func realmForeignLeaf(t *testing.T, ca *x509.Certificate, caKey ed25519.PrivateKey, name string) (tls.Certificate, [32]byte, string, string) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: name}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, pub, caKey)
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
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert, sha256.Sum256(parsed.RawSubjectPublicKeyInfo), string(certPEM), string(keyPEM)
}

// TestRealmProductForeignIdentityAcrossProcesses proves the independent Realm
// product closes the foreign identity chain against a real issuer Core
// product: the pinned target gate precedes credential-bearing calls, one
// issuer proof bound to the admitted caller's owner establishes exactly the
// issuer's subject, the online liveness check runs over the product's own
// pinned issuer connection, and the resulting caller receives only the
// seeded authorization. An offline issuer, a restarted issuer that no longer
// knows the proof, and a proof naming a different owner all stay rejected.
func TestRealmProductForeignIdentityAcrossProcesses(t *testing.T) {
	issuer := newNetworkFixtureRealm(t, false, "identity-home")
	dir := t.TempDir()
	ca, caKey := realmForeignCA(t)
	_, serverSPKI, serverCertPEM, serverKeyPEM := realmForeignLeaf(t, ca, caKey, "realm-a")
	userCert, userSPKI, _, _ := realmForeignLeaf(t, ca, caKey, "realm-user")

	// The receiver Realm seeds exactly one binding for the issuer's subject;
	// nothing else is granted to the foreign principal.
	storePath := filepath.Join(dir, "receiver-realm.db")
	subject := values.PrincipalRef{Realm: "identity-home", Principal: "operator"}
	seeded, err := realminstance.CreateDurable(realm.Seed{ID: "realm-a", Bindings: []realm.PermissionBinding{
		{Subject: subject, Permission: "ucupa.test.read", Scope: values.CoreElementScope(realmForeignAuthority, realmForeignElement)},
	}}, storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := seeded.Close(); err != nil {
		t.Fatal(err)
	}

	// The issuer Core signs proofs and admits only the receiver's pinned
	// service identity for the online check.
	signingPub, signingKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signingKeyDER, err := x509.MarshalPKCS8PrivateKey(signingKey)
	if err != nil {
		t.Fatal(err)
	}
	signingPubDER, err := x509.MarshalPKIXPublicKey(signingPub)
	if err != nil {
		t.Fatal(err)
	}
	writeNetworkFile(t, filepath.Join(issuer.dir, "identity-signing.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: signingKeyDER}))
	signingPubPath := filepath.Join(dir, "issuer-signing.pub")
	writeNetworkFile(t, signingPubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: signingPubDER}))
	caPath := filepath.Join(dir, "receiver-ca.pem")
	writeNetworkFile(t, caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}))
	issuerCAPath := filepath.Join(dir, "issuer-ca.pem")
	writeNetworkFile(t, issuerCAPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuer.config.Certificate.Certificate[1]}))

	startIssuer := prepareNetworkProduct(t, issuer, map[string]any{
		"read_delegation": map[string]any{
			"client_ca_file":     caPath,
			"client_spki_sha256": map[string]string{string(realmForeignVerifier): values.EncodePublicBytes(serverSPKI[:])},
			"max_ttl":            "30s", "max_grants": 4, "max_per_caller": 3,
		},
		"identity_issuer": map[string]any{
			"signing_key_file": filepath.Join(issuer.dir, "identity-signing.key"),
			"max_ttl":          "30s", "max_proofs": 8, "max_per_caller": 6,
		},
	})

	// The receiver product binds the issuer trust explicitly: one signing key,
	// one pinned Core target and one verifier identity. Nothing is learned
	// from an incoming proof.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	receiverAddr := probe.Addr().String()
	_ = probe.Close()
	writeNetworkFile(t, filepath.Join(dir, "receiver.pem"), []byte(serverCertPEM))
	writeNetworkFile(t, filepath.Join(dir, "receiver.key"), []byte(serverKeyPEM))
	receiverConfig := map[string]any{
		"realm_store": storePath, "realm_id": "realm-a", "listen": receiverAddr,
		"next_hop":         "https://realm-a.invalid:8443",
		"certificate_file": filepath.Join(dir, "receiver.pem"), "private_key_file": filepath.Join(dir, "receiver.key"),
		"client_roots_file": caPath,
		"admitted_services": []any{map[string]any{"spki_sha256": hex.EncodeToString(userSPKI[:]), "owner": string(realmForeignUser)}},
		"handle_ttl":        "600s", "challenge_ttl": "600s", "rpc_deadline": "10s", "handshake_timeout": "5s",
		"connection_lifetime": "60s", "shutdown_timeout": "10s",
		"max_handles": 64, "max_handles_per_caller": 8, "max_challenges": 16, "max_challenges_per_owner": 8,
		"max_requirements": 16, "max_connections": 16, "max_concurrent_rpcs": 8,
		"max_response_bytes": 1048576, "max_streams": 64, "max_metadata_bytes": 8192,
		"foreign": map[string]any{
			"max_proof_ttl": "30s",
			"issuers": map[string]any{"identity-home": map[string]any{
				"signing_public_key_file": signingPubPath,
				"next_hop":                string(issuer.config.NextHop),
				"expected_core":           string(issuer.config.Expected),
				"ca_file":                 issuerCAPath,
				"spki_sha256":             hex.EncodeToString(issuer.clientConfig.SPKISHA256[:]),
				"verifier":                string(realmForeignVerifier),
			}},
		},
	}
	startReceiver := prepareServiceProduct(t, dir, "ucupa-realm", receiverConfig)

	stopIssuer := startIssuer()
	stopReceiver := startReceiver()
	issuer.connect(t)
	defer stopReceiver()
	defer stopIssuer()

	roots := x509.NewCertPool()
	roots.AddCert(ca)
	client, err := realmsdk.New(realmsdk.Config{
		NextHop:          values.NextHop("https://" + receiverAddr),
		Expected:         "realm-a",
		Roots:            roots,
		SPKISHA256:       serverSPKI,
		Certificate:      &userCert,
		RPCDeadline:      10 * time.Second,
		ConnectTimeout:   10 * time.Second,
		MaxResponseBytes: 1 << 20,
		MaxMetadataBytes: 8192,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	if realmID, err := client.CheckTarget(ctx); err != nil || realmID != "realm-a" {
		t.Fatalf("target check = %v %v", realmID, err)
	}
	requirement := &realmpb.Requirement{Permission: proto.String("ucupa.test.read"), Scope: &realmpb.Scope{
		Kind: realmpb.ScopeKind_CORE_ELEMENT.Enum(), Authority: proto.String(string(realmForeignAuthority)), Element: proto.String(string(realmForeignElement)),
	}}
	establish := func(actor values.FactAuthorityRef) (*realmpb.EstablishForeignCallerResponse, error) {
		t.Helper()
		challenge, err := client.IdentityService().BeginChallenge(ctx, &realmpb.BeginChallengeRequest{})
		if err != nil {
			t.Fatal(err)
		}
		proof, err := issuer.client.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{
			Actor: proto.String(string(actor)), Challenge: challenge.GetChallenge(), TtlSeconds: proto.Uint64(25),
		})
		if err != nil {
			t.Fatal(err)
		}
		return client.IdentityService().EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
			Actor: proto.String(string(realmForeignUser)), Challenge: challenge.GetChallenge(), Payload: proof.GetPayload(), Signature: proof.GetSignature(),
		})
	}

	// A correctly bound proof establishes exactly the issuer's subject and the
	// caller receives the seeded authorization for it.
	established, err := establish(realmForeignUser)
	if err != nil {
		t.Fatal(err)
	}
	if established.GetSubjectRealm() != "identity-home" || established.GetSubjectPrincipal() != "operator" {
		t.Fatalf("foreign subject = %v/%v", established.GetSubjectRealm(), established.GetSubjectPrincipal())
	}
	authorized, err := client.AuthorizationService().AuthorizeView(ctx, &realmpb.AuthorizeViewRequest{
		Caller: established.GetCaller(), Requirements: []*realmpb.Requirement{requirement},
	})
	if err != nil || authorized.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("foreign authorization = %v %v", authorized.GetDecision(), err)
	}

	// A proof naming a different owner is rejected before any identity exists.
	if _, err := establish(realmForeignOtherOwner); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("misbound proof accepted: %v", err)
	}

	// While the issuer is down the port is unavailable, never an offline pass,
	// and the unconsumed challenge survives for the retry after recovery.
	challenge, err := client.IdentityService().BeginChallenge(ctx, &realmpb.BeginChallengeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := issuer.client.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{
		Actor: proto.String(string(realmForeignUser)), Challenge: challenge.GetChallenge(), TtlSeconds: proto.Uint64(25),
	})
	if err != nil {
		t.Fatal(err)
	}
	present := func() error {
		_, err := client.IdentityService().EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
			Actor: proto.String(string(realmForeignUser)), Challenge: challenge.GetChallenge(), Payload: proof.GetPayload(), Signature: proof.GetSignature(),
		})
		return err
	}
	stopIssuer()
	if err := present(); status.Code(err) != codes.Unavailable {
		t.Fatalf("offline issuer accepted: %v", err)
	}
	// The receiver's issuer connection recovers asynchronously: while it is
	// still down the port fails unavailable; once it returns, the restarted
	// issuer no longer knows the old proof and rejects it.
	_ = startIssuer()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := present()
		if status.Code(err) == codes.Unauthenticated {
			break
		}
		if status.Code(err) != codes.Unavailable || time.Now().After(deadline) {
			t.Fatalf("proof revived across issuer restart: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A fresh proof from the restarted issuer establishes the subject again.
	_ = issuer.client.Close()
	issuer.connect(t)
	fresh, err := establish(realmForeignUser)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetSubjectRealm() != "identity-home" || fresh.GetSubjectPrincipal() != "operator" {
		t.Fatalf("fresh foreign subject = %v/%v", fresh.GetSubjectRealm(), fresh.GetSubjectPrincipal())
	}
	authorized, err = client.AuthorizationService().AuthorizeView(ctx, &realmpb.AuthorizeViewRequest{
		Caller: fresh.GetCaller(), Requirements: []*realmpb.Requirement{requirement},
	})
	if err != nil || authorized.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("fresh foreign authorization = %v %v", authorized.GetDecision(), err)
	}
}
