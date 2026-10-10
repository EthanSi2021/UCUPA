package acceptance_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func TestNetworkCoreFacetRelationshipCommands(t *testing.T) {
	f, key := startControlFixture(t, true)
	ctx := context.Background()
	scope := values.CoreElementScope(f.c.AuthorityRef(), networkRoot)
	for _, p := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetPropose, corecontract.PermissionFacetCancel, corecontract.PermissionFacetContactUpdate} {
		f.grant(t, p, scope, false)
	}
	api := f.client.FacetControl()
	initial, err := api.InspectFacet(ctx, &pb.InspectFacetRequest{Element: proto.String(string(networkRoot))})
	if err != nil || initial.GetResult() == nil {
		t.Fatalf("inspect: %v %v", initial, err)
	}
	command := &pb.PublicFacetCommand{Kind: proto.String("propose"), Element: proto.String(string(networkRoot)), Expected: initial.GetResult().Revision, Target: key.Facet, NextHop: proto.String("https://facet.example:443")}
	request := &pb.ApplyFacetRequest{OperationId: []byte("network-facet-propose"), Command: command}
	pending, err := api.ApplyFacet(ctx, request)
	if err != nil || pending.GetResult() == nil {
		t.Fatalf("propose: %v %v", pending, err)
	}
	p := pending.GetResult().GetState()
	resolve := &pb.ResolveProposalControlRequest{Element: command.Element, Proposal: p.PendingProposal}
	resolved, err := f.client.AttachmentControl().ResolveProposalControl(ctx, resolve)
	if err != nil || resolved.GetResult().GetPending() == nil || resolved.GetResult().GetPending().GetFacet() != string(controlFacet) || !proto.Equal(resolved.GetResult().Proposal, p.PendingProposal) {
		t.Fatalf("resolve fixed service: %v %v", resolved, err)
	}
	// This client deliberately has both permissions in the fixture; ordinary
	// user sessions without the configured service binding are tested separately.
	repair := &pb.ApplyFacetRequest{OperationId: []byte("network-facet-contact"), Command: &pb.PublicFacetCommand{Kind: proto.String("contact"), Element: command.Element, Expected: p.ContactRevision, NextHop: proto.String("https://repaired.example:443")}}
	repaired, err := api.ApplyFacet(ctx, repair)
	if err != nil || repaired.GetResult().GetState().GetNextHop() != repair.Command.GetNextHop() {
		t.Fatalf("repair: %v %v", repaired, err)
	}
	replay, err := api.ApplyFacet(ctx, request)
	if err != nil || !proto.Equal(pending, replay) {
		t.Fatalf("old exact result: %v %v", replay, err)
	}
	bad := proto.Clone(repair).(*pb.ApplyFacetRequest)
	bad.OperationId = []byte("malformed-contact")
	bad.Command.Target = proto.String("")
	if _, err = api.ApplyFacet(ctx, bad); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("irrelevant present field accepted: %v", err)
	}
	f.grant(t, corecontract.PermissionFacetCancel, scope, true)
	cancel := &pb.ApplyFacetRequest{OperationId: []byte("network-cancel"), Command: &pb.PublicFacetCommand{Kind: proto.String("cancel"), Element: command.Element, Expected: p.Revision}}
	denied, err := api.ApplyFacet(ctx, cancel)
	if err != nil || denied.GetFailure() != netpb.Failure_DENIED {
		t.Fatalf("denied cancel: %v %v", denied, err)
	}
	f.grant(t, corecontract.PermissionFacetCancel, scope, false)
	cancelled, err := api.ApplyFacet(ctx, cancel)
	if err != nil || cancelled.GetResult().GetState().GetPendingTarget() != "" {
		t.Fatalf("cancel: %v %v", cancelled, err)
	}
	stale, err := f.client.AttachmentControl().ResolveProposalControl(ctx, resolve)
	if err != nil || stale.GetFailure() != netpb.Failure_CONFLICT {
		t.Fatalf("old proposal resolved: %v %v", stale, err)
	}
}

func TestNetworkResolveProposalRequiresFixedFacetService(t *testing.T) {
	f := newNetworkFixture(t, true)
	f.grant(t, corecontract.PermissionFacetProposalRead, values.CoreElementScope(f.c.AuthorityRef(), networkRoot), false)
	token, _ := values.NewPublicToken(f.c.AuthorityRef(), "attachment-revision")
	b, _ := values.DecodePublicBytes(token.Value)
	_, err := f.client.AttachmentControl().ResolveProposalControl(context.Background(), &pb.ResolveProposalControlRequest{Element: proto.String(string(networkRoot)), Proposal: &netpb.Token{Owner: proto.String(string(token.Owner)), Kind: netpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: b}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ordinary user accessed service evidence: %v", err)
	}
}
