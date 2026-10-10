package mount

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// AuthorizeMountAcceptance captures the original requester's target-local
// authorization basis. The view is private service evidence, never a wire grant.
// The authorization runs outside the fact lock; only the open-owner check and
// cut capture happen under it.
func (core *Operations) AuthorizeMountAcceptance(ctx context.Context, caller values.CallerContext, element values.ElementID) (values.AuthorizationView, error) {
	if core == nil || !core.admission.Enabled() {
		return values.AuthorizationView{}, values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() {
		return values.AuthorizationView{}, values.PublicInvalid
	}
	if _, err := core.captureReadableCut(); err != nil {
		return values.AuthorizationView{}, err
	}
	view, _, err := core.admission.Authorize(ctx, caller, []values.Requirement{core.requirement(corecontract.PermissionMountAccept, element)})
	return view, err
}

// CheckMountAcceptance validates the captured view, including credential
// liveness and authorization ABA, without refreshing it into a new approval.
// The online recheck runs before the lock; the in-lock step only guards with
// the evidence this recheck minted.
func (core *Operations) CheckMountAcceptance(ctx context.Context, caller values.CallerContext, element values.ElementID, view values.AuthorizationView) error {
	if core == nil || !core.admission.Enabled() {
		return values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() || view == (values.AuthorizationView{}) {
		return values.PublicInvalid
	}
	cut, err := core.captureReadableCut()
	if err != nil {
		return err
	}
	requirements := []values.Requirement{core.requirement(corecontract.PermissionMountAccept, element)}
	permit, err := core.admission.Recheck(ctx, caller, view, requirements, false)
	if err != nil {
		return err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || core.owner.CurrentCutLocked() != cut {
		return values.PublicUnavailable
	}
	return core.admission.Validate(ctx, caller, view, requirements, false, permit)
}
