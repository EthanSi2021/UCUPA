package realmsdk

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

// gRPC fills this option with the immutable AuthInfo of the transport that
// actually answered, not a global counter sampled after the response.
func recordReplyTransport(t *testing.T, opts []grpc.CallOption, epoch uint64) {
	t.Helper()
	for _, opt := range opts {
		if capture, ok := opt.(grpc.PeerCallOption); ok {
			capture.PeerAddr.AuthInfo = connectionAuthInfo{epoch: epoch}
			return
		}
	}
	t.Fatal("response transport was not captured")
}

func connectionTestClient() *Client {
	return &Client{config: Config{Expected: "realm-a", RPCDeadline: time.Second, MaxResponseBytes: 1 << 20}, handshakeEpoch: 1, verifiedEpoch: 1}
}

func TestClientTargetReplyBindsOnlyTheAnsweringTransport(t *testing.T) {
	for _, tc := range []struct {
		name                string
		before, after, peer uint64
		code                codes.Code
	}{
		{"initial lazy handshake", 0, 1, 1, codes.OK},
		{"unchanged connection", 1, 1, 1, codes.OK},
		{"answer from rebuilt connection", 1, 2, 2, codes.OK},
		{"old answer after replacement", 1, 2, 1, codes.Unavailable},
		{"missing transport evidence", 1, 1, 0, codes.DataLoss},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := connectionTestClient()
			c.handshakeEpoch = tc.before
			out := &realmpb.CheckTargetResponse{}
			invoke := func(_ context.Context, _ string, _, response any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
				c.stateMu.Lock()
				c.handshakeEpoch = tc.after
				c.stateMu.Unlock()
				recordReplyTransport(t, opts, tc.peer)
				*response.(*realmpb.CheckTargetResponse) = realmpb.CheckTargetResponse{RealmId: proto.String("realm-a")}
				return nil
			}
			err := c.intercept(context.Background(), realmpb.RealmTargetService_CheckTarget_FullMethodName, &realmpb.CheckTargetRequest{}, out, nil, invoke, grpc.StaticMethod())
			if status.Code(err) != tc.code {
				t.Fatalf("target reply = %v, want %v", err, tc.code)
			}
			want := uint64(0)
			if tc.code == codes.OK {
				want = tc.peer
			}
			if c.verifiedEpoch != want {
				t.Fatalf("verified transport = %d, want %d", c.verifiedEpoch, want)
			}
		})
	}
}

func TestClientOldTargetReplyCannotOverwriteNewVerification(t *testing.T) {
	c := connectionTestClient()
	call := func(invoke grpc.UnaryInvoker) error {
		return c.intercept(context.Background(), realmpb.RealmTargetService_CheckTarget_FullMethodName, &realmpb.CheckTargetRequest{}, &realmpb.CheckTargetResponse{}, nil, invoke, grpc.StaticMethod())
	}
	old := func(_ context.Context, _ string, _, out any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
		if epoch, err := c.acceptHandshake(); err != nil || epoch != 2 {
			t.Fatalf("replacement handshake = %d %v", epoch, err)
		}
		fresh := func(_ context.Context, _ string, _, out any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
			recordReplyTransport(t, opts, 2)
			*out.(*realmpb.CheckTargetResponse) = realmpb.CheckTargetResponse{RealmId: proto.String("realm-a")}
			return nil
		}
		if err := call(fresh); err != nil {
			t.Fatal(err)
		}
		recordReplyTransport(t, opts, 1)
		*out.(*realmpb.CheckTargetResponse) = realmpb.CheckTargetResponse{RealmId: proto.String("realm-a")}
		return nil
	}
	if err := call(old); status.Code(err) != codes.Unavailable {
		t.Fatalf("stale target reply accepted: %v", err)
	}
	if c.verifiedEpoch != 2 {
		t.Fatalf("new verification overwritten: %d", c.verifiedEpoch)
	}
}

func TestClientProtectedCallAlwaysReleasesHandshakeGate(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"canceled", status.Error(codes.Canceled, "canceled")},
		{"deadline", status.Error(codes.DeadlineExceeded, "deadline")},
		{"disconnected", status.Error(codes.Unavailable, "disconnected")},
		{"malformed success", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := connectionTestClient()
			invoke := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
				if _, err := c.acceptHandshake(); err == nil {
					t.Fatal("in-flight handshake accepted")
				}
				return tc.err
			}
			err := c.intercept(context.Background(), realmpb.RealmIdentityService_EstablishCaller_FullMethodName, &realmpb.EstablishCallerRequest{}, &realmpb.EstablishCallerResponse{}, nil, invoke, grpc.StaticMethod())
			if err == nil || c.protectedInFlight != 0 {
				t.Fatalf("failure leaked the handshake gate: %v, in-flight=%d", err, c.protectedInFlight)
			}
			if epoch, err := c.acceptHandshake(); err != nil || epoch != 2 {
				t.Fatalf("reconnect after failure = %d %v", epoch, err)
			}
		})
	}
}

func TestClientTargetMismatchInvalidatesPriorVerification(t *testing.T) {
	c := connectionTestClient()
	invoke := func(_ context.Context, _ string, _, out any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
		recordReplyTransport(t, opts, 1)
		*out.(*realmpb.CheckTargetResponse) = realmpb.CheckTargetResponse{RealmId: proto.String("realm-b")}
		return nil
	}
	err := c.intercept(context.Background(), realmpb.RealmTargetService_CheckTarget_FullMethodName, &realmpb.CheckTargetRequest{}, &realmpb.CheckTargetResponse{}, nil, invoke, grpc.StaticMethod())
	if status.Code(err) != codes.Unavailable || c.verifiedEpoch != 0 {
		t.Fatalf("mismatch retained verification: %v, epoch=%d", err, c.verifiedEpoch)
	}
}

func TestClientCredentialsClonePreservesTheConnectionGate(t *testing.T) {
	c := connectionTestClient()
	base := &connectionCredentials{TransportCredentials: credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, ServerName: "realm-a"}), client: c}
	clone, ok := base.Clone().(*connectionCredentials)
	if !ok || clone.client != c {
		t.Fatal("credentials clone lost the shared connection gate")
	}
	if err := clone.OverrideServerName("clone"); err != nil {
		t.Fatal(err)
	}
	if base.Info().ServerName != "realm-a" || clone.Info().ServerName != "clone" {
		t.Fatal("TLS clone mutated the original configuration")
	}
}
