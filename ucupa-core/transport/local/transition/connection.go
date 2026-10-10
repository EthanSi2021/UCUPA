// Package transition connects two explicitly designated, trusted local Cores.
// This is not a Ref directory, network authentication or a transfer coordinator.
package transition

import (
	"context"
	"reflect"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
)

type Connection struct {
	source  port.SourcePeer
	target  port.TargetPeer
	binding contract.TransitionApprovalBinding
	caller  values.CallerContext
	view    values.AuthorizationView
}

// New fixes an existing target caller/view and exact source/target instances.
// It neither issues approval nor persists another copy of transfer state.
// A zero view can fulfil an existing durable promise but cannot create one.
func New(source port.SourcePeer, target port.TargetPeer, b contract.TransitionApprovalBinding, caller values.CallerContext, view values.AuthorizationView) (*Connection, error) {
	if nilPort(source) || nilPort(target) || !b.Source.Valid() || !b.Command.Target.Valid() || b.Source == b.Command.Target ||
		b.Source != source.AuthorityRef() || b.Command.Target != target.AuthorityRef() ||
		!b.Command.SourceAddress.Valid() || !b.Command.TargetAddress.Valid() || b.Command.SourceAddress == b.Command.TargetAddress ||
		len(b.Operation) == 0 || len(b.Operation) > 128 {
		return nil, values.PublicInvalid
	}
	return &Connection{source: source, target: target, binding: b, caller: caller, view: view}, nil
}

func nilPort(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return r.IsNil()
	}
	return false
}

func (c *Connection) check(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding, source bool) error {
	if c == nil || ctx == nil {
		return values.PublicInvalid
	}
	if ctx.Err() != nil {
		return values.PublicUnavailable
	}
	if b != c.binding {
		return contract.ErrOperationConflict
	}
	wantAddress, wantOwner, actual := b.Command.TargetAddress, b.Command.Target, c.target.AuthorityRef()
	if source {
		wantAddress, wantOwner, actual = b.Command.SourceAddress, b.Source, c.source.AuthorityRef()
	}
	if address != wantAddress || expected != wantOwner || actual != expected {
		return values.PublicUnavailable
	}
	return nil
}

func (c *Connection) CheckImport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding) error {
	if err := c.check(ctx, address, expected, b, false); err != nil {
		return err
	}
	return c.target.CheckTransitionImport(ctx, c.caller, b, c.view)
}

func (c *Connection) AcceptImport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding) (port.Acceptance, error) {
	if err := c.check(ctx, address, expected, b, false); err != nil {
		return port.Acceptance{}, err
	}
	return c.target.AcceptTransitionImport(ctx, c.source.AuthorityRef(), c.caller, b, c.view, c)
}

func (c *Connection) Install(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding) (port.Completion, error) {
	if err := c.check(ctx, address, expected, b, false); err != nil {
		return port.Completion{}, err
	}
	return c.target.InstallTransition(ctx, c.source.AuthorityRef(), b, c)
}

func (c *Connection) ReadTransitionFence(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding) (port.Frozen, error) {
	if err := c.check(ctx, address, expected, b, true); err != nil {
		return port.Frozen{}, err
	}
	return c.source.ReadTransitionFence(ctx, c.target.AuthorityRef(), b)
}

func (c *Connection) ReadTransitionDecision(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.TransitionApprovalBinding) (port.Decision, error) {
	if err := c.check(ctx, address, expected, b, true); err != nil {
		return port.Decision{}, err
	}
	return c.source.ReadTransitionDecision(ctx, c.target.AuthorityRef(), b)
}
