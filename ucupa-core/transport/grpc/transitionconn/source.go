// Package transitionconn connects transfer ports to explicitly installed,
// authenticated clients. It cannot choose a successor or resolve an owner Ref.
package transitionconn

import (
	"context"
	"crypto/sha256"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

type Connections struct {
	owner    values.FactAuthorityRef
	peers    map[values.NextHop]*coresdk.Client
	active   chan struct{}
	deadline time.Duration
}

func (p *Connections) ConfiguredFor(owner values.FactAuthorityRef) bool {
	return p != nil && p.owner == owner
}

// The receiving host owns these clients and closes them after draining work.
// Both aggregate pull concurrency and elapsed lifetime are explicit bounds.
func NewConnections(owner values.FactAuthorityRef, clients []*coresdk.Client, maxConcurrent int, deadline time.Duration) (*Connections, error) {
	if !owner.Valid() || len(clients) > 1024 || maxConcurrent <= 0 || maxConcurrent > 1024 || deadline <= 0 {
		return nil, contract.ErrInvalid
	}
	p := &Connections{owner: owner, peers: make(map[values.NextHop]*coresdk.Client), active: make(chan struct{}, maxConcurrent), deadline: deadline}
	for _, client := range clients {
		if client == nil {
			return nil, contract.ErrInvalid
		}
		address, source := client.Target()
		if source == owner || p.peers[address] != nil {
			return nil, contract.ErrInvalid
		}
		p.peers[address] = client
	}
	return p, nil
}

func (p *Connections) peer(ctx context.Context, address values.NextHop, source values.FactAuthorityRef, b contract.TransitionApprovalBinding) (pb.TransitionSourceServiceClient, *pb.TransitionApprovalBinding, error) {
	if p == nil || ctx == nil || ctx.Err() != nil {
		return nil, nil, contract.ErrUnavailable
	}
	if source != b.Source || p.owner != b.Command.Target || address != b.Command.SourceAddress {
		return nil, nil, contract.ErrOwnerMismatch
	}
	w, err := transitionwire.BindingWire(b)
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
	return client.TransitionSource(), w, nil
}

func (p *Connections) ReadTransitionFence(ctx context.Context, address values.NextHop, source values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Frozen, error) {
	client, w, err := p.peer(ctx, address, source, b)
	if err != nil {
		return transition.Frozen{}, err
	}
	select {
	case p.active <- struct{}{}:
		defer func() { <-p.active }()
	default:
		return transition.Frozen{}, values.PublicResourceExhausted
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	return pullFrozen(ctx, client, b, w)
}

func pullFrozen(ctx context.Context, client pb.TransitionSourceServiceClient, b contract.TransitionApprovalBinding, w *pb.TransitionApprovalBinding) (transition.Frozen, error) {
	var raw []byte
	var fence fact.SourceFence
	var total uint64
	for offset := uint64(0); ; {
		if ctx.Err() != nil {
			return transition.Frozen{}, contract.ErrUnavailable
		}
		out, err := client.ReadTransitionChunk(ctx, &pb.ReadTransitionChunkRequest{Binding: w, Offset: proto.Uint64(offset), MaxBytes: proto.Uint32(transitionwire.MaxChunkBytes)})
		if err != nil || out.GetResult() == nil {
			return transition.Frozen{}, remoteError(err, out.GetFailure())
		}
		r := out.GetResult()
		f, err := transitionwire.Fence(b, r.Fence)
		if err != nil || !proto.Equal(r.Binding, w) || !transitionwire.MatchesFence(b, f) || r.GetTotalBytes() == 0 || r.GetTotalBytes() > transitionwire.MaxManifestBytes || r.GetOffset() != offset || offset >= r.GetTotalBytes() ||
			uint64(len(r.Data)) != min(uint64(transitionwire.MaxChunkBytes), r.GetTotalBytes()-offset) {
			return transition.Frozen{}, contract.ErrOwnerMismatch
		}
		if offset == 0 {
			fence, total = f, r.GetTotalBytes()
			raw = make([]byte, 0, int(total))
		} else if f != fence || total != r.GetTotalBytes() {
			return transition.Frozen{}, contract.ErrOwnerMismatch
		}
		raw = append(raw, r.Data...)
		offset += uint64(len(r.Data))
		if offset == total {
			break
		}
	}
	if ctx.Err() != nil {
		return transition.Frozen{}, contract.ErrUnavailable
	}
	if sha256.Sum256(raw) != fence.ManifestDigest {
		return transition.Frozen{}, contract.ErrOwnerMismatch
	}
	return exchange.Freeze(b, fence, string(raw)), nil
}

func (p *Connections) ReadTransitionDecision(ctx context.Context, address values.NextHop, source values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Decision, error) {
	client, w, err := p.peer(ctx, address, source, b)
	if err != nil {
		return transition.Decision{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.deadline)
	defer cancel()
	out, err := client.ReadTransitionDecision(ctx, &pb.ReadTransitionDecisionRequest{Binding: w})
	if err != nil || out.GetResult() == nil {
		return transition.Decision{}, remoteError(err, out.GetFailure())
	}
	d, err := transitionwire.Decision(b, out.GetResult())
	if err != nil || !proto.Equal(out.GetResult().Binding, w) || ctx.Err() != nil {
		return transition.Decision{}, contract.ErrUnavailable
	}
	return exchange.Decide(b, d), nil
}

func remoteError(err error, failure netpb.Failure) error {
	if err != nil {
		if status.Code(err) == codes.DataLoss {
			return contract.ErrOwnerMismatch
		}
		return contract.ErrUnavailable
	}
	switch failure {
	case netpb.Failure_INVALID:
		return contract.ErrInvalid
	case netpb.Failure_DENIED:
		return values.PublicDenied
	case netpb.Failure_INVALID_CONTEXT:
		return values.PublicInvalidContext
	case netpb.Failure_RESOURCE_EXHAUSTED:
		return values.PublicResourceExhausted
	case netpb.Failure_NOT_FOUND:
		return contract.ErrNotFound
	case netpb.Failure_CONFLICT:
		return contract.ErrConflict
	default:
		return contract.ErrUnavailable
	}
}
