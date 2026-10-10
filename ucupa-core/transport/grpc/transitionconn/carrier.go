package transitionconn

import (
	"context"
	"crypto/sha256"

	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
	coresdk "ucupa/ucupa-core/sdk/go"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
)

// carrierConnection is the parent's one bounded orchestration transport for a
// single recovery invocation. It binds the two retained peer approval tokens
// to their designated hosts and never refreshes or re-issues either approval.
type carrierConnection struct {
	peers       *Connections
	binding     contract.CarrierApprovalBinding
	wire        *pb.CarrierApprovalBinding
	sourceToken [32]byte
	targetToken [32]byte
}

// Carrier selects the installed source and target clients for one parent-side
// recovery invocation. The parent loads its durable plan itself; this factory
// only binds addresses and retained tokens, and rejects any other input before
// network I/O.
func (p *Connections) Carrier(b contract.CarrierApprovalBinding, sourceToken, targetToken []byte) (transition.CarrierTransport, error) {
	if p == nil || p.owner != b.Parent {
		return nil, contract.ErrOwnerMismatch
	}
	c := b.Command
	if !b.Parent.Valid() || !c.Source.Valid() || !c.Target.Valid() ||
		b.Parent == c.Source || b.Parent == c.Target || c.Source == c.Target ||
		!c.ParentAddress.Valid() || !c.SourceAddress.Valid() || !c.TargetAddress.Valid() ||
		c.ParentAddress == c.SourceAddress || c.ParentAddress == c.TargetAddress || c.SourceAddress == c.TargetAddress ||
		len(b.Operation) == 0 || len(b.Operation) > 128 {
		return nil, contract.ErrInvalid
	}
	w, err := transitionwire.CarrierBindingWire(b)
	if err != nil || len(sourceToken) != 32 || len(targetToken) != 32 {
		return nil, contract.ErrInvalid
	}
	source := p.peers[c.SourceAddress]
	target := p.peers[c.TargetAddress]
	if source == nil || target == nil {
		return nil, contract.ErrUnavailable
	}
	sourceAddress, sourceOwner := source.Target()
	targetAddress, targetOwner := target.Target()
	if sourceAddress != c.SourceAddress || sourceOwner != c.Source || targetAddress != c.TargetAddress || targetOwner != c.Target {
		return nil, contract.ErrOwnerMismatch
	}
	conn := &carrierConnection{peers: p, binding: b, wire: w}
	copy(conn.sourceToken[:], sourceToken)
	copy(conn.targetToken[:], targetToken)
	return conn, nil
}

func (p *carrierConnection) check(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding, source bool) (*pb.CarrierApprovalBinding, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return nil, contract.ErrUnavailable
	}
	if b != p.binding {
		return nil, contract.ErrOwnerMismatch
	}
	want, next := b.Command.Source, b.Command.SourceAddress
	if !source {
		want, next = b.Command.Target, b.Command.TargetAddress
	}
	if address != next || expected != want {
		return nil, contract.ErrOwnerMismatch
	}
	return p.wire, nil
}

func (p *carrierConnection) acquire() (func(), error) {
	select {
	case p.peers.active <- struct{}{}:
		return func() { <-p.peers.active }, nil
	default:
		return nil, values.PublicResourceExhausted
	}
}

// ReadCarrierPlan, ReadCarrierDecision, ReadCarrierSource and
// ReadCarrierCompletion are out of role for the parent orchestrator: the
// parent's durable evidence is loaded through its own owner, and the peer
// reads run through the source and target hosts' own bounded transports.
func (p *carrierConnection) ReadCarrierPlan(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.CarrierPlan, error) {
	return transition.CarrierPlan{}, values.PublicInvalid
}
func (p *carrierConnection) ReadCarrierDecision(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.CarrierDecision, error) {
	return transition.CarrierDecision{}, values.PublicInvalid
}
func (p *carrierConnection) ReadCarrierSource(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.CarrierFrozen, error) {
	return transition.CarrierFrozen{}, values.PublicInvalid
}
func (p *carrierConnection) ReadCarrierCompletion(context.Context, values.NextHop, values.FactAuthorityRef, contract.CarrierApprovalBinding) (transition.Completion, error) {
	return transition.Completion{}, values.PublicInvalid
}

