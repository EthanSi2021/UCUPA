package transfer

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
)

// A fixed, authenticated target may fulfil this exact fenced operation. These
// endpoints never impersonate a user, enumerate operations or accept proofs.
func (o *Operations) ReadTransitionFence(ctx context.Context, peer values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Frozen, error) {
	if o == nil || ctx == nil {
		return transition.Frozen{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.Frozen{}, err
	}
	defer o.admission.Release()
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return transition.Frozen{}, values.PublicUnavailable
	}
	r, err := o.owner.TransferPeerRecordLocked(peer, b)
	if err != nil {
		return transition.Frozen{}, err
	}
	f, raw, err := o.owner.TransferSourceFenceLocked(r.Input.Actor, b.Operation)
	if err != nil {
		return transition.Frozen{}, err
	}
	return exchange.Freeze(b, f, raw), nil
}

func (o *Operations) ReadTransitionDecision(ctx context.Context, peer values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Decision, error) {
	if o == nil || ctx == nil {
		return transition.Decision{}, values.PublicInvalid
	}
	if err := o.admission.Acquire(); err != nil {
		return transition.Decision{}, err
	}
	defer o.admission.Release()
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return transition.Decision{}, values.PublicUnavailable
	}
	r, err := o.owner.TransferPeerRecordLocked(peer, b)
	if err != nil {
		return transition.Decision{}, err
	}
	d, err := o.owner.TransferSourceDecisionLocked(r.Input.Actor, b.Operation)
	if err != nil {
		return transition.Decision{}, err
	}
	return exchange.Decide(b, d), nil
}
