package writer

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	"ucupa/ucupa-core/transport/grpc/internal/admission"
	"ucupa/ucupa-core/transport/grpc/internal/session"
)

const ownerID values.FactAuthorityRef = "00000000-0000-4000-8000-000000000011"

var actor = values.CallerContext{Subject: values.PrincipalRef{Realm: "test", Principal: "operator"}}

type witness struct {
	alive bool
	calls int
}

func (w *witness) BindEntrySession(context.Context, values.CallerContext) (admission.EntrySession, error) {
	if !w.alive {
		return admission.EntrySession{}, values.PublicDenied
	}
	return admission.EntrySession{}, nil
}
func (w *witness) CheckEntrySession(admission.EntrySession) error {
	if !w.alive {
		return values.PublicDenied
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
		ctx = session.WithCaller(ctx, actor)
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
func TestDelegationCannotBecomeWriterApproval(t *testing.T) {
	w := &witness{alive: true}
	s := New(w, ownerID, w)
	ctx := session.WithGrant(session.WithCaller(context.Background(), actor), [32]byte{1})
	if out, err := s.InspectWriter(ctx, &netpb.WriterInspectRequest{}); out != nil || status.Code(err) != codes.PermissionDenied || w.calls != 0 {
		t.Fatalf("%v %v %d", out, err, w.calls)
	}
}
