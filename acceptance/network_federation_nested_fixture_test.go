package acceptance_test

import (
	"context"
	"encoding/pem"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	mountconn "ucupa/ucupa-core/transport/grpc/mountconn"
	realm "ucupa/ucupa-realm/contract"
)

const nestedProductAnchor values.ElementID = "00000000-0000-4000-8000-000000000004"
const nestedProductLeaf values.ElementID = "00000000-0000-4000-8000-000000000005"
const nestedWritableAnchor values.ElementID = "00000000-0000-4000-8000-000000000006"

// Extend the provisioned A->B with B->C using the real authenticated mount
// control transport. The subsequent test closes all of these owners before
// starting three product executables; no source projection is mocked.
func addNestedProductCarrier(t *testing.T, b *networkFixture) *networkFixture {
	t.Helper()
	c := newNetworkFixtureRealm(t, false, "nested-carrier-realm")
	if err := c.c.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	c.c, err = core.CreateDurable(c.realmLinks, corecontract.RealmInitialization{Realm: c.actor.Realm, NextHop: c.realmAddress}, filepath.Join(c.dir, "empty-carrier.db"), corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	c.config.Expected, c.clientConfig.Expected = c.c.AuthorityRef(), c.c.AuthorityRef()
	c.config.ReadDelegation = identityPeerTrust(b)
	c.config.MountApprovals = &coregrpc.MountApprovalConfig{MaxTTL: 5 * time.Second, MaxApprovals: 3, MaxPerCaller: 2}
	startIdentityServer(t, c)
	c.connect(t)
	b.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(b.config.Expected, values.ElementID(networkEntity)), false)
	b.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(b.config.Expected, nestedProductAnchor), false)
	c.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(c.config.Expected, nestedProductAnchor), false)
	radius, _ := corecontract.NewBoundedSphere(5)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	created, err := b.c.Elements().CreateAnchor(b.actor, nestedProductAnchor, corecontract.SpatialTuple{Parent: values.ElementID(networkEntity), LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "nested-product-anchor")
	if err != nil {
		t.Fatal(err)
	}
	command := corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: nestedProductAnchor, ExpectedElementRevision: created.Record.Anchor.Revision, CarrierAuthority: c.config.Expected, CarrierNextHop: c.config.NextHop, ParentNextHop: b.config.NextHop}
	operation := corecontract.OperationID("nested-product-mount")
	digest, err := b.c.Mounts().CommandCommitment(b.config.Expected, c.config.Expected, operation, command)
	if err != nil {
		t.Fatal(err)
	}
	approval := createMountApproval(t, c, &pb.MountApprovalBinding{Actor: proto.String(string(b.config.Expected)), Element: proto.String(string(nestedProductAnchor)), OperationId: []byte(operation), CommandDigest: digest[:]}, 5)
	cfg := c.clientConfig
	cfg.Actor, cfg.ActorCertificate = b.config.Expected, &b.config.Certificate
	client, err := coresdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	connections, err := mountconn.NewMountConnections(b.config.Expected, b.c.Mounts(), []*coresdk.Client{client})
	if err != nil {
		t.Fatal(err)
	}
	connections, err = connections.WithApproval(approval)
	if err != nil {
		t.Fatal(err)
	}
	caller, err := b.r.EstablishCaller(realm.CredentialEvidence{Audience: b.actor.Realm, ID: realm.CredentialID(b.id), Secret: realm.CredentialSecret(b.secret)})
	if err != nil {
		t.Fatal(err)
	}
	view, err := b.c.Mounts().AuthorizeMountControl(context.Background(), caller, command)
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.c.Mounts().ApplyMountControl(context.Background(), caller, view, operation, command, connections)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatal(result, err)
	}
	c.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(c.config.Expected, nestedProductAnchor), false)
	if _, err := c.c.Elements().CreateEntity(c.actor, nestedProductLeaf, corecontract.SpatialTuple{Parent: nestedProductAnchor, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "nested-product-leaf"); err != nil {
		t.Fatal(err)
	}
	c.grant(t, corecontract.PermissionCreateAnchor, values.CoreElementScope(c.config.Expected, nestedProductAnchor), false)
	small, _ := corecontract.NewBoundedSphere(2)
	if _, err := c.c.Elements().CreateAnchor(c.actor, nestedWritableAnchor, corecontract.SpatialTuple{Parent: nestedProductAnchor, LocalPose: corecontract.Pose{Rotation: rotation}, Range: small}, "nested-writable-anchor"); err != nil {
		t.Fatal(err)
	}
	c.grant(t, corecontract.PermissionCreateEntity, values.AnchorClosureScope(c.config.Expected, nestedWritableAnchor), false)
	return c
}

func federationProductTrust(t *testing.T, host *networkFixture, peers ...*networkFixture) map[string]any {
	t.Helper()
	var roots []byte
	pins := map[string]string{}
	for _, peer := range peers {
		roots = append(roots, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peer.config.Certificate.Certificate[1]})...)
		pin := peer.clientConfig.SPKISHA256
		pins[string(peer.config.Expected)] = values.EncodePublicBytes(pin[:])
	}
	path := filepath.Join(host.dir, "federation-peers.pem")
	writeNetworkFile(t, path, roots)
	return map[string]any{"client_ca_file": path, "client_spki_sha256": pins, "max_ttl": "30s", "max_grants": 16, "max_per_caller": 12}
}
