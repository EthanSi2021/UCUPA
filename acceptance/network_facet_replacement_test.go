package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/composition"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	facetsdk "ucupa/ucupa-facet/sdk/go"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
)

const replacementEmptyElement values.ElementID = "00000000-0000-4000-8000-00000000fa52"

func facetAccessFromInspection(t *testing.T, coreOwner values.FactAuthorityRef, inspection *corepb.PublicFacetInspection) *facetpb.Access {
	t.Helper()
	if inspection == nil || inspection.GetState() != corepb.AttachmentState_ACTIVE || inspection.GetTarget() == "" || inspection.GetNextHop() == "" || inspection.GetProposal() == nil {
		t.Fatalf("invalid active Facet inspection: %#v", inspection)
	}
	proposal, ok := proto.Clone(inspection.GetProposal()).(*networkpb.Token)
	if !ok {
		t.Fatal("Facet proposal token has unexpected protobuf type")
	}
	return &facetpb.Access{
		NextHop: proto.String(inspection.GetNextHop()),
		Key: &networkpb.AttachmentKey{
			Core: proto.String(string(coreOwner)), Facet: proto.String(inspection.GetTarget()),
			Element: proto.String(inspection.GetElement()), Proposal: proposal,
		},
	}
}

// fixedServiceCredentials supplies one fixed service credential to the
// composition adapters, which maintain the session before every peer call.
type fixedServiceCredentials struct{ evidence realm.CredentialEvidence }

func (c fixedServiceCredentials) Evidence() (values.RealmID, [16]byte, [32]byte, error) {
	return c.evidence.Audience, [16]byte(c.evidence.ID), [32]byte(c.evidence.Secret), nil
}

