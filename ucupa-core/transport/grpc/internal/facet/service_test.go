package facet

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

const (
	testFacetOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	testFacetElement values.ElementID        = "00000000-0000-4000-8000-000000000002"
)

var _ pb.FacetControlServiceServer = (*Service)(nil)

type ownerStub struct {
	inspectErr              error
	applyErr                error
	inspectCalls            int
	applyCalls              int
	applyState              corecontract.MutationStatus
	inspection              corecontract.PublicFacetInspection
	beginCalls              int
	inspectReplacementCalls int
	cancelCalls             int
	recoverCalls            int
	replacement             corecontract.PublicFacetReplacementStatus
	replacementErr          error
	recoveredPeers          replacement.FacetPeers
}

func (o *ownerStub) InspectFacetControl(context.Context, values.CallerContext, values.ElementID) (corecontract.PublicFacetInspection, error) {
	o.inspectCalls++
	return o.inspection, o.inspectErr
}

func (o *ownerStub) ApplyFacetControl(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicFacetCommand) (corecontract.MutationStatus, corecontract.PublicFacetInspection, error) {
	o.applyCalls++
	return o.applyState, o.inspection, o.applyErr
}

func (o *ownerStub) BeginFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicFacetReplacementCommand) (corecontract.PublicFacetReplacementStatus, error) {
	o.beginCalls++
	return o.replacement, o.replacementErr
}

func (o *ownerStub) InspectFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID) (corecontract.PublicFacetReplacementStatus, error) {
	o.inspectReplacementCalls++
	return o.replacement, o.replacementErr
}

func (o *ownerStub) CancelFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID, values.PublicToken) (corecontract.PublicFacetReplacementStatus, error) {
	o.cancelCalls++
	return o.replacement, o.replacementErr
}

func (o *ownerStub) RecoverFacetReplacement(_ context.Context, _ values.CallerContext, _ corecontract.OperationID, _ values.ElementID, peers replacement.FacetPeers) (corecontract.PublicFacetReplacementStatus, error) {
	o.recoverCalls++
	o.recoveredPeers = peers
	return o.replacement, o.replacementErr
}

func facetFixture(t *testing.T) (*Service, *ownerStub, context.Context) {
	t.Helper()
	owner := &ownerStub{applyState: corecontract.MutationCommitted, inspection: corecontract.PublicFacetInspection{Element: testFacetElement, State: corecontract.FacetNone}}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "caller"}}
	return New(owner, replacement.FacetPeers{}), owner, grpcsession.WithCaller(context.Background(), caller)
}

func facetCommandWire(t *testing.T) *pb.PublicFacetCommand {
	t.Helper()
	token, err := values.PublicTokenFromBytes(testFacetOwner, "attachment-revision", [32]byte{7})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := values.DecodePublicBytes(token.Value)
	if err != nil {
		t.Fatal(err)
	}
	return &pb.PublicFacetCommand{Kind: proto.String("propose"), Element: proto.String(string(testFacetElement)), Expected: &netpb.Token{Owner: proto.String(string(testFacetOwner)), Kind: netpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: raw}}
}

func TestServiceUnsupportedOwnerRemainsRPCUnavailable(t *testing.T) {
	service := New(struct{}{}, replacement.FacetPeers{})
	out, err := service.InspectFacet(context.Background(), &pb.InspectFacetRequest{})
	if status.Code(err) != codes.Unimplemented || out != nil {
		t.Fatalf("unsupported owner changed RPC-time rejection: %v %v", out, err)
	}
}

func TestServiceDelegatedContextRejectedForBothWrites(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	ctx = grpcsession.WithGrant(ctx, [32]byte{1})
	if out, err := service.InspectFacet(ctx, &pb.InspectFacetRequest{}); status.Code(err) != codes.PermissionDenied || out != nil {
		t.Fatalf("delegated inspect was not rejected: %v %v", out, err)
	}
	if out, err := service.ApplyFacet(ctx, &pb.ApplyFacetRequest{}); status.Code(err) != codes.PermissionDenied || out != nil {
		t.Fatalf("delegated apply was not rejected before parsing: %v %v", out, err)
	}
	if owner.inspectCalls != 0 || owner.applyCalls != 0 {
		t.Fatalf("delegated request reached owner: inspect=%d apply=%d", owner.inspectCalls, owner.applyCalls)
	}
}

