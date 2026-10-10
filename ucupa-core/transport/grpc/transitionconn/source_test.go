package transitionconn

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	"ucupa/ucupa-core/internal/transfer/exchange"
	"ucupa/ucupa-core/internal/transfer/fact"
	"ucupa/ucupa-core/transport/grpc/internal/transitionwire"
)

type chunkPeer struct {
	pb.TransitionSourceServiceClient
	raw    string
	fence  fact.SourceFence
	mutate func(*pb.ReadTransitionChunkRequest, *pb.TransitionChunk) error
	calls  int
}

func (p *chunkPeer) ReadTransitionChunk(_ context.Context, q *pb.ReadTransitionChunkRequest, _ ...grpc.CallOption) (*pb.ReadTransitionChunkResponse, error) {
	p.calls++
	start, end := int(q.GetOffset()), min(len(p.raw), int(q.GetOffset())+int(q.GetMaxBytes()))
	r := &pb.TransitionChunk{Binding: proto.Clone(q.Binding).(*pb.TransitionApprovalBinding), Fence: transitionwire.FenceWire(p.fence),
		Offset: proto.Uint64(q.GetOffset()), TotalBytes: proto.Uint64(uint64(len(p.raw))), Data: []byte(p.raw[start:end])}
	if p.mutate != nil {
		if err := p.mutate(q, r); err != nil {
			return nil, err
		}
	}
	return &pb.ReadTransitionChunkResponse{Outcome: &pb.ReadTransitionChunkResponse_Result{Result: r}}, nil
}

func pullFixture(t *testing.T) (contract.TransitionApprovalBinding, *pb.TransitionApprovalBinding, *chunkPeer) {
	t.Helper()
	source, target := values.FactAuthorityRef("00000000-0000-4000-8000-000000000001"), values.FactAuthorityRef("00000000-0000-4000-8000-000000000002")
	id := values.ElementID("00000000-0000-4000-8000-000000000003")
	expected, _ := values.PublicTokenFromBytes(source, "record-revision", [32]byte{9})
	b := contract.TransitionApprovalBinding{Source: source, Operation: "transfer", Command: contract.TransitionCommand{Kind: contract.TransitionRoot, Target: target, Boundary: id, Expected: expected, SourceAddress: "https://127.0.0.1:1234", TargetAddress: "https://127.0.0.1:5678"}}
	w, err := transitionwire.BindingWire(b)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Repeat("manifest", transitionwire.MaxChunkBytes/8+3)
	f := fact.SourceFence{Input: fact.Intent{Profile: fact.RootSuccessor, Operation: b.Operation, Actor: values.PrincipalRef{Realm: "r", Principal: "u"}, Source: source, Target: target, Boundary: id,
		Expected: token.RevisionFromParts([32]byte{1}, id, 2), SourceAddress: b.Command.SourceAddress, TargetAddress: b.Command.TargetAddress},
		FinalCut: token.FromParts([32]byte{1}, 3), ManifestDigest: sha256.Sum256([]byte(raw))}
	return b, w, &chunkPeer{raw: raw, fence: f}
}

func TestPullFrozenChecksWholeTranscript(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-binding", "wrong-offset", "too-large", "empty", "short", "extra", "changed-fence", "changed-total", "corrupt", "lost-tail", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			b, w, p := pullFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.mutate = func(q *pb.ReadTransitionChunkRequest, r *pb.TransitionChunk) error {
				if q.GetOffset() == 0 {
					return nil
				}
				switch mode {
				case "wrong-binding":
					r.Binding.Command.OperationId = []byte("other")
				case "wrong-offset":
					r.Offset = proto.Uint64(0)
				case "too-large":
					r.TotalBytes = proto.Uint64(1 << 63)
				case "empty":
					r.Data = nil
				case "short":
					r.Data = r.Data[:len(r.Data)-1]
				case "extra":
					r.Data = append(r.Data, 1)
				case "changed-fence":
					r.Fence.Actor.Principal = []byte("other")
				case "changed-total":
					r.TotalBytes = proto.Uint64(r.GetTotalBytes() + 1)
					r.Data = append(r.Data, 1)
				case "corrupt":
					r.Data[0] ^= 1
				case "lost-tail":
					return contract.ErrUnavailable
				case "cancelled":
					cancel()
				}
				return nil
			}
			v, err := pullFrozen(ctx, p, b, w)
			if mode != "valid" {
				if err == nil || v != (exchange.Frozen{}) {
					t.Fatalf("accepted %s: %v", mode, err)
				}
				return
			}
			actual, fence, raw := exchange.OpenFrozen(v)
			if err != nil || actual != b || fence != p.fence || raw != p.raw || p.calls != 2 {
				t.Fatalf("whole pull: %v calls=%d", err, p.calls)
			}
		})
	}
}

func TestPullFrozenBoundsBeforeAllocationAndRejectsMissingTail(t *testing.T) {
	for _, size := range []uint64{0, transitionwire.MaxManifestBytes + 1, ^uint64(0)} {
		b, w, p := pullFixture(t)
		p.mutate = func(_ *pb.ReadTransitionChunkRequest, r *pb.TransitionChunk) error {
			r.TotalBytes = proto.Uint64(size)
			return nil
		}
		if _, err := pullFrozen(context.Background(), p, b, w); err == nil || p.calls != 1 {
			t.Fatalf("unsafe allocation/continued read: size=%d calls=%d err=%v", size, p.calls, err)
		}
	}
}
