package mount

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
)

// DescribeMountApproval derives only the target and exact command commitment.
// It creates neither an intent nor a grant. A later Apply captures a new view.
func (core *Operations) DescribeMountApproval(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, input corecontract.PublicMountCommand) (values.FactAuthorityRef, [32]byte, bool, error) {
	command, view, permit, err := core.resolveMountControl(ctx, caller, id, input)
	if err != nil {
		return "", [32]byte{}, false, err
	}
	defer core.admission.ReleasePermit(ctx, permit)
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return "", [32]byte{}, false, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, core.mountControlRequirements(command), false, permit); err != nil {
		core.owner.RUnlock()
		return "", [32]byte{}, false, err
	}
	plan := core.owner.MountControlPlanLocked(id, command)
	if !plan.Approval {
		core.owner.RUnlock()
		return "", [32]byte{}, false, nil
	}
	digest, err := core.CommandCommitment(core.owner.AuthorityRef(), plan.Carrier, id, command)
	core.owner.RUnlock()
	return plan.Carrier, digest, true, err
}

func (core *Operations) InspectMountControl(ctx context.Context, caller values.CallerContext, element values.ElementID) (corecontract.PublicMountInspection, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.PublicMountInspection{}, values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() {
		return corecontract.PublicMountInspection{}, values.PublicInvalid
	}
	cut, err := core.captureReadableCut()
	if err != nil {
		return corecontract.PublicMountInspection{}, err
	}
	requirements := []values.Requirement{core.requirement(corecontract.PermissionMountRead, element)}
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.PublicMountInspection{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || core.owner.CurrentCutLocked() != cut {
		return corecontract.PublicMountInspection{}, values.PublicUnavailable
	}
	anchor, exists := core.owner.FactsLocked().Elements.Anchor(element)
	if !exists {
		return corecontract.PublicMountInspection{}, values.PublicNotFound
	}
	record := core.owner.FactsLocked().Mount.Record(element)
	result := corecontract.PublicMountInspection{Element: element,
		ElementRevision: core.owner.SigningLocked().ElementRevision(anchor.Revision),
		MountRevision:   core.owner.SigningLocked().MountRevision(record.Revision), State: record.State,
		Carrier: record.Key.CarrierAuthority, NextHop: record.Contact.NextHop}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.PublicMountInspection{}, err
	}
	return result, nil
}

// ResolveMountControl resolves current comparisons or an exact retained retry.
// The returned command/view are private trusted-adapter evidence; wire adapters
// expose only the commitment, never serialize the returned command as a public
// response. ApplyMountControl revalidates the captured view before reserving.
// On success the adapter owns release and must call it once, outside fact locks,
// after the final use of view (including failure before ApplyMountControl).
func (core *Operations) ResolveMountControl(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, input corecontract.PublicMountCommand) (corecontract.MountCommand, values.AuthorizationView, func(), error) {
	command, view, permit, err := core.resolveMountControl(ctx, caller, id, input)
	if err != nil {
		return command, view, nil, err
	}
	return command, view, func() { core.admission.ReleasePermit(ctx, permit) }, nil
}

// resolveMountControl transfers cleanup only on success. A later Apply mints
// its own permit by rechecking online, but still needs the original view live.
func (core *Operations) resolveMountControl(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, input corecontract.PublicMountCommand) (corecontract.MountCommand, values.AuthorizationView, accesspkg.Permit, error) {
	var empty corecontract.MountCommand
	if core == nil || !core.admission.Enabled() {
		return empty, values.AuthorizationView{}, nil, values.PublicUnavailable
	}
	if !commitbinding.ValidOperationID(id) || !input.Element.Valid() {
		return empty, values.AuthorizationView{}, nil, values.PublicInvalid
	}
	command := corecontract.MountCommand{Kind: input.Kind, MountAnchorUUID: input.Element,
		CarrierAuthority: input.Carrier, CarrierNextHop: input.CarrierNextHop,
		ParentNextHop: input.ParentNextHop, Final: input.Final}
	// Check the closed variant before looking up any facts. Dummy revisions are
	// shape checks only and never used as an accepted comparison.
	dummy := corecontract.ServiceRevision{Incarnation: [32]byte{1}, Element: input.Element, Sequence: 1}
	switch input.Kind {
	case corecontract.MountEstablish, corecontract.MountUpdate:
		if !input.ExpectedElement.Valid() || input.ExpectedElement.Owner != core.owner.AuthorityRef() || input.ExpectedElement.Kind != "record-revision" {
			return empty, values.AuthorizationView{}, nil, values.PublicInvalid
		}
		command.ExpectedElementRevision, _ = dummy.ElementRevision()
	case corecontract.MountRevoke:
		if input.ExpectedElement != (values.PublicToken{}) {
			return empty, values.AuthorizationView{}, nil, values.PublicInvalid
		}
	default:
		return empty, values.AuthorizationView{}, nil, values.PublicInvalid
	}
	if input.Kind == corecontract.MountEstablish {
		if input.ExpectedMount != (values.PublicToken{}) {
			return empty, values.AuthorizationView{}, nil, values.PublicInvalid
		}
	} else {
		if !input.ExpectedMount.Valid() || input.ExpectedMount.Owner != core.owner.AuthorityRef() || input.ExpectedMount.Kind != "mount-revision" {
			return empty, values.AuthorizationView{}, nil, values.PublicInvalid
		}
		command.ExpectedMountRevision, _ = dummy.MountRevision()
	}
	canonical, ok := commitpkg.CanonicalMountCommand(command)
	if !ok || canonical != command {
		return empty, values.AuthorizationView{}, nil, values.PublicInvalid
	}
	cut, err := core.captureReadableCut()
	if err != nil {
		return empty, values.AuthorizationView{}, nil, err
	}
	requirements := core.mountControlRequirements(canonical)
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	transferred := false
	defer func() {
		if !transferred {
			core.admission.ReleasePermit(ctx, permit)
		}
	}()
	if err != nil {
		return empty, values.AuthorizationView{}, nil, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || core.owner.CurrentCutLocked() != cut {
		return empty, values.AuthorizationView{}, nil, values.PublicUnavailable
	}
	command, err = core.owner.ResolvePublicMountLocked(caller.Subject, id, input, canonical)
	if err != nil {
		return empty, values.AuthorizationView{}, nil, err
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return empty, values.AuthorizationView{}, nil, err
	}
	transferred = true
	return command, view, permit, nil
}

// captureReadableCut holds the read lock only long enough to record that the
// owner is open and to capture its current cut, releasing it before any
// authorization wait. A later recheck compares the same cut, so a decision
// made while facts moved is never consumed as current evidence.
func (core *Operations) captureReadableCut() (corecontract.CoreReadCut, error) {
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.CoreReadCut{}, values.PublicUnavailable
	}
	return core.owner.CurrentCutLocked(), nil
}
