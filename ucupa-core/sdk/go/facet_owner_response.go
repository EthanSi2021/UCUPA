package coresdk

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

// Only DETACHED may disclose a former Core's proposal, and only the proven
// DETACHED predecessor shape may carry a current-owner pending proposal next
// to it: the same-Facet re-acceptance proposal created after a settled Core
// transfer, or the recovery proposal created after this owner's local
// detach. RETIRED discloses exactly the disposed lineage key the current
// owner released. Context, current relationship CAS, pending proposal and
// contact comparisons stay current-owner qualified. This does not validate or
// reactivate the old content relationship.
func validateFacetResponseOwners(m protoreflect.Message, owner values.FactAuthorityRef) (bool, error) {
	bad := func() error { return status.Error(codes.DataLoss, "incoherent Facet key owner") }
	check := func(t *netpb.Token, expected values.FactAuthorityRef) error {
		if t == nil || !expected.Valid() || t.GetOwner() != string(expected) {
			return bad()
		}
		return nil
	}
	key := func(state pb.AttachmentState, core, facet string, proposal *netpb.Token, present bool) error {
		if state == pb.AttachmentState_NONE {
			if present || proposal != nil || core != "" || facet != "" {
				return bad()
			}
			return nil
		}
		if (state != pb.AttachmentState_ACTIVE && state != pb.AttachmentState_DETACHED && state != pb.AttachmentState_RETIRED) || !present || !values.FactAuthorityRef(facet).Valid() {
			return bad()
		}
		if state == pb.AttachmentState_ACTIVE && core != string(owner) {
			return bad()
		}
		return check(proposal, values.FactAuthorityRef(core))
	}
	switch a := m.Interface().(type) {
	case *pb.Attachment:
		if err := check(a.Revision, owner); err != nil {
			return true, err
		}
		if err := key(a.GetState(), a.Key.GetCore(), a.Key.GetFacet(), a.Key.GetProposal(), a.Key != nil); err != nil {
			return true, err
		}
		if a.GetState() != pb.AttachmentState_ACTIVE {
			if a.NextHop != nil || a.ContactRevision != nil {
				return true, bad()
			}
			return true, nil
		}
		if !values.NextHop(a.GetNextHop()).Valid() {
			return true, bad()
		}
		return true, check(a.ContactRevision, owner)
	case *pb.PublicFacetInspection:
		if err := check(a.Revision, owner); err != nil {
			return true, err
		}
		if (a.Target != nil) != (a.KeyCore != nil) {
			return true, bad()
		}
		if err := key(a.GetState(), a.GetKeyCore(), a.GetTarget(), a.Proposal, a.Target != nil); err != nil {
			return true, err
		}
		pending := a.PendingTarget != nil || a.PendingProposal != nil
		if pending {
			if (a.GetState() != pb.AttachmentState_NONE && a.GetState() != pb.AttachmentState_DETACHED) || !values.FactAuthorityRef(a.GetPendingTarget()).Valid() {
				return true, bad()
			}
			if err := check(a.PendingProposal, owner); err != nil {
				return true, err
			}
		}
		if a.GetState() == pb.AttachmentState_ACTIVE || pending {
			if !values.NextHop(a.GetNextHop()).Valid() {
				return true, bad()
			}
			return true, check(a.ContactRevision, owner)
		}
		if a.NextHop != nil || a.ContactRevision != nil {
			return true, bad()
		}
		return true, nil
	}
	return false, nil
}
