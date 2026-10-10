package transitionwire

import (
	"google.golang.org/protobuf/proto"

	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func Acceptance(b contract.TransitionApprovalBinding, w *pb.TransitionAcceptance) (fact.TargetAcceptance, error) {
	if w == nil {
		return fact.TargetAcceptance{}, contract.ErrInvalid
	}
	actual, err := wire.TransitionBinding(w.Binding, b.Command.Target)
	if err != nil || actual != b {
		return fact.TargetAcceptance{}, contract.ErrInvalid
	}
	f, err := Fence(b, w.Fence)
	if err != nil {
		return fact.TargetAcceptance{}, err
	}
	at, err := wire.MountCut(w.AcceptedAt)
	if err != nil || !token.Valid(at) || token.Scope(at) == token.Scope(f.FinalCut) || w.GetAcceptanceRevision() == 0 {
		return fact.TargetAcceptance{}, contract.ErrInvalid
	}
	return fact.TargetAcceptance{Fence: f, At: at, Revision: w.GetAcceptanceRevision()}, nil
}

func AcceptanceWire(b *pb.TransitionApprovalBinding, a fact.TargetAcceptance) *pb.TransitionAcceptance {
	return &pb.TransitionAcceptance{Binding: proto.Clone(b).(*pb.TransitionApprovalBinding), Fence: FenceWire(a.Fence),
		AcceptedAt: wire.MountCutWire(a.At), AcceptanceRevision: proto.Uint64(a.Revision)}
}

func Completion(b contract.TransitionApprovalBinding, w *pb.TransitionCompletion) (fact.CoreCompletion, error) {
	if w == nil {
		return fact.CoreCompletion{}, contract.ErrInvalid
	}
	d, err := Decision(b, w.Decision)
	if err != nil {
		return fact.CoreCompletion{}, err
	}
	i, err := wire.MountCut(w.InstalledAt)
	if err != nil {
		return fact.CoreCompletion{}, err
	}
	at, err := wire.MountCut(w.CompletedAt)
	if err != nil {
		return fact.CoreCompletion{}, err
	}
	c := fact.CoreCompletion{Installed: fact.CoreInstallation{Source: fact.TargetInstallation{Decision: d, At: i, Revision: w.GetInstallationRevision()}}, At: at, Revision: w.GetCompletionRevision()}
	if !c.Valid() {
		return fact.CoreCompletion{}, contract.ErrInvalid
	}
	return c, nil
}

func CompletionWire(b *pb.TransitionApprovalBinding, c fact.CoreCompletion) *pb.TransitionCompletion {
	return &pb.TransitionCompletion{Decision: DecisionWire(b, c.Installed.Source.Decision),
		InstalledAt: wire.MountCutWire(c.Installed.Source.At), InstallationRevision: proto.Uint64(c.Installed.Source.Revision),
		CompletedAt: wire.MountCutWire(c.At), CompletionRevision: proto.Uint64(c.Revision)}
}
