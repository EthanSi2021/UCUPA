package writer

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

const ownerID values.FactAuthorityRef = "00000000-0000-4000-8000-000000000011"

var actor = values.CallerContext{Subject: values.PrincipalRef{Realm: "test", Principal: "operator"}}

type witness struct {
	alive bool
	calls int
}

func (w *witness) Caller(context.Context) (values.CallerContext, error) {
	if !w.alive {
		return values.CallerContext{}, status.Error(codes.Unauthenticated, "expired")
	}
	return actor, nil
}
func (w *witness) CheckWriterCaller(ctx context.Context, c values.CallerContext) error {
	if ctx.Err() != nil {
		return status.FromContextError(ctx.Err()).Err()
	}
	if !w.alive || c != actor {
		return status.Error(codes.Unauthenticated, "expired")
	}
	return nil
}
func (w *witness) AuthorityRef() values.FactAuthorityRef { return ownerID }
func (w *witness) InspectWriter(context.Context, values.CallerContext, string) (values.WriterStatus, error) {
	w.calls++
	w.alive = false
	return values.WriterStatus{Owner: ownerID, Epoch: 1}, nil
}
func (w *witness) HandoffWriter(context.Context, values.CallerContext, values.WriterHandoffRequest) (values.WriterStatus, error) {
	w.calls++
	w.alive = false
	return values.WriterStatus{Owner: ownerID, Epoch: 1, Pending: true}, nil
}

func TestWriterReplyRequiresOriginalLiveSession(t *testing.T) {
	for _, write := range []bool{false, true} {
		w := &witness{alive: true}
		s := New(w, ownerID, w)
		ctx := context.Background()

		var out *netpb.WriterResponse
		var err error
		if write {
			out, err = s.HandoffWriter(ctx, &netpb.WriterHandoffRequest{Operation: []byte("handoff"), ExpectedEpoch: proto.Uint64(1), Target: append([]byte{1}, make([]byte, 31)...)})
		} else {
			out, err = s.InspectWriter(ctx, &netpb.WriterInspectRequest{})
		}
		if w.calls != 1 || out != nil || status.Code(err) != codes.Unauthenticated {
			t.Fatalf("disclosure after session expiry: calls=%d out=%v error=%v", w.calls, out, err)
		}
	}
}
