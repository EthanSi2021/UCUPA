package facetsdk

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

type boundedSessionFixture struct {
	netpb.UnimplementedSessionServiceServer
	calls atomic.Uint64
	ttl   atomic.Uint64
	bad   atomic.Bool
}

func (s *boundedSessionFixture) EstablishSession(_ context.Context, in *netpb.EstablishSessionRequest) (*netpb.EstablishSessionResponse, error) {
	s.calls.Add(1)
	if in.GetAudience() == "" || len(in.CredentialId) != 16 || len(in.Secret) != 32 {
		return nil, status.Error(codes.Unauthenticated, "credential required")
	}
	token := make([]byte, 32)
	token[0] = byte(s.calls.Load())
	if s.bad.Load() {
		token = []byte{1}
	}
	return &netpb.EstablishSessionResponse{Session: token, ExpiresInSeconds: s.ttl.Load()}, nil
}

func (s *boundedSessionFixture) ReleaseSession(context.Context, *netpb.ReleaseSessionRequest) (*netpb.ReleaseSessionResponse, error) {
	return &netpb.ReleaseSessionResponse{}, nil
}

func newSessionFixture(t *testing.T) (*Client, *boundedSessionFixture) {
	t.Helper()
	session := &boundedSessionFixture{}
	session.ttl.Store(60)
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	netpb.RegisterSessionServiceServer(server, session)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	client := &Client{config: Config{Expected: responseOwner, RPCDeadline: time.Second, MaxResponseBytes: 4096}}
	// In-process transport isolates SDK lease behavior; TLS target checks are
	// independently exercised by the product network acceptance suite.
	conn, err := grpc.NewClient("passthrough:///session-fixture", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithUnaryInterceptor(client.intercept))
	if err != nil {
		t.Fatal(err)
	}
	client.conn = conn
	t.Cleanup(func() { _ = client.Close() })
	return client, session
}

func TestEnsureSessionBoundedReuseAndRenewal(t *testing.T) {
	c, s := newSessionFixture(t)
	audience := values.RealmID("realm")
	id, secret := [16]byte{1}, [32]byte{2}
	if err := c.EnsureSession(context.Background(), audience, id, secret); err != nil {
		t.Fatal(err)
	}
	first := c.session
	if err := c.EnsureSession(context.Background(), audience, id, secret); err != nil || s.calls.Load() != 1 {
		t.Fatalf("fresh session reauthenticated: %v calls=%d", err, s.calls.Load())
	}
	c.mu.Lock()
	c.sessionExpires = time.Now().Add(-time.Second)
	c.mu.Unlock()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- c.EnsureSession(context.Background(), audience, id, secret) }()
	}
	wg.Wait()
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if s.calls.Load() != 2 || c.session == first {
		t.Fatalf("concurrent renewal calls=%d", s.calls.Load())
	}
	id[0]++
	if err := c.EnsureSession(context.Background(), audience, id, secret); err != nil || s.calls.Load() != 3 {
		t.Fatalf("credential changed: %v %d", err, s.calls.Load())
	}
	if err := c.EnsureSession(context.Background(), "other-realm", id, secret); err != nil || s.calls.Load() != 4 {
		t.Fatalf("audience changed: %v %d", err, s.calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.EnsureSession(ctx, audience, id, secret); err == nil || s.calls.Load() != 4 {
		t.Fatalf("cancelled request contacted server: %v", err)
	}
	if err := c.Release(context.Background()); err != nil || c.session != "" {
		t.Fatalf("release: %v", err)
	}
}

func TestEnsureSessionRejectsMalformedOrUnboundedLease(t *testing.T) {
	for _, mode := range []string{"zero", "overflow", "token"} {
		t.Run(mode, func(t *testing.T) {
			c, s := newSessionFixture(t)
			switch mode {
			case "zero":
				s.ttl.Store(0)
			case "overflow":
				s.ttl.Store(^uint64(0))
			case "token":
				s.bad.Store(true)
			}
			if err := c.EnsureSession(context.Background(), "realm", [16]byte{1}, [32]byte{2}); status.Code(err) != codes.DataLoss || c.session != "" || !c.sessionExpires.IsZero() {
				t.Fatalf("bad session retained: %v", err)
			}
		})
	}
}
