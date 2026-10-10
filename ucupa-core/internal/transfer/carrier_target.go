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

func (o *Operations) AcceptCarrierTarget(ctx context.Context, peer values.FactAuthorityRef, caller values.CallerContext, b contract.CarrierApprovalBinding, v values.AuthorizationView, t transition.CarrierTransport) (transition.CarrierAcceptance, error) {
	r, err := o.carrierRequirements(b, true)
	if err != nil || ctx == nil || t == nil || peer != b.Parent {
		return transition.CarrierAcceptance{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.CarrierAcceptance{}, err
	}
	defer o.admission.Release()
	// The online recheck of the retained view runs before any lock is held; the
	// in-lock steps only guard with the evidence this recheck minted.
	o.owner.RLock()
	var check bool
	if !o.owner.HasTransferAcceptanceLocked(b.Parent, b.Operation) {
		check = true
	} else {
		err = o.owner.CheckTransferTargetAdmissionLocked()
	}
	o.owner.RUnlock()
	if err != nil {
		return transition.CarrierAcceptance{}, err
	}
	var permit access.Permit
	if check {
		// A missing runtime approval cannot be rechecked online and must not
		// reach the peer; first acceptance needs one, exactly like the
		// in-lock check that follows.
		if caller == (values.CallerContext{}) || v == (values.AuthorizationView{}) {
			return transition.CarrierAcceptance{}, values.PublicDenied
		}
		permit, err = o.admission.Recheck(ctx, caller, v, r, false)
		if err != nil {
			return transition.CarrierAcceptance{}, err
		}
	}
	c, err := carrierPlan(ctx, b, t)
	if err != nil {
		return transition.CarrierAcceptance{}, err
	}
	frozen, err := t.ReadCarrierSource(ctx, b.Command.SourceAddress, b.Command.Source, b)
	if err != nil {
		return transition.CarrierAcceptance{}, err
	}
	actual, f, raw := exchange.OpenCarrierFrozen(frozen)
	if actual != b || !f.Valid() || f.Permit != c {
		return transition.CarrierAcceptance{}, contract.ErrConflict
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return transition.CarrierAcceptance{}, values.PublicUnavailable
	}
	if !o.owner.HasTransferAcceptanceLocked(b.Parent, b.Operation) {
		if err := o.checkImportLocked(ctx, caller, v, r, permit); err != nil {
			return transition.CarrierAcceptance{}, err
		}
	}
	// Parent actor is authenticated provenance, never the target permission identity.
	a, err := o.owner.AcceptCarrierTargetLocked(c.Reservation.Input.Actor, f, raw, nil)
	if err != nil {
		return transition.CarrierAcceptance{}, err
	}
	return exchange.AcceptCarrier(a), nil
}

func (o *Operations) InstallCarrierTarget(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding, t transition.CarrierTransport) (transition.Completion, error) {
	if _, err := o.carrierRequirements(b, true); err != nil || ctx == nil || t == nil || peer != b.Parent {
		return transition.Completion{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.Completion{}, err
	}
	defer o.admission.Release()
	d, err := carrierDecision(ctx, b, t)
	if err != nil {
		return transition.Completion{}, err
	}
	o.owner.Lock()
	defer o.owner.Unlock()
	if ctx.Err() != nil {
		return transition.Completion{}, values.PublicUnavailable
	}
	i, err := o.owner.InstallCarrierTargetLocked(d)
	if err != nil {
		return transition.Completion{}, err
	}
	c, err := o.owner.CompleteTransferTargetLocked(fact.CoreInstallation{Carrier: i}, nil)
	if err != nil {
		return transition.Completion{}, err
	}
	return exchange.Complete(c), nil
}

func (o *Operations) ReadCarrierCompletion(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding, t transition.CarrierTransport) (transition.Completion, error) {
	if _, err := o.carrierRequirements(b, true); err != nil || ctx == nil || t == nil || (peer != b.Parent && peer != b.Command.Source) {
		return transition.Completion{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.Completion{}, err
	}
	defer o.admission.Release()
	d, err := carrierDecision(ctx, b, t)
	if err != nil {
		return transition.Completion{}, err
	}
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return transition.Completion{}, values.PublicUnavailable
	}
	c, err := o.owner.CarrierCompletionLocked(d)
	if err != nil {
		return transition.Completion{}, err
	}
	return exchange.Complete(c), nil
}
