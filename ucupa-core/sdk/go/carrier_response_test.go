package coresdk

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func carrierSDKFixture() (*Client, *pb.BeginCarrierTransitionRequest, *pb.CarrierTransitionResponse) {
	parent := values.FactAuthorityRef("00000000-0000-4000-8000-000000000011")
	q := &pb.BeginCarrierTransitionRequest{OperationId: []byte{255, 0, 128}, Source: proto.String("00000000-0000-4000-8000-000000000012"),
		Target: proto.String("00000000-0000-4000-8000-000000000013"), Mount: proto.String("00000000-0000-4000-8000-000000000014"),
		ExpectedElement: &netpb.Token{Owner: proto.String(string(parent)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)},
		ExpectedMount:   &netpb.Token{Owner: proto.String(string(parent)), Kind: netpb.TokenKind_MOUNT_REVISION.Enum(), Value: make([]byte, 32)},
		ParentAddress:   proto.String("https://parent.example:443"), SourceAddress: proto.String("https://source.example:443"), TargetAddress: proto.String("https://target.example:443")}
	r := &pb.CarrierTransitionResponse{Outcome: &pb.CarrierTransitionResponse_Result{Result: &pb.CarrierStatus{
		Parent: proto.String(string(parent)), Source: q.Source, Target: q.Target, OperationId: q.OperationId, Mount: q.Mount, Progress: pb.CarrierProgress_CARRIER_RESERVED.Enum(),
		Revision: &netpb.Token{Owner: proto.String(string(parent)), Kind: netpb.TokenKind_CARRIER_REVISION.Enum(), Value: make([]byte, 32)},
	}}}
	return &Client{config: Config{Expected: parent, RPCDeadline: time.Second, MaxResponseBytes: 4096}, session: "original-session"}, q, r
}

// Exercise the actual interceptor rather than calling only the new validators.
func carrierSDKCall(t *testing.T, c *Client, method string, q proto.Message, reply *pb.CarrierTransitionResponse) (int, error) {
	t.Helper()
	calls := 0
	err := c.intercept(context.Background(), method, q, &pb.CarrierTransitionResponse{}, nil,
		func(ctx context.Context, _ string, _, out any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			calls++
			md, _ := metadata.FromOutgoingContext(ctx)
			if got := md.Get("ucupa-target"); len(got) != 1 || got[0] != string(c.config.Expected) {
				t.Fatal("request not addressed to parent", md)
			}
			if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer original-session" {
				t.Fatal("caller session not used", md)
			}
			proto.Merge(out.(*pb.CarrierTransitionResponse), reply)
			return nil
		}, grpc.StaticMethod())
	return calls, err
}

func TestCarrierSDKRejectsMalformedRequestBeforeSending(t *testing.T) {
	for name, mutate := range map[string]func(*pb.BeginCarrierTransitionRequest){
		"missing id":         func(q *pb.BeginCarrierTransitionRequest) { q.OperationId = nil },
		"oversize id":        func(q *pb.BeginCarrierTransitionRequest) { q.OperationId = make([]byte, 129) },
		"missing mount":      func(q *pb.BeginCarrierTransitionRequest) { q.Mount = nil },
		"same parties":       func(q *pb.BeginCarrierTransitionRequest) { q.Target = q.Source },
		"target parent":      func(q *pb.BeginCarrierTransitionRequest) { q.Target = q.ExpectedMount.Owner },
		"source parent":      func(q *pb.BeginCarrierTransitionRequest) { q.Source = q.ExpectedMount.Owner },
		"missing comparison": func(q *pb.BeginCarrierTransitionRequest) { q.ExpectedElement = nil },
		"comparison owner":   func(q *pb.BeginCarrierTransitionRequest) { q.ExpectedMount.Owner = q.Source },
		"comparison kind": func(q *pb.BeginCarrierTransitionRequest) {
			q.ExpectedMount.Kind = netpb.TokenKind_CARRIER_REVISION.Enum()
		},
		"comparison bytes": func(q *pb.BeginCarrierTransitionRequest) { q.ExpectedElement.Value = []byte{1} },
		"missing address":  func(q *pb.BeginCarrierTransitionRequest) { q.ParentAddress = nil },
		"oversize":         func(q *pb.BeginCarrierTransitionRequest) { q.ProtoReflect().SetUnknown(make([]byte, 4097)) },
	} {
		t.Run(name, func(t *testing.T) {
			c, q, r := carrierSDKFixture()
			mutate(q)
			if calls, err := carrierSDKCall(t, c, pb.TransitionControlService_BeginCarrierTransition_FullMethodName, q, r); calls != 0 || status.Code(err) != codes.InvalidArgument {
				t.Fatal("invalid request sent", calls, err)
			}
		})
	}
}

