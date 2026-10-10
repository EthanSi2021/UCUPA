package coresdk

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestMountExchangeRejectsOtherAttemptAndOwner(t *testing.T) {
	const a values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	const b values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	r := &pb.ServiceMountPrepare{Key: &pb.ServiceMountKey{ParentCore: proto.String(string(a)), CarrierCore: proto.String(string(b))}, OperationId: []byte("one")}
	in := &pb.PrepareMountRequest{Request: r}
	out := &pb.PrepareMountResponse{Outcome: &pb.PrepareMountResponse_Result{Result: &pb.ServiceMountPrepared{Request: proto.Clone(r).(*pb.ServiceMountPrepare), Prepared: proto.Bool(true), Rejection: pb.MountRejection_MOUNT_NO_REJECTION.Enum()}}}
	if err := validateMountExchange(in, out, b, a); err != nil {
		t.Fatal(err)
	}
	out.GetResult().Request.OperationId = []byte("two")
	if err := validateMountExchange(in, out, b, a); err == nil {
		t.Fatal("another attempt accepted")
	}
	out.GetResult().Request = proto.Clone(r).(*pb.ServiceMountPrepare)
	if err := validateMountExchange(in, out, a, b); err == nil {
		t.Fatal("wrong owners accepted")
	}
	out.GetResult().Rejection = pb.MountRejection_MOUNT_CONFLICT.Enum()
	if err := validateMountExchange(in, out, b, a); err == nil {
		t.Fatal("prepared rejection accepted")
	}
	decision := &pb.ServiceMountDecision{Request: r, Decision: pb.MountDecisionKind_MOUNT_COMMITTED.Enum()}
	finish := &pb.FinishMountResponse{Outcome: &pb.FinishMountResponse_Result{Result: &pb.ServiceMountClosed{Evidence: proto.Clone(decision).(*pb.ServiceMountDecision)}}}
	if err := validateMountExchange(&pb.FinishMountRequest{Evidence: decision}, finish, b, a); err != nil {
		t.Fatal(err)
	}
	finish.GetResult().Evidence.Decision = pb.MountDecisionKind_MOUNT_ABORTED.Enum()
	if err := validateMountExchange(&pb.FinishMountRequest{Evidence: decision}, finish, b, a); err == nil {
		t.Fatal("wrong decision accepted")
	}
	read := &pb.ReadMountDecisionRequest{ParentCore: proto.String(string(a)), Actor: proto.String(string(b)), OperationId: []byte("one")}
	response := &pb.ReadMountDecisionResponse{Outcome: &pb.ReadMountDecisionResponse_Result{Result: decision}}
	if err := validateMountExchange(read, response, a, b); err != nil {
		t.Fatal(err)
	}
	read.OperationId = []byte("two")
	if err := validateMountExchange(read, response, a, b); err == nil {
		t.Fatal("another decision accepted")
	}
}

func TestPublicMountExchangeRejectsMismatchedReplies(t *testing.T) {
	const a values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	const b values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	command := &pb.PublicMountCommand{Element: proto.String(string(b))}
	in := &pb.DescribeMountRequest{OperationId: []byte("one"), Command: command}
	base := &pb.MountApprovalDescription{Required: proto.Bool(true), Target: proto.String(string(b)), Binding: &pb.MountApprovalBinding{Actor: proto.String(string(a)), Element: command.Element, OperationId: []byte("one"), CommandDigest: make([]byte, 32)}}
	for _, field := range []string{"valid", "actor", "element", "operation", "digest", "target", "not-required"} {
		r := proto.Clone(base).(*pb.MountApprovalDescription)
		switch field {
		case "actor":
			r.Binding.Actor = proto.String(string(b))
		case "element":
			r.Binding.Element = proto.String(string(a))
		case "operation":
			r.Binding.OperationId = []byte("two")
		case "digest":
			r.Binding.CommandDigest = []byte{1}
		case "target":
			r.Target = proto.String(string(a))
		case "not-required":
			r.Required = proto.Bool(false)
		}
		err := validateMountExchange(in, &pb.DescribeMountResponse{Outcome: &pb.DescribeMountResponse_Result{Result: r}}, a, "")
		if (err == nil) != (field == "valid") {
			t.Fatalf("%s: %v", field, err)
		}
	}
	apply := &pb.ApplyMountRequest{OperationId: []byte("one"), Command: command}
	r := &pb.PublicMountOutcome{OperationId: []byte("one"), Element: command.Element, Decision: pb.MountDecisionKind_MOUNT_COMMITTED.Enum(), Rejection: pb.MountRejection_MOUNT_NO_REJECTION.Enum(), Settled: proto.Bool(true)}
	out := &pb.ApplyMountResponse{Outcome: &pb.ApplyMountResponse_Result{Result: r}}
	if err := validateMountExchange(apply, out, a, ""); err != nil {
		t.Fatal(err)
	}
	r.OperationId = []byte("two")
	if err := validateMountExchange(apply, out, a, ""); err == nil {
		t.Fatal("other operation accepted")
	}
	r.OperationId = []byte("one")
	r.Decision = pb.MountDecisionKind_MOUNT_PENDING.Enum()
	if err := validateMountExchange(apply, out, a, ""); err == nil {
		t.Fatal("settled pending accepted")
	}
}
