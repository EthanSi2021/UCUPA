package acceptance_test

import (
	"context"
	"encoding/pem"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func mountProductConnections(t *testing.T, host, peer *networkFixture) []any {
	t.Helper()
	ca := filepath.Join(host.dir, "mount-peer-ca.pem")
	writeNetworkFile(t, ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peer.config.Certificate.Certificate[1]}))
	pin := peer.clientConfig.SPKISHA256
	return []any{map[string]any{"next_hop": peer.config.NextHop, "expected_core": peer.config.Expected, "ca_file": ca, "spki_sha256": values.EncodePublicBytes(pin[:])}}
}

func TestNetworkMountControlIndependentProductsRestart(t *testing.T) {
	a, b := newNetworkFixtureRealm(t, false, "mount-a"), newNetworkFixtureRealm(t, false, "mount-b")
	if err := b.c.Close(); err != nil {
		t.Fatal(err)
	}
	bPath := filepath.Join(b.dir, "carrier.db")
	var err error
	b.c, err = core.CreateDurable(b.realmLinks, corecontract.RealmInitialization{Realm: b.actor.Realm, NextHop: b.realmAddress}, bPath, corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	b.config.Expected, b.clientConfig.Expected = b.c.AuthorityRef(), b.c.AuthorityRef()
	element := values.ElementID(networkEntity)
	for _, permission := range []values.PermissionID{corecontract.PermissionMountControl, corecontract.PermissionMountRead, corecontract.PermissionUpdateElement} {
		a.grant(t, permission, values.CoreElementScope(a.config.Expected, element), false)
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionCreateAnchor, corecontract.PermissionPlaceElement} {
		a.grant(t, permission, values.CoreElementScope(a.config.Expected, networkRoot), false)
	}
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.config.Expected, element), false)
	b.grant(t, corecontract.PermissionMountRead, values.CoreElementScope(b.config.Expected, element), false)
	sphere, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	if _, err := a.c.Elements().CreateAnchor(a.actor, element, corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: sphere}, "product-anchor"); err != nil {
		t.Fatal(err)
	}
	// Both owners and Realms are closed by preparation. The test thereafter
	// controls only actual product processes and speaks public gRPC.
	aConfig := map[string]any{"read_delegation": identityTrustFiles(t, a, b), "mount_connections": mountProductConnections(t, a, b)}
	bConfig := map[string]any{"core_store": bPath, "read_delegation": identityTrustFiles(t, b, a), "mount_connections": mountProductConnections(t, b, a), "mount_approvals": map[string]any{"max_ttl": "30s", "max_approvals": 8, "max_per_caller": 6}}
	startA, startB := prepareNetworkProduct(t, a, aConfig), prepareNetworkProduct(t, b, bConfig)
	stopB, stopA := startB(), startA()
	a.connect(t)
	b.connect(t)
	ctx := context.Background()
	inspect := func() *pb.PublicMountInspection {
		t.Helper()
		out, err := a.client.MountControl().InspectMount(ctx, &pb.InspectMountRequest{Element: proto.String(string(element))})
		if err != nil || out.GetResult() == nil {
			t.Fatalf("inspect: %v %v", out, err)
		}
		return out.GetResult()
	}
	initial := inspect()
	command := &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_ESTABLISH.Enum(), Element: proto.String(string(element)), ExpectedElement: initial.ElementRevision, CarrierCore: proto.String(string(b.config.Expected)), CarrierNextHop: proto.String(string(b.config.NextHop)), ParentNextHop: proto.String(string(a.config.NextHop))}
	d, err := a.client.MountControl().DescribeMount(ctx, &pb.DescribeMountRequest{OperationId: []byte("process-establish"), Command: command})
	if err != nil || !d.GetResult().GetRequired() {
		t.Fatalf("describe: %v %v", d, err)
	}
	approval := createMountApproval(t, b, d.GetResult().Binding, 25)
	request := &pb.ApplyMountRequest{OperationId: []byte("process-establish"), Command: command, Approval: approval}
	apply := func(request *pb.ApplyMountRequest) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for {
			out, err := a.client.MountControl().ApplyMount(ctx, request)
			if err == nil && out.GetResult() != nil && out.GetResult().GetDecision() == pb.MountDecisionKind_MOUNT_COMMITTED && out.GetResult().GetSettled() {
				return
			}
			if time.Now().After(deadline) || err == nil && out.GetFailure() != netpb.Failure_UNAVAILABLE && out.GetResult() == nil {
				t.Fatalf("exact application retry: %v %v", out, err)
			}
			// Explicit test recovery of the SAME command/id, never SDK retry or
			// updated CAS. A startup connection may not yet be ready.
			time.Sleep(20 * time.Millisecond)
		}
	}
	apply(request)
	// Contact maintenance is local to the receiving owner: no peer approval or
	// roundtrip is required, including while the other process is stopped.
	stopB()
	contact, err := a.client.MountContact().InspectMountContact(ctx, &pb.InspectMountContactRequest{Element: proto.String(string(element)), Incoming: proto.Bool(false)})
	if err != nil || contact.GetResult() == nil {
		t.Fatalf("contact: %v %v", contact, err)
	}
	wrong := &pb.RepairMountContactRequest{OperationId: []byte("offline-contact"), Element: proto.String(string(element)), Incoming: proto.Bool(false), Expected: contact.GetResult().Revision, NextHop: proto.String("https://127.0.0.1:1")}
	repaired, err := a.client.MountContact().RepairMountContact(ctx, wrong)
	if err != nil || repaired.GetResult() == nil {
		t.Fatalf("offline repair: %v %v", repaired, err)
	}
	replay, err := a.client.MountContact().RepairMountContact(ctx, wrong)
	if err != nil || !proto.Equal(repaired, replay) {
		t.Fatalf("contact replay: %v %v", replay, err)
	}
	stale := proto.Clone(wrong).(*pb.RepairMountContactRequest)
	stale.OperationId = []byte("stale-contact")
	if out, err := a.client.MountContact().RepairMountContact(ctx, stale); err != nil || out.GetFailure() != netpb.Failure_CONFLICT {
		t.Fatalf("stale contact: %v %v", out, err)
	}
	restored, err := a.client.MountContact().RepairMountContact(ctx, &pb.RepairMountContactRequest{OperationId: []byte("restore-contact"), Element: proto.String(string(element)), Incoming: proto.Bool(false), Expected: repaired.GetResult().Revision, NextHop: proto.String(string(b.config.NextHop))})
	if err != nil || restored.GetResult() == nil {
		t.Fatalf("restore: %v %v", restored, err)
	}
	stopB = startB()
	_ = b.client.Close()
	b.connect(t)
	incoming, err := b.client.MountContact().InspectMountContact(ctx, &pb.InspectMountContactRequest{Element: proto.String(string(element)), Incoming: proto.Bool(true)})
	if err != nil || incoming.GetResult() == nil || incoming.GetResult().GetPeerCore() != string(a.config.Expected) {
		t.Fatalf("reverse contact: %v %v", incoming, err)
	}
	noChange, err := b.client.MountContact().RepairMountContact(ctx, &pb.RepairMountContactRequest{OperationId: []byte("reverse-contact"), Element: proto.String(string(element)), Incoming: proto.Bool(true), Expected: incoming.GetResult().Revision, NextHop: proto.String(string(a.config.NextHop))})
	if err != nil || noChange.GetResult() == nil || !proto.Equal(incoming.GetResult(), noChange.GetResult()) {
		t.Fatalf("reverse repair: %v %v", noChange, err)
	}
	before := inspect()
	stopA()
	stopB()
	stopB, stopA = startB(), startA()
	_ = a.client.Close()
	_ = b.client.Close()
	a.connect(t)
	b.connect(t)
	// Old sessions and B approvals died. A durable decision remains replayable
	// with a freshly authenticated original caller and the original opaque CAS.
	request.Approval = nil
	apply(request)
	after := inspect()
	if !proto.Equal(before, after) {
		t.Fatal("restart changed the relationship or public comparisons")
	}
	revoke := &pb.ApplyMountRequest{OperationId: []byte("process-revoke"), Command: &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_REVOKE.Enum(), Element: proto.String(string(element)), ExpectedMount: after.MountRevision}}
	apply(revoke)
	if inspect().GetState() != uint32(corecontract.MountDetached) {
		t.Fatal("revoke not persisted")
	}
	stopA()
	stopB()
	_ = startB()
	_ = startA()
	_ = a.client.Close()
	a.connect(t)
	apply(revoke)
	if inspect().GetState() != uint32(corecontract.MountDetached) {
		t.Fatal("revoke lost at restart")
	}
}
