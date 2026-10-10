package facetsdk

import (
	"google.golang.org/protobuf/proto"
	"testing"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
)

const relationshipCore = "00000000-0000-4000-8000-000000000002"
const relationshipElement = "00000000-0000-4000-8000-000000000003"

func testRelationshipRef() *pb.PublicAcceptanceRef {
	t := responseToken(netpb.TokenKind_ATTACHMENT_REVISION)
	t.Owner = proto.String(relationshipCore)
	return &pb.PublicAcceptanceRef{Core: proto.String(relationshipCore), Element: proto.String(relationshipElement), Proposal: t}
}

func TestFacetSDKRelationshipResponseBinding(t *testing.T) {
	request := &pb.AcceptAttachmentRequest{Ref: testRelationshipRef(), CoreNextHop: proto.String("https://core.example:443"), OperationId: []byte("accept")}
	valid := &pb.AcceptAttachmentResponse{Outcome: &pb.AcceptAttachmentResponse_Result{Result: &pb.PublicAcceptanceOutcome{
		Status: pb.CommitStatus_COMMITTED.Enum(), Inspection: &pb.PublicAcceptanceInspection{
			Ref: testRelationshipRef(), Found: proto.Bool(true), CoreNextHop: proto.String("https://core.example:443"), ContactRevision: responseToken(netpb.TokenKind_CONTACT_REVISION),
		},
	}}}
	if err := validateResponse(valid.ProtoReflect(), responseOwner, 0); err != nil {
		t.Fatalf("Core proposal comparison rejected: %v", err)
	}
	if err := validateRelationshipExchange(request, valid, responseOwner); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"core", "element", "proposal-owner", "proposal-value", "contact-owner", "contact-kind", "missing-contact", "not-found", "unknown"} {
		t.Run(mutation, func(t *testing.T) {
			response := proto.Clone(valid).(*pb.AcceptAttachmentResponse)
			r := response.GetResult().Inspection
			switch mutation {
			case "core":
				r.Ref.Core = proto.String(string(responseOwner))
				r.Ref.Proposal.Owner = proto.String(string(responseOwner))
			case "element":
				r.Ref.Element = proto.String("00000000-0000-4000-8000-000000000004")
			case "proposal-owner":
				r.Ref.Proposal.Owner = proto.String(string(responseOwner))
			case "proposal-value":
				r.Ref.Proposal.Value[0]++
			case "contact-owner":
				r.ContactRevision.Owner = proto.String(relationshipCore)
			case "contact-kind":
				r.ContactRevision.Kind = netpb.TokenKind_ATTACHMENT_REVISION.Enum()
			case "missing-contact":
				r.ContactRevision = nil
			case "not-found":
				r.Found, r.ContactRevision, r.CoreNextHop = proto.Bool(false), nil, nil
			case "unknown":
				r.Ref.Proposal.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
			}
			shape, binding := validateResponse(response.ProtoReflect(), responseOwner, 0), validateRelationshipExchange(request, response, responseOwner)
			if shape == nil && binding == nil {
				t.Fatal("accepted substituted or malformed relationship")
			}
		})
	}
	// A Core-owned token remains forbidden at ordinary Facet content positions.
	content := &pb.GetResult{Receipt: responseToken(netpb.TokenKind_READ_RECEIPT), State: pb.Presence_NOT_FOUND.Enum(), TypeRef: make([]byte, 32)}
	content.Receipt.Owner = proto.String(relationshipCore)
	if validateResponse(content.ProtoReflect(), responseOwner, 0) == nil {
		t.Fatal("public proposal exception leaked to content receipts")
	}
	update := &pb.UpdateCoreContactRequest{Ref: request.Ref, CoreNextHop: proto.String("https://new.example:443"), OperationId: []byte("repair"), Expected: valid.GetResult().Inspection.ContactRevision}
	response := &pb.UpdateCoreContactResponse{Outcome: &pb.UpdateCoreContactResponse_Result{Result: valid.GetResult()}}
	if validateRelationshipExchange(update, response, responseOwner) == nil {
		t.Fatal("update accepted another destination")
	}
}

func TestFacetSDKPrivateAcceptanceMatchesExactServiceRequest(t *testing.T) {
	incarnation := make([]byte, 32)
	incarnation[0] = 1
	request := &pb.ReadAcceptanceRequest{Key: &pb.ServiceAttachmentKey{Core: proto.String(relationshipCore), Facet: proto.String(string(responseOwner)), Element: proto.String(relationshipElement), Proposal: &pb.ServiceRevision{Incarnation: incarnation, Sequence: proto.Uint64(1<<60 + 1)}}}
	valid := &pb.ReadAcceptanceResponse{Outcome: &pb.ReadAcceptanceResponse_Result{Result: &pb.ServiceAcceptanceRead{
		Key: proto.Clone(request.Key).(*pb.ServiceAttachmentKey), Found: proto.Bool(true),
		Cut: &pb.ServiceFacetCut{Incarnation: incarnation, Sequence: proto.Uint64(9)}, AcceptedAt: &pb.ServiceFacetCut{Incarnation: incarnation, Sequence: proto.Uint64(8)},
	}}}
	if err := validateResponse(valid.ProtoReflect(), responseOwner, 0); err != nil {
		t.Fatal(err)
	}
	if err := validateRelationshipExchange(request, valid, responseOwner); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"key", "facet", "cut-incarnation", "cut-sequence", "future", "other-store", "found", "missing"} {
		t.Run(mode, func(t *testing.T) {
			response := proto.Clone(valid).(*pb.ReadAcceptanceResponse)
			r := response.GetResult()
			switch mode {
			case "key":
				r.Key.Proposal.Sequence = proto.Uint64(r.Key.Proposal.GetSequence() + 1)
			case "facet":
				r.Key.Facet = proto.String(relationshipCore)
			case "cut-incarnation":
				r.Cut.Incarnation = make([]byte, 32)
			case "cut-sequence":
				r.Cut.Sequence = proto.Uint64(0)
			case "future":
				r.AcceptedAt.Sequence = proto.Uint64(10)
			case "other-store":
				r.AcceptedAt.Incarnation[0]++
			case "found":
				r.Found = proto.Bool(false)
			case "missing":
				r.AcceptedAt = nil
			}
			if validateRelationshipExchange(request, response, responseOwner) == nil {
				t.Fatal("invalid service evidence accepted")
			}
		})
	}
	absent := proto.Clone(valid).(*pb.ReadAcceptanceResponse)
	absent.GetResult().Found, absent.GetResult().AcceptedAt = proto.Bool(false), nil
	if err := validateRelationshipExchange(request, absent, values.FactAuthorityRef(responseOwner)); err != nil {
		t.Fatal(err)
	}
}
