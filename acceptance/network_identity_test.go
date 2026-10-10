package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func TestNetworkIdentityProofIssueCheckAndRevocation(t *testing.T) {
	f, actor := newNetworkFixture(t, false), newNetworkFixture(t, false)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := realminstance.NewIdentityIssuer(f.r, realm.IdentityIssuerConfig{Key: key, MaxTTL: time.Minute, MaxProofs: 4, MaxPerCaller: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	f.config.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: actor.clientConfig.Roots, Peers: map[values.FactAuthorityRef][32]byte{actor.c.AuthorityRef(): actor.clientConfig.SPKISHA256}, MaxTTL: 10 * time.Second, MaxGrants: 4, MaxPerCaller: 3}
	f.config.IdentityProofs = composition.RealmIdentityProofs{Issuer: issuer}
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
	f.connect(t)
	ctx := context.Background()
	challenge := [32]byte{17}
	proof, err := f.client.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{Actor: proto.String(string(actor.c.AuthorityRef())), Challenge: challenge[:], TtlSeconds: proto.Uint64(10)})
	if err != nil {
		t.Fatal(err)
	}
	v, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{f.r.ID(): pub}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(realm.IdentityProof{Payload: proof.Payload, Signature: proof.Signature}, actor.c.AuthorityRef(), challenge, time.Now()); err != nil {
		t.Fatal(err)
	}
	check := &networkpb.CheckIdentityProofRequest{Verifier: proto.String(string(actor.c.AuthorityRef())), Proof: proof}
	if _, err := f.client.IdentityProofs().CheckIdentityProof(ctx, check); status.Code(err) != codes.Unauthenticated {
		t.Fatal("user cannot introspect as service", err)
	}
	cfg := f.clientConfig
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &actor.config.Certificate
	service, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if result, err := service.IdentityProofs().CheckIdentityProof(ctx, check); err != nil || !result.GetValid() {
		t.Fatal("service check", result, err)
	}
	wrong := proto.Clone(check).(*networkpb.CheckIdentityProofRequest)
	wrong.Verifier = proto.String(string(f.c.AuthorityRef()))
	if _, err := service.IdentityProofs().CheckIdentityProof(ctx, wrong); status.Code(err) != codes.Unauthenticated {
		t.Fatal("wrong verifier", err)
	}
	forged := proto.Clone(check).(*networkpb.CheckIdentityProofRequest)
	forged.Proof.Signature[0] ^= 1
	if _, err := service.IdentityProofs().CheckIdentityProof(ctx, forged); status.Code(err) != codes.Unauthenticated {
		t.Fatal("forged proof", err)
	}
	grant := issueReadDelegation(t, f, actor, false, 5, corepb.Intent_READ)
	delegate := delegatedClient(t, f, actor, grant)
	if _, err := delegate.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{Actor: grant.Actor, Challenge: challenge[:], TtlSeconds: proto.Uint64(1)}); status.Code(err) != codes.Unauthenticated {
		t.Fatal("delegation became issuer", err)
	}
	if _, err := f.r.RevokeCredential(f.admin, realm.CredentialID(f.id), f.r.CredentialSetRevision(), "revoke-identity-proof"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.IdentityProofs().CheckIdentityProof(ctx, check); status.Code(err) != codes.Unauthenticated {
		t.Fatal("proof survived source revoke", err)
	}
	issuer.Close()
	if _, err := service.IdentityProofs().CheckIdentityProof(ctx, check); status.Code(err) != codes.Unavailable {
		t.Fatal("closed issuer treated as false identity", err)
	}
}
