package coresdk

import (
	"testing"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestTransitionTargetReplyKeepsSourceTokenAndExactBinding(t *testing.T) {
	source, target := values.FactAuthorityRef("00000000-0000-4000-8000-000000000011"), values.FactAuthorityRef("00000000-0000-4000-8000-000000000012")
	b := &pb.TransitionApprovalBinding{Source: proto.String(string(source)), Command: &pb.BeginTransitionRequest{OperationId: []byte{255, 0, 128}, Kind: pb.TransitionKind_TRANSITION_ROOT.Enum(), Target: proto.String(string(target)), Boundary: proto.String(string(target)), Expected: &netpb.Token{Owner: proto.String(string(source)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)}, SourceAddress: proto.String("local:source"), TargetAddress: proto.String("local:target")}}
	f := &pb.ServiceTransitionFence{Actor: &pb.TransitionActor{Realm: []byte{255, 0}, Principal: []byte{128, 0}}, Expected: &pb.ServiceRevision{Incarnation: make([]byte, 32), Sequence: proto.Uint64(1)}, FinalCut: &pb.ServiceCoreCut{Incarnation: make([]byte, 32), Sequence: proto.Uint64(2)}, ManifestDigest: make([]byte, 32)}
	f.Expected.Incarnation[0], f.FinalCut.Incarnation[0], f.ManifestDigest[0] = 1, 1, 1
	q := &pb.AcceptTransitionRequest{Binding: b, ApprovalToken: make([]byte, 32)}
	good := &pb.AcceptTransitionResponse{Outcome: &pb.AcceptTransitionResponse_Result{Result: &pb.TransitionAcceptance{Binding: b, Fence: f, AcceptedAt: &pb.ServiceCoreCut{Incarnation: make([]byte, 32), Sequence: proto.Uint64(3)}, AcceptanceRevision: proto.Uint64(1)}}}
	good.GetResult().AcceptedAt.Incarnation[0] = 2
	validate := func(v *pb.AcceptTransitionResponse) error {
		if err := validateResponse(v.ProtoReflect(), 0); err != nil {
			return err
		}
		if err := validateTokenOwners(v.ProtoReflect(), target); err != nil {
			return err
		}
		return validateTransitionTargetExchange(q, v)
	}
	if err := validateTransitionTargetRequest(q, target, source); err != nil {
		t.Fatal(err)
	}
	if err := validate(good); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*pb.TransitionAcceptance){
		"operation":   func(v *pb.TransitionAcceptance) { v.Binding.Command.OperationId = []byte("other") },
		"wrong-owner": func(v *pb.TransitionAcceptance) { v.Binding.Command.Expected.Owner = proto.String(string(target)) },
		"actor":       func(v *pb.TransitionAcceptance) { v.Fence.Actor = nil },
		"missing-cut": func(v *pb.TransitionAcceptance) { v.AcceptedAt = nil },
		"revision":    func(v *pb.TransitionAcceptance) { v.AcceptanceRevision = proto.Uint64(0) },
		"unknown":     func(v *pb.TransitionAcceptance) { v.ProtoReflect().SetUnknown([]byte{0xa0, 6, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			v := proto.Clone(good).(*pb.AcceptTransitionResponse)
			change(v.GetResult())
			if validate(v) == nil {
				t.Fatal("invalid target reply")
			}
		})
	}
	if validateTransitionTargetRequest(q, target, target) == nil {
		t.Fatal("wrong service identity")
	}
	q.ApprovalToken = q.ApprovalToken[:31]
	if validateTransitionTargetRequest(q, target, source) == nil {
		t.Fatal("short approval")
	}
	d := &pb.TransitionDecision{Binding: b, Fence: f, AcceptedAt: good.GetResult().AcceptedAt, AcceptanceRevision: proto.Uint64(1), DecidedAt: f.FinalCut, DecisionRevision: proto.Uint64(2)}
	installed := &pb.InstallTransitionResponse{Outcome: &pb.InstallTransitionResponse_Result{Result: &pb.TransitionCompletion{Decision: d, InstalledAt: d.AcceptedAt, InstallationRevision: proto.Uint64(2), CompletedAt: d.AcceptedAt, CompletionRevision: proto.Uint64(2)}}}
	install := &pb.InstallTransitionRequest{Binding: b}
	if err := validateTokenOwners(installed.ProtoReflect(), target); err != nil {
		t.Fatal(err)
	}
	if err := validateTransitionTargetExchange(install, installed); err != nil {
		t.Fatal(err)
	}
	bad := proto.Clone(installed).(*pb.InstallTransitionResponse)
	bad.GetResult().Decision.Binding.Command.Expected.Owner = proto.String(string(target))
	if validateTokenOwners(bad.ProtoReflect(), target) == nil {
		t.Fatal("completion substituted target comparison")
	}
}