func TestServiceApplyCommandValidationPrecedesOwner(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	out, err := service.ApplyFacet(ctx, &pb.ApplyFacetRequest{})
	if status.Code(err) != codes.InvalidArgument || out != nil || owner.applyCalls != 0 {
		t.Fatalf("invalid Facet command reached owner: %v %v calls=%d", out, err, owner.applyCalls)
	}
}

func TestServiceMapsFacetControlFailures(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		failure netpb.Failure
	}{
		{name: "indeterminate", err: corecontract.ErrAttachmentIndeterminate, failure: netpb.Failure_INDETERMINATE},
		{name: "conflict", err: corecontract.ErrAttachmentRevisionConflict, failure: netpb.Failure_CONFLICT},
		{name: "not found", err: corecontract.ErrAcceptanceNotFound, failure: netpb.Failure_NOT_FOUND},
		{name: "disposition not found", err: corecontract.ErrDispositionNotFound, failure: netpb.Failure_NOT_FOUND},
		{name: "mount fallback", err: corecontract.ErrInvalid, failure: netpb.Failure_INVALID},
		{name: "public fallback", err: values.PublicDenied, failure: netpb.Failure_DENIED},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service, owner, ctx := facetFixture(t)
			owner.inspectErr = testCase.err
			out, err := service.InspectFacet(ctx, &pb.InspectFacetRequest{Element: proto.String(string(testFacetElement))})
			if err != nil || out == nil || out.GetFailure() != testCase.failure {
				t.Fatalf("failure mapping = %v %v, want %v", out, err, testCase.failure)
			}
		})
	}
}

func TestServiceApplyReleaseCarriesRoutingComparisons(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	command := facetCommandWire(t)
	command.Kind = proto.String("release")
	command.Target = proto.String(string(testFacetOwner))
	command.NextHop = proto.String("https://facet.example:443")
	if out, err := service.ApplyFacet(ctx, &pb.ApplyFacetRequest{OperationId: []byte("release"), Command: command}); err != nil || out == nil || out.GetResult() == nil || owner.applyCalls != 1 {
		t.Fatalf("release command rejected: %v %v calls=%d", out, err, owner.applyCalls)
	}
	// Other kinds keep refusing the routing comparisons only propose and
	// release may carry.
	command.Kind = proto.String("activate")
	if out, err := service.ApplyFacet(ctx, &pb.ApplyFacetRequest{OperationId: []byte("activate"), Command: command}); status.Code(err) != codes.InvalidArgument || out != nil || owner.applyCalls != 1 {
		t.Fatalf("activate with routing comparisons reached owner: %v %v calls=%d", out, err, owner.applyCalls)
	}
}

func TestServiceApplyCopiesOperationIDAndUsesOwnerResult(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	operation := []byte{1, 2, 3}
	out, err := service.ApplyFacet(ctx, &pb.ApplyFacetRequest{OperationId: operation, Command: facetCommandWire(t)})
	if err != nil || out == nil || out.GetResult() == nil || !bytes.Equal(out.GetResult().OperationId, operation) || out.GetResult().GetStatus() != uint32(corecontract.MutationCommitted) || owner.applyCalls != 1 {
		t.Fatalf("Facet apply result changed: %v %v calls=%d", out, err, owner.applyCalls)
	}
	operation[0] = 9
	if out.GetResult().OperationId[0] != 1 {
		t.Fatal("Facet operation ID was not copied")
	}
}

