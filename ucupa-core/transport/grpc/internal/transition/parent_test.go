package transition

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"testing"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

type parentEvidenceProbe struct {
	port.CarrierParentPeer
	id, allowed, received values.FactAuthorityRef
	result                port.CarrierPlan
	after                 func()
	calls                 int
	checks                []values.FactAuthorityRef
	checkError            error
}

func (p *parentEvidenceProbe) AuthorityRef() values.FactAuthorityRef { return p.id }
func (p *parentEvidenceProbe) CheckTransitionPeer(_ context.Context, peer values.FactAuthorityRef) error {
	p.checks = append(p.checks, peer)
	if p.checkError != nil {
		return p.checkError
	}
	if peer != p.allowed {
		return status.Error(codes.Unauthenticated, "wrong TLS peer")
	}
	return nil
}
func (p *parentEvidenceProbe) ReadCarrierPlan(_ context.Context, peer values.FactAuthorityRef, _ contract.CarrierApprovalBinding) (port.CarrierPlan, error) {
	p.calls++
	p.received = peer
	if p.after != nil {
		p.after()
	}
	return p.result, nil
}

func parentEvidenceFixture(t *testing.T) (*parentEvidenceProbe, *Parent, *pb.ReadCarrierPlanRequest) {
	t.Helper()
	w := &pb.CarrierApprovalBinding{Parent: proto.String(string(ownerID)), Command: carrierRequest()}
	b, err := wire.CarrierBinding(w)
	if err != nil {
		t.Fatal(err)
	}
	scope := [32]byte{7}
	i := fact.CarrierIntent{Operation: b.Operation, Actor: values.PrincipalRef{Realm: "realm", Principal: "caller"}, Parent: b.Parent, Source: b.Command.Source, Target: b.Command.Target, Mount: b.Command.Mount,
		ExpectedElement: token.RevisionFromParts(scope, b.Command.Mount, 2), ExpectedMount: token.MountRevisionFromParts(scope, b.Command.Mount, 9), ParentAddress: b.Command.ParentAddress, SourceAddress: b.Command.SourceAddress, TargetAddress: b.Command.TargetAddress}
	k := contract.MountKey{ParentAuthority: i.Parent, CarrierAuthority: i.Source, MountAnchorUUID: i.Mount, ProposalRevision: token.MountRevisionFromParts(scope, i.Mount, 3)}
	r, _ := contract.NewRotation(1, 0, 0, 0)
	pose, _ := contract.NewPose(contract.Vec3{}, r)
	sphere, _ := contract.NewBoundedSphere(20)
	c := fact.CarrierControl{Reservation: fact.CarrierReservation{Input: i, Previous: contract.MountAdmissionKey{MountKey: k, OperationID: "previous"}, PreviousContact: token.NextHopRevisionFromParts(scope, k, false, 3),
		Tuple: contract.SpatialTuple{Parent: values.ElementID(ownerID), LocalPose: pose, Range: sphere}, NextKey: contract.MountKey{ParentAuthority: i.Parent, CarrierAuthority: i.Target, MountAnchorUUID: i.Mount, ProposalRevision: token.MountRevisionFromParts(scope, i.Mount, 10)}, At: token.FromParts(scope, 10)}}
	if !c.Valid() {
		t.Fatal("invalid finite fixture")
	}
	p := &parentEvidenceProbe{id: ownerID, allowed: oldCarrierID, result: exchange.PlanCarrier(b, c)}
	return p, NewParent(p, ownerID, p), &pb.ReadCarrierPlanRequest{Binding: w}
}

func TestCarrierParentBindsActualPeerAndRechecksSamePeerBeforeDisclosure(t *testing.T) {
	for _, mode := range []string{"source", "target", "outsider", "disabled", "wrong-owner", "stopped", "changed-peer-during-read", "invalid-evidence"} {
		t.Run(mode, func(t *testing.T) {
			p, s, q := parentEvidenceFixture(t)
			want, calls := codes.OK, 1
			switch mode {
			case "target":
				p.allowed = targetID
			case "outsider":
				p.allowed = ownerID
				want, calls = codes.Unauthenticated, 0
			case "disabled":
				s.owner = nil
				want, calls = codes.Unimplemented, 0
			case "wrong-owner":
				p.id = targetID
				want, calls = codes.Unavailable, 0
			case "stopped":
				p.checkError = status.Error(codes.Unavailable, "stopped")
				want, calls = codes.Unavailable, 0
			case "changed-peer-during-read":
				p.after = func() { p.allowed = targetID }
				want = codes.Unauthenticated
			case "invalid-evidence":
				p.result = port.CarrierPlan{}
				want = codes.DataLoss
			}
			r, err := s.ReadCarrierPlan(context.Background(), q)
			if status.Code(err) != want || p.calls != calls {
				t.Fatal(r, err, p.calls)
			}
			if want == codes.OK && (r.GetResult() == nil || p.received != p.allowed) {
				t.Fatal("wrong actual peer or no plan")
			}
			if mode == "changed-peer-during-read" && (r != nil || p.received != oldCarrierID || len(p.checks) != 2 || p.checks[1] != oldCarrierID) {
				t.Fatal("changed identity selected again for disclosure", p.checks)
			}
			if mode == "stopped" && len(p.checks) != 1 {
				t.Fatal("availability failure retried as another identity")
			}
		})
	}
}
