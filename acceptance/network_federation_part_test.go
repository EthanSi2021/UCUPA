package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	federationconn "ucupa/ucupa-core/transport/grpc/federationconn"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

type federationNetworkFixture struct {
	entry, target *networkFixture
	connections   *federationconn.FederationConnections
	bound         federationcontract.FederationConnections
	caller        values.CallerContext
	request       federationcontract.FederationReadRequest
}

func newFederationNetworkFixture(t *testing.T, retain bool) federationNetworkFixture {
	t.Helper()
	e := newNetworkFixtureRealm(t, false, "federation-entry")
	d := newNetworkFixtureRealm(t, false, "federation-target")
	if retain {
		if err := d.c.Close(); err != nil {
			t.Fatal(err)
		}
		ttl := time.Minute
		var err error
		d.c, err = core.OpenDurable(d.realmLinks, filepath.Join(d.dir, "core.db"), corecontract.Config{SnapshotRetention: &ttl, PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
		if err != nil {
			t.Fatal(err)
		}
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := realminstance.NewIdentityIssuer(e.r, realm.IdentityIssuerConfig{Key: key, MaxTTL: time.Minute, MaxProofs: 32, MaxPerCaller: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(issuer.Close)
	e.config.IdentityProofs = composition.RealmIdentityProofs{Issuer: issuer}
	e.config.ReadDelegation = identityPeerTrust(d)
	startIdentityServer(t, e)
	checker := composition.NetworkIdentityCheck{Client: identityServiceClient(t, e, d), Verifier: d.c.AuthorityRef()}
	v, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{e.r.ID(): pub}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	external, err := composition.NewRealmExternalIdentity(d.realmLinks, d.c.Access(), v, map[values.RealmID]composition.IdentityLiveness{e.r.ID(): checker})
	if err != nil {
		t.Fatal(err)
	}
	d.config.ExternalIdentity, d.config.ReadDelegation = external, identityPeerTrust(e)
	startIdentityServer(t, d)
	d.connect(t)
	create, err := d.client.Core().CreateEntity(context.Background(), &pb.CreateEntityRequest{Handle: d.enter(t, pb.Intent_CREATE_ENTITY), Element: proto.String(networkEntity), Final: networkTuple(1), OperationId: []byte("federation-fixture-entity")})
	if err != nil || create.GetResult() == nil {
		t.Fatalf("fixture create: %v %v", create, err)
	}
	create, err = d.client.Core().CreateEntity(context.Background(), &pb.CreateEntityRequest{Handle: d.enter(t, pb.Intent_CREATE_ENTITY), Element: proto.String("00000000-0000-4000-8000-000000000003"), Final: networkTuple(1), OperationId: []byte("federation-fixture-second")})
	if err != nil || create.GetResult() == nil {
		t.Fatalf("fixture second create: %v %v", create, err)
	}
	cfg := d.clientConfig
	cfg.Actor, cfg.ActorCertificate = e.c.AuthorityRef(), &e.config.Certificate
	connections, err := federationconn.NewFederationConnections(federationconn.FederationConnectionConfig{Source: e.c.AuthorityRef(), Peers: []coresdk.Config{cfg}, MaxParts: 1, TTL: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(connections.Close)
	caller, err := e.r.EstablishCaller(realm.CredentialEvidence{Audience: e.actor.Realm, ID: realm.CredentialID(e.id), Secret: realm.CredentialSecret(e.secret)})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := connections.WithCaller(caller, e.config.IdentityProofs, time.Now().Add(25*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return federationNetworkFixture{entry: e, target: d, connections: connections, bound: bound, caller: caller, request: federationcontract.FederationReadRequest{Boundary: networkRoot, Intent: "read", Purpose: [32]byte{1}}}
}

func (f federationNetworkFixture) grant(t *testing.T, p values.PermissionID, remove bool, id string) {
	t.Helper()
	b := realm.PermissionBinding{Subject: f.entry.actor, Permission: p, Scope: values.AnchorClosureScope(f.target.c.AuthorityRef(), networkRoot)}
	var err error
	if remove {
		_, err = f.target.r.DeleteBinding(f.target.actor, b, f.target.r.BindingSetRevision(), realm.OperationID(id))
	} else {
		_, err = f.target.r.PutBinding(f.target.actor, b, f.target.r.BindingSetRevision(), realm.OperationID(id))
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (f federationNetworkFixture) open() (federationcontract.FederationReadConnection, error) {
	return f.bound.OpenFederationRead(context.Background(), f.caller, f.target.config.NextHop, f.target.c.AuthorityRef(), f.request)
}

// Original operation permissions govern the private projection too. Query-only
// authorization cannot disclose Read records, and List does not require Read.
func TestNetworkFederationPartOriginalIdentityAndCapacity(t *testing.T) {
	f := newFederationNetworkFixture(t, false)
	f.grant(t, corecontract.PermissionQueryRegion, false, "query-only")
	if _, err := f.open(); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("query became record disclosure: %v", err)
	}
	f.request.Intent = "list"
	f.grant(t, corecontract.PermissionListDirect, false, "list-only")
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	if _, err := f.open(); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("outbound part capacity: %v", err)
	}
	projection := federationcontract.FederationProjectionRequest{Kind: "list", Element: networkRoot}
	first, err := part.Project(context.Background(), projection, nil, 1)
	if err != nil || first.Complete || first.Continuation == nil || len(first.Records) != 1 || first.Records[0].ID != networkEntity || first.Records[0].Attachment != nil {
		t.Fatalf("first source page: %+v %v", first, err)
	}
	second, err := part.Project(context.Background(), projection, first.Continuation, 1)
	if err != nil || !second.Complete || len(second.Records) != 1 || second.Records[0].ID != "00000000-0000-4000-8000-000000000003" {
		t.Fatalf("second source page: %+v %v", second, err)
	}
	if err := part.Check(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := part.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := part.Project(context.Background(), projection, nil, 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("released part reused: %v", err)
	}
	// Releasing one lease frees local capacity without extending the old grant.
	part, err = f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	if _, err := f.entry.r.RevokeCredential(f.entry.admin, realm.CredentialID(f.entry.id), f.entry.r.CredentialSetRevision(), "revoke-original"); err != nil {
		t.Fatal(err)
	}
	if err := part.Check(context.Background(), false); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("source credential revocation ignored: %v", err)
	}
}

func TestNetworkFederationPartTargetAuthorizationABA(t *testing.T) {
	f := newFederationNetworkFixture(t, true)
	for i, p := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionListDirect} {
		f.grant(t, p, false, fmt.Sprintf("grant-%d", i))
	}
	part, err := f.open()
	if err != nil {
		t.Fatal(err)
	}
	defer part.Release(context.Background())
	f.grant(t, corecontract.PermissionReadElement, true, "revoke")
	f.grant(t, corecontract.PermissionReadElement, false, "regrant")
	if err := part.Check(context.Background(), false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("target ABA refreshed old part: %v", err)
	}
}
