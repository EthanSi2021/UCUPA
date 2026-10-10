package acceptance_test

import (
	"context"
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

func TestNetworkMountIntermediateProcessCrashes(t *testing.T) {
	for _, phase := range []string{"before-prepare", "after-prepare", "before-finish", "after-finish"} {
		t.Run(phase, func(t *testing.T) { testMountIntermediateCrash(t, phase) })
	}
}

func testMountIntermediateCrash(t *testing.T, phase string) {
	a, b := newNetworkFixtureRealm(t, false, "crash-a"), newNetworkFixtureRealm(t, false, "crash-b")
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
	for _, permission := range []values.PermissionID{corecontract.PermissionMountControl, corecontract.PermissionMountRead} {
		a.grant(t, permission, values.CoreElementScope(a.config.Expected, element), false)
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionCreateAnchor, corecontract.PermissionPlaceElement} {
		a.grant(t, permission, values.CoreElementScope(a.config.Expected, networkRoot), false)
	}
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.config.Expected, element), false)
	sphere, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	if _, err := a.c.Elements().CreateAnchor(a.actor, element, corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: sphere}, "crash-anchor"); err != nil {
		t.Fatal(err)
	}
	aConfig := map[string]any{"read_delegation": identityTrustFiles(t, a, b), "mount_connections": mountProductConnections(t, a, b)}
	bConfig := map[string]any{"core_store": bPath, "read_delegation": identityTrustFiles(t, b, a), "mount_connections": mountProductConnections(t, b, a), "mount_approvals": map[string]any{"max_ttl": "30s", "max_approvals": 8, "max_per_caller": 6}}
	startA, startB := prepareNetworkProduct(t, a, aConfig), prepareNetworkProduct(t, b, bConfig)
	crashed := startMountCrashProduct(t, mountCrashConfig{Dir: b.dir, Store: bPath, Phase: phase, Expected: b.config.Expected, Peer: a.config.Expected, PeerPin: a.clientConfig.SPKISHA256, PeerCA: filepath.Join(b.dir, "peer-ca.pem"), NextHop: b.config.NextHop, RealmNextHop: b.realmAddress})
	stopA := startA()
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
	const id = "crash-operation"
	d, err := a.client.MountControl().DescribeMount(ctx, &pb.DescribeMountRequest{OperationId: []byte(id), Command: command})
	if err != nil || !d.GetResult().GetRequired() {
		t.Fatalf("describe: %v %v", d, err)
	}
	approval := createMountApproval(t, b, d.GetResult().Binding, 25)
	request := &pb.ApplyMountRequest{OperationId: []byte(id), Command: command, Approval: approval}
	var failed *pb.ApplyMountResponse
	deadline := time.Now().Add(10 * time.Second)
	for {
		failed, err = a.client.MountControl().ApplyMount(ctx, request)
		if err == nil && failed.GetResult() != nil {
			break
		}
		if time.Now().After(deadline) || err == nil && failed.GetFailure() != netpb.Failure_UNAVAILABLE {
			t.Fatalf("dispatch: %v %v", failed, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	crashed()
	want := pb.MountDecisionKind_MOUNT_PENDING
	if phase == "before-finish" || phase == "after-finish" {
		want = pb.MountDecisionKind_MOUNT_COMMITTED
	}
	if failed.GetResult().GetDecision() != want || failed.GetResult().GetSettled() {
		t.Fatalf("wrong interrupted state: %v", failed)
	}
	stopA()
	_ = startB()
	_ = startA()
	_ = a.client.Close()
	_ = b.client.Close()
	a.connect(t)
	b.connect(t)
	if want == pb.MountDecisionKind_MOUNT_PENDING && inspect().GetState() != uint32(corecontract.MountEstablishing) {
		t.Fatal("startup decided pending user write")
	}
	// The carrier's authenticated recovery channel reads the parent's durable
	// decision independently of the application session and expired approval.
	peer := identityServiceClient(t, a, b)
	decision, err := peer.MountPeer().ReadMountDecision(ctx, &pb.ReadMountDecisionRequest{ParentCore: proto.String(string(a.config.Expected)), Actor: proto.String(string(b.config.Expected)), OperationId: []byte(id)})
	if err != nil || decision.GetResult() == nil || decision.GetResult().GetDecision() != want {
		t.Fatalf("durable decision channel: %v %v", decision, err)
	}
	// Restart destroys B's process-local approval. A decided operation needs no
	// replacement; a pending operation requires a freshly approved exact intent.
	request.Approval = nil
	if want == pb.MountDecisionKind_MOUNT_PENDING {
		fresh, err := a.client.MountControl().DescribeMount(ctx, &pb.DescribeMountRequest{OperationId: []byte(id), Command: command})
		if err != nil || !fresh.GetResult().GetRequired() || !proto.Equal(fresh.GetResult().Binding, d.GetResult().Binding) {
			t.Fatalf("changed pending commitment: %v %v", fresh, err)
		}
		request.Approval = createMountApproval(t, b, fresh.GetResult().Binding, 25)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		out, err := a.client.MountControl().ApplyMount(ctx, request)
		if err == nil && out.GetResult() != nil && out.GetResult().GetDecision() == pb.MountDecisionKind_MOUNT_COMMITTED && out.GetResult().GetSettled() {
			break
		}
		if time.Now().After(deadline) || err == nil && out.GetResult() == nil && out.GetFailure() != netpb.Failure_UNAVAILABLE {
			t.Fatalf("recovery: %v %v", out, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if inspect().GetState() != uint32(corecontract.MountActive) {
		t.Fatal("recovered mount is not active")
	}
}
