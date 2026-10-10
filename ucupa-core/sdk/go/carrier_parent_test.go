package coresdk

import (
	"context"
	"crypto/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func parentSDKFixture() (*Client, *pb.CarrierApprovalBinding, *pb.CarrierPlan) {
	c, q, _ := carrierSDKFixture()
	c.config.Actor = values.FactAuthorityRef(q.GetSource())
	c.config.ActorCertificate = &tls.Certificate{}
	c.config.MaxResponseBytes = 32 << 10
	b := &pb.CarrierApprovalBinding{Parent: proto.String(string(c.config.Expected)), Command: q}
	rev := func(n uint64) *pb.ServiceRevision {
		return &pb.ServiceRevision{Incarnation: append([]byte{7}, make([]byte, 31)...), Sequence: proto.Uint64(n)}
	}
	key := &pb.ServiceMountKey{ParentCore: b.Parent, CarrierCore: q.Source, Element: q.Mount, Proposal: rev(3)}
	next := proto.Clone(key).(*pb.ServiceMountKey)
	next.CarrierCore = q.Target
	next.Proposal = rev(10)
	p := &pb.CarrierPlan{Binding: proto.Clone(b).(*pb.CarrierApprovalBinding), Reservation: &pb.ServiceCarrierReservation{
		Actor: &pb.TransitionActor{Realm: []byte{255, 0, 128}, Principal: []byte{128, 0, 255}}, ExpectedElement: rev(2), ExpectedMount: rev(9),
		Previous: &pb.ServiceMountAdmission{Key: key, OperationId: []byte("previous")}, PreviousContact: rev(3), NextKey: next,
		At:    &pb.ServiceCoreCut{Incarnation: append([]byte{7}, make([]byte, 31)...), Sequence: proto.Uint64(10)},
		Tuple: &pb.SpatialTuple{Parent: b.Parent, Pose: &pb.Pose{Translation: &netpb.Vector3{Items: []float64{0, 0, 0}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(1), Ratios: &netpb.Vector3{Items: []float64{0, 0, 0}}}}, Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(20)}},
	}}
	return c, b, p
}

func parentSDKCall(t *testing.T, c *Client, method string, in, out, reply proto.Message) (int, error) {
	t.Helper()
	calls := 0
	err := c.intercept(context.Background(), method, in, out, nil, func(ctx context.Context, _ string, _, response any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		calls++
		md, _ := metadata.FromOutgoingContext(ctx)
		if len(md.Get("authorization")) != 0 {
			t.Fatal("user session leaked into service-evidence read")
		}
		if got := md.Get("ucupa-target"); len(got) != 1 || got[0] != string(c.config.Expected) {
			t.Fatal("wrong parent routing")
		}
		proto.Merge(response.(proto.Message), reply)
		return nil
	}, grpc.StaticMethod())
	return calls, err
}

