package acceptance_test

import (
	"bytes"
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

const replacementProductEntity values.ElementID = "00000000-0000-4000-8000-00000000fa9e"
const replacementProductEmptyElement values.ElementID = "00000000-0000-4000-8000-00000000fa9f"

// TestFacetReplacementIndependentProducts moves one element's namespace from
// a source ucupa-facet product to a target ucupa-facet product under one real
// ucupa-core product, three independent processes. Both relationships are
// built over the public planes, two entries are written through the source
// product, and the authorized replacement settles through the Core product's
// public control plane while every peer step crosses the process network: the
// target serves both moved entries byte for byte, the fenced source refuses
// its former public access, and the settled recovery replays with its exact
// original receipt. A second reservation on a never-written namespace
// survives a full restart of all three products and is recovered afterwards,
// leaving an installed namespace that reads absence with zero types while
// both fenced sources stay sealed.
func TestFacetReplacementIndependentProducts(t *testing.T) {
	runFacetReplacementProducts(t, false)
}

func runFacetReplacementProducts(t *testing.T, networkRealm bool) {
	ctx := context.Background()
	c := newNetworkFixture(t, false)
	dir, sourceDir, targetDir := t.TempDir(), t.TempDir(), t.TempDir()
	entity, emptyEntity := replacementProductEntity, replacementProductEmptyElement
	coreOwner := c.c.AuthorityRef()
	typ, notes := fixtureType("replacement-product-profile"), fixtureType("replacement-product-notes")
	emptyType := fixtureType("replacement-product-empty")
	content := []struct {
		typ     values.FacetTypeRef
		payload []byte
	}{{typ, fixturePayload("replacement-product-profile")}, {notes, fixturePayload("replacement-product-notes")}}

	// Each independent Facet realm holds the operator the test drives plus one
	// fixed service principal for the Core product's inbound calls.
	provision := func(name, storeDir string) (values.PrincipalRef, values.PrincipalRef, realm.CredentialEvidence, realm.CredentialEvidence, *realminstance.Realm) {
		operator := values.PrincipalRef{Realm: values.RealmID("facet-replacement-" + name), Principal: "operator"}
		service := values.PrincipalRef{Realm: operator.Realm, Principal: values.PrincipalID("core-service")}
		operatorCredential := realm.CredentialEvidence{Audience: operator.Realm}
		rand.Read(operatorCredential.ID[:])
		rand.Read(operatorCredential.Secret[:])
		serviceCredential := realm.CredentialEvidence{Audience: operator.Realm}
		rand.Read(serviceCredential.ID[:])
		rand.Read(serviceCredential.Secret[:])
		r, err := realminstance.CreateDurable(realm.Seed{
			ID:              operator.Realm,
			LocalPrincipals: []values.PrincipalID{operator.Principal, service.Principal},
			Credentials: []realm.CredentialBootstrap{
				{Subject: operator.Principal, Evidence: operatorCredential},
				{Subject: service.Principal, Evidence: serviceCredential},
			},
			Bindings: []realm.PermissionBinding{{Subject: operator, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(operator.Realm)}},
		}, filepath.Join(storeDir, "realm.db"))
		if err != nil {
			t.Fatal(err)
		}
		return operator, service, operatorCredential, serviceCredential, r
	}
	sourceOperator, sourceService, sourceOperatorCredential, sourceServiceCredential, sourceRealm := provision("source", sourceDir)
	targetOperator, targetService, targetOperatorCredential, targetServiceCredential, targetRealm := provision("target", targetDir)

	// This provisioning adapter stays unused: both relationships below are
	// built entirely over the public RPC planes once the three products run.
	links := &composition.LocalFacetCoreConnections{}
	if err := links.BindEndpoint("local:bootstrap", composition.LocalFacetCoreEndpoint{Read: c.c.Facets(), Control: c.c.Facets(), Access: c.c.Access()}); err != nil {
		t.Fatal(err)
	}
	caller, err := c.r.EstablishCaller(realm.CredentialEvidence{Audience: c.actor.Realm, ID: realm.CredentialID(c.id), Secret: realm.CredentialSecret(c.secret)})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := links.ForFacetCaller(caller)
	if err != nil {
		t.Fatal(err)
	}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint(values.NextHop("local:"+string(sourceOperator.Realm)), sourceRealm); err != nil {
		t.Fatal(err)
	}
	if err = realmLinks.BindEndpoint(values.NextHop("local:"+string(targetOperator.Realm)), targetRealm); err != nil {
		t.Fatal(err)
	}
	sourceFacet, err := facet.CreateDurable(realmLinks, bootstrap, facetcontract.RealmInitialization{Realm: sourceOperator.Realm, NextHop: values.NextHop("local:" + string(sourceOperator.Realm))}, filepath.Join(sourceDir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	targetFacet, err := facet.CreateDurable(realmLinks, bootstrap, facetcontract.RealmInitialization{Realm: targetOperator.Realm, NextHop: values.NextHop("local:" + string(targetOperator.Realm))}, filepath.Join(targetDir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	sourceOwner, targetOwner := sourceFacet.AuthorityRef(), targetFacet.AuthorityRef()

	grant := func(r *realminstance.Realm, operator, subject values.PrincipalRef, permission values.PermissionID, scope values.OperationScope, name string) {
		t.Helper()
		if _, err := r.PutBinding(operator, realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}, r.BindingSetRevision(), realm.OperationID(name)); err != nil {
			t.Fatal(err)
		}
	}
	for i, e := range []values.ElementID{entity, emptyEntity} {
		grant(sourceRealm, sourceOperator, sourceOperator, facetcontract.PermissionAccept, values.FacetElementScope(sourceOwner, e), fmt.Sprintf("replacement-product-accept-%d", i))
		grant(sourceRealm, sourceOperator, sourceService, facetcontract.PermissionNamespaceExport, values.FacetElementScope(sourceOwner, e), fmt.Sprintf("replacement-product-export-%d", i))
		grant(sourceRealm, sourceOperator, sourceService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(sourceOwner, e), fmt.Sprintf("replacement-product-acceptance-read-%d", i))
		grant(targetRealm, targetOperator, targetService, facetcontract.PermissionNamespaceImport, values.FacetElementScope(targetOwner, e), fmt.Sprintf("replacement-product-import-%d", i))
	}
	for i, entry := range content {
		grant(sourceRealm, sourceOperator, sourceOperator, facetcontract.PermissionPut, values.FacetKeyScope(sourceOwner, entity, entry.typ), fmt.Sprintf("replacement-product-put-%d", i))
		grant(sourceRealm, sourceOperator, sourceOperator, facetcontract.PermissionGet, values.FacetKeyScope(sourceOwner, entity, entry.typ), fmt.Sprintf("replacement-product-source-read-%d", i))
		grant(targetRealm, targetOperator, targetOperator, facetcontract.PermissionGet, values.FacetKeyScope(targetOwner, entity, entry.typ), fmt.Sprintf("replacement-product-target-read-%d", i))
	}
	grant(sourceRealm, sourceOperator, sourceOperator, facetcontract.PermissionGet, values.FacetKeyScope(sourceOwner, emptyEntity, emptyType), "replacement-product-source-read-empty")
	grant(targetRealm, targetOperator, targetOperator, facetcontract.PermissionGet, values.FacetKeyScope(targetOwner, emptyEntity, emptyType), "replacement-product-target-read-empty")
	grant(targetRealm, targetOperator, targetOperator, facetcontract.PermissionListTypes, values.FacetElementScope(targetOwner, emptyEntity), "replacement-product-list-empty")
	c.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(coreOwner, networkRoot), false)
	for _, e := range []values.ElementID{entity, emptyEntity} {
		for _, p := range []values.PermissionID{corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionFacetReplacementBegin, corecontract.PermissionFacetReplacementRecover, corecontract.PermissionFacetReplacementRead} {
			c.grant(t, p, values.CoreElementScope(coreOwner, e), false)
		}
	}
	// The target Facet product's presenting principal exercises only the
	// key-bound attachment read of its own install step.
	for i, e := range []values.ElementID{entity, emptyEntity} {
		if _, err := c.r.PutBinding(c.actor, realm.PermissionBinding{Subject: c.facetService, Permission: corecontract.PermissionFacetAttachmentRead, Scope: values.CoreElementScope(coreOwner, e)}, c.r.BindingSetRevision(), realm.OperationID(fmt.Sprintf("replacement-product-facet-service-read-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	// Both moving elements are children of the fixture root, created through
	// the real owner API before any product process exists.
	if _, err := c.c.Elements().CreateEntity(c.actor, entity, corecontract.SpatialTuple{
		Parent: networkRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "replacement-product-create"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.c.Elements().CreateEntity(c.actor, emptyEntity, corecontract.SpatialTuple{
		Parent: networkRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 6}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "replacement-product-empty-create"); err != nil {
		t.Fatal(err)
	}
	var sourceRealmEndpoint, targetRealmEndpoint map[string]any
	var startSourceRealm, startTargetRealm func() *serviceProductProcess
	if networkRealm {
		sourceRealmEndpoint, startSourceRealm = prepareFacetNetworkRealm(t, sourceDir, sourceRealm, sourceFacet, sourceOperator)
		targetRealmEndpoint, startTargetRealm = prepareFacetNetworkRealm(t, targetDir, targetRealm, targetFacet, targetOperator)
	}
	if err := sourceFacet.Close(); err != nil {
		t.Fatal(err)
	}
	if err := targetFacet.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sourceRealm.Close(); err != nil {
		t.Fatal(err)
	}
	if err := targetRealm.Close(); err != nil {
		t.Fatal(err)
	}
	if networkRealm {
		startSourceRealm()
		startTargetRealm()
	}
	certificate, roots, pin, certPEM, keyPEM := networkCertificate(t)
	writeNetworkFile(t, filepath.Join(dir, "tls.pem"), certPEM)
	writeNetworkFile(t, filepath.Join(dir, "tls.key"), keyPEM)
	writeNetworkFile(t, filepath.Join(dir, "facet-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[1]}))
	writeNetworkFile(t, filepath.Join(dir, "core-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.config.Certificate.Certificate[1]}))
	writeCredential := func(path string, audience values.RealmID, id, secret []byte) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"audience": audience, "credential_id": values.EncodePublicBytes(id), "secret": values.EncodePublicBytes(secret)})
		writeNetworkFile(t, path, b)
	}
	// The source Facet product presents the Core realm's operator credential;
	// the target Facet product presents the Core realm's inert facet-service
	// principal. The Core product maps each presenting principal to its own
	// Facet: the source's acceptance resolutions and the target's key-bound
	// install read each name their own relationship party.
	writeCredential(filepath.Join(dir, "source-core-credential.json"), c.actor.Realm, c.id[:], c.secret[:])
	writeCredential(filepath.Join(dir, "target-core-credential.json"), c.actor.Realm, c.facetServiceID[:], c.facetServiceSecret[:])
	writeCredential(filepath.Join(dir, "source-service-credential.json"), sourceOperator.Realm, sourceServiceCredential.ID[:], sourceServiceCredential.Secret[:])
	writeCredential(filepath.Join(dir, "target-service-credential.json"), targetOperator.Realm, targetServiceCredential.ID[:], targetServiceCredential.Secret[:])

	pickAddress := func() (string, values.NextHop) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := l.Addr().String()
		l.Close()
		return address, values.NextHop("https://" + address)
	}
	sourceAddress, sourceHop := pickAddress()
	targetAddress, targetHop := pickAddress()
	corePin := c.clientConfig.SPKISHA256
	startCore := prepareNetworkProduct(t, c, map[string]any{
		// Each Facet product's presenting principal maps to its own Facet: the
		// acceptance resolutions belong to the source, the key-bound install
		// read names the target, and the replacement record read admits
		// either record party.
		"facet_services": []any{
			map[string]any{"realm": c.actor.Realm, "principal": c.actor.Principal, "facet": sourceOwner},
			map[string]any{"realm": c.actor.Realm, "principal": c.facetService.Principal, "facet": targetOwner},
		},
		"facet_connections": []any{
			map[string]any{"next_hop": sourceHop, "expected_facet": sourceOwner, "ca_file": filepath.Join(dir, "facet-ca.pem"), "spki_sha256": values.EncodePublicBytes(pin[:]), "credential_file": filepath.Join(dir, "source-service-credential.json")},
			map[string]any{"next_hop": targetHop, "expected_facet": targetOwner, "ca_file": filepath.Join(dir, "facet-ca.pem"), "spki_sha256": values.EncodePublicBytes(pin[:]), "credential_file": filepath.Join(dir, "target-service-credential.json")},
		},
	})
	facetConfiguration := func(owner values.FactAuthorityRef, operator, service values.PrincipalRef, address string, hop values.NextHop, storeDir, credentialFile string) map[string]any {
		configuration := map[string]any{
			"listen": address, "next_hop": hop, "expected_facet": owner, "realm_id": operator.Realm, "realm_next_hop": values.NextHop("local:" + string(operator.Realm)),
			"facet_store": filepath.Join(storeDir, "facet.db"), "realm_store": filepath.Join(storeDir, "realm.db"),
			"certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"),
			"session_ttl": "30s", "rpc_deadline": "4s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
			"max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8, "max_streams": 8,
			"max_metadata_bytes": 8192, "max_response_bytes": 4 << 20, "context_ttl": "1m", "observation_ttl": "1m",
			"core_services":    []any{map[string]any{"realm": operator.Realm, "principal": service.Principal, "core": c.config.Expected}},
			"core_connections": []any{map[string]any{"next_hop": c.config.NextHop, "expected_core": c.config.Expected, "ca_file": filepath.Join(dir, "core-ca.pem"), "spki_sha256": values.EncodePublicBytes(corePin[:]), "credential_file": credentialFile}},
		}
		if networkRealm {
			endpoint := sourceRealmEndpoint
			if owner == targetOwner {
				endpoint = targetRealmEndpoint
			}
			delete(configuration, "realm_store")
			delete(configuration, "realm_next_hop")
			configuration["realm_profile"] = "network"
			configuration["realm_endpoints"] = []any{endpoint}
		}
		return configuration
	}
	startSourceFacet := prepareServiceProduct(t, sourceDir, "ucupa-facet", facetConfiguration(sourceOwner, sourceOperator, sourceService, sourceAddress, sourceHop, sourceDir, filepath.Join(dir, "source-core-credential.json")))
	startTargetFacet := prepareServiceProduct(t, targetDir, "ucupa-facet", facetConfiguration(targetOwner, targetOperator, targetService, targetAddress, targetHop, targetDir, filepath.Join(dir, "target-core-credential.json")))
	stopCore, stopSourceFacet, stopTargetFacet := startCore(), startSourceFacet(), startTargetFacet()
	defer func() { stopCore(); stopSourceFacet(); stopTargetFacet() }()
	c.connect(t)
	newFacetClient := func(expected values.FactAuthorityRef, hop values.NextHop) *facetsdk.Client {
		client, err := facetsdk.New(facetsdk.Config{NextHop: hop, Expected: expected, Roots: roots, SPKISHA256: pin, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	sourceClient, targetClient := newFacetClient(sourceOwner, sourceHop), newFacetClient(targetOwner, targetHop)
	defer sourceClient.Close()
	defer targetClient.Close()
	login := func(client *facetsdk.Client, credential realm.CredentialEvidence) {
		t.Helper()
		if err := client.Establish(ctx, credential.Audience, [16]byte(credential.ID), [32]byte(credential.Secret)); err != nil {
			t.Fatal(err)
		}
	}
	login(sourceClient, sourceOperatorCredential)
	login(targetClient, targetOperatorCredential)

	// Both relationships are proposed, accepted and activated over the public
	// planes of the Core product and the source Facet product; the live
	// namespace holds two entries and the second one is never written.
	control := c.client.FacetControl()
	relationships := sourceClient.Relationships()
	attach := func(e values.ElementID, name string) *facetpb.Access {
		t.Helper()
		inspected, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(e))})
		if err != nil || inspected.GetResult() == nil {
			t.Fatalf("%s inspect: %v %v", name, inspected, err)
		}
		proposed, err := control.ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("replacement-product-" + name + "-propose"), Command: &corepb.PublicFacetCommand{
			Kind: proto.String("propose"), Element: proto.String(string(e)), Expected: inspected.GetResult().GetRevision(),
			Target: proto.String(string(sourceOwner)), NextHop: proto.String(string(sourceHop)),
		}})
		if err != nil || proposed.GetResult() == nil || proposed.GetResult().GetState().GetPendingProposal() == nil {
			t.Fatalf("%s propose: %v %v", name, proposed, err)
		}
		ref := &facetpb.PublicAcceptanceRef{Core: proto.String(string(c.config.Expected)), Element: proto.String(string(e)), Proposal: proposed.GetResult().GetState().GetPendingProposal()}
		accepted, err := relationships.AcceptAttachment(ctx, &facetpb.AcceptAttachmentRequest{Ref: ref, CoreNextHop: proto.String(string(c.config.NextHop)), OperationId: []byte("replacement-product-" + name + "-accept")})
		if err != nil || accepted.GetResult() == nil || accepted.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
			!accepted.GetResult().GetInspection().GetFound() || accepted.GetResult().GetInspection().GetCoreNextHop() != string(c.config.NextHop) {
			t.Fatalf("%s accept: %v %v", name, accepted, err)
		}
		activated, err := control.ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("replacement-product-" + name + "-activate"), Command: &corepb.PublicFacetCommand{
			Kind: proto.String("activate"), Element: proto.String(string(e)), Expected: proposed.GetResult().GetState().GetRevision(),
		}})
		if err != nil || activated.GetResult() == nil || activated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
			t.Fatalf("%s activate: %v %v", name, activated, err)
		}
		return &facetpb.Access{NextHop: proto.String(string(sourceHop)), Key: &networkpb.AttachmentKey{
			Core: proto.String(string(c.config.Expected)), Facet: proto.String(string(sourceOwner)), Element: proto.String(string(e)), Proposal: ref.Proposal,
		}}
	}
	sourceAccess := attach(entity, "attach")
	emptySourceAccess := attach(emptyEntity, "attach-empty")
	for i, entry := range content {
		put, err := sourceClient.Facet().Put(ctx, &facetpb.PutRequest{Access: sourceAccess, TypeRef: entry.typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte(fmt.Sprintf("replacement-product-put-%d", i)), Payload: entry.payload})
		if err != nil || put.GetResult() == nil || put.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED {
			t.Fatalf("source put %d: %v %v", i, put, err)
		}
	}

	// The authorized replacement settles through the Core product's public
	// control plane while every peer step crosses the process network; the
	// exact recovery replays and the inspection discloses the settled record.
	inspected, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(entity))})
	if err != nil || inspected.GetResult() == nil || inspected.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		inspected.GetResult().GetTarget() != string(sourceOwner) || inspected.GetResult().GetNextHop() != string(sourceHop) {
		t.Fatalf("inspect before replacement: %v %v", inspected, err)
	}
	const op corecontract.OperationID = "replacement-product-move"
	begin, err := control.BeginFacetReplacement(ctx, &corepb.BeginFacetReplacementRequest{OperationId: []byte(op), Command: &corepb.PublicFacetReplacementCommand{
		Element: proto.String(string(entity)), Expected: inspected.GetResult().GetRevision(),
		Target: proto.String(string(targetOwner)), TargetAddress: proto.String(string(targetHop)), SourceAddress: proto.String(string(c.config.NextHop)),
	}})
	if err != nil || begin.GetResult() == nil {
		t.Fatalf("begin replacement: %v %v", begin, err)
	}
	if b := begin.GetResult(); b.GetProgress() != uint32(corecontract.FacetReplacementReserved) || b.GetRefused() ||
		b.GetSource() != string(sourceOwner) || b.GetTarget() != string(targetOwner) ||
		b.GetRevision().GetKind() != networkpb.TokenKind_FACET_REPLACEMENT_REVISION || b.GetRevision().GetOwner() != string(coreOwner) || len(b.GetRevision().GetValue()) != 32 {
		t.Fatalf("begin status = %#v", b)
	}
	controlRequest := &corepb.ReplacementControlRequest{OperationId: []byte(op), Element: proto.String(string(entity))}
	final, err := control.RecoverFacetReplacement(ctx, controlRequest)
	if err != nil || final.GetResult() == nil || final.GetResult().GetProgress() != uint32(corecontract.FacetReplacementSettled) || final.GetResult().GetRefused() ||
		!proto.Equal(final.GetResult().GetRevision(), begin.GetResult().GetRevision()) {
		t.Fatalf("recover replacement: %v %v", final, err)
	}
	if replay, err := control.RecoverFacetReplacement(ctx, controlRequest); err != nil || !proto.Equal(replay.GetResult(), final.GetResult()) {
		t.Fatalf("replayed recovery = %#v, err=%v", replay.GetResult(), err)
	}
	if status, err := control.InspectFacetReplacement(ctx, controlRequest); err != nil || !proto.Equal(status.GetResult(), final.GetResult()) {
		t.Fatalf("inspected status = %#v, err=%v", status.GetResult(), err)
	}

	// The switched relationship discloses the target's own acceptance, and the
	// target product serves both moved entries byte for byte while the fenced
	// source refuses its former public access.
	switched, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(entity))})
	if err != nil || switched.GetResult() == nil || switched.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		switched.GetResult().GetTarget() != string(targetOwner) || switched.GetResult().GetNextHop() != string(targetHop) {
		t.Fatalf("switched attachment: %v %v", switched, err)
	}
	targetAccess := facetAccessFromInspection(t, coreOwner, switched.GetResult())
	for _, entry := range content {
		got, err := targetClient.Facet().Get(ctx, &facetpb.GetRequest{Access: targetAccess, TypeRef: entry.typ.Bytes()})
		if err != nil || got.GetResult() == nil || got.GetResult().GetState() != facetpb.Presence_PRESENT || !bytes.Equal(got.GetResult().GetPayload(), entry.payload) {
			t.Fatalf("target content: %v %v", got, err)
		}
	}
	if refused, err := sourceClient.Facet().Get(ctx, &facetpb.GetRequest{Access: sourceAccess, TypeRef: typ.Bytes()}); refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("fenced source served content: %v %v", refused, err)
	}

	// A second reservation on the never-written namespace is persisted before
	// the restart and left for recovery afterwards.
	emptyInspection, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(emptyEntity))})
	if err != nil || emptyInspection.GetResult() == nil || emptyInspection.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		emptyInspection.GetResult().GetTarget() != string(sourceOwner) || emptyInspection.GetResult().GetNextHop() != string(sourceHop) {
		t.Fatalf("inspect empty source attachment: %v %v", emptyInspection, err)
	}
	const emptyOp corecontract.OperationID = "replacement-product-empty-recover-after-restart"
	emptyBegin, err := control.BeginFacetReplacement(ctx, &corepb.BeginFacetReplacementRequest{OperationId: []byte(emptyOp), Command: &corepb.PublicFacetReplacementCommand{
		Element: proto.String(string(emptyEntity)), Expected: emptyInspection.GetResult().GetRevision(),
		Target: proto.String(string(targetOwner)), TargetAddress: proto.String(string(targetHop)), SourceAddress: proto.String(string(c.config.NextHop)),
	}})
	if err != nil || emptyBegin.GetResult() == nil || emptyBegin.GetResult().GetProgress() != uint32(corecontract.FacetReplacementReserved) || emptyBegin.GetResult().GetRefused() {
		t.Fatalf("begin empty namespace replacement: %#v, err=%v", emptyBegin, err)
	}

	// Full restarts of all three products keep the settled record and the
	// pending reservation: the settled recovery replays with its exact original
	// receipt, and the reserved replacement settles afterwards under its own
	// reserved revision while every peer step recrosses the process network.
	stopCore()
	stopSourceFacet()
	stopTargetFacet()
	stopCore, stopSourceFacet, stopTargetFacet = startCore(), startSourceFacet(), startTargetFacet()
	c.client.Close()
	c.connect(t)
	control = c.client.FacetControl()
	login(sourceClient, sourceOperatorCredential)
	login(targetClient, targetOperatorCredential)
	recovered, err := control.RecoverFacetReplacement(ctx, controlRequest)
	if err != nil || !proto.Equal(recovered.GetResult(), final.GetResult()) {
		t.Fatalf("replayed settled recovery after restart: %#v, err=%v", recovered.GetResult(), err)
	}
	if persisted, err := control.InspectFacetReplacement(ctx, controlRequest); err != nil || !proto.Equal(persisted.GetResult(), final.GetResult()) {
		t.Fatalf("persisted status = %#v, err=%v", persisted.GetResult(), err)
	}
	emptyControlRequest := &corepb.ReplacementControlRequest{OperationId: []byte(emptyOp), Element: proto.String(string(emptyEntity))}
	emptyFinal, err := control.RecoverFacetReplacement(ctx, emptyControlRequest)
	if err != nil || emptyFinal.GetResult() == nil || emptyFinal.GetResult().GetProgress() != uint32(corecontract.FacetReplacementSettled) || emptyFinal.GetResult().GetRefused() ||
		!proto.Equal(emptyFinal.GetResult().GetRevision(), emptyBegin.GetResult().GetRevision()) {
		t.Fatalf("recover empty namespace replacement after restart: %#v, err=%v", emptyFinal.GetResult(), err)
	}

	// The restarted products keep both content decisions: the moved entries
	// stay byte-identical on the target, the fenced sources stay sealed, and
	// the installed empty namespace reads absence with zero types.
	for _, entry := range content {
		got, err := targetClient.Facet().Get(ctx, &facetpb.GetRequest{Access: targetAccess, TypeRef: entry.typ.Bytes()})
		if err != nil || got.GetResult() == nil || !bytes.Equal(got.GetResult().GetPayload(), entry.payload) {
			t.Fatalf("reopened target content: %v %v", got, err)
		}
	}
	if refused, err := sourceClient.Facet().Get(ctx, &facetpb.GetRequest{Access: sourceAccess, TypeRef: typ.Bytes()}); refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("reopened fenced source served content: %v %v", refused, err)
	}
	if refused, err := sourceClient.Facet().Get(ctx, &facetpb.GetRequest{Access: emptySourceAccess, TypeRef: emptyType.Bytes()}); refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("reopened fenced empty source served content: %v %v", refused, err)
	}
	emptyTargetInspection, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(emptyEntity))})
	if err != nil || emptyTargetInspection.GetResult() == nil || emptyTargetInspection.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		emptyTargetInspection.GetResult().GetTarget() != string(targetOwner) || emptyTargetInspection.GetResult().GetNextHop() != string(targetHop) {
		t.Fatalf("inspect installed empty target: %v %v", emptyTargetInspection, err)
	}
	emptyTargetAccess := facetAccessFromInspection(t, coreOwner, emptyTargetInspection.GetResult())
	if empty, err := targetClient.Facet().Get(ctx, &facetpb.GetRequest{Access: emptyTargetAccess, TypeRef: emptyType.Bytes()}); err != nil || empty.GetResult() == nil || empty.GetResult().GetState() != facetpb.Presence_NOT_FOUND {
		t.Fatalf("read installed empty target: %v %v", empty, err)
	}
	if listed, err := targetClient.Facet().ListTypes(ctx, &facetpb.ListTypesRequest{Access: emptyTargetAccess}); err != nil || listed.GetResult() == nil || len(listed.GetResult().GetTypes()) != 0 {
		t.Fatalf("list installed empty target: %v %v", listed, err)
	}
}
