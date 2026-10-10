package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	realm "ucupa/ucupa-realm/contract"
)

func identityTrustFiles(t *testing.T, host, peer *networkFixture) map[string]any {
	t.Helper()
	ca := filepath.Join(host.dir, "peer-ca.pem")
	writeNetworkFile(t, ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peer.config.Certificate.Certificate[1]}))
	pin := peer.clientConfig.SPKISHA256
	return map[string]any{"client_ca_file": ca, "client_spki_sha256": map[string]string{string(peer.config.Expected): values.EncodePublicBytes(pin[:])}, "max_ttl": "30s", "max_grants": 8, "max_per_caller": 6}
}

func TestNetworkExternalIdentityIndependentProcesses(t *testing.T) {
	issuer := newNetworkFixtureRealm(t, false, "identity-home")
	target, entry := newNetworkFixture(t, false), newNetworkFixture(t, false)
	binding := realm.PermissionBinding{Subject: issuer.actor, Permission: corecontract.PermissionReadElement, Scope: values.AnchorClosureScope(target.config.Expected, networkRoot)}
	if _, err := target.r.PutBinding(target.actor, binding, target.r.BindingSetRevision(), "external-user"); err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	signing := filepath.Join(issuer.dir, "identity.pem")
	verification := filepath.Join(target.dir, "issuer.pub.pem")
	issuerCA := filepath.Join(target.dir, "issuer-ca.pem")
	writeNetworkFile(t, signing, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}))
	writeNetworkFile(t, verification, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}))
	writeNetworkFile(t, issuerCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuer.config.Certificate.Certificate[1]}))
	startIssuer := prepareNetworkProduct(t, issuer, map[string]any{"read_delegation": identityTrustFiles(t, issuer, target), "identity_issuer": map[string]any{"signing_key_file": signing, "max_ttl": "30s", "max_proofs": 8, "max_per_caller": 6}})
	pin := issuer.clientConfig.SPKISHA256
	startTarget := prepareNetworkProduct(t, target, map[string]any{"read_delegation": identityTrustFiles(t, target, entry), "external_identity": map[string]any{"max_ttl": "30s", "issuers": map[string]any{string(issuer.actor.Realm): map[string]any{"signing_public_key_file": verification, "next_hop": issuer.config.NextHop, "expected_core": issuer.config.Expected, "ca_file": issuerCA, "spki_sha256": values.EncodePublicBytes(pin[:])}}}})
	stopIssuer, stopTarget := startIssuer(), startTarget()
	issuer.connect(t)
	actor := identityServiceClient(t, target, entry)
	ctx := context.Background()
	exchange := func() *corepb.ReadDelegation {
		t.Helper()
		challenge, err := actor.Delegations().BeginIdentityChallenge(ctx, &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: proto.String(string(entry.config.Expected)), Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(25)})
		if err != nil {
			t.Fatal(err)
		}
		p, err := issuer.client.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{Actor: proto.String(string(entry.config.Expected)), Challenge: challenge.Challenge, TtlSeconds: proto.Uint64(20)})
		if err != nil {
			t.Fatal(err)
		}
		out, err := actor.Delegations().ExchangeIdentity(ctx, &corepb.ExchangeIdentityRequest{Actor: proto.String(string(entry.config.Expected)), Challenge: challenge.Challenge, Proof: p})
		if err != nil || out.GetResult() == nil {
			t.Fatal("product exchange", out, err)
		}
		return out.GetResult()
	}
	grant := exchange()
	client := delegatedClient(t, target, entry, grant)
	request := &corepb.ReadElementRequest{Handle: grant.Handle, Address: &networkpb.Address{}}
	if out, err := client.Core().ReadElement(ctx, request); err != nil || out.GetResult().GetRecord() == nil {
		t.Fatal("product identity read", out, err)
	}
	stopIssuer()
	if _, err := client.Core().ReadElement(ctx, request); status.Code(err) != codes.Unavailable {
		t.Fatal("offline issuer accepted", err)
	}
	_ = startIssuer()
	// Recovery of the target's fixed issuer connection is asynchronous. While
	// disconnected it must fail unavailable; once connected, old proof is invalid.
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := client.Core().ReadElement(ctx, request)
		if status.Code(err) == codes.Unauthenticated {
			break
		}
		if status.Code(err) != codes.Unavailable || time.Now().After(deadline) {
			t.Fatal("proof revived across issuer restart", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = issuer.client.Close()
	issuer.connect(t)
	freshGrant := exchange()
	fresh := delegatedClient(t, target, entry, freshGrant)
	freshRequest := &corepb.ReadElementRequest{Handle: freshGrant.Handle, Address: &networkpb.Address{}}
	if out, err := fresh.Core().ReadElement(ctx, freshRequest); err != nil || out.GetResult().GetRecord() == nil {
		t.Fatal("new proof after restart", out, err)
	}
	stopTarget()
	_ = startTarget()
	if _, err := fresh.Core().ReadElement(ctx, freshRequest); status.Code(err) != codes.Unauthenticated {
		t.Fatal("target revived grant", err)
	}
}