func facetReplacementStatus(t *testing.T) corecontract.PublicFacetReplacementStatus {
	t.Helper()
	revision, err := values.PublicTokenFromBytes(testFacetOwner, "facet-replacement-revision", [32]byte{5})
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.PublicFacetReplacementStatus{Authority: testFacetOwner, Operation: "replacement-op", Element: testFacetElement, Source: testFacetOwner, Target: testFacetOwner, Progress: corecontract.FacetReplacementReserved, Revision: revision}
}

func facetReplacementCommandWire(t *testing.T) *pb.PublicFacetReplacementCommand {
	t.Helper()
	token, err := values.PublicTokenFromBytes(testFacetOwner, "attachment-revision", [32]byte{9})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := values.DecodePublicBytes(token.Value)
	if err != nil {
		t.Fatal(err)
	}
	return &pb.PublicFacetReplacementCommand{Element: proto.String(string(testFacetElement)), Expected: &netpb.Token{Owner: proto.String(string(testFacetOwner)), Kind: netpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: raw}, Target: proto.String(string(testFacetOwner)), TargetAddress: proto.String("local:target"), SourceAddress: proto.String("local:source")}
}

func facetReplacementRevisionWire(t *testing.T) *netpb.Token {
	t.Helper()
	token, err := values.PublicTokenFromBytes(testFacetOwner, "facet-replacement-revision", [32]byte{6})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := values.DecodePublicBytes(token.Value)
	if err != nil {
		t.Fatal(err)
	}
	return &netpb.Token{Owner: proto.String(string(testFacetOwner)), Kind: netpb.TokenKind_FACET_REPLACEMENT_REVISION.Enum(), Value: raw}
}

func TestServiceBeginReplacementValidationPrecedesOwner(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	if out, err := service.BeginFacetReplacement(ctx, &pb.BeginFacetReplacementRequest{}); status.Code(err) != codes.InvalidArgument || out != nil || owner.beginCalls != 0 {
		t.Fatalf("invalid replacement command reached owner: %v %v calls=%d", out, err, owner.beginCalls)
	}
}

func TestServiceBeginReplacementEncodesStatus(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	owner.replacement = facetReplacementStatus(t)
	out, err := service.BeginFacetReplacement(ctx, &pb.BeginFacetReplacementRequest{OperationId: []byte("replacement-op"), Command: facetReplacementCommandWire(t)})
	if err != nil || out == nil || out.GetResult() == nil || owner.beginCalls != 1 {
		t.Fatalf("replacement begin failed: %v %v calls=%d", out, err, owner.beginCalls)
	}
	r := out.GetResult()
	if r.GetElement() != string(testFacetElement) || !bytes.Equal(r.GetOperationId(), []byte("replacement-op")) || r.GetSource() != string(testFacetOwner) || r.GetTarget() != string(testFacetOwner) ||
		r.GetProgress() != uint32(corecontract.FacetReplacementReserved) || r.GetRefused() || r.Revision == nil || r.Revision.GetKind() != netpb.TokenKind_FACET_REPLACEMENT_REVISION || r.Revision.GetOwner() != string(testFacetOwner) {
		t.Fatalf("replacement status encoding changed: %v", r)
	}
}

func TestServiceCancelReplacementRequiresRevisionToken(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	owner.replacement = facetReplacementStatus(t)
	if out, err := service.CancelFacetReplacement(ctx, &pb.CancelFacetReplacementRequest{OperationId: []byte("op"), Element: proto.String(string(testFacetElement))}); status.Code(err) != codes.InvalidArgument || out != nil || owner.cancelCalls != 0 {
		t.Fatalf("invalid replacement revision reached owner: %v %v calls=%d", out, err, owner.cancelCalls)
	}
	out, err := service.CancelFacetReplacement(ctx, &pb.CancelFacetReplacementRequest{OperationId: []byte("op"), Element: proto.String(string(testFacetElement)), Expected: facetReplacementRevisionWire(t)})
	if err != nil || out == nil || out.GetResult() == nil || owner.cancelCalls != 1 {
		t.Fatalf("replacement cancel failed: %v %v calls=%d", out, err, owner.cancelCalls)
	}
}

