package mount

import (
	"context"
	"errors"
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

var (
	_ pb.MountControlServiceServer = (*Service)(nil)
	_ pb.MountContactServiceServer = (*Service)(nil)
)

type mountRPCCall struct {
	name string
	call func(*Service, context.Context) (bool, error)
}

func mountRPCCalls() []mountRPCCall {
	return []mountRPCCall{
		{name: "inspect", call: func(service *Service, ctx context.Context) (bool, error) {
			out, err := service.InspectMount(ctx, nil)
			return out == nil, err
		}},
		{name: "describe", call: func(service *Service, ctx context.Context) (bool, error) {
			out, err := service.DescribeMount(ctx, nil)
			return out == nil, err
		}},
		{name: "apply", call: func(service *Service, ctx context.Context) (bool, error) {
			out, err := service.ApplyMount(ctx, nil)
			return out == nil, err
		}},
		{name: "inspect contact", call: func(service *Service, ctx context.Context) (bool, error) {
			out, err := service.InspectMountContact(ctx, nil)
			return out == nil, err
		}},
		{name: "repair contact", call: func(service *Service, ctx context.Context) (bool, error) {
			out, err := service.RepairMountContact(ctx, nil)
			return out == nil, err
		}},
	}
}

func TestMountRPCAdmissionPrecedesCallerDelegationAndPayload(t *testing.T) {
	tests := []struct {
		name    string
		service func(*serviceFixture) *Service
		ctx     func() context.Context
		code    codes.Code
	}{
		{
			name:    "missing owner precedes caller and payload",
			service: func(fixture *serviceFixture) *Service { return New(nil, testOwner, fixture.connections) },
			ctx:     context.Background,
			code:    codes.Unimplemented,
		},
		{
			name:    "missing connections precedes caller and payload",
			service: func(fixture *serviceFixture) *Service { return New(fixture.owner, testOwner, nil) },
			ctx:     context.Background,
			code:    codes.Unimplemented,
		},
		{
			name:    "missing owner precedes payload",
			service: func(fixture *serviceFixture) *Service { return New(nil, testOwner, fixture.connections) },
			ctx:     callerContext,
			code:    codes.Unimplemented,
		},
		{
			name:    "missing connections precedes payload",
			service: func(fixture *serviceFixture) *Service { return New(fixture.owner, testOwner, nil) },
			ctx:     callerContext,
			code:    codes.Unimplemented,
		},
		{
			name:    "missing owner precedes delegated context",
			service: func(fixture *serviceFixture) *Service { return New(nil, testOwner, fixture.connections) },
			ctx:     func() context.Context { return grantedContext(true) },
			code:    codes.Unimplemented,
		},
		{
			name:    "missing caller precedes grant and payload",
			service: func(fixture *serviceFixture) *Service { return fixture.service },
			ctx:     func() context.Context { return grantedContext(false) },
			code:    codes.Unauthenticated,
		},
		{
			name:    "delegated caller precedes payload",
			service: func(fixture *serviceFixture) *Service { return fixture.service },
			ctx:     func() context.Context { return grantedContext(true) },
			code:    codes.PermissionDenied,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			for _, rpc := range mountRPCCalls() {
				t.Run(rpc.name, func(t *testing.T) {
					fixture := newServiceFixture(t)
					outNil, err := rpc.call(testCase.service(fixture), testCase.ctx())
					if !outNil || status.Code(err) != testCase.code || len(fixture.owner.calls) != 0 {
						t.Fatalf("admission result: outNil=%v code=%v calls=%v", outNil, status.Code(err), fixture.owner.calls)
					}
				})
			}
		})
	}
}

