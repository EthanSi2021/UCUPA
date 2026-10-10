package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
)

// RecoverTransition advances one exact operation. I/O never owns the commit
// lock. A failed call leaves its durable milestones recoverable by the same ID;
// SDKs and callers do not orchestrate or roll back the intermediate steps.
func (o *Operations) RecoverTransition(ctx context.Context, caller values.CallerContext, expected contract.TransitionApprovalBinding, transport transition.TargetTransport) (contract.TransitionStatus, error) {
	id, boundary := expected.Operation, expected.Command.Boundary
	if o == nil || ctx == nil || transport == nil || !caller.Subject.Valid() || !boundary.Valid() || len(id) == 0 || len(id) > 128 {
		return contract.TransitionStatus{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return contract.TransitionStatus{}, err
	}
	defer o.admission.Release()
	inspect := []values.Requirement{{Permission: contract.PermissionTransitionInspect, Scope: values.CoreElementScope(o.owner.AuthorityRef(), boundary)}}
	readView, readPermit, err := o.admission.Authorize(ctx, caller, inspect)
	defer o.admission.ReleasePermit(ctx, readPermit)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	o.owner.RLock()
	if err = o.validateRecoveryLocked(ctx, caller, readView, readPermit, inspect); err != nil {
		o.owner.RUnlock()
		return contract.TransitionStatus{}, err
	}
	r, b, err := o.owner.PublicTransitionPlanLocked(caller.Subject, id, boundary)
	o.owner.RUnlock()
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	if b != expected {
		return contract.TransitionStatus{}, contract.ErrOperationConflict
	}
	if r.Progress == fact.IntentCancelled || r.Progress == fact.IntentSettled {
		return o.finishRecovery(ctx, caller, id, boundary, readView, readPermit, inspect)
	}
	if r.Progress == fact.IntentPending || r.Progress == fact.IntentFenced {
		write := []values.Requirement{
			{Permission: contract.PermissionTransitionExport, Scope: values.CoreElementScope(b.Source, boundary)},
			{Permission: contract.PermissionTransitionCommit, Scope: values.CoreElementScope(b.Source, boundary)},
		}
		writeView, writePermit, err := o.admission.Authorize(ctx, caller, write)
		defer o.admission.ReleasePermit(ctx, writePermit)
		if err != nil {
			return contract.TransitionStatus{}, err
		}
		if r.Progress == fact.IntentPending {
			if err := transport.CheckImport(ctx, b.Command.TargetAddress, b.Command.Target, b); err != nil {
				return contract.TransitionStatus{}, err
			}
			r, err = o.fenceRecovery(ctx, caller, r, writeView, writePermit, write)
			if err != nil {
				return contract.TransitionStatus{}, err
			}
		}
		if r.Progress == fact.IntentFenced {
			accepted, err := transport.AcceptImport(ctx, b.Command.TargetAddress, b.Command.Target, b)
			if err != nil {
				return contract.TransitionStatus{}, err
			}
			if err := o.commitRecovery(ctx, caller, r, exchange.OpenAcceptance(accepted), writeView, writePermit, write); err != nil {
				return contract.TransitionStatus{}, err
			}
		}
	}
	completed, err := transport.Install(ctx, b.Command.TargetAddress, b.Command.Target, b)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	completion := exchange.OpenCompletion(completed)
	if completion.Installed.Source.Decision.Acceptance.Fence.Input != r.Input {
		return contract.TransitionStatus{}, contract.ErrConflict
	}
	o.owner.Lock()
	if ctx.Err() != nil {
		err = values.PublicUnavailable
	} else {
		_, err = o.owner.SettleTransferSourceLocked(completion)
	}
	o.owner.Unlock()
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	return o.finishRecovery(ctx, caller, id, boundary, readView, readPermit, inspect)
}

func (o *Operations) validateRecoveryLocked(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, permit access.Permit, r []values.Requirement) error {
	if ctx.Err() != nil || o.owner.ClosedLocked() || !o.owner.AvailableLocked() {
		return values.PublicUnavailable
	}
	if err := o.admission.Validate(ctx, caller, view, r, false, permit); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	return nil
}

func (o *Operations) fenceRecovery(ctx context.Context, caller values.CallerContext, expected fact.IntentRecord, view values.AuthorizationView, permit access.Permit, requirements []values.Requirement) (fact.IntentRecord, error) {
	o.owner.Lock()
	defer o.owner.Unlock()
	if err := o.validateRecoveryLocked(ctx, caller, view, permit, requirements); err != nil {
		return fact.IntentRecord{}, err
	}
	r, _, err := o.owner.PublicTransitionPlanLocked(caller.Subject, expected.Input.Operation, expected.Input.Boundary)
	if err != nil {
		return fact.IntentRecord{}, err
	}
	if r.Input != expected.Input {
		return fact.IntentRecord{}, contract.ErrOperationConflict
	}
	if r.SourceClosed() {
		return r, nil
	}
	if r.Progress != fact.IntentPending || r.Revision != expected.Revision {
		return fact.IntentRecord{}, contract.ErrRevisionConflict
	}
	if err := o.owner.CheckUnconnectedTransferSourceLocked(r); err != nil {
		return fact.IntentRecord{}, err
	}
	return o.owner.FenceTransferSourceLocked(caller.Subject, r.Input.Operation, r.Revision, fact.ParentHold{})
}

func (o *Operations) commitRecovery(ctx context.Context, caller values.CallerContext, r fact.IntentRecord, a fact.TargetAcceptance, view values.AuthorizationView, permit access.Permit, requirements []values.Requirement) error {
	if a.Fence.Input != r.Input {
		return contract.ErrConflict
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	current, _, err := o.owner.PublicTransitionPlanLocked(caller.Subject, r.Input.Operation, r.Input.Boundary)
	if err != nil {
		return err
	}
	// Already committed means fulfilment, not a new user authorization decision.
	if current.Progress != fact.IntentCommitted && current.Progress != fact.IntentSettled {
		if err := o.validateRecoveryLocked(ctx, caller, view, permit, requirements); err != nil {
			return err
		}
	}
	_, err = o.owner.CommitTransferSourceLocked(caller.Subject, a, r.FenceRevision)
	return err
}

func (o *Operations) finishRecovery(ctx context.Context, caller values.CallerContext, id contract.OperationID, boundary values.ElementID, view values.AuthorizationView, permit access.Permit, r []values.Requirement) (contract.TransitionStatus, error) {
	o.owner.RLock()
	defer o.owner.RUnlock()
	if err := o.validateRecoveryLocked(ctx, caller, view, permit, r); err != nil {
		return contract.TransitionStatus{}, err
	}
	return o.owner.InspectPublicTransitionLocked(caller.Subject, id, boundary)
}
