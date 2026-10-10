package transfer

import (
	"context"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
)

// Only an authenticated designated source or target may read the parent's
// retained plan. Public caller controls do not accept these opaque proofs.
func (o *Operations) carrierPeerPlan(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding) (fact.CarrierControl, fact.CarrierBinding, error) {
	if o == nil || ctx == nil || b.Parent != o.owner.AuthorityRef() || !peer.Valid() || (peer != b.Command.Source && peer != b.Command.Target) {
		return fact.CarrierControl{}, fact.CarrierBinding{}, values.PublicDenied
	}
	if err := o.admission.Acquire(); err != nil {
		return fact.CarrierControl{}, fact.CarrierBinding{}, err
	}
	defer o.admission.Release()
	o.owner.RLock()
	defer o.owner.RUnlock()
	if ctx.Err() != nil {
		return fact.CarrierControl{}, fact.CarrierBinding{}, values.PublicUnavailable
	}
	return o.owner.CarrierPlanLocked(b)
}
func (o *Operations) ReadCarrierPlan(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierPlan, error) {
	c, _, err := o.carrierPeerPlan(ctx, peer, b)
	if err != nil {
		return transition.CarrierPlan{}, err
	}
	return exchange.PlanCarrier(b, c), nil
}
func (o *Operations) ReadCarrierDecision(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierDecision, error) {
	_, d, err := o.carrierPeerPlan(ctx, peer, b)
	if err != nil {
		return transition.CarrierDecision{}, err
	}
	if !d.Valid() {
		return transition.CarrierDecision{}, values.PublicUnavailable
	}
	return exchange.DecideCarrier(b, d), nil
}

func carrierPlan(ctx context.Context, b contract.CarrierApprovalBinding, t transition.CarrierTransport) (fact.CarrierControl, error) {
	p, err := t.ReadCarrierPlan(ctx, b.Command.ParentAddress, b.Parent, b)
	if err != nil {
		return fact.CarrierControl{}, err
	}
	actual, c := exchange.OpenCarrierPlan(p)
	if actual != b || !carrierInputMatches(b, c.Reservation.Input) || !c.Valid() {
		return fact.CarrierControl{}, contract.ErrConflict
	}
	return c, nil
}
func carrierInputMatches(b contract.CarrierApprovalBinding, i fact.CarrierIntent) bool {
	return i.Valid() && i.Parent == b.Parent && i.Operation == b.Operation && i.Source == b.Command.Source && i.Target == b.Command.Target && i.Mount == b.Command.Mount &&
		i.ParentAddress == b.Command.ParentAddress && i.SourceAddress == b.Command.SourceAddress && i.TargetAddress == b.Command.TargetAddress
}
func carrierDecision(ctx context.Context, b contract.CarrierApprovalBinding, t transition.CarrierTransport) (fact.CarrierBinding, error) {
	v, err := t.ReadCarrierDecision(ctx, b.Command.ParentAddress, b.Parent, b)
	if err != nil {
		return fact.CarrierBinding{}, err
	}
	actual, d := exchange.OpenCarrierDecision(v)
	if actual != b || !d.Valid() || !carrierInputMatches(b, d.Acceptance.Fence.Permit.Reservation.Input) {
		return fact.CarrierBinding{}, contract.ErrConflict
	}
	return d, nil
}
