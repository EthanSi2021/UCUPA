package transfer

import (
	"context"
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/transfer/exchange"
)

func (o *Operations) FenceCarrierSource(ctx context.Context, peer values.FactAuthorityRef, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView, t transition.CarrierTransport) (transition.CarrierFrozen, error) {
	r, err := o.carrierRequirements(b, false)
	if err != nil || ctx == nil || t == nil || peer != b.Parent {
		return transition.CarrierFrozen{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.CarrierFrozen{}, err
	}
	defer o.admission.Release()
	// The online recheck of the retained view runs before any lock is held; the
	// in-lock steps only guard with the evidence this recheck minted.
	o.owner.RLock()
	var check bool
	if !o.owner.HasCarrierSourceFenceLocked(b.Parent, b.Operation) {
		check = true
	} else {
		err = o.owner.CheckTransferTargetAdmissionLocked()
	}
	o.owner.RUnlock()
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	var permit access.Permit
	if check {
		// A missing runtime approval cannot be rechecked online and must not
		// reach the peer; first acceptance needs one, exactly like the
		// in-lock check that follows.
		if caller == (values.CallerContext{}) || v == (values.AuthorizationView{}) {
			return transition.CarrierFrozen{}, values.PublicDenied
		}
		permit, err = o.admission.Recheck(ctx, caller, v, r, false)
		if err != nil {
			return transition.CarrierFrozen{}, err
		}
	}
	if ctx.Err() != nil {
		return transition.CarrierFrozen{}, values.PublicUnavailable
	}
	c, err := carrierPlan(ctx, b, t)
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	if c.PermitAt == (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
		return transition.CarrierFrozen{}, values.PublicUnavailable
	}
	i := c.Reservation.Input
	o.owner.RLock()
	f, raw, oldErr := o.owner.CarrierSourceFenceLocked(i.Actor, i.Key())
	o.owner.RUnlock()
	if oldErr == nil {
		if ctx.Err() != nil {
			return transition.CarrierFrozen{}, values.PublicUnavailable
		}
		if f.Permit != c {
			return transition.CarrierFrozen{}, contract.ErrOperationConflict
		}
		return exchange.FreezeCarrier(b, f, raw), nil
	}
	if !errors.Is(oldErr, contract.ErrNotFound) {
		return transition.CarrierFrozen{}, oldErr
	}
	// Target preflight is rechecked immediately before first source withdrawal.
	// It is not the target's durable content acceptance and is never renewed here.
	if err := t.CheckCarrierImport(ctx, b.Command.TargetAddress, b.Command.Target, b); err != nil {
		return transition.CarrierFrozen{}, err
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	// A concurrent exact call may already have fenced while the peer was checked.
	if f, raw, err := o.owner.CarrierSourceFenceLocked(i.Actor, i.Key()); err == nil {
		if ctx.Err() != nil {
			return transition.CarrierFrozen{}, values.PublicUnavailable
		}
		if f.Permit != c {
			return transition.CarrierFrozen{}, contract.ErrOperationConflict
		}
		return exchange.FreezeCarrier(b, f, raw), nil
	} else if !errors.Is(err, contract.ErrNotFound) {
		return transition.CarrierFrozen{}, err
	}
	if err := o.checkImportLocked(ctx, caller, v, r, permit); err != nil {
		return transition.CarrierFrozen{}, err
	}
	if err := o.owner.CheckUnconnectedCarrierSourceLocked(c); err != nil {
		return transition.CarrierFrozen{}, err
	}
	f, raw, err = o.owner.FenceCarrierSourceLocked(i.Actor, c)
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	return exchange.FreezeCarrier(b, f, raw), nil
}

func (o *Operations) ReadCarrierSource(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding, t transition.CarrierTransport) (transition.CarrierFrozen, error) {
	if _, err := o.carrierRequirements(b, false); err != nil || ctx == nil || t == nil || (peer != b.Parent && peer != b.Command.Target) {
		return transition.CarrierFrozen{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.CarrierFrozen{}, err
	}
	defer o.admission.Release()
	c, err := carrierPlan(ctx, b, t)
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return transition.CarrierFrozen{}, values.PublicUnavailable
	}
	f, raw, err := o.owner.CarrierSourceFenceLocked(c.Reservation.Input.Actor, c.Reservation.Input.Key())
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	if f.Permit != c {
		return transition.CarrierFrozen{}, contract.ErrOperationConflict
	}
	return exchange.FreezeCarrier(b, f, raw), nil
}

func (o *Operations) SettleCarrierSource(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding, t transition.CarrierTransport) error {
	if _, err := o.carrierRequirements(b, false); err != nil || ctx == nil || t == nil || peer != b.Parent {
		return values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return err
	}
	defer o.admission.Release()
	v, err := t.ReadCarrierCompletion(ctx, b.Command.TargetAddress, b.Command.Target, b)
	if err != nil {
		return err
	}
	c := exchange.OpenCompletion(v)
	if !c.Valid() || !carrierInputMatches(b, c.Installed.Carrier.Binding.Acceptance.Fence.Permit.Reservation.Input) {
		return contract.ErrConflict
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	_, err = o.owner.SettleCarrierSourceLocked(c)
	return err
}
