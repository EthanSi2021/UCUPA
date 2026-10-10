package mount

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
)

func (core *Operations) mountControlRequirements(command corecontract.MountCommand) []values.Requirement {
	requirements := []values.Requirement{core.requirement(corecontract.PermissionMountControl, command.MountAnchorUUID)}
	if command.Kind == corecontract.MountUpdate {
		requirements = append(requirements, core.requirement(corecontract.PermissionUpdateElement, command.MountAnchorUUID), core.requirement(corecontract.PermissionPlaceElement, command.Final.Parent))
	}
	return requirements
}

// AuthorizeMountControl captures a target-local original authorization view.
// This is trusted adapter evidence, not a remotely acceptable user assertion.
// The authorization itself runs outside the fact lock; only the open-owner
// check and cut capture happen under it.
func (core *Operations) AuthorizeMountControl(ctx context.Context, caller values.CallerContext, command corecontract.MountCommand) (values.AuthorizationView, error) {
	if core == nil || !core.admission.Enabled() {
		return values.AuthorizationView{}, values.PublicUnavailable
	}
	if ctx == nil {
		return values.AuthorizationView{}, values.PublicInvalid
	}
	command, ok := commitpkg.CanonicalMountCommand(command)
	if !ok {
		return values.AuthorizationView{}, values.PublicInvalid
	}
	if _, err := core.captureReadableCut(); err != nil {
		return values.AuthorizationView{}, err
	}
	view, _, err := core.admission.Authorize(ctx, caller, core.mountControlRequirements(command))
	return view, err
}

// ApplyMountControl uses the captured original parent view and checks the
// carrier before reserving. It reuses the same transitions as local ApplyMount,
// but never treats a bare LocalCallers map as network authentication. The
// captured view is rechecked online before the lock; each in-lock step only
// guards with the evidence this one operation minted.
func (core *Operations) ApplyMountControl(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, id corecontract.OperationID, command corecontract.MountCommand, transport corecontract.MountControlTransport) (corecontract.MountControlResult, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.MountControlResult{}, values.PublicUnavailable
	}
	canonical, ok := commitpkg.CanonicalMountCommand(command)
	if ctx == nil || !ok || !commitbinding.ValidOperationID(id) || view == (values.AuthorizationView{}) {
		return corecontract.MountControlResult{}, values.PublicInvalid
	}
	if ctx.Err() != nil {
		return corecontract.MountControlResult{}, values.PublicUnavailable
	}
	if err := core.admission.Acquire(); err != nil {
		return corecontract.MountControlResult{}, err
	}
	defer core.admission.Release()
	requirements := core.mountControlRequirements(canonical)
	permit, err := core.admission.Recheck(ctx, caller, view, requirements, false)
	if err != nil {
		return corecontract.MountControlResult{}, err
	}
	guard := &controlAuthorization{ctx: ctx, caller: caller, view: view, requirements: requirements, permit: permit}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return corecontract.MountControlResult{}, values.PublicUnavailable
	}
	if err := guard.validate(core.admission); err != nil {
		core.owner.RUnlock()
		return corecontract.MountControlResult{}, err
	}
	plan := core.owner.MountControlPlanLocked(id, canonical)
	core.owner.RUnlock()
	if plan.Carrier == core.owner.AuthorityRef() {
		return corecontract.MountControlResult{}, values.PublicInvalid
	}
	peer := plan.Peer
	if plan.Approval {
		if transport == nil {
			return corecontract.MountControlResult{}, values.PublicUnavailable
		}
		var err error
		peer, err = transport.CheckApproval(ctx, plan.Address, plan.Carrier, id, canonical)
		if err != nil {
			return corecontract.MountControlResult{}, err
		}
		if !peer.Valid() {
			return corecontract.MountControlResult{}, values.PublicDenied
		}
	}
	// The parent relationship may have changed while B checked the approval.
	// Recheck the actual contact/required proof under the reservation lock.
	reserveGuard := &reservation{ctx: ctx, authorization: guard, id: id, command: canonical, plan: plan}
	callers := corecontract.LocalCallers{core.owner.AuthorityRef(): caller.Subject, plan.Carrier: peer}
	if err := core.reserveMountIntentGuarded(callers, id, commitpkg.MountBinding{Actor: caller.Subject, Command: canonical}, reserveGuard); err != nil {
		return corecontract.MountControlResult{}, err
	}
	result, err := core.driveMountGuarded(ctx, id, transport, guard)
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.MountControlResult{}, values.PublicUnavailable
	}
	if authorization := guard.validate(core.admission); authorization != nil {
		return corecontract.MountControlResult{}, authorization
	}
	return result, err
}
