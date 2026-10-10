package acceptance_test

import (
	"context"
	"crypto/x509"
	"testing"
	corecontract "ucupa/ucupa-core/contract"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	coresdk "ucupa/ucupa-core/sdk/go"
)

func TestNetworkCorePublicClosure(t *testing.T) {
	f := newNetworkFixture(t, true)
	runNetworkClosure(t, f)
}

func runNetworkClosure(t *testing.T, f *networkFixture) {
	t.Helper()
	ctx := context.Background()
	api := f.client.Core()
	handle := f.enter(t, corepb.Intent_READ)
	initial, err := api.ListDirect(ctx, &corepb.ListDirectRequest{Handle: f.enter(t, corepb.Intent_LIST), Address: &networkpb.Address{}, PrepareObserve: proto.Bool(true)})
	if err != nil || initial.GetResult() == nil || len(initial.GetResult().Records) != 0 {
		t.Fatalf("Read: %v %v", initial, err)
	}
	begin, err := api.BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: initial.GetResult().Preparation})
	if err != nil || begin.GetResult() == nil {
		t.Fatalf("Begin: %v %v", begin, err)
	}
	create := &corepb.CreateEntityRequest{Handle: f.enter(t, corepb.Intent_CREATE_ENTITY), Element: proto.String(networkEntity), OperationId: []byte("create-entity"), Final: networkTuple(1)}
	created, err := api.CreateEntity(ctx, create)
	if err != nil || created.GetResult() == nil {
		t.Fatalf("Create: %v %v", created, err)
	}
	replayed, err := api.CreateEntity(ctx, create)
	if err != nil || !proto.Equal(created, replayed) {
		t.Fatalf("exact replay differs: %v %v", replayed, err)
	}
	query, err := api.QueryRegion(ctx, &corepb.QueryRegionRequest{Handle: handle, Start: &networkpb.Address{}, Basis: &networkpb.Address{}, Sphere: &corepb.Sphere{Center: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(2)}, UpDepth: proto.Int64(-1)})
	if err != nil || query.GetResult() == nil || len(query.GetResult().Matches) != 2 {
		t.Fatalf("Query: %v %v", query, err)
	}
	for _, match := range query.GetResult().Matches {
		read, err := api.ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: match.Address, Basis: query.GetResult().Receipt})
		if err != nil || read.GetResult().GetRecord() == nil || !proto.Equal(read.GetResult().Record.Revision, match.Revision) {
			t.Fatalf("same-basis exact read: %v %v", read, err)
		}
	}
	list, err := api.ListDirect(ctx, &corepb.ListDirectRequest{Handle: f.enter(t, corepb.Intent_LIST), Address: &networkpb.Address{}})
	if err != nil || list.GetResult() == nil || len(list.GetResult().Records) != 1 {
		t.Fatalf("List: %v %v", list, err)
	}
	update, err := api.UpdateEntity(ctx, &corepb.UpdateEntityRequest{Handle: f.enter(t, corepb.Intent_UPDATE_ENTITY), Element: proto.String(networkEntity), Expected: created.GetResult().GetRecord().Revision, OperationId: []byte("update-entity"), Final: networkTuple(2)})
	if err != nil || update.GetResult() == nil {
		t.Fatalf("Update: %v %v", update, err)
	}
	deleted, err := api.DeleteEntity(ctx, &corepb.DeleteEntityRequest{Handle: f.enter(t, corepb.Intent_DELETE_ENTITY), Element: proto.String(networkEntity), Expected: update.GetResult().GetRecord().Revision, OperationId: []byte("delete-entity")})
	if err != nil || deleted.GetResult() == nil {
		t.Fatalf("Delete: %v %v", deleted, err)
	}
	poll, err := api.PollChanges(ctx, &corepb.PollChangesRequest{Cursor: begin.GetResult().Cursor})
	if err != nil || poll.GetResult() == nil || len(poll.GetResult().Groups) == 0 {
		t.Fatalf("Poll: %v %v", poll, err)
	}
	_, err = api.ReleaseObserve(ctx, &corepb.ReleaseObserveRequest{Context: &corepb.ReleaseObserveRequest_Cursor{Cursor: poll.GetResult().NextCursor}})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := api.Close(ctx, &corepb.CloseRequest{Handle: handle})
	if err != nil || closed.GetResult() == nil || !closed.GetResult().GetClosed() {
		t.Fatalf("Close: %v %v", closed, err)
	}
}

func TestNetworkAuthorityAndRevocation(t *testing.T) {
	f := newNetworkFixture(t, true)
	ctx := context.Background()
	handle := f.enter(t, corepb.Intent_READ)
	f.grant(t, corecontract.PermissionReadElement, values.AnchorClosureScope(f.c.AuthorityRef(), networkRoot), true)
	read, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}})
	if err != nil || read.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("revoked: %v %v", read, err)
	}
	for _, kind := range []string{"pin", "ca", "owner"} {
		t.Run(kind, func(t *testing.T) {
			cfg := f.clientConfig
			switch kind {
			case "pin":
				cfg.SPKISHA256[0] ^= 1
			case "ca":
				cfg.Roots = x509.NewCertPool()
			case "owner":
				cfg.Expected = values.FactAuthorityRef(networkEntity)
			}
			client, e := coresdk.New(cfg)
			if e != nil {
				t.Fatal(e)
			}
			defer client.Close()
			if e = client.Establish(ctx, f.actor.Realm, f.id, f.secret); e == nil {
				t.Fatal("wrong target authenticated")
			}
		})
	}
}

func TestNetworkSessionAndMalformedMessages(t *testing.T) {
	f := newNetworkFixture(t, true)
	ctx := context.Background()
	handle := f.enter(t, corepb.Intent_READ)
	in := &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}}
	in.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	if _, err := f.client.Core().ReadElement(ctx, in); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unknown field: %v", err)
	}
	if _, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing field: %v", err)
	}
	if _, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}}, grpc.WaitForReady(true)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("retry option: %v", err)
	}
	if err := f.client.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("released session: %v", err)
	}
	if err := f.client.Establish(ctx, f.actor.Realm, f.id, f.secret); err != nil {
		t.Fatal(err)
	}
	out, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}})
	if err != nil || out.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("fresh context borrowed handle: %v %v", out, err)
	}
}
