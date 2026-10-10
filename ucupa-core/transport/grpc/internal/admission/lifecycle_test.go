package admission

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
)

const admissionTestOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"

func admissionTargetContext(ctx context.Context, target string) context.Context {
	return metadata.NewIncomingContext(ctx, metadata.Pairs("ucupa-target", target))
}

func TestInterceptStopWaitAndAdmissionOrder(t *testing.T) {
	service := New(nil, nil, Config{Expected: admissionTestOwner, MaxConcurrentRPCs: 1})
	ctx := admissionTargetContext(context.Background(), string(admissionTestOwner))
	info := &grpc.UnaryServerInfo{FullMethod: networkpb.SessionService_ReleaseSession_FullMethodName}
	request := &networkpb.ReleaseSessionRequest{}
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Intercept(ctx, request, info, func(context.Context, any) (any, error) {
			close(entered)
			<-release
			return &networkpb.ReleaseSessionResponse{}, nil
		})
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("admitted RPC did not enter handler")
	}

	if listener := service.BeginStop(); listener != nil {
		t.Fatal("service without a bound listener returned one at stop")
	}
	waitStarted := make(chan struct{})
	waitDone := make(chan struct{})
	go func() {
		close(waitStarted)
		service.Wait()
		close(waitDone)
	}()
	<-waitStarted
	select {
	case <-waitDone:
		t.Fatal("Wait returned while the admitted handler was blocked")
	case <-time.After(25 * time.Millisecond):
	}

	unexpectedHandler := make(chan struct{}, 4)
	rejectedHandler := func(context.Context, any) (any, error) {
		unexpectedHandler <- struct{}{}
		return &networkpb.ReleaseSessionResponse{}, nil
	}
	if out, err := service.Intercept(admissionTargetContext(context.Background(), "invalid"), request, info, rejectedHandler); out != nil || status.Code(err) != codes.InvalidArgument {
		t.Fatalf("target validation no longer precedes capacity: %v %v", out, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if out, err := service.Intercept(admissionTargetContext(canceled, string(admissionTestOwner)), request, info, rejectedHandler); out != nil || status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("capacity no longer precedes context and stop: %v %v", out, err)
	}

	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("admitted RPC failed after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("admitted RPC did not finish after release")
	}
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("Wait did not finish after the admitted RPC returned")
	}

	if out, err := service.Intercept(admissionTargetContext(canceled, string(admissionTestOwner)), request, info, rejectedHandler); out != nil || status.Code(err) != codes.Canceled {
		t.Fatalf("context no longer precedes stopped state: %v %v", out, err)
	}
	if out, err := service.Intercept(ctx, request, info, rejectedHandler); out != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("stopped service admitted another RPC: %v %v", out, err)
	}
	if len(unexpectedHandler) != 0 {
		t.Fatalf("%d rejected RPCs entered the handler", len(unexpectedHandler))
	}
}

type admissionTestAddress string

func (address admissionTestAddress) Network() string { return "test" }
func (address admissionTestAddress) String() string  { return string(address) }

type admissionTestListener struct {
	closed     bool
	closeCalls int
}

func (*admissionTestListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (listener *admissionTestListener) Close() error {
	listener.closed = true
	listener.closeCalls++
	return nil
}
func (*admissionTestListener) Addr() net.Addr { return admissionTestAddress("admission-test") }

func TestBindListenerSharesStopLifecycle(t *testing.T) {
	service := New(nil, nil, Config{MaxConcurrentRPCs: 1})
	first := &admissionTestListener{}
	limited, err := service.BindListener(first, 2, time.Minute)
	if err != nil || limited == nil || first.closed {
		t.Fatalf("first listener was not bound: limited=%v err=%v closed=%v", limited, err, first.closed)
	}

	duplicate := &admissionTestListener{}
	if got, err := service.BindListener(duplicate, 2, time.Minute); got != nil || err == nil || !duplicate.closed || duplicate.closeCalls != 1 {
		t.Fatalf("duplicate listener was not closed and rejected: got=%v err=%v closed=%v calls=%d", got, err, duplicate.closed, duplicate.closeCalls)
	}

	if got := service.BeginStop(); got != limited || first.closed {
		t.Fatalf("stop did not return the single bound listener: got=%v want=%v closed=%v", got, limited, first.closed)
	}
	afterStop := &admissionTestListener{}
	if got, err := service.BindListener(afterStop, 2, time.Minute); got != nil || err == nil || !afterStop.closed || afterStop.closeCalls != 1 {
		t.Fatalf("post-stop listener was not closed and rejected: got=%v err=%v closed=%v calls=%d", got, err, afterStop.closed, afterStop.closeCalls)
	}
	if got := service.BeginStop(); got != limited {
		t.Fatalf("repeated stop returned a different listener: got=%v want=%v", got, limited)
	}
	if err := limited.Close(); err != nil || !first.closed || first.closeCalls != 1 {
		t.Fatalf("returned listener did not retain the accepted listener: err=%v closed=%v calls=%d", err, first.closed, first.closeCalls)
	}
}
