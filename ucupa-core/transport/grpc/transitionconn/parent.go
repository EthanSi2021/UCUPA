package transitionconn

import (
	"context"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
)

func (p *Connections) carrierParent(ctx context.Context, address values.NextHop, parent values.FactAuthorityRef, b contract.CarrierApprovalBinding) (pb.TransitionParentServiceClient, *pb.CarrierApprovalBinding, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, contract.ErrUnavailable
	}
	if parent != b.Parent || address != b.Command.ParentAddress || (p.owner != b.Command.Source && p.owner != b.Command.Target) {
		return nil, nil, contract.ErrOwnerMismatch
	}
	w, err := transitionwire.CarrierBindingWire(b)
	if err != nil {
		return nil, nil, err
	}
	c := p.peers[address]
	if c == nil {
		return nil, nil, contract.ErrUnavailable
	}
	actualAddress, actual := c.Target()
	if actualAddress != address || actual != parent {
		return nil, nil, contract.ErrOwnerMismatch
	}
	return c.TransitionParent(), w, nil
}

func (p *Connections) ReadCarrierPlan(ctx context.Context, address values.NextHop, parent values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierPlan, error) {
	c, w, err := p.carrierParent(ctx, address, parent, b)
	if err != nil {
		return transition.CarrierPlan{}, err
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	default:
		return transition.CarrierPlan{}, values.PublicResourceExhausted
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	r, err := c.ReadCarrierPlan(ctx, &pb.ReadCarrierPlanRequest{Binding: w})
	if err != nil || r.GetResult() == nil {
		return transition.CarrierPlan{}, remoteError(err, r.GetFailure())
	}
	v, err := transitionwire.CarrierPlan(b, r.GetResult())
	if err != nil {
		return transition.CarrierPlan{}, contract.ErrOwnerMismatch
	}
	return exchange.PlanCarrier(b, v), nil
}

func (p *Connections) ReadCarrierDecision(ctx context.Context, address values.NextHop, parent values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierDecision, error) {
	c, w, err := p.carrierParent(ctx, address, parent, b)
	if err != nil {
		return transition.CarrierDecision{}, err
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	default:
		return transition.CarrierDecision{}, values.PublicResourceExhausted
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	r, err := c.ReadCarrierDecision(ctx, &pb.ReadCarrierDecisionRequest{Binding: w})
	if err != nil || r.GetResult() == nil {
		return transition.CarrierDecision{}, remoteError(err, r.GetFailure())
	}
	v, err := transitionwire.CarrierDecision(b, r.GetResult())
	if err != nil {
		return transition.CarrierDecision{}, contract.ErrOwnerMismatch
	}
	return exchange.DecideCarrier(b, v), nil
}
