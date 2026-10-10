package transition

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/transport/grpc/internal/rpcfailure"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
)

type TargetAdmission interface {
	PeerAuthentication
	AcceptFrozenTransition(context.Context, contract.TransitionApprovalBinding, []byte, port.Frozen) (port.Acceptance, error)
}

type Target struct {
	pb.UnimplementedTransitionTargetServiceServer
	owner            port.TargetPeer
	expected         values.FactAuthorityRef
	admission        TargetAdmission
	source           port.SourceTransport
	carrier          port.CarrierTargetPeer
	carrierAdmission CarrierTargetAdmission
	carrierParents   CarrierPlanReader
	carrierSources   CarrierSourceReader
}

func NewTarget(owner port.TargetPeer, expected values.FactAuthorityRef, admission TargetAdmission, source port.SourceTransport, carrier port.CarrierTargetPeer, carrierAdmission CarrierTargetAdmission, carrierParents CarrierPlanReader, carrierSources CarrierSourceReader) *Target {
	return &Target{owner: owner, expected: expected, admission: admission, source: source, carrier: carrier, carrierAdmission: carrierAdmission, carrierParents: carrierParents, carrierSources: carrierSources}
}

func (s *Target) binding(ctx context.Context, w *pb.TransitionApprovalBinding) (contract.TransitionApprovalBinding, error) {
	if s.owner == nil || s.admission == nil || s.source == nil {
		return contract.TransitionApprovalBinding{}, status.Error(codes.Unimplemented, "transition target disabled")
	}
	if ctx == nil || s.owner.AuthorityRef() != s.expected {
		return contract.TransitionApprovalBinding{}, status.Error(codes.Unavailable, "transition target unavailable")
	}
	if w == nil || proto.Size(w) > 4096 {
		return contract.TransitionApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid target binding")
	}
	b, err := wire.TransitionBinding(w, s.expected)
	if err != nil {
		return b, err
	}
	return b, s.admission.CheckTransitionPeer(ctx, b.Source)
}

func (s *Target) AcceptTransition(ctx context.Context, in *pb.AcceptTransitionRequest) (*pb.AcceptTransitionResponse, error) {
	b, err := s.binding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	if len(in.GetApprovalToken()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid target approval token")
	}
	// Fetch first, without an owner or admission lock. Revocation can proceed
	// while this bounded transfer waits; admission rechecks at the real commit.
	frozen, err := s.source.ReadTransitionFence(ctx, b.Command.SourceAddress, b.Source, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.AcceptTransitionResponse{Outcome: &pb.AcceptTransitionResponse_Failure{Failure: f}}, e
	}
	v, err := s.admission.AcceptFrozenTransition(ctx, b, in.ApprovalToken, frozen)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		f, e := rpcfailure.Transition(err, true)
		return &pb.AcceptTransitionResponse{Outcome: &pb.AcceptTransitionResponse_Failure{Failure: f}}, e
	}
	a := exchange.OpenAcceptance(v)
	if !transitionwire.MatchesFence(b, a.Fence) {
		return nil, status.Error(codes.DataLoss, "target acceptance binding mismatch")
	}
	w := transitionwire.AcceptanceWire(in.Binding, a)
	if _, err := transitionwire.Acceptance(b, w); err != nil {
		return nil, status.Error(codes.DataLoss, "invalid target acceptance")
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Source); err != nil {
		return nil, err
	}
	return &pb.AcceptTransitionResponse{Outcome: &pb.AcceptTransitionResponse_Result{Result: w}}, nil
}

