package transfer

import (
	"context"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func (o *Operations) carrierRequirements(b contract.CarrierApprovalBinding, importing bool) ([]values.Requirement, error) {
	if o == nil || len(b.Operation) == 0 || len(b.Operation) > 128 || !commit.PublicCarrierInputValid(b.Parent, b.Command) {
		return nil, values.PublicInvalid
	}
	owner, permission := b.Command.Source, contract.PermissionTransitionExport
	if importing {
		owner, permission = b.Command.Target, contract.PermissionTransitionImport
	}
	if owner != o.owner.AuthorityRef() {
		return nil, values.PublicInvalid
	}
	return []values.Requirement{{Permission: permission, Scope: values.CoreElementScope(owner, b.Command.Mount)}}, nil
}

func (o *Operations) AuthorizeCarrierExport(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding) (values.AuthorizationView, error) {
	return o.authorizeCarrier(ctx, caller, b, false)
}
func (o *Operations) AuthorizeCarrierImport(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding) (values.AuthorizationView, error) {
	return o.authorizeCarrier(ctx, caller, b, true)
}
func (o *Operations) authorizeCarrier(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, importing bool) (values.AuthorizationView, error) {
	r, err := o.carrierRequirements(b, importing)
	if err != nil || ctx == nil {
		return values.AuthorizationView{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return values.AuthorizationView{}, err
	}
	defer o.admission.Release()
	v, permit, err := o.admission.Authorize(ctx, caller, r)
	if err != nil {
		return values.AuthorizationView{}, err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	if err := o.checkImportLocked(ctx, caller, v, r, permit); err != nil {
		return values.AuthorizationView{}, err
	}
	return v, nil
}
func (o *Operations) CheckCarrierExport(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView) error {
	return o.checkCarrierApproval(ctx, caller, b, v, false)
}
func (o *Operations) CheckCarrierImport(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView) error {
	return o.checkCarrierApproval(ctx, caller, b, v, true)
}
func (o *Operations) checkCarrierApproval(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView, importing bool) error {
	r, err := o.carrierRequirements(b, importing)
	if err != nil || ctx == nil {
		return values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return err
	}
	defer o.admission.Release()
	// A missing runtime approval cannot be rechecked and must not reach
	// first-acceptance authorization, exactly like the in-lock check.
	if caller == (values.CallerContext{}) || v == (values.AuthorizationView{}) {
		return values.PublicDenied
	}
	permit, err := o.admission.Recheck(ctx, caller, v, r, false)
	if err != nil {
		return err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	return o.checkImportLocked(ctx, caller, v, r, permit)
}
