package transitionwire

import (
	"google.golang.org/protobuf/proto"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func CarrierDecision(b contract.CarrierApprovalBinding, w *pb.CarrierDecision) (fact.CarrierBinding, error) {
	bad := func() (fact.CarrierBinding, error) { return fact.CarrierBinding{}, contract.ErrInvalid }
	if w == nil || proto.Size(w) > MaxCarrierEvidenceBytes || len(w.ManifestDigest) != 32 {
		return bad()
	}
	c, err := CarrierPlan(b, w.Plan)
	if err != nil {
		return bad()
	}
	f, err := wire.MountCut(w.FinalCut)
	if err != nil {
		return bad()
	}
	a, err := wire.MountCut(w.AcceptedAt)
	if err != nil {
		return bad()
	}
	d, err := wire.MountCut(w.DecidedAt)
	if err != nil {
		return bad()
	}
	binding := fact.CarrierBinding{Acceptance: fact.CarrierAcceptance{Fence: fact.CarrierFence{Permit: c, FinalCut: f, ManifestDigest: [32]byte(w.ManifestDigest), Revision: w.GetFenceRevision()}, At: a, Revision: w.GetAcceptanceRevision()}, At: d, Revision: w.GetDecisionRevision()}
	if !binding.Valid() {
		return bad()
	}
	return binding, nil
}

func CarrierDecisionWire(b contract.CarrierApprovalBinding, d fact.CarrierBinding) (*pb.CarrierDecision, error) {
	if !d.Valid() {
		return nil, contract.ErrInvalid
	}
	c, err := CarrierPlanWire(b, d.Acceptance.Fence.Permit)
	if err != nil {
		return nil, err
	}
	a := d.Acceptance
	f := a.Fence
	w := &pb.CarrierDecision{Plan: c, FinalCut: wire.MountCutWire(f.FinalCut), ManifestDigest: append([]byte(nil), f.ManifestDigest[:]...), FenceRevision: proto.Uint64(f.Revision),
		AcceptedAt: wire.MountCutWire(a.At), AcceptanceRevision: proto.Uint64(a.Revision), DecidedAt: wire.MountCutWire(d.At), DecisionRevision: proto.Uint64(d.Revision)}
	if actual, err := CarrierDecision(b, w); err != nil || actual != d {
		return nil, contract.ErrInvalid
	}
	return w, nil
}
