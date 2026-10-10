package mount

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	coretoken "ucupa/ucupa-core/internal/token"
)

func (core *Operations) UpdateMountNextHop(actor values.PrincipalRef, id corecontract.OperationID, element values.ElementID, key corecontract.MountKey, expected corecontract.NextHopRevision, nextHop values.NextHop) (corecontract.NextHopUpdateResult, error) {
	if core == nil || !commitbinding.ValidOperationID(id) || !element.Valid() || !key.Valid() || key.MountAnchorUUID != element || !coretoken.ValidNextHopRevision(expected) || !nextHop.Valid() {
		return corecontract.NextHopUpdateResult{}, corecontract.ErrInvalid
	}
	permission := corecontract.PermissionMountControl
	if key.CarrierAuthority == core.owner.AuthorityRef() {
		permission = corecontract.PermissionMountAccept
	} else if key.ParentAuthority != core.owner.AuthorityRef() {
		return corecontract.NextHopUpdateResult{}, corecontract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(permission, element))
	if err != nil {
		return corecontract.NextHopUpdateResult{}, err
	}
	binding := commitpkg.NextHopBinding{Actor: actor, Element: element, Key: key, Expected: expected, NextHop: nextHop}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.NextHopUpdateResult{}, corecontract.ErrStore
	}
	if !core.settled(approval) {
		return corecontract.NextHopUpdateResult{}, corecontract.ErrUnavailable
	}
	return core.updateMountAddressLocked(id, binding)
}

func (core *Operations) updateMountAddressLocked(id corecontract.OperationID, binding commitpkg.NextHopBinding) (corecontract.NextHopUpdateResult, error) {
	return core.owner.ApplyMountAddressLocked(id, binding)
}