func TestCarrierParentSDKChecksOriginalBindingAndResponse(t *testing.T) {
	for name, mutate := range map[string]func(*pb.CarrierPlan){
		"valid":         func(*pb.CarrierPlan) {},
		"operation":     func(p *pb.CarrierPlan) { p.Binding.Command.OperationId = []byte("other") },
		"address":       func(p *pb.CarrierPlan) { p.Binding.Command.ParentAddress = proto.String("https://other.example:443") },
		"token":         func(p *pb.CarrierPlan) { p.Binding.Command.ExpectedElement.Value[0]++ },
		"actor":         func(p *pb.CarrierPlan) { p.Reservation.Actor.Principal = nil },
		"size":          func(p *pb.CarrierPlan) { p.Reservation.Actor.Principal = make([]byte, 17000) },
		"missing cut":   func(p *pb.CarrierPlan) { p.Reservation.At = nil },
		"empty scope":   func(p *pb.CarrierPlan) { p.Reservation.PreviousContact.Incarnation = make([]byte, 32) },
		"zero sequence": func(p *pb.CarrierPlan) { p.Reservation.At.Sequence = proto.Uint64(0) },
		"refusal": func(p *pb.CarrierPlan) {
			p.Refusal = &pb.ServiceCarrierRefusal{At: p.Reservation.At, Revision: proto.Uint64(1)}
		},
		"unknown": func(p *pb.CarrierPlan) { p.ProtoReflect().SetUnknown([]byte{0xa0, 6, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			c, b, p := parentSDKFixture()
			mutate(p)
			r := &pb.ReadCarrierPlanResponse{Outcome: &pb.ReadCarrierPlanResponse_Result{Result: p}}
			calls, err := parentSDKCall(t, c, pb.TransitionParentService_ReadCarrierPlan_FullMethodName, &pb.ReadCarrierPlanRequest{Binding: b}, &pb.ReadCarrierPlanResponse{}, r)
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

func TestCarrierParentSDKRejectsRequestWithoutParticipantAuthentication(t *testing.T) {
	for _, mode := range []string{"user", "parent", "other", "wrong-address-owner", "bad-comparison"} {
		t.Run(mode, func(t *testing.T) {
			c, b, p := parentSDKFixture()
			want := codes.Unauthenticated
			switch mode {
			case "user":
				c.config.ActorCertificate = nil
			case "parent":
				c.config.Actor = c.config.Expected
			case "other":
				c.config.Actor = "00000000-0000-4000-8000-000000000099"
			case "wrong-address-owner":
				b.Parent = b.Command.Target
			case "bad-comparison":
				b.Command.ExpectedMount.Value = []byte{1}
				want = codes.InvalidArgument
			}
			r := &pb.ReadCarrierPlanResponse{Outcome: &pb.ReadCarrierPlanResponse_Result{Result: p}}
			calls, err := parentSDKCall(t, c, pb.TransitionParentService_ReadCarrierPlan_FullMethodName, &pb.ReadCarrierPlanRequest{Binding: b}, &pb.ReadCarrierPlanResponse{}, r)
			if calls != 0 || status.Code(err) != want {
				t.Fatal(calls, err)
			}
		})
	}
}

func TestCarrierParentSDKDecisionRequiresCompleteEvidence(t *testing.T) {
	for _, mode := range []string{"valid", "binding", "no-permit", "digest", "revision", "missing-cut", "missing-result"} {
		t.Run(mode, func(t *testing.T) {
			c, b, p := parentSDKFixture()
			cut := func(s byte, n uint64) *pb.ServiceCoreCut {
				return &pb.ServiceCoreCut{Incarnation: append([]byte{s}, make([]byte, 31)...), Sequence: proto.Uint64(n)}
			}
			p.PermittedAt = cut(7, 11)
			d := &pb.CarrierDecision{Plan: p, FinalCut: cut(8, 7), ManifestDigest: append([]byte{1}, make([]byte, 31)...), FenceRevision: proto.Uint64(3), AcceptedAt: cut(9, 4), AcceptanceRevision: proto.Uint64(5), DecidedAt: cut(7, 12), DecisionRevision: proto.Uint64(6)}
			switch mode {
			case "binding":
				d.Plan.Binding.Command.OperationId = []byte("different")
			case "no-permit":
				d.Plan.PermittedAt = nil
			case "digest":
				d.ManifestDigest = []byte{1}
			case "revision":
				d.DecisionRevision = proto.Uint64(0)
			case "missing-cut":
				d.AcceptedAt = nil
			}
			r := &pb.ReadCarrierDecisionResponse{Outcome: &pb.ReadCarrierDecisionResponse_Result{Result: d}}
			if mode == "missing-result" {
				r.Outcome = nil
			}
			calls, err := parentSDKCall(t, c, pb.TransitionParentService_ReadCarrierDecision_FullMethodName, &pb.ReadCarrierDecisionRequest{Binding: b}, &pb.ReadCarrierDecisionResponse{}, r)
			want := codes.DataLoss
			if mode == "valid" {
				want = codes.OK
			}
			if calls != 1 || status.Code(err) != want {
				t.Fatal(calls, err)
			}
		})
	}
}