func TestMountInspectionAndContactForwardFiniteValues(t *testing.T) {
	t.Run("mount inspection", func(t *testing.T) {
		fixture := newServiceFixture(t)
		elementRevision := testToken(t, "record-revision", 0x11)
		mountRevision := testToken(t, "mount-revision", 0x12)
		fixture.owner.inspectResult = corecontract.PublicMountInspection{
			Element:         testElement,
			ElementRevision: elementRevision,
			MountRevision:   mountRevision,
			State:           corecontract.MountActive,
			Carrier:         testTarget,
			NextHop:         testCarrier,
		}
		out, err := fixture.service.InspectMount(callerContext(), &pb.InspectMountRequest{Element: proto.String(string(testElement))})
		want := &pb.PublicMountInspection{
			Element:         proto.String(string(testElement)),
			ElementRevision: wire.ControlToken(elementRevision, networkpb.TokenKind_RECORD_REVISION),
			MountRevision:   wire.ControlToken(mountRevision, networkpb.TokenKind_MOUNT_REVISION),
			State:           proto.Uint32(uint32(corecontract.MountActive)),
			CarrierCore:     proto.String(string(testTarget)),
			NextHop:         proto.String(string(testCarrier)),
		}
		if err != nil || out.GetResult() == nil || !proto.Equal(out.GetResult(), want) {
			t.Fatalf("inspection result changed: %v %v", out, err)
		}
		if fixture.owner.inspectCaller != testCaller || fixture.owner.inspectElement != testElement || !reflect.DeepEqual(fixture.owner.calls, []string{"inspect"}) {
			t.Fatalf("inspection input changed: caller=%v element=%v calls=%v", fixture.owner.inspectCaller, fixture.owner.inspectElement, fixture.owner.calls)
		}
	})

	t.Run("contact inspection", func(t *testing.T) {
		fixture := newServiceFixture(t)
		revision := testToken(t, "contact-revision", 0x21)
		fixture.owner.contactResult = corecontract.PublicMountContact{Element: testElement, Incoming: true, Peer: testTarget, NextHop: testParent, Revision: revision}
		out, err := fixture.service.InspectMountContact(callerContext(), &pb.InspectMountContactRequest{Element: proto.String(string(testElement)), Incoming: proto.Bool(true)})
		want := &pb.PublicMountContact{Element: proto.String(string(testElement)), Incoming: proto.Bool(true), PeerCore: proto.String(string(testTarget)), NextHop: proto.String(string(testParent)), Revision: wire.ControlToken(revision, networkpb.TokenKind_CONTACT_REVISION)}
		if err != nil || out.GetResult() == nil || !proto.Equal(out.GetResult(), want) {
			t.Fatalf("contact inspection result changed: %v %v", out, err)
		}
		if fixture.owner.contactCaller != testCaller || fixture.owner.contactElement != testElement || !fixture.owner.contactIncoming || !reflect.DeepEqual(fixture.owner.calls, []string{"inspect-contact"}) {
			t.Fatalf("contact inspection input changed: caller=%v element=%v incoming=%v calls=%v", fixture.owner.contactCaller, fixture.owner.contactElement, fixture.owner.contactIncoming, fixture.owner.calls)
		}
	})

	t.Run("contact repair", func(t *testing.T) {
		fixture := newServiceFixture(t)
		expected := testToken(t, "contact-revision", 0x31)
		revision := testToken(t, "contact-revision", 0x32)
		fixture.owner.repairResult = corecontract.PublicMountContact{Element: testElement, Incoming: false, Peer: testTarget, NextHop: testCarrier, Revision: revision}
		operation := []byte("repair-operation")
		out, err := fixture.service.RepairMountContact(callerContext(), &pb.RepairMountContactRequest{
			OperationId: operation,
			Element:     proto.String(string(testElement)),
			Incoming:    proto.Bool(false),
			Expected:    wire.ControlToken(expected, networkpb.TokenKind_CONTACT_REVISION),
			NextHop:     proto.String(string(testCarrier)),
		})
		want := &pb.PublicMountContact{Element: proto.String(string(testElement)), Incoming: proto.Bool(false), PeerCore: proto.String(string(testTarget)), NextHop: proto.String(string(testCarrier)), Revision: wire.ControlToken(revision, networkpb.TokenKind_CONTACT_REVISION)}
		if err != nil || out.GetResult() == nil || !proto.Equal(out.GetResult(), want) {
			t.Fatalf("contact repair result changed: %v %v", out, err)
		}
		if fixture.owner.repairCaller != testCaller || fixture.owner.repairID != corecontract.OperationID(operation) || fixture.owner.repairElement != testElement || fixture.owner.repairIncoming || fixture.owner.repairExpected != expected || fixture.owner.repairNextHop != testCarrier || !reflect.DeepEqual(fixture.owner.calls, []string{"repair-contact"}) {
			t.Fatalf("contact repair input changed: caller=%v id=%q element=%v incoming=%v expected=%v next=%v calls=%v", fixture.owner.repairCaller, fixture.owner.repairID, fixture.owner.repairElement, fixture.owner.repairIncoming, fixture.owner.repairExpected, fixture.owner.repairNextHop, fixture.owner.calls)
		}
	})
}

