package mount

import (
	"context"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
)

// controlAuthorization retains exactly the original request view and the
// permit minted for this one operation's online recheck. It cannot mint a
// fresh approval, acquire facts or supply an arbitrary transaction hook.
// The retained context is the original request's, so a request that died
// during a wait never passes a boundary on the retained permit.
type controlAuthorization struct {
	ctx          context.Context
	caller       values.CallerContext
	view         values.AuthorizationView
	requirements []values.Requirement
	permit       access.Permit
}

func (a *controlAuthorization) validate(admission *access.Admission) error {
	return admission.Validate(a.ctx, a.caller, a.view, a.requirements, false, a.permit)
}

// reservation is the fixed post-approval check, under the original owner lock.
// Both cancellation checks and the contact/plan comparison preserve their
// positions around authorization; an I/O roundtrip cannot refresh the intent.
type reservation struct {
	ctx           context.Context
	authorization *controlAuthorization
	id            contract.OperationID
	command       contract.MountCommand
	plan          commit.MountControlPlan
}

func (r *reservation) validateLocked(core *Operations) error {
	if r.ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if err := r.authorization.validate(core.admission); err != nil {
		return err
	}
	if r.ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if core.owner.MountControlPlanLocked(r.id, r.command) != r.plan {
		return contract.ErrRevisionConflict
	}
	return nil
}
