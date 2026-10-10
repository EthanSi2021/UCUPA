package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// A transferred mount is an explicit boundary, not permission to omit the
// external carrier or copy its content. Only stable active relations can enter
// this profile; unfinished controls and detached content need their own proof.
func (s Slice) selectMount(elements elementfact.View, mounts mountfact.View, id values.ElementID) (mountfact.Record, error) {
	r := mounts.Record(id)
	if r.Slot != "" {
		return mountfact.Record{}, contract.ErrMountBusy
	}
	if r.State == contract.MountNone {
		return mountfact.Record{}, nil
	}
	a, ok := elements.Anchor(id)
	scope, serial := token.Scope(s.cut), token.Serial(s.cut)
	if !ok || a.Root() || elements.HasDirectContent(id) || r.State != contract.MountActive || !r.Key.Valid() ||
		r.Key.ParentAuthority != s.source || r.Key.MountAnchorUUID != id || !mountfact.ValidMountAdmissionKey(r.Admission) || r.Admission.MountKey != r.Key ||
		!token.ValidMountRevision(r.Revision) || token.MountRevisionScope(r.Revision) != scope || token.MountRevisionElement(r.Revision) != id || token.MountRevisionSerial(r.Revision) > serial ||
		token.MountRevisionScope(r.Key.ProposalRevision) != scope || token.MountRevisionSerial(r.Key.ProposalRevision) > token.MountRevisionSerial(r.Revision) ||
		!r.Contact.NextHop.Valid() || !token.ValidNextHopRevision(r.Contact.Revision) || token.NextHopSerial(r.Contact.Revision) > serial ||
		r.Contact.Revision != token.NextHopRevisionFromParts(scope, r.Key, false, token.NextHopSerial(r.Contact.Revision)) {
		return mountfact.Record{}, contract.ErrMountCoordinationRequired
	}
	return r, nil
}

func (s Slice) HasMounts() bool {
	for _, m := range s.members {
		if m.Mount.State != contract.MountNone {
			return true
		}
	}
	return false
}
