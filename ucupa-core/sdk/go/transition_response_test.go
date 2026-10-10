package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestTransitionResponsesBindOriginalRequest(t *testing.T) {
	owner := values.FactAuthorityRef("00000000-0000-4000-8000-000000000011")
	target := "00000000-0000-4000-8000-000000000012"
	boundary := "00000000-0000-4000-8000-000000000013"
	request := &pb.BeginTransitionRequest{OperationId: []byte{255, 0, 128}, Boundary: &boundary, Target: &target, Kind: pb.TransitionKind_TRANSITION_SLICE.Enum()}
	good := &pb.TransitionResponse{Outcome: &pb.TransitionResponse_Result{Result: &pb.TransitionStatus{Source: proto.String(string(owner)), OperationId: request.OperationId, Boundary: &boundary, Target: &target, Kind: request.Kind, Progress: pb.TransitionProgress_TRANSITION_INTENT.Enum(), Revision: &netpb.Token{Owner: proto.String(string(owner)), Kind: netpb.TokenKind_TRANSITION_REVISION.Enum(), Value: make([]byte, 32)}}}}
	validate := func(in any, r *pb.TransitionResponse) error {
		if err := validateResponse(r.ProtoReflect(), 0); err != nil {
			return err
		}
		if err := validateTokenOwners(r.ProtoReflect(), owner); err != nil {
			return err
		}
		return validateTransitionExchange(in, r, owner)
	}
	if err := validate(request, good); err != nil {
		t.Fatal(err)
	}
	request.Expected = &netpb.Token{Owner: proto.String(string(owner))}
	recover := &pb.RecoverTransitionRequest{Binding: &pb.TransitionApprovalBinding{Source: proto.String(string(owner)), Command: request}, ApprovalToken: make([]byte, 32)}
	if err := validateTransitionRecoveryRequest(recover, owner); err != nil {
		t.Fatal(err)
	}
	if err := validate(recover, good); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*pb.RecoverTransitionRequest){
		func(r *pb.RecoverTransitionRequest) { r.ApprovalToken = nil },
		func(r *pb.RecoverTransitionRequest) { r.Binding.Source = &target },
		func(r *pb.RecoverTransitionRequest) { r.Binding.Command.Expected.Owner = &target },
		func(r *pb.RecoverTransitionRequest) { r.Binding.Command.OperationId = make([]byte, 129) },
	} {
		r := proto.Clone(recover).(*pb.RecoverTransitionRequest)
		mutate(r)
		if validateTransitionRecoveryRequest(r, owner) == nil {
			t.Fatal("invalid recovery request accepted")
		}
	}
	for name, change := range map[string]func(*pb.TransitionStatus){
		"source":                 func(r *pb.TransitionStatus) { r.Source = &target },
		"operation":              func(r *pb.TransitionStatus) { r.OperationId = []byte{255, 0, 129} },
		"boundary":               func(r *pb.TransitionStatus) { r.Boundary = &target },
		"target":                 func(r *pb.TransitionStatus) { r.Target = &boundary },
		"same owner":             func(r *pb.TransitionStatus) { r.Target = proto.String(string(owner)) },
		"kind":                   func(r *pb.TransitionStatus) { r.Kind = pb.TransitionKind_TRANSITION_ROOT.Enum() },
		"unknown progress":       func(r *pb.TransitionStatus) { r.Progress = pb.TransitionProgress(99).Enum() },
		"missing progress":       func(r *pb.TransitionStatus) { r.Progress = nil },
		"wrong comparison kind":  func(r *pb.TransitionStatus) { r.Revision.Kind = netpb.TokenKind_RECORD_REVISION.Enum() },
		"wrong comparison owner": func(r *pb.TransitionStatus) { r.Revision.Owner = &target },
		"unknown fields":         func(r *pb.TransitionStatus) { r.ProtoReflect().SetUnknown([]byte{0xa0, 6, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			r := proto.Clone(good).(*pb.TransitionResponse)
			change(r.GetResult())
			if validate(request, r) == nil {
				t.Fatal("accepted mismatched result")
			}
			if validate(recover, r) == nil {
				t.Fatal("accepted mismatched recovery result")
			}
		})
	}
	inspect := &pb.InspectTransitionRequest{OperationId: request.OperationId, Boundary: &boundary}
	if err := validate(inspect, good); err != nil {
		t.Fatal(err)
	}
	cancel := &pb.CancelTransitionRequest{OperationId: request.OperationId, Boundary: &boundary, Expected: good.GetResult().Revision}
	if validate(cancel, good) == nil {
		t.Fatal("cancel accepted non-cancelled result")
	}
	good.GetResult().Progress = pb.TransitionProgress_TRANSITION_CANCELLED.Enum()
	if err := validate(cancel, good); err != nil {
		t.Fatal(err)
	}
	if err := validate(request, &pb.TransitionResponse{Outcome: &pb.TransitionResponse_Error{Error: netpb.Failure_INDETERMINATE}}); err != nil {
		t.Fatal(err)
	}
}
