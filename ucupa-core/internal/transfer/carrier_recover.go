package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
)

// RecoverCarrierTransition is parent-owned orchestration over the existing
// durable milestones. No peer call is made under the parent's state lock.
func (o *Operations) RecoverCarrierTransition(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, t transition.CarrierTransport) (contract.CarrierStatus, error) {
	if o == nil || ctx == nil || t == nil || b.Parent != o.owner.AuthorityRef() || !caller.Subject.Valid() ||
		len(b.Operation) == 0 || len(b.Operation) > 128 || !commit.PublicCarrierInputValid(b.Parent, b.Command) {
		return contract.CarrierStatus{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return contract.CarrierStatus{}, err
	}
	defer o.admission.Release()
	inspect := []values.Requirement{{Permission: contract.PermissionTransitionInspect, Scope: values.CoreElementScope(b.Parent, b.Command.Mount)}}
	readView, readPermit, err := o.admission.Authorize(ctx, caller, inspect)
	defer o.admission.ReleasePermit(ctx, readPermit)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	o.owner.RLock()
	if err = o.validateRecoveryLocked(ctx, caller, readView, readPermit, inspect); err != nil {
		o.owner.RUnlock()
		return contract.CarrierStatus{}, err
	}
	status, err := o.owner.InspectPublicCarrierLocked(caller.Subject, b.Operation, b.Command.Mount)
	var c fact.CarrierControl
	var decision fact.CarrierBinding
	if err == nil {
		c, decision, err = o.owner.CarrierPlanLocked(b)
	}
	o.owner.RUnlock()
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	if status.Progress == contract.CarrierCancelled || status.Progress == contract.CarrierSettled {
		return o.finishCarrierRecovery(ctx, caller, b, readView, readPermit, inspect)
	}
	if !decision.Valid() {
		write := []values.Requirement{{Permission: contract.PermissionTransitionCommit, Scope: values.CoreElementScope(b.Parent, b.Command.Mount)}}
		writeView, writePermit, err := o.admission.Authorize(ctx, caller, write)
		defer o.admission.ReleasePermit(ctx, writePermit)
		if err != nil {
			return contract.CarrierStatus{}, err
		}
		if c.PermitAt == (contract.CoreReadCut{}) {
			if err := t.CheckCarrierExport(ctx, b.Command.SourceAddress, b.Command.Source, b); err != nil {
				return contract.CarrierStatus{}, err
			}
			if err := t.CheckCarrierImport(ctx, b.Command.TargetAddress, b.Command.Target, b); err != nil {
				return contract.CarrierStatus{}, err
			}
			c, err = o.permitCarrierRecovery(ctx, caller, b, writeView, writePermit, write)
			if err != nil {
				return contract.CarrierStatus{}, err
			}
		}
		frozen, err := t.FenceCarrierSource(ctx, b.Command.SourceAddress, b.Command.Source, b)
		if err != nil {
			return contract.CarrierStatus{}, err
		}
		actual, fence, _ := exchange.OpenCarrierFrozen(frozen)
		if actual != b || !fence.Valid() || fence.Permit != c {
			return contract.CarrierStatus{}, contract.ErrConflict
		}
		accepted, err := t.AcceptCarrierTarget(ctx, b.Command.TargetAddress, b.Command.Target, b)
		if err != nil {
			return contract.CarrierStatus{}, err
		}
		a := exchange.OpenCarrierAcceptance(accepted)
		if !a.Valid() || a.Fence != fence {
			return contract.CarrierStatus{}, contract.ErrConflict
		}
		decision, err = o.commitCarrierRecovery(ctx, caller, b, a, writeView, writePermit, write)
		if err != nil {
			return contract.CarrierStatus{}, err
		}
	}
	installed, err := t.InstallCarrierTarget(ctx, b.Command.TargetAddress, b.Command.Target, b)
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	completion := exchange.OpenCompletion(installed)
	if !completion.Valid() || completion.Installed.Carrier.Binding != decision {
		return contract.CarrierStatus{}, contract.ErrConflict
	}
	if err := t.SettleCarrierSource(ctx, b.Command.SourceAddress, b.Command.Source, b); err != nil {
		return contract.CarrierStatus{}, err
	}
	o.owner.Lock()
	if ctx.Err() != nil {
		err = values.PublicUnavailable
	} else {
		_, err = o.owner.SettleCarrierParentLocked(completion)
	}
	o.owner.Unlock()
	if err != nil {
		return contract.CarrierStatus{}, err
	}
	return o.finishCarrierRecovery(ctx, caller, b, readView, readPermit, inspect)
}

func (o *Operations) permitCarrierRecovery(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView, permit access.Permit, requirements []values.Requirement) (fact.CarrierControl, error) {
	o.owner.Lock()
	defer o.owner.Unlock()
	if err := o.validateRecoveryLocked(ctx, caller, v, permit, requirements); err != nil {
		return fact.CarrierControl{}, err
	}
	c, _, err := o.owner.CarrierPlanLocked(b)
	if err != nil {
		return fact.CarrierControl{}, err
	}
	if c.CancelledAt != (contract.CoreReadCut{}) {
		return fact.CarrierControl{}, contract.ErrConflict
	}
	return o.owner.PermitCarrierFenceLocked(caller.Subject, b.Operation, c.Reservation.At)
}

func (o *Operations) commitCarrierRecovery(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, a fact.CarrierAcceptance, v values.AuthorizationView, permit access.Permit, requirements []values.Requirement) (fact.CarrierBinding, error) {
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return fact.CarrierBinding{}, values.PublicUnavailable
	}
	c, d, err := o.owner.CarrierPlanLocked(b)
	if err != nil {
		return fact.CarrierBinding{}, err
	}
	if c != a.Fence.Permit {
		return fact.CarrierBinding{}, contract.ErrOperationConflict
	}
	// A repeated committed decision fulfils the original obligation; it is not
	// a second grant. The transaction still compares the complete acceptance.
	if !d.Valid() {
		if err := o.validateRecoveryLocked(ctx, caller, v, permit, requirements); err != nil {
			return fact.CarrierBinding{}, err
		}
	}
	return o.owner.CommitCarrierBindingLocked(caller.Subject, a)
}

func (o *Operations) finishCarrierRecovery(ctx context.Context, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView, permit access.Permit, requirements []values.Requirement) (contract.CarrierStatus, error) {
	o.owner.RLock()
	defer o.owner.RUnlock()
	if err := o.validateRecoveryLocked(ctx, caller, v, permit, requirements); err != nil {
		return contract.CarrierStatus{}, err
	}
	return o.owner.InspectPublicCarrierLocked(caller.Subject, b.Operation, b.Command.Mount)
}
