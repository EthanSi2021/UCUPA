package coresdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestFacetControlResponseBinding(t *testing.T) {
	owner := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001")
	target := "00000000-0000-4000-8000-000000000002"
	element := "00000000-0000-4000-8000-000000000003"
	token := &netpb.Token{Owner: proto.String(string(owner)), Kind: netpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: make([]byte, 32)}
	req := &pb.ApplyFacetRequest{OperationId: []byte("operation"), Command: &pb.PublicFacetCommand{Kind: proto.String("propose"), Element: &element, Expected: token, Target: &target, NextHop: proto.String("https://example.test:443")}}
	out := &pb.ApplyFacetResponse{Outcome: &pb.ApplyFacetResponse_Result{Result: &pb.PublicFacetOutcome{OperationId: req.OperationId, Status: proto.Uint32(1), State: &pb.PublicFacetInspection{Element: &element, State: pb.AttachmentState_NONE.Enum(), Revision: token, PendingTarget: &target, PendingProposal: token}}}}
	if err := validateFacetControlExchange(req, out, owner); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"operation", "element", "status", "target-pair", "invalid-target", "active-with-pending", "contact-pair"} {
		t.Run(mode, func(t *testing.T) {
			changed := proto.Clone(out).(*pb.ApplyFacetResponse)
			r := changed.GetResult()
			s := r.GetState()
			switch mode {
			case "operation":
				r.OperationId = []byte("other")
			case "element":
				s.Element = proto.String(target)
			case "status":
				r.Status = proto.Uint32(3)
			case "target-pair":
				s.PendingTarget = nil
			case "invalid-target":
				s.PendingTarget = proto.String("invalid")
			case "active-with-pending":
				s.State = pb.AttachmentState_ACTIVE.Enum()
				s.Target = &target
				s.Proposal = token
			case "contact-pair":
				s.NextHop = proto.String("https://example.test:443")
			}
			if err := validateFacetControlExchange(req, changed, owner); err == nil {
				t.Fatal("malformed success accepted")
			}
		})
	}
}

func TestResolveProposalResponseRejectsAnotherProposal(t *testing.T) {
	owner := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001")
	element := "00000000-0000-4000-8000-000000000002"
	token := &netpb.Token{Owner: proto.String(string(owner)), Kind: netpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: make([]byte, 32)}
	req := &pb.ResolveProposalControlRequest{Element: &element, Proposal: token}
	out := &pb.ReadControlResponse{Outcome: &pb.ReadControlResponse_Result{Result: &pb.ControlRead{Core: proto.String(string(owner)), Element: &element, State: pb.AttachmentState_NONE.Enum(), Proposal: token, Pending: &pb.ServiceAttachmentKey{Core: proto.String(string(owner)), Element: &element}}}}
	if err := validateFacetControlExchange(req, out, owner); err != nil {
		t.Fatal(err)
	}
	other := proto.Clone(token).(*netpb.Token)
	other.Value[0] = 1
	out.GetResult().Proposal = other
	if err := validateFacetControlExchange(req, out, owner); err == nil {
		t.Fatal("another proposal comparison accepted")
	}
}

func TestResolveProposalResponseDetachedPredecessorShapes(t *testing.T) {
	owner := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001")
	former := "00000000-0000-4000-8000-000000000002"
	element := "00000000-0000-4000-8000-000000000003"
	facet := "00000000-0000-4000-8000-000000000004"
	other := "00000000-0000-4000-8000-000000000005"
	token := &netpb.Token{Owner: proto.String(string(owner)), Kind: netpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: make([]byte, 32)}
	req := &pb.ResolveProposalControlRequest{Element: &element, Proposal: token}
	detached := func(activeCore, activeFacet, activeElement string) *pb.ReadControlResponse {
		return &pb.ReadControlResponse{Outcome: &pb.ReadControlResponse_Result{Result: &pb.ControlRead{Core: proto.String(string(owner)), Element: &element, State: pb.AttachmentState_DETACHED.Enum(), Proposal: token,
			Pending: &pb.ServiceAttachmentKey{Core: proto.String(string(owner)), Element: &element, Facet: proto.String(facet)},
			Active:  &pb.ServiceAttachmentKey{Core: proto.String(activeCore), Facet: proto.String(activeFacet), Element: proto.String(activeElement)}}}}
	}
	// Both proven DETACHED predecessor shapes resolve: the former Core's key
	// after a settled transfer and this owner's revoked key after a local
	// detach.
	for name, r := range map[string]*pb.ReadControlResponse{
		"reacceptance": detached(former, facet, element),
		"recovery":     detached(string(owner), facet, element),
	} {
		if err := validateFacetControlExchange(req, r, owner); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, r := range map[string]*pb.ReadControlResponse{
		"other-element": detached(string(owner), facet, other),
		"other-facet":   detached(string(owner), other, element),
	} {
		if err := validateFacetControlExchange(req, r, owner); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	r := detached(string(owner), facet, element)
	r.GetResult().Active = nil
	if err := validateFacetControlExchange(req, r, owner); err == nil {
		t.Fatal("missing predecessor accepted")
	}
}
