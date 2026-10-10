package acceptance_test

import (
	"bytes"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
)

// This fixture tests typed adaptation with the real owners. It deliberately
// does not claim a TLS host or remote fixed-service Core connection is tested.
type facetAdapterCaller struct {
	caller values.CallerContext
	err    error
}

func (c *facetAdapterCaller) Caller(context.Context) (values.CallerContext, error) {
	return c.caller, c.err
}

func facetAdapterFixture(t *testing.T) (*publicFacetWorld, *facetgrpc.Service, *facetpb.Access, *facetAdapterCaller) {
	t.Helper()
	w := newPublicFacetWorld(t)
	caller := &facetAdapterCaller{caller: w.caller}
	hop := values.NextHop("https://facet.example:443")
	s, err := facetgrpc.NewService(w.facets[0].Namespace, caller, w.facets[0].AuthorityRef(), hop, 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	k := w.access["key"].(map[string]any)
	p := k["proposal"].(map[string]any)
	b, err := values.DecodePublicBytes(p["value"].(string))
	if err != nil {
		t.Fatal(err)
	}
	access := &facetpb.Access{NextHop: proto.String(string(hop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(k["core"].(string)), Facet: proto.String(k["facet"].(string)), Element: proto.String(k["element"].(string)),
		Proposal: &networkpb.Token{Owner: proto.String(p["owner"].(string)), Kind: networkpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: b},
	}}
	return w, s, access, caller
}

func TestFacetTypedAdapterOwnerClosure(t *testing.T) {
	w, s, access, _ := facetAdapterFixture(t)
	ctx := context.Background()
	read, err := s.Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: w.typ.Bytes(), PrepareObserve: proto.Bool(true)})
	if err != nil || read.GetResult() == nil || read.GetResult().GetState() != facetpb.Presence_NOT_FOUND || read.GetResult().Preparation == nil {
		t.Fatalf("prepare %v %v", read, err)
	}
	put := &facetpb.PutRequest{Access: access, TypeRef: w.typ.Bytes(), Payload: []byte(`"hello"`), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("typed-put")}
	written, err := s.Put(ctx, put)
	if err != nil || written.GetResult() == nil || written.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED {
		t.Fatalf("put %v %v", written, err)
	}
	replayed, err := s.Put(ctx, put)
	if err != nil || !proto.Equal(replayed, written) {
		t.Fatalf("replay %v %v", replayed, err)
	}
	recovered, err := s.RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: access, OperationId: put.OperationId, Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: put}}})
	if err != nil || !proto.Equal(recovered.GetResult(), written.GetResult()) {
		t.Fatalf("recover %v %v", recovered, err)
	}
	changed := proto.Clone(put).(*facetpb.PutRequest)
	changed.Payload = []byte(`"different"`)
	conflict, err := s.RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: access, OperationId: put.OperationId, Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: changed}}})
	if err != nil || conflict.GetFailure() != networkpb.Failure_CONFLICT {
		t.Fatalf("changed recovery %v %v", conflict, err)
	}
	begun, err := s.BeginObserve(ctx, &facetpb.BeginObserveRequest{Preparation: read.GetResult().Preparation})
	if err != nil || begun.GetResult() == nil {
		t.Fatalf("begin %v %v", begun, err)
	}
	batch, err := s.PollChanges(ctx, &facetpb.PollChangesRequest{Cursor: begun.GetResult().Cursor})
	if err != nil || batch.GetResult() == nil || len(batch.GetResult().Groups) != 1 {
		t.Fatalf("poll %v %v", batch, err)
	}
	repeated, err := s.PollChanges(ctx, &facetpb.PollChangesRequest{Cursor: begun.GetResult().Cursor})
	if err != nil || !proto.Equal(batch, repeated) {
		t.Fatalf("poll replay %v %v", repeated, err)
	}
	listed, err := s.ListTypes(ctx, &facetpb.ListTypesRequest{Access: access})
	if err != nil || listed.GetResult() == nil || len(listed.GetResult().Types) != 1 {
		t.Fatalf("list %v %v", listed, err)
	}
	got, err := s.Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: w.typ.Bytes()})
	if err != nil || got.GetResult() == nil || !bytes.Equal(got.GetResult().Payload, put.Payload) {
		t.Fatalf("get %v %v", got, err)
	}
	deleted, err := s.Delete(ctx, &facetpb.DeleteRequest{Access: access, TypeRef: w.typ.Bytes(), Expected: got.GetResult().Revision, OperationId: []byte("typed-delete")})
	if err != nil || deleted.GetResult() == nil {
		t.Fatalf("delete %v %v", deleted, err)
	}
	missing, err := s.Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: w.typ.Bytes()})
	if err != nil || missing.GetResult() == nil || missing.GetResult().GetState() != facetpb.Presence_NOT_FOUND {
		t.Fatalf("missing %v %v", missing, err)
	}
	released, err := s.ReleaseObserve(ctx, &facetpb.ReleaseObserveRequest{Context: &facetpb.ReleaseObserveRequest_Cursor{Cursor: batch.GetResult().NextCursor}})
	if err != nil || !released.GetResult().GetReleased() {
		t.Fatalf("release %v %v", released, err)
	}
}

func TestFacetAdapterDoesNotManufactureCaller(t *testing.T) {
	w, s, access, callers := facetAdapterFixture(t)
	request := &facetpb.GetRequest{Access: access, TypeRef: w.typ.Bytes()}
	callers.err = status.Error(codes.Unauthenticated, "no session")
	if reply, err := s.Get(context.Background(), request); reply != nil || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no session %v %v", reply, err)
	}
	callers.err = nil
	callers.caller = values.CallerContext{}
	if reply, err := s.Get(context.Background(), request); reply != nil || status.Code(err) != codes.Unauthenticated {
		t.Fatalf("empty caller %v %v", reply, err)
	}
	callers.caller = w.caller
	callers.caller.Proof[0] ^= 1
	if reply, err := s.Get(context.Background(), request); err != nil || reply.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("forged caller %v %v", reply, err)
	}
	callers.caller = w.caller
	wrong := proto.Clone(request).(*facetpb.GetRequest)
	wrong.Access.NextHop = proto.String("https://wrong.example:443")
	if reply, err := s.Get(context.Background(), wrong); reply != nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong address %v %v", reply, err)
	}
}
