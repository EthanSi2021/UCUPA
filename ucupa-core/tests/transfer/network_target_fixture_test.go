package transfer_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/transition"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	"ucupa/ucupa-core/transport/grpc/transitionconn"
)

// The hook delays only an actual source reply. It cannot create source proof or
// target acceptance, and allows a real target approval revoke during the pull.
type sourceReplyBoundary struct {
	transition.SourcePeer
	mu    sync.Mutex
	after func()
}

func (s *sourceReplyBoundary) ReadTransitionFence(ctx context.Context, peer values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Frozen, error) {
	f, err := s.SourcePeer.ReadTransitionFence(ctx, peer, b)
	s.mu.Lock()
	after := s.after
	s.after = nil
	s.mu.Unlock()
	if after != nil {
		after()
	}
	return f, err
}

type networkTargetFixture struct {
	source     *fixture
	target     *targetFixture
	b          contract.TransitionApprovalBinding
	w          *pb.TransitionApprovalBinding
	sourcePort *sourceReplyBoundary
	host       *coregrpc.Server
	config     coregrpc.Config
	peerConfig coresdk.Config
	peer, user *coresdk.Client
	control    *coresdk.Client
}

func newNetworkTargetFixture(t *testing.T, root bool) *networkTargetFixture {
	t.Helper()
	f := &networkTargetFixture{source: newFixture(t), target: newTargetFixture(t, root)}
	listen := func() net.Listener {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		return l
	}
	sl, tl := listen(), listen()
	f.source.command.SourceAddress = values.NextHop("https://" + sl.Addr().String())
	f.source.command.TargetAddress = values.NextHop("https://" + tl.Addr().String())
	f.b = beginPair(t, f.source, f.target, root)
	f.target.grant(t, contract.PermissionReadElement, f.b.Command.Boundary, false)
	f.w = networkBinding(t, f.b)
	scert, sroots, spin := peerCertificate(t)
	tcert, troots, tpin := peerCertificate(t)
	base := coregrpc.Config{SessionTTL: time.Minute, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second,
		MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 16, MaxConcurrentRPCs: 16, MaxStreams: 16, MaxMetadataBytes: 8192, MaxResponseBytes: 128 << 10}
	scfg := base
	scfg.Expected, scfg.NextHop, scfg.Certificate = f.b.Source, f.b.Command.SourceAddress, scert
	f.sourcePort = &sourceReplyBoundary{SourcePeer: f.source.c.Transfers()}
	scfg.TransitionSource = f.sourcePort
	scfg.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: troots, Peers: map[values.FactAuthorityRef][32]byte{f.b.Command.Target: tpin}, MaxTTL: time.Minute, MaxGrants: 8, MaxPerCaller: 4}
	sourceOutbound, err := coresdk.New(coresdk.Config{NextHop: f.b.Command.TargetAddress, Expected: f.b.Command.Target, Roots: troots, SPKISHA256: tpin, Actor: f.b.Source, ActorCertificate: &scert,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sourceOutbound.Close() })
	scfg.TransitionConnections, err = transitionconn.NewConnections(f.b.Source, []*coresdk.Client{sourceOutbound}, 4, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	scfg.TransitionControl, scfg.TransitionTarget = f.source.c.Transfers(), f.source.c.Transfers()
	shost, err := coregrpc.New(f.source.c.Access(), composition.RealmNetworkAuthentication{Connections: f.source.realmLinks(t), Owner: f.source.c.Access()}, scfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(shost.Stop)
	go func() { _ = shost.Serve(sl) }()
	f.control, err = coresdk.New(coresdk.Config{NextHop: f.b.Command.SourceAddress, Expected: f.b.Source, Roots: sroots, SPKISHA256: spin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.control.Close() })
	if err := f.control.Establish(context.Background(), actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
	readClient, err := coresdk.New(coresdk.Config{NextHop: f.b.Command.SourceAddress, Expected: f.b.Source, Roots: sroots, SPKISHA256: spin, Actor: f.b.Command.Target, ActorCertificate: &tcert,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = readClient.Close() })
	reads, err := transitionconn.NewConnections(f.b.Command.Target, []*coresdk.Client{readClient}, 4, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	f.config = base
	f.config.Expected, f.config.NextHop, f.config.Certificate = f.b.Command.Target, f.b.Command.TargetAddress, tcert
	f.config.TransitionImport, f.config.TransitionTarget, f.config.TransitionConnections = f.target.c.Transfers(), f.target.c.Transfers(), reads
	f.config.TransitionApprovals = &coregrpc.TransitionApprovalConfig{MaxTTL: time.Minute, MaxApprovals: 8, MaxPerCaller: 4}
	f.config.ReadDelegation = &coregrpc.ReadDelegationConfig{ClientRoots: sroots, Peers: map[values.FactAuthorityRef][32]byte{f.b.Source: spin}, MaxTTL: time.Minute, MaxGrants: 8, MaxPerCaller: 4}
	f.host, err = coregrpc.New(f.target.c.Access(), composition.RealmNetworkAuthentication{Connections: f.target.realmLinks(t), Owner: f.target.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.host.Stop() })
	go func(host *coregrpc.Server) { _ = host.Serve(tl) }(f.host)
	f.peerConfig = coresdk.Config{NextHop: f.b.Command.TargetAddress, Expected: f.b.Command.Target, Roots: troots, SPKISHA256: tpin, Actor: f.b.Source, ActorCertificate: &scert,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192}
	f.peer, err = coresdk.New(f.peerConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.peer.Close() })
	userConfig := f.peerConfig
	userConfig.Actor, userConfig.ActorCertificate = "", nil
	f.user, err = coresdk.New(userConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.user.Close() })
	if err := f.user.Establish(context.Background(), f.target.caller.Subject.Realm, [16]byte{72}, [32]byte{73}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *networkTargetFixture) approval(t *testing.T, seconds uint64) *pb.TransitionApproval {
	t.Helper()
	r, err := f.user.TransitionApprovals().CreateTransitionApproval(context.Background(), &pb.CreateTransitionApprovalRequest{Binding: f.w, TtlSeconds: proto.Uint64(seconds)})
	if err != nil || r.GetResult() == nil {
		t.Fatalf("approval: %v %v", r, err)
	}
	return r.GetResult()
}

func (f *networkTargetFixture) link(t *testing.T, a *pb.TransitionApproval) *transitionconn.TargetConnection {
	t.Helper()
	l, err := transitionconn.NewTargetConnection(f.peer, f.b, a)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (f *networkTargetFixture) reopenTarget(t *testing.T) {
	t.Helper()
	f.host.Stop()
	_ = f.peer.Close()
	f.target.reopen(t)
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.b.Command.TargetAddress), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f.config.TransitionTarget, f.config.TransitionImport = f.target.c.Transfers(), f.target.c.Transfers()
	f.host, err = coregrpc.New(f.target.c.Access(), composition.RealmNetworkAuthentication{Connections: f.target.realmLinks(t), Owner: f.target.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func(host *coregrpc.Server) { _ = host.Serve(l) }(f.host)
	f.peer, err = coresdk.New(f.peerConfig)
	if err != nil {
		t.Fatal(err)
	}
}

func networkBinding(t *testing.T, b contract.TransitionApprovalBinding) *pb.TransitionApprovalBinding {
	t.Helper()
	c := b.Command
	raw, err := values.DecodePublicBytes(c.Expected.Value)
	if err != nil {
		t.Fatal(err)
	}
	w := &pb.TransitionApprovalBinding{Source: proto.String(string(b.Source)), Command: &pb.BeginTransitionRequest{OperationId: []byte(b.Operation), Kind: pb.TransitionKind(c.Kind).Enum(), Target: proto.String(string(c.Target)), Boundary: proto.String(string(c.Boundary)),
		Expected: &netpb.Token{Owner: proto.String(string(c.Expected.Owner)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: raw}, SourceAddress: proto.String(string(c.SourceAddress)), TargetAddress: proto.String(string(c.TargetAddress))}}
	if c.Kind == contract.TransitionSlice {
		p, r := c.Final.LocalPose.Translation, c.Final.LocalPose.Rotation.Ratios()
		w.Command.Final = &pb.SpatialTuple{Parent: proto.String(string(c.Final.Parent)), Pose: &pb.Pose{Translation: &netpb.Vector3{Items: []float64{p.X, p.Y, p.Z}}, Rotation: &pb.Rotation{Pivot: proto.Uint32(uint32(c.Final.LocalPose.Rotation.Pivot())), Ratios: &netpb.Vector3{Items: []float64{r[0], r[1], r[2]}}}}, Range: &pb.Range{Kind: pb.RangeKind_BOUNDED_SPHERE.Enum(), Radius: proto.Float64(c.Final.Range.Radius())}}
	}
	return w
}