// TestFacetNetworkReplacementMovesNamespace runs one complete namespace
// replacement across three TLS hosts: the controlling Core, the source Facet
// and the target Facet. The authorized begin and the whole recovery cross the
// Core's public control plane, every peer step crosses the network, the fenced
// source refuses its former public access over the wire, the target serves the
// moved bytes publicly, and the settled state survives a full restart of all
// three owners and hosts.
func TestFacetNetworkReplacementMovesNamespace(t *testing.T) {
	f := newFacetNetworkFixture(t)
	w := f.w
	ctx := context.Background()
	const element values.ElementID = publicRoot
	notes := fixtureType("replacement-notes")
	emptyType := fixtureType("replacement-empty")
	profilePayload, notesPayload := fixturePayload("network-replacement-profile"), fixturePayload("network-replacement-notes")
	sourceOwner, targetOwner, coreOwner := w.facets[0].AuthorityRef(), w.facets[1].AuthorityRef(), w.cores[0].AuthorityRef()

	// A second, never-written active namespace exercises the exact zero-entry
	// manifest path without changing the nonempty move below.
	w.grant(t, w.actor, corecontract.PermissionCreateEntity, values.CoreElementScope(coreOwner, element))
	if _, err := w.cores[0].Elements().CreateEntity(w.actor, replacementEmptyElement, corecontract.SpatialTuple{
		Parent: element, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "network-replacement-empty-create"); err != nil {
		t.Fatal(err)
	}
	emptyKey := w.attach(t, 0, 0, replacementEmptyElement)

	// The actor relays a second type on the source and reads both relayed
	// types on the target; the fixed Core service exports the source
	// namespace and imports the target one.
	w.grant(t, w.actor, facetcontract.PermissionPut, values.FacetKeyScope(sourceOwner, element, notes))
	w.grant(t, w.actor, corecontract.PermissionFacetProposalRead, values.CoreElementScope(coreOwner, element))
	w.grant(t, w.actor, corecontract.PermissionFacetReplacementBegin, values.CoreElementScope(coreOwner, element))
	w.grant(t, w.actor, corecontract.PermissionFacetReplacementRecover, values.CoreElementScope(coreOwner, element))
	w.grant(t, w.actor, corecontract.PermissionFacetReplacementRead, values.CoreElementScope(coreOwner, element))
	w.grant(t, w.service, facetcontract.PermissionNamespaceExport, values.FacetElementScope(sourceOwner, element))
	w.grant(t, w.service, facetcontract.PermissionNamespaceImport, values.FacetElementScope(targetOwner, element))
	for _, typ := range []values.FacetTypeRef{w.typ, notes} {
		w.grant(t, w.actor, facetcontract.PermissionGet, values.FacetKeyScope(targetOwner, element, typ))
	}
	w.grant(t, w.actor, corecontract.PermissionFacetReplacementBegin, values.CoreElementScope(coreOwner, replacementEmptyElement))
	w.grant(t, w.actor, corecontract.PermissionFacetReplacementRecover, values.CoreElementScope(coreOwner, replacementEmptyElement))
	w.grant(t, w.actor, corecontract.PermissionFacetReplacementRead, values.CoreElementScope(coreOwner, replacementEmptyElement))
	w.grant(t, w.actor, corecontract.PermissionFacetProposalRead, values.CoreElementScope(coreOwner, replacementEmptyElement))
	w.grant(t, w.service, facetcontract.PermissionNamespaceExport, values.FacetElementScope(sourceOwner, replacementEmptyElement))
	w.grant(t, w.service, facetcontract.PermissionNamespaceImport, values.FacetElementScope(targetOwner, replacementEmptyElement))
	w.grant(t, w.actor, facetcontract.PermissionGet, values.FacetKeyScope(sourceOwner, replacementEmptyElement, emptyType))
	w.grant(t, w.actor, facetcontract.PermissionGet, values.FacetKeyScope(targetOwner, replacementEmptyElement, emptyType))
	w.grant(t, w.actor, facetcontract.PermissionListTypes, values.FacetElementScope(targetOwner, replacementEmptyElement))

	// Both live entries are written through the original source host before
	// the owners switch to network contacts.
	f.login(t)
	api := f.client.Facet()
	for _, write := range []struct {
		op      string
		typ     values.FacetTypeRef
		payload []byte
	}{{"network-replacement-put-a", w.typ, profilePayload}, {"network-replacement-put-b", notes, notesPayload}} {
		put, err := api.Put(ctx, &facetpb.PutRequest{Access: f.access, TypeRef: write.typ.Bytes(), Payload: write.payload, Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte(write.op)})
		if err != nil || put.GetResult() == nil || put.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED {
			t.Fatalf("source put %s: %v %v", write.op, put, err)
		}
	}

	// The three hosts share one certificate authority for this test; the
	// original source-host certificate retires with the fixture's server.
	cert, roots, pin, _, _ := networkCertificate(t)
	f.server.Stop()
	if err := w.facets[0].Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.facets[1].Close(); err != nil {
		t.Fatal(err)
	}
	sourceAddress, _ := f.config.NextHop.NetworkTarget()
	sourceListener, err := net.Listen("tcp", sourceAddress)
	if err != nil {
		t.Fatal(err)
	}
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	coreListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sourceHop, targetAddress := f.config.NextHop, targetListener.Addr().String()
	targetHop, coreAddress := values.NextHop("https://"+targetAddress), coreListener.Addr().String()
	coreHop := values.NextHop("https://" + coreAddress)

	coreClientConfig := coresdk.Config{Expected: coreOwner, NextHop: coreHop, Roots: roots, SPKISHA256: pin, RPCDeadline: 3 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20}
	newCoreClient := func() *coresdk.Client {
		c, err := coresdk.New(coreClientConfig)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	newFacetClient := func(expected values.FactAuthorityRef, hop values.NextHop) *facetsdk.Client {
		c, err := facetsdk.New(facetsdk.Config{Expected: expected, NextHop: hop, Roots: roots, SPKISHA256: pin, RPCDeadline: 3 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 4 << 20, MaxMetadataBytes: 8192})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}

	// Each reopened Facet reads its controlling Core only at the Core host.
	sourceNetSource, err := composition.NewNetworkFacetCoreSource(sourceOwner, []composition.FacetCoreEndpoint{{Address: coreHop, Core: coreOwner, Client: newCoreClient(), Credentials: fixedServiceCredentials{f.serviceEvidence}}})
	if err != nil {
		t.Fatal(err)
	}
	targetNetSource, err := composition.NewNetworkFacetCoreSource(targetOwner, []composition.FacetCoreEndpoint{{Address: coreHop, Core: coreOwner, Client: newCoreClient(), Credentials: fixedServiceCredentials{f.serviceEvidence}}})
	if err != nil {
		t.Fatal(err)
	}
	w.facets[0], err = facet.OpenDurable(w.realmLinks, sourceNetSource, w.paths[2], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	w.facets[1], err = facet.OpenDurable(w.realmLinks, targetNetSource, w.paths[3], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if w.facets[0].AuthorityRef() != sourceOwner || w.facets[1].AuthorityRef() != targetOwner {
		t.Fatal("reopened Facet identities differ")
	}

	// Both Facet hosts admit only the fixed Core service for the peer ports.
	sourceConfig := f.config
	sourceConfig.Certificate = cert
	sourceConfig.CoreServices = map[values.PrincipalRef]values.FactAuthorityRef{w.service: coreOwner}
	sourceServer, err := facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sourceServer.Stop)
	go sourceServer.Serve(sourceListener)
	targetConfig := facetgrpc.Config{Expected: targetOwner, NextHop: targetHop, Certificate: cert, SessionTTL: 30 * time.Second, RPCDeadline: 3 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20, CoreServices: map[values.PrincipalRef]values.FactAuthorityRef{w.service: coreOwner}}
	targetServer, err := facetgrpc.New(w.facets[1].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[1].Namespace}, targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(targetServer.Stop)
	go targetServer.Serve(targetListener)

	// The durable contacts now name the network hosts; both repairs stay the
	// original caller's own bounded control operations.
	acceptance, err := w.facets[0].Namespace.ReadAcceptance(w.actor, w.key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.facets[0].Namespace.UpdateCoreNextHop(w.actor, "network-replacement-core-contact", w.key, acceptance.NextHopRevision, coreHop); err != nil {
		t.Fatal(err)
	}
	read, err := w.cores[0].Elements().ReadElement(w.actor, element, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.cores[0].Facets().UpdateFacetNextHop(w.actor, "network-replacement-facet-contact", w.key, read.Attachment.NextHopRevision, sourceHop); err != nil {
		t.Fatal(err)
	}
	emptyAcceptance, err := w.facets[0].Namespace.ReadAcceptance(w.actor, emptyKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.facets[0].Namespace.UpdateCoreNextHop(w.actor, "network-empty-replacement-core-contact", emptyKey, emptyAcceptance.NextHopRevision, coreHop); err != nil {
		t.Fatal(err)
	}
	emptyCoreRead, err := w.cores[0].Elements().ReadElement(w.actor, replacementEmptyElement, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.cores[0].Facets().UpdateFacetNextHop(w.actor, "network-empty-replacement-facet-contact", emptyKey, emptyCoreRead.Attachment.NextHopRevision, sourceHop); err != nil {
		t.Fatal(err)
	}

	// The Core host wires the fixed replacement peers from the same trusted
	// Facet endpoints. The shared test service principal maps to the target:
	// the install premise is the target's own attachment read, and the
	// replacement record read admits either record party.
	peerSource, err := composition.NewNetworkCoreFacetSource(coreOwner, []composition.CoreFacetEndpoint{
		{Address: sourceHop, Facet: sourceOwner, Client: newFacetClient(sourceOwner, sourceHop), Credentials: fixedServiceCredentials{f.serviceEvidence}},
		{Address: targetHop, Facet: targetOwner, Client: newFacetClient(targetOwner, targetHop), Credentials: fixedServiceCredentials{f.serviceEvidence}},
	})
	if err != nil {
		t.Fatal(err)
	}
	coreConfig := coregrpc.Config{Expected: coreOwner, NextHop: coreHop, Certificate: cert, SessionTTL: time.Minute, RPCDeadline: 3 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20,
		FacetServices:    map[values.PrincipalRef]values.FactAuthorityRef{w.service: targetOwner},
		ReplacementPeers: peerSource.ForReplacement(),
	}
	coreConfig.FacetControl = w.cores[0].Facets()
	coreConfig.AttachmentControl = w.cores[0].Facets()
	coreServer, err := coregrpc.New(w.cores[0].Access(), composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.cores[0].Access()}, coreConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(coreServer.Stop)
	go coreServer.Serve(coreListener)

	// The whole control plane crosses the network: inspect for the exact
	// current comparison, begin, and the single-writer recovery.
	admin := newCoreClient()
	if err = admin.Establish(ctx, f.admin.Audience, [16]byte(f.admin.ID), [32]byte(f.admin.Secret)); err != nil {
		t.Fatal(err)
	}
	control := admin.FacetControl()
	inspected, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(element))})
	if err != nil || inspected.GetResult() == nil || inspected.GetResult().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("inspect: %v %v", inspected, err)
	}
	const op corecontract.OperationID = "network-replacement-move"
	begin, err := control.BeginFacetReplacement(ctx, &corepb.BeginFacetReplacementRequest{OperationId: []byte(op), Command: &corepb.PublicFacetReplacementCommand{Element: proto.String(string(element)), Expected: inspected.GetResult().GetRevision(), Target: proto.String(string(targetOwner)), TargetAddress: proto.String(string(targetHop)), SourceAddress: proto.String(string(coreHop))}})
	if err != nil || begin.GetResult() == nil {
		t.Fatalf("begin: %v %v", begin, err)
	}
	if b := begin.GetResult(); b.GetProgress() != uint32(corecontract.FacetReplacementReserved) || b.GetRefused() ||
		b.GetSource() != string(sourceOwner) || b.GetTarget() != string(targetOwner) ||
		b.GetRevision().GetKind() != networkpb.TokenKind_FACET_REPLACEMENT_REVISION || b.GetRevision().GetOwner() != string(coreOwner) || len(b.GetRevision().GetValue()) != 32 {
		t.Fatalf("begin status = %#v", b)
	}
	controlRequest := &corepb.ReplacementControlRequest{OperationId: []byte(op), Element: proto.String(string(element))}
	final, err := control.RecoverFacetReplacement(ctx, controlRequest)
	if err != nil || final.GetResult() == nil {
		t.Fatalf("recover: %v %v", final, err)
	}
	if final.GetResult().GetProgress() != uint32(corecontract.FacetReplacementSettled) || final.GetResult().GetRefused() ||
		!proto.Equal(final.GetResult().GetRevision(), begin.GetResult().GetRevision()) {
		t.Fatalf("final status = %#v", final.GetResult())
	}
	replay, err := control.RecoverFacetReplacement(ctx, controlRequest)
	if err != nil || !proto.Equal(replay.GetResult(), final.GetResult()) {
		t.Fatalf("replayed recovery = %#v, err=%v", replay, err)
	}
	inspectedStatus, err := control.InspectFacetReplacement(ctx, controlRequest)
	if err != nil || !proto.Equal(inspectedStatus.GetResult(), final.GetResult()) {
		t.Fatalf("inspected status = %#v, err=%v", inspectedStatus, err)
	}

	switched, err := w.cores[0].Elements().ReadElement(w.actor, element, nil)
	if err != nil {
		t.Fatal(err)
	}
	if switched.Attachment.State != corecontract.FacetActive || switched.Attachment.Key.Facet != targetOwner ||
		switched.Attachment.Key.Element != element || switched.Attachment.NextHop != targetHop {
		t.Fatalf("switched attachment = %#v", switched.Attachment)
	}

	// The public Core read discloses the switched relationship; the public
	// target access is built only from that projection.
	caller, err := w.policy.TrustLocalCaller(w.actor)
	if err != nil {
		t.Fatal(err)
	}
	links := &composition.LocalCoreConnections{}
	if err = links.BindEndpoint("local:entry", composition.CoreEndpoint{Read: w.cores[0].Elements(), Public: w.cores[0].Access()}); err != nil {
		t.Fatal(err)
	}
	public := func(request map[string]any) map[string]any {
		t.Helper()
		raw, err := links.PublicCall(ctx, "local:entry", coreOwner, caller, wireJSON(t, request))
		body, callErr := decodeWire(t, raw, err)
		if callErr != nil {
			t.Fatal(callErr)
		}
		return body
	}
	entered := public(map[string]any{"op": "enter", "reference": map[string]any{"boundary": map[string]any{"core": string(coreOwner), "anchor": string(element)}, "nextHop": "local:entry"}, "intent": "read"})
	readReply := public(map[string]any{"op": "read", "handle": entered["handle"], "address": map[string]any{"down": []any{}}})
	attachment := readReply["attachment"].(map[string]any)
	if attachment["nextHop"] != string(targetHop) {
		t.Fatalf("public next hop = %#v", attachment["nextHop"])
	}
	accessKey := attachment["key"].(map[string]any)
	proposal := accessKey["proposal"].(map[string]any)
	proposalValue, err := values.DecodePublicBytes(proposal["value"].(string))
	if err != nil {
		t.Fatal(err)
	}
	targetAccess := &facetpb.Access{NextHop: proto.String(string(targetHop)), Key: &networkpb.AttachmentKey{Core: proto.String(accessKey["core"].(string)), Facet: proto.String(accessKey["facet"].(string)), Element: proto.String(accessKey["element"].(string)), Proposal: &networkpb.Token{Owner: proto.String(proposal["owner"].(string)), Kind: networkpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: proposalValue}}}

	// The target serves both moved entries publicly, byte for byte.
	targetClient := newFacetClient(targetOwner, targetHop)
	if err = targetClient.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	for _, expectedContent := range []struct {
		typ     values.FacetTypeRef
		payload []byte
	}{{w.typ, profilePayload}, {notes, notesPayload}} {
		got, err := targetClient.Facet().Get(ctx, &facetpb.GetRequest{Access: targetAccess, TypeRef: expectedContent.typ.Bytes()})
		if err != nil || got.GetResult() == nil || got.GetResult().GetState() != facetpb.Presence_PRESENT || !bytes.Equal(got.GetResult().GetPayload(), expectedContent.payload) {
			t.Fatalf("target content %q: %v %v", string(expectedContent.typ.Bytes()), got, err)
		}
	}

	// The fenced source refuses its former public access over the wire.
	sourceClient := newFacetClient(sourceOwner, sourceHop)
	if err = sourceClient.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	refused, err := sourceClient.Facet().Get(ctx, &facetpb.GetRequest{Access: f.access, TypeRef: w.typ.Bytes()})
	if refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("fenced source served content: %v %v", refused, err)
	}

	// Persist a second public reservation but leave it for recovery after the
	// three hosts restart. This empty namespace must carry a real zero-entry
	// manifest envelope and stable digest across the full network path.
	emptyInspection, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(replacementEmptyElement))})
	if err != nil || emptyInspection.GetResult() == nil || emptyInspection.GetResult().GetTarget() != string(sourceOwner) || emptyInspection.GetResult().GetNextHop() != string(sourceHop) {
		t.Fatalf("inspect empty source attachment: %v %v", emptyInspection, err)
	}
	emptySourceAccess := facetAccessFromInspection(t, coreOwner, emptyInspection.GetResult())
	const emptyOp corecontract.OperationID = "network-replacement-empty-recover-after-restart"
	emptyBegin, err := control.BeginFacetReplacement(ctx, &corepb.BeginFacetReplacementRequest{
		OperationId: []byte(emptyOp),
		Command: &corepb.PublicFacetReplacementCommand{
			Element: proto.String(string(replacementEmptyElement)), Expected: emptyInspection.GetResult().GetRevision(),
			Target: proto.String(string(targetOwner)), TargetAddress: proto.String(string(targetHop)), SourceAddress: proto.String(string(coreHop)),
		},
	})
	if err != nil || emptyBegin.GetResult() == nil || emptyBegin.GetResult().GetProgress() != uint32(corecontract.FacetReplacementReserved) {
		t.Fatalf("begin empty namespace replacement: %#v, err=%v", emptyBegin, err)
	}

	// Full restart of all three owners and hosts: the settled record, the
	// switched relationship and both durable fences must survive with
	// identical public projections.
	sourceServer.Stop()
	targetServer.Stop()
	coreServer.Stop()
	if err = w.facets[0].Close(); err != nil {
		t.Fatal(err)
	}
	if err = w.facets[1].Close(); err != nil {
		t.Fatal(err)
	}
	if err = w.cores[0].Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := core.OpenDurable(w.realmLinks, w.paths[0], corecontract.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.AuthorityRef() != coreOwner {
		t.Fatalf("reopened Core identity %q differs from %q", reopened.AuthorityRef(), coreOwner)
	}
	w.cores[0] = reopened
	sharedCoreClient := newCoreClient()
	reopenedSourceCore, err := composition.NewNetworkFacetCoreSource(sourceOwner, []composition.FacetCoreEndpoint{{Address: coreHop, Core: coreOwner, Client: sharedCoreClient, Credentials: fixedServiceCredentials{f.serviceEvidence}}})
	if err != nil {
		t.Fatal(err)
	}
	reopenedTargetCore, err := composition.NewNetworkFacetCoreSource(targetOwner, []composition.FacetCoreEndpoint{{Address: coreHop, Core: coreOwner, Client: sharedCoreClient, Credentials: fixedServiceCredentials{f.serviceEvidence}}})
	if err != nil {
		t.Fatal(err)
	}
	w.facets[0], err = facet.OpenDurable(w.realmLinks, reopenedSourceCore, w.paths[2], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	w.facets[1], err = facet.OpenDurable(w.realmLinks, reopenedTargetCore, w.paths[3], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if w.facets[0].AuthorityRef() != sourceOwner || w.facets[1].AuthorityRef() != targetOwner {
		t.Fatal("reopened Facet identities differ")
	}
	// The local composition links reach the reopened owners again.
	if err = w.facetLinks.BindEndpoint("local:facet0", w.facets[0]); err != nil {
		t.Fatal(err)
	}
	if err = w.facetLinks.BindEndpoint("local:facet1", w.facets[1]); err != nil {
		t.Fatal(err)
	}
	acceptanceSource, err := w.facetLinks.ForCore(w.service)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.cores[0].Facets().SetFacetAcceptanceSource(acceptanceSource); err != nil {
		t.Fatal(err)
	}

	// Fresh sessions and peer contacts; the previous ones died with the hosts.
	reopenedPeerSource, err := composition.NewNetworkCoreFacetSource(coreOwner, []composition.CoreFacetEndpoint{
		{Address: sourceHop, Facet: sourceOwner, Client: newFacetClient(sourceOwner, sourceHop), Credentials: fixedServiceCredentials{f.serviceEvidence}},
		{Address: targetHop, Facet: targetOwner, Client: newFacetClient(targetOwner, targetHop), Credentials: fixedServiceCredentials{f.serviceEvidence}},
	})
	if err != nil {
		t.Fatal(err)
	}
	coreConfig.FacetControl = w.cores[0].Facets()
	coreConfig.AttachmentControl = w.cores[0].Facets()
	coreConfig.ReplacementPeers = reopenedPeerSource.ForReplacement()
	reopenedCoreListener, err := net.Listen("tcp", coreAddress)
	if err != nil {
		t.Fatal(err)
	}
	reopenedCoreServer, err := coregrpc.New(w.cores[0].Access(), composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.cores[0].Access()}, coreConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopenedCoreServer.Stop)
	go reopenedCoreServer.Serve(reopenedCoreListener)
	reopenedSourceListener, err := net.Listen("tcp", sourceAddress)
	if err != nil {
		t.Fatal(err)
	}
	reopenedSourceServer, err := facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopenedSourceServer.Stop)
	go reopenedSourceServer.Serve(reopenedSourceListener)
	reopenedTargetListener, err := net.Listen("tcp", targetAddress)
	if err != nil {
		t.Fatal(err)
	}
	reopenedTargetServer, err := facetgrpc.New(w.facets[1].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[1].Namespace}, targetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopenedTargetServer.Stop)
	go reopenedTargetServer.Serve(reopenedTargetListener)

	// The reopened Core discloses the same settled control record, and the
	// reopened hosts keep both content decisions.
	reopenedAdmin := newCoreClient()
	if err = reopenedAdmin.Establish(ctx, f.admin.Audience, [16]byte(f.admin.ID), [32]byte(f.admin.Secret)); err != nil {
		t.Fatal(err)
	}
	emptyControlRequest := &corepb.ReplacementControlRequest{OperationId: []byte(emptyOp), Element: proto.String(string(replacementEmptyElement))}
	emptyFinal, err := reopenedAdmin.FacetControl().RecoverFacetReplacement(ctx, emptyControlRequest)
	if err != nil || emptyFinal.GetResult() == nil || emptyFinal.GetResult().GetProgress() != uint32(corecontract.FacetReplacementSettled) || !proto.Equal(emptyFinal.GetResult().GetRevision(), emptyBegin.GetResult().GetRevision()) {
		t.Fatalf("recover empty namespace replacement after restart: %#v, err=%v", emptyFinal, err)
	}
	persisted, err := reopenedAdmin.FacetControl().InspectFacetReplacement(ctx, controlRequest)
	if err != nil || !proto.Equal(persisted.GetResult(), final.GetResult()) {
		t.Fatalf("persisted status = %#v, err=%v", persisted, err)
	}
	retained, err := w.cores[0].Elements().ReadElement(w.actor, element, nil)
	if err != nil || retained.Attachment.State != corecontract.FacetActive || retained.Attachment.Key != switched.Attachment.Key || retained.Attachment.NextHop != targetHop {
		t.Fatalf("reopened attachment = %#v, err=%v", retained.Attachment, err)
	}
	servedTarget := newFacetClient(targetOwner, targetHop)
	if err = servedTarget.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	for _, expectedContent := range []struct {
		typ     values.FacetTypeRef
		payload []byte
	}{{w.typ, profilePayload}, {notes, notesPayload}} {
		got, err := servedTarget.Facet().Get(ctx, &facetpb.GetRequest{Access: targetAccess, TypeRef: expectedContent.typ.Bytes()})
		if err != nil || got.GetResult() == nil || !bytes.Equal(got.GetResult().GetPayload(), expectedContent.payload) {
			t.Fatalf("reopened target content %q: %v %v", string(expectedContent.typ.Bytes()), got, err)
		}
	}
	servedSource := newFacetClient(sourceOwner, sourceHop)
	if err = servedSource.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	refusedAgain, err := servedSource.Facet().Get(ctx, &facetpb.GetRequest{Access: f.access, TypeRef: w.typ.Bytes()})
	if refusedAgain.GetResult() != nil || (err == nil && refusedAgain.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("reopened fenced source served content: %v %v", refusedAgain, err)
	}
	refusedEmpty, err := servedSource.Facet().Get(ctx, &facetpb.GetRequest{Access: emptySourceAccess, TypeRef: emptyType.Bytes()})
	if refusedEmpty.GetResult() != nil || (err == nil && refusedEmpty.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("reopened fenced empty source served content: %v %v", refusedEmpty, err)
	}
	emptyTargetInspection, err := reopenedAdmin.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(replacementEmptyElement))})
	if err != nil || emptyTargetInspection.GetResult() == nil || emptyTargetInspection.GetResult().GetTarget() != string(targetOwner) || emptyTargetInspection.GetResult().GetNextHop() != string(targetHop) {
		t.Fatalf("inspect installed empty target: %v %v", emptyTargetInspection, err)
	}
	emptyTargetAccess := facetAccessFromInspection(t, coreOwner, emptyTargetInspection.GetResult())
	emptyTarget, err := servedTarget.Facet().Get(ctx, &facetpb.GetRequest{Access: emptyTargetAccess, TypeRef: emptyType.Bytes()})
	if err != nil || emptyTarget.GetResult() == nil || emptyTarget.GetResult().GetState() != facetpb.Presence_NOT_FOUND {
		t.Fatalf("read installed empty target: %v %v", emptyTarget, err)
	}
	emptyTypes, err := servedTarget.Facet().ListTypes(ctx, &facetpb.ListTypesRequest{Access: emptyTargetAccess})
	if err != nil || emptyTypes.GetResult() == nil || len(emptyTypes.GetResult().GetTypes()) != 0 {
		t.Fatalf("list installed empty target: %v %v", emptyTypes, err)
	}
	replacementPeers := reopenedPeerSource.ForReplacement()
	emptySourceState, err := replacementPeers.Source.CheckSourceExport(ctx, sourceHop, emptyKey, emptyOp)
	if err != nil || !emptySourceState.Found || !emptySourceState.Fenced || !emptySourceState.Settled || emptySourceState.FinalCut == 0 || emptySourceState.ManifestDigest != sha256.Sum256(nil) || len(emptySourceState.Manifest) == 0 {
		t.Fatalf("settled empty source evidence: %#v, err=%v", emptySourceState, err)
	}
	emptyManifest, err := facetcontract.DecodeNamespaceManifest(emptySourceState.Manifest)
	if err != nil || emptyManifest.Element != replacementEmptyElement || emptyManifest.Source != sourceOwner || emptyManifest.FinalCut != emptySourceState.FinalCut || len(emptyManifest.Entries) != 0 {
		t.Fatalf("decoded empty network manifest = %#v, err=%v", emptyManifest, err)
	}
	// Take the exact target acceptance key from the reopened owner's ordinary
	// attachment projection; no caller-supplied source lineage is trusted.
	emptyCurrent, err := w.cores[0].Elements().ReadElement(w.actor, replacementEmptyElement, nil)
	if err != nil || emptyCurrent.Attachment.State != corecontract.FacetActive || emptyCurrent.Attachment.Key.Facet != targetOwner {
		t.Fatalf("reopened empty target attachment = %#v, err=%v", emptyCurrent.Attachment, err)
	}
	targetKey := emptyCurrent.Attachment.Key
	serviceCaller, err := w.policy.TrustLocalCaller(w.service)
	if err != nil {
		t.Fatal(err)
	}
	emptyImport, err := w.facets[1].Namespace.ReadNamespaceImport(ctx, serviceCaller, targetKey, facetcontract.OperationID(emptyOp))
	if err != nil || !emptyImport.Installed || emptyImport.Accepted || emptyImport.FinalCut != emptySourceState.FinalCut || emptyImport.Digest != emptySourceState.ManifestDigest || len(emptyImport.Live) != 0 {
		t.Fatalf("installed empty network target evidence = %#v, err=%v", emptyImport, err)
	}
}
