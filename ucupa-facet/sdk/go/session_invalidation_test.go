package facetsdk

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
	netpb "ucupa/shared/bindings/go/protobuf"
)

func TestFailedSessionInvalidationDoesNotReplayOrClearNewEstablishment(t *testing.T) {
	for _, mode := range []string{"rejected", "renewed", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			c := &Client{config: Config{RPCDeadline: time.Second}, session: "old"}
			calls := 0
			code := codes.Unauthenticated
			if mode == "unavailable" {
				code = codes.Unavailable
			}
			invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				calls++
				if mode == "renewed" {
					c.mu.Lock()
					c.session = "new"
					c.mu.Unlock()
				}
				return status.Error(code, "session rejected")
			}
			err := c.intercept(context.Background(), netpb.SessionService_ReleaseSession_FullMethodName, &netpb.ReleaseSessionRequest{}, &netpb.ReleaseSessionResponse{}, nil, invoke, grpc.StaticMethod())
			want := map[string]string{"rejected": "", "renewed": "new", "unavailable": "old"}[mode]
			if status.Code(err) != code || calls != 1 || c.session != want {
				t.Fatalf("calls=%d session=%s err=%v", calls, c.session, err)
			}
		})
	}
}
