package transition

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/session"
)

const oldCarrierID values.FactAuthorityRef = "00000000-0000-4000-8000-000000000014"

func carrierRequest() *pb.BeginCarrierTransitionRequest {
	return &pb.BeginCarrierTransitionRequest{OperationId: []byte("\xff\x00intent"), Source: proto.String(string(oldCarrierID)), Target: proto.String(string(targetID)), Mount: proto.String(string(boundary)),
		ExpectedElement: &netpb.Token{Owner: proto.String(string(ownerID)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)},
		ExpectedMount:   &netpb.Token{Owner: proto.String(string(ownerID)), Kind: netpb.TokenKind_MOUNT_REVISION.Enum(), Value: make([]byte, 32)},
		ParentAddress:   proto.String("https://parent.example:443"), SourceAddress: proto.String("https://source.example:443"), TargetAddress: proto.String("https://target.example:443")}
}

func carrierFixture() (*witness, *Service, context.Context) {
	w, s, ctx := fixture()
	w.carrier = contract.CarrierStatus{Parent: ownerID, Source: oldCarrierID, Target: targetID, Operation: w.result.Operation, Mount: boundary,
		Progress: contract.CarrierReserved, Revision: values.PublicToken{Owner: ownerID, Kind: "carrier-revision", Value: values.EncodePublicBytes(make([]byte, 32))}}
	return w, s, ctx
}

func TestCarrierRPCUsesParentSessionAndRetainsFailureMeaning(t *testing.T) {
	for _, mode := range []string{"valid", "missing session", "expired", "read grant", "other owner", "unconfigured", "release after dispatch", "store mutation", "store inspect", "wrong result parent", "wrong result source", "wrong result progress"} {
		t.Run(mode, func(t *testing.T) {
			w, s, ctx := carrierFixture()
			wantCode, calls := codes.OK, 1
			switch mode {
			case "missing session":
				ctx, wantCode, calls = context.Background(), codes.Unauthenticated, 0
			case "expired":
				w.alive, wantCode, calls = false, codes.Unauthenticated, 0
			case "read grant":
				ctx, wantCode, calls = session.WithGrant(ctx, [32]byte{1}), codes.PermissionDenied, 0
			case "other owner":
				s.expected, wantCode, calls = targetID, codes.Unavailable, 0
			case "unconfigured":
				s.owner, wantCode, calls = nil, codes.Unimplemented, 0
			case "release after dispatch":
				w.after = func() { w.alive = false }
				wantCode = codes.Unauthenticated
			case "store mutation", "store inspect":
				w.err = contract.ErrStore
			case "wrong result parent":
				w.carrier.Parent, wantCode = targetID, codes.Internal
			case "wrong result source":
				w.carrier.Source, wantCode = ownerID, codes.Internal
			case "wrong result progress":
				w.carrier.Progress, wantCode = contract.CarrierCommitted, codes.Internal
			}
			var out *pb.CarrierTransitionResponse
			var err error
			if mode == "store inspect" || mode == "other owner" {
				out, err = s.InspectCarrierTransition(ctx, &pb.InspectCarrierTransitionRequest{OperationId: carrierRequest().OperationId, Mount: carrierRequest().Mount})
			} else {
				out, err = s.BeginCarrierTransition(ctx, carrierRequest())
			}
			if status.Code(err) != wantCode || w.calls != calls {
				t.Fatal(out, err, w.calls)
			}
			if calls != 0 && w.caller.Subject.Principal != "operator" {
				t.Fatal("parent caller not preserved", w.caller)
			}
			if mode == "store mutation" && out.GetError() != netpb.Failure_INDETERMINATE || mode == "store inspect" && out.GetError() != netpb.Failure_UNAVAILABLE {
				t.Fatal("lost write uncertainty", out)
			}
		})
	}
}

func TestCarrierRPCRejectsMalformedComparisonsBeforeDispatch(t *testing.T) {
	for _, mutate := range []func(*pb.BeginCarrierTransitionRequest){
		func(r *pb.BeginCarrierTransitionRequest) { r.OperationId = nil },
		func(r *pb.BeginCarrierTransitionRequest) { r.OperationId = make([]byte, 129) },
		func(r *pb.BeginCarrierTransitionRequest) { r.Mount = nil },
		func(r *pb.BeginCarrierTransitionRequest) { r.Source = r.Target },
		func(r *pb.BeginCarrierTransitionRequest) { r.Target = proto.String(string(ownerID)) },
		func(r *pb.BeginCarrierTransitionRequest) { r.ExpectedElement = nil },
		func(r *pb.BeginCarrierTransitionRequest) { r.ExpectedMount.Value = make([]byte, 31) },
		func(r *pb.BeginCarrierTransitionRequest) {
			r.ExpectedElement.Kind = netpb.TokenKind_MOUNT_REVISION.Enum()
		},
		func(r *pb.BeginCarrierTransitionRequest) { r.ExpectedMount.Owner = r.Source },
		func(r *pb.BeginCarrierTransitionRequest) { r.ParentAddress = nil },
		func(r *pb.BeginCarrierTransitionRequest) { r.ProtoReflect().SetUnknown(make([]byte, 4097)) },
	} {
		w, s, ctx := carrierFixture()
		r := carrierRequest()
		mutate(r)
		if _, err := s.BeginCarrierTransition(ctx, r); status.Code(err) != codes.InvalidArgument || w.calls != 0 {
			t.Fatal("malformed request dispatched", err, w.calls)
		}
	}
	w, s, ctx := carrierFixture()
	r := &pb.CancelCarrierTransitionRequest{OperationId: carrierRequest().OperationId, Mount: carrierRequest().Mount,
		Expected: &netpb.Token{Owner: proto.String(string(ownerID)), Kind: netpb.TokenKind_TRANSITION_REVISION.Enum(), Value: make([]byte, 32)}}
	if _, err := s.CancelCarrierTransition(ctx, r); status.Code(err) != codes.InvalidArgument || w.calls != 0 {
		t.Fatal("source comparison used for parent cancellation", err)
	}
	r.Expected.Kind = netpb.TokenKind_CARRIER_REVISION.Enum()
	w.carrier.Progress = contract.CarrierCancelled
	if out, err := s.CancelCarrierTransition(ctx, r); err != nil || out.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_CANCELLED {
		t.Fatal(out, err)
	}
}
