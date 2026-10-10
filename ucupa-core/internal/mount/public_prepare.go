package mount

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	commitpkg "ucupa/ucupa-core/internal/commit"
)

// PrepareMountControl is the authenticated carrier-side prepare boundary.
// Trusted assembly supplies source from the authenticated peer, never from a
// consumer field. caller is independently established at this Core and retains
// the original requester, not the forwarding service's principal. The network
// adapter must additionally bind its finite approval to the exact request.
func (core *Operations) PrepareMountControl(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	return core.prepareMountControl(ctx, caller, source, request, values.AuthorizationView{})
}

// PrepareApprovedMountControl consumes the originally captured authorization
// basis, not a fresh view. Transport validates the token/actor/expiry and passes
// its retained caller, view and commitment; none is trusted from message fields.
func (core *Operations) PrepareApprovedMountControl(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request corecontract.MountPrepareRequest, view values.AuthorizationView, commitment [32]byte) (corecontract.MountPrepared, error) {
	if core == nil {
		return corecontract.MountPrepared{}, values.PublicUnavailable
	}
	want, err := core.CommandCommitment(source, core.owner.AuthorityRef(), request.OperationID, request.Command)
	if err != nil || commitment != want || view == (values.AuthorizationView{}) {
		return corecontract.MountPrepared{}, values.PublicInvalid
	}
	return core.prepareMountControl(ctx, caller, source, request, view)
}

func (core *Operations) prepareMountControl(ctx context.Context, caller values.CallerContext, source values.FactAuthorityRef, request corecontract.MountPrepareRequest, view values.AuthorizationView) (corecontract.MountPrepared, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.MountPrepared{}, values.PublicUnavailable
	}
	if ctx == nil || !commitpkg.ValidMountRequest(request) || request.Command.Kind == corecontract.MountRevoke ||
		source != request.Key.ParentAuthority || core.owner.AuthorityRef() != request.Key.CarrierAuthority {
		return corecontract.MountPrepared{}, values.PublicInvalid
	}
	if !caller.Subject.Valid() || caller.Subject != request.CarrierActor {
		return corecontract.MountPrepared{}, values.PublicDenied
	}
	if ctx.Err() != nil {
		return corecontract.MountPrepared{}, values.PublicUnavailable
	}
	if err := core.admission.Acquire(); err != nil {
		return corecontract.MountPrepared{}, err
	}
	defer core.admission.Release()
	requirements := []values.Requirement{core.requirement(corecontract.PermissionMountAccept, request.Key.MountAnchorUUID)}
	var permit accesspkg.Permit
	if view == (values.AuthorizationView{}) {
		var err error
		view, permit, err = core.admission.Authorize(ctx, caller, requirements)
		defer core.admission.ReleasePermit(ctx, permit)
		if err != nil {
			return corecontract.MountPrepared{}, err
		}
	} else {
		var err error
		permit, err = core.admission.Recheck(ctx, caller, view, requirements, false)
		if err != nil {
			return corecontract.MountPrepared{}, err
		}
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || ctx.Err() != nil {
		return corecontract.MountPrepared{}, values.PublicUnavailable
	}
	// Validate under the owner lock immediately before either replay disclosure
	// or the existing durable transition. A valid subject name alone is not a
	// credential, and revoke/regrant cannot revive this captured view.
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.MountPrepared{}, err
	}
	if ctx.Err() != nil {
		return corecontract.MountPrepared{}, values.PublicUnavailable
	}
	return core.prepareMountPeerLocked(source, request, true)
}
