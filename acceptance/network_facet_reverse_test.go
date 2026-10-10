package acceptance_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facet "ucupa/ucupa-facet/instance"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
)

func TestFacetNetworkReverseControlAndObservation(t *testing.T) {
	f := newFacetNetworkFixture(t)
	w := f.w
	ctx := context.Background()
	cert, roots, pin, _, _ := networkCertificate(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hop := values.NextHop("https://" + l.Addr().String())
	owner := w.cores[0].AuthorityRef()
	config := coregrpc.Config{Expected: owner, NextHop: hop, Certificate: cert, SessionTTL: time.Minute, RPCDeadline: 3 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20, FacetServices: map[values.PrincipalRef]values.FactAuthorityRef{w.service: w.facets[0].AuthorityRef()}}
	config.FacetControl = w.cores[0].Facets()
	config.AttachmentControl = w.cores[0].Facets()
	config.MountControl = w.cores[0].Mounts()
	server, err := coregrpc.New(w.cores[0].Access(), composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.cores[0].Access()}, config)
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	defer server.Stop()
	go server.Serve(l)
	client, err := coresdk.New(coresdk.Config{Expected: owner, NextHop: hop, Roots: roots, SPKISHA256: pin, RPCDeadline: 3 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err = client.Establish(ctx, f.serviceEvidence.Audience, [16]byte(f.serviceEvidence.ID), [32]byte(f.serviceEvidence.Secret)); err != nil {
		t.Fatal(err)
	}
	source, err := composition.NewNetworkFacetCoreSource(w.facets[0].AuthorityRef(), []composition.FacetCoreEndpoint{{Address: hop, Core: owner, Client: client}})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := source.ReadProposal(ctx, hop, w.key)
	if err != nil || proposal.Active != w.key {
		t.Fatalf("proposal projection: %v %v", proposal, err)
	}
	acceptance, err := w.facets[0].Namespace.ReadAcceptance(w.actor, w.key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.facets[0].Namespace.UpdateCoreNextHop(w.actor, "network-reverse-contact", w.key, acceptance.NextHopRevision, hop); err != nil {
		t.Fatal(err)
	}
	f.server.Stop()
	if err = w.facets[0].Close(); err != nil {
		t.Fatal(err)
	}
	w.facets[0], err = facet.OpenDurable(w.realmLinks, source, w.paths[2], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	// The old local links cannot satisfy any reverse read after this point.
	if err = w.coreLinks.UnbindEndpoint("local:core0"); err != nil {
		t.Fatal(err)
	}
	address, _ := f.config.NextHop.NetworkTarget()
	l, err = net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	f.server, err = facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, f.config)
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	go f.server.Serve(l)
	f.login(t)
	api := f.client.Facet()
	read, err := api.Get(ctx, &facetpb.GetRequest{Access: f.access, TypeRef: w.typ.Bytes(), PrepareObserve: proto.Bool(true)})
	if err != nil || read.GetResult() == nil || read.GetResult().Preparation == nil {
		t.Fatalf("network F0: %v %v", read, err)
	}
	put, err := api.Put(ctx, &facetpb.PutRequest{Access: f.access, TypeRef: w.typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("network-reverse-put"), Payload: []byte(`"network"`)})
	if err != nil || put.GetResult() == nil {
		t.Fatalf("put: %v %v", put, err)
	}
	begun, err := api.BeginObserve(ctx, &facetpb.BeginObserveRequest{Preparation: read.GetResult().Preparation})
	if err != nil || begun.GetResult() == nil {
		t.Fatalf("begin: %v %v", begun, err)
	}
	polled, err := api.PollChanges(ctx, &facetpb.PollChangesRequest{Cursor: begun.GetResult().Cursor})
	if err != nil || polled.GetResult() == nil || len(polled.GetResult().Groups) != 1 {
		t.Fatalf("poll: %v %v", polled, err)
	}
	server.Stop()
	unavailable, err := api.Get(ctx, &facetpb.GetRequest{Access: f.access, TypeRef: w.typ.Bytes(), PrepareObserve: proto.Bool(false)})
	if err == nil && unavailable.GetFailure() != networkpb.Failure_UNAVAILABLE {
		t.Fatalf("offline Core became content or absence: %v", unavailable)
	}
}