func (s *Target) InstallTransition(ctx context.Context, in *pb.InstallTransitionRequest) (*pb.InstallTransitionResponse, error) {
	b, err := s.binding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	v, err := s.owner.InstallTransition(ctx, b.Source, b, s.source)
	if err != nil {
		f, e := rpcfailure.Transition(err, true)
		return &pb.InstallTransitionResponse{Outcome: &pb.InstallTransitionResponse_Failure{Failure: f}}, e
	}
	c := exchange.OpenCompletion(v)
	if !c.Valid() || !transitionwire.MatchesFence(b, c.Installed.Source.Decision.Acceptance.Fence) {
		return nil, status.Error(codes.DataLoss, "target completion binding mismatch")
	}
	w := transitionwire.CompletionWire(in.Binding, c)
	if _, err := transitionwire.Completion(b, w); err != nil {
		return nil, status.Error(codes.DataLoss, "invalid target completion")
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Source); err != nil {
		return nil, err
	}
	return &pb.InstallTransitionResponse{Outcome: &pb.InstallTransitionResponse_Result{Result: w}}, nil
}

// carrierBinding validates one carrier binding for this target host. The
// caller peer is authenticated per method: the parent for token checks and
// acceptance, the source for the stateless preflight and completion reads.
func (s *Target) carrierBinding(ctx context.Context, w *pb.CarrierApprovalBinding) (contract.CarrierApprovalBinding, error) {
	if ctx == nil || s.carrier.AuthorityRef() != s.expected {
		return contract.CarrierApprovalBinding{}, status.Error(codes.Unavailable, "carrier target unavailable")
	}
	b, err := wire.CarrierBinding(w)
	if err != nil || b.Command.Target != s.expected {
		return contract.CarrierApprovalBinding{}, status.Error(codes.InvalidArgument, "invalid carrier target binding")
	}
	return b, nil
}

// CheckCarrierImport serves two authenticated peers. With a token it is the
// parent's strict check of its retained import approval; without one it is the
// source carrier's stateless preflight proving one live approval exists.
func (s *Target) CheckCarrierImport(ctx context.Context, in *pb.CheckCarrierImportRequest) (*pb.CheckCarrierImportResponse, error) {
	if s.carrier == nil || s.carrierAdmission == nil || s.admission == nil {
		return nil, status.Error(codes.Unimplemented, "carrier target disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	if len(in.GetToken()) != 0 {
		caller, err := s.carrierAdmission.CheckedCarrierImport(ctx, b, in.Token)
		if err != nil {
			if _, ok := status.FromError(err); ok {
				return nil, err
			}
			f, e := rpcfailure.Transition(err, false)
			return &pb.CheckCarrierImportResponse{Outcome: &pb.CheckCarrierImportResponse_Failure{Failure: f}}, e
		}
		return &pb.CheckCarrierImportResponse{Outcome: &pb.CheckCarrierImportResponse_Result{Result: &pb.CarrierApprovalCheck{
			Issuer: proto.String(string(s.expected)), Binding: proto.Clone(in.Binding).(*pb.CarrierApprovalBinding),
			Realm: proto.String(string(caller.Subject.Realm)), Principal: proto.String(string(caller.Subject.Principal)),
		}}}, nil
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Command.Source); err != nil {
		return nil, err
	}
	if err := s.carrierAdmission.CheckCarrierImportPreflight(ctx, b); err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		f, e := rpcfailure.Transition(err, false)
		return &pb.CheckCarrierImportResponse{Outcome: &pb.CheckCarrierImportResponse_Failure{Failure: f}}, e
	}
	return &pb.CheckCarrierImportResponse{Outcome: &pb.CheckCarrierImportResponse_Preflight{Preflight: &pb.CarrierImportPreflight{
		Issuer: proto.String(string(s.expected)), Binding: proto.Clone(in.Binding).(*pb.CarrierApprovalBinding),
	}}}, nil
}

