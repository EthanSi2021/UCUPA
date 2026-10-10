package transitionwire

import (
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func TestCarrierBindingHandlesDeclaredFieldBounds(t *testing.T) {
	b, _ := carrierValues(t)
	w, err := CarrierBindingWire(b)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	w.Command.ParentAddress = proto.String("https://" + host + ":65533")
	w.Command.SourceAddress = proto.String("https://" + host + ":65534")
	w.Command.TargetAddress = proto.String("https://" + host + ":65535")
	w.Command.OperationId = make([]byte, 128)
	actual, err := wire.CarrierBinding(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CarrierBindingWire(actual); err != nil {
		t.Fatal("valid maximum command cannot be wrapped", err)
	}
	w.ProtoReflect().SetUnknown(make([]byte, 4097))
	if _, err := wire.CarrierBinding(w); err == nil {
		t.Fatal("oversized command accepted")
	}
}

// Finite codec values only; actual three-owner persistence is exercised by
// tests/transfer. The codec neither authenticates nor manufactures authority.
func carrierValues(t *testing.T) (contract.CarrierApprovalBinding, fact.CarrierControl) {
	t.Helper()
	parent, source, target := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001"), values.FactAuthorityRef("00000000-0000-4000-8000-000000000002"), values.FactAuthorityRef("00000000-0000-4000-8000-000000000003")
	id, scope := values.ElementID("00000000-0000-4000-8000-000000000004"), [32]byte{7}
	e, _ := values.PublicTokenFromBytes(parent, "record-revision", [32]byte{8})
	m, _ := values.PublicTokenFromBytes(parent, "mount-revision", [32]byte{9})
	b := contract.CarrierApprovalBinding{Parent: parent, Operation: contract.OperationID(string([]byte{255, 0, 128})), Command: contract.CarrierCommand{Source: source, Target: target, Mount: id, ExpectedElement: e, ExpectedMount: m, ParentAddress: "local:parent", SourceAddress: "local:source", TargetAddress: "local:target"}}
	i := fact.CarrierIntent{Operation: b.Operation, Parent: parent, Source: source, Target: target, Mount: id,
		Actor:           values.PrincipalRef{Realm: values.RealmID(string([]byte{255, 0, 128})), Principal: values.PrincipalID(string([]byte{128, 0, 255}))},
		ExpectedElement: token.RevisionFromParts(scope, id, 2), ExpectedMount: token.MountRevisionFromParts(scope, id, 9), ParentAddress: b.Command.ParentAddress, SourceAddress: b.Command.SourceAddress, TargetAddress: b.Command.TargetAddress}
	k := contract.MountKey{ParentAuthority: parent, CarrierAuthority: source, MountAnchorUUID: id, ProposalRevision: token.MountRevisionFromParts(scope, id, 3)}
	r, _ := contract.NewRotation(1, 0, 0, 0)
	p, _ := contract.NewPose(contract.Vec3{}, r)
	sphere, _ := contract.NewBoundedSphere(20)
	c := fact.CarrierControl{Reservation: fact.CarrierReservation{Input: i, Previous: contract.MountAdmissionKey{MountKey: k, OperationID: "previous"}, PreviousContact: token.NextHopRevisionFromParts(scope, k, false, 3),
		Tuple: contract.SpatialTuple{Parent: values.ElementID(parent), LocalPose: p, Range: sphere}, NextKey: contract.MountKey{ParentAuthority: parent, CarrierAuthority: target, MountAnchorUUID: id, ProposalRevision: token.MountRevisionFromParts(scope, id, 10)}, At: token.FromParts(scope, 10)}}
	if !c.Valid() {
		t.Fatal("invalid fixture")
	}
	return b, c
}

func TestCarrierPlanRoundTripPreservesOriginalIdentityAndMilestones(t *testing.T) {
	b, c := carrierValues(t)
	for _, mode := range []string{"reserved", "permitted", "cancelled", "refused"} {
		t.Run(mode, func(t *testing.T) {
			v := c
			if mode == "permitted" || mode == "refused" {
				v.PermitAt = token.FromParts([32]byte{7}, 11)
			}
			if mode == "cancelled" || mode == "refused" {
				v.CancelledAt = token.FromParts([32]byte{7}, 12)
			}
			if mode == "refused" {
				v.Refusal = fact.CarrierRefusal{Key: v.Reservation.Input.Key(), Source: b.Command.Source, ReservationAt: v.Reservation.At, PermitAt: v.PermitAt, At: token.FromParts([32]byte{8}, 5), Revision: 1}
			}
			w, err := CarrierPlanWire(b, v)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := proto.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			out := &pb.CarrierPlan{}
			if err := proto.Unmarshal(raw, out); err != nil {
				t.Fatal(err)
			}
			if _, err := wire.ToValue(out.ProtoReflect(), 0); err != nil {
				t.Fatal(err)
			}
			if got, err := CarrierPlan(b, out); err != nil || got != v {
				t.Fatal("round trip", err)
			}
		})
	}
	w, err := CarrierPlanWire(b, c)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*pb.CarrierPlan){
		"input":             func(w *pb.CarrierPlan) { w.Binding.Command.OperationId = []byte("other") },
		"public comparison": func(w *pb.CarrierPlan) { w.Binding.Command.ExpectedElement.Value[0]++ },
		"actor":             func(w *pb.CarrierPlan) { w.Reservation.Actor = nil },
		"comparison scope":  func(w *pb.CarrierPlan) { w.Reservation.ExpectedMount.Incarnation[0]++ },
		"contact scope":     func(w *pb.CarrierPlan) { w.Reservation.PreviousContact.Incarnation[0]++ },
		"stale contact":     func(w *pb.CarrierPlan) { w.Reservation.PreviousContact.Sequence = proto.Uint64(10) },
		"wrong key":         func(w *pb.CarrierPlan) { w.Reservation.NextKey = w.Reservation.Previous.Key },
		"stale permit":      func(w *pb.CarrierPlan) { w.PermittedAt = w.Reservation.At },
		"refusal without permit": func(w *pb.CarrierPlan) {
			w.Refusal = &pb.ServiceCarrierRefusal{At: w.Reservation.At, Revision: proto.Uint64(1)}
		},
		"permit cancelled without refusal": func(w *pb.CarrierPlan) {
			w.PermittedAt = wire.MountCutWire(token.FromParts([32]byte{7}, 11))
			w.CancelledAt = wire.MountCutWire(token.FromParts([32]byte{7}, 12))
		},
		"oversize": func(w *pb.CarrierPlan) { w.Reservation.Actor.Principal = make([]byte, MaxCarrierEvidenceBytes+1) },
	} {
		t.Run(name, func(t *testing.T) {
			v := proto.Clone(w).(*pb.CarrierPlan)
			mutate(v)
			if _, err := CarrierPlan(b, v); err == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
}

func TestCarrierFenceRoundTripPreservesPermitAndDigest(t *testing.T) {
	b, c := carrierValues(t)
	c.PermitAt = token.FromParts([32]byte{7}, 11)
	f := fact.CarrierFence{Permit: c, FinalCut: token.FromParts([32]byte{8}, 7), ManifestDigest: [32]byte{3}, Revision: 2}
	bw, err := CarrierBindingWire(b)
	if err != nil {
		t.Fatal(err)
	}
	w, err := CarrierFenceWire(b, f)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := CarrierFence(b, w); err != nil || got != f {
		t.Fatal("fence round trip", err)
	}
	a := fact.CarrierAcceptance{Fence: f, At: token.FromParts([32]byte{9}, 4), Revision: 5}
	aw, err := CarrierAcceptanceWire(bw, a)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := CarrierAcceptance(b, aw); err != nil || got != a {
		t.Fatal("acceptance round trip", err)
	}
	d := fact.CarrierBinding{Acceptance: a, At: token.FromParts([32]byte{7}, 12), Revision: 6}
	completion := fact.CoreCompletion{Installed: fact.CoreInstallation{Carrier: fact.CarrierInstallation{Binding: d, At: token.FromParts([32]byte{9}, 6), Revision: 7}}, At: token.FromParts([32]byte{9}, 8), Revision: 9}
	cw, err := CarrierCompletionWire(bw, completion)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := CarrierCompletion(b, cw); err != nil || got != completion {
		t.Fatal("completion round trip", err)
	}
	for name, mutate := range map[string]func(*pb.ServiceCarrierFence){
		"wrong binding": func(w *pb.ServiceCarrierFence) { w.Permit.Binding.Command.SourceAddress = proto.String("local:other") },
		"no permit":     func(w *pb.ServiceCarrierFence) { w.Permit.PermittedAt = nil },
		"short digest":  func(w *pb.ServiceCarrierFence) { w.ManifestDigest = []byte{3} },
		"empty digest":  func(w *pb.ServiceCarrierFence) { w.ManifestDigest = make([]byte, 32) },
		"zero revision": func(w *pb.ServiceCarrierFence) { w.Revision = proto.Uint64(0) },
		"cancelled": func(w *pb.ServiceCarrierFence) {
			w.Permit.CancelledAt = wire.MountCutWire(token.FromParts([32]byte{7}, 12))
		},
		"parent scope": func(w *pb.ServiceCarrierFence) { w.FinalCut.Incarnation = w.Permit.PermittedAt.Incarnation },
		"oversize": func(w *pb.ServiceCarrierFence) {
			w.Permit.Reservation.Actor.Principal = make([]byte, MaxCarrierEvidenceBytes+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := proto.Clone(w).(*pb.ServiceCarrierFence)
			mutate(v)
			if _, err := CarrierFence(b, v); err == nil {
				t.Fatal("invalid fence accepted")
			}
		})
	}
}

func TestCarrierDecisionRequiresBoundThreePartyCuts(t *testing.T) {
	b, c := carrierValues(t)
	c.PermitAt = token.FromParts([32]byte{7}, 11)
	d := fact.CarrierBinding{Acceptance: fact.CarrierAcceptance{Fence: fact.CarrierFence{Permit: c, FinalCut: token.FromParts([32]byte{8}, 7), ManifestDigest: [32]byte{1}, Revision: 3}, At: token.FromParts([32]byte{9}, 4), Revision: 5}, At: token.FromParts([32]byte{7}, 12), Revision: 6}
	w, err := CarrierDecisionWire(b, d)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := CarrierDecision(b, w); err != nil || got != d {
		t.Fatal("decision round trip", err)
	}
	for name, mutate := range map[string]func(*pb.CarrierDecision){
		"wrong binding":       func(w *pb.CarrierDecision) { w.Plan.Binding.Command.TargetAddress = proto.String("local:other") },
		"no permit":           func(w *pb.CarrierDecision) { w.Plan.PermittedAt = nil },
		"short digest":        func(w *pb.CarrierDecision) { w.ManifestDigest = []byte{1} },
		"empty digest":        func(w *pb.CarrierDecision) { w.ManifestDigest = make([]byte, 32) },
		"fence revision":      func(w *pb.CarrierDecision) { w.FenceRevision = nil },
		"acceptance revision": func(w *pb.CarrierDecision) { w.AcceptanceRevision = proto.Uint64(0) },
		"decision revision":   func(w *pb.CarrierDecision) { w.DecisionRevision = nil },
		"source parent scope": func(w *pb.CarrierDecision) { w.FinalCut.Incarnation = w.DecidedAt.Incarnation },
		"source target scope": func(w *pb.CarrierDecision) { w.AcceptedAt.Incarnation = w.FinalCut.Incarnation },
		"wrong controller":    func(w *pb.CarrierDecision) { w.DecidedAt.Incarnation = w.FinalCut.Incarnation },
		"stale decision":      func(w *pb.CarrierDecision) { w.DecidedAt.Sequence = proto.Uint64(11) },
	} {
		t.Run(name, func(t *testing.T) {
			v := proto.Clone(w).(*pb.CarrierDecision)
			mutate(v)
			if _, err := CarrierDecision(b, v); err == nil {
				t.Fatal("invalid decision accepted")
			}
		})
	}
}
