package acceptance_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	realm "ucupa/ucupa-realm/contract"
)

func federationProductPeer(t *testing.T, host, peer *networkFixture) map[string]any {
	t.Helper()
	ca := filepath.Join(host.dir, "peer-"+string(peer.config.Expected)+".pem")
	writeNetworkFile(t, ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peer.config.Certificate.Certificate[1]}))
	pin := peer.clientConfig.SPKISHA256
	return map[string]any{"next_hop": peer.config.NextHop, "expected_core": peer.config.Expected, "ca_file": ca, "spki_sha256": values.EncodePublicBytes(pin[:])}
}

func TestNetworkFederationIndependentProductsRestart(t *testing.T) {
	runFederationProductsRestart(t, false)
}

func TestNetworkFederationNestedProductsRestart(t *testing.T) {
	runFederationProductsRestart(t, true)
}

func runFederationProductsRestart(t *testing.T, nested bool) {
	runFederationRealmProductsRestart(t, nested, false)
}

func runFederationRealmProductsRestart(t *testing.T, nested, networkRealms bool) {
	a, b, leaf := publicFederationNetworkFixture(t)
	var c *networkFixture
	if nested {
		c = addNestedProductCarrier(t, b)
		for _, permission := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionListDirect, corecontract.PermissionQueryRegion, corecontract.PermissionObserveElement, corecontract.PermissionObserveAttachment} {
			binding := realm.PermissionBinding{Subject: a.actor, Permission: permission, Scope: values.AnchorClosureScope(c.config.Expected, nestedProductAnchor)}
			if _, err := c.r.PutBinding(c.actor, binding, c.r.BindingSetRevision(), realm.OperationID("nested-original-"+string(permission))); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, permission := range []values.PermissionID{corecontract.PermissionObserveElement, corecontract.PermissionObserveAttachment} {
		binding := realm.PermissionBinding{Subject: a.actor, Permission: permission, Scope: values.AnchorClosureScope(b.config.Expected, values.ElementID(networkEntity))}
		if _, err := b.r.PutBinding(b.actor, binding, b.r.BindingSetRevision(), realm.OperationID("product-"+string(permission))); err != nil {
			t.Fatal(err)
		}
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	signing, verification := filepath.Join(a.dir, "product-issuer.pem"), filepath.Join(b.dir, "product-issuer.pub.pem")
	writeNetworkFile(t, signing, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	writeNetworkFile(t, verification, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
	ab, ba := federationProductPeer(t, a, b), federationProductPeer(t, b, a)
	aPeers, bPeers := []any{ab}, []any{ba}
	aTrust, bTrust := federationProductTrust(t, a, b), federationProductTrust(t, b, a)
	var startC func() func()
	if c != nil {
		aPeers = append(aPeers, federationProductPeer(t, a, c))
		bPeers = append(bPeers, federationProductPeer(t, b, c))
		aTrust, bTrust = federationProductTrust(t, a, b, c), federationProductTrust(t, b, a, c)
		ca, cb := federationProductPeer(t, c, a), federationProductPeer(t, c, b)
		verificationC := filepath.Join(c.dir, "product-issuer.pub.pem")
		writeNetworkFile(t, verificationC, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
		issuerC := map[string]any{"signing_public_key_file": verificationC, "next_hop": ca["next_hop"], "expected_core": ca["expected_core"], "ca_file": ca["ca_file"], "spki_sha256": ca["spki_sha256"]}
		c.server.Stop()
		_ = c.client.Close()
		startC = prepareNetworkProduct(t, c, map[string]any{"core_store": filepath.Join(c.dir, "empty-carrier.db"), "mount_connections": []any{cb}, "read_delegation": federationProductTrust(t, c, a, b), "external_identity": map[string]any{"max_ttl": "30s", "issuers": map[string]any{string(a.actor.Realm): issuerC}}})
	}
	// Provisioning used real owner/control APIs. From here all owners are closed;
	// only separate product executables and generated public RPCs are used.
	for _, f := range []*networkFixture{a, b} {
		f.server.Stop()
		if f.client != nil {
			_ = f.client.Close()
		}
	}
	extraA := map[string]any{
		"mount_connections": []any{ab}, "read_delegation": aTrust,
		"identity_issuer": map[string]any{"signing_key_file": signing, "max_ttl": "30s", "max_proofs": 32, "max_per_caller": 24},
		"federation":      map[string]any{"ttl": "20s", "max_parts": 16, "peers": aPeers},
	}
	issuer := map[string]any{"signing_public_key_file": verification, "next_hop": ba["next_hop"], "expected_core": ba["expected_core"], "ca_file": ba["ca_file"], "spki_sha256": ba["spki_sha256"]}
	extraB := map[string]any{"core_store": filepath.Join(b.dir, "empty-carrier.db"), "mount_connections": bPeers, "read_delegation": bTrust, "external_identity": map[string]any{"max_ttl": "30s", "issuers": map[string]any{string(a.actor.Realm): issuer}}}
	var startRealms func()
	if networkRealms {
		startRealms = prepareFederationNetworkRealms(t, a, b, extraA, extraB, verification, issuer)
	}
	startA := prepareNetworkProduct(t, a, extraA)
	startB := prepareNetworkProduct(t, b, extraB)
	if startRealms != nil {
		startRealms()
	}
	_ = startA()
	stopB := startB()
	if startC != nil {
		_ = startC()
	}
	a.connect(t)
	ctx := context.Background()
	var listContinuation *pb.ListDirectRequest
	var releaseList func()
	if nested {
		listContinuation, releaseList = verifyFederatedProductList(t, a, b)
	}
	handle := a.enter(t, pb.Intent_QUERY)
	root := &netpb.Address{Origin: proto.String(string(networkRoot)), Up: proto.Uint64(0), Down: &netpb.ElementPath{}}
	query := &pb.QueryRegionRequest{Handle: handle, Start: root, Basis: root, Sphere: &pb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(10)}, Limit: proto.Uint32(1), PrepareObserve: proto.Bool(true)}
	first, err := a.client.Core().QueryRegion(ctx, query)
	if err != nil || first.GetResult() == nil || first.GetResult().Continuation == nil || first.GetResult().Preparation == nil {
		t.Fatal("product first page", first, err)
	}
	begin, err := a.client.Core().BeginObserve(ctx, &pb.BeginObserveRequest{Preparation: first.GetResult().Preparation})
	if err != nil || begin.GetResult() == nil {
		t.Fatal(begin, err)
	}
	seen := map[string]bool{}
	var nestedAddress *netpb.Address
	page := first.GetResult()
	wantSources := 2
	if nested {
		wantSources = 3
	}
	if page.SourceGraph == nil || len(page.SourceGraph.Sources) != wantSources {
		t.Fatal("product source graph incomplete", page.SourceGraph)
	}
	for _, s := range page.SourceGraph.Sources {
		if s.Coverage != nil && len(s.Parents) == 0 {
			t.Fatal("product query source missing parent closure", s.GetOwner())
		}
	}
	for i := 0; i < 8; i++ {
		for _, match := range page.Matches {
			if seen[match.GetElement()] {
				t.Fatal("duplicate page")
			}
			seen[match.GetElement()] = true
			if match.GetElement() == string(nestedProductLeaf) {
				nestedAddress = match.Address
			}
		}
		if page.Continuation == nil {
			break
		}
		query.Continuation = page.Continuation
		response, err := a.client.Core().QueryRegion(ctx, query)
		if err != nil || response.GetResult() == nil {
			t.Fatal(response, err)
		}
		page = response.GetResult()
		if !proto.Equal(page.SourceGraph, first.GetResult().SourceGraph) {
			t.Fatal("product source graph changed across pages")
		}
		if len(page.Closure) != 32 || !bytes.Equal(page.Closure, first.GetResult().Closure) {
			t.Fatal("product continuation changed closure")
		}
	}
	if !seen[string(leaf)] || page.GetStatus() != pb.Coverage_COMPLETE {
		t.Fatal("mounted product source omitted", seen, page)
	}
	if c != nil && (!seen[string(nestedProductAnchor)] || !seen[string(nestedProductLeaf)]) {
		t.Fatal("nested source omitted", seen)
	}
	if c != nil {
		read, err := a.client.Core().ReadElement(ctx, &pb.ReadElementRequest{Handle: handle, Address: nestedAddress, Basis: first.GetResult().Receipt})
		if err != nil || read.GetResult().GetRecord().GetElement() != string(nestedProductLeaf) || read.GetResult().GetRecord().GetRevision().GetOwner() != string(c.config.Expected) {
			t.Fatal("nested product same-basis read", read, err)
		}
		if !bytes.Equal(read.GetResult().Closure, first.GetResult().Closure) {
			t.Fatal("nested historical read replaced closure")
		}
		if !proto.Equal(read.GetResult().SourceGraph, first.GetResult().SourceGraph) {
			t.Fatal("product exact Read replaced source vector")
		}
		verifyNestedProductObservation(t, a, c, begin.GetResult().Cursor)
	}
	stopB()
	offlineQuery := proto.Clone(query).(*pb.QueryRegionRequest)
	offlineQuery.Continuation = nil
	offlineQuery.PrepareObserve = proto.Bool(false)
	offlineQuery.Limit = proto.Uint32(64)
	offline, err := a.client.Core().QueryRegion(ctx, offlineQuery)
	if err != nil || offline.GetResult() == nil || offline.GetResult().GetStatus() != pb.Coverage_PARTIAL || offline.GetResult().GetGap() != pb.QueryGap_UNRESOLVED_COVERAGE {
		t.Fatal("offline source misreported", offline, err)
	}
	for _, match := range offline.GetResult().Matches {
		if match.GetElement() == string(leaf) || match.GetElement() == string(nestedProductLeaf) {
			t.Fatal("offline source fabricated a live result")
		}
	}
	_ = startB()
	if listContinuation != nil {
		oldList, err := a.client.Core().ListDirect(ctx, listContinuation)
		if err == nil && oldList.GetResult() != nil {
			t.Fatal("list silently replaced restarted source", oldList)
		}
		assertLostFederationLease(t, oldList.GetFailure(), err)
		releaseList()
	}
	// Neither a public continuation nor Observe may replace B's lost run-local
	// source lease after restart. A new query may establish a new lease.
	query.Continuation = first.GetResult().Continuation
	old, err := a.client.Core().QueryRegion(ctx, query)
	if err == nil && old.GetResult() != nil {
		t.Fatal("old continuation renewed after restart", old)
	}
	assertLostFederationLease(t, old.GetFailure(), err)
	poll, err := a.client.Core().PollChanges(ctx, &pb.PollChangesRequest{Cursor: begin.GetResult().Cursor})
	if err == nil && poll.GetResult() != nil {
		t.Fatal("old observation renewed", poll)
	}
	assertLostFederationLease(t, poll.GetFailure(), err)
	query.Continuation = nil
	query.Limit = proto.Uint32(64)
	fresh, err := a.client.Core().QueryRegion(ctx, query)
	if err != nil || fresh.GetResult() == nil || fresh.GetResult().GetStatus() != pb.Coverage_COMPLETE {
		t.Fatal("fresh query failed", fresh, err)
	}
	found := false
	foundNested := c == nil
	for _, match := range fresh.GetResult().Matches {
		found = found || match.GetElement() == string(leaf)
		foundNested = foundNested || match.GetElement() == string(nestedProductLeaf)
	}
	if !found || !foundNested {
		t.Fatal("restart lost durable contents")
	}
}

func assertLostFederationLease(t *testing.T, failure netpb.Failure, err error) {
	t.Helper()
	if err != nil {
		if code := status.Code(err); code != codes.Unauthenticated && code != codes.Unavailable {
			t.Fatal("unrelated transport failure", err)
		}
		return
	}
	// Restart discards the source's finite identity grant. Its authentication
	// rejection maps to DENIED at the entry; this is not renewed authorization.
	if failure != netpb.Failure_GAP && failure != netpb.Failure_INVALID_CONTEXT && failure != netpb.Failure_UNAVAILABLE && failure != netpb.Failure_DENIED {
		t.Fatal("unrelated owner failure", failure)
	}
}
