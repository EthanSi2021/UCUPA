package transitionwire

import (
	"google.golang.org/protobuf/proto"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func CarrierFence(b contract.CarrierApprovalBinding, w *pb.ServiceCarrierFence) (fact.CarrierFence, error) {
	bad := func() (fact.CarrierFence, error) { return fact.CarrierFence{}, contract.ErrInvalid }
	if w == nil || len(w.ManifestDigest) != 32 || w.GetRevision() == 0 || proto.Size(w) > MaxCarrierEvidenceBytes {
		return bad()
	}
	c, err := CarrierPlan(b, w.Permit)
	if err != nil {
		return bad()
	}
	cut, err := wire.MountCut(w.FinalCut)
	if err != nil {
		return bad()
	}
	f := fact.CarrierFence{Permit: c, FinalCut: cut, ManifestDigest: [32]byte(w.ManifestDigest), Revision: w.GetRevision()}
	if !f.Valid() || !MatchesCarrier(b, c.Reservation.Input) {
		return bad()
	}
	return f, nil
}

func CarrierFenceWire(b contract.CarrierApprovalBinding, f fact.CarrierFence) (*pb.ServiceCarrierFence, error) {
	permit, err := CarrierPlanWire(b, f.Permit)
	if err != nil || !f.Valid() || !MatchesCarrier(b, f.Permit.Reservation.Input) {
		return nil, contract.ErrInvalid
	}
	w := &pb.ServiceCarrierFence{Permit: permit, FinalCut: wire.MountCutWire(f.FinalCut),
		ManifestDigest: append([]byte(nil), f.ManifestDigest[:]...), Revision: proto.Uint64(f.Revision)}
	if actual, err := CarrierFence(b, w); err != nil || actual != f {
		return nil, contract.ErrInvalid
	}
	return w, nil
}

func CarrierAcceptance(b contract.CarrierApprovalBinding, w *pb.CarrierTargetAcceptance) (fact.CarrierAcceptance, error) {
	bad := func() (fact.CarrierAcceptance, error) { return fact.CarrierAcceptance{}, contract.ErrInvalid }
	if w == nil || proto.Size(w) > MaxCarrierEvidenceBytes {
		return bad()
	}
	actual, err := wire.CarrierBinding(w.Binding)
	if err != nil || actual != b {
		return bad()
	}
	f, err := CarrierFence(b, w.Fence)
	if err != nil {
		return bad()
	}
	at, err := wire.MountCut(w.AcceptedAt)
	if err != nil || w.GetAcceptanceRevision() == 0 {
		return bad()
	}
	a := fact.CarrierAcceptance{Fence: f, At: at, Revision: w.GetAcceptanceRevision()}
	if !a.Valid() {
		return bad()
	}
	return a, nil
}

func CarrierAcceptanceWire(b *pb.CarrierApprovalBinding, a fact.CarrierAcceptance) (*pb.CarrierTargetAcceptance, error) {
	binding, err := wire.CarrierBinding(b)
	if err != nil {
		return nil, contract.ErrInvalid
	}
	f, err := CarrierFenceWire(binding, a.Fence)
	if err != nil || !a.Valid() {
		return nil, contract.ErrInvalid
	}
	w := &pb.CarrierTargetAcceptance{Binding: proto.Clone(b).(*pb.CarrierApprovalBinding), Fence: f,
		AcceptedAt: wire.MountCutWire(a.At), AcceptanceRevision: proto.Uint64(a.Revision)}
	if actual, err := CarrierAcceptance(binding, w); err != nil || actual != a {
		return nil, contract.ErrInvalid
	}
	return w, nil
}

func CarrierCompletion(b contract.CarrierApprovalBinding, w *pb.CarrierCompletion) (fact.CoreCompletion, error) {
	bad := func() (fact.CoreCompletion, error) { return fact.CoreCompletion{}, contract.ErrInvalid }
	if w == nil || proto.Size(w) > MaxCarrierEvidenceBytes {
		return bad()
	}
	d, err := CarrierDecision(b, w.Decision)
	if err != nil {
		return bad()
	}
	installed, err := wire.MountCut(w.InstalledAt)
	if err != nil || w.GetInstallationRevision() == 0 {
		return bad()
	}
	at, err := wire.MountCut(w.CompletedAt)
	if err != nil {
		return bad()
	}
	c := fact.CoreCompletion{Installed: fact.CoreInstallation{Carrier: fact.CarrierInstallation{Binding: d, At: installed, Revision: w.GetInstallationRevision()}}, At: at, Revision: w.GetCompletionRevision()}
	if !c.Valid() {
		return bad()
	}
	return c, nil
}

func CarrierCompletionWire(b *pb.CarrierApprovalBinding, c fact.CoreCompletion) (*pb.CarrierCompletion, error) {
	binding, err := wire.CarrierBinding(b)
	if err != nil {
		return nil, contract.ErrInvalid
	}
	d, err := CarrierDecisionWire(binding, c.Installed.Carrier.Binding)
	if err != nil || !c.Valid() {
		return nil, contract.ErrInvalid
	}
	w := &pb.CarrierCompletion{Decision: d, InstalledAt: wire.MountCutWire(c.Installed.Carrier.At), InstallationRevision: proto.Uint64(c.Installed.Carrier.Revision),
		CompletedAt: wire.MountCutWire(c.At), CompletionRevision: proto.Uint64(c.Revision)}
	if actual, err := CarrierCompletion(binding, w); err != nil || actual != c {
		return nil, contract.ErrInvalid
	}
	return w, nil
}
