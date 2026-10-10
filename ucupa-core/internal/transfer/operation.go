// Package transfer owns caller-authorized transfer control. Facts, durable
// decisions and replay remain in the single commit owner, not this handle.
package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
)

type Operations struct {
	owner     *commit.Owner
	admission *access.Admission
}

func New(owner *commit.Owner, admission *access.Admission) *Operations {
	return &Operations{owner: owner, admission: admission}
}

func (o *Operations) AuthorityRef() values.FactAuthorityRef { return o.owner.AuthorityRef() }

// BeginTransition records an exact intent, not a completed transfer or a
// target approval. Cancelling this phase leaves source content active.
func (o *Operations) BeginTransition(ctx context.Context, caller values.CallerContext, id contract.OperationID, command contract.TransitionCommand) (contract.TransitionStatus, error) {
	if o == nil || !commit.PublicTransitionInputValid(o.owner.AuthorityRef(), command) {
		return contract.TransitionStatus{}, values.PublicInvalid
	}
	return o.control(ctx, caller, id, command.Boundary, &command, values.PublicToken{})
}

func (o *Operations) InspectTransition(ctx context.Context, caller values.CallerContext, id contract.OperationID, boundary values.ElementID) (contract.TransitionStatus, error) {
	return o.control(ctx, caller, id, boundary, nil, values.PublicToken{})
}

func (o *Operations) CancelTransition(ctx context.Context, caller values.CallerContext, id contract.OperationID, boundary values.ElementID, expected values.PublicToken) (contract.TransitionStatus, error) {
	if o == nil || !expected.Valid() || expected.Owner != o.owner.AuthorityRef() || expected.Kind != "transition-revision" {
		return contract.TransitionStatus{}, values.PublicInvalid
	}
	return o.control(ctx, caller, id, boundary, nil, expected)
}

func (o *Operations) control(ctx context.Context, caller values.CallerContext, id contract.OperationID, boundary values.ElementID, command *contract.TransitionCommand, expected values.PublicToken) (contract.TransitionStatus, error) {
	if o == nil || ctx == nil || !caller.Subject.Valid() || len(id) == 0 || len(id) > 128 || !boundary.Valid() {
		return contract.TransitionStatus{}, values.PublicInvalid
	}
	if ctx.Err() != nil {
		return contract.TransitionStatus{}, values.PublicUnavailable
	}
	if err := o.admission.Acquire(); err != nil {
		return contract.TransitionStatus{}, err
	}
	defer o.admission.Release()
	permission := contract.PermissionTransitionInspect
	if command != nil || expected != (values.PublicToken{}) {
		permission = contract.PermissionTransitionExport
	}
	requirements := []values.Requirement{{Permission: permission, Scope: values.CoreElementScope(o.owner.AuthorityRef(), boundary)}}
	view, permit, err := o.admission.Authorize(ctx, caller, requirements)
	defer o.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil || o.owner.ClosedLocked() || !o.owner.AvailableLocked() {
		return contract.TransitionStatus{}, values.PublicUnavailable
	}
	if err := o.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return contract.TransitionStatus{}, err
	}
	if command != nil {
		return o.owner.BeginPublicTransitionLocked(caller.Subject, id, *command)
	}
	if expected != (values.PublicToken{}) {
		return o.owner.CancelPublicTransitionLocked(caller.Subject, id, boundary, expected)
	}
	return o.owner.InspectPublicTransitionLocked(caller.Subject, id, boundary)
}
