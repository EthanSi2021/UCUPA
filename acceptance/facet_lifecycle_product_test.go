package acceptance_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
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

const lifecycleEntity = "00000000-0000-4000-8000-00000000fa9c"

// TestFacetRecoveryRetirementIndependentProducts runs the P7.3 relationship
// lifecycle across one real ucupa-core product and one real ucupa-facet
// product, two independent processes. The relationship is proposed, accepted
// and activated over public RPC; the written payload survives a same-Core
// recovery byte-identically without a second writable lineage; the final
// disposal retires the namespace through the Facet product and releases the
// Core product's occupancy; the released element becomes deletable, and full
// product restarts replay the committed retire and release with their exact
// original receipts while the retired namespace stays sealed.
func TestFacetRecoveryRetirementIndependentProducts(t *testing.T) {
	runFacetLifecycleProducts(t, false)
}

func runFacetLifecycleProducts(t *testing.T, networkRealm bool) {
	ctx := context.Background()
	c := newNetworkFixture(t, false)
	dir := t.TempDir()
	actor := values.PrincipalRef{Realm: "facet-lifecycle", Principal: "operator"}
	credential := realm.CredentialEvidence{Audience: actor.Realm}
	rand.Read(credential.ID[:])
	rand.Read(credential.Secret[:])
	r, err := realminstance.CreateDurable(realm.Seed{ID: actor.Realm, LocalPrincipals: []values.PrincipalID{actor.Principal}, Credentials: []realm.CredentialBootstrap{{Subject: actor.Principal, Evidence: credential}}, Bindings: []realm.PermissionBinding{{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)}}}, filepath.Join(dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	// This provisioning adapter stays unused: the relationship below is built
	// entirely over the public RPC plane once both products run.
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
	if err = realmLinks.BindEndpoint("local:facet-lifecycle", r); err != nil {
		t.Fatal(err)
	}
	f, err := facet.CreateDurable(realmLinks, source, facetcontract.RealmInitialization{Realm: actor.Realm, NextHop: "local:facet-lifecycle"}, filepath.Join(dir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	owner := f.AuthorityRef()
	typ := fixtureType("content")
	for _, p := range []values.PermissionID{facetcontract.PermissionAccept, facetcontract.PermissionReadAcceptance, facetcontract.PermissionCoreContactUpdate, facetcontract.PermissionRetireContent} {
		if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: p, Scope: values.FacetElementScope(owner, values.ElementID(lifecycleEntity))}, r.BindingSetRevision(), realm.OperationID(string(p))); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionGet, facetcontract.PermissionPut, facetcontract.PermissionDelete} {
		if _, err = r.PutBinding(actor, realm.PermissionBinding{Subject: actor, Permission: p, Scope: values.FacetKeyScope(owner, values.ElementID(lifecycleEntity), typ)}, r.BindingSetRevision(), realm.OperationID(string(p))); err != nil {
			t.Fatal(err)
		}
	}
	c.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(c.c.AuthorityRef(), networkRoot), false)
	for _, p := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetDetach, corecontract.PermissionFacetContactUpdate, corecontract.PermissionFacetRelease, corecontract.PermissionDeleteEntity} {
		c.grant(t, p, values.CoreElementScope(c.c.AuthorityRef(), values.ElementID(lifecycleEntity)), false)
	}
	// The retiring element is a child of the fixture root, so the released
	// occupancy can be proven by an ordinary public deletion.
	if _, err = c.c.Elements().CreateEntity(c.actor, values.ElementID(lifecycleEntity), corecontract.SpatialTuple{
		Parent: networkRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "lifecycle-create"); err != nil {
		t.Fatal(err)
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
	if networkRealm {
		endpoint, startRealm := prepareFacetNetworkRealm(t, dir, r, f, actor)
		realmEndpoint = endpoint
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		startRealm()
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	startCore := prepareNetworkProduct(t, c, map[string]any{
		"facet_services":    []any{map[string]any{"realm": c.actor.Realm, "principal": c.actor.Principal, "facet": owner}},
		"facet_connections": []any{map[string]any{"next_hop": hop, "expected_facet": owner, "ca_file": filepath.Join(c.dir, "facet-ca.pem"), "spki_sha256": values.EncodePublicBytes(pin[:]), "credential_file": filepath.Join(c.dir, "facet-credential.json")}},
	})
	cp := c.clientConfig.SPKISHA256
	configuration := map[string]any{"listen": address, "next_hop": hop, "expected_facet": owner, "realm_id": actor.Realm, "realm_next_hop": "local:facet-lifecycle", "facet_store": filepath.Join(dir, "facet.db"), "realm_store": filepath.Join(dir, "realm.db"), "certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"), "session_ttl": "30s", "rpc_deadline": "4s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s", "max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8, "max_streams": 8, "max_metadata_bytes": 8192, "max_response_bytes": 4 << 20, "context_ttl": "1m", "observation_ttl": "1m",
		"core_services":    []any{map[string]any{"realm": actor.Realm, "principal": actor.Principal, "core": c.config.Expected}},
		"core_connections": []any{map[string]any{"next_hop": c.config.NextHop, "expected_core": c.config.Expected, "ca_file": filepath.Join(dir, "core-ca.pem"), "spki_sha256": values.EncodePublicBytes(cp[:]), "credential_file": filepath.Join(dir, "core-credential.json")}},
	}
	if networkRealm {
		delete(configuration, "realm_store")
		delete(configuration, "realm_next_hop")
		configuration["realm_profile"] = "network"
		configuration["realm_endpoints"] = []any{realmEndpoint}
	}
	startFacet := prepareServiceProduct(t, dir, "ucupa-facet", configuration)
	stopCore, stopFacet := startCore(), startFacet()
	c.connect(t)
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
	relationships := client.Relationships()
	control := c.client.FacetControl()
	inspected, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(lifecycleEntity)})
	if err != nil || inspected.GetResult() == nil {
		t.Fatalf("initial inspect: %v %v", inspected, err)
	}
	build := func(name string) (*facetpb.Access, *facetpb.PublicAcceptanceRef) {
		t.Helper()
		proposal := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-" + name + "-propose"), Command: &corepb.PublicFacetCommand{
			Kind: proto.String("propose"), Element: proto.String(lifecycleEntity), Expected: inspected.GetResult().GetRevision(),
			Target: proto.String(string(owner)), NextHop: proto.String(string(hop)),
		}}
		p, err := control.ApplyFacet(ctx, proposal)
		if err != nil || p.GetResult() == nil || p.GetResult().GetState().GetPendingProposal() == nil {
			t.Fatalf("%s propose: %v %v", name, p, err)
		}
		ref := &facetpb.PublicAcceptanceRef{Core: proto.String(string(c.config.Expected)), Element: proto.String(lifecycleEntity), Proposal: p.GetResult().GetState().GetPendingProposal()}
		accept := &facetpb.AcceptAttachmentRequest{Ref: ref, CoreNextHop: proto.String(string(c.config.NextHop)), OperationId: []byte("lifecycle-" + name + "-accept")}
		a, err := relationships.AcceptAttachment(ctx, accept)
		if err != nil || a.GetResult() == nil || a.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED {
			t.Fatalf("%s accept: %v %v", name, a, err)
		}
		activate := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-" + name + "-activate"), Command: &corepb.PublicFacetCommand{
			Kind: proto.String("activate"), Element: proto.String(lifecycleEntity), Expected: p.GetResult().GetState().GetRevision(),
		}}
		active, err := control.ApplyFacet(ctx, activate)
		if err != nil || active.GetResult() == nil || active.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
			t.Fatalf("%s activate: %v %v", name, active, err)
		}
		inspected, err = control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(lifecycleEntity)})
		if err != nil || inspected.GetResult() == nil {
			t.Fatalf("%s reinspect: %v %v", name, inspected, err)
		}
		access := &facetpb.Access{NextHop: proto.String(string(hop)), Key: &networkpb.AttachmentKey{
			Core: proto.String(string(c.config.Expected)), Facet: proto.String(string(owner)), Element: proto.String(lifecycleEntity), Proposal: ref.Proposal,
		}}
		return access, ref
	}
	access, ref := build("attach")
	put, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("lifecycle-put"), Payload: []byte(`"lifecycle"`)})
	if err != nil || put.GetResult() == nil {
		t.Fatalf("content write: %v %v", put, err)
	}

	// Detaching ends the only writable lineage: the held content answers
	// UNAVAILABLE, never absence or a still-open write gate.
	detach := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-detach"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("detach"), Element: proto.String(lifecycleEntity), Expected: inspected.GetResult().GetRevision(),
	}}
	detached, err := control.ApplyFacet(ctx, detach)
	if err != nil || detached.GetResult() == nil || detached.GetResult().GetState().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("detach: %v %v", detached, err)
	}
	held, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if held.GetResult() != nil || (err == nil && held.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("detached content readable: %v %v", held, err)
	}
	inspected, err = control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(lifecycleEntity)})
	if err != nil || inspected.GetResult() == nil || inspected.GetResult().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("reinspect detached: %v %v", inspected, err)
	}

	// Same-Core recovery rebinds one new lineage over the DETACHED predecessor
	// without copying content: the payload stays byte-identical under the new
	// acceptance while the old key is permanently unavailable.
	oldAccess := access
	access, ref = build("recover")
	recovered, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	if err != nil || recovered.GetResult() == nil || string(recovered.GetResult().Payload) != `"lifecycle"` {
		t.Fatalf("recovered payload: %v %v", recovered, err)
	}
	stale, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: oldAccess, TypeRef: typ.Bytes()})
	if stale.GetResult() != nil || (err == nil && stale.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("old key still serving: %v %v", stale, err)
	}

	// A disposal request while the relationship is still active has no
	// retirement evidence: it fails safely and stays retryable.
	premature := &facetpb.RetireContentRequest{Ref: ref, OperationId: []byte("lifecycle-premature-retire")}
	if out, err := relationships.RetireContent(ctx, premature); out.GetResult() != nil || (err == nil && out.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("premature retire committed: %v %v", out, err)
	}

	// The final detach discloses the evidence the disposal needs.
	finalDetach := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-detach-final"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("detach"), Element: proto.String(lifecycleEntity), Expected: inspected.GetResult().GetRevision(),
	}}
	detachedFinal, err := control.ApplyFacet(ctx, finalDetach)
	if err != nil || detachedFinal.GetResult() == nil || detachedFinal.GetResult().GetState().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("final detach: %v %v", detachedFinal, err)
	}
	detachedRevision := detachedFinal.GetResult().GetState().GetRevision()

	// The explicit disposal runs on the Facet product and reads the Core
	// product's DETACHED disclosure over the wire as its evidence.
	retire := &facetpb.RetireContentRequest{Ref: ref, OperationId: []byte("lifecycle-retire")}
	retired, err := relationships.RetireContent(ctx, retire)
	if err != nil || retired.GetResult() == nil || retired.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		retired.GetResult().GetInspection().GetFound() || !proto.Equal(retired.GetResult().GetInspection().GetRef(), ref) {
		t.Fatalf("retire: %v %v", retired, err)
	}
	retireReplay, err := relationships.RetireContent(ctx, retire)
	if err != nil || !proto.Equal(retireReplay, retired) {
		t.Fatalf("replayed retire: %v %v", retireReplay, err)
	}

	// The restarted Facet product loses every runtime session. The Core
	// product's fixed service session turns stale on the first release, which
	// fails safely without a journal decision; the exact retry re-establishes
	// the session and commits the release.
	stopFacet()
	stopFacet = startFacet()
	login()
	release := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-release"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("release"), Element: proto.String(lifecycleEntity), Expected: detachedRevision,
		Target: proto.String(string(owner)), NextHop: proto.String(string(hop)),
	}}
	released, err := control.ApplyFacet(ctx, release)
	if released.GetResult() != nil || (err == nil && released.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("release before session recovery: %v %v", released, err)
	}
	released, err = control.ApplyFacet(ctx, release)
	if err != nil || released.GetResult() == nil || released.GetResult().GetState().GetState() != corepb.AttachmentState_RETIRED ||
		released.GetResult().GetState().GetTarget() != string(owner) || released.GetResult().GetState().GetKeyCore() != string(c.config.Expected) {
		t.Fatalf("release: %v %v", released, err)
	}
	releaseReplay, err := control.ApplyFacet(ctx, release)
	if err != nil || !proto.Equal(releaseReplay, released) {
		t.Fatalf("replayed release: %v %v", releaseReplay, err)
	}

	// The retired slot is terminal for the product surfaces: a second release
	// or a reviving proposal is a state conflict, and neither content reads nor
	// a second disposal serve the retired namespace again.
	retiredInspect, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(lifecycleEntity)})
	if err != nil || retiredInspect.GetResult() == nil || retiredInspect.GetResult().GetState() != corepb.AttachmentState_RETIRED {
		t.Fatalf("inspect retired: %v %v", retiredInspect, err)
	}
	again := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-release-again"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("release"), Element: proto.String(lifecycleEntity), Expected: retiredInspect.GetResult().GetRevision(),
		Target: proto.String(string(owner)), NextHop: proto.String(string(hop)),
	}}
	if second, err := control.ApplyFacet(ctx, again); second.GetResult() != nil || (err == nil && second.GetFailure() != networkpb.Failure_CONFLICT) {
		t.Fatalf("second release: %v %v", second, err)
	}
	revive := &corepb.ApplyFacetRequest{OperationId: []byte("lifecycle-revive"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(lifecycleEntity), Expected: retiredInspect.GetResult().GetRevision(),
		Target: proto.String(string(owner)), NextHop: proto.String(string(hop)),
	}}
	if out, err := control.ApplyFacet(ctx, revive); out.GetResult() != nil || (err == nil && out.GetFailure() != networkpb.Failure_CONFLICT) {
		t.Fatalf("retired slot re-proposed: %v %v", out, err)
	}
	if sealed, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()}); sealed.GetResult() != nil || (err == nil && sealed.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired content readable: %v %v", sealed, err)
	}
	if out, err := relationships.RetireContent(ctx, &facetpb.RetireContentRequest{Ref: ref, OperationId: []byte("lifecycle-retire-again")}); out.GetResult() != nil || (err == nil && out.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired namespace disposed again: %v %v", out, err)
	}

	// The discharged obligation unblocks the ordinary public deletion of the
	// released element; the deleted element no longer reads.
	current, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: c.enter(t, corepb.Intent_READ), Address: &networkpb.Address{Down: &networkpb.ElementPath{Items: []string{lifecycleEntity}}}})
	if err != nil || current.GetResult() == nil || current.GetResult().GetRecord() == nil {
		t.Fatalf("read before delete: %v %v", current, err)
	}
	deleted, err := c.client.Core().DeleteEntity(ctx, &corepb.DeleteEntityRequest{Handle: c.enter(t, corepb.Intent_DELETE_ENTITY), Element: proto.String(lifecycleEntity), Expected: current.GetResult().GetRecord().GetRevision(), OperationId: []byte("lifecycle-delete")})
	if err != nil || deleted.GetResult() == nil {
		t.Fatalf("delete after release: %v %v", deleted, err)
	}
	if gone, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: c.enter(t, corepb.Intent_READ), Address: &networkpb.Address{Down: &networkpb.ElementPath{Items: []string{lifecycleEntity}}}}); gone.GetResult() != nil || (err == nil && gone.GetFailure() != networkpb.Failure_NOT_FOUND) {
		t.Fatalf("deleted element readable: %v %v", gone, err)
	}

	// A full restart of both products replays the committed disposal and the
	// journaled release with their exact original receipts, keeps the retired
	// namespace sealed and the deleted element gone.
	stopCore()
	stopFacet()
	stopCore, stopFacet = startCore(), startFacet()
	c.client.Close()
	c.connect(t)
	control = c.client.FacetControl()
	login()
	recoveredRetire, err := relationships.RetireContent(ctx, retire)
	if err != nil || !proto.Equal(recoveredRetire, retired) {
		t.Fatalf("replayed retire after restart: %v %v", recoveredRetire, err)
	}
	replayedRelease, err := control.ApplyFacet(ctx, release)
	if err != nil || !proto.Equal(replayedRelease, released) {
		t.Fatalf("replayed release after restart: %v %v", replayedRelease, err)
	}
	if sealed, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()}); sealed.GetResult() != nil || (err == nil && sealed.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired content readable after restart: %v %v", sealed, err)
	}
	if gone, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: c.enter(t, corepb.Intent_READ), Address: &networkpb.Address{Down: &networkpb.ElementPath{Items: []string{lifecycleEntity}}}}); gone.GetResult() != nil || (err == nil && gone.GetFailure() != networkpb.Failure_NOT_FOUND) {
		t.Fatalf("deleted element readable after restart: %v %v", gone, err)
	}
}
