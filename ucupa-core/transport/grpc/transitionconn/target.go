package transitionconn

import (
	"context"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	coresdk "ucupa/ucupa-core/sdk/go"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
)

// TargetConnection binds one operation's existing approval and designated SDK
// client. It issues no approval, stores no workflow state and resolves no Ref.
type TargetConnection struct {
	client  *coresdk.Client
	binding contract.TransitionApprovalBinding
	wire    *pb.TransitionApprovalBinding
	token   [32]byte
}

// Target selects an installed address for a single recovery invocation. The
// owner loads its original intent; TargetConnection rejects any different input
// before network I/O. The token is target admission, not proof of acceptance.
func (p *Connections) Target(b contract.TransitionApprovalBinding, token []byte) (transition.TargetTransport, error) {
	if p == nil || p.owner != b.Source {
		return nil, contract.ErrOwnerMismatch
	}
	w, err := transitionwire.BindingWire(b)
	if err != nil || len(token) != 32 {
		return nil, contract.ErrInvalid
	}
	client := p.peers[b.Command.TargetAddress]
	if client == nil {
		return nil, contract.ErrUnavailable
	}
	return NewTargetConnection(client, b, &pb.TransitionApproval{Target: proto.String(string(b.Command.Target)), Binding: w, Token: token})
}

func NewTargetConnection(client *coresdk.Client, b contract.TransitionApprovalBinding, approval *pb.TransitionApproval) (*TargetConnection, error) {
	w, err := transitionwire.BindingWire(b)
	if err != nil || client == nil || approval == nil || len(approval.Token) != 32 || approval.GetTarget() != string(b.Command.Target) || !proto.Equal(approval.Binding, w) {
		return nil, contract.ErrInvalid
	}
	address, target := client.Target()
	if address != b.Command.TargetAddress || target != b.Command.Target {
		return nil, contract.ErrOwnerMismatch
	}
	return &TargetConnection{client: client, binding: b, wire: w, token: [32]byte(approval.Token)}, nil
}

func (p *TargetConnection) check(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.TransitionApprovalBinding) error {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return contract.ErrUnavailable
	}
	if b != p.binding || address != b.Command.TargetAddress || target != b.Command.Target {
		return contract.ErrOwnerMismatch
	}
	return nil
}

func (p *TargetConnection) CheckImport(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.TransitionApprovalBinding) error {
	if err := p.check(ctx, address, target, b); err != nil {
		return err
	}
	r, err := p.client.TransitionApprovals().CheckTransitionApproval(ctx, &pb.CheckTransitionApprovalRequest{Binding: p.wire, Token: p.token[:]})
	if err != nil || r.GetResult() == nil {
		return remoteError(err, r.GetFailure())
	}
	return nil
}

func (p *TargetConnection) AcceptImport(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Acceptance, error) {
	if err := p.check(ctx, address, target, b); err != nil {
		return transition.Acceptance{}, err
	}
	r, err := p.client.TransitionTarget().AcceptTransition(ctx, &pb.AcceptTransitionRequest{Binding: p.wire, ApprovalToken: p.token[:]})
	if err != nil || r.GetResult() == nil {
		return transition.Acceptance{}, remoteError(err, r.GetFailure())
	}
	a, err := transitionwire.Acceptance(b, r.GetResult())
	if err != nil {
		return transition.Acceptance{}, contract.ErrOwnerMismatch
	}
	return exchange.Accept(a), nil
}

func (p *TargetConnection) Install(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Completion, error) {
	if err := p.check(ctx, address, target, b); err != nil {
		return transition.Completion{}, err
	}
	r, err := p.client.TransitionTarget().InstallTransition(ctx, &pb.InstallTransitionRequest{Binding: p.wire})
	if err != nil || r.GetResult() == nil {
		return transition.Completion{}, remoteError(err, r.GetFailure())
	}
	c, err := transitionwire.Completion(b, r.GetResult())
	if err != nil {
		return transition.Completion{}, contract.ErrOwnerMismatch
	}
	return exchange.Complete(c), nil
}