func (s *Target) AcceptCarrierTarget(ctx context.Context, in *pb.AcceptCarrierTargetRequest) (*pb.AcceptCarrierTargetResponse, error) {
	if s.carrier == nil || s.carrierAdmission == nil || s.carrierParents == nil || s.carrierSources == nil {
		return nil, status.Error(codes.Unimplemented, "carrier target disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	if len(in.GetApprovalToken()) != 32 {
		return nil, status.Error(codes.InvalidArgument, "invalid carrier import approval")
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return nil, err
	}
	// Fetch both evidence values first, without an owner or admission lock.
	plan, err := s.carrierParents.ReadCarrierPlan(ctx, b.Command.ParentAddress, b.Parent, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.AcceptCarrierTargetResponse{Outcome: &pb.AcceptCarrierTargetResponse_Failure{Failure: f}}, e
	}
	frozen, err := s.carrierSources.ReadCarrierSource(ctx, b.Command.SourceAddress, b.Command.Source, b)
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.AcceptCarrierTargetResponse{Outcome: &pb.AcceptCarrierTargetResponse_Failure{Failure: f}}, e
	}
	v, err := s.carrierAdmission.AcceptFrozenCarrier(ctx, b, in.ApprovalToken, plan, frozen)
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return nil, err
		}
		f, e := rpcfailure.Transition(err, true)
		return &pb.AcceptCarrierTargetResponse{Outcome: &pb.AcceptCarrierTargetResponse_Failure{Failure: f}}, e
	}
	a := exchange.OpenCarrierAcceptance(v)
	w, werr := transitionwire.CarrierAcceptanceWire(in.Binding, a)
	if werr != nil {
		return nil, status.Error(codes.DataLoss, "invalid carrier acceptance")
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return nil, err
	}
	return &pb.AcceptCarrierTargetResponse{Outcome: &pb.AcceptCarrierTargetResponse_Result{Result: w}}, nil
}

func (s *Target) InstallCarrierTarget(ctx context.Context, in *pb.InstallCarrierTargetRequest) (*pb.InstallCarrierTargetResponse, error) {
	if s.carrier == nil || s.carrierParents == nil || s.carrierSources == nil {
		return nil, status.Error(codes.Unimplemented, "carrier target disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return nil, err
	}
	v, err := s.carrier.InstallCarrierTarget(ctx, b.Parent, b, carrierTargetTransport{binding: b, parent: s.carrierParents, sources: s.carrierSources})
	if err != nil {
		f, e := rpcfailure.Transition(err, true)
		return &pb.InstallCarrierTargetResponse{Outcome: &pb.InstallCarrierTargetResponse_Failure{Failure: f}}, e
	}
	c := exchange.OpenCompletion(v)
	w, werr := transitionwire.CarrierCompletionWire(in.Binding, c)
	if !c.Valid() || werr != nil {
		return nil, status.Error(codes.DataLoss, "invalid carrier completion")
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Parent); err != nil {
		return nil, err
	}
	return &pb.InstallCarrierTargetResponse{Outcome: &pb.InstallCarrierTargetResponse_Result{Result: w}}, nil
}

func (s *Target) ReadCarrierCompletion(ctx context.Context, in *pb.ReadCarrierCompletionRequest) (*pb.ReadCarrierCompletionResponse, error) {
	if s.carrier == nil || s.carrierParents == nil || s.carrierSources == nil {
		return nil, status.Error(codes.Unimplemented, "carrier target disabled")
	}
	b, err := s.carrierBinding(ctx, in.GetBinding())
	if err != nil {
		return nil, err
	}
	// The source reads its own completion evidence; the parent does not.
	if err := s.admission.CheckTransitionPeer(ctx, b.Command.Source); err != nil {
		return nil, err
	}
	v, err := s.carrier.ReadCarrierCompletion(ctx, b.Command.Source, b, carrierTargetTransport{binding: b, parent: s.carrierParents, sources: s.carrierSources})
	if err != nil {
		f, e := rpcfailure.Transition(err, false)
		return &pb.ReadCarrierCompletionResponse{Outcome: &pb.ReadCarrierCompletionResponse_Failure{Failure: f}}, e
	}
	c := exchange.OpenCompletion(v)
	w, werr := transitionwire.CarrierCompletionWire(in.Binding, c)
	if !c.Valid() || werr != nil {
		return nil, status.Error(codes.DataLoss, "invalid carrier completion")
	}
	if err := s.admission.CheckTransitionPeer(ctx, b.Command.Source); err != nil {
		return nil, err
	}
	return &pb.ReadCarrierCompletionResponse{Outcome: &pb.ReadCarrierCompletionResponse_Result{Result: w}}, nil
}