func TestCarrierSDKPinsResultToParentAndOriginalRequest(t *testing.T) {
	for name, mutate := range map[string]func(*pb.CarrierStatus){
		"valid":            func(*pb.CarrierStatus) {},
		"parent":           func(r *pb.CarrierStatus) { r.Parent = r.Source },
		"source":           func(r *pb.CarrierStatus) { r.Source = r.Target },
		"target":           func(r *pb.CarrierStatus) { r.Target = r.Mount },
		"raw operation":    func(r *pb.CarrierStatus) { r.OperationId = []byte{255, 0, 129} },
		"mount":            func(r *pb.CarrierStatus) { r.Mount = r.Target },
		"missing parent":   func(r *pb.CarrierStatus) { r.Parent = nil },
		"missing progress": func(r *pb.CarrierStatus) { r.Progress = nil },
		"unknown progress": func(r *pb.CarrierStatus) { r.Progress = pb.CarrierProgress(99).Enum() },
		"wrong progress":   func(r *pb.CarrierStatus) { r.Progress = pb.CarrierProgress_CARRIER_COMMITTED.Enum() },
		"missing revision": func(r *pb.CarrierStatus) { r.Revision = nil },
		"revision owner":   func(r *pb.CarrierStatus) { r.Revision.Owner = r.Source },
		"revision kind":    func(r *pb.CarrierStatus) { r.Revision.Kind = netpb.TokenKind_TRANSITION_REVISION.Enum() },
		"revision bytes":   func(r *pb.CarrierStatus) { r.Revision.Value = []byte{1} },
		"unknown":          func(r *pb.CarrierStatus) { r.ProtoReflect().SetUnknown([]byte{0xa0, 6, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			c, q, r := carrierSDKFixture()
			mutate(r.GetResult())
			calls, err := carrierSDKCall(t, c, pb.TransitionControlService_BeginCarrierTransition_FullMethodName, q, r)
			want := codes.DataLoss
			if name == "valid" {
				want = codes.OK
			}
			if calls != 1 || status.Code(err) != want {
				t.Fatal(calls, err)
			}
		})
	}
}

func TestCarrierSDKInspectCancelAndFailureResponses(t *testing.T) {
	c, q, r := carrierSDKFixture()
	inspect := &pb.InspectCarrierTransitionRequest{OperationId: q.OperationId, Mount: q.Mount}
	for _, progress := range []pb.CarrierProgress{pb.CarrierProgress_CARRIER_RESERVED, pb.CarrierProgress_CARRIER_PERMITTED, pb.CarrierProgress_CARRIER_CANCELLED, pb.CarrierProgress_CARRIER_COMMITTED, pb.CarrierProgress_CARRIER_SETTLED} {
		r.GetResult().Progress = progress.Enum()
		if _, err := carrierSDKCall(t, c, pb.TransitionControlService_InspectCarrierTransition_FullMethodName, inspect, r); err != nil {
			t.Fatal(err)
		}
	}
	cancel := &pb.CancelCarrierTransitionRequest{OperationId: q.OperationId, Mount: q.Mount, Expected: r.GetResult().Revision}
	if _, err := carrierSDKCall(t, c, pb.TransitionControlService_CancelCarrierTransition_FullMethodName, cancel, r); status.Code(err) != codes.DataLoss {
		t.Fatal("cancel accepted settled", err)
	}
	r.GetResult().Progress = pb.CarrierProgress_CARRIER_CANCELLED.Enum()
	if _, err := carrierSDKCall(t, c, pb.TransitionControlService_CancelCarrierTransition_FullMethodName, cancel, r); err != nil {
		t.Fatal(err)
	}
	cancel.Expected = q.ExpectedElement
	if calls, err := carrierSDKCall(t, c, pb.TransitionControlService_CancelCarrierTransition_FullMethodName, cancel, r); calls != 0 || status.Code(err) != codes.InvalidArgument {
		t.Fatal(calls, err)
	}
	for _, failure := range []netpb.Failure{netpb.Failure_DENIED, netpb.Failure_UNAVAILABLE, netpb.Failure_INDETERMINATE} {
		r.Outcome = &pb.CarrierTransitionResponse_Error{Error: failure}
		if _, err := carrierSDKCall(t, c, pb.TransitionControlService_BeginCarrierTransition_FullMethodName, q, r); err != nil {
			t.Fatal(err)
		}
	}
	c.session = ""
	if calls, err := carrierSDKCall(t, c, pb.TransitionControlService_BeginCarrierTransition_FullMethodName, q, r); calls != 0 || status.Code(err) != codes.Unauthenticated {
		t.Fatal(calls, err)
	}
}
