package mount

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func (core *Operations) ApplyMount(ctx context.Context, callers corecontract.LocalCallers, id corecontract.OperationID, command corecontract.MountCommand) (corecontract.MountControlResult, error) {
	canonical, ok := commitpkg.CanonicalMountCommand(command)
	if core == nil || ctx == nil || !ok || !commitbinding.ValidOperationID(id) {
		return corecontract.MountControlResult{}, corecontract.ErrInvalid
	}
	actor := callers[core.owner.AuthorityRef()]
	approval, err := core.authorizeMount(actor, canonical)
	if err != nil {
		return corecontract.MountControlResult{}, err
	}
	if err := core.reserveMountIntent(callers, id, commitpkg.MountBinding{Actor: actor, Command: canonical}, approval); err != nil {
		return corecontract.MountControlResult{}, err
	}
	result, err := core.driveMount(ctx, id)
	approval, authErr := core.authorizeMount(actor, canonical)
	if authErr != nil {
		return corecontract.MountControlResult{}, authErr
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if !core.settled(approval) {
		return corecontract.MountControlResult{}, corecontract.ErrUnavailable
	}
	return result, err
}

func (core *Operations) authorizeMount(actor values.PrincipalRef, command corecontract.MountCommand) (corecontract.RealmAttachment, error) {
	return core.authorize(actor, core.mountControlRequirements(command)...)
}

func (core *Operations) reserveMountIntent(callers corecontract.LocalCallers, id corecontract.OperationID, binding commitpkg.MountBinding, approval corecontract.RealmAttachment) error {
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	if !core.settled(approval) {
		return corecontract.ErrUnavailable
	}
	return core.owner.ApplyMountIntentLocked(callers, id, binding)
}

func (core *Operations) reserveMountIntentGuarded(callers corecontract.LocalCallers, id corecontract.OperationID, binding commitpkg.MountBinding, guard *reservation) error {
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	if guard != nil {
		if err := guard.validateLocked(core); err != nil {
			return err
		}
	}
	return core.owner.ApplyMountIntentLocked(callers, id, binding)
}

func (core *Operations) executeMountEventLocked(event commitpkg.MountEvent) error {
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	return core.owner.ApplyMountEventLocked(event)
}

func (core *Operations) ReadMount(actor values.PrincipalRef, element values.ElementID) (corecontract.MountRead, error) {
	if core == nil || !element.Valid() {
		return corecontract.MountRead{}, corecontract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(corecontract.PermissionMountRead, element))
	if err != nil {
		return corecontract.MountRead{}, err
	}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return corecontract.MountRead{}, corecontract.ErrStore
	}
	if !core.settled(approval) {
		core.owner.RUnlock()
		return corecontract.MountRead{}, corecontract.ErrUnavailable
	}
	state := core.owner.FactsLocked()
	result := corecontract.MountRead{Cut: state.CurrentCut()}
	if incoming := state.Mount.Incoming(); incoming.Key.Valid() && incoming.Key.MountAnchorUUID == element {
		result.Incoming = true
		result.MountKey, result.AdmissionKey = incoming.Key, incoming.Admission.Key
		result.MountRevision = incoming.Key.ProposalRevision
		result.State = corecontract.MountEstablishing
		result.Range = incoming.Acceptance.Tuple.Range
		if mountfact.ValidMountAdmissionKey(incoming.Admission.Key) {
			result.State, result.Range = corecontract.MountActive, incoming.Admission.RangeValue
		}
		if incoming.Detached {
			result.State = corecontract.MountDetached
		}
		result.NextHop, result.NextHopRevision = incoming.Contact.NextHop, incoming.Contact.Revision
		result.ControlSlot, result.WriteGateOpen = incoming.Preparing.OperationID, incoming.Gate == mountfact.GateOpen
	} else {
		record := state.Mount.Record(element)
		anchor, exists := state.Elements.Anchor(element)
		if !exists {
			core.owner.RUnlock()
			return corecontract.MountRead{}, corecontract.ErrNotFound
		}
		result.State, result.MountRevision, result.MountKey = record.State, record.Revision, record.Key
		result.AdmissionKey, result.ControlSlot = record.Admission, record.Slot
		result.NextHop, result.NextHopRevision = record.Contact.NextHop, record.Contact.Revision
		result.Range, _ = anchor.Range.BoundedSphere()
	}
	core.owner.RUnlock()
	approval, err = core.authorize(actor, core.requirement(corecontract.PermissionMountRead, element))
	if err != nil {
		return corecontract.MountRead{}, err
	}
	core.owner.RLock()
	if !core.settled(approval) {
		core.owner.RUnlock()
		return corecontract.MountRead{}, corecontract.ErrUnavailable
	}
	core.owner.RUnlock()
	return result, nil
}

func (core *Operations) ReadMountOperation(actor values.PrincipalRef, element values.ElementID, id corecontract.OperationID) (corecontract.MountControlResult, error) {
	if core == nil || !element.Valid() || !commitbinding.ValidOperationID(id) {
		return corecontract.MountControlResult{}, corecontract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(corecontract.PermissionMountRead, element))
	if err != nil {
		return corecontract.MountControlResult{}, err
	}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return corecontract.MountControlResult{}, corecontract.ErrStore
	}
	if !core.settled(approval) {
		core.owner.RUnlock()
		return corecontract.MountControlResult{}, corecontract.ErrUnavailable
	}
	operation, exists := core.owner.MountOperationLocked(id)
	core.owner.RUnlock()
	approval, err = core.authorize(actor, core.requirement(corecontract.PermissionMountRead, element))
	if err != nil {
		return corecontract.MountControlResult{}, err
	}
	core.owner.RLock()
	if !core.settled(approval) {
		core.owner.RUnlock()
		return corecontract.MountControlResult{}, corecontract.ErrUnavailable
	}
	core.owner.RUnlock()
	if !exists || operation.Binding.Command.MountAnchorUUID != element {
		return corecontract.MountControlResult{}, corecontract.ErrNotFound
	}
	return operation.Result(id), nil
}
