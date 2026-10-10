package admission

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
)

// A finite port stub exercises the admission lifecycle only. Real authorization
// and durable Core behavior are tested through public owners and product RPCs.
type importPort struct {
	after func()
}

func (*importPort) AuthorityRef() values.FactAuthorityRef { return admissionTestOwner }
func (p *importPort) AuthorizeTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding) (values.AuthorizationView, error) {
	if p.after != nil {
		p.after()
	}
	return values.AuthorizationView{1}, nil
}
func (p *importPort) CheckTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView) error {
	if p.after != nil {
		p.after()
	}
	return nil
}

func transitionApprovalFixture(t *testing.T) (*Service, *importPort, context.Context, context.Context, *pb.CreateTransitionApprovalRequest, [32]byte) {
	t.Helper()
	source := values.FactAuthorityRef("00000000-0000-4000-8000-000000000099")
	port := &importPort{}
	cert := &x509.Certificate{RawSubjectPublicKeyInfo: []byte("verified-test-peer-key")}
	s := New(nil, nil, Config{Expected: admissionTestOwner, TransitionImport: port,
		TransitionApprovals: &TransitionApprovalConfig{MaxTTL: time.Minute, MaxApprovals: 2, MaxPerCaller: 1},
		ReadDelegation:      &ReadDelegationConfig{Peers: map[values.FactAuthorityRef][32]byte{source: sha256.Sum256(cert.RawSubjectPublicKeyInfo)}},
	})
	raw := [32]byte{7}
	key := sha256.Sum256(raw[:])
	s.sessions[key] = session{caller: entryTestCaller, expires: time.Now().Add(10 * time.Second)}
	user := grpcsession.WithCaller(entryBearerContext(raw[:]), entryTestCaller)
	service := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}, PeerCertificates: []*x509.Certificate{cert}}}})
	request := &pb.CreateTransitionApprovalRequest{TtlSeconds: proto.Uint64(60), Binding: &pb.TransitionApprovalBinding{Source: proto.String(string(source)), Command: &pb.BeginTransitionRequest{
		OperationId: []byte{255, 0, 128}, Kind: pb.TransitionKind_TRANSITION_ROOT.Enum(), Target: proto.String(string(admissionTestOwner)), Boundary: proto.String(string(admissionTestOwner)), Expected: &netpb.Token{Owner: proto.String(string(source)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: make([]byte, 32)}, SourceAddress: proto.String("local:source"), TargetAddress: proto.String("local:target"),
	}}}
	return s, port, user, service, request, key
}

func TestTransitionApprovalSessionExpiryCapAndLostSession(t *testing.T) {
	s, port, user, service, request, key := transitionApprovalFixture(t)
	issued, err := s.CreateTransitionApproval(user, request)
	if err != nil || issued.GetResult().GetExpiresInSeconds() > 10 {
		t.Fatalf("expiry escaped session: %v %v", issued, err)
	}
	check := &pb.CheckTransitionApprovalRequest{Binding: request.Binding, Token: issued.GetResult().Token}
	if _, err := s.CheckTransitionApproval(service, check); err != nil {
		t.Fatal(err)
	}
	port.after = func() { s.mu.Lock(); delete(s.sessions, key); s.mu.Unlock() }
	if _, err := s.CheckTransitionApproval(service, check); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("lost original session after check: %v", err)
	}
}

func TestTransitionApprovalNoDisclosureAfterLifecycleChange(t *testing.T) {
	for _, mode := range []string{"expired", "replaced session", "stopped", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			s, port, user, _, request, key := transitionApprovalFixture(t)
			ctx, cancel := context.WithCancel(user)
			defer cancel()
			port.after = func() {
				switch mode {
				case "stopped":
					s.BeginStop()
				case "cancelled":
					cancel()
				default:
					s.mu.Lock()
					entry := s.sessions[key]
					if mode == "expired" {
						entry.expires = time.Now().Add(-time.Second)
					} else {
						entry.caller.Proof[0] ^= 1
					}
					s.sessions[key] = entry
					s.mu.Unlock()
				}
			}
			if _, err := s.CreateTransitionApproval(ctx, request); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("disclosed after %s: %v", mode, err)
			}
			if len(s.transitionApprovals) != 0 {
				t.Fatal("failed creation leaked approval")
			}
		})
	}
}

func TestTransitionApprovalBoundedConcurrencyAndNoCallerAliasing(t *testing.T) {
	s, _, user, service, request, _ := transitionApprovalFixture(t)
	var wg sync.WaitGroup
	results := make(chan *pb.CreateTransitionApprovalResponse, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := s.CreateTransitionApproval(user, request)
			if err != nil {
				errors <- err
			} else {
				results <- out
			}
		}()
	}
	wg.Wait()
	if len(results) != 1 || len(errors) != 7 || len(s.transitionApprovals) != 1 {
		t.Fatalf("capacity: results=%d errors=%d entries=%d", len(results), len(errors), len(s.transitionApprovals))
	}
	close(errors)
	for err := range errors {
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatal(err)
		}
	}
	a := (<-results).GetResult()
	original := proto.Clone(a.Binding).(*pb.TransitionApprovalBinding)
	request.Binding.Command.OperationId[0] = 1
	a.Binding.Command.OperationId[0] = 2
	if _, err := s.CheckTransitionApproval(service, &pb.CheckTransitionApprovalRequest{Binding: original, Token: a.Token}); err != nil {
		t.Fatalf("caller mutated retained binding: %v", err)
	}
	if _, err := s.CheckTransitionApproval(context.Background(), &pb.CheckTransitionApprovalRequest{Binding: original, Token: a.Token}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("token replaced service authentication: %v", err)
	}
	s.BeginStop()
	if len(s.transitionApprovals) != 0 {
		t.Fatal("stop retained runtime approvals")
	}
}
