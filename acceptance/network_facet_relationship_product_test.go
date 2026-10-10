package acceptance_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	netpb "ucupa/shared/bindings/go/protobuf"
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

// Relations are initially absent. Once product hosts start, every proposal,
// acceptance, activation, contact update, content mutation and detach is RPC.
func TestFacetRelationshipIndependentProducts(t *testing.T) {
	runFacetRelationshipProducts(t, false)
}

func runFacetRelationshipProducts(t *testing.T, networkRealm bool) {
	runFacetRelationshipRealmProducts(t, networkRealm, false, false)
}

func runFacetRelationshipRealmProducts(t *testing.T, networkRealm, combined, shared bool) {
	ctx := context.Background()
	c := newNetworkFixture(t, false)
	dir := t.TempDir()
	actor := values.PrincipalRef{Realm: "facet-relation", Principal: "operator"}
	credential := realm.CredentialEvidence{Audience: actor.Realm}
	rand.Read(credential.ID[:])
	rand.Read(credential.Secret[:])
	admin := realm.CredentialEvidence{Audience: actor.Realm}
	rand.Read(admin.ID[:])
	rand.Read(admin.Secret[:])
	var r *realminstance.Realm
	var err error
	realmStore := filepath.Join(dir, "realm.db")
	if shared {
		r, actor, admin = c.r, c.actor, c.admin
		credential = realm.CredentialEvidence{Audience: actor.Realm, ID: realm.CredentialID(c.id), Secret: realm.CredentialSecret(c.secret)}
		realmStore = filepath.Join(c.dir, "realm.db")
	} else {
		r, err = realminstance.CreateDurable(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: credential}, {Subject: actor.Principal, Evidence: admin}}, Bindings: []realm.PermissionBinding{{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)}, {Subject: actor, Permission: realm.PermissionRevokeCredential, Scope: values.RealmScope(actor.Realm)}, {Subject: actor, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope(actor.Realm)}}}, realmStore)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	// This provisioning adapter is unused: no relation or content exists yet.
	links := &composition.LocalFacetCoreConnections{}
	if err = links.BindEndpoint("local:bootstrap", composition.LocalFacetCoreEndpoint{Read: c.c.Facets(), Control: c.c.Facets(), Access: c.c.Access()}); err != nil {
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
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:facet-relation", r); err != nil {
		t.Fatal(err)
	}
	f, err := facet.CreateDurable(realmLinks, source, facetcontract.RealmInitialization{Realm: actor.Realm, NextHop: "local:facet-relation"}, filepath.Join(dir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	owner := f.AuthorityRef()
	typ := fixtureType("content")
	for _, p := range []values.PermissionID{facetcontract.PermissionAccept, facetcontract.PermissionReadAcceptance, facetcontract.PermissionCoreContactUpdate} {
		if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: p, Scope: values.FacetElementScope(owner, networkRoot)}, r.BindingSetRevision(), realm.OperationID(fmt.Sprint(p))); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionGet, facetcontract.PermissionPut, facetcontract.PermissionDelete} {
		if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: p, Scope: values.FacetKeyScope(owner, networkRoot, typ)}, r.BindingSetRevision(), realm.OperationID(fmt.Sprint(p))); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" {
		if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: facetcontract.PermissionListTypes, Scope: values.FacetElementScope(owner, networkRoot)}, r.BindingSetRevision(), "p8-list-types"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 256; i++ {
			other := fixtureType(fmt.Sprintf("p8-type-%d", i))
			if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: facetcontract.PermissionPut, Scope: values.FacetKeyScope(owner, networkRoot, other)}, r.BindingSetRevision(), realm.OperationID(fmt.Sprintf("p8-grant-type-%d", i))); err != nil {
				t.Fatalf("grant P8 type %d: %v", i, err)
			}
		}
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetDetach, corecontract.PermissionFacetContactUpdate} {
		c.grant(t, p, values.CoreElementScope(c.c.AuthorityRef(), networkRoot), false)
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" {
		seedP8FixedCell(t, c)
	}
	certificate, roots, pin, cert, key := networkCertificate(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()
	hop := values.NextHop("https://" + address)
	writeNetworkFile(t, filepath.Join(dir, "tls.pem"), cert)
	writeNetworkFile(t, filepath.Join(dir, "tls.key"), key)
	writeNetworkFile(t, filepath.Join(dir, "core-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.config.Certificate.Certificate[1]}))
	writeNetworkFile(t, filepath.Join(c.dir, "facet-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[1]}))
	writeCredential := func(path string, a values.RealmID, id, secret []byte) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"audience": a, "credential_id": values.EncodePublicBytes(id), "secret": values.EncodePublicBytes(secret)})
		writeNetworkFile(t, path, b)
	}
	writeCredential(filepath.Join(dir, "core-credential.json"), c.actor.Realm, c.id[:], c.secret[:])
	writeCredential(filepath.Join(c.dir, "facet-credential.json"), actor.Realm, credential.ID[:], credential.Secret[:])
	var realmEndpoint map[string]any
	var startRealm func() *serviceProductProcess
	var realmProcess *serviceProductProcess
	var coreRealmExtra map[string]any
	var startCoreRealm func() *serviceProductProcess
	if networkRealm {
		realmEndpoint, startRealm = prepareFacetNetworkRealm(t, dir, r, f, actor)
		if combined {
			coreRealmExtra, startCoreRealm = prepareCombinationCoreRealm(t, c, dir, realmStore, realmEndpoint, shared)
		}
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	if networkRealm {
		realmProcess = startRealm()
		verifyFacetNetworkMaintenance(t, dir, realmEndpoint, credential)
	}
	coreProductConfig := map[string]any{
		"facet_services":    []any{map[string]any{"realm": c.actor.Realm, "principal": c.actor.Principal, "facet": owner}},
		"facet_connections": []any{map[string]any{"next_hop": hop, "expected_facet": owner, "ca_file": filepath.Join(c.dir, "facet-ca.pem"), "spki_sha256": values.EncodePublicBytes(pin[:]), "credential_file": filepath.Join(c.dir, "facet-credential.json")}},
	}
	for name, value := range coreRealmExtra {
		coreProductConfig[name] = value
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" || os.Getenv("UCUPA_P8_CRASH_GATE") != "" {
		coreProductConfig["session_ttl"] = "10m"
		coreProductConfig["connection_lifetime"] = "10m"
		coreProductConfig["public_max_contexts"] = 1024
		coreProductConfig["public_max_contexts_per_caller"] = 512
	}
	startCore := prepareNetworkProductControl(t, c, coreProductConfig)
	var coreRealmProcess *serviceProductProcess
	if startCoreRealm != nil {
		coreRealmProcess = startCoreRealm()
	}
	cp := c.clientConfig.SPKISHA256
	configuration := map[string]any{"listen": address, "next_hop": hop, "expected_facet": owner, "realm_id": actor.Realm, "realm_next_hop": "local:facet-relation", "facet_store": filepath.Join(dir, "facet.db"), "realm_store": filepath.Join(dir, "realm.db"), "certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"), "session_ttl": "30s", "rpc_deadline": "4s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s", "max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8, "max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 4 << 20, "context_ttl": "1m", "observation_ttl": "1m",
		"core_services":    []any{map[string]any{"realm": actor.Realm, "principal": actor.Principal, "core": c.config.Expected}},
		"core_connections": []any{map[string]any{"next_hop": c.config.NextHop, "expected_core": c.config.Expected, "ca_file": filepath.Join(dir, "core-ca.pem"), "spki_sha256": values.EncodePublicBytes(cp[:]), "credential_file": filepath.Join(dir, "core-credential.json")}},
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" || os.Getenv("UCUPA_P8_CRASH_GATE") != "" {
		configuration["session_ttl"] = "10m"
		configuration["connection_lifetime"] = "10m"
		configuration["public_max_contexts"] = 1024
		configuration["public_max_contexts_per_caller"] = 512
	}
	if networkRealm {
		delete(configuration, "realm_store")
		delete(configuration, "realm_next_hop")
		configuration["realm_profile"] = "network"
		configuration["realm_endpoints"] = []any{realmEndpoint}
	}
	startFacet := prepareServiceProductControl(t, dir, "ucupa-facet", configuration)
	if combined {
		verifyCombinationStartup(t, dir, configuration, realmEndpoint)
	}
	coreProcess, facetProcess := startCore(), startFacet()
	stopCore, stopFacet := coreProcess.stop, facetProcess.stop
	c.connect(t)
	// Root provisioning above is an owner operation; exercise the public
	// Core write/read path in the same two-product cell as the Facet flow.
	const cellEntity = "00000000-0000-4000-8000-0000000000c8"
	created, err := c.client.Core().CreateEntity(ctx, &corepb.CreateEntityRequest{
		Handle: c.enter(t, corepb.Intent_CREATE_ENTITY), Element: proto.String(cellEntity),
		OperationId: []byte("cell-create-entity"), Final: networkTuple(1),
	})
	if err != nil || created.GetResult() == nil {
		t.Fatalf("cell Core public write: %v %v", created, err)
	}
	entityRead, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{
		Handle:  c.enter(t, corepb.Intent_READ),
		Address: &netpb.Address{Down: &netpb.ElementPath{Items: []string{cellEntity}}},
	})
	if err != nil || entityRead.GetResult().GetRecord() == nil || !proto.Equal(created.GetResult().GetRecord().Revision, entityRead.GetResult().GetRecord().Revision) {
		t.Fatalf("cell Core public read: %v %v", entityRead, err)
	}
	client, err := facetsdk.New(facetsdk.Config{NextHop: hop, Expected: owner, Roots: roots, SPKISHA256: pin, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	login := func() {
		t.Helper()
		if err := client.Establish(ctx, actor.Realm, [16]byte(credential.ID), [32]byte(credential.Secret)); err != nil {
			t.Fatal(err)
		}
	}
	login()
	inspect, err := c.client.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRoot))})
	if err != nil || inspect.GetResult() == nil {
		t.Fatalf("initial: %v %v", inspect, err)
	}
	propose := &corepb.ApplyFacetRequest{OperationId: []byte("relation-propose"), Command: &corepb.PublicFacetCommand{Kind: proto.String("propose"), Element: proto.String(string(networkRoot)), Expected: inspect.GetResult().Revision, Target: proto.String(string(owner)), NextHop: proto.String(string(hop))}}
	p, err := c.client.FacetControl().ApplyFacet(ctx, propose)
	if err != nil || p.GetResult() == nil {
		t.Fatalf("proposal: %v %v", p, err)
	}
	ref := &facetpb.PublicAcceptanceRef{Core: proto.String(string(c.config.Expected)), Element: proto.String(string(networkRoot)), Proposal: p.GetResult().GetState().PendingProposal}
	accept := &facetpb.AcceptAttachmentRequest{Ref: ref, CoreNextHop: proto.String(string(c.config.NextHop)), OperationId: []byte("relation-accept")}
	a, err := client.Relationships().AcceptAttachment(ctx, accept)
	if err != nil || a.GetResult() == nil {
		t.Fatalf("accept: %v %v", a, err)
	}
	activate := &corepb.ApplyFacetRequest{OperationId: []byte("relation-activate"), Command: &corepb.PublicFacetCommand{Kind: proto.String("activate"), Element: proto.String(string(networkRoot)), Expected: p.GetResult().GetState().Revision}}
	active, err := c.client.FacetControl().ApplyFacet(ctx, activate)
	if err != nil || active.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("activate: %v %v", active, err)
	}
	access := &facetpb.Access{NextHop: proto.String(string(hop)), Key: &netpb.AttachmentKey{Core: ref.Core, Facet: proto.String(string(owner)), Element: ref.Element, Proposal: ref.Proposal}}
	put, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("relation-content"), Payload: []byte(`"network"`)})
	if err != nil || put.GetResult() == nil {
		t.Fatalf("content write: %v %v", put, err)
	}
	if combined {
		verifyCombinationPermissions(t, c, realmStore, actor, owner, client, access, typ, put, startRealm, &realmProcess, startCoreRealm, &coreRealmProcess, shared, func() {
			coreProcess.gracefulStop()
			facetProcess.gracefulStop()
			coreProcess, facetProcess = startCore(), startFacet()
			stopCore, stopFacet = coreProcess.stop, facetProcess.stop
			reconnectCombinationCore(t, c)
			login()
		})
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" {
		measureP8FixedCell(t, c, client, access, typ, dir)
	}
	// Contact repair is owned locally. A broken opposite service must not
	// prevent repairing its address or force a fresh proposal/acceptance.
	stopCore()
	contact := a.GetResult().GetInspection().ContactRevision
	for i, address := range []string{"https://127.0.0.1:1", string(c.config.NextHop)} {
		request := &facetpb.UpdateCoreContactRequest{Ref: ref, CoreNextHop: proto.String(address), OperationId: []byte(fmt.Sprintf("repair-core-%d", i)), Expected: contact}
		updated, err := client.Relationships().UpdateCoreContact(ctx, request)
		if err != nil || updated.GetResult() == nil || updated.GetResult().GetInspection().GetCoreNextHop() != address {
			t.Fatalf("repair unavailable Core: %v %v", updated, err)
		}
		retry, err := client.Relationships().UpdateCoreContact(ctx, request)
		if err != nil || !proto.Equal(updated, retry) {
			t.Fatalf("repair Core retry: %v %v", retry, err)
		}
		contact = updated.GetResult().GetInspection().ContactRevision
	}
	coreProcess = startCore()
	stopCore = coreProcess.stop
	c.client.Close()
	c.connect(t)
	stopFacet()
	inspection, err := c.client.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: ref.Element})
	if err != nil || inspection.GetResult() == nil {
		t.Fatalf("inspect before repair: %v %v", inspection, err)
	}
	contact = inspection.GetResult().ContactRevision
	for i, address := range []string{"https://127.0.0.1:1", string(hop)} {
		request := &corepb.ApplyFacetRequest{OperationId: []byte(fmt.Sprintf("repair-facet-%d", i)), Command: &corepb.PublicFacetCommand{Kind: proto.String("contact"), Element: ref.Element, Expected: contact, NextHop: proto.String(address)}}
		updated, err := c.client.FacetControl().ApplyFacet(ctx, request)
		if err != nil || updated.GetResult() == nil || updated.GetResult().GetState().GetNextHop() != address {
			t.Fatalf("repair unavailable Facet: %v %v", updated, err)
		}
		retry, err := c.client.FacetControl().ApplyFacet(ctx, request)
		if err != nil || !proto.Equal(updated, retry) {
			t.Fatalf("repair Facet retry: %v %v", retry, err)
		}
		contact = updated.GetResult().GetState().ContactRevision
	}
	facetProcess = startFacet()
	stopFacet = facetProcess.stop
	login()
	if current, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()}); err != nil || current.GetResult() == nil {
		t.Fatalf("repaired relationship before orderly stop: %v %v", current, err)
	}
	// Every durable owner is reopened; retries must not require the original
	// transient service sessions or reaccept a newer proposal.
	coreProcess.gracefulStop()
	facetProcess.gracefulStop()
	archive := t.TempDir()
	for name, original := range map[string]string{
		"core.db": filepath.Join(c.dir, "core.db"), "core-realm.db": filepath.Join(c.dir, "realm.db"),
		"facet.db": filepath.Join(dir, "facet.db"), "facet-realm.db": realmStore,
	} {
		contents, err := os.ReadFile(original)
		if err != nil {
			t.Fatalf("read stopped Store %s: %v", name, err)
		}
		copied := filepath.Join(archive, name)
		writeNetworkFile(t, copied, contents)
		archived, err := os.ReadFile(copied)
		if err != nil || sha256.Sum256(contents) != sha256.Sum256(archived) {
			t.Fatalf("cold archive mismatch %s: %v", name, err)
		}
	}
	for _, malformed := range []bool{false, true} {
		rejectUnopenableProductStore(t, c.dir, "ucupa-core", "core_store", "cannot open Core Store", malformed)
		rejectUnopenableProductStore(t, dir, "ucupa-facet", "facet_store", "cannot open Facet Store", malformed)
	}
	reopenOriginals := func(kind string, iteration int) {
		t.Helper()
		coreStarted := time.Now()
		coreProcess = startCore()
		stopCore = coreProcess.stop
		c.client.Close()
		c.connect(t)
		if os.Getenv("UCUPA_P8_MEASURE") == "1" {
			t.Logf("P8 %s recovery %d Core process start to Establish=%s", kind, iteration, time.Since(coreStarted))
		}
		facetStarted := time.Now()
		facetProcess = startFacet()
		stopFacet = facetProcess.stop
		login()
		if os.Getenv("UCUPA_P8_MEASURE") == "1" {
			t.Logf("P8 %s recovery %d Facet process start to Establish=%s", kind, iteration, time.Since(facetStarted))
		}
	}
	reopenOriginals("orderly-stop", 1)
	defer func() {
		stopCore()
		stopFacet()
	}()
	if os.Getenv("UCUPA_P8_MEASURE") == "1" {
		for iteration := 2; iteration <= 3; iteration++ {
			coreProcess.gracefulStop()
			facetProcess.gracefulStop()
			reopenOriginals("orderly-stop", iteration)
		}
		for iteration := 1; iteration <= 3; iteration++ {
			stopCore()
			stopFacet()
			reopenOriginals("forced-stop", iteration)
		}
	}
	recoveryStarted := time.Now()
	recoveredEntity, err := c.client.Core().RecoverOutcome(ctx, &corepb.RecoverOutcomeRequest{Reference: c.reference(), OperationId: []byte("cell-create-entity"),
		Request: &corepb.MutationCommand{Command: &corepb.MutationCommand_CreateEntity{CreateEntity: &corepb.CreateEntityCommand{
			Element: proto.String(cellEntity), OperationId: []byte("cell-create-entity"), Final: networkTuple(1),
		}}}})
	if err != nil || !proto.Equal(created.GetResult(), recoveredEntity.GetResult()) {
		t.Fatalf("Core original receipt after restart: %v %v", recoveredEntity, err)
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" {
		t.Logf("P8 Core original OperationID recovery RTT=%s", time.Since(recoveryStarted))
	}
	recoveryStarted = time.Now()
	recoveredPayload, err := client.Facet().RecoverOutcome(ctx, &facetpb.RecoverOutcomeRequest{Access: access, OperationId: []byte("relation-content"),
		Request: &facetpb.MutationCommand{Command: &facetpb.MutationCommand_Put{Put: &facetpb.PutRequest{
			Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}},
			OperationId: []byte("relation-content"), Payload: []byte(`"network"`),
		}}}})
	if err != nil || !proto.Equal(put.GetResult(), recoveredPayload.GetResult()) {
		t.Fatalf("Facet original receipt after restart: %v %v", recoveredPayload, err)
	}
	if os.Getenv("UCUPA_P8_MEASURE") == "1" {
		t.Logf("P8 Facet original OperationID recovery RTT=%s", time.Since(recoveryStarted))
	}
	replayed, err := client.Relationships().AcceptAttachment(ctx, accept)
	if err != nil || !proto.Equal(replayed, a) {
		t.Fatalf("acceptance reopen: %v %v", replayed, err)
	}
	current, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || current.GetResult() == nil || string(current.GetResult().Payload) != `"network"` {
		t.Fatalf("content after repaired contacts and reopen: %v %v", current, err)
	}
	if os.Getenv("UCUPA_P8_DISK_FULL") == "1" {
		verifyP8IsolatedDiskFull(t, dir, c.dir, client, access, typ.Bytes(), current.GetResult().Revision, func() {
			stopFacet()
			facetProcess = startFacet()
			stopFacet = facetProcess.stop
			login()
		})
	}
	if os.Getenv("UCUPA_P8_CRASH_GATE") != "" {
		if os.Getenv("UCUPA_P8_DISK_FULL") == "1" {
			t.Fatal("run one P8 fault mode at a time")
		}
		verifyP8InFlightCrash(t, facetProcess.cmd.Process.Pid, client, access, typ.Bytes(), current.GetResult().Revision, func() {
			stopFacet()
			facetProcess = startFacet()
			stopFacet = facetProcess.stop
			login()
		})
	}
	if networkRealm {
		// More completed calls than the unchanged per-caller Realm budget prove
		// that finished authorization views are actually released, not TTL-only.
		for i := 0; i < 80; i++ {
			inspected, err := client.Relationships().InspectAcceptance(ctx, &facetpb.InspectAcceptanceRequest{Ref: ref})
			if err != nil || inspected.GetResult() == nil {
				t.Fatalf("network Realm view cleanup %d: %v %v", i, inspected, err)
			}
		}
		realmProcess.stop()
		unavailable, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
		if status.Code(err) != codes.Unavailable && (err != nil || unavailable.GetFailure() != netpb.Failure_UNAVAILABLE) {
			t.Fatalf("offline Realm must not serve cached Allow: %v %v", unavailable, err)
		}
		realmProcess = startRealm()
		oldSession, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
		if err == nil && oldSession.GetFailure() != netpb.Failure_UNAVAILABLE {
			t.Fatalf("Realm restart reused an old runtime caller: %v", oldSession)
		}
		// Existing pinned gRPC connections may still be in transport backoff.
		// Each attempt is a new explicit login, never reuse of the old caller.
		deadline := time.Now().Add(10 * time.Second)
		for {
			err := client.Establish(ctx, actor.Realm, [16]byte(credential.ID), [32]byte(credential.Secret))
			if err == nil {
				break
			}
			if status.Code(err) != codes.Unavailable || !time.Now().Before(deadline) {
				t.Fatalf("new session after Realm restart: %v", err)
			}
			time.Sleep(25 * time.Millisecond)
		}
		if shared {
			// Both products obtained identities from the restarted run. Neither
			// the user nor the reverse-service session can continue that run.
			if stale, err := c.client.Core().Enter(ctx, &corepb.EnterRequest{Reference: c.reference(), Intent: corepb.Intent_READ.Enum()}); err == nil && stale.GetResult() != nil {
				t.Fatal("shared Realm restart revived Core caller")
			}
			coreProcess.gracefulStop()
			coreProcess = startCore()
			stopCore = coreProcess.stop
			reconnectCombinationCore(t, c)
			facetProcess.gracefulStop()
			facetProcess = startFacet()
			stopFacet = facetProcess.stop
			login()
		}
		fresh, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
		if err != nil || fresh.GetResult() == nil {
			t.Fatalf("fresh Realm session lost original content: %v %v", fresh, err)
		}
	}
	d, err := c.client.FacetControl().ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("relation-detach"), Command: &corepb.PublicFacetCommand{Kind: proto.String("detach"), Element: ref.Element, Expected: active.GetResult().GetState().Revision}})
	if err != nil || d.GetResult() == nil {
		t.Fatalf("detach: %v %v", d, err)
	}
	get, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || get.GetFailure() != netpb.Failure_UNAVAILABLE {
		t.Fatalf("detached content must be explicitly unavailable: %v %v", get, err)
	}
	// Realm administration is a cold owner operation. The product must reopen
	// the original Store and refuse the credential revoked while it was stopped.
	stopCore()
	stopFacet()
	if networkRealm {
		realmProcess.stop()
	}
	if coreRealmProcess != nil {
		coreRealmProcess.stop()
	}
	policy, err := realminstance.OpenDurable(realmStore)
	if err != nil {
		t.Fatal(err)
	}
	_, err = policy.RevokeCredential(admin, credential.ID, policy.CredentialSetRevision(), "cell-cold-revoke")
	if err != nil {
		t.Fatal(err)
	}
	if err = policy.Close(); err != nil {
		t.Fatal(err)
	}
	if networkRealm {
		realmProcess = startRealm()
	}
	if startCoreRealm != nil {
		coreRealmProcess = startCoreRealm()
	}
	coreProcess, facetProcess = startCore(), startFacet()
	stopCore, stopFacet = coreProcess.stop, facetProcess.stop
	if err = client.Establish(ctx, actor.Realm, [16]byte(credential.ID), [32]byte(credential.Secret)); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("cold-revoked credential established: %v", err)
	}
}

func rejectUnopenableProductStore(t *testing.T, dir, program, field, expected string, malformed bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "service.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(dir, "missing-original.db")
	if malformed {
		store = filepath.Join(dir, "malformed-original.db")
		writeNetworkFile(t, store, []byte("not a UCUPA Store"))
	}
	config[field] = store
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unopenable-store.json")
	writeNetworkFile(t, path, raw)
	exe := filepath.Join(dir, program)
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "-config", path).CombinedOutput()
	if err == nil || ctx.Err() != nil || !strings.Contains(string(out), expected) {
		t.Fatalf("missing original Store started %s: %v %s", program, err, out)
	}
	if malformed {
		contents, err := os.ReadFile(store)
		if err != nil || string(contents) != "not a UCUPA Store" {
			t.Fatalf("malformed Store changed %s: %v", program, err)
		}
	} else if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatalf("missing original Store was created %s: %v", program, err)
	}
}