// CheckCarrierExport re-validates the parent's retained original export
// approval at the source host under the parent's authenticated service
// identity. A check is admission evidence, never a write permit by itself.
func (p *carrierConnection) CheckCarrierExport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	w, err := p.check(ctx, address, expected, b, true)
	if err != nil {
		return err
	}
	client := p.peers.peers[b.Command.SourceAddress]
	if client == nil {
		return contract.ErrUnavailable
	}
	release, err := p.acquire()
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, p.peers.deadline)
	defer cancel()
	r, err := client.CarrierApprovals().CheckCarrierApproval(ctx, &pb.CheckCarrierApprovalRequest{Binding: w, Token: p.sourceToken[:]})
	if err != nil || r.GetResult() == nil {
		return remoteError(err, r.GetFailure())
	}
	return nil
}

func (p *carrierConnection) CheckCarrierImport(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	w, err := p.check(ctx, address, expected, b, false)
	if err != nil {
		return err
	}
	client := p.peers.peers[b.Command.TargetAddress]
	if client == nil {
		return contract.ErrUnavailable
	}
	release, err := p.acquire()
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, p.peers.deadline)
	defer cancel()
	r, err := client.CarrierApprovals().CheckCarrierApproval(ctx, &pb.CheckCarrierApprovalRequest{Binding: w, Token: p.targetToken[:]})
	if err != nil || r.GetResult() == nil {
		return remoteError(err, r.GetFailure())
	}
	return nil
}

// FenceCarrierSource dispatches the source withdrawal with the retained export
// approval. The reply carries the durable fence only; the manifest itself is
// pulled by the target host through its own bounded transport.
func (p *carrierConnection) FenceCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierFrozen, error) {
	w, err := p.check(ctx, address, expected, b, true)
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	client := p.peers.peers[b.Command.SourceAddress]
	if client == nil {
		return transition.CarrierFrozen{}, contract.ErrUnavailable
	}
	release, err := p.acquire()
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, p.peers.deadline)
	defer cancel()
	r, err := client.TransitionSource().FenceCarrierSource(ctx, &pb.FenceCarrierSourceRequest{Binding: w, ApprovalToken: p.sourceToken[:]})
	if err != nil || r.GetResult() == nil {
		return transition.CarrierFrozen{}, remoteError(err, r.GetFailure())
	}
	f, err := transitionwire.CarrierFence(b, r.GetResult())
	if err != nil || ctx.Err() != nil {
		return transition.CarrierFrozen{}, contract.ErrOwnerMismatch
	}
	return exchange.FreezeCarrier(b, f, ""), nil
}

