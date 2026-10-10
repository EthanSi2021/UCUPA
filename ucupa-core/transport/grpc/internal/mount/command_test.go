package mount

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

func clonePublicMountCommand(t *testing.T, command *pb.PublicMountCommand) *pb.PublicMountCommand {
	t.Helper()
	return proto.Clone(command).(*pb.PublicMountCommand)
}

func TestMountCommandPayloadRejectedBeforeOwner(t *testing.T) {
	base, _ := testPublicCommand(t)
	invalid := []struct {
		name    string
		command func() *pb.PublicMountCommand
	}{
		{name: "missing command", command: func() *pb.PublicMountCommand { return nil }},
		{name: "expected element kind", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.ExpectedElement.Kind = networkpb.TokenKind_MOUNT_REVISION.Enum()
			return command
		}},
		{name: "expected element length", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.ExpectedElement.Value = []byte{1}
			return command
		}},
		{name: "expected mount kind", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.ExpectedMount = wire.ControlToken(testToken(t, "mount-revision", 0x61), networkpb.TokenKind_RECORD_REVISION)
			return command
		}},
		{name: "expected mount length", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.ExpectedMount = &networkpb.Token{Owner: proto.String(string(testOwner)), Kind: networkpb.TokenKind_MOUNT_REVISION.Enum(), Value: []byte{1}}
			return command
		}},
		{name: "update carrier core", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.Kind = pb.MountCommandKind_MOUNT_UPDATE.Enum()
			command.CarrierNextHop, command.ParentNextHop = nil, nil
			return command
		}},
		{name: "update carrier next hop", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.Kind = pb.MountCommandKind_MOUNT_UPDATE.Enum()
			command.CarrierCore, command.ParentNextHop = nil, nil
			return command
		}},
		{name: "update parent next hop", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.Kind = pb.MountCommandKind_MOUNT_UPDATE.Enum()
			command.CarrierCore, command.CarrierNextHop = nil, nil
			return command
		}},
		{name: "establish final tuple", command: func() *pb.PublicMountCommand {
			command := clonePublicMountCommand(t, base)
			command.Final = &pb.SpatialTuple{}
			return command
		}},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			t.Run("describe", func(t *testing.T) {
				fixture := newServiceFixture(t)
				out, err := fixture.service.DescribeMount(callerContext(), &pb.DescribeMountRequest{Command: testCase.command()})
				if out != nil || status.Code(err) != codes.InvalidArgument || len(fixture.owner.calls) != 0 {
					t.Fatalf("invalid Describe payload reached owner: %v %v calls=%v", out, err, fixture.owner.calls)
				}
			})
			t.Run("apply", func(t *testing.T) {
				fixture := newServiceFixture(t)
				out, err := fixture.service.ApplyMount(callerContext(), &pb.ApplyMountRequest{Command: testCase.command()})
				if out != nil || status.Code(err) != codes.InvalidArgument || len(fixture.owner.calls) != 0 {
					t.Fatalf("invalid Apply payload reached owner: %v %v calls=%v", out, err, fixture.owner.calls)
				}
			})
		})
	}
}

func TestApplyMountResolvesBeforeApprovalValidation(t *testing.T) {
	command, public := testPublicCommand(t)
	operation := []byte("resolve-before-approval")
	badApproval := &pb.MountApproval{}

	t.Run("resolve failure wins", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.owner.resolveErr = corecontract.ErrDenied
		out, err := fixture.service.ApplyMount(callerContext(), &pb.ApplyMountRequest{OperationId: operation, Command: command, Approval: badApproval})
		if err != nil || out.GetFailure() != networkpb.Failure_DENIED || !reflect.DeepEqual(fixture.owner.calls, []string{"resolve"}) {
			t.Fatalf("approval displaced resolve failure: %v %v calls=%v", out, err, fixture.owner.calls)
		}
		if fixture.owner.resolveCaller != testCaller || fixture.owner.resolveID != corecontract.OperationID(operation) || fixture.owner.resolvePublic != public {
			t.Fatalf("resolve input changed: caller=%v id=%q command=%+v", fixture.owner.resolveCaller, fixture.owner.resolveID, fixture.owner.resolvePublic)
		}
		if fixture.owner.releases != 0 {
			t.Fatal("failed resolver transferred cleanup")
		}
	})

	t.Run("approval is checked after successful resolve", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.owner.resolveCommand = testResolvedCommand(t)
		fixture.owner.resolveView = values.AuthorizationView{0x71}
		out, err := fixture.service.ApplyMount(callerContext(), &pb.ApplyMountRequest{OperationId: operation, Command: command, Approval: badApproval})
		if out != nil || status.Code(err) != codes.InvalidArgument || !reflect.DeepEqual(fixture.owner.calls, []string{"resolve"}) {
			t.Fatalf("invalid approval reached Apply owner: %v %v calls=%v", out, err, fixture.owner.calls)
		}
		if fixture.owner.releases != 1 {
			t.Fatal("invalid approval leaked resolved evidence")
		}
	})
}

