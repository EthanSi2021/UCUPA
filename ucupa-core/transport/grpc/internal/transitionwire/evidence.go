// Package transitionwire converts finite authenticated transfer evidence. It
// does not read a Store, authorize an operation or accept caller-supplied proof.
package transitionwire

import (
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

// This profile bounds a whole pull as well as each RPC. Its manifest limit is
// intentionally no larger than the owner accepts; no truncation is successful.
const MaxManifestBytes = 1 << 20
const MaxChunkBytes = 64 << 10

func Fence(b contract.TransitionApprovalBinding, w *pb.ServiceTransitionFence) (fact.SourceFence, error) {
	if w == nil || len(w.ManifestDigest) != 32 {
		return fact.SourceFence{}, contract.ErrInvalid
	}
	actor := values.PrincipalRef{Realm: values.RealmID(w.GetActor().GetRealm()), Principal: values.PrincipalID(w.GetActor().GetPrincipal())}
	if !actor.Valid() || len(actor.Realm)+len(actor.Principal) > 4096 {
		return fact.SourceFence{}, contract.ErrInvalid
	}
	revision, err := wire.MountRevision(w.Expected, string(b.Command.Boundary))
	if err != nil {
		return fact.SourceFence{}, err
	}
	expected, err := revision.ElementRevision()
	if err != nil {
		return fact.SourceFence{}, err
	}
	cut, err := wire.MountCut(w.FinalCut)
	if err != nil {
		return fact.SourceFence{}, err
	}
	i := fact.Intent{Profile: fact.Profile(b.Command.Kind), Operation: b.Operation, Actor: actor,
		Source: b.Source, Target: b.Command.Target, Boundary: b.Command.Boundary, Expected: expected,
		Final: b.Command.Final, SourceAddress: b.Command.SourceAddress, TargetAddress: b.Command.TargetAddress}
	f := fact.SourceFence{Input: i, FinalCut: cut, ManifestDigest: [32]byte(w.ManifestDigest)}
	if !(fact.EdgeFence{Source: f, Mount: i.Boundary}).Valid() || token.Serial(cut) < token.RevisionSerial(expected) {
		return fact.SourceFence{}, contract.ErrInvalid
	}
	return f, nil
}

func FenceWire(f fact.SourceFence) *pb.ServiceTransitionFence {
	return &pb.ServiceTransitionFence{Actor: &pb.TransitionActor{Realm: []byte(f.Input.Actor.Realm), Principal: []byte(f.Input.Actor.Principal)},
		Expected: wire.MountRevisionWire(f.Input.Expected.ServiceEvidence()), FinalCut: wire.MountCutWire(f.FinalCut),
		ManifestDigest: append([]byte(nil), f.ManifestDigest[:]...)}
}

func Decision(b contract.TransitionApprovalBinding, w *pb.TransitionDecision) (fact.SourceDecision, error) {
	if w == nil {
		return fact.SourceDecision{}, contract.ErrInvalid
	}
	actual, err := wire.TransitionBinding(w.Binding, b.Command.Target)
	if err != nil || actual != b {
		return fact.SourceDecision{}, contract.ErrInvalid
	}
	f, err := Fence(b, w.Fence)
	if err != nil {
		return fact.SourceDecision{}, err
	}
	a, err := wire.MountCut(w.AcceptedAt)
	if err != nil {
		return fact.SourceDecision{}, err
	}
	d, err := wire.MountCut(w.DecidedAt)
	if err != nil || !token.Valid(a) || !token.Valid(d) || w.GetAcceptanceRevision() == 0 || w.GetDecisionRevision() == 0 ||
		token.Scope(a) == token.Scope(f.FinalCut) || token.Scope(d) != token.Scope(f.FinalCut) || token.Serial(d) <= token.Serial(f.FinalCut) {
		return fact.SourceDecision{}, contract.ErrInvalid
	}
	return fact.SourceDecision{Acceptance: fact.TargetAcceptance{Fence: f, At: a, Revision: w.GetAcceptanceRevision()}, At: d, Revision: w.GetDecisionRevision()}, nil
}

func DecisionWire(b *pb.TransitionApprovalBinding, d fact.SourceDecision) *pb.TransitionDecision {
	return &pb.TransitionDecision{Binding: proto.Clone(b).(*pb.TransitionApprovalBinding), Fence: FenceWire(d.Acceptance.Fence),
		AcceptedAt: wire.MountCutWire(d.Acceptance.At), AcceptanceRevision: proto.Uint64(d.Acceptance.Revision),
		DecidedAt: wire.MountCutWire(d.At), DecisionRevision: proto.Uint64(d.Revision)}
}

// MatchesFence binds all non-secret input, not a new authorization decision.
// The public expected token is checked by the source's actual operation record.
func MatchesFence(b contract.TransitionApprovalBinding, f fact.SourceFence) bool {
	i := f.Input
	return i.Source == b.Source && i.Target == b.Command.Target && i.Operation == b.Operation &&
		i.Boundary == b.Command.Boundary && contract.TransitionKind(i.Profile) == b.Command.Kind &&
		i.Final == b.Command.Final && i.SourceAddress == b.Command.SourceAddress && i.TargetAddress == b.Command.TargetAddress &&
		b.Command.Expected.Owner == b.Source && b.Command.Expected.Valid() && i.Actor != (values.PrincipalRef{})
}
