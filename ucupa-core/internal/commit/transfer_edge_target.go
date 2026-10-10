package commit

import (
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Install new parent-owned relations in the same cut as their local anchors.
// Their exact transferred placement remains reserved until every carrier has
// acknowledged the new binding. These records never copy carrier content.
func (s *coreState) installTransferredDelegations(r acceptedTransfer, slice transferfact.Slice) error {
	parent := r.installedProof()
	closed := make(map[transferfact.EdgeKey]transferfact.EdgeClosure, len(r.closures))
	for _, c := range r.closures {
		closed[c.Fence.Key()] = c
	}
	for _, member := range slice.Members() {
		if member.Mount.State == contract.MountNone {
			continue
		}
		c, exists := closed[parent.Fence(member.ID()).Key()]
		if !exists {
			return contract.ErrConflict
		}
		key, err := parent.Delegation(c)
		if err != nil {
			return err
		}
		a, ok := s.elements.Anchor(c.Fence.Mount)
		rangeValue, bounded := a.Range.BoundedSphere()
		if !ok || !bounded || rangeValue != c.Range || s.elements.HasDirectContent(a.ID) {
			return contract.ErrConflict
		}
		contact := member.Mount.Contact
		if !contact.NextHop.Valid() {
			return contract.ErrConflict
		}
		s.mount.facts.ReplaceOutgoing(a.ID, mountfact.Record{State: contract.MountActive, Key: key.MountKey, Admission: key,
			Revision: s.mountRevision(a.ID), Contact: s.newContact(key.MountKey, contact.NextHop, false)})
		delete(closed, c.Fence.Key())
	}
	if len(closed) != 0 {
		return contract.ErrConflict
	}
	return nil
}
