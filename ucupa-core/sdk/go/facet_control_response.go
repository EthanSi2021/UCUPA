package coresdk

import (
	"bytes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func validateFacetControlExchange(in, out any, owner values.FactAuthorityRef) error {
	bad := func() error { return status.Error(codes.DataLoss, "Facet control result does not match request") }
	check := func(r *pb.PublicFacetInspection, element string) error {
		if r == nil || r.GetElement() != element || r.GetState() < pb.AttachmentState_NONE || r.GetState() > pb.AttachmentState_RETIRED {
			return bad()
		}
		if (r.Target != nil) != (r.Proposal != nil) || (r.PendingTarget != nil) != (r.PendingProposal != nil) || (r.NextHop != nil) != (r.ContactRevision != nil) {
			return bad()
		}
		if (r.GetState() == pb.AttachmentState_NONE) != (r.Target == nil) {
			return bad()
		}
		if r.PendingTarget != nil && ((r.GetState() != pb.AttachmentState_NONE && r.GetState() != pb.AttachmentState_DETACHED) || !values.FactAuthorityRef(r.GetPendingTarget()).Valid()) {
			return bad()
		}
		if r.Target != nil && !values.FactAuthorityRef(r.GetTarget()).Valid() {
			return bad()
		}
		if r.NextHop != nil && !values.NextHop(r.GetNextHop()).Valid() {
			return bad()
		}
		return nil
	}
	switch req := in.(type) {
	case *pb.InspectFacetRequest:
		r, ok := out.(*pb.InspectFacetResponse)
		if !ok {
			return bad()
		}
		if r.GetResult() != nil {
			return check(r.GetResult(), req.GetElement())
		}
	case *pb.ApplyFacetRequest:
		r, ok := out.(*pb.ApplyFacetResponse)
		if !ok {
			return bad()
		}
		if x := r.GetResult(); x != nil {
			if !bytes.Equal(x.OperationId, req.OperationId) || x.GetStatus() < 1 || x.GetStatus() > 2 {
				return bad()
			}
			return check(x.GetState(), req.GetCommand().GetElement())
		}
	case *pb.ResolveProposalControlRequest:
		r, ok := out.(*pb.ReadControlResponse)
		if !ok {
			return bad()
		}
		if x := r.GetResult(); x != nil {
			if x.GetCore() != string(owner) || x.GetElement() != req.GetElement() || x.Pending == nil || x.Pending.GetElement() != req.GetElement() || x.Pending.GetCore() != string(owner) || !proto.Equal(x.Proposal, req.Proposal) || x.Preparation != nil {
				return bad()
			}
			switch x.GetState() {
			case pb.AttachmentState_NONE:
				if x.Active != nil {
					return bad()
				}
			case pb.AttachmentState_DETACHED:
				// Proven predecessor shapes: Active is the revoked key of the
				// same element and Facet — the former Core's after a settled
				// transfer (re-acceptance) or this owner's after a local
				// detach (recovery); pending stays current-owner.
				if x.Active == nil || !values.FactAuthorityRef(x.Active.GetCore()).Valid() || x.Active.GetElement() != req.GetElement() || x.Active.GetFacet() != x.Pending.GetFacet() {
					return bad()
				}
			default:
				return bad()
			}
		}
	}
	return nil
}