func (p *carrierConnection) AcceptCarrierTarget(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierAcceptance, error) {
	w, err := p.check(ctx, address, expected, b, false)
	if err != nil {
		return transition.CarrierAcceptance{}, err
	}
	client := p.peers.peers[b.Command.TargetAddress]
	if client == nil {
		return transition.CarrierAcceptance{}, contract.ErrUnavailable
	}
	release, err := p.acquire()
	if err != nil {
		return transition.CarrierAcceptance{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, p.peers.deadline)
	defer cancel()
	r, err := client.TransitionTarget().AcceptCarrierTarget(ctx, &pb.AcceptCarrierTargetRequest{Binding: w, ApprovalToken: p.targetToken[:]})
	if err != nil || r.GetResult() == nil {
		return transition.CarrierAcceptance{}, remoteError(err, r.GetFailure())
	}
	a, err := transitionwire.CarrierAcceptance(b, r.GetResult())
	if err != nil || ctx.Err() != nil {
		return transition.CarrierAcceptance{}, contract.ErrOwnerMismatch
	}
	return exchange.AcceptCarrier(a), nil
}

func (p *carrierConnection) InstallCarrierTarget(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.Completion, error) {
	w, err := p.check(ctx, address, expected, b, false)
	if err != nil {
		return transition.Completion{}, err
	}
	client := p.peers.peers[b.Command.TargetAddress]
	if client == nil {
		return transition.Completion{}, contract.ErrUnavailable
	}
	release, err := p.acquire()
	if err != nil {
		return transition.Completion{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, p.peers.deadline)
	defer cancel()
	r, err := client.TransitionTarget().InstallCarrierTarget(ctx, &pb.InstallCarrierTargetRequest{Binding: w})
	if err != nil || r.GetResult() == nil {
		return transition.Completion{}, remoteError(err, r.GetFailure())
	}
	c, err := transitionwire.CarrierCompletion(b, r.GetResult())
	if err != nil || ctx.Err() != nil {
		return transition.Completion{}, contract.ErrOwnerMismatch
	}
	return exchange.Complete(c), nil
}

// SettleCarrierSource carries no failure oneof: any transport error or missing
// positive result is reported as unavailable, never as a business outcome.
func (p *carrierConnection) SettleCarrierSource(ctx context.Context, address values.NextHop, expected values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	w, err := p.check(ctx, address, expected, b, true)
	if err != nil {
		return err
	}
	client := p.peers.peers[b.Command.SourceAddress]
	if client == nil {
		return contract.ErrUnavailable
	}
	release, err := p.acquire()
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, p.peers.deadline)
	defer cancel()
	r, err := client.TransitionSource().SettleCarrierSource(ctx, &pb.SettleCarrierSourceRequest{Binding: w})
	if err != nil || !r.GetSettled() {
		return remoteError(err, netpb.Failure_FAILURE_UNSPECIFIED)
	}
	return nil
}

// ReadCarrierSource pulls the fenced carrier content from the source host for
// the target carrier. Chunking is stateless: immutable fence bytes survive
// retry and source reopen.
func (p *Connections) ReadCarrierSource(ctx context.Context, address values.NextHop, source values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierFrozen, error) {
	client, w, err := p.carrierSourcePeer(ctx, address, source, b)
	if err != nil {
		return transition.CarrierFrozen{}, err
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	default:
		return transition.CarrierFrozen{}, values.PublicResourceExhausted
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	var fence fact.CarrierFence
	var raw []byte
	var total uint64
	for offset := uint64(0); ; {
		if ctx.Err() != nil {
			return transition.CarrierFrozen{}, contract.ErrUnavailable
		}
		out, err := client.TransitionSource().ReadCarrierSourceChunk(ctx, &pb.ReadCarrierSourceChunkRequest{Binding: w, Offset: proto.Uint64(offset), MaxBytes: proto.Uint32(transitionwire.MaxChunkBytes)})
		if err != nil || out.GetResult() == nil {
			return transition.CarrierFrozen{}, remoteError(err, out.GetFailure())
		}
		r := out.GetResult()
		f, err := transitionwire.CarrierFence(b, r.Fence)
		if err != nil || !proto.Equal(r.Binding, w) || r.GetTotalBytes() == 0 || r.GetTotalBytes() > transitionwire.MaxManifestBytes || r.GetOffset() != offset || offset >= r.GetTotalBytes() ||
			uint64(len(r.Data)) != min(uint64(transitionwire.MaxChunkBytes), r.GetTotalBytes()-offset) {
			return transition.CarrierFrozen{}, contract.ErrOwnerMismatch
		}
		if offset == 0 {
			fence, total = f, r.GetTotalBytes()
			raw = make([]byte, 0, int(total))
		} else if f != fence || total != r.GetTotalBytes() {
			return transition.CarrierFrozen{}, contract.ErrOwnerMismatch
		}
		raw = append(raw, r.Data...)
		offset += uint64(len(r.Data))
		if offset == total {
			break
		}
	}
	if ctx.Err() != nil {
		return transition.CarrierFrozen{}, contract.ErrUnavailable
	}
	if sha256.Sum256(raw) != fence.ManifestDigest {
		return transition.CarrierFrozen{}, contract.ErrOwnerMismatch
	}
	return exchange.FreezeCarrier(b, fence, string(raw)), nil
}

// CheckCarrierImport is the source carrier's tokenless preflight against the
// target host: it proves one live original import approval exists for this
// exact binding. It is not the target's durable content acceptance.
func (p *Connections) CheckCarrierImport(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.CarrierApprovalBinding) error {
	client, w, err := p.carrierTargetPeer(ctx, address, target, b)
	if err != nil {
		return err
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	default:
		return values.PublicResourceExhausted
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	r, err := client.TransitionTarget().CheckCarrierImport(ctx, &pb.CheckCarrierImportRequest{Binding: w})
	if err != nil || r.GetPreflight() == nil || !proto.Equal(r.GetPreflight().GetBinding(), w) {
		return remoteError(err, r.GetFailure())
	}
	return nil
}

// ReadCarrierCompletion reads the target's durable completion evidence for the
// source carrier's settle. The parent does not use this entry.
func (p *Connections) ReadCarrierCompletion(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.Completion, error) {
	client, w, err := p.carrierTargetPeer(ctx, address, target, b)
	if err != nil {
		return transition.Completion{}, err
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	default:
		return transition.Completion{}, values.PublicResourceExhausted
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	r, err := client.TransitionTarget().ReadCarrierCompletion(ctx, &pb.ReadCarrierCompletionRequest{Binding: w})
	if err != nil || r.GetResult() == nil {
		return transition.Completion{}, remoteError(err, r.GetFailure())
	}
	c, err := transitionwire.CarrierCompletion(b, r.GetResult())
	if err != nil || ctx.Err() != nil {
		return transition.Completion{}, contract.ErrOwnerMismatch
	}
	return exchange.Complete(c), nil
}

// carrierSourcePeer authenticates this target host's fixed client for the
// designated source of one carrier binding.
func (p *Connections) carrierSourcePeer(ctx context.Context, address values.NextHop, source values.FactAuthorityRef, b contract.CarrierApprovalBinding) (*coresdk.Client, *pb.CarrierApprovalBinding, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, contract.ErrUnavailable
	}
	if p.owner != b.Command.Target || address != b.Command.SourceAddress || source != b.Command.Source {
		return nil, nil, contract.ErrOwnerMismatch
	}
	w, err := transitionwire.CarrierBindingWire(b)
	if err != nil {
		return nil, nil, err
	}
	client := p.peers[address]
	if client == nil {
		return nil, nil, contract.ErrUnavailable
	}
	actualAddress, actualSource := client.Target()
	if actualAddress != address || actualSource != source {
		return nil, nil, contract.ErrOwnerMismatch
	}
	return client, w, nil
}

// carrierTargetPeer authenticates this source host's fixed client for the
// designated target of one carrier binding.
func (p *Connections) carrierTargetPeer(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, b contract.CarrierApprovalBinding) (*coresdk.Client, *pb.CarrierApprovalBinding, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, contract.ErrUnavailable
	}
	if p.owner != b.Command.Source || address != b.Command.TargetAddress || target != b.Command.Target {
		return nil, nil, contract.ErrOwnerMismatch
	}
	w, err := transitionwire.CarrierBindingWire(b)
	if err != nil {
		return nil, nil, err
	}
	client := p.peers[address]
	if client == nil {
		return nil, nil, contract.ErrUnavailable
	}
	actualAddress, actualTarget := client.Target()
	if actualAddress != address || actualTarget != target {
		return nil, nil, contract.ErrOwnerMismatch
	}
	return client, w, nil
}
