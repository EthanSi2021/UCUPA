package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

// These parent-side operations use the same admission and durable owner as
// source transitions. Parent permission is not source export or target import.
func (o *Operations) BeginCarrierTransition(ctx context.Context, caller values.CallerContext, id contract.OperationID, command contract.CarrierCommand) (contract.CarrierStatus, error) {
	if o == nil || !commit.PublicCarrierInputValid(o.owner.AuthorityRef(), command) {
		return contract.CarrierStatus{}, values.PublicInvalid
	}
	return o.carrierControl(ctx, caller, id, command.Mount, &command, values.PublicToken{})
}

func (o *Operations) InspectCarrierTransition(ctx context.Context, caller values.CallerContext, id contract.OperationID, mount values.ElementID) (contract.CarrierStatus, error) {
	return o.carrierControl(ctx, caller, id, mount, nil, values.PublicToken{})
}

func (o *Operations) CancelCarrierTransition(ctx context.Context, caller values.CallerContext, id contract.OperationID, mount values.ElementID, expected values.PublicToken) (contract.CarrierStatus, error) {
	if o == nil || !expected.Valid() || expected.Owner != o.owner.AuthorityRef() || expected.Kind != "carrier-revision" {
		return contract.CarrierStatus{}, values.PublicInvalid
	}
	return o.carrierControl(ctx, caller, id, mount, nil, expected)
}

func (o *Operations) carrierControl(ctx context.Context, caller values.CallerContext, id contract.OperationID, mount values.ElementID, command *contract.CarrierCommand, expected values.PublicToken) (contract.CarrierStatus, error) {
	if o == nil || ctx == nil || !caller.Subject.Valid() || len(id) == 0 || len(id) > 128 || !mount.Valid() {
		return contract.CarrierStatus{}, values.PublicInvalid
	}
	if ctx.Err() != nil {
		return contract.CarrierStatus{}, values.PublicUnavailable
	}
	if err := o.admission.Acquire(); err != nil {
		return contract.CarrierStatus{}, err
	}
	defer o.admission.Release()
	permission := contract.PermissionTransitionInspect
	if command != nil || expected != (values.PublicToken{}) {
		permission = contract.PermissionTransitionCommit
	}
	requirements := []values.Requirement{{Permission: permission, Scope: values.CoreElementScope(o.owner.AuthorityRef(), mount)}}
	view, permit, err := o.admission.Authorize(ctx, caller, requirements)
	defer o.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil || o.owner.ClosedLocked() || !o.owner.AvailableLocked() {
		return contract.CarrierStatus{}, values.PublicUnavailable
	}
	if err := o.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return contract.CarrierStatus{}, err
	}
	if command != nil {
		return o.owner.BeginPublicCarrierLocked(caller.Subject, id, *command)
	}
	if expected != (values.PublicToken{}) {
		return o.owner.CancelPublicCarrierLocked(caller.Subject, id, mount, expected)
	}
	return o.owner.InspectPublicCarrierLocked(caller.Subject, id, mount)
}