func TestRepairMountContactRejectsInvalidExpectedBeforeOwner(t *testing.T) {
	valid := wire.ControlToken(testToken(t, "contact-revision", 0x41), networkpb.TokenKind_CONTACT_REVISION)
	tests := []struct {
		name     string
		expected *networkpb.Token
	}{
		{name: "missing"},
		{name: "wrong kind", expected: &networkpb.Token{Owner: proto.String(string(testOwner)), Kind: networkpb.TokenKind_RECORD_REVISION.Enum(), Value: append([]byte(nil), valid.Value...)}},
		{name: "short value", expected: &networkpb.Token{Owner: proto.String(string(testOwner)), Kind: networkpb.TokenKind_CONTACT_REVISION.Enum(), Value: []byte{1}}},
		{name: "invalid owner", expected: &networkpb.Token{Owner: proto.String("invalid"), Kind: networkpb.TokenKind_CONTACT_REVISION.Enum(), Value: append([]byte(nil), valid.Value...)}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			out, err := fixture.service.RepairMountContact(callerContext(), &pb.RepairMountContactRequest{Expected: testCase.expected})
			if out != nil || status.Code(err) != codes.InvalidArgument || len(fixture.owner.calls) != 0 {
				t.Fatalf("invalid contact comparison reached owner: %v %v calls=%v", out, err, fixture.owner.calls)
			}
		})
	}
}

func TestMountRPCsMapOwnerFailure(t *testing.T) {
	tests := []struct {
		name      string
		call      func(*testing.T, *serviceFixture) (networkpb.Failure, error)
		wantCalls []string
	}{
		{name: "inspect", call: func(t *testing.T, fixture *serviceFixture) (networkpb.Failure, error) {
			fixture.owner.inspectErr = values.PublicDenied
			out, err := fixture.service.InspectMount(callerContext(), &pb.InspectMountRequest{})
			return out.GetFailure(), err
		}, wantCalls: []string{"inspect"}},
		{name: "describe", call: func(t *testing.T, fixture *serviceFixture) (networkpb.Failure, error) {
			fixture.owner.describeErr = values.PublicDenied
			command, _ := testPublicCommand(t)
			out, err := fixture.service.DescribeMount(callerContext(), &pb.DescribeMountRequest{Command: command})
			return out.GetFailure(), err
		}, wantCalls: []string{"describe"}},
		{name: "apply", call: func(t *testing.T, fixture *serviceFixture) (networkpb.Failure, error) {
			fixture.owner.resolveCommand = testResolvedCommand(t)
			fixture.owner.resolveView = values.AuthorizationView{1}
			fixture.owner.applyErr = values.PublicDenied
			command, _ := testPublicCommand(t)
			out, err := fixture.service.ApplyMount(callerContext(), &pb.ApplyMountRequest{Command: command})
			return out.GetFailure(), err
		}, wantCalls: []string{"resolve", "apply"}},
		{name: "inspect contact", call: func(t *testing.T, fixture *serviceFixture) (networkpb.Failure, error) {
			fixture.owner.contactErr = values.PublicDenied
			out, err := fixture.service.InspectMountContact(callerContext(), &pb.InspectMountContactRequest{})
			return out.GetFailure(), err
		}, wantCalls: []string{"inspect-contact"}},
		{name: "repair contact", call: func(t *testing.T, fixture *serviceFixture) (networkpb.Failure, error) {
			fixture.owner.repairErr = values.PublicDenied
			expected := wire.ControlToken(testToken(t, "contact-revision", 0x51), networkpb.TokenKind_CONTACT_REVISION)
			out, err := fixture.service.RepairMountContact(callerContext(), &pb.RepairMountContactRequest{Expected: expected})
			return out.GetFailure(), err
		}, wantCalls: []string{"repair-contact"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			failure, err := testCase.call(t, fixture)
			if err != nil || failure != networkpb.Failure_DENIED || !reflect.DeepEqual(fixture.owner.calls, testCase.wantCalls) {
				t.Fatalf("owner failure mapping changed: failure=%v err=%v calls=%v", failure, err, fixture.owner.calls)
			}
		})
	}
}

