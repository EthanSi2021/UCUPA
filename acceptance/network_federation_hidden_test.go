package acceptance_test

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	realm "ucupa/ucupa-realm/contract"
)

func TestNetworkFederationHiddenClosureAndQuietObserve(t *testing.T) {
	a, b, leaf := publicFederationNetworkFixture(t)
	ctx := context.Background()
	m := values.ElementID(networkEntity)
	binding := realm.PermissionBinding{Subject: a.actor, Permission: corecontract.PermissionQueryRegion, Scope: values.AnchorClosureScope(b.config.Expected, m)}
	if _, err := b.r.DeleteBinding(b.actor, binding, b.r.BindingSetRevision(), "hide-query"); err != nil {
		t.Fatal(err)
	}
	root := &netpb.Address{Origin: proto.String(string(networkRoot)), Up: proto.Uint64(0), Down: &netpb.ElementPath{}}
	request := &pb.QueryRegionRequest{Handle: a.enter(t, pb.Intent_QUERY), Start: root, Basis: root, Sphere: &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)}, PrepareObserve: proto.Bool(true), Limit: proto.Uint32(1)}
	first, err := a.client.Core().QueryRegion(ctx, request)
	if err != nil || first.GetResult() == nil || first.GetResult().Preparation == nil || first.GetResult().Continuation == nil {
		t.Fatal(first, err)
	}
	begin, err := a.client.Core().BeginObserve(ctx, &pb.BeginObserveRequest{Preparation: first.GetResult().Preparation})
	if err != nil || begin.GetResult() == nil {
		t.Fatal(begin, err)
	}
	poll := &pb.PollChangesRequest{Cursor: begin.GetResult().Cursor, Limit: proto.Uint32(1)}
	before, err := a.client.Core().PollChanges(ctx, poll)
	if err != nil || before.GetResult() == nil || len(before.GetResult().Groups) != 0 {
		t.Fatal(before, err)
	}
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	radius, _ := corecontract.NewBoundedSphere(1)
	if _, err := b.c.Elements().CreateEntity(b.actor, values.ElementID("00000000-0000-4000-8000-000000000009"), corecontract.SpatialTuple{Parent: m, LocalPose: corecontract.Pose{Rotation: rotation}, Range: radius}, "hidden-change"); err != nil {
		t.Fatal(err)
	}
	after, err := a.client.Core().PollChanges(ctx, poll)
	if err != nil || !proto.Equal(before, after) {
		t.Fatal("hidden change leaked through observation", before, after, err)
	}
	next := proto.Clone(request).(*pb.QueryRegionRequest)
	next.Continuation = first.GetResult().Continuation
	last, err := a.client.Core().QueryRegion(ctx, next)
	if err != nil || last.GetResult() == nil || last.GetResult().GetStatus() != pb.Coverage_COMPLETE || last.GetResult().Gap != nil || !bytes.Equal(last.GetResult().Closure, first.GetResult().Closure) {
		t.Fatal("denied branch did not close across hidden writes", last, err)
	}
	for _, response := range []*pb.QueryRegionResponse{first, last} {
		graph := response.GetResult().SourceGraph
		if graph == nil || len(graph.Sources) != 1 || graph.Sources[0].GetOwner() != string(a.config.Expected) {
			t.Fatal("hidden source vector disclosed", graph)
		}
		encoded, _ := proto.Marshal(response)
		if bytes.Contains(encoded, []byte(string(b.config.Expected))) {
			t.Fatal("hidden owner leaked in public response")
		}
		for _, match := range response.GetResult().Matches {
			if match.GetElement() == string(leaf) || match.Revision.GetOwner() == string(b.config.Expected) {
				t.Fatal("hidden facts disclosed", match)
			}
		}
	}
	if !proto.Equal(first.GetResult().SourceGraph, last.GetResult().SourceGraph) {
		t.Fatal("hidden write changed source graph")
	}
	if _, err := b.r.PutBinding(b.actor, binding, b.r.BindingSetRevision(), "unhide-query"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.r.DeleteBinding(b.actor, binding, b.r.BindingSetRevision(), "hide-query-again"); err != nil {
		t.Fatal(err)
	}
	stale, err := a.client.Core().PollChanges(ctx, poll)
	if err != nil || stale.GetResult() != nil {
		t.Fatal("grant/revoke ABA preserved hidden observation", stale, err)
	}
	b.server.Stop()
	offline := proto.Clone(request).(*pb.QueryRegionRequest)
	offline.PrepareObserve = proto.Bool(false)
	offline.Limit = proto.Uint32(100)
	unknown, err := a.client.Core().QueryRegion(ctx, offline)
	if err != nil || unknown.GetResult() == nil || unknown.GetResult().GetStatus() != pb.Coverage_PARTIAL || unknown.GetResult().GetGap() != pb.QueryGap_UNRESOLVED_COVERAGE {
		t.Fatal("offline treated as proven denial", unknown, err)
	}
}
