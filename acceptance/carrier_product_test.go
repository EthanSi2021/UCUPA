package acceptance_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

// Provision the existing mount through actual owner APIs. All in-process
// owners are then closed: only the parent product handles transfer controls.
// No source/target service is needed for the pre-permit reservation/cancel;
// this test deliberately does not claim a completed three-party transfer.
func carrierProductParent(t *testing.T) (*networkFixture, values.FactAuthorityRef, values.NextHop, map[string]any) {
	t.Helper()
	a, b := newNetworkFixtureRealm(t, false, "carrier-parent"), newNetworkFixtureRealm(t, false, "carrier-source")
	if err := b.c.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	b.c, err = core.CreateDurable(b.realmLinks, contract.RealmInitialization{Realm: b.actor.Realm, NextHop: b.realmAddress}, filepath.Join(b.dir, "empty-carrier.db"), contract.Config{
		PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	element := values.ElementID(networkEntity)
	for _, permission := range []values.PermissionID{contract.PermissionCreateAnchor, contract.PermissionPlaceElement} {
		a.grant(t, permission, values.CoreElementScope(a.c.AuthorityRef(), networkRoot), false)
	}
	for _, permission := range []values.PermissionID{contract.PermissionMountControl, contract.PermissionMountRead, contract.PermissionTransitionCommit, contract.PermissionTransitionInspect} {
		a.grant(t, permission, values.CoreElementScope(a.c.AuthorityRef(), element), false)
	}
	b.grant(t, contract.PermissionMountAccept, values.CoreElementScope(b.c.AuthorityRef(), element), false)
	radius, _ := contract.NewBoundedSphere(10)
	rotation, _ := contract.NewRotation(1, 0, 0, 0)
	created, err := a.c.Elements().CreateAnchor(a.actor, element, contract.SpatialTuple{Parent: networkRoot, LocalPose: contract.Pose{Rotation: rotation}, Range: radius}, "carrier-anchor")
	if err != nil || created.Record == nil || created.Record.Anchor == nil {
		t.Fatal(created, err)
	}
	connections := &composition.LocalMountConnections{}
	if err := connections.BindEndpoint("local:carrier-parent", a.c.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	if err := connections.BindEndpoint("local:carrier-source", b.c.Mounts().MountPeer()); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*core.Core{a.c, b.c} {
		link, err := connections.TransportFor(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Mounts().ConfigureMountTransport(link); err != nil {
			t.Fatal(err)
		}
	}
	command := contract.MountCommand{Kind: contract.MountEstablish, MountAnchorUUID: element, ExpectedElementRevision: created.Record.Anchor.Revision,
		CarrierAuthority: b.c.AuthorityRef(), ParentNextHop: "local:carrier-parent", CarrierNextHop: "local:carrier-source"}
	r, err := a.c.Mounts().ApplyMount(context.Background(), contract.LocalCallers{a.c.AuthorityRef(): a.actor, b.c.AuthorityRef(): b.actor}, "original-mount", command)
	if err != nil || !r.Settled || r.Decision != contract.MountCommitted {
		t.Fatal(r, err)
	}
	source := b.c.AuthorityRef()
	b.config.Expected = source
	// The existing mount inspection API requires its ordinary connection profile;
	// the peer remains offline throughout parent reservation and cancellation.
	config := map[string]any{"mount_connections": mountProductConnections(t, a, b)}
	if err := b.c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.r.Close(); err != nil {
		t.Fatal(err)
	}
	return a, source, command.CarrierNextHop, config
}

func TestCarrierControlIndependentParentProductRestart(t *testing.T) {
	f, source, sourceAddress, config := carrierProductParent(t)
	start := prepareNetworkProduct(t, f, config)
	stop := start()
	defer stop()
	f.connect(t)
	ctx := context.Background()
	mount, err := f.client.MountControl().InspectMount(ctx, &pb.InspectMountRequest{Element: proto.String(networkEntity)})
	if err != nil || mount.GetResult() == nil {
		t.Fatal(mount, err)
	}
	q := &pb.BeginCarrierTransitionRequest{OperationId: []byte{255, 0, 128, 1}, Mount: proto.String(networkEntity), Source: proto.String(string(source)),
		Target: proto.String("00000000-0000-4000-8000-000000000031"), ExpectedElement: mount.GetResult().ElementRevision, ExpectedMount: mount.GetResult().MountRevision,
		ParentAddress: proto.String("local:carrier-parent"), SourceAddress: proto.String(string(sourceAddress)), TargetAddress: proto.String("https://target.example:443")}
	begin, err := f.client.Transitions().BeginCarrierTransition(ctx, q)
	if err != nil || begin.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_RESERVED {
		t.Fatal(begin, err)
	}
	if begin.GetResult().GetParent() != string(f.config.Expected) || begin.GetResult().GetSource() != string(source) {
		t.Fatal("controller confused with carrier", begin)
	}
	wrong := proto.Clone(q).(*pb.BeginCarrierTransitionRequest)
	wrong.TargetAddress = proto.String("https://other.example:443")
	if r, err := f.client.Transitions().BeginCarrierTransition(ctx, wrong); err != nil || r.GetError() != netpb.Failure_CONFLICT {
		t.Fatal("changed operation accepted", r, err)
	}
	inspect := &pb.InspectCarrierTransitionRequest{OperationId: q.OperationId, Mount: q.Mount}
	stop()
	stop = start()
	defer stop()
	if _, err := f.client.Transitions().InspectCarrierTransition(ctx, inspect); status.Code(err) != codes.Unauthenticated {
		t.Fatal("old session survived", err)
	}
	_ = f.client.Close()
	f.connect(t)
	if r, err := f.client.Transitions().BeginCarrierTransition(ctx, q); err != nil || !proto.Equal(begin, r) {
		t.Fatal("restart reservation lost", r, err)
	}
	cancel := &pb.CancelCarrierTransitionRequest{OperationId: q.OperationId, Mount: q.Mount, Expected: begin.GetResult().Revision}
	ended, err := f.client.Transitions().CancelCarrierTransition(ctx, cancel)
	if err != nil || ended.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_CANCELLED {
		t.Fatal(ended, err)
	}
	stop()
	stop = start()
	defer stop()
	_ = f.client.Close()
	f.connect(t)
	if r, err := f.client.Transitions().CancelCarrierTransition(ctx, cancel); err != nil || !proto.Equal(ended, r) {
		t.Fatal("restart cancellation lost", r, err)
	}
	if r, err := f.client.Transitions().BeginCarrierTransition(ctx, q); err != nil || !proto.Equal(begin, r) {
		t.Fatal("original reservation changed", r, err)
	}
	if r, err := f.client.Transitions().InspectCarrierTransition(ctx, inspect); err != nil || !proto.Equal(ended, r) {
		t.Fatal("cancelled operation revived", r, err)
	}
	after, err := f.client.MountControl().InspectMount(ctx, &pb.InspectMountRequest{Element: q.Mount})
	if err != nil || after.GetResult() == nil || !proto.Equal(mount.GetResult().ElementRevision, after.GetResult().ElementRevision) || mount.GetResult().GetState() != after.GetResult().GetState() || mount.GetResult().GetCarrierCore() != after.GetResult().GetCarrierCore() || mount.GetResult().GetNextHop() != after.GetResult().GetNextHop() {
		t.Fatal("reservation/cancellation changed mount truth", after, err)
	}
	// Cancellation releases the slot with a fresh comparison revision. It does
	// not undo history or permit a new operation using the stale original CAS.
	if proto.Equal(mount.GetResult().MountRevision, after.GetResult().MountRevision) {
		t.Fatal("cancel reused old mount revision")
	}
	stale := proto.Clone(q).(*pb.BeginCarrierTransitionRequest)
	stale.OperationId = []byte("stale-reservation")
	if r, err := f.client.Transitions().BeginCarrierTransition(ctx, stale); err != nil || r.GetError() != netpb.Failure_CONFLICT {
		t.Fatal("stale CAS accepted", r, err)
	}
}
