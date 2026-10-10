package acceptance_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetsdk "ucupa/ucupa-facet/sdk/go"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
)

type facetNetworkFixture struct {
	w               *publicFacetWorld
	server          *facetgrpc.Server
	client          *facetsdk.Client
	config          facetgrpc.Config
	clientConfig    facetsdk.Config
	evidence        realm.CredentialEvidence
	admin           realm.CredentialEvidence
	serviceEvidence realm.CredentialEvidence
	access          *facetpb.Access
}

func newFacetNetworkFixture(t *testing.T) *facetNetworkFixture {
	t.Helper()
	f := &facetNetworkFixture{evidence: realm.CredentialEvidence{Audience: "facet-world"}}
	rand.Read(f.evidence.ID[:])
	rand.Read(f.evidence.Secret[:])
	f.admin.Audience = f.evidence.Audience
	rand.Read(f.admin.ID[:])
	rand.Read(f.admin.Secret[:])
	f.serviceEvidence.Audience = f.evidence.Audience
	rand.Read(f.serviceEvidence.ID[:])
	rand.Read(f.serviceEvidence.Secret[:])
	f.w = newPublicFacetWorldCredentials(t, []realm.CredentialBootstrap{{Subject: "caller", Evidence: f.evidence}, {Subject: "caller", Evidence: f.admin}, {Subject: "service", Evidence: f.serviceEvidence}})
	cert, roots, pin, _, _ := networkCertificate(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hop := values.NextHop("https://" + l.Addr().String())
	owner := f.w.facets[0].AuthorityRef()
	f.config = facetgrpc.Config{Expected: owner, NextHop: hop, Certificate: cert, SessionTTL: 30 * time.Second, RPCDeadline: 3 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20}
	f.server, err = facetgrpc.New(f.w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: f.w.realmLinks, Owner: f.w.facets[0].Namespace}, f.config)
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	go f.server.Serve(l)
	f.clientConfig = facetsdk.Config{Expected: owner, NextHop: hop, Roots: roots, SPKISHA256: pin, RPCDeadline: 3 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 4 << 20, MaxMetadataBytes: 8192}
	f.client, err = facetsdk.New(f.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	k := f.w.access["key"].(map[string]any)
	p := k["proposal"].(map[string]any)
	b, err := values.DecodePublicBytes(p["value"].(string))
	if err != nil {
		t.Fatal(err)
	}
	f.access = &facetpb.Access{NextHop: proto.String(string(hop)), Key: &networkpb.AttachmentKey{Core: proto.String(k["core"].(string)), Facet: proto.String(k["facet"].(string)), Element: proto.String(k["element"].(string)), Proposal: &networkpb.Token{Owner: proto.String(p["owner"].(string)), Kind: networkpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: b}}}
	t.Cleanup(func() { f.client.Close(); f.server.Stop() })
	return f
}
func (f *facetNetworkFixture) login(t *testing.T) {
	t.Helper()
	if err := f.client.Establish(context.Background(), f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
}

func TestFacetNetworkContentLifecycle(t *testing.T) {
	f := newFacetNetworkFixture(t)
	ctx := context.Background()
	api := f.client.Facet()
	get := &facetpb.GetRequest{Access: f.access, TypeRef: f.w.typ.Bytes(), PrepareObserve: proto.Bool(true)}
	if _, err := api.Get(ctx, get); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no session %v", err)
	}
	f.login(t)
	before, err := api.Get(ctx, get)
	if err != nil || before.GetResult() == nil || before.GetResult().GetState() != facetpb.Presence_NOT_FOUND {
		t.Fatalf("get %v %v", before, err)
	}
	put := &facetpb.PutRequest{Access: f.access, TypeRef: f.w.typ.Bytes(), Payload: []byte(`"network"`), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("network-put")}
	written, err := api.Put(ctx, put)
	if err != nil || written.GetResult() == nil {
		t.Fatalf("put %v %v", written, err)
	}
	recovered, err := api.RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: f.access, OperationId: put.OperationId, Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: put}}})
	if err != nil || !proto.Equal(recovered.GetResult(), written.GetResult()) {
		t.Fatalf("recover %v %v", recovered, err)
	}
	begun, err := api.BeginObserve(ctx, &facetpb.BeginObserveRequest{Preparation: before.GetResult().Preparation})
	if err != nil || begun.GetResult() == nil {
		t.Fatalf("begin %v %v", begun, err)
	}
	polled, err := api.PollChanges(ctx, &facetpb.PollChangesRequest{Cursor: begun.GetResult().Cursor})
	if err != nil || polled.GetResult() == nil || len(polled.GetResult().Groups) != 1 {
		t.Fatalf("poll %v %v", polled, err)
	}
	listed, err := api.ListTypes(ctx, &facetpb.ListTypesRequest{Access: f.access})
	if err != nil || listed.GetResult() == nil || len(listed.GetResult().Types) != 1 {
		t.Fatalf("list %v %v", listed, err)
	}
	read, err := api.Get(ctx, &facetpb.GetRequest{Access: f.access, TypeRef: put.TypeRef})
	if err != nil || read.GetResult() == nil || !bytes.Equal(read.GetResult().Payload, put.Payload) {
		t.Fatalf("read %v %v", read, err)
	}
	deleted, err := api.Delete(ctx, &facetpb.DeleteRequest{Access: f.access, TypeRef: put.TypeRef, Expected: read.GetResult().Revision, OperationId: []byte("network-delete")})
	if err != nil || deleted.GetResult() == nil {
		t.Fatalf("delete %v %v", deleted, err)
	}
	released, err := api.ReleaseObserve(ctx, &facetpb.ReleaseObserveRequest{Context: &facetpb.ReleaseObserveRequest_Cursor{Cursor: polled.GetResult().NextCursor}})
	if err != nil || !released.GetResult().GetReleased() {
		t.Fatalf("release %v %v", released, err)
	}
	if _, err := api.Get(ctx, get, grpc.WaitForReady(true)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("option injection %v", err)
	}
	if err := f.client.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Get(ctx, get); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("released session %v", err)
	}
}

