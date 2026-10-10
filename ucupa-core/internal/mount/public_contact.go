package mount

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	coretoken "ucupa/ucupa-core/internal/token"
)

func publicMountContactResult(signer coretoken.PublicSigner, key corecontract.MountKey, contact mountfact.Contact, incoming bool) corecontract.PublicMountContact {
	peer := key.CarrierAuthority
	if incoming {
		peer = key.ParentAuthority
	}
	return corecontract.PublicMountContact{Element: key.MountAnchorUUID, Incoming: incoming, Peer: peer, NextHop: contact.NextHop, Revision: signer.MountContact(contact.Revision)}
}

func (core *Operations) InspectMountContact(ctx context.Context, caller values.CallerContext, element values.ElementID, incoming bool) (corecontract.PublicMountContact, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.PublicMountContact{}, values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() {
		return corecontract.PublicMountContact{}, values.PublicInvalid
	}
	cut, err := core.captureReadableCut()
	if err != nil {
		return corecontract.PublicMountContact{}, err
	}
	requirements := []values.Requirement{core.requirement(corecontract.PermissionMountRead, element)}
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.PublicMountContact{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || core.owner.CurrentCutLocked() != cut {
		return corecontract.PublicMountContact{}, values.PublicUnavailable
	}
	key, contact := core.owner.MountContactAtLocked(element, incoming)
	if !key.Valid() {
		return corecontract.PublicMountContact{}, values.PublicNotFound
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.PublicMountContact{}, err
	}
	return publicMountContactResult(core.owner.SigningLocked(), key, contact, incoming), nil
}

// RepairMountContact never uses the peer connection or mount write gate. It
// must remain usable when the old address is unavailable or a range operation
// is paused. The existing address transition remains the only fact mutation.
// The authorization wait runs before the write lock; the retained cut confirms
// the resolved comparison still matches the state the decision was made for.
func (core *Operations) RepairMountContact(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, element values.ElementID, incoming bool, expected values.PublicToken, nextHop values.NextHop) (corecontract.PublicMountContact, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.PublicMountContact{}, values.PublicUnavailable
	}
	if ctx == nil || !commitbinding.ValidOperationID(id) || !element.Valid() || !nextHop.Valid() || !expected.Valid() || expected.Owner != core.owner.AuthorityRef() || expected.Kind != "contact-revision" {
		return corecontract.PublicMountContact{}, values.PublicInvalid
	}
	permission := corecontract.PermissionMountControl
	if incoming {
		permission = corecontract.PermissionMountAccept
	}
	requirements := []values.Requirement{core.requirement(permission, element)}
	core.owner.Lock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.Unlock()
		return corecontract.PublicMountContact{}, values.PublicUnavailable
	}
	cut := core.owner.CurrentCutLocked()
	core.owner.Unlock()
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.PublicMountContact{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || core.owner.CurrentCutLocked() != cut {
		return corecontract.PublicMountContact{}, values.PublicUnavailable
	}
	binding, err := core.owner.ResolvePublicMountContactLocked(caller.Subject, id, element, incoming, expected, nextHop)
	if err != nil {
		return corecontract.PublicMountContact{}, err
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.PublicMountContact{}, err
	}
	r, err := core.updateMountAddressLocked(id, binding)
	if err != nil {
		return corecontract.PublicMountContact{}, err
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.PublicMountContact{}, err
	}
	return publicMountContactResult(core.owner.SigningLocked(), r.MountKey, mountfact.Contact{NextHop: r.NextHop, Revision: r.NextHopRevision}, incoming), nil
}
