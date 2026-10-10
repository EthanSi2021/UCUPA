package transfer_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// Only the outbound source reads cross TLS in this test. Target control calls
// still use the real in-process owner; this is not full network orchestration.
type networkSourceTarget struct {
	target transition.TargetPeer
	source values.FactAuthorityRef
	caller values.CallerContext
	view   values.AuthorizationView
	reads  transition.SourceTransport
}

func (p networkSourceTarget) CheckImport(ctx context.Context, _ values.NextHop, _ values.FactAuthorityRef, b contract.TransitionApprovalBinding) error {
	return p.target.CheckTransitionImport(ctx, p.caller, b, p.view)
}
func (p networkSourceTarget) AcceptImport(ctx context.Context, _ values.NextHop, _ values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Acceptance, error) {
	return p.target.AcceptTransitionImport(ctx, p.source, p.caller, b, p.view, p.reads)
}
func (p networkSourceTarget) Install(ctx context.Context, _ values.NextHop, _ values.FactAuthorityRef, b contract.TransitionApprovalBinding) (transition.Completion, error) {
	return p.target.InstallTransition(ctx, p.source, b, p.reads)
}

func peerCertificate(t *testing.T) (tls.Certificate, *x509.CertPool, [32]byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, BasicConstraintsValid: true, IsCA: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: private}, roots, sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
}

func TestTransferPullsActualSourceOverAuthenticatedTLS(t *testing.T) {
	for _, rootTransfer := range []bool{false, true} {
		t.Run(map[bool]string{false: "slice", true: "root"}[rootTransfer], func(t *testing.T) {
			s, target := newFixture(t), newTargetFixture(t, rootTransfer)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			s.command.SourceAddress = values.NextHop("https://" + listener.Addr().String())
			b := beginPair(t, s, target, rootTransfer)
			serverCert, roots, pin := peerCertificate(t)
			clientCert, clientRoots, clientPin := peerCertificate(t)
			serverConfig := coregrpc.Config{
				Expected: b.Source, NextHop: b.Command.SourceAddress, Certificate: serverCert, TransitionSource: s.c.Transfers(),
				SessionTTL: time.Minute, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second,
				MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 128 << 10,
				ReadDelegation: &coregrpc.ReadDelegationConfig{ClientRoots: clientRoots, Peers: map[values.FactAuthorityRef][32]byte{target.c.AuthorityRef(): clientPin}, MaxTTL: time.Minute, MaxGrants: 8, MaxPerCaller: 4},
			}
			server, err := coregrpc.New(s.c.Access(), composition.RealmNetworkAuthentication{Connections: s.realmLinks(t), Owner: s.c.Access()}, serverConfig)
			if err != nil {
				t.Fatal(err)
			}
			go func(host *coregrpc.Server, l net.Listener) { _ = host.Serve(l) }(server, listener)
			defer func() { server.Stop() }()
			cfg := coresdk.Config{NextHop: b.Command.SourceAddress, Expected: b.Source, Roots: roots, SPKISHA256: pin, Actor: target.c.AuthorityRef(), ActorCertificate: &clientCert,
				RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192}
			client, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			reads, err := transitionconn.NewConnections(target.c.AuthorityRef(), []*coresdk.Client{client}, 2, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			view, err := target.c.Transfers().AuthorizeTransitionImport(context.Background(), target.caller, b)
			if err != nil {
				t.Fatal(err)
			}
			link := networkSourceTarget{target: target.c.Transfers(), source: b.Source, caller: target.caller, view: view, reads: reads}
			if _, err := reads.ReadTransitionFence(context.Background(), b.Command.SourceAddress, b.Source, b); err == nil {
				t.Fatal("unfenced source leaked a manifest")
			}
			recoverPair(t, s, b, link)
			frozen, err := reads.ReadTransitionFence(context.Background(), b.Command.SourceAddress, b.Source, b)
			if err != nil || frozen == (transition.Frozen{}) {
				t.Fatalf("retained fence: %v", err)
			}
			decision, err := reads.ReadTransitionDecision(context.Background(), b.Command.SourceAddress, b.Source, b)
			if err != nil || decision == (transition.Decision{}) {
				t.Fatalf("retained decision: %v", err)
			}
			server.Stop()
			s.reopen(t)
			listener, err = net.Listen("tcp", strings.TrimPrefix(string(b.Command.SourceAddress), "https://"))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			serverConfig.TransitionSource = s.c.Transfers()
			server, err = coregrpc.New(s.c.Access(), composition.RealmNetworkAuthentication{Connections: s.realmLinks(t), Owner: s.c.Access()}, serverConfig)
			if err != nil {
				t.Fatal(err)
			}
			go func(host *coregrpc.Server, l net.Listener) { _ = host.Serve(l) }(server, listener)
			// No session/approval renewal: these are retained promises for the
			// same pinned peer, not a renewed user authorization decision.
			restarted, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			reads, err = transitionconn.NewConnections(b.Command.Target, []*coresdk.Client{restarted}, 2, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := reads.ReadTransitionFence(context.Background(), b.Command.SourceAddress, b.Source, b); err != nil || got != frozen {
				t.Fatalf("reopened source fence changed: %v", err)
			}
			if got, err := reads.ReadTransitionDecision(context.Background(), b.Command.SourceAddress, b.Source, b); err != nil || got != decision {
				t.Fatalf("reopened source decision changed: %v", err)
			}
			changed := b
			changed.Operation = "other"
			if _, err := reads.ReadTransitionFence(context.Background(), b.Command.SourceAddress, b.Source, changed); err == nil {
				t.Fatal("different operation disclosed content")
			}
			if _, err := reads.ReadTransitionFence(context.Background(), "https://127.0.0.1:1", b.Source, b); err == nil {
				t.Fatal("implicit address fallback")
			}
			// A normal user client cannot turn a source operation into service evidence.
			cfg.Actor, cfg.ActorCertificate = "", nil
			user, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer user.Close()
			rawExpected, _ := values.DecodePublicBytes(b.Command.Expected.Value)
			q := &pb.ReadTransitionDecisionRequest{Binding: &pb.TransitionApprovalBinding{Source: proto.String(string(b.Source)), Command: &pb.BeginTransitionRequest{OperationId: []byte(b.Operation), Target: proto.String(string(b.Command.Target)), Expected: &netpb.Token{Value: rawExpected}}}}
			if _, err := user.TransitionSource().ReadTransitionDecision(context.Background(), q); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("user admitted: %v", err)
			}
			// A matching Ref with a different key cannot satisfy mutual TLS.
			cfg.Actor, cfg.ActorCertificate = b.Command.Target, &serverCert
			impostor, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer impostor.Close()
			badReads, err := transitionconn.NewConnections(b.Command.Target, []*coresdk.Client{impostor}, 1, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := badReads.ReadTransitionFence(context.Background(), b.Command.SourceAddress, b.Source, b); err == nil {
				t.Fatal("Ref string was treated as authentication")
			}
		})
	}
}
