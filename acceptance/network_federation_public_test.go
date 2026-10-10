package acceptance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	federationconn "ucupa/ucupa-core/transport/grpc/federationconn"
	mountconn "ucupa/ucupa-core/transport/grpc/mountconn"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// Control establishes a real mount through the already-tested authenticated
// control channel. All discovery/read assertions below use the public entry's
// gRPC SDK, not the internal source projection interface.
func publicFederationNetworkFixture(t *testing.T) (*networkFixture, *networkFixture, values.ElementID) {
	t.Helper()
	b, a, service, _ := mountApprovalFixture(t, true)
	ctx := context.Background()
	m := values.ElementID(networkEntity)
	a.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(a.c.AuthorityRef(), m), false)
	a.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(a.c.AuthorityRef(), networkRoot), false)
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.c.AuthorityRef(), m), false)
	radius, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	created, err := a.c.Elements().CreateAnchor(a.actor, m, corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "public-federation-mount")
	if err != nil {
		t.Fatal(err)
	}
	command := corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: m, ExpectedElementRevision: created.Record.Anchor.Revision, CarrierAuthority: b.c.AuthorityRef(), CarrierNextHop: b.config.NextHop, ParentNextHop: a.config.NextHop}
	digest, err := a.c.Mounts().CommandCommitment(a.c.AuthorityRef(), b.c.AuthorityRef(), "public-federation-establish", command)
	if err != nil {
		t.Fatal(err)
	}
	approval := createMountApproval(t, b, &pb.MountApprovalBinding{Actor: proto.String(string(a.c.AuthorityRef())), Element: proto.String(string(m)), OperationId: []byte("public-federation-establish"), CommandDigest: digest[:]}, 5)
	mount, err := mountconn.NewMountConnections(a.c.AuthorityRef(), a.c.Mounts(), []*coresdk.Client{service})
	if err != nil {
		t.Fatal(err)
	}
	mount, err = mount.WithApproval(approval)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := a.r.EstablishCaller(realm.CredentialEvidence{Audience: a.actor.Realm, ID: realm.CredentialID(a.id), Secret: realm.CredentialSecret(a.secret)})
	if err != nil {
		t.Fatal(err)
	}
	view, err := a.c.Mounts().AuthorizeMountControl(ctx, caller, command)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.c.Mounts().ApplyMountControl(ctx, caller, view, "public-federation-establish", command, mount)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("mount: %+v %v", result, err)
	}
	leaf := values.ElementID("00000000-0000-4000-8000-000000000003")
	b.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(b.c.AuthorityRef(), m), false)
	if _, err := b.c.Elements().CreateEntity(b.actor, leaf, corecontract.SpatialTuple{Parent: m, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "public-federation-leaf"); err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := realminstance.NewIdentityIssuer(a.r, realm.IdentityIssuerConfig{Key: key, MaxTTL: time.Minute, MaxProofs: 32, MaxPerCaller: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(issuer.Close)
	a.config.IdentityProofs = composition.RealmIdentityProofs{Issuer: issuer}
	a.config.ReadDelegation = identityPeerTrust(b)
	checker := composition.NetworkIdentityCheck{Client: identityServiceClient(t, a, b), Verifier: b.c.AuthorityRef()}
	verifier, err := realminstance.NewIdentityVerifier(map[values.RealmID]ed25519.PublicKey{a.r.ID(): pub}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	external, err := composition.NewRealmExternalIdentity(b.realmLinks, b.c.Access(), verifier, map[values.RealmID]composition.IdentityLiveness{a.r.ID(): checker})
	if err != nil {
		t.Fatal(err)
	}
	b.server.Stop()
	b.config.ExternalIdentity = external
	b.config.ReadDelegation = identityPeerTrust(a)
	startIdentityServer(t, b)
	for i, permission := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionListDirect, corecontract.PermissionQueryRegion} {
		binding := realm.PermissionBinding{Subject: a.actor, Permission: permission, Scope: values.AnchorClosureScope(b.c.AuthorityRef(), m)}
		if _, err := b.r.PutBinding(b.actor, binding, b.r.BindingSetRevision(), realm.OperationID("public-original-"+string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	cfg := b.clientConfig
	cfg.Actor, cfg.ActorCertificate = a.c.AuthorityRef(), &a.config.Certificate
	connections, err := federationconn.NewFederationConnections(federationconn.FederationConnectionConfig{Source: a.c.AuthorityRef(), Peers: []coresdk.Config{cfg}, MaxParts: 16, TTL: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(connections.Close)
	a.config.FederationConnections = connections
	startIdentityServer(t, a)
	a.connect(t)
	return a, b, leaf
}

func TestNetworkFederationPublicQueryAndReadThroughEntry(t *testing.T) {
	a, b, leaf := publicFederationNetworkFixture(t)
	ctx := context.Background()
	handle := a.enter(t, pb.Intent_QUERY)
	root := &netpb.Address{Origin: proto.String(string(networkRoot)), Up: proto.Uint64(0), Down: &netpb.ElementPath{}}
	request := &pb.QueryRegionRequest{Handle: handle, Start: root, Basis: root, Sphere: &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)}, Selector: pb.Selector_BOTH.Enum(), Limit: proto.Uint32(1)}
	seen := map[string]bool{}
	var address *netpb.Address
	var basis *netpb.Token
	var graph *pb.PublicSourceGraph
	for i := 0; i < 4; i++ {
		response, err := a.client.Core().QueryRegion(ctx, request)
		if err != nil || response.GetResult() == nil {
			t.Fatalf("public query: %v %v", response, err)
		}
		result := response.GetResult()
		if result.SourceGraph == nil || len(result.SourceGraph.Sources) != 2 {
			t.Fatal("missing public source views", result)
		}
		for _, s := range result.SourceGraph.Sources {
			if s.Coverage != nil && len(s.Parents) == 0 {
				t.Fatal("query source missing parent closure", s.GetOwner())
			}
		}
		if graph == nil {
			graph = proto.Clone(result.SourceGraph).(*pb.PublicSourceGraph)
		} else if !proto.Equal(graph, result.SourceGraph) {
			t.Fatal("source graph changed across pages")
		}
		basis = result.Receipt
		for _, match := range result.Matches {
			if seen[match.GetElement()] {
				t.Fatal("duplicate across public pages")
			}
			seen[match.GetElement()] = true
			if match.GetElement() == string(leaf) {
				address = match.Address
				if match.Revision.GetOwner() != string(b.c.AuthorityRef()) {
					t.Fatal("entry replaced source owner")
				}
			}
		}
		if result.Continuation == nil {
			if result.GetStatus() != pb.Coverage_COMPLETE {
				t.Fatal("not complete", result)
			}
			break
		}
		request.Continuation = result.Continuation
	}
	if len(seen) != 3 || address == nil {
		t.Fatal("lost mounted source", seen)
	}
	read, err := a.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: handle, Address: address})
	if err != nil || read.GetResult() == nil || read.GetResult().GetRecord().GetElement() != string(leaf) {
		t.Fatalf("public remote exact read: %v %v", read, err)
	}
	if read.GetResult().Attachment.GetRevision().GetOwner() != string(b.c.AuthorityRef()) {
		t.Fatal("wrong attachment owner")
	}
	sameCut, err := a.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: handle, Address: address, Basis: basis})
	if err != nil || sameCut.GetResult() == nil || !proto.Equal(sameCut.GetResult().GetRecord(), read.GetResult().GetRecord()) {
		t.Fatalf("public composite read-at with independent proof: %v %v", sameCut, err)
	}
	if !proto.Equal(graph, sameCut.GetResult().SourceGraph) {
		t.Fatal("derived Read changed original source views")
	}
	closed, err := a.client.Core().Close(ctx, &pb.CloseRequest{Handle: handle})
	if err != nil || !closed.GetResult().GetClosed() {
		t.Fatalf("close: %v %v", closed, err)
	}
}
