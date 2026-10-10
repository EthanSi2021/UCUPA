package transfer_test

import (
	"context"
	"net"
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
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
)

// Parent control alone crosses TLS here. The mount fixture is real but local;
// this does not stand in for the subsequent three-party transfer workflow.
func TestNetworkCarrierControlOriginalCallerPermissionAndRestart(t *testing.T) {
	f, source, command := newCarrierFixture(t)
	cert, roots, pin := peerCertificate(t)
	listen := func(address string) net.Listener {
		t.Helper()
		l, err := net.Listen("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		return l
	}
	l := listen("127.0.0.1:0")
	address := l.Addr().String()
	config := coregrpc.Config{Expected: f.c.AuthorityRef(), NextHop: values.NextHop("https://" + address), Certificate: cert,
		SessionTTL: time.Minute, RPCDeadline: 5 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second,
		MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 128 << 10}
	serve := func(l net.Listener) *coregrpc.Server {
		t.Helper()
		config.TransitionControl = f.c.Transfers()
		h, err := coregrpc.New(f.c.Access(), composition.RealmNetworkAuthentication{Connections: f.realmLinks(t), Owner: f.c.Access()}, config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(h.Stop)
		go func() { _ = h.Serve(l) }()
		return h
	}
	host := serve(l)
	clientConfig := coresdk.Config{Expected: config.Expected, NextHop: config.NextHop, Roots: roots, SPKISHA256: pin,
		RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 128 << 10, MaxMetadataBytes: 8192}
	client, err := coresdk.New(clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	encode := func(v values.PublicToken, kind netpb.TokenKind) *netpb.Token {
		t.Helper()
		raw, err := values.DecodePublicBytes(v.Value)
		if err != nil {
			t.Fatal(err)
		}
		return &netpb.Token{Owner: proto.String(string(v.Owner)), Kind: kind.Enum(), Value: raw}
	}
	q := &pb.BeginCarrierTransitionRequest{OperationId: []byte{255, 0, 128}, Mount: proto.String(string(command.Mount)), Source: proto.String(string(command.Source)), Target: proto.String(string(command.Target)),
		ExpectedElement: encode(command.ExpectedElement, netpb.TokenKind_RECORD_REVISION), ExpectedMount: encode(command.ExpectedMount, netpb.TokenKind_MOUNT_REVISION),
		ParentAddress: proto.String(string(command.ParentAddress)), SourceAddress: proto.String(string(command.SourceAddress)), TargetAddress: proto.String(string(command.TargetAddress))}
	if _, err := client.Transitions().BeginCarrierTransition(ctx, q); status.Code(err) != codes.Unauthenticated {
		t.Fatal("missing session admitted", err)
	}
	if err := client.Establish(ctx, actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
	if r, err := client.Transitions().BeginCarrierTransition(ctx, q); err != nil || r.GetError() != netpb.Failure_DENIED {
		t.Fatal("mount rights implied transfer rights", r, err)
	}
	scope := values.CoreElementScope(f.c.AuthorityRef(), carrierMount)
	f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
	// A changed authorization view cannot be reused after revoke-and-regrant.
	f.a.afterAuthorize(func() {
		f.grant(t, actor, contract.PermissionTransitionCommit, scope, true)
		f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
	})
	if r, err := client.Transitions().BeginCarrierTransition(ctx, q); err != nil || r.GetResult() != nil {
		t.Fatal("stale authorization committed", r, err)
	}
	begin, err := client.Transitions().BeginCarrierTransition(ctx, q)
	if err != nil || begin.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_RESERVED {
		t.Fatal(begin, err)
	}
	inspect := &pb.InspectCarrierTransitionRequest{OperationId: q.OperationId, Mount: q.Mount}
	if r, err := client.Transitions().InspectCarrierTransition(ctx, inspect); err != nil || r.GetError() != netpb.Failure_DENIED {
		t.Fatal("commit implied inspect", r, err)
	}
	f.grant(t, actor, contract.PermissionTransitionInspect, scope, false)
	if r, err := client.Transitions().InspectCarrierTransition(ctx, inspect); err != nil || !proto.Equal(begin, r) {
		t.Fatal(r, err)
	}
	wrong := proto.Clone(q).(*pb.BeginCarrierTransitionRequest)
	wrong.TargetAddress = proto.String("local:other")
	if r, err := client.Transitions().BeginCarrierTransition(ctx, wrong); err != nil || r.GetError() != netpb.Failure_CONFLICT {
		t.Fatal("changed retry accepted", r, err)
	}
	cancel := &pb.CancelCarrierTransitionRequest{OperationId: q.OperationId, Mount: q.Mount, Expected: begin.GetResult().Revision}
	f.grant(t, actor, contract.PermissionTransitionCommit, scope, true)
	if r, err := client.Transitions().CancelCarrierTransition(ctx, cancel); err != nil || r.GetError() != netpb.Failure_DENIED {
		t.Fatal("revoked caller cancelled", r, err)
	}
	f.grant(t, actor, contract.PermissionTransitionCommit, scope, false)
	forged := proto.Clone(cancel).(*pb.CancelCarrierTransitionRequest)
	forged.Expected.Value[0] ^= 1
	if r, err := client.Transitions().CancelCarrierTransition(ctx, forged); err != nil || r.GetError() != netpb.Failure_CONFLICT {
		t.Fatal("forged comparison accepted", r, err)
	}
	host.Stop()
	f.reopen(t)
	host = serve(listen(address))
	if _, err := client.Transitions().InspectCarrierTransition(ctx, inspect); status.Code(err) != codes.Unauthenticated {
		t.Fatal("old session survived restart", err)
	}
	if err := client.Establish(ctx, actor.Realm, [16]byte{62}, [32]byte{63}); err != nil {
		t.Fatal(err)
	}
	if r, err := client.Transitions().BeginCarrierTransition(ctx, q); err != nil || !proto.Equal(begin, r) {
		t.Fatal("original reservation lost", r, err)
	}
	ended, err := client.Transitions().CancelCarrierTransition(ctx, cancel)
	if err != nil || ended.GetResult().GetProgress() != pb.CarrierProgress_CARRIER_CANCELLED {
		t.Fatal(ended, err)
	}
	if r, err := client.Transitions().CancelCarrierTransition(ctx, cancel); err != nil || !proto.Equal(ended, r) {
		t.Fatal("cancel retry changed result", r, err)
	}
	if r, err := client.Transitions().BeginCarrierTransition(ctx, q); err != nil || !proto.Equal(begin, r) {
		t.Fatal("begin did not retain original reservation", r, err)
	}
	if r, err := client.Transitions().InspectCarrierTransition(ctx, inspect); err != nil || !proto.Equal(ended, r) {
		t.Fatal("cancelled state revived", r, err)
	}
	read, err := source.Mounts().ReadMount(actor, carrierMount)
	if err != nil || !read.WriteGateOpen {
		t.Fatal("parent intent/cancel fenced source", read, err)
	}
	if err := client.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Transitions().CancelCarrierTransition(ctx, cancel); status.Code(err) != codes.Unauthenticated {
		t.Fatal("released session accepted", err)
	}
}