func TestFacetNetworkTargetAndSessionBoundaries(t *testing.T) {
	f := newFacetNetworkFixture(t)
	ctx := context.Background()
	for _, change := range []func(*facetsdk.Config){func(c *facetsdk.Config) { c.SPKISHA256[0] ^= 1 }, func(c *facetsdk.Config) { c.Expected = "00000000-0000-4000-8000-000000000999" }} {
		c := f.clientConfig
		change(&c)
		client, err := facetsdk.New(c)
		if err != nil {
			t.Fatal(err)
		}
		err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret))
		client.Close()
		if err == nil {
			t.Fatal("wrong target accepted")
		}
	}
	bad := [32]byte(f.evidence.Secret)
	bad[0] ^= 1
	if err := f.client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), bad); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad credential %v", err)
	}
	f.login(t)
	f.server.Stop()
	l, err := net.Listen("tcp", stringAddress(t, f.config.NextHop))
	if err != nil {
		t.Fatal(err)
	}
	f.server, err = facetgrpc.New(f.w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: f.w.realmLinks, Owner: f.w.facets[0].Namespace}, f.config)
	if err != nil {
		t.Fatal(err)
	}
	go f.server.Serve(l)
	request := &facetpb.GetRequest{Access: f.access, TypeRef: f.w.typ.Bytes()}
	deadline := time.Now().Add(8 * time.Second)
	for {
		_, err = f.client.Facet().Get(ctx, request)
		if status.Code(err) == codes.Unauthenticated {
			break
		}
		if status.Code(err) != codes.Unavailable || time.Now().After(deadline) {
			t.Fatalf("restart %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.login(t)
	if response, err := f.client.Facet().Get(ctx, request); err != nil || response.GetResult() == nil {
		t.Fatalf("new session %v %v", response, err)
	}
}
func stringAddress(t *testing.T, hop values.NextHop) string {
	t.Helper()
	target, ok := hop.NetworkTarget()
	if !ok {
		t.Fatal("bad fixture address")
	}
	return target
}

func TestFacetNetworkCredentialRevocationAndCapacity(t *testing.T) {
	f := newFacetNetworkFixture(t)
	for i := 0; i < f.config.MaxSessionsPerCaller; i++ {
		f.login(t)
	}
	if err := f.client.Establish(context.Background(), f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("session capacity %v", err)
	}
	request := &facetpb.GetRequest{Access: f.access, TypeRef: f.w.typ.Bytes()}
	if reply, err := f.client.Facet().Get(context.Background(), request); err != nil || reply.GetResult() == nil {
		t.Fatalf("existing session after capacity %v %v", reply, err)
	}
	if _, err := f.w.policy.RevokeCredential(f.admin, f.evidence.ID, f.w.policy.CredentialSetRevision(), "network-revoke"); err != nil {
		t.Fatal(err)
	}
	if reply, err := f.client.Facet().Get(context.Background(), request); err != nil || reply.GetFailure() != networkpb.Failure_DENIED {
		t.Fatalf("revoked existing session %v %v", reply, err)
	}
	if err := f.client.Establish(context.Background(), f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked establishment %v", err)
	}
}
