package acceptance_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
)

type countedNetworkAuth struct {
	composition.RealmNetworkAuthentication
	calls atomic.Int32
}

func TestNetworkRevokeRegrantDoesNotReviveObservation(t *testing.T) {
	f := newNetworkFixture(t, true)
	ctx := context.Background()
	handle := f.enter(t, corepb.Intent_READ)
	initial, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}, PrepareObserve: proto.Bool(true)})
	if err != nil || initial.GetResult().Preparation == nil {
		t.Fatalf("prepared read: %v %v", initial, err)
	}
	scope := values.AnchorClosureScope(f.config.Expected, networkRoot)
	f.grant(t, corecontract.PermissionReadElement, scope, true)
	f.grant(t, corecontract.PermissionReadElement, scope, false)
	observed, err := f.client.Core().BeginObserve(ctx, &corepb.BeginObserveRequest{Preparation: initial.GetResult().Preparation})
	if err != nil || observed.GetFailure() != networkpb.Failure_GAP {
		t.Fatalf("old authorization basis revived: %v %v", observed, err)
	}
	_, err = f.r.RevokeCredential(f.admin, realm.CredentialID(f.id), f.r.CredentialSetRevision(), "revoke-network-caller")
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: handle, Address: &networkpb.Address{}})
	if err != nil || read.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("session bypassed credential revocation: %v %v", read, err)
	}
}

func (a *countedNetworkAuth) Establish(ctx context.Context, realm values.RealmID, id [16]byte, secret [32]byte) (values.CallerContext, error) {
	a.calls.Add(1)
	return a.RealmNetworkAuthentication.Establish(ctx, realm, id, secret)
}

func TestNetworkCredentialsNeverReachUntrustedTarget(t *testing.T) {
	f := newNetworkFixture(t, false)
	l, err := net.Listen("tcp", strings.TrimPrefix(string(f.config.NextHop), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	auth := &countedNetworkAuth{RealmNetworkAuthentication: composition.RealmNetworkAuthentication{Connections: f.realmLinks, Owner: f.c.Access()}}
	f.config.FacetControl = f.c.Facets()
	f.config.MountControl = f.c.Mounts()
	f.server, err = coregrpc.New(f.c.Access(), auth, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = f.server.Serve(l) }()
	for _, kind := range []string{"unknown-ca", "wrong-pin", "wrong-owner"} {
		t.Run(kind, func(t *testing.T) {
			cfg := f.clientConfig
			switch kind {
			case "unknown-ca":
				cfg.Roots = x509.NewCertPool()
			case "wrong-pin":
				cfg.SPKISHA256[0] ^= 1
			case "wrong-owner":
				cfg.Expected = values.FactAuthorityRef(networkEntity)
			}
			client, err := coresdk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err = client.Establish(context.Background(), f.actor.Realm, f.id, f.secret); err == nil {
				t.Fatal("untrusted target authenticated")
			}
			if auth.calls.Load() != 0 {
				t.Fatal("credentials reached Realm through untrusted target")
			}
		})
	}
	// An environment proxy is not a second route or a credential recipient.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	f.connect(t)
	if auth.calls.Load() != 1 {
		t.Fatal("direct authenticated connection not used")
	}
}

func TestNetworkMetadataAndUnsupportedRPC(t *testing.T) {
	f := newNetworkFixture(t, true)
	target, _ := f.config.NextHop.NetworkTarget()
	cc, err := grpc.NewClient("passthrough:///"+target, grpc.WithNoProxy(), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: f.clientConfig.Roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"h2"}})))
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	sessions := networkpb.NewSessionServiceClient(cc)
	for _, md := range []metadata.MD{metadata.MD{}, metadata.Pairs("ucupa-target", string(f.config.Expected), "ucupa-target", string(f.config.Expected)), metadata.Pairs("ucupa-target", "bad-id")} {
		ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), md), f.clientConfig.RPCDeadline)
		_, err = sessions.EstablishSession(ctx, &networkpb.EstablishSessionRequest{Audience: proto.String(string(f.actor.Realm)), CredentialId: f.id[:], Secret: f.secret[:]})
		cancel()
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid metadata: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.Pairs("ucupa-target", string(f.config.Expected))), f.clientConfig.RPCDeadline)
	defer cancel()
	err = cc.Invoke(ctx, "/ucupa.core.CoreService/NotProvided", &networkpb.ReleaseSessionRequest{}, &networkpb.ReleaseSessionResponse{})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("missing RPC: %v", err)
	}
}

func TestNetworkNoPlaintextOrMissingALPN(t *testing.T) {
	f := newNetworkFixture(t, true)
	target, _ := f.config.NextHop.NetworkTarget()
	plain, err := net.DialTimeout("tcp", target, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	_ = plain.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = plain.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
	var data [32]byte
	if n, err := plain.Read(data[:]); err == nil || n != 0 {
		t.Fatalf("plaintext accepted: %d %v", n, err)
	}
	for _, protocols := range [][]string{nil, {"http/1.1"}} {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", target, &tls.Config{RootCAs: f.clientConfig.Roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: protocols})
		if err != nil {
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = conn.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
		n, err := conn.Read(data[:])
		_ = conn.Close()
		if err == nil || n != 0 {
			t.Fatalf("missing h2 negotiation accepted: %d %v", n, err)
		}
	}
}
