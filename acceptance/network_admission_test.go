package acceptance_test

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	core "ucupa/ucupa-core/instance"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

type pausedNetworkOwner struct {
	*core.Core
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (o *pausedNetworkOwner) PublicCall(ctx context.Context, caller values.CallerContext, raw []byte) ([]byte, error) {
	if o.armed.CompareAndSwap(true, false) {
		close(o.entered)
		<-o.release // Intentionally uncooperative fixture; never production behavior.
	}
	return o.Core.Access().PublicCall(ctx, caller, raw)
}

func TestNetworkAdmissionAndForcedShutdownAreBounded(t *testing.T) {
	f := newNetworkFixture(t, false)
	f.config.MaxConcurrentRPCs = 1
	f.config.ShutdownTimeout = 100 * time.Millisecond
	owner := &pausedNetworkOwner{Core: f.c, entered: make(chan struct{}), release: make(chan struct{})}
	defer close(owner.release)
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	f.server, err = coregrpc.New(owner, composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
	f.connect(t)
	handle := f.enter(t, corepb.Intent_READ)
	request := &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}}
	owner.armed.Store(true)
	finished := make(chan error, 1)
	go func() { _, err := f.client.Core().ReadElement(context.Background(), request); finished <- err }()
	select {
	case <-owner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not dispatched")
	}
	if _, err := f.client.Core().ReadElement(context.Background(), request); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("execution admission: %v", err)
	}
	stopped := make(chan struct{})
	go func() { f.server.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("forced shutdown waited for uncooperative handler")
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("shutdown fabricated success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller not released by shutdown")
	}
}

type observedNetworkListener struct {
	net.Listener
	accepted chan struct{}
}

func (l *observedNetworkListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted <- struct{}{}
	}
	return c, err
}

func TestNetworkShutdownClosesUnfinishedHandshake(t *testing.T) {
	f := newNetworkFixture(t, false)
	f.config.HandshakeTimeout = 30 * time.Second
	f.config.ShutdownTimeout = 100 * time.Millisecond
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedNetworkListener{Listener: l, accepted: make(chan struct{}, 1)}
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(observed) }()
	raw, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	// A TLS record prefix without the rest of the handshake.
	if _, err := raw.Write([]byte{0x16}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-observed.accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("connection not accepted")
	}
	stopped := make(chan struct{})
	go func() { f.server.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown waited for handshake timeout")
	}
	_ = raw.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err := raw.Read(b[:]); err == nil {
		t.Fatal("unfinished handshake remained connected")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("shutdown did not close raw connection")
	}
}
