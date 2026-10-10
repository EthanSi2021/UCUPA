package transition

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
)

// CarrierConnection binds three trusted local endpoints and the original
// peer approvals. Its role-bound copies contain no mutable transfer state.
type CarrierConnection struct {
	parent                     port.CarrierParentPeer
	source                     port.CarrierSourcePeer
	target                     port.CarrierTargetPeer
	binding                    contract.CarrierApprovalBinding
	actor                      values.FactAuthorityRef
	sourceCaller, targetCaller values.CallerContext
	sourceView, targetView     values.AuthorizationView
}

func NewCarrier(parent port.CarrierParentPeer, source port.CarrierSourcePeer, target port.CarrierTargetPeer, b contract.CarrierApprovalBinding, sourceCaller values.CallerContext, sourceView values.AuthorizationView, targetCaller values.CallerContext, targetView values.AuthorizationView) (*CarrierConnection, error) {
	c := b.Command
	if nilPort(parent) || nilPort(source) || nilPort(target) || !b.Parent.Valid() || !c.Source.Valid() || !c.Target.Valid() ||
		b.Parent == c.Source || b.Parent == c.Target || c.Source == c.Target ||
		parent.AuthorityRef() != b.Parent || source.AuthorityRef() != c.Source || target.AuthorityRef() != c.Target ||
		!c.ParentAddress.Valid() || !c.SourceAddress.Valid() || !c.TargetAddress.Valid() ||
		c.ParentAddress == c.SourceAddress || c.ParentAddress == c.TargetAddress || c.SourceAddress == c.TargetAddress ||
		len(b.Operation) == 0 || len(b.Operation) > 128 {
		return nil, values.PublicInvalid
	}
	return &CarrierConnection{parent: parent, source: source, target: target, binding: b, actor: b.Parent,
		sourceCaller: sourceCaller, sourceView: sourceView, targetCaller: targetCaller, targetView: targetView}, nil
}

func (c *CarrierConnection) forPeer(actor values.FactAuthorityRef) *CarrierConnection {
	next := *c
	next.actor = actor
	return &next
}

func (c *CarrierConnection) check(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding, role string) error {
	if c == nil || ctx == nil {
		return values.PublicInvalid
	}
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if b != c.binding {
		return contract.ErrOperationConflict
	}
	var want values.FactAuthorityRef
	var next values.NextHop
	var actual values.FactAuthorityRef
	switch role {
	case "parent":
		want, next, actual = b.Parent, b.Command.ParentAddress, c.parent.AuthorityRef()
	case "source":
		want, next, actual = b.Command.Source, b.Command.SourceAddress, c.source.AuthorityRef()
	case "target":
		want, next, actual = b.Command.Target, b.Command.TargetAddress, c.target.AuthorityRef()
	default:
		return values.PublicInvalid
	}
	if address != next || expected != want || actual != want {
		return values.PublicUnavailable
	}
	return nil
}

func (c *CarrierConnection) ReadCarrierPlan(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierPlan, error) {
	if err := c.check(ctx, address, expected, b, "parent"); err != nil {
		return port.CarrierPlan{}, err
	}
	return c.parent.ReadCarrierPlan(ctx, c.actor, b)
}
func (c *CarrierConnection) ReadCarrierDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierDecision, error) {
	if err := c.check(ctx, address, expected, b, "parent"); err != nil {
		return port.CarrierDecision{}, err
	}
	return c.parent.ReadCarrierDecision(ctx, c.actor, b)
}
func (c *CarrierConnection) CheckCarrierExport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	if err := c.check(ctx, address, expected, b, "source"); err != nil {
		return err
	}
	return c.source.CheckCarrierExport(ctx, c.sourceCaller, b, c.sourceView)
}
func (c *CarrierConnection) CheckCarrierImport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	if err := c.check(ctx, address, expected, b, "target"); err != nil {
		return err
	}
	return c.target.CheckCarrierImport(ctx, c.targetCaller, b, c.targetView)
}
func (c *CarrierConnection) FenceCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierFrozen, error) {
	if err := c.check(ctx, address, expected, b, "source"); err != nil {
		return port.CarrierFrozen{}, err
	}
	return c.source.FenceCarrierSource(ctx, c.actor, c.sourceCaller, b, c.sourceView, c.forPeer(b.Command.Source))
}
func (c *CarrierConnection) ReadCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierFrozen, error) {
	if err := c.check(ctx, address, expected, b, "source"); err != nil {
		return port.CarrierFrozen{}, err
	}
	return c.source.ReadCarrierSource(ctx, c.actor, b, c.forPeer(b.Command.Source))
}
func (c *CarrierConnection) AcceptCarrierTarget(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.CarrierAcceptance, error) {
	if err := c.check(ctx, address, expected, b, "target"); err != nil {
		return port.CarrierAcceptance{}, err
	}
	return c.target.AcceptCarrierTarget(ctx, c.actor, c.targetCaller, b, c.targetView, c.forPeer(b.Command.Target))
}
func (c *CarrierConnection) InstallCarrierTarget(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.Completion, error) {
	if err := c.check(ctx, address, expected, b, "target"); err != nil {
		return port.Completion{}, err
	}
	return c.target.InstallCarrierTarget(ctx, c.actor, b, c.forPeer(b.Command.Target))
}
func (c *CarrierConnection) ReadCarrierCompletion(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (port.Completion, error) {
	if err := c.check(ctx, address, expected, b, "target"); err != nil {
		return port.Completion{}, err
	}
	return c.target.ReadCarrierCompletion(ctx, c.actor, b, c.forPeer(b.Command.Target))
}
func (c *CarrierConnection) SettleCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	if err := c.check(ctx, address, expected, b, "source"); err != nil {
		return err
	}
	return c.source.SettleCarrierSource(ctx, c.actor, b, c.forPeer(b.Command.Source))
}
