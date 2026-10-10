package coresdk

import (
	"testing"

	"google.golang.org/protobuf/proto"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestTransitionApprovalResponseKeepsSourceComparisonAndExactBinding(t *testing.T) {
	source, target := "00000000-0000-4000-8000-000000000011", values.FactAuthorityRef("00000000-0000-4000-8000-000000000012")
	b := &pb.TransitionApprovalBinding{Source: &source, Command: &pb.BeginTransitionRequest{
		OperationId: []byte{255, 0, 128}, Kind: pb.TransitionKind_TRANSITION_ROOT.Enum(), Target: proto.String(string(target)), Boundary: proto.String("00000000-0000-4000-8000-000000000013"),
		Expected: &netpb.Token{Owner: &source, Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)}, SourceAddress: proto.String("local:source"), TargetAddress: proto.String("local:target"),
	}}
	request := &pb.CreateTransitionApprovalRequest{Binding: b, TtlSeconds: proto.Uint64(5)}
	good := &pb.CreateTransitionApprovalResponse{Outcome: &pb.CreateTransitionApprovalResponse_Result{Result: &pb.TransitionApproval{Target: proto.String(string(target)), Binding: proto.Clone(b).(*pb.TransitionApprovalBinding), Token: make([]byte, 32), ExpiresInSeconds: proto.Uint64(5)}}}
	validate := func(in any, out proto.Message) error {
		if err := validateResponse(out.ProtoReflect(), 0); err != nil {
			return err
		}
		if err := validateTokenOwners(out.ProtoReflect(), target); err != nil {
			return err
		}
		return validateTransitionApprovalExchange(in, out, target)
	}
	if err := validate(request, good); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*pb.TransitionApproval){
		"target":           func(a *pb.TransitionApproval) { a.Target = &source },
		"source":           func(a *pb.TransitionApproval) { a.Binding.Source = proto.String(string(target)) },
		"operation":        func(a *pb.TransitionApproval) { a.Binding.Command.OperationId = []byte{255, 0, 129} },
		"comparison owner": func(a *pb.TransitionApproval) { a.Binding.Command.Expected.Owner = proto.String(string(target)) },
		"comparison kind": func(a *pb.TransitionApproval) {
			a.Binding.Command.Expected.Kind = netpb.TokenKind_TRANSITION_REVISION.Enum()
		},
		"comparison value": func(a *pb.TransitionApproval) { a.Binding.Command.Expected.Value[0] = 1 },
		"address":          func(a *pb.TransitionApproval) { a.Binding.Command.TargetAddress = proto.String("local:other") },
		"token":            func(a *pb.TransitionApproval) { a.Token = []byte{1} },
		"zero expiry":      func(a *pb.TransitionApproval) { a.ExpiresInSeconds = proto.Uint64(0) },
		"extended expiry":  func(a *pb.TransitionApproval) { a.ExpiresInSeconds = proto.Uint64(6) },
		"unknown":          func(a *pb.TransitionApproval) { a.ProtoReflect().SetUnknown([]byte{0xa0, 6, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := proto.Clone(good).(*pb.CreateTransitionApprovalResponse)
			change(changed.GetResult())
			if validate(request, changed) == nil {
				t.Fatal("invalid approval accepted")
			}
		})
	}
	check := &pb.CheckTransitionApprovalRequest{Binding: b, Token: good.GetResult().Token}
	checked := &pb.CheckTransitionApprovalResponse{Outcome: &pb.CheckTransitionApprovalResponse_Result{Result: &pb.TransitionApprovalCheck{Target: proto.String(string(target)), Binding: b, Realm: proto.String("target-realm"), Principal: proto.String("original-caller")}}}
	if err := validate(check, checked); err != nil {
		t.Fatal(err)
	}
	checked.GetResult().Principal = proto.String("")
	if validate(check, checked) == nil {
		t.Fatal("missing original caller accepted")
	}
}
