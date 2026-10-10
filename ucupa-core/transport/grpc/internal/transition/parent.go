package transition

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

type Parent struct {
	pb.UnimplementedTransitionParentServiceServer
	owner    port.CarrierParentPeer
	expected values.FactAuthorityRef
	peers    PeerAuthentication
}

func NewParent(owner port.CarrierParentPeer, expected values.FactAuthorityRef, peers PeerAuthentication) *Parent {
	return &Parent{owner: owner, expected: expected, peers: peers}
}

func (p *Parent) binding(ctx context.Context, w *pb.CarrierApprovalBinding) (contract.CarrierApprovalBinding, values.FactAuthorityRef, error) {
	bad := func(err error) (contract.CarrierApprovalBinding, values.FactAuthorityRef, error) {
		return contract.CarrierApprovalBinding{}, "", err
	}
	if p.owner == nil || p.peers == nil {
		return bad(status.Error(codes.Unimplemented, "transition parent disabled"))
	}
	if ctx == nil || p.owner.AuthorityRef() != p.expected {
		return bad(status.Error(codes.Unavailable, "transition parent unavailable"))
	}
	b, err := wire.CarrierBinding(w)
	if err != nil {
		return bad(err)
	}
	if b.Parent != p.expected {
		return bad(status.Error(codes.InvalidArgument, "wrong carrier parent"))
	}
	// The body does not choose the requesting identity. Match the TLS peer to
	// one of the two exact participants and preserve that choice at disclosure.
	peer := b.Command.Source
	err = p.peers.CheckTransitionPeer(ctx, peer)
	if status.Code(err) == codes.Unauthenticated {
		peer = b.Command.Target
		err = p.peers.CheckTransitionPeer(ctx, peer)
	}
	if err != nil {
		return bad(err)
	}
	return b, peer, nil
}

func (p *Parent) ReadCarrierPlan(ctx context.Context, in *pb.ReadCarrierPlanRequest) (*pb.ReadCarrierPlanResponse, error) {
	b, peer, err := p.binding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	v, err := p.owner.ReadCarrierPlan(ctx, peer, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.ReadCarrierPlanResponse{Outcome: &pb.ReadCarrierPlanResponse_Failure{Failure: f}}, e
	}
	actual, c := exchange.OpenCarrierPlan(v)
	w, err := transitionwire.CarrierPlanWire(b, c)
	if err != nil || actual != b {
		return nil, status.Error(codes.DataLoss, "invalid parent plan")
	}
	if err := p.peers.CheckTransitionPeer(ctx, peer); err != nil {
		return nil, err
	}
	return &pb.ReadCarrierPlanResponse{Outcome: &pb.ReadCarrierPlanResponse_Result{Result: w}}, nil
}

func (p *Parent) ReadCarrierDecision(ctx context.Context, in *pb.ReadCarrierDecisionRequest) (*pb.ReadCarrierDecisionResponse, error) {
	b, peer, err := p.binding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	v, err := p.owner.ReadCarrierDecision(ctx, peer, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.ReadCarrierDecisionResponse{Outcome: &pb.ReadCarrierDecisionResponse_Failure{Failure: f}}, e
	}
	actual, d := exchange.OpenCarrierDecision(v)
	w, err := transitionwire.CarrierDecisionWire(b, d)
	if err != nil || actual != b {
		return nil, status.Error(codes.DataLoss, "invalid parent decision")
	}
	if err := p.peers.CheckTransitionPeer(ctx, peer); err != nil {
		return nil, err
	}
	return &pb.ReadCarrierDecisionResponse{Outcome: &pb.ReadCarrierDecisionResponse_Result{Result: w}}, nil
}
