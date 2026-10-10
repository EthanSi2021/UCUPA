package acceptance_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	realm "ucupa/ucupa-realm/contract"
)

func TestNetworkFederationPublicPreparedQueryObservation(t *testing.T) {
	testNetworkFederationPublicObservation(t, false)
}

func TestNetworkFederationPublicPreparedReadObservation(t *testing.T) {
	testNetworkFederationPublicObservation(t, true)
}

func testNetworkFederationPublicObservation(t *testing.T, exact bool) {
	a, b, leaf := publicFederationNetworkFixture(t)
	ctx := context.Background()
	m := values.ElementID(networkEntity)
	binding := realm.PermissionBinding{Subject: a.actor, Permission: corecontract.PermissionObserveElement, Scope: values.AnchorClosureScope(b.c.AuthorityRef(), m)}
	if _, err := b.r.PutBinding(b.actor, binding, b.r.BindingSetRevision(), "public-source-observe"); err != nil {
		t.Fatal(err)
	}
	attachmentBinding := binding
	attachmentBinding.Permission = corecontract.PermissionObserveAttachment
	if _, err := b.r.PutBinding(b.actor, attachmentBinding, b.r.BindingSetRevision(), "public-source-observe-attachment"); err != nil {
		t.Fatal(err)
	}
	intent := pb.Intent_QUERY
	if exact {
		intent = pb.Intent_READ
	}
	handle := a.enter(t, intent)
	root := &netpb.Address{Origin: proto.String(string(networkRoot)), Up: proto.Uint64(0), Down: &netpb.ElementPath{}}
	var prep *netpb.Token
	if exact {
		address := proto.Clone(root).(*netpb.Address)
		address.Down.Items = []string{string(m), string(leaf)}
		response, err := a.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: handle, Address: address, PrepareObserve: proto.Bool(true)})
		if err != nil || response.GetResult() == nil {
			t.Fatal(response, err)
		}
		prep = response.GetResult().Preparation
	} else {
		response, err := a.client.Core().QueryRegion(ctx, &pb.QueryRegionRequest{Handle: handle, Start: root, Basis: root, Sphere: &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)}, PrepareObserve: proto.Bool(true)})
		if err != nil || response.GetResult() == nil {
			t.Fatal(response, err)
		}
		prep = response.GetResult().Preparation
	}
	if prep == nil {
		t.Fatal("missing preparation")
	}
	// A real owner commit occurs after the public query cut and before Begin.
	// All read/observation calls use the public entry's authenticated SDK.
	radius, _ := corecontract.NewBoundedSphere(1)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	if exact {
		b.grant(t, corecontract.PermissionPlaceElement, values.CoreElementScope(b.c.AuthorityRef(), m), false)
		b.grant(t, corecontract.PermissionUpdateElement, values.CoreElementScope(b.c.AuthorityRef(), leaf), false)
		b.grant(t, corecontract.PermissionReadElement, values.CoreElementScope(b.c.AuthorityRef(), leaf), false)
		current, err := b.c.Elements().ReadElement(b.actor, leaf, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.c.Elements().UpdateEntity(b.actor, leaf, corecontract.SpatialTuple{Parent: m, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, current.Record.Entity.Revision, "public-observed-update"); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := b.c.Elements().CreateEntity(b.actor, values.ElementID("00000000-0000-4000-8000-000000000038"), corecontract.SpatialTuple{Parent: m, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "public-observed-write"); err != nil {
			t.Fatal(err)
		}
	}
	begin, err := a.client.Core().BeginObserve(ctx, &pb.BeginObserveRequest{Preparation: prep})
	if err != nil || begin.GetResult() == nil {
		t.Fatal(begin, err)
	}
	poll := &pb.PollChangesRequest{Cursor: begin.GetResult().Cursor, Limit: proto.Uint32(1)}
	page, err := a.client.Core().PollChanges(ctx, poll)
	if err != nil || page.GetResult() == nil || len(page.GetResult().Groups) != 1 {
		t.Fatal(page, err)
	}
	group := page.GetResult().Groups[0]
	if group.GetOwner() != string(b.c.AuthorityRef()) || group.ChangeId.GetOwner() != group.GetOwner() || page.GetResult().NextCursor.GetOwner() != string(a.c.AuthorityRef()) {
		t.Fatal("owner/cursor provenance lost", page)
	}
	repeat, err := a.client.Core().PollChanges(ctx, poll)
	if err != nil || !proto.Equal(page, repeat) {
		t.Fatal("unstable public retry", repeat, err)
	}
	if _, err := b.r.DeleteBinding(b.actor, binding, b.r.BindingSetRevision(), "public-observe-revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.r.PutBinding(b.actor, binding, b.r.BindingSetRevision(), "public-observe-regrant"); err != nil {
		t.Fatal(err)
	}
	denied, err := a.client.Core().PollChanges(ctx, poll)
	if err != nil || denied.GetFailure() != netpb.Failure_GAP {
		t.Fatal("cached retry bypassed original view", denied, err)
	}
	released, err := a.client.Core().ReleaseObserve(ctx, &pb.ReleaseObserveRequest{Context: &pb.ReleaseObserveRequest_Preparation{Preparation: prep}})
	if err != nil || released.GetResult() == nil || !released.GetResult().GetReleased() {
		t.Fatal(released, err)
	}
}
