package acceptance_test

import (
	"bytes"
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

const reacceptanceProductEntity = "00000000-0000-4000-8000-00000000fa9d"

// TestFacetReacceptanceIndependentProducts moves one attached element from a
// source ucupa-core product to a second target ucupa-core product while one
// real ucupa-facet product keeps the payload. The whole chain stays on the
// public planes: the slice transfer settles through the two Core products
// under a live target approval, the attachment arrives DETACHED, and the same
// Facet re-accepts through the target's proposal while the successor proof
// crosses back to the departed source product. A request naming the target
// Core but routed to the source address is refused, and the departed source's
// fenced proposal proves nothing. The rebound payload stays byte-identical
// under the new acceptance while the superseded key is sealed, and full
// restarts of all three products replay the settled transfer and the committed
// acceptance with their exact original receipts.
func TestFacetReacceptanceIndependentProducts(t *testing.T) {
	ctx := context.Background()
	target, source, _ := transitionApprovalSetup(t)
	entity := values.ElementID(reacceptanceProductEntity)
	dir := t.TempDir()

	// The independent Facet realm provisions the operator plus one fixed
	// service principal for each Core product's inbound acceptance reads.
	actor := values.PrincipalRef{Realm: "facet-reacceptance", Principal: "operator"}
	serviceCredential := func(name string) (values.PrincipalRef, realm.CredentialEvidence) {
		p := values.PrincipalRef{Realm: actor.Realm, Principal: values.PrincipalID(name)}
		e := realm.CredentialEvidence{Audience: actor.Realm}
		rand.Read(e.ID[:])
		rand.Read(e.Secret[:])
		return p, e
	}
	sourceService, sourceServiceEvidence := serviceCredential("core-source")
	targetService, targetServiceEvidence := serviceCredential("core-target")
	operatorCredential := realm.CredentialEvidence{Audience: actor.Realm}
	rand.Read(operatorCredential.ID[:])
	rand.Read(operatorCredential.Secret[:])
	facetRealm, err := realminstance.CreateDurable(realm.Seed{
		ID:              actor.Realm,
		LocalPrincipals: []values.PrincipalID{actor.Principal, sourceService.Principal, targetService.Principal},
		Credentials: []realm.CredentialBootstrap{
			{Subject: actor.Principal, Evidence: operatorCredential},
			{Subject: sourceService.Principal, Evidence: sourceServiceEvidence},
			{Subject: targetService.Principal, Evidence: targetServiceEvidence},
		},
		Bindings: []realm.PermissionBinding{{Subject: actor, Permission: realm.PermissionPutBinding, Scope: values.RealmScope(actor.Realm)}},
	}, filepath.Join(dir, "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	// This provisioning adapter stays unused: the relationship below is built
	// entirely over the public RPC planes once the three products run.
	links := &composition.LocalFacetCoreConnections{}
	if err = links.BindEndpoint("local:bootstrap", composition.LocalFacetCoreEndpoint{Read: source.c.Facets(), Control: source.c.Facets(), Access: source.c.Access()}); err != nil {
		t.Fatal(err)
	}
	caller, err := source.r.EstablishCaller(realm.CredentialEvidence{Audience: source.actor.Realm, ID: realm.CredentialID(source.id), Secret: realm.CredentialSecret(source.secret)})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := links.ForFacetCaller(caller)
	if err != nil {
		t.Fatal(err)
	}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:facet-reacceptance", facetRealm); err != nil {
		t.Fatal(err)
	}
	holder, err := facet.CreateDurable(realmLinks, bootstrap, facetcontract.RealmInitialization{Realm: actor.Realm, NextHop: "local:facet-reacceptance"}, filepath.Join(dir, "facet.db"), facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	owner := holder.AuthorityRef()
	typ := fixtureType("content")
	grantFacet := func(subject values.PrincipalRef, permission values.PermissionID, scope values.OperationScope, name string) {
		t.Helper()
		if _, err = facetRealm.PutBinding(actor, realm.PermissionBinding{Subject: subject, Permission: permission, Scope: scope}, facetRealm.BindingSetRevision(), realm.OperationID(name)); err != nil {
			t.Fatal(err)
		}
	}
	grantFacet(actor, facetcontract.PermissionAccept, values.FacetElementScope(owner, entity), "reacceptance-product-operator-accept")
	for _, p := range []values.PermissionID{facetcontract.PermissionGet, facetcontract.PermissionPut} {
		grantFacet(actor, p, values.FacetKeyScope(owner, entity, typ), "reacceptance-product-operator-"+string(p))
	}
	for _, s := range []struct {
		subject values.PrincipalRef
		name    string
	}{{sourceService, "core-source"}, {targetService, "core-target"}} {
		grantFacet(s.subject, facetcontract.PermissionReadAcceptance, values.FacetElementScope(owner, entity), "reacceptance-product-"+s.name+"-read-acceptance")
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionTransitionExport, corecontract.PermissionTransitionCommit, corecontract.PermissionTransitionInspect} {
		source.grant(t, p, values.CoreElementScope(source.c.AuthorityRef(), entity), false)
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead, corecontract.PermissionTransitionImport} {
		target.grant(t, p, values.CoreElementScope(target.c.AuthorityRef(), entity), false)
	}
	source.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(source.c.AuthorityRef(), networkRoot), false)
	// The moving element is a child of the fixture root, created through the
	// real owner API before any product process exists.
	if _, err = source.c.Elements().CreateEntity(source.actor, entity, corecontract.SpatialTuple{
		Parent: networkRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "reacceptance-product-create"); err != nil {
		t.Fatal(err)
	}
	if err = holder.Close(); err != nil {
		t.Fatal(err)
	}
	if err = facetRealm.Close(); err != nil {
		t.Fatal(err)
	}
	certificate, roots, pin, certPEM, keyPEM := networkCertificate(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	facetAddress := l.Addr().String()
	l.Close()
	facetHop := values.NextHop("https://" + facetAddress)
	writeNetworkFile(t, filepath.Join(dir, "tls.pem"), certPEM)
	writeNetworkFile(t, filepath.Join(dir, "tls.key"), keyPEM)
	facetCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[1]})
	writeNetworkFile(t, filepath.Join(source.dir, "facet-ca.pem"), facetCA)
	writeNetworkFile(t, filepath.Join(target.dir, "facet-ca.pem"), facetCA)
	writeNetworkFile(t, filepath.Join(dir, "source-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.config.Certificate.Certificate[1]}))
	writeNetworkFile(t, filepath.Join(dir, "target-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.config.Certificate.Certificate[1]}))
	writeCredential := func(path string, audience values.RealmID, id, secret []byte) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"audience": audience, "credential_id": values.EncodePublicBytes(id), "secret": values.EncodePublicBytes(secret)})
		writeNetworkFile(t, path, b)
	}
	writeCredential(filepath.Join(dir, "source-credential.json"), source.actor.Realm, source.id[:], source.secret[:])
	writeCredential(filepath.Join(dir, "target-credential.json"), target.actor.Realm, target.id[:], target.secret[:])
	writeCredential(filepath.Join(source.dir, "facet-service-credential.json"), actor.Realm, sourceServiceEvidence.ID[:], sourceServiceEvidence.Secret[:])
	writeCredential(filepath.Join(target.dir, "facet-service-credential.json"), actor.Realm, targetServiceEvidence.ID[:], targetServiceEvidence.Secret[:])

	// The two Core products carry the transfer plane plus one fixed Facet
	// service each; the Facet product reaches both Core products through its
	// two configured Core connections.
	source.server.Stop()
	_ = source.client.Close()
	configuration := func(host, peer *networkFixture) map[string]any {
		return map[string]any{
			"read_delegation":        identityTrustFiles(t, host, peer),
			"transition_connections": map[string]any{"peers": mountProductConnections(t, host, peer), "max_concurrent_pulls": 4, "pull_deadline": "5s"},
			"transition_approvals":   map[string]any{"max_ttl": "30s", "max_approvals": 8, "max_per_caller": 6},
			"facet_services":         []any{map[string]any{"realm": host.actor.Realm, "principal": host.actor.Principal, "facet": owner}},
			"facet_connections":      []any{map[string]any{"next_hop": facetHop, "expected_facet": owner, "ca_file": filepath.Join(host.dir, "facet-ca.pem"), "spki_sha256": values.EncodePublicBytes(pin[:]), "credential_file": filepath.Join(host.dir, "facet-service-credential.json")}},
		}
	}
	startSource := prepareNetworkProduct(t, source, configuration(source, target))
	startTarget := prepareNetworkProduct(t, target, configuration(target, source))
	startFacet := prepareServiceProduct(t, dir, "ucupa-facet", map[string]any{
		"listen": facetAddress, "next_hop": facetHop, "expected_facet": owner, "realm_id": actor.Realm, "realm_next_hop": "local:facet-reacceptance",
		"facet_store": filepath.Join(dir, "facet.db"), "realm_store": filepath.Join(dir, "realm.db"),
		"certificate_file": filepath.Join(dir, "tls.pem"), "private_key_file": filepath.Join(dir, "tls.key"),
		"session_ttl": "30s", "rpc_deadline": "4s", "handshake_timeout": "1s", "connection_lifetime": "1m", "shutdown_timeout": "1s",
		"max_sessions": 16, "max_sessions_per_caller": 8, "max_connections": 16, "max_concurrent_rpcs": 8, "max_streams": 8,
		"max_metadata_bytes": 8192, "max_response_bytes": 4 << 20, "context_ttl": "1m", "observation_ttl": "1m",
		"core_services": []any{
			map[string]any{"realm": actor.Realm, "principal": sourceService.Principal, "core": source.config.Expected},
			map[string]any{"realm": actor.Realm, "principal": targetService.Principal, "core": target.config.Expected},
		},
		"core_connections": []any{
			map[string]any{"next_hop": source.config.NextHop, "expected_core": source.config.Expected, "ca_file": filepath.Join(dir, "source-ca.pem"), "spki_sha256": values.EncodePublicBytes(source.clientConfig.SPKISHA256[:]), "credential_file": filepath.Join(dir, "source-credential.json")},
			map[string]any{"next_hop": target.config.NextHop, "expected_core": target.config.Expected, "ca_file": filepath.Join(dir, "target-ca.pem"), "spki_sha256": values.EncodePublicBytes(target.clientConfig.SPKISHA256[:]), "credential_file": filepath.Join(dir, "target-credential.json")},
		},
	})
	stopSource, stopTarget, stopFacet := startSource(), startTarget(), startFacet()
	defer func() { stopSource(); stopTarget(); stopFacet() }()
	source.connect(t)
	target.connect(t)
	client, err := facetsdk.New(facetsdk.Config{NextHop: facetHop, Expected: owner, Roots: roots, SPKISHA256: pin, RPCDeadline: 5 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	login := func() {
		t.Helper()
		if err := client.Establish(ctx, actor.Realm, [16]byte(operatorCredential.ID), [32]byte(operatorCredential.Secret)); err != nil {
			t.Fatal(err)
		}
	}
	login()
	relationships := client.Relationships()

	// The first relationship is proposed, accepted and activated over the
	// public planes of the source Core product and the Facet product, and the
	// payload is written through the Facet product before any transfer.
	sourceControl := source.client.FacetControl()
	inspected, err := sourceControl.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(entity))})
	if err != nil || inspected.GetResult() == nil {
		t.Fatalf("initial inspect: %v %v", inspected, err)
	}
	proposed, err := sourceControl.ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("reacceptance-product-attach-propose"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(string(entity)), Expected: inspected.GetResult().GetRevision(),
		Target: proto.String(string(owner)), NextHop: proto.String(string(facetHop)),
	}})
	if err != nil || proposed.GetResult() == nil || proposed.GetResult().GetState().GetPendingProposal() == nil {
		t.Fatalf("attach propose: %v %v", proposed, err)
	}
	oldRef := &facetpb.PublicAcceptanceRef{Core: proto.String(string(source.config.Expected)), Element: proto.String(string(entity)), Proposal: proposed.GetResult().GetState().GetPendingProposal()}
	attached, err := relationships.AcceptAttachment(ctx, &facetpb.AcceptAttachmentRequest{Ref: oldRef, CoreNextHop: proto.String(string(source.config.NextHop)), OperationId: []byte("reacceptance-product-attach-accept")})
	if err != nil || attached.GetResult() == nil || attached.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		!attached.GetResult().GetInspection().GetFound() || attached.GetResult().GetInspection().GetCoreNextHop() != string(source.config.NextHop) {
		t.Fatalf("attach accept: %v %v", attached, err)
	}
	activated, err := sourceControl.ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("reacceptance-product-attach-activate"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("activate"), Element: proto.String(string(entity)), Expected: proposed.GetResult().GetState().GetRevision(),
	}})
	if err != nil || activated.GetResult() == nil || activated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("attach activate: %v %v", activated, err)
	}
	oldAccess := &facetpb.Access{NextHop: proto.String(string(facetHop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(string(source.config.Expected)), Facet: proto.String(string(owner)), Element: proto.String(string(entity)), Proposal: oldRef.Proposal,
	}}
	notes := fixturePayload("reacceptance-product-notes")
	if put, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: oldAccess, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("reacceptance-product-put"), Payload: notes}); err != nil || put.GetResult() == nil {
		t.Fatalf("content write: %v %v", put, err)
	}

	// The slice transfer settles through both Core products under a live
	// target approval; the attachment arrives DETACHED on the target and the
	// departed source stops serving the element.
	read, err := source.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: source.enter(t, corepb.Intent_READ), Address: &networkpb.Address{Down: &networkpb.ElementPath{Items: []string{string(entity)}}}})
	if err != nil || read.GetResult().GetRecord() == nil {
		t.Fatalf("read before transfer: %v %v", read, err)
	}
	binding := &corepb.TransitionApprovalBinding{Source: proto.String(string(source.config.Expected)), Command: &corepb.BeginTransitionRequest{
		OperationId: []byte("reacceptance-product-transfer"), Kind: corepb.TransitionKind_TRANSITION_SLICE.Enum(),
		Target: proto.String(string(target.config.Expected)), Boundary: proto.String(string(entity)),
		Expected: read.GetResult().GetRecord().GetRevision(), Final: networkTuple(4),
		SourceAddress: proto.String(string(source.config.NextHop)), TargetAddress: proto.String(string(target.config.NextHop)),
	}}
	intent, err := source.client.Transitions().BeginTransition(ctx, binding.Command)
	if err != nil || intent.GetResult().GetProgress() != corepb.TransitionProgress_TRANSITION_INTENT {
		t.Fatalf("begin transfer: %v %v", intent, err)
	}
	approval := createTransitionApproval(t, target, binding, 30)
	recover := &corepb.RecoverTransitionRequest{Binding: binding, ApprovalToken: approval.Token}
	finished, err := source.client.Transitions().RecoverTransition(ctx, recover)
	if err != nil || finished.GetResult().GetProgress() != corepb.TransitionProgress_TRANSITION_SETTLED {
		t.Fatalf("recover transfer: %v %v", finished, err)
	}
	if gone, err := source.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: source.enter(t, corepb.Intent_READ), Address: &networkpb.Address{Down: &networkpb.ElementPath{Items: []string{string(entity)}}}}); gone.GetResult() != nil || (err == nil && gone.GetFailure() != networkpb.Failure_NOT_FOUND) {
		t.Fatalf("departed source still serves the element: %v %v", gone, err)
	}

	// The target's public control plane discloses the DETACHED attachment with
	// the departed source as its Core, and the explicit re-acceptance proposal
	// is created there.
	targetControl := target.client.FacetControl()
	detached, err := targetControl.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(entity))})
	if err != nil || detached.GetResult() == nil || detached.GetResult().GetState() != corepb.AttachmentState_DETACHED ||
		detached.GetResult().GetKeyCore() != string(source.config.Expected) {
		t.Fatalf("inspect transferred attachment: %v %v", detached, err)
	}
	reproposed, err := targetControl.ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("reacceptance-product-repropose"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(string(entity)), Expected: detached.GetResult().GetRevision(),
		Target: proto.String(string(owner)), NextHop: proto.String(string(facetHop)),
	}})
	if err != nil || reproposed.GetResult() == nil || reproposed.GetResult().GetState().GetPendingProposal() == nil ||
		reproposed.GetResult().GetState().GetPendingTarget() != string(owner) {
		t.Fatalf("re-propose: %v %v", reproposed, err)
	}
	newRef := &facetpb.PublicAcceptanceRef{Core: proto.String(string(target.config.Expected)), Element: proto.String(string(entity)), Proposal: reproposed.GetResult().GetState().GetPendingProposal()}
	reattach := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(target.config.NextHop)), OperationId: []byte("reacceptance-product-accept")}

	// No product answers for another Core's identity: a request naming the
	// target Core but routed to the departed source's address is refused.
	wrongHop := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(source.config.NextHop)), OperationId: []byte("reacceptance-product-wrong-hop")}
	if refused, err := relationships.AcceptAttachment(ctx, wrongHop); refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("wrong Core hop accepted: %v %v", refused, err)
	}

	// The same Facet re-accepts through the target product's proposal: the
	// resolution crosses to the target while the successor proof crosses back
	// to the departed source product, and the payload never moves.
	accepted, err := relationships.AcceptAttachment(ctx, reattach)
	if err != nil || accepted.GetResult() == nil || accepted.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		!accepted.GetResult().GetInspection().GetFound() || accepted.GetResult().GetInspection().GetCoreNextHop() != string(target.config.NextHop) {
		t.Fatalf("re-acceptance: %v %v", accepted, err)
	}
	if replayed, err := relationships.AcceptAttachment(ctx, reattach); err != nil || !proto.Equal(replayed, accepted) {
		t.Fatalf("replayed re-acceptance: %v %v", replayed, err)
	}

	// The departed source's fenced proposal proves nothing once its lineage is
	// superseded: the resolution reaches the departed source product and is
	// refused, never replayed as the old acceptance.
	staleProposal := &facetpb.AcceptAttachmentRequest{
		Ref:         &facetpb.PublicAcceptanceRef{Core: proto.String(string(source.config.Expected)), Element: proto.String(string(entity)), Proposal: oldRef.Proposal},
		CoreNextHop: proto.String(string(source.config.NextHop)), OperationId: []byte("reacceptance-product-stale-proposal"),
	}
	if stale, err := relationships.AcceptAttachment(ctx, staleProposal); stale.GetResult() != nil || (err == nil && stale.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("stale proposal accepted: %v %v", stale, err)
	}
	reactivated, err := targetControl.ApplyFacet(ctx, &corepb.ApplyFacetRequest{OperationId: []byte("reacceptance-product-activate"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("activate"), Element: proto.String(string(entity)), Expected: reproposed.GetResult().GetState().GetRevision(),
	}})
	if err != nil || reactivated.GetResult() == nil || reactivated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("re-activate: %v %v", reactivated, err)
	}
	newAccess := &facetpb.Access{NextHop: proto.String(string(facetHop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(string(target.config.Expected)), Facet: proto.String(string(owner)), Element: proto.String(string(entity)), Proposal: newRef.Proposal,
	}}
	served, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: typ.Bytes()})
	if err != nil || served.GetResult() == nil || !bytes.Equal(served.GetResult().GetPayload(), notes) {
		t.Fatalf("rebound payload: %v %v", served, err)
	}
	if sealed, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: oldAccess, TypeRef: typ.Bytes()}); sealed.GetResult() != nil || (err == nil && sealed.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("superseded key served content: %v %v", sealed, err)
	}

	// Full restarts of all three products keep the settled transfer, the
	// committed acceptance and the payload: each original operation replays
	// with its exact original receipt, the superseded key stays sealed and the
	// reactivated relationship stays ACTIVE on the target.
	stopSource()
	stopTarget()
	stopFacet()
	stopSource, stopTarget, stopFacet = startSource(), startTarget(), startFacet()
	_ = source.client.Close()
	_ = target.client.Close()
	source.connect(t)
	target.connect(t)
	login()
	recovered, err := source.client.Transitions().RecoverTransition(ctx, recover)
	if err != nil || !proto.Equal(recovered, finished) {
		t.Fatalf("replayed transfer after restart: %v %v", recovered, err)
	}
	afterRestart, err := relationships.AcceptAttachment(ctx, reattach)
	if err != nil || !proto.Equal(afterRestart, accepted) {
		t.Fatalf("replayed re-acceptance after restart: %v %v", afterRestart, err)
	}
	after, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: typ.Bytes()})
	if err != nil || after.GetResult() == nil || !bytes.Equal(after.GetResult().GetPayload(), notes) {
		t.Fatalf("payload after restart: %v %v", after, err)
	}
	if sealed, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: oldAccess, TypeRef: typ.Bytes()}); sealed.GetResult() != nil || (err == nil && sealed.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("superseded key served after restart: %v %v", sealed, err)
	}
	finalState, err := target.client.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(entity))})
	if err != nil || finalState.GetResult() == nil || finalState.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		finalState.GetResult().GetKeyCore() != string(target.config.Expected) {
		t.Fatalf("final attachment state: %v %v", finalState, err)
	}
}
