package acceptance_test

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	corecontract "ucupa/ucupa-core/contract"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	core "ucupa/ucupa-core/instance"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

type dropListener struct {
	net.Listener
	mu          sync.Mutex
	connections []net.Conn
}

func (l *dropListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e == nil {
		l.mu.Lock()
		l.connections = append(l.connections, c)
		l.mu.Unlock()
	}
	return c, e
}
func (l *dropListener) drop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.connections {
		_ = c.Close()
	}
	l.connections = nil
}

type droppingOwner struct {
	*core.Core
	listener *dropListener
	calls    atomic.Int32
}

func (o *droppingOwner) PublicCall(ctx context.Context, caller values.CallerContext, in []byte) ([]byte, error) {
	out, err := o.Core.Access().PublicCall(ctx, caller, in)
	parsed, _ := values.ParseCanonicalJSON(in, values.PublicJSONLimits())
	if request, ok := parsed.(map[string]any); ok && request["op"] == "create-entity" {
		o.calls.Add(1)
		if err == nil {
			o.listener.drop()
		}
	}
	return out, err
}

func TestNetworkLostResponseRecoversWithoutAutomaticMutationRetry(t *testing.T) {
	f := newNetworkFixture(t, false)
	listener, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	l := &dropListener{Listener: listener}
	owner := &droppingOwner{Core: f.c, listener: l}
	f.server, err = coregrpc.New(owner, composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
	f.connect(t)
	ctx := context.Background()
	request := &corepb.CreateEntityRequest{Handle: f.enter(t, corepb.Intent_CREATE_ENTITY), Element: proto.String(networkEntity), OperationId: []byte("lost-response"), Final: networkTuple(1)}
	out, err := f.client.Core().CreateEntity(ctx, request)
	if err == nil || out != nil {
		t.Fatalf("cut connection reported definitive receipt: %v %v", out, err)
	}
	if owner.calls.Load() != 1 {
		t.Fatalf("automatic processed mutation retries: %d", owner.calls.Load())
	}
	// The outcome is still unknown to the consumer when the service is stopped.
	// Reopen the durable owner and establish a new session before recovering it.
	f.server.Stop()
	_ = f.client.Close()
	if err = f.c.Close(); err != nil {
		t.Fatal(err)
	}
	f.c, err = core.OpenDurable(f.realmLinks, filepath.Join(f.dir, "core.db"), corecontract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.config.Federation = f.c.Federation()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(reopened) }()
	f.connect(t)
	recovery := &corepb.RecoverOutcomeRequest{Reference: f.reference(), OperationId: request.OperationId, Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateEntity{CreateEntity: &corepb.CreateEntityCommand{Element: request.Element, OperationId: request.OperationId, Final: request.Final}}}}
	// An explicit recovery may encounter reconnect-in-progress. Retrying that
	// exact read of the recorded outcome never creates a new operation or CAS.
	var recovered *corepb.RecoverOutcomeResponse
	deadline := time.Now().Add(5 * time.Second)
	for {
		recovered, err = f.client.Core().RecoverOutcome(ctx, recovery)
		if err == nil {
			break
		}
		if status.Code(err) != codes.Unavailable || time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if recovered.GetResult() == nil || recovered.GetResult().GetRecord().GetElement() != networkEntity || owner.calls.Load() != 1 {
		t.Fatalf("recovery: %v calls=%d", recovered, owner.calls.Load())
	}
}

func TestNetworkSessionCapacityAndExpiry(t *testing.T) {
	f := newNetworkFixture(t, false)
	f.config.SessionTTL = time.Second
	f.config.MaxSessions = 1
	f.config.MaxSessionsPerCaller = 1
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.config.Federation = f.c.Federation()
	f.server, err = coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
	f.connect(t)
	handle := f.enter(t, corepb.Intent_READ)
	if err = f.client.Establish(context.Background(), f.actor.Realm, f.id, f.secret); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("session limit: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err = f.client.Core().ReadElement(context.Background(), &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired session: %v", err)
	}
	if err = f.client.Establish(context.Background(), f.actor.Realm, f.id, f.secret); err != nil {
		t.Fatalf("expired capacity not reclaimed: %v", err)
	}
}

func TestNetworkRejectsUnboundedAndMalformedCalls(t *testing.T) {
	f := newNetworkFixture(t, true)
	ctx := context.Background()
	handle := f.enter(t, corepb.Intent_QUERY)
	query := &corepb.QueryRegionRequest{Handle: handle, Start: &networkpb.Address{}, Basis: &networkpb.Address{}, Sphere: &corepb.Sphere{Center: &networkpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(1)}, UpDepth: proto.Int64(-2)}
	if _, err := f.client.Core().QueryRegion(ctx, query); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid depth: %v", err)
	}
	oversized := &corepb.EnterRequest{Reference: f.reference(), Intent: corepb.Intent_READ.Enum()}
	oversized.Reference.TrustHint = proto.String(strings.Repeat("x", 5<<20))
	if _, err := f.client.Core().Enter(ctx, oversized); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("oversized request: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.client.Core().QueryRegion(cancelled, query); status.Code(err) != codes.Canceled {
		t.Fatalf("cancelled: %v", err)
	}
}
