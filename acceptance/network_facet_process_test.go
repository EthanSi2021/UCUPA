package acceptance_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	facetsdk "ucupa/ucupa-facet/sdk/go"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func TestFacetAndCoreIndependentProducts(t *testing.T) {
	runFacetAndCoreProducts(t, false)
}

func TestWriterFacetIndependentProducts(t *testing.T) {
	runFacetAndCoreProducts(t, true)
}

func runFacetAndCoreProducts(t *testing.T, writerHandoff bool) {
	ctx := context.Background()
	var c *networkFixture
	if writerHandoff {
		c = newNetworkFixtureWriter(t, false, "network-realm", &values.WriterStartup{Handoff: true})
		for _, p := range []values.PermissionID{corecontract.PermissionInspectWriter, corecontract.PermissionHandoffWriter} {
			c.grant(t, p, values.AuthorityScope(c.c.AuthorityRef()), false)
		}
	} else {
		c = newNetworkFixture(t, false)
	}
	dir := t.TempDir()
	actor := values.PrincipalRef{Realm: "facet-product", Principal: "operator"}
	evidence := realm.CredentialEvidence{Audience: actor.Realm}
	rand.Read(evidence.ID[:])
	rand.Read(evidence.Secret[:])
	r, err := realminstance.CreateDurable(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: evidence}}, Bindings: []realm.PermissionBinding{{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)}}}, filepath.Join(dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	links := &composition.LocalFacetCoreConnections{}
	if err = links.BindEndpoint("local:core", composition.LocalFacetCoreEndpoint{Read: c.c.Facets(), Control: c.c.Facets(), Access: c.c.Access()}); err != nil {
		t.Fatal(err)
	}
	caller, err := c.r.EstablishCaller(realm.CredentialEvidence{Audience: c.actor.Realm, ID: realm.CredentialID(c.id), Secret: realm.CredentialSecret(c.secret)})
	if err != nil {
		t.Fatal(err)
	}
	source, err := links.ForFacetCaller(caller)
	if err != nil {
		t.Fatal(err)
	}
	fc := facet.Config{PublicAccessPolicy: publicPolicy()}
	if writerHandoff {
		fc.Writer = &values.WriterStartup{Handoff: true}
	}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:facet-product", r); err != nil {
		t.Fatal(err)
	}
	f, err := facet.CreateDurable(realmLinks, source, facetcontract.RealmInitialization{Realm: actor.Realm, NextHop: "local:facet-product"}, filepath.Join(dir, "facet.db"), fc)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	typ := fixtureType("content")
	grant := func(p values.PermissionID, scope values.OperationScope) {
		t.Helper()
		if _, err := r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: p, Scope: scope}, r.BindingSetRevision(), realm.OperationID(fmt.Sprint(p))); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionAccept, facetcontract.PermissionReadAcceptance, facetcontract.PermissionCoreContactUpdate, facetcontract.PermissionObserveElement} {
		grant(p, values.FacetElementScope(f.AuthorityRef(), networkRoot))
	}
	if writerHandoff {
		grant(facetcontract.PermissionInspectWriter, values.AuthorityScope(f.AuthorityRef()))
		grant(facetcontract.PermissionHandoffWriter, values.AuthorityScope(f.AuthorityRef()))
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionGet, facetcontract.PermissionPut, facetcontract.PermissionDelete, facetcontract.PermissionObserveKey} {
		grant(p, values.FacetKeyScope(f.AuthorityRef(), networkRoot, typ))
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetContactUpdate, corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionObserveAttachment} {
		c.grant(t, p, values.CoreElementScope(c.c.AuthorityRef(), networkRoot), false)
	}
	facetLinks := &composition.LocalFacetConnections{}
	if err = facetLinks.BindEndpoint("local:facet", f); err != nil {
		t.Fatal(err)
	}
	a, err := facetLinks.ForCore(actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.c.Facets().SetFacetAcceptanceSource(a); err != nil {
		t.Fatal(err)
	}
	before, err := c.c.Facets().ReadFacetAttachment(c.actor, networkRoot)
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.c.Facets().ProposeFacet(c.actor, networkRoot, f.AuthorityRef(), "local:facet", before.Attachment.Revision, "product-propose")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.Namespace.AcceptAttachment(ctx, actor, p.Pending, "local:core", "product-accept")
	if err != nil {
		t.Fatal(err)
	}
	activated, err := c.c.Facets().ActivateFacet(ctx, c.actor, networkRoot, p.Attachment.Revision, "product-activate")
	if err != nil {
		t.Fatal(err)
	}
	_, roots, pin, certPEM, keyPEM := networkCertificate(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	hop := values.NextHop("https://" + address)
	if _, err = c.c.Facets().UpdateFacetNextHop(c.actor, "product-facet-address", p.Pending, activated.NextHopRevision, hop); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Namespace.UpdateCoreNextHop(actor, "product-core-address", p.Pending, accepted.NextHopRevision, c.config.NextHop); err != nil {
		t.Fatal(err)
	}
	facetOwner := f.AuthorityRef()
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	coreConfig := map[string]any{"session_ttl": "2s", "facet_services": []any{map[string]any{"realm": c.actor.Realm, "principal": c.actor.Principal, "facet": facetOwner}}}
	if writerHandoff {
		coreConfig["writer"] = map[string]any{"handoff": true}
		coreConfig["session_ttl"] = "1m"
	}
	startCore := prepareNetworkProduct(t, c, coreConfig)
	stopCore := startCore()
	c.connect(t)
	read, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: c.enter(t, corepb.Intent_READ), Address: &networkpb.Address{}, PrepareObserve: proto.Bool(false)})
	if err != nil || read.GetResult() == nil {
		t.Fatalf("Core read: %v %v", read, err)
	}
	access := &facetpb.Access{NextHop: proto.String(string(hop)), Key: read.GetResult().Attachment.Key}
	writeNetworkFile(t, filepath.Join(dir, "tls.pem"), certPEM)
	writeNetworkFile(t, filepath.Join(dir, "tls.key"), keyPEM)
	writeNetworkFile(t, filepath.Join(dir, "core-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.config.Certificate.Certificate[1]}))
	credential, _ := json.Marshal(map[string]any{"audience": c.actor.Realm, "credential_id": values.EncodePublicBytes(c.id[:]), "secret": values.EncodePublicBytes(c.secret[:])})
	writeNetworkFile(t, filepath.Join(dir, "core-credential.json"), credential)
	corePin := c.clientConfig.SPKISHA256
	configuration := map[string]any{
		"listen": address, "next_hop": hop, "expected_facet": facetOwner, "realm_id": actor.Realm, "realm_next_hop": "local:facet-product",
		"facet_store": filepath.Join(dir, "facet.db"), "realm_store": filepath.Join(dir, "realm.db"), "certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"),
		"session_ttl": "30s", "rpc_deadline": "3s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
		"max_sessions": 8, "max_sessions_per_caller": 4, "max_connections": 8, "max_concurrent_rpcs": 8, "max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 4 << 20, "context_ttl": "1m", "observation_ttl": "1m",
		"core_connections": []any{map[string]any{"next_hop": c.config.NextHop, "expected_core": c.config.Expected, "ca_file": filepath.Join(dir, "core-ca.pem"), "spki_sha256": values.EncodePublicBytes(corePin[:]), "credential_file": filepath.Join(dir, "core-credential.json")}},
	}
	startFacet := prepareServiceProduct(t, dir, "ucupa-facet", configuration)
	if writerHandoff {
		setWriterProductConfig(t, dir, map[string]any{"handoff": true})
	}
	stopFacet := startFacet()
	clientConfig := facetsdk.Config{NextHop: hop, Expected: facetOwner, Roots: roots, SPKISHA256: pin, RPCDeadline: 4 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20}
	client, err := facetsdk.New(clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	login := func() {
		t.Helper()
		if err := client.Establish(ctx, evidence.Audience, [16]byte(evidence.ID), [32]byte(evidence.Secret)); err != nil {
			t.Fatal(err)
		}
	}
	login()
	api := client.Facet()
	get := &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes(), PrepareObserve: proto.Bool(true)}
	initial, err := api.Get(ctx, get)
	if err != nil || initial.GetResult() == nil {
		t.Fatalf("product initial read: %v %v", initial, err)
	}
	put, err := api.Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("product-put"), Payload: []byte(`"durable"`)})
	if err != nil || put.GetResult() == nil {
		t.Fatalf("product write: %v %v", put, err)
	}
	if writerHandoff {
		installation, request := writerRPCRequest(t)
		fenced, err := c.client.Writer().HandoffWriter(ctx, request)
		if err != nil || !fenced.GetResult().GetPending() {
			t.Fatalf("Core handoff in two-product cell: %v %v", fenced, err)
		}
		stopCore()
		prepareWriterInstall(t, c.dir, "ucupa-core", installation)
		stopCore = startCore()
		defer stopCore()
		_ = c.client.Close()
		c.connect(t)
		coreWriter, err := c.client.Writer().InspectWriter(ctx, &networkpb.WriterInspectRequest{Operation: []byte(installation.Operation)})
		if err != nil || coreWriter.GetResult().GetEpoch() != 2 || !coreWriter.GetResult().GetDecision().GetInstalled() {
			t.Fatalf("Core installed handoff in two-product cell: %v %v", coreWriter, err)
		}
		verifyFacetWriterProduct(t, dir, client, startFacet, stopFacet, login, get, put)
		return
	}
	// Only the service's 2s Core session expires. The Facet user session stays
	// alive; a new read reauthenticates, but an old preparation cannot survive.
	time.Sleep(2100 * time.Millisecond)
	latest, err := api.Get(ctx, get)
	if err != nil || latest.GetResult() == nil || string(latest.GetResult().Payload) != `"durable"` {
		t.Fatalf("service renewal: %v %v", latest, err)
	}
	old, err := api.BeginObserve(ctx, &facetpb.BeginObserveRequest{Preparation: initial.GetResult().Preparation})
	if err != nil || old.GetFailure() != networkpb.Failure_GAP {
		t.Fatalf("renewal preserved old control context: %v %v", old, err)
	}
	stopFacet()
	stopFacet = startFacet()
	defer stopFacet()
	login()
	latest, err = api.Get(ctx, get)
	if err != nil || latest.GetResult() == nil || string(latest.GetResult().Payload) != `"durable"` {
		t.Fatalf("Facet reopen: %v %v", latest, err)
	}
	stopCore()
	lost, err := api.Get(ctx, get)
	if err == nil && lost.GetFailure() != networkpb.Failure_UNAVAILABLE {
		t.Fatalf("Core offline became absence: %v", lost)
	}
}
