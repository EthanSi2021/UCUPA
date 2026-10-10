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
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func startIdentityServer(t *testing.T, f *networkFixture) {
	t.Helper()
	var err error
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.config.Federation = f.c.Federation()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
}

func identityServiceClient(t *testing.T, target, actor *networkFixture) *coresdk.Client {
	t.Helper()
	cfg := target.clientConfig
	cfg.Actor, cfg.ActorCertificate = actor.c.AuthorityRef(), &actor.config.Certificate
	c, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func identityPeerTrust(actor *networkFixture) *coregrpc.ReadDelegationConfig {
	return &coregrpc.ReadDelegationConfig{ClientRoots: actor.clientConfig.Roots, Peers: map[values.FactAuthorityRef][32]byte{actor.c.AuthorityRef(): actor.clientConfig.SPKISHA256}, MaxTTL: time.Minute, MaxGrants: 8, MaxPerCaller: 6}
}

func TestNetworkExternalIdentityChallengeAndIndependentAuthorization(t *testing.T) {
	home := values.RealmID("identity-home")
	issuerHost, target, entry := newNetworkFixtureRealm(t, false, home), newNetworkFixture(t, false), newNetworkFixture(t, false)
	user := values.PrincipalRef{Realm: home, Principal: "alice"}
	evidence := realm.CredentialEvidence{Audience: home, ID: realm.CredentialID(issuerHost.id), Secret: realm.CredentialSecret(issuerHost.secret)}
	admin := issuerHost.admin
	admin.Audience = home
	r, err := realminstance.New(realm.Seed{ID: home, LocalPrincipals: []values.PrincipalID{"alice"}, Credentials: []realm.CredentialBootstrap{{Subject: "alice", Evidence: evidence}, {Subject: "alice", Evidence: admin}}, Bindings: []realm.PermissionBinding{{Subject: user, Permission: realm.PermissionRevokeCredential, Scope: values.RealmScope(home)}}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	issuer, err := realminstance.NewIdentityIssuer(r, realm.IdentityIssuerConfig{Key: key, MaxTTL: time.Minute, MaxProofs: 8, MaxPerCaller: 6})
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	issuerHost.config.ReadDelegation = identityPeerTrust(target)
	issuerHost.config.IdentityProofs = composition.RealmIdentityProofs{Issuer: issuer}
	// The issuer host's contact address is rebound to the custom home realm,
	// which carries the same realm ID, so authentication and the identity
	// issuer resolve through it.
	if err := issuerHost.realmLinks.BindEndpoint(issuerHost.realmAddress, r); err != nil {
		t.Fatal(err)
	}
	startIdentityServer(t, issuerHost)
	issuerHost.actor = user
	issuerHost.connect(t)
	checker := composition.NetworkIdentityCheck{Client: identityServiceClient(t, issuerHost, target), Verifier: target.c.AuthorityRef()}
	v, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{home: pub}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	external, err := composition.NewRealmExternalIdentity(target.realmLinks, target.c.Access(), v, map[values.RealmID]composition.IdentityLiveness{home: checker})
	if err != nil {
		t.Fatal(err)
	}
	target.config.ReadDelegation = identityPeerTrust(entry)
	target.config.ExternalIdentity = external
	startIdentityServer(t, target)
	actorClient := identityServiceClient(t, target, entry)
	ctx := context.Background()
	exchange := func() (*corepb.CreateReadDelegationResponse, *corepb.ExchangeIdentityRequest) {
		t.Helper()
		c, err := actorClient.Delegations().BeginIdentityChallenge(ctx, &corepb.CreateReadDelegationRequest{Reference: target.reference(), Actor: proto.String(string(entry.c.AuthorityRef())), Intents: []corepb.Intent{corepb.Intent_READ}, Observe: proto.Bool(false), TtlSeconds: proto.Uint64(30)})
		if err != nil {
			t.Fatal(err)
		}
		// Only random bytes go to the user's issuer. There is no target URL,
		// target principal, internal Core cut or target user secret in this call.
		p, err := issuerHost.client.IdentityProofs().IssueIdentityProof(ctx, &networkpb.IssueIdentityProofRequest{Actor: proto.String(string(entry.c.AuthorityRef())), Challenge: c.Challenge, TtlSeconds: proto.Uint64(20)})
		if err != nil {
			t.Fatal(err)
		}
		request := &corepb.ExchangeIdentityRequest{Actor: proto.String(string(entry.c.AuthorityRef())), Challenge: c.Challenge, Proof: p}
		out, err := actorClient.Delegations().ExchangeIdentity(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return out, request
	}
	denied, used := exchange()
	if denied.GetFailure() != networkpb.Failure_DENIED {
		t.Fatal("trust became permission", denied)
	}
	if _, err := actorClient.Delegations().ExchangeIdentity(ctx, used); status.Code(err) != codes.Unauthenticated {
		t.Fatal("challenge replay", err)
	}
	binding := realm.PermissionBinding{Subject: user, Permission: corecontract.PermissionReadElement, Scope: values.AnchorClosureScope(target.c.AuthorityRef(), networkRoot)}
	if _, err := target.r.PutBinding(target.actor, binding, target.r.BindingSetRevision(), "grant-external-reader"); err != nil {
		t.Fatal(err)
	}
	accepted, _ := exchange()
	if accepted.GetResult() == nil {
		t.Fatal("authorized exchange", accepted)
	}
	reader := delegatedClient(t, target, entry, accepted.GetResult())
	read := &corepb.ReadElementRequest{Handle: accepted.GetResult().Handle, Address: &networkpb.Address{}}
	if out, err := reader.Core().ReadElement(ctx, read); err != nil || out.GetResult().GetRecord() == nil {
		t.Fatal("external read", out, err)
	}
	if _, err := r.RevokeCredential(admin, evidence.ID, r.CredentialSetRevision(), "revoke-forwarded-identity"); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Core().ReadElement(ctx, read); status.Code(err) != codes.Unauthenticated {
		t.Fatal("remote revoke ignored", err)
	}
}
