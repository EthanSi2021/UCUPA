// Package handoff implements owner-authorized single-Store writer control.
// The commit owner remains the sole durable decision and epoch authority.
package handoff

import (
	"context"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
)

type Operations struct {
	owner     *commit.Owner
	admission *access.Admission
	resources *registry.Registry
}

func New(owner *commit.Owner, admission *access.Admission, resources *registry.Registry) *Operations {
	return &Operations{owner: owner, admission: admission, resources: resources}
}
func (o *Operations) AuthorityRef() values.FactAuthorityRef { return o.owner.AuthorityRef() }

func (o *Operations) InspectWriter(ctx context.Context, c values.CallerContext, operation string) (values.WriterStatus, error) {
	return o.call(ctx, c, operation, nil)
}
func (o *Operations) HandoffWriter(ctx context.Context, c values.CallerContext, r values.WriterHandoffRequest) (values.WriterStatus, error) {
	if !r.Valid() {
		return values.WriterStatus{}, values.PublicInvalid
	}
	return o.call(ctx, c, r.Operation, &r)
}
func (o *Operations) call(ctx context.Context, c values.CallerContext, id string, r *values.WriterHandoffRequest) (values.WriterStatus, error) {
	if o == nil || ctx == nil || !c.Subject.Valid() || len(id) > 128 {
		return values.WriterStatus{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return values.WriterStatus{}, err
	}
	defer o.admission.Release()
	permission := contract.PermissionInspectWriter
	if r != nil {
		permission = contract.PermissionHandoffWriter
	}
	req := []values.Requirement{{Permission: permission, Scope: values.AuthorityScope(o.owner.AuthorityRef())}}
	view, permit, err := o.admission.Authorize(ctx, c, req)
	defer o.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return values.WriterStatus{}, err
	}
	o.owner.Lock()
	if ctx.Err() != nil {
		o.owner.Unlock()
		return values.WriterStatus{}, values.PublicUnavailable
	}
	if err = o.admission.Validate(ctx, c, view, req, false, permit); err != nil {
		o.owner.Unlock()
		return values.WriterStatus{}, err
	}
	var result values.WriterStatus
	if r == nil {
		result, err = o.owner.WriterStatusLocked(id)
	} else {
		result, err = o.owner.HandoffWriterLocked(c.Subject, *r)
		if o.owner.WriterFencedLocked() {
			o.resources.CloseLocked()
		}
	}
	o.owner.Unlock()
	if r != nil {
		o.resources.DrainRetired()
	}
	return result, err
}
