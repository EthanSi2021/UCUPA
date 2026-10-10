package transfer_test

import (
	"context"
	"crypto/x509"
	"net"
	"strings"
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
	local "ucupa/ucupa-core/transport/local/transition"
)

// Only the two parent-evidence reads cross TLS here. Source/target control
// calls remain trusted-local, with their original independent Realm views.
type networkCarrierParent struct {
	owner values.FactAuthorityRef
	reads map[values.FactAuthorityRef]*transitionconn.Connections
}

func (p networkCarrierParent) AuthorityRef() values.FactAuthorityRef { return p.owner }
func (p networkCarrierParent) ReadCarrierPlan(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierPlan, error) {
	return p.reads[peer].ReadCarrierPlan(ctx, b.Command.ParentAddress, b.Parent, b)
}
func (p networkCarrierParent) ReadCarrierDecision(ctx context.Context, peer values.FactAuthorityRef, b contract.CarrierApprovalBinding) (transition.CarrierDecision, error) {
	return p.reads[peer].ReadCarrierDecision(ctx, b.Command.ParentAddress, b.Parent, b)
}

type carrierParentNetworkFixture struct {
	*carrierFixture
	host          *coregrpc.Server
	config        coregrpc.Config
	clients       map[values.FactAuthorityRef]*coresdk.Client
	clientConfigs map[values.FactAuthorityRef]coresdk.Config
	parent        networkCarrierParent
	w             *pb.CarrierApprovalBinding
}

func newCarrierParentNetwork(t *testing.T) *carrierParentNetworkFixture {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f := &carrierParentNetworkFixture{carrierFixture: newCarrierRecoveryWithParent(t, false, values.NextHop("https://"+l.Addr().String())),
		clients: make(map[values.FactAuthorityRef]*coresdk.Client), clientConfigs: make(map[values.FactAuthorityRef]coresdk.Config)}
	f.parent = networkCarrierParent{owner: f.binding.Parent, reads: make(map[values.FactAuthorityRef]*transitionconn.Connections)}
	cert, roots, pin := peerCertificate(t)
	peerRoots, pins := x509.NewCertPool(), make(map[values.FactAuthorityRef][32]byte)
	for _, owner := range []values.FactAuthorityRef{f.binding.Command.Source, f.binding.Command.Target} {
		peerCert, _, peerPin := peerCertificate(t)
		leaf, err := x509.ParseCertificate(peerCert.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		peerRoots.AddCert(leaf)
		pins[owner] = peerPin
		f.clientConfigs[owner] = coresdk.Config{NextHop: f.binding.Command.ParentAddress, Expected: f.binding.Parent, Roots: roots, SPKISHA256: pin,
			Actor: owner, ActorCertificate: &peerCert, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192}
	}
	f.config = coregrpc.Config{Expected: f.binding.Parent, NextHop: f.binding.Command.ParentAddress, Certificate: cert,
		SessionTTL: time.Minute, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second,
		MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 16, MaxConcurrentRPCs: 16, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 128 << 10,
		ReadDelegation: &coregrpc.ReadDelegationConfig{ClientRoots: peerRoots, Peers: pins, MaxTTL: time.Minute, MaxGrants: 8, MaxPerCaller: 4}}
	f.start(t, l)
	t.Cleanup(func() {
		f.host.Stop()
		for _, c := range f.clients {
			_ = c.Close()
		}
	})
	c := f.binding.Command
	e, _ := values.DecodePublicBytes(c.ExpectedElement.Value)
	m, _ := values.DecodePublicBytes(c.ExpectedMount.Value)
	f.w = &pb.CarrierApprovalBinding{Parent: proto.String(string(f.binding.Parent)), Command: &pb.BeginCarrierTransitionRequest{OperationId: []byte(f.binding.Operation),
		Source: proto.String(string(c.Source)), Target: proto.String(string(c.Target)), Mount: proto.String(string(c.Mount)),
		ExpectedElement: &netpb.Token{Owner: proto.String(string(f.binding.Parent)), Kind: netpb.TokenKind_RECORD_REVISION.Enum(), Value: e},
		ExpectedMount:   &netpb.Token{Owner: proto.String(string(f.binding.Parent)), Kind: netpb.TokenKind_MOUNT_REVISION.Enum(), Value: m},
		ParentAddress:   proto.String(string(c.ParentAddress)), SourceAddress: proto.String(string(c.SourceAddress)), TargetAddress: proto.String(string(c.TargetAddress))}}
	return f
}

func (f *carrierParentNetworkFixture) start(t *testing.T, l net.Listener) {
	t.Helper()
	f.config.TransitionParent = f.carrierFixture.parent.c.Transfers()
	var err error
	f.host, err = coregrpc.New(f.carrierFixture.parent.c.Access(), composition.RealmNetworkAuthentication{Connections: f.carrierFixture.parent.realmLinks(t), Owner: f.carrierFixture.parent.c.Access()}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func(h *coregrpc.Server) { _ = h.Serve(l) }(f.host)
	for owner, cfg := range f.clientConfigs {
		c, err := coresdk.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		f.clients[owner] = c
		f.parent.reads[owner], err = transitionconn.NewConnections(owner, []*coresdk.Client{c}, 4, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (f *carrierParentNetworkFixture) reopenParent(t *testing.T) {
	t.Helper()
	f.host.Stop()
	for _, c := range f.clients {
		_ = c.Close()
	}
	f.carrierFixture.parent.reopen(t)
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.binding.Command.ParentAddress), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f.start(t, l)
}

func (f *carrierParentNetworkFixture) link(t *testing.T) *local.CarrierConnection {
	t.Helper()
	sv, err := f.source.Transfers().AuthorizeCarrierExport(context.Background(), f.carrierFixture.parent.caller, f.binding)
	if err != nil {
		t.Fatal(err)
	}
	tv, err := f.target.c.Transfers().AuthorizeCarrierImport(context.Background(), f.target.caller, f.binding)
	if err != nil {
		t.Fatal(err)
	}
	c, err := local.NewCarrier(f.parent, f.source.Transfers(), f.target.c.Transfers(), f.binding, f.carrierFixture.parent.caller, sv, f.target.caller, tv)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
