package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestTransitionSourceResponseIsBoundedAndExact(t *testing.T) {
	source, target := values.FactAuthorityRef("00000000-0000-4000-8000-000000000011"), values.FactAuthorityRef("00000000-0000-4000-8000-000000000012")
	b := &pb.TransitionApprovalBinding{Source: proto.String(string(source)), Command: &pb.BeginTransitionRequest{
		OperationId: []byte{255, 0, 128}, Kind: pb.TransitionKind_TRANSITION_ROOT.Enum(), Target: proto.String(string(target)), Boundary: proto.String(string(target)),
		Expected: &netpb.Token{Owner: proto.String(string(source)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)}, SourceAddress: proto.String("local:source"), TargetAddress: proto.String("local:target"),
	}}
	q := &pb.ReadTransitionChunkRequest{Binding: b, Offset: proto.Uint64(0), MaxBytes: proto.Uint32(64 << 10)}
	scope := make([]byte, 32)
	scope[0] = 1
	f := &pb.ServiceTransitionFence{Actor: &pb.TransitionActor{Realm: []byte("r"), Principal: []byte{255, 0, 128}}, Expected: &pb.ServiceRevision{Incarnation: scope, Sequence: proto.Uint64(2)}, FinalCut: &pb.ServiceCoreCut{Incarnation: scope, Sequence: proto.Uint64(3)}, ManifestDigest: make([]byte, 32)}
	good := &pb.ReadTransitionChunkResponse{Outcome: &pb.ReadTransitionChunkResponse_Result{Result: &pb.TransitionChunk{Binding: b, Fence: f, TotalBytes: proto.Uint64(3), Offset: proto.Uint64(0), Data: []byte("abc")}}}
	validate := func(v *pb.ReadTransitionChunkResponse) error {
		if err := validateResponse(v.ProtoReflect(), 0); err != nil {
			return err
		}
		if err := validateTokenOwners(v.ProtoReflect(), source); err != nil {
			return err
		}
		return validateTransitionSourceExchange(q, v)
	}
	if err := validateTransitionSourceRequest(q, source, target); err != nil {
		t.Fatal(err)
	}
	if err := validate(good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*pb.TransitionChunk){
		"operation":      func(v *pb.TransitionChunk) { v.Binding.Command.OperationId = []byte("wrong") },
		"owner":          func(v *pb.TransitionChunk) { v.Binding.Source = proto.String(string(target)) },
		"short":          func(v *pb.TransitionChunk) { v.Data = v.Data[:2] },
		"extra":          func(v *pb.TransitionChunk) { v.Data = append(v.Data, 1) },
		"offset":         func(v *pb.TransitionChunk) { v.Offset = proto.Uint64(1) },
		"total":          func(v *pb.TransitionChunk) { v.TotalBytes = proto.Uint64(1 << 63) },
		"missing-offset": func(v *pb.TransitionChunk) { v.Offset = nil },
		"digest":         func(v *pb.TransitionChunk) { v.Fence.ManifestDigest = []byte{1} },
		"actor":          func(v *pb.TransitionChunk) { v.Fence.Actor.Principal = nil },
		"unknown":        func(v *pb.TransitionChunk) { v.ProtoReflect().SetUnknown([]byte{0xa0, 6, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			r := proto.Clone(good).(*pb.ReadTransitionChunkResponse)
			mutate(r.GetResult())
			if validate(r) == nil {
				t.Fatal("invalid reply accepted")
			}
		})
	}
	if validateTransitionSourceRequest(q, source, source) == nil {
		t.Fatal("wrong actor admitted")
	}
	q.MaxBytes = proto.Uint32(65537)
	if validateTransitionSourceRequest(q, source, target) == nil {
		t.Fatal("oversized pull admitted")
	}
}
