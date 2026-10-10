package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
)

func (o *Operations) importRequirements(b contract.TransitionApprovalBinding) ([]values.Requirement, error) {
	if o == nil || len(b.Operation) == 0 || len(b.Operation) > 128 || b.Command.Target != o.owner.AuthorityRef() || !commit.PublicTransitionInputValid(b.Source, b.Command) {
		return nil, values.PublicInvalid
	}
	r := []values.Requirement{{Permission: contract.PermissionTransitionImport, Scope: values.CoreElementScope(b.Command.Target, b.Command.Boundary)}}
	if b.Command.Kind == contract.TransitionSlice {
		r = append(r, values.Requirement{Permission: contract.PermissionPlaceElement, Scope: values.CoreElementScope(b.Command.Target, b.Command.Final.Parent)})
	}
	return r, nil
}

// AuthorizeTransitionImport captures the target's original caller view. The
// trusted adapter binds it to the exact command and limits its lifetime. This
// is preflight permission only, not placement reservation or durable Accept.
func (o *Operations) AuthorizeTransitionImport(ctx context.Context, caller values.CallerContext, b contract.TransitionApprovalBinding) (values.AuthorizationView, error) {
	r, err := o.importRequirements(b)
	if err != nil || ctx == nil {
		return values.AuthorizationView{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return values.AuthorizationView{}, err
	}
	defer o.admission.Release()
	if ctx.Err() != nil {
		return values.AuthorizationView{}, values.PublicUnavailable
	}
	view, permit, err := o.admission.Authorize(ctx, caller, r)
	if err != nil {
		return values.AuthorizationView{}, err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	if err := o.checkImportLocked(ctx, caller, view, r, permit); err != nil {
		return values.AuthorizationView{}, err
	}
	return view, nil
}

// CheckTransitionImport never replaces a stale view with a newly granted one.
// The retained view is process-local trusted adapter state, not a wire field.
// The online recheck runs before the lock; the in-lock step only guards with
// the evidence that recheck minted.
func (o *Operations) CheckTransitionImport(ctx context.Context, caller values.CallerContext, b contract.TransitionApprovalBinding, view values.AuthorizationView) error {
	r, err := o.importRequirements(b)
	if err != nil || ctx == nil || view == (values.AuthorizationView{}) {
		return values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return err
	}
	defer o.admission.Release()
	permit, err := o.admission.Recheck(ctx, caller, view, r, false)
	if err != nil {
		return err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	return o.checkImportLocked(ctx, caller, view, r, permit)
}

func (o *Operations) checkImportLocked(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements []values.Requirement, permit access.Permit) error {
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	// A missing runtime approval may replay a durable acceptance, but cannot
	// reach first-acceptance authorization, even with a permissive test adapter.
	if caller == (values.CallerContext{}) || view == (values.AuthorizationView{}) {
		return values.PublicDenied
	}
	if err := o.owner.CheckTransferTargetAdmissionLocked(); err != nil {
		return err
	}
	if err := o.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	return nil
}