func TestServiceMapsReplacementFailures(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		failure netpb.Failure
	}{
		{name: "not found", err: corecontract.ErrNotFound, failure: netpb.Failure_NOT_FOUND},
		{name: "revision conflict", err: corecontract.ErrRevisionConflict, failure: netpb.Failure_CONFLICT},
		{name: "unavailable", err: corecontract.ErrUnavailable, failure: netpb.Failure_UNAVAILABLE},
		{name: "denied", err: values.PublicDenied, failure: netpb.Failure_DENIED},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service, owner, ctx := facetFixture(t)
			owner.replacementErr = testCase.err
			out, err := service.InspectFacetReplacement(ctx, &pb.ReplacementControlRequest{OperationId: []byte("op"), Element: proto.String(string(testFacetElement))})
			if err != nil || out == nil || out.GetFailure() != testCase.failure {
				t.Fatalf("failure mapping = %v %v, want %v", out, err, testCase.failure)
			}
		})
	}
}

func TestServiceRecoverReplacementRequiresWiredPeers(t *testing.T) {
	service, owner, ctx := facetFixture(t)
	out, err := service.RecoverFacetReplacement(ctx, &pb.ReplacementControlRequest{OperationId: []byte("op"), Element: proto.String(string(testFacetElement))})
	if status.Code(err) != codes.Unimplemented || out != nil || owner.recoverCalls != 0 {
		t.Fatalf("recovery without host peers reached owner: %v %v calls=%d", out, err, owner.recoverCalls)
	}
}

type replacementPeerStub struct{}

func (replacementPeerStub) CheckSourceExport(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID) (replacement.FacetReplacementSourceState, error) {
	return replacement.FacetReplacementSourceState{}, nil
}

func (replacementPeerStub) FenceSource(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, values.FactAuthorityRef) (replacement.FacetReplacementFenced, error) {
	return replacement.FacetReplacementFenced{}, nil
}

func (replacementPeerStub) ReadSourceEntry(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, values.FacetTypeRef) (replacement.FacetReplacementEntry, error) {
	return replacement.FacetReplacementEntry{}, nil
}

func (replacementPeerStub) SettleSource(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, values.FactAuthorityRef) error {
	return nil
}

func (replacementPeerStub) CheckTargetImport(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, []byte, values.NextHop) (replacement.FacetReplacementImportState, error) {
	return replacement.FacetReplacementImportState{}, nil
}

func (replacementPeerStub) StageTargetEntry(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, values.FacetTypeRef, []byte) error {
	return nil
}

func (replacementPeerStub) AcceptTarget(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, values.NextHop) (replacement.FacetReplacementAccepted, error) {
	return replacement.FacetReplacementAccepted{}, nil
}

func (replacementPeerStub) InstallTarget(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID, values.FactAuthorityRef, uint64) error {
	return nil
}

func (replacementPeerStub) ReadTargetImport(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID) (replacement.FacetReplacementImportState, error) {
	return replacement.FacetReplacementImportState{}, nil
}

func TestServiceRecoverReplacementPassesHostPeers(t *testing.T) {
	owner := &ownerStub{replacement: facetReplacementStatus(t)}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm", Principal: "caller"}}
	ctx := grpcsession.WithCaller(context.Background(), caller)
	peers := replacement.FacetPeers{Source: replacementPeerStub{}, Target: replacementPeerStub{}}
	service := New(owner, peers)
	out, err := service.RecoverFacetReplacement(ctx, &pb.ReplacementControlRequest{OperationId: []byte("replacement-op"), Element: proto.String(string(testFacetElement))})
	if err != nil || out == nil || out.GetResult() == nil || owner.recoverCalls != 1 {
		t.Fatalf("recovery with host peers failed: %v %v calls=%d", out, err, owner.recoverCalls)
	}
	if _, ok := owner.recoveredPeers.Source.(replacementPeerStub); !ok {
		t.Fatalf("host peers were not passed through: %T", owner.recoveredPeers.Source)
	}
}
