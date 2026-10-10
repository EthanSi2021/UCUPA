package admission

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestTransitionPeerRejectsUserBearerWrongKeyAndStoppedHost(t *testing.T) {
	s, _, _, peer, q, _ := transitionApprovalFixture(t)
	actor := values.FactAuthorityRef(q.Binding.GetSource())
	if err := s.CheckTransitionPeer(peer, actor); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{context.Background(), metadata.NewIncomingContext(peer, metadata.Pairs("authorization", "Bearer user")), metadata.NewIncomingContext(peer, metadata.Pairs("authorization", "Delegation token"))} {
		if status.Code(s.CheckTransitionPeer(ctx, actor)) != codes.Unauthenticated {
			t.Fatal("non-service authentication admitted")
		}
	}
	if status.Code(s.CheckTransitionPeer(peer, s.config.Expected)) != codes.Unauthenticated {
		t.Fatal("self actor admitted")
	}
	s.config.ReadDelegation.Peers[actor] = [32]byte{3}
	if status.Code(s.CheckTransitionPeer(peer, actor)) != codes.Unauthenticated {
		t.Fatal("wrong key admitted")
	}
	s, _, _, peer, q, _ = transitionApprovalFixture(t)
	s.stopped = true
	if status.Code(s.CheckTransitionPeer(peer, values.FactAuthorityRef(q.Binding.GetSource()))) != codes.Unavailable {
		t.Fatal("stopped host admitted")
	}
	for _, method := range []string{pb.TransitionSourceService_ReadTransitionChunk_FullMethodName, pb.TransitionSourceService_ReadTransitionDecision_FullMethodName,
		pb.TransitionParentService_ReadCarrierPlan_FullMethodName, pb.TransitionParentService_ReadCarrierDecision_FullMethodName} {
		if !serviceIdentityMethod(method) {
			t.Fatalf("peer RPC incorrectly requires user bearer: %s", method)
		}
	}
}