func TestApplyMountForwardsResolvedCommandAndLocalApprovalCopy(t *testing.T) {
	fixture := newServiceFixture(t)
	command, public := testPublicCommand(t)
	resolved := testResolvedCommand(t)
	view := values.AuthorizationView{0x81}
	operation := []byte("request-operation")
	digest := [32]byte{0x82}
	approval := testApproval(operation, digest)
	originalApproval := proto.Clone(approval).(*pb.MountApproval)
	want := corecontract.MountControlResult{
		OperationID:     corecontract.OperationID("owner-operation"),
		MountAnchorUUID: testOther,
		Decision:        corecontract.MountAborted,
		Settled:         true,
		Changed:         true,
		AbortCause:      corecontract.MountAbortConflict,
		BlockedBy:       corecontract.MountBlockedPeerStore,
	}
	fixture.owner.resolveCommand = resolved
	fixture.owner.resolveView = view
	fixture.owner.applyResult = want
	ctx := callerContext()
	out, err := fixture.service.ApplyMount(ctx, &pb.ApplyMountRequest{OperationId: operation, Command: command, Approval: approval})
	if err != nil || out.GetResult() == nil {
		t.Fatalf("Apply failed: %v %v", out, err)
	}
	if fixture.owner.releases != 1 || fixture.owner.releasedEarly {
		t.Fatal("resolved evidence not released exactly after Apply")
	}
	if !reflect.DeepEqual(fixture.owner.calls, []string{"resolve", "apply"}) || fixture.owner.resolveCaller != testCaller || fixture.owner.resolveID != corecontract.OperationID(operation) || fixture.owner.resolvePublic != public {
		t.Fatalf("resolve forwarding changed: caller=%v id=%q command=%+v calls=%v", fixture.owner.resolveCaller, fixture.owner.resolveID, fixture.owner.resolvePublic, fixture.owner.calls)
	}
	if fixture.owner.applyContext != ctx || fixture.owner.applyCaller != testCaller || fixture.owner.applyView != view || fixture.owner.applyID != corecontract.OperationID(operation) || fixture.owner.applyCommand != resolved {
		t.Fatalf("apply forwarding changed: ctx=%v caller=%v view=%v id=%q command=%+v", fixture.owner.applyContext == ctx, fixture.owner.applyCaller, fixture.owner.applyView, fixture.owner.applyID, fixture.owner.applyCommand)
	}
	if !fixture.appliedWithLocalApprovalCopy() || !fixture.connections.ConfiguredFor(testOwner) || !proto.Equal(approval, originalApproval) {
		t.Fatalf("approval binding mutated installed or input state: localCopy=%v configured=%v approvalEqual=%v", fixture.appliedWithLocalApprovalCopy(), fixture.connections.ConfiguredFor(testOwner), proto.Equal(approval, originalApproval))
	}
	result := out.GetResult()
	if !bytes.Equal(result.OperationId, []byte(want.OperationID)) || result.GetElement() != string(want.MountAnchorUUID) || result.GetDecision() != pb.MountDecisionKind_MOUNT_ABORTED || !result.GetSettled() || !result.GetChanged() || result.GetRejection() != pb.MountRejection_MOUNT_CONFLICT || result.GetBlockedBy() != uint32(corecontract.MountBlockedPeerStore) {
		t.Fatalf("Apply returned request data instead of owner outcome: %+v", result)
	}
}

func TestApplyMountFailureReleasesResolvedEvidence(t *testing.T) {
	for _, failure := range []error{values.PublicDenied, values.PublicUnavailable, context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			fixture := newServiceFixture(t)
			command, _ := testPublicCommand(t)
			fixture.owner.resolveCommand = testResolvedCommand(t)
			fixture.owner.resolveView = values.AuthorizationView{1}
			fixture.owner.applyErr = failure
			fixture.service.ApplyMount(callerContext(), &pb.ApplyMountRequest{OperationId: []byte("failed-apply"), Command: command})
			if fixture.owner.releases != 1 || fixture.owner.releasedEarly || !reflect.DeepEqual(fixture.owner.calls, []string{"resolve", "apply"}) {
				t.Fatalf("failure cleanup: releases=%d early=%v calls=%v", fixture.owner.releases, fixture.owner.releasedEarly, fixture.owner.calls)
			}
		})
	}
}

func TestDescribeMountUsesExpectedActorAndCopiesBinding(t *testing.T) {
	fixture := newServiceFixture(t)
	command, public := testPublicCommand(t)
	operation := []byte("describe-operation")
	digest := [32]byte{0x91, 0x92}
	fixture.owner.describeTarget = testTarget
	fixture.owner.describeDigest = digest
	fixture.owner.describeNeeded = true
	out, err := fixture.service.DescribeMount(callerContext(), &pb.DescribeMountRequest{OperationId: operation, Command: command})
	if err != nil || out.GetResult() == nil || !out.GetResult().GetRequired() {
		t.Fatalf("Describe failed: %v %v", out, err)
	}
	if fixture.owner.describeCaller != testCaller || fixture.owner.describeID != corecontract.OperationID(operation) || fixture.owner.describePublic != public || !reflect.DeepEqual(fixture.owner.calls, []string{"describe"}) {
		t.Fatalf("Describe input changed: caller=%v id=%q command=%+v calls=%v", fixture.owner.describeCaller, fixture.owner.describeID, fixture.owner.describePublic, fixture.owner.calls)
	}
	result := out.GetResult()
	binding := result.GetBinding()
	if result.GetTarget() != string(testTarget) || binding == nil || binding.GetActor() != string(testOwner) || binding.GetElement() != string(testElement) || !bytes.Equal(binding.OperationId, operation) || !bytes.Equal(binding.CommandDigest, digest[:]) {
		t.Fatalf("Describe binding changed: %+v", result)
	}
	wantOperation := append([]byte(nil), operation...)
	wantDigest := append([]byte(nil), digest[:]...)
	operation[0] ^= 0xff
	fixture.owner.describeDigest[0] ^= 0xff
	if !bytes.Equal(binding.OperationId, wantOperation) || !bytes.Equal(binding.CommandDigest, wantDigest) {
		t.Fatalf("Describe binding aliases owner or request memory: operation=%v digest=%v", binding.OperationId, binding.CommandDigest)
	}
}
