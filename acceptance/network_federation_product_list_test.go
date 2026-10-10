package acceptance_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Both owners are independent product processes by the time this helper runs.
// List, same-cut Read, preparation, Begin and Poll all enter through public RPC.
func verifyFederatedProductList(t *testing.T, entry, carrier *networkFixture) (*pb.ListDirectRequest, func()) {
	t.Helper()
	ctx := context.Background()
	address := &netpb.Address{Origin: proto.String(string(networkRoot)), Down: &netpb.ElementPath{Items: []string{string(networkEntity)}}}
	request := &pb.ListDirectRequest{Handle: entry.enter(t, pb.Intent_LIST), Address: address, Limit: proto.Uint32(1), PrepareObserve: proto.Bool(true)}
	first, err := entry.client.Core().ListDirect(ctx, request)
	if err != nil || first.GetResult() == nil || first.GetResult().Continuation == nil || first.GetResult().Preparation == nil || len(first.GetResult().Records) != 1 {
		t.Fatal("product first list page", first, err)
	}
	one := first.GetResult()
	request.Continuation = one.Continuation
	second, err := entry.client.Core().ListDirect(ctx, request)
	if err != nil || second.GetResult() == nil || second.GetResult().Continuation != nil || len(second.GetResult().Records) != 1 {
		t.Fatal("product final list page", second, err)
	}
	two := second.GetResult()
	if !proto.Equal(one.SourceGraph, two.SourceGraph) || one.SourceGraph == nil || len(one.SourceGraph.Sources) != 2 || one.SourceGraph.GetOperation() != "list" {
		t.Fatal("list replaced original source graph")
	}
	if one.Records[0].GetElement() >= two.Records[0].GetElement() {
		t.Fatal("list repeated or reordered records")
	}
	for _, r := range []*pb.Record{one.Records[0], two.Records[0]} {
		if r.GetParent() != string(networkEntity) || r.Revision.GetOwner() != string(carrier.config.Expected) {
			t.Fatal("list ownership or parent", r)
		}
	}
	readAddress := proto.Clone(address).(*netpb.Address)
	readAddress.Down.Items = append(readAddress.Down.Items, one.Records[0].GetElement())
	read, err := entry.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: request.Handle, Address: readAddress, Basis: one.Receipt})
	if err != nil || read.GetResult() == nil || read.GetResult().Record.GetElement() != one.Records[0].GetElement() || !proto.Equal(read.GetResult().SourceGraph, one.SourceGraph) {
		t.Fatal("list same-cut independent Read", read, err)
	}
	begin, err := entry.client.Core().BeginObserve(ctx, &pb.BeginObserveRequest{Preparation: one.Preparation})
	if err != nil || begin.GetResult() == nil {
		t.Fatal("list prepared Begin", begin, err)
	}
	poll, err := entry.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: begin.GetResult().Cursor})
	if err != nil || poll.GetResult() == nil || len(poll.GetResult().Groups) != 0 {
		t.Fatal("list empty observation", poll, err)
	}
	return request, func() {
		released, err := entry.client.Core().ReleaseObserve(ctx, &pb.ReleaseObserveRequest{Context: &pb.ReleaseObserveRequest_Preparation{Preparation: one.Preparation}})
		if err != nil || released.GetResult() == nil || !released.GetResult().GetReleased() {
			t.Fatal("release original list after source restart", released, err)
		}
	}
}
