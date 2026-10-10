package acceptance_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func verifyNestedProductObservation(t *testing.T, entry, source *networkFixture, cursor *netpb.Token) {
	t.Helper()
	ctx := context.Background()
	source.connect(t)
	ref := &netpb.Reference{Boundary: &netpb.Boundary{Core: proto.String(string(source.config.Expected)), Anchor: proto.String(string(nestedWritableAnchor))}, NextHop: proto.String(string(source.config.NextHop))}
	entered, err := source.client.Core().Enter(ctx, &pb.EnterRequest{Reference: ref, Intent: pb.Intent_CREATE_ENTITY.Enum()})
	if err != nil || entered.GetResult() == nil {
		t.Fatal("nested product enter", entered, err)
	}
	tuple := networkTuple(1)
	tuple.Parent = proto.String(string(nestedWritableAnchor))
	command := &pb.CreateEntityRequest{Handle: entered.GetResult().Handle, Element: proto.String("00000000-0000-4000-8000-000000000007"), OperationId: []byte("nested-product-observed-create"), Final: tuple}
	created, err := source.client.Core().CreateEntity(ctx, command)
	if err != nil || created.GetResult() == nil {
		t.Fatal("nested product write", created, err)
	}
	repeated, err := source.client.Core().CreateEntity(ctx, command)
	if err != nil || !proto.Equal(created, repeated) {
		t.Fatal("nested write retry", repeated, err)
	}
	request := &pb.PollChangesRequest{Cursor: cursor, Limit: proto.Uint32(1)}
	page, err := entry.client.Core().PollChanges(ctx, request)
	if err != nil || page.GetResult() == nil || len(page.GetResult().Groups) != 1 {
		t.Fatal("nested product observe", page, err)
	}
	group := page.GetResult().Groups[0]
	if group.GetOwner() != string(source.config.Expected) || group.ChangeId.GetOwner() != group.GetOwner() || page.GetResult().NextCursor.GetOwner() != string(entry.config.Expected) {
		t.Fatal("lost nested provenance", page)
	}
	retry, err := entry.client.Core().PollChanges(ctx, request)
	if err != nil || !proto.Equal(page, retry) {
		t.Fatal("nested observation retry", retry, err)
	}
}