func TestMountPeerFailureVocabulary(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		failure networkpb.Failure
	}{
		{name: "invalid", err: corecontract.ErrInvalid, failure: networkpb.Failure_INVALID},
		{name: "denied", err: corecontract.ErrDenied, failure: networkpb.Failure_DENIED},
		{name: "not found", err: corecontract.ErrNotFound, failure: networkpb.Failure_NOT_FOUND},
		{name: "conflict", err: corecontract.ErrConflict, failure: networkpb.Failure_CONFLICT},
		{name: "revision conflict", err: corecontract.ErrRevisionConflict, failure: networkpb.Failure_CONFLICT},
		{name: "operation conflict", err: corecontract.ErrOperationConflict, failure: networkpb.Failure_CONFLICT},
		{name: "mount busy", err: corecontract.ErrMountBusy, failure: networkpb.Failure_CONFLICT},
		{name: "store", err: corecontract.ErrStore, failure: networkpb.Failure_UNAVAILABLE},
		{name: "unavailable", err: corecontract.ErrUnavailable, failure: networkpb.Failure_UNAVAILABLE},
		{name: "public invalid", err: values.PublicInvalid, failure: networkpb.Failure_INVALID},
		{name: "public denied", err: values.PublicDenied, failure: networkpb.Failure_DENIED},
		{name: "public unavailable", err: values.PublicUnavailable, failure: networkpb.Failure_UNAVAILABLE},
		{name: "public not found", err: values.PublicNotFound, failure: networkpb.Failure_NOT_FOUND},
		{name: "public gap", err: values.PublicGap, failure: networkpb.Failure_GAP},
		{name: "public invalid context", err: values.PublicInvalidContext, failure: networkpb.Failure_INVALID_CONTEXT},
		{name: "public exhausted", err: values.PublicResourceExhausted, failure: networkpb.Failure_RESOURCE_EXHAUSTED},
		{name: "public conflict", err: values.PublicConflict, failure: networkpb.Failure_CONFLICT},
		{name: "public indeterminate", err: values.PublicIndeterminate, failure: networkpb.Failure_INDETERMINATE},
		{name: "public outcome unknown", err: values.PublicOutcomeUnknown, failure: networkpb.Failure_OUTCOME_UNKNOWN},
		{name: "public unsupported", err: values.PublicUnsupportedType, failure: networkpb.Failure_UNSUPPORTED_TYPE},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.owner.inspectErr = testCase.err
			out, err := fixture.service.InspectMount(callerContext(), &pb.InspectMountRequest{})
			if err != nil || out.GetFailure() != testCase.failure || !reflect.DeepEqual(fixture.owner.calls, []string{"inspect"}) {
				t.Fatalf("failure mapping = %v %v calls=%v, want %v", out.GetFailure(), err, fixture.owner.calls, testCase.failure)
			}
		})
	}

	t.Run("known grpc status", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.owner.inspectErr = status.Error(codes.Aborted, "aborted")
		out, err := fixture.service.InspectMount(callerContext(), &pb.InspectMountRequest{})
		if out == nil || out.GetFailure() != networkpb.Failure_FAILURE_UNSPECIFIED || status.Code(err) != codes.Aborted {
			t.Fatalf("grpc status mapping changed: %v %v", out, err)
		}
	})

	t.Run("unknown error", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.owner.inspectErr = errors.New("unknown")
		out, err := fixture.service.InspectMount(callerContext(), &pb.InspectMountRequest{})
		if out == nil || out.GetFailure() != networkpb.Failure_FAILURE_UNSPECIFIED || status.Code(err) != codes.Internal {
			t.Fatalf("unknown error mapping changed: %v %v", out, err)
		}
	})
}
