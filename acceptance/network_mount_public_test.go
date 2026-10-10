package acceptance_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	mountconn "ucupa/ucupa-core/transport/grpc/mountconn"
)

func TestNetworkPublicMountControlOpaqueCommands(t *testing.T) {
	b, a, service, _ := mountApprovalFixture(t, true)
	ctx := context.Background()
	element := values.ElementID(networkEntity)
	for _, permission := range []values.PermissionID{corecontract.PermissionMountControl, corecontract.PermissionMountRead, corecontract.PermissionUpdateElement} {
		a.grant(t, permission, values.CoreElementScope(a.c.AuthorityRef(), element), false)
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionCreateAnchor, corecontract.PermissionPlaceElement} {
		a.grant(t, permission, values.CoreElementScope(a.c.AuthorityRef(), networkRoot), false)
	}
	b.grant(t, corecontract.PermissionMountAccept, values.CoreElementScope(b.c.AuthorityRef(), element), false)
	sphere, _ := corecontract.NewBoundedSphere(10)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	if _, err := a.c.Elements().CreateAnchor(a.actor, element, corecontract.SpatialTuple{Parent: networkRoot, LocalPose: corecontract.Pose{Rotation: rotation}, Range: sphere}, "public-mount-anchor"); err != nil {
		t.Fatal(err)
	}
	connections, err := mountconn.NewMountConnections(a.c.AuthorityRef(), a.c.Mounts(), []*coresdk.Client{service})
	if err != nil {
		t.Fatal(err)
	}
	a.config.MountConnections = connections
	l, err := net.Listen("tcp", strings.TrimPrefix(string(a.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	a.config.FacetControl = a.c.Facets()
	a.config.MountControl = a.c.Mounts()
	a.server, err = coregrpc.New(a.c.Access(), composition.RealmNetworkAuthentication{Connections: a.realmLinks, Owner: a.c.Access()}, a.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = a.server.Serve(l) }()
	a.connect(t)
	control := a.client.MountControl()
	inspect := func() *pb.PublicMountInspection {
		t.Helper()
		out, err := control.InspectMount(ctx, &pb.InspectMountRequest{Element: proto.String(string(element))})
		if err != nil || out.GetResult() == nil {
			t.Fatalf("inspect: %v %v", out, err)
		}
		return out.GetResult()
	}
	initial := inspect()
	command := &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_ESTABLISH.Enum(), Element: proto.String(string(element)), ExpectedElement: initial.ElementRevision, CarrierCore: proto.String(string(b.c.AuthorityRef())), CarrierNextHop: proto.String(string(b.config.NextHop)), ParentNextHop: proto.String(string(a.config.NextHop))}
	// A missing target approval or a denied original caller must not reserve a
	// parent control slot. No local write helper is used for these requests.
	missing, err := control.ApplyMount(ctx, &pb.ApplyMountRequest{OperationId: []byte("missing-approval"), Command: command})
	if err != nil || missing.GetFailure() != netpb.Failure_UNAVAILABLE {
		t.Fatalf("missing approval: %v %v", missing, err)
	}
	a.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(a.c.AuthorityRef(), element), true)
	denied, err := control.ApplyMount(ctx, &pb.ApplyMountRequest{OperationId: []byte("denied-original"), Command: command})
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("denied original: %v %v", denied, err)
	}
	a.grant(t, corecontract.PermissionMountControl, values.CoreElementScope(a.c.AuthorityRef(), element), false)
	if inspect().GetState() != uint32(corecontract.MountNone) {
		t.Fatal("failed requests reserved a mount")
	}
	apply := func(id string, command *pb.PublicMountCommand, requires bool) {
		t.Helper()
		d, err := control.DescribeMount(ctx, &pb.DescribeMountRequest{OperationId: []byte(id), Command: command})
		if err != nil || d.GetResult() == nil || d.GetResult().GetRequired() != requires {
			t.Fatalf("describe %s: %v %v", id, d, err)
		}
		request := &pb.ApplyMountRequest{OperationId: []byte(id), Command: command}
		if requires {
			request.Approval = createMountApproval(t, b, d.GetResult().Binding, 5)
		}
		out, err := control.ApplyMount(ctx, request)
		if err != nil || out.GetResult() == nil || out.GetResult().GetDecision() != pb.MountDecisionKind_MOUNT_COMMITTED || !out.GetResult().GetSettled() {
			t.Fatalf("apply %s: %v %v", id, out, err)
		}
		// The same command is recovered without replacing its original target
		// approval. The persisted decision remains authoritative.
		request.Approval = nil
		out, err = control.ApplyMount(ctx, request)
		if err != nil || out.GetResult() == nil || !out.GetResult().GetSettled() {
			t.Fatalf("retry %s: %v %v", id, out, err)
		}
	}
	apply("public-establish", command, true)
	changed := proto.Clone(command).(*pb.PublicMountCommand)
	changed.ParentNextHop = proto.String("https://127.0.0.1:1")
	out, err := control.ApplyMount(ctx, &pb.ApplyMountRequest{OperationId: []byte("public-establish"), Command: changed})
	if err != nil || out.GetFailure() != netpb.Failure_CONFLICT {
		t.Fatalf("changed retry: %v %v", out, err)
	}
	active := inspect()
	// Geometry is ordinary public values, not a ServiceRevision or private cut.
	tuple := &pb.SpatialTuple{Parent: proto.String(string(networkRoot)), Pose: &pb.Pose{Translation: &netpb.Vector3{Items: []float64{0, 0, 0}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(1), Ratios: &netpb.Vector3{Items: []float64{0, 0, 0}}}}, Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(5)}}
	update := &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_UPDATE.Enum(), Element: proto.String(string(element)), ExpectedElement: active.ElementRevision, ExpectedMount: active.MountRevision, Final: tuple}
	apply("public-resize", update, true)
	active = inspect()
	tuple = proto.Clone(tuple).(*pb.SpatialTuple)
	tuple.Pose.Translation.Items[0] = 1
	update = &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_UPDATE.Enum(), Element: proto.String(string(element)), ExpectedElement: active.ElementRevision, ExpectedMount: active.MountRevision, Final: tuple}
	apply("public-pose", update, false)
	active = inspect()
	apply("public-revoke", &pb.PublicMountCommand{Kind: pb.MountCommandKind_MOUNT_REVOKE.Enum(), Element: proto.String(string(element)), ExpectedMount: active.MountRevision}, false)
	if inspect().GetState() != uint32(corecontract.MountDetached) {
		t.Fatal("not detached")
	}
}
