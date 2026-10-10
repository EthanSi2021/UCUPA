package transition

import (
	"context"
	"crypto/sha256"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

type PeerAuthentication interface {
	CheckTransitionPeer(context.Context, values.FactAuthorityRef) error
}

// Source serves only durable evidence obtained through the real source owner.
// Chunking is stateless: immutable fence bytes survive retry and source reopen.
type Source struct {
	pb.UnimplementedTransitionSourceServiceServer
	owner    port.SourcePeer
	expected values.FactAuthorityRef
	peers    PeerAuthentication
	carrier  port.CarrierSourcePeer
	exports  CarrierSourceAdmission
	parents  CarrierPlanReader
	targets  CarrierTargetReader
}

func NewSource(owner port.SourcePeer, expected values.FactAuthorityRef, peers PeerAuthentication, carrier port.CarrierSourcePeer, exports CarrierSourceAdmission, parents CarrierPlanReader, targets CarrierTargetReader) *Source {
	return &Source{owner: owner, expected: expected, peers: peers, carrier: carrier, exports: exports, parents: parents, targets: targets}
}

func (s *Source) binding(ctx context.Context, w *pb.TransitionApprovalBinding) (contract.TransitionApprovalBinding, error) {
	if s.owner == nil || s.peers == nil {
		return contract.TransitionApprovalBinding{}, status.Error(codes.Unimplemented, "transition source disabled")
	}
	if ctx == nil || s.owner.AuthorityRef() != s.expected {
		return contract.TransitionApprovalBinding{}, status.Error(codes.Unavailable, "transition source unavailable")
	}
	if w == nil || proto.Size(w) > 4096 || w.GetSource() != string(s.expected) {
		return contract.TransitionApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid source binding")
	}
	b, err := wire.TransitionBinding(w, values.FactAuthorityRef(w.GetCommand().GetTarget()))
	if err != nil {
		return b, err
	}
	return b, s.peers.CheckTransitionPeer(ctx, b.Command.Target)
}

func (s *Source) ReadTransitionChunk(ctx context.Context, in *pb.ReadTransitionChunkRequest) (*pb.ReadTransitionChunkResponse, error) {
	b, err := s.binding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	if in.Offset == nil || in.MaxBytes == nil || in.GetMaxBytes() == 0 || in.GetMaxBytes() > transitionwire.MaxChunkBytes {
		return nil, status.Error(codes.InvalidArgument, "invalid chunk bounds")
	}
	v, err := s.owner.ReadTransitionFence(ctx, b.Command.Target, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.ReadTransitionChunkResponse{Outcome: &pb.ReadTransitionChunkResponse_Failure{Failure: f}}, e
	}
	actual, fence, raw := exchange.OpenFrozen(v)
	if actual != b || !transitionwire.MatchesFence(b, fence) || len(raw) == 0 || len(raw) > transitionwire.MaxManifestBytes || sha256.Sum256([]byte(raw)) != fence.ManifestDigest {
		return nil, status.Error(codes.DataLoss, "invalid source evidence")
	}
	if _, err := transitionwire.Fence(b, transitionwire.FenceWire(fence)); err != nil {
		return nil, status.Error(codes.DataLoss, "invalid source fence")
	}
	if in.GetOffset() >= uint64(len(raw)) {
		return nil, status.Error(codes.InvalidArgument, "chunk outside manifest")
	}
	if err := s.peers.CheckTransitionPeer(ctx, b.Command.Target); err != nil {
		return nil, err
	}
	start := int(in.GetOffset()) // bounded above before conversion or addition
	end := min(len(raw), start+int(in.GetMaxBytes()))
	r := &pb.TransitionChunk{Binding: proto.Clone(in.Binding).(*pb.TransitionApprovalBinding), Fence: transitionwire.FenceWire(fence),
		TotalBytes: proto.Uint64(uint64(len(raw))), Offset: proto.Uint64(in.GetOffset()), Data: []byte(raw[start:end])}
	return &pb.ReadTransitionChunkResponse{Outcome: &pb.ReadTransitionChunkResponse_Result{Result: r}}, nil
}

func (s *Source) ReadTransitionDecision(ctx context.Context, in *pb.ReadTransitionDecisionRequest) (*pb.ReadTransitionDecisionResponse, error) {
	b, err := s.binding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	v, err := s.owner.ReadTransitionDecision(ctx, b.Command.Target, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.ReadTransitionDecisionResponse{Outcome: &pb.ReadTransitionDecisionResponse_Failure{Failure: f}}, e
	}
	actual, decision := exchange.OpenDecision(v)
	w := transitionwire.DecisionWire(in.Binding, decision)
	if actual != b || !transitionwire.MatchesFence(b, decision.Acceptance.Fence) {
		return nil, status.Error(codes.DataLoss, "invalid source decision")
	}
	if _, err := transitionwire.Decision(b, w); err != nil {
		return nil, status.Error(codes.DataLoss, "invalid source decision")
	}
	if err := s.peers.CheckTransitionPeer(ctx, b.Command.Target); err != nil {
		return nil, err
	}
	return &pb.ReadTransitionDecisionResponse{Outcome: &pb.ReadTransitionDecisionResponse_Result{Result: w}}, nil
}

// carrierBinding validates one carrier binding for this source host and
// authenticates exactly one calling peer. The body never chooses the identity.
func (s *Source) carrierBinding(ctx context.Context, w *pb.CarrierApprovalBinding, peer values.FactAuthorityRef) (contract.CarrierApprovalBinding, error) {
	if ctx == nil || s.carrier.AuthorityRef() != s.expected {
		return contract.CarrierApprovalBinding{}, status.Error(codes.Unavailable, "carrier source unavailable")
	}
	b, err := wire.CarrierBinding(w)
	if err != nil || b.Command.Source != s.expected {
		return contract.CarrierApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid carrier source binding")
	}
	return b, s.peers.CheckTransitionPeer(ctx, peer)
}

func (s *Source) FenceCarrierSource(ctx context.Context, in *pb.FenceCarrierSourceRequest) (*pb.FenceCarrierSourceResponse, error) {
	if s.carrier == nil || s.exports == nil || s.parents == nil || s.targets == nil || s.peers == nil {
		return nil, status.Error(codes.Unimplemented, "carrier source disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding(), values.FactAuthorityRef(in.GetBinding().GetParent()))
	if err != nil {
		return nil, err
	}
	if len(in.GetApprovalToken()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier export approval")
	}
	// An expired or absent approval resolves to zero caller/view: the owner may
	// still replay an existing durable fence, but cannot fence for the first time.
	caller, view, _, err := s.exports.ResolveCarrierExport(ctx, b, in.ApprovalToken)
	if err != nil {
		return nil, err
	}
	frozen, err := s.carrier.FenceCarrierSource(ctx, b.Parent, caller, b, view, carrierSourceTransport{binding: b, parent: s.parents, targets: s.targets})
	if err != nil {
		f, e := rpcfailure.Transition(err, true)
		return &pb.FenceCarrierSourceResponse{Outcome: &pb.FenceCarrierSourceResponse_Failure{Failure: f}}, e
	}
	actual, fence, _ := exchange.OpenCarrierFrozen(frozen)
	w, werr := transitionwire.CarrierFenceWire(b, fence)
	if actual != b || werr != nil {
		return nil, status.Error(codes.DataLoss, "invalid carrier fence")
	}
	if err := s.peers.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return nil, err
	}
	return &pb.FenceCarrierSourceResponse{Outcome: &pb.FenceCarrierSourceResponse_Result{Result: w}}, nil
}

func (s *Source) ReadCarrierSourceChunk(ctx context.Context, in *pb.ReadCarrierSourceChunkRequest) (*pb.ReadCarrierSourceChunkResponse, error) {
	if s.carrier == nil || s.parents == nil || s.peers == nil {
		return nil, status.Error(codes.Unimplemented, "carrier source disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding(), values.FactAuthorityRef(in.GetBinding().GetCommand().GetTarget()))
	if err != nil {
		return nil, err
	}
	if in.Offset == nil || in.MaxBytes == nil || in.GetMaxBytes() == 0 || in.GetMaxBytes() > transitionwire.MaxChunkBytes {
		return nil, status.Error(codes.InvalidArgument, "invalid chunk bounds")
	}
	frozen, err := s.carrier.ReadCarrierSource(ctx, b.Command.Target, b, carrierSourceTransport{binding: b, parent: s.parents, targets: s.targets})
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.ReadCarrierSourceChunkResponse{Outcome: &pb.ReadCarrierSourceChunkResponse_Failure{Failure: f}}, e
	}
	actual, fence, raw := exchange.OpenCarrierFrozen(frozen)
	w, werr := transitionwire.CarrierFenceWire(b, fence)
	if actual != b || werr != nil || len(raw) == 0 || len(raw) > transitionwire.MaxManifestBytes || sha256.Sum256([]byte(raw)) != fence.ManifestDigest {
		return nil, status.Error(codes.DataLoss, "invalid carrier source evidence")
	}
	if in.GetOffset() >= uint64(len(raw)) {
		return nil, status.Error(codes.InvalidArgument, "chunk outside manifest")
	}
	if err := s.peers.CheckTransitionPeer(ctx, b.Command.Target); err != nil {
		return nil, err
	}
	start := int(in.GetOffset()) // bounded above before conversion or addition
	end := min(len(raw), start+int(in.GetMaxBytes()))
	r := &pb.CarrierSourceChunk{Binding: proto.Clone(in.Binding).(*pb.CarrierApprovalBinding), Fence: w,
		TotalBytes: proto.Uint64(uint64(len(raw))), Offset: proto.Uint64(in.GetOffset()), Data: []byte(raw[start:end])}
	return &pb.ReadCarrierSourceChunkResponse{Outcome: &pb.ReadCarrierSourceChunkResponse_Result{Result: r}}, nil
}

func (s *Source) SettleCarrierSource(ctx context.Context, in *pb.SettleCarrierSourceRequest) (*pb.SettleCarrierSourceResponse, error) {
	if s.carrier == nil || s.targets == nil || s.peers == nil {
		return nil, status.Error(codes.Unimplemented, "carrier source disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding(), values.FactAuthorityRef(in.GetBinding().GetParent()))
	if err != nil {
		return nil, err
	}
	err = s.carrier.SettleCarrierSource(ctx, b.Parent, b, carrierSourceTransport{binding: b, parent: s.parents, targets: s.targets})
	if err != nil {
		return nil, carrierSettleStatus(err)
	}
	if err := s.peers.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return nil, err
	}
	return &pb.SettleCarrierSourceResponse{Settled: proto.Bool(true)}, nil
}

// carrierSettleStatus maps the one settle failure vocabulary onto transport
// codes; SettleCarrierSourceResponse carries no failure oneof.
func carrierSettleStatus(err error) error {
	f, e := rpcfailure.Transition(err, true)
	if e != nil {
		return e
	}
	switch f {
	case netpb.Failure_INVALID:
		return status.Error(codes.InvalidArgument, "carrier settle invalid")
	case netpb.Failure_DENIED:
		return status.Error(codes.PermissionDenied, "carrier settle denied")
	case netpb.Failure_NOT_FOUND:
		return status.Error(codes.NotFound, "carrier settle not found")
	case netpb.Failure_CONFLICT:
		return status.Error(codes.FailedPrecondition, "carrier settle conflict")
	case netpb.Failure_RESOURCE_EXHAUSTED:
		return status.Error(codes.ResourceExhausted, "carrier settle exhausted")
	default:
		return status.Error(codes.Unavailable, "carrier settle unavailable")
	}
}
