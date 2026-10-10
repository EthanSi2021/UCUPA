package transitionwire

import (
	"google.golang.org/protobuf/proto"
	"testing"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func TestSourceDecisionEncodingPreservesIdentityAndCuts(t *testing.T) {
	source, target := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001"), values.FactAuthorityRef("00000000-0000-4000-8000-000000000002")
	id := values.ElementID(target)
	public, _ := values.PublicTokenFromBytes(source, "record-revision", [32]byte{1})
	b := contract.TransitionApprovalBinding{Source: source, Operation: contract.OperationID(string([]byte{255, 0, 128})), Command: contract.TransitionCommand{Kind: contract.TransitionRoot, Target: target, Boundary: id, Expected: public, SourceAddress: "local:source", TargetAddress: "local:target"}}
	w, err := BindingWire(b)
	if err != nil {
		t.Fatal(err)
	}
	f := fact.SourceFence{Input: fact.Intent{Profile: fact.RootSuccessor, Source: source, Target: target, Actor: values.PrincipalRef{Realm: "r", Principal: "u"}, Operation: b.Operation, Boundary: id, Expected: token.RevisionFromParts([32]byte{1}, id, 2), SourceAddress: b.Command.SourceAddress, TargetAddress: b.Command.TargetAddress}, FinalCut: token.FromParts([32]byte{1}, 3), ManifestDigest: [32]byte{1}}
	d := fact.SourceDecision{Acceptance: fact.TargetAcceptance{Fence: f, At: token.FromParts([32]byte{2}, 2), Revision: 7}, At: token.FromParts([32]byte{1}, 4), Revision: 9}
	accepted := AcceptanceWire(w, d.Acceptance)
	if got, err := Acceptance(b, accepted); err != nil || got != d.Acceptance {
		t.Fatalf("acceptance round trip: %v", err)
	}
	for name, change := range map[string]func(*pb.TransitionAcceptance){
		"wrong-binding": func(v *pb.TransitionAcceptance) { v.Binding.Command.OperationId = []byte("other") },
		"source-scope":  func(v *pb.TransitionAcceptance) { v.AcceptedAt.Incarnation[0] = 1 },
		"zero-cut":      func(v *pb.TransitionAcceptance) { v.AcceptedAt.Sequence = proto.Uint64(0) },
		"zero-revision": func(v *pb.TransitionAcceptance) { v.AcceptanceRevision = proto.Uint64(0) },
	} {
		t.Run("acceptance-"+name, func(t *testing.T) {
			v := proto.Clone(accepted).(*pb.TransitionAcceptance)
			change(v)
			if _, err := Acceptance(b, v); err == nil {
				t.Fatal("invalid acceptance")
			}
		})
	}
	c := fact.CoreCompletion{Installed: fact.CoreInstallation{Source: fact.TargetInstallation{Decision: d, At: token.FromParts([32]byte{2}, 3), Revision: 8}}, At: token.FromParts([32]byte{2}, 3), Revision: 8}
	completed := CompletionWire(w, c)
	if got, err := Completion(b, completed); err != nil || got != c {
		t.Fatalf("completion round trip: %v", err)
	}
	for name, change := range map[string]func(*pb.TransitionCompletion){
		"stale-install":             func(v *pb.TransitionCompletion) { v.InstalledAt.Sequence = proto.Uint64(2) },
		"wrong-install-owner":       func(v *pb.TransitionCompletion) { v.InstalledAt.Incarnation[0] = 1 },
		"stale-install-revision":    func(v *pb.TransitionCompletion) { v.InstallationRevision = proto.Uint64(7) },
		"stale-completion":          func(v *pb.TransitionCompletion) { v.CompletedAt.Sequence = proto.Uint64(2) },
		"wrong-completion-owner":    func(v *pb.TransitionCompletion) { v.CompletedAt.Incarnation[0] = 1 },
		"stale-completion-revision": func(v *pb.TransitionCompletion) { v.CompletionRevision = proto.Uint64(7) },
	} {
		t.Run("completion-"+name, func(t *testing.T) {
			v := proto.Clone(completed).(*pb.TransitionCompletion)
			change(v)
			if _, err := Completion(b, v); err == nil {
				t.Fatal("invalid completion")
			}
		})
	}
	good := DecisionWire(w, d)
	d.Acceptance.Fence.Input.Actor = values.PrincipalRef{Realm: values.RealmID(string([]byte{128, 0, 255})), Principal: values.PrincipalID(string([]byte{255, 0, 128}))}
	good = DecisionWire(w, d)
	encoded, err := proto.Marshal(good)
	if err != nil {
		t.Fatal(err)
	}
	good = &pb.TransitionDecision{}
	if err := proto.Unmarshal(encoded, good); err != nil {
		t.Fatal(err)
	}
	if _, err := wire.ToValue(good.ProtoReflect(), 0); err != nil {
		t.Fatal(err)
	}
	if actual, err := Decision(b, good); err != nil || actual != d {
		t.Fatalf("round trip: %v", err)
	}
	for name, mutate := range map[string]func(*pb.TransitionDecision){
		"wrong-source":      func(v *pb.TransitionDecision) { v.Binding.Source = proto.String(string(target)) },
		"wrong-operation":   func(v *pb.TransitionDecision) { v.Binding.Command.OperationId = []byte("other") },
		"empty-actor":       func(v *pb.TransitionDecision) { v.Fence.Actor = nil },
		"bad-expected":      func(v *pb.TransitionDecision) { v.Fence.Expected.Sequence = proto.Uint64(0) },
		"empty-digest":      func(v *pb.TransitionDecision) { v.Fence.ManifestDigest = make([]byte, 32) },
		"wrong-fence-scope": func(v *pb.TransitionDecision) { v.Fence.FinalCut.Incarnation[0] = 3 },
		"same-owner-cuts": func(v *pb.TransitionDecision) {
			v.AcceptedAt.Incarnation = append([]byte(nil), v.Fence.FinalCut.Incarnation...)
		},
		"stale-decision":       func(v *pb.TransitionDecision) { v.DecidedAt.Sequence = proto.Uint64(3) },
		"missing-revision":     func(v *pb.TransitionDecision) { v.AcceptanceRevision = nil },
		"wrong-decision-owner": func(v *pb.TransitionDecision) { v.DecidedAt.Incarnation[0] = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			v := proto.Clone(good).(*pb.TransitionDecision)
			mutate(v)
			if _, err := Decision(b, v); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}
