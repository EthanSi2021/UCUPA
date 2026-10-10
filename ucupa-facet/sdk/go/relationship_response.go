package facetsdk

import (
	"bytes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
)

func validateRelationshipExchange(in, out any, target values.FactAuthorityRef) error {
	bad := func() error { return status.Error(codes.DataLoss, "relationship response does not match request") }
	var ref *pb.PublicAcceptanceRef
	var inspection *pb.PublicAcceptanceInspection
	var result *pb.PublicAcceptanceOutcome
	switch request := in.(type) {
	case *pb.AcceptAttachmentRequest:
		response, ok := out.(*pb.AcceptAttachmentResponse)
		if !ok {
			return bad()
		}
		ref, result = request.Ref, response.GetResult()
	case *pb.InspectAcceptanceRequest:
		response, ok := out.(*pb.InspectAcceptanceResponse)
		if !ok {
			return bad()
		}
		ref, inspection = request.Ref, response.GetResult()
	case *pb.UpdateCoreContactRequest:
		response, ok := out.(*pb.UpdateCoreContactResponse)
		if !ok {
			return bad()
		}
		ref, result = request.Ref, response.GetResult()
		if result != nil && result.GetInspection().GetCoreNextHop() != request.GetCoreNextHop() {
			return bad()
		}
	case *pb.RetireContentRequest:
		response, ok := out.(*pb.RetireContentResponse)
		if !ok {
			return bad()
		}
		if result := response.GetResult(); result != nil {
			// The committed retire proves the disposed relationship absent; a
			// found inspection can never be its result.
			inspection := result.GetInspection()
			if inspection == nil || inspection.GetFound() || !proto.Equal(inspection.Ref, request.Ref) {
				return bad()
			}
		}
		return nil
	case *pb.ReadDispositionRequest:
		response, ok := out.(*pb.ReadDispositionResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if r.GetFound() {
				if !validServiceKey(r.Key, target) || !validServiceCut(r.Cut) {
					return bad()
				}
			} else if r.Key != nil || r.Cut != nil {
				return bad()
			}
		}
		return nil
	case *pb.ReadAcceptanceRequest:
		response, ok := out.(*pb.ReadAcceptanceResponse)
		if !ok {
			return bad()
		}
		if r := response.GetResult(); r != nil {
			if !proto.Equal(r.Key, request.Key) || !validServiceKey(r.Key, target) || !validServiceCut(r.Cut) {
				return bad()
			}
			if r.GetFound() {
				if !validServiceCut(r.AcceptedAt) || !bytes.Equal(r.Cut.Incarnation, r.AcceptedAt.Incarnation) || r.AcceptedAt.GetSequence() > r.Cut.GetSequence() {
					return bad()
				}
			} else if r.AcceptedAt != nil {
				return bad()
			}
		}
		return nil
	default:
		return nil
	}
	if result != nil {
		inspection = result.Inspection
		if inspection == nil || !inspection.GetFound() {
			return bad()
		}
	}
	if inspection != nil && (!proto.Equal(inspection.Ref, ref) || ref == nil) {
		return bad()
	}
	return nil
}

func validServiceKey(k *pb.ServiceAttachmentKey, facet values.FactAuthorityRef) bool {
	return k != nil && values.FactAuthorityRef(k.GetCore()).Valid() && k.GetFacet() == string(facet) &&
		values.ElementID(k.GetElement()).Valid() && k.Proposal != nil && len(k.Proposal.Incarnation) == 32 &&
		!bytes.Equal(k.Proposal.Incarnation, make([]byte, 32)) && k.Proposal.GetSequence() != 0
}

func validServiceCut(c *pb.ServiceFacetCut) bool {
	return c != nil && len(c.Incarnation) == 32 && !bytes.Equal(c.Incarnation, make([]byte, 32)) && c.GetSequence() != 0
}
