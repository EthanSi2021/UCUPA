package wire

import (
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func PublicFacetInspectionWire(r corecontract.PublicFacetInspection) *pb.PublicFacetInspection {
	x := &pb.PublicFacetInspection{Element: proto.String(string(r.Element)), State: pb.AttachmentState(r.State).Enum(), Revision: ControlToken(r.Revision, netpb.TokenKind_ATTACHMENT_REVISION), Proposal: ControlToken(r.Proposal, netpb.TokenKind_ATTACHMENT_REVISION), PendingProposal: ControlToken(r.PendingProposal, netpb.TokenKind_ATTACHMENT_REVISION), ContactRevision: ControlToken(r.ContactRevision, netpb.TokenKind_CONTACT_REVISION)}
	if r.Target.Valid() {
		x.Target = proto.String(string(r.Target))
		x.KeyCore = proto.String(string(r.KeyCore))
	}
	if r.PendingTarget.Valid() {
		x.PendingTarget = proto.String(string(r.PendingTarget))
	}
	if r.NextHop.Valid() {
		x.NextHop = proto.String(string(r.NextHop))
	}
	return x
}
