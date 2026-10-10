package acceptance_test

import (
	"bytes"
	"context"
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
	localtransition "ucupa/ucupa-core/transport/local/transition"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	facetsdk "ucupa/ucupa-facet/sdk/go"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
)

const (
	reacceptanceTargetRoot values.ElementID = "00000000-0000-4000-8000-00000000fa61"
	reacceptanceEntity     values.ElementID = "00000000-0000-4000-8000-00000000fa62"
)

func publicCoreRead(t *testing.T, c *core.Core, caller values.CallerContext, anchor, id values.ElementID) map[string]any {
	t.Helper()
	links := &composition.LocalCoreConnections{}
	if err := links.BindEndpoint("local:entry", composition.CoreEndpoint{Read: c.Elements(), Public: c.Access()}); err != nil {
		t.Fatal(err)
	}
	call := func(request map[string]any) map[string]any {
		raw, err := links.PublicCall(context.Background(), "local:entry", c.AuthorityRef(), caller, wireJSON(t, request))
		reply, callErr := decodeWire(t, raw, err)
		if callErr != nil {
			t.Fatal(callErr)
		}
		return reply
	}
	entered := call(map[string]any{"op": "enter", "reference": map[string]any{"boundary": map[string]any{"core": string(c.AuthorityRef()), "anchor": string(anchor)}, "nextHop": "local:entry"}, "intent": "read"})
	return call(map[string]any{"op": "read", "handle": entered["handle"], "address": map[string]any{"down": []any{string(id)}}})
}

func publicRevisionToken(t *testing.T, reply map[string]any) values.PublicToken {
	t.Helper()
	v := reply["record"].(map[string]any)["revision"].(map[string]any)
	return values.PublicToken{Owner: values.FactAuthorityRef(v["owner"].(string)), Kind: v["kind"].(string), Value: v["value"].(string)}
}

// TestFacetNetworkReacceptanceRebindsSameFacet moves one attached element from
// one TLS Core host to a second one while a single Facet keeps the payload and
// its acceptance lineage. The re-acceptance then crosses the public control
// plane: the proposal resolution and the predecessor-successor proof both
// travel over the wire between the two Core hosts and the Facet host, a wrong
// Core identity and the departed Core's stale proposal are both refused, and
// the rebound state, the superseded old key and the payload all survive a full
// Facet host restart with identical public projections.
func TestFacetNetworkReacceptanceRebindsSameFacet(t *testing.T) {
	f := newFacetNetworkFixture(t)
	w := f.w
	ctx := context.Background()
	ownerA, ownerB, holderOwner := w.cores[0].AuthorityRef(), w.cores[1].AuthorityRef(), w.facets[0].AuthorityRef()

	// The moving element is a child of the public root, attached to the one
	// holder Facet, with one payload entry written before any host changes.
	w.grant(t, w.actor, corecontract.PermissionCreateEntity, values.CoreElementScope(ownerA, publicRoot))
	if _, err := w.cores[0].Elements().CreateEntity(w.actor, reacceptanceEntity, corecontract.SpatialTuple{
		Parent: publicRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "network-reacceptance-create"); err != nil {
		t.Fatal(err)
	}
	oldKey := w.attach(t, 0, 0, reacceptanceEntity)
	notesPayload := fixturePayload("network-reacceptance-notes")
	if _, err := w.facets[0].Namespace.Put(ctx, w.actor, oldKey, w.typ, notesPayload, facetcontract.ExpectAbsent(), "network-reacceptance-put"); err != nil {
		t.Fatal(err)
	}

	// The transfer and re-acceptance authorization mirrors the local chain.
	w.root(t, 1, reacceptanceTargetRoot)
	for _, p := range []values.PermissionID{corecontract.PermissionTransitionExport, corecontract.PermissionTransitionCommit, corecontract.PermissionTransitionInspect, corecontract.PermissionTransitionImport, corecontract.PermissionFacetProposalRead} {
		w.grant(t, w.actor, p, values.CoreElementScope(ownerA, reacceptanceEntity))
	}
	w.grant(t, w.actor, corecontract.PermissionPlaceElement, values.CoreElementScope(ownerA, publicRoot))
	for _, p := range []values.PermissionID{corecontract.PermissionTransitionExport, corecontract.PermissionTransitionCommit, corecontract.PermissionTransitionInspect, corecontract.PermissionTransitionImport} {
		w.grant(t, w.actor, p, values.CoreElementScope(ownerB, reacceptanceEntity))
	}
	w.grant(t, w.actor, corecontract.PermissionReadElement, values.AnchorClosureScope(ownerB, reacceptanceTargetRoot))
	w.grant(t, w.actor, corecontract.PermissionReadElement, values.CoreElementScope(ownerB, reacceptanceEntity))
	w.grant(t, w.actor, corecontract.PermissionPlaceElement, values.CoreElementScope(ownerB, reacceptanceTargetRoot))
	for _, p := range []values.PermissionID{corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetProposalRead} {
		w.grant(t, w.actor, p, values.CoreElementScope(ownerB, reacceptanceEntity))
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead} {
		w.grant(t, w.service, p, values.CoreElementScope(ownerB, reacceptanceEntity))
	}

	// The Facet host keeps its address while both Cores start TLS hosts for
	// the fixed Facet service and the actor's control session.
	cert, roots, pin, _, _ := networkCertificate(t)
	f.server.Stop()
	if err := w.facets[0].Close(); err != nil {
		t.Fatal(err)
	}
	facetAddress, _ := f.config.NextHop.NetworkTarget()
	facetListener, err := net.Listen("tcp", facetAddress)
	if err != nil {
		t.Fatal(err)
	}
	coreAListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	coreBListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	facetHop, coreHopA, coreHopB := f.config.NextHop, values.NextHop("https://"+coreAListener.Addr().String()), values.NextHop("https://"+coreBListener.Addr().String())
	newCoreClient := func(expected values.FactAuthorityRef, hop values.NextHop) *coresdk.Client {
		c, err := coresdk.New(coresdk.Config{Expected: expected, NextHop: hop, Roots: roots, SPKISHA256: pin, RPCDeadline: 3 * time.Second, ConnectTimeout: time.Second, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	newFacetClient := func() *facetsdk.Client {
		c, err := facetsdk.New(facetsdk.Config{Expected: holderOwner, NextHop: facetHop, Roots: roots, SPKISHA256: pin, RPCDeadline: 3 * time.Second, ConnectTimeout: time.Second, MaxResponseBytes: 4 << 20, MaxMetadataBytes: 8192})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	coreHost := func(i int, hop values.NextHop, l net.Listener) {
		owner := w.cores[i].AuthorityRef()
		config := coregrpc.Config{Expected: owner, NextHop: hop, Certificate: cert, SessionTTL: time.Minute, RPCDeadline: 3 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20,
			FacetServices:     map[values.PrincipalRef]values.FactAuthorityRef{w.service: holderOwner},
			FacetControl:      w.cores[i].Facets(),
			AttachmentControl: w.cores[i].Facets(),
		}
		server, err := coregrpc.New(w.cores[i].Access(), composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.cores[i].Access()}, config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(server.Stop)
		go server.Serve(l)
	}

	// The reopened Facet reaches both Cores only at their network hosts; the
	// local composition links keep serving the activation acceptance read.
	source, err := composition.NewNetworkFacetCoreSource(holderOwner, []composition.FacetCoreEndpoint{
		{Address: coreHopA, Core: ownerA, Client: newCoreClient(ownerA, coreHopA), Credentials: fixedServiceCredentials{f.serviceEvidence}},
		{Address: coreHopB, Core: ownerB, Client: newCoreClient(ownerB, coreHopB), Credentials: fixedServiceCredentials{f.serviceEvidence}},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.facets[0], err = facet.OpenDurable(w.realmLinks, source, w.paths[2], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if w.facets[0].AuthorityRef() != holderOwner {
		t.Fatal("reopened Facet identity differs")
	}
	if err = w.facetLinks.BindEndpoint("local:facet0", w.facets[0]); err != nil {
		t.Fatal(err)
	}
	facetConfig := f.config
	facetConfig.Certificate = cert
	facetServer, err := facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, facetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(facetServer.Stop)
	go facetServer.Serve(facetListener)
	coreHost(0, coreHopA, coreAListener)
	coreHost(1, coreHopB, coreBListener)

	// The durable contacts now name the network Core hosts.
	acceptance, err := w.facets[0].Namespace.ReadAcceptance(w.actor, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.facets[0].Namespace.UpdateCoreNextHop(w.actor, "network-reacceptance-contact", oldKey, acceptance.NextHopRevision, coreHopA); err != nil {
		t.Fatal(err)
	}

	// The departed Core's current public proposal is captured while the
	// element is still attached there.
	adminA := newCoreClient(ownerA, coreHopA)
	if err = adminA.Establish(ctx, f.admin.Audience, [16]byte(f.admin.ID), [32]byte(f.admin.Secret)); err != nil {
		t.Fatal(err)
	}
	inspectedA, err := adminA.FacetControl().InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(reacceptanceEntity))})
	if err != nil || inspectedA.GetResult() == nil || inspectedA.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		inspectedA.GetResult().GetTarget() != string(holderOwner) || inspectedA.GetResult().GetProposal() == nil {
		t.Fatalf("inspect source attachment: %v %v", inspectedA, err)
	}
	oldProposal := inspectedA.GetResult().GetProposal()

	// The element, with its facet attachment record, moves to Core B.
	command := corecontract.TransitionCommand{
		Kind: corecontract.TransitionSlice, Target: ownerB, Boundary: reacceptanceEntity,
		Expected:      publicRevisionToken(t, publicCoreRead(t, w.cores[0], w.caller, publicRoot, reacceptanceEntity)),
		Final:         corecontract.SpatialTuple{Parent: reacceptanceTargetRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1)},
		SourceAddress: "local:core0", TargetAddress: "local:core1",
	}
	binding0 := corecontract.TransitionApprovalBinding{Source: ownerA, Operation: "network-reacceptance-transfer", Command: command}
	if _, err = w.cores[0].Transfers().BeginTransition(ctx, w.caller, binding0.Operation, binding0.Command); err != nil {
		t.Fatal(err)
	}
	view, err := w.cores[1].Transfers().AuthorizeTransitionImport(ctx, w.caller, binding0)
	if err != nil {
		t.Fatal(err)
	}
	link, err := localtransition.New(w.cores[0].Transfers(), w.cores[1].Transfers(), binding0, w.caller, view)
	if err != nil {
		t.Fatal(err)
	}
	done, err := w.cores[0].Transfers().RecoverTransition(ctx, w.caller, binding0, link)
	if err != nil || done.Progress != corecontract.TransitionSettled {
		t.Fatalf("transfer = %+v, %v", done, err)
	}
	transferred, err := w.cores[1].Elements().ReadElement(w.actor, reacceptanceEntity, nil)
	if err != nil || transferred.Attachment.State != corecontract.FacetDetached ||
		transferred.Attachment.Key.Core != ownerA || transferred.Attachment.Key.Facet != holderOwner {
		t.Fatalf("transferred attachment = %#v, err=%v", transferred.Attachment, err)
	}
	if _, err = w.cores[0].Elements().ReadElement(w.actor, reacceptanceEntity, nil); err == nil {
		t.Fatal("departed source still serves the element")
	}

	// The explicit re-acceptance proposal is created on Core B's public
	// control plane and disclosed as the current pending proposal.
	adminB := newCoreClient(ownerB, coreHopB)
	if err = adminB.Establish(ctx, f.admin.Audience, [16]byte(f.admin.ID), [32]byte(f.admin.Secret)); err != nil {
		t.Fatal(err)
	}
	controlB := adminB.FacetControl()
	inspectedB, err := controlB.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(reacceptanceEntity))})
	if err != nil || inspectedB.GetResult() == nil || inspectedB.GetResult().GetState() != corepb.AttachmentState_DETACHED ||
		inspectedB.GetResult().GetKeyCore() != string(ownerA) {
		t.Fatalf("inspect transferred attachment: %v %v", inspectedB, err)
	}
	repropose := &corepb.ApplyFacetRequest{OperationId: []byte("network-reacceptance-repropose"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(string(reacceptanceEntity)), Expected: inspectedB.GetResult().GetRevision(),
		Target: proto.String(string(holderOwner)), NextHop: proto.String("local:facet0"),
	}}
	reproposed, err := controlB.ApplyFacet(ctx, repropose)
	if err != nil || reproposed.GetResult() == nil || reproposed.GetResult().GetState().GetPendingProposal() == nil ||
		reproposed.GetResult().GetState().GetPendingTarget() != string(holderOwner) {
		t.Fatalf("re-propose: %v %v", reproposed, err)
	}
	newRef := &facetpb.PublicAcceptanceRef{Core: proto.String(string(ownerB)), Element: proto.String(string(reacceptanceEntity)), Proposal: reproposed.GetResult().GetState().GetPendingProposal()}
	accept := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(coreHopB)), OperationId: []byte("network-reacceptance-accept")}

	client := newFacetClient()
	if err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	relationships := client.Relationships()

	// A request naming Core B but routed to Core A's address is refused: no
	// peer may answer for another Core's identity.
	wrongIdentity := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(coreHopA)), OperationId: []byte("network-reacceptance-wrong-identity")}
	refused, err := relationships.AcceptAttachment(ctx, wrongIdentity)
	if refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("wrong identity accepted: %v %v", refused, err)
	}

	// The same Facet re-accepts through Core B's proposal over the wire: the
	// resolve crosses to Core B, the successor proof crosses back to the
	// departed Core A, and the payload never moves.
	accepted, err := relationships.AcceptAttachment(ctx, accept)
	if err != nil || accepted.GetResult() == nil || accepted.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		!accepted.GetResult().GetInspection().GetFound() || accepted.GetResult().GetInspection().GetCoreNextHop() != string(coreHopB) {
		t.Fatalf("re-acceptance: %v %v", accepted, err)
	}
	replayed, err := relationships.AcceptAttachment(ctx, accept)
	if err != nil || !proto.Equal(replayed, accepted) {
		t.Fatalf("replayed re-acceptance = %#v, err=%v", replayed, err)
	}

	// The departed Core's stale proposal is refused persistently over the
	// wire; its fenced record proves nothing after the settled transfer.
	staleProposal := &facetpb.AcceptAttachmentRequest{
		Ref:         &facetpb.PublicAcceptanceRef{Core: proto.String(string(ownerA)), Element: proto.String(string(reacceptanceEntity)), Proposal: oldProposal},
		CoreNextHop: proto.String(string(coreHopA)), OperationId: []byte("network-reacceptance-stale-proposal"),
	}
	stale, err := relationships.AcceptAttachment(ctx, staleProposal)
	if stale.GetResult() != nil || (err == nil && stale.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("stale proposal accepted: %v %v", stale, err)
	}

	// Activation crosses Core B's public control plane; the acceptance read
	// resolves the local facet composition link.
	activate := &corepb.ApplyFacetRequest{OperationId: []byte("network-reacceptance-activate"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("activate"), Element: proto.String(string(reacceptanceEntity)), Expected: reproposed.GetResult().GetState().GetRevision(),
	}}
	activated, err := controlB.ApplyFacet(ctx, activate)
	if err != nil || activated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("activate: %v %v", activated, err)
	}

	// The public Core read discloses the rebound relationship; the public
	// access is built only from that projection.
	publicRead := publicCoreRead(t, w.cores[1], w.caller, reacceptanceTargetRoot, reacceptanceEntity)
	attachment := publicRead["attachment"].(map[string]any)
	if attachment["state"] != "ACTIVE" {
		t.Fatalf("public attachment state = %#v", attachment["state"])
	}
	accessKey := attachment["key"].(map[string]any)
	if accessKey["core"] != string(ownerB) || accessKey["facet"] != string(holderOwner) || accessKey["element"] != string(reacceptanceEntity) {
		t.Fatalf("public attachment key = %#v", accessKey)
	}
	proposal := accessKey["proposal"].(map[string]any)
	proposalValue, err := values.DecodePublicBytes(proposal["value"].(string))
	if err != nil {
		t.Fatal(err)
	}
	newAccess := &facetpb.Access{NextHop: proto.String(string(facetHop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(accessKey["core"].(string)), Facet: proto.String(accessKey["facet"].(string)), Element: proto.String(accessKey["element"].(string)),
		Proposal: &networkpb.Token{Owner: proto.String(proposal["owner"].(string)), Kind: networkpb.TokenKind_ATTACHMENT_REVISION.Enum(), Value: proposalValue},
	}}
	served, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: w.typ.Bytes()})
	if err != nil || served.GetResult() == nil || !bytes.Equal(served.GetResult().GetPayload(), notesPayload) {
		t.Fatalf("rebound payload: %v %v", served, err)
	}

	// The superseded old key no longer writes through the same public host.
	oldAccess := &facetpb.Access{NextHop: proto.String(string(facetHop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(string(ownerA)), Facet: proto.String(string(holderOwner)), Element: proto.String(string(reacceptanceEntity)), Proposal: oldProposal,
	}}
	superseded, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: oldAccess, TypeRef: w.typ.Bytes()})
	if superseded.GetResult() != nil || (err == nil && superseded.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("superseded key served content: %v %v", superseded, err)
	}

	// The settled owners show one ACTIVE attachment with the unique new
	// lineage; the previous lineage is gone from the facet journal.
	finalRead, err := w.cores[1].Elements().ReadElement(w.actor, reacceptanceEntity, nil)
	if err != nil || finalRead.Attachment.State != corecontract.FacetActive || finalRead.Attachment.Key.Core != ownerB ||
		finalRead.Attachment.Key.Facet != holderOwner || finalRead.Attachment.Key.Element != reacceptanceEntity {
		t.Fatalf("activated attachment = %#v, err=%v", finalRead.Attachment, err)
	}
	rebound, err := w.facets[0].Namespace.ReadAcceptance(w.actor, finalRead.Attachment.Key)
	if err != nil || !rebound.Found || rebound.CoreNextHop != coreHopB {
		t.Fatalf("rebound lineage = %#v, err=%v", rebound, err)
	}
	previous, err := w.facets[0].Namespace.ReadAcceptance(w.actor, oldKey)
	if err != nil || previous.Found {
		t.Fatalf("previous lineage kept: %#v, err=%v", previous, err)
	}

	// A full Facet host and owner restart replays the same re-acceptance and
	// keeps serving the same payload; the Core hosts keep their records.
	facetServer.Stop()
	if err = w.facets[0].Close(); err != nil {
		t.Fatal(err)
	}
	w.facets[0], err = facet.OpenDurable(w.realmLinks, source, w.paths[2], facet.Config{PublicAccessPolicy: publicPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	if w.facets[0].AuthorityRef() != holderOwner {
		t.Fatal("reopened Facet identity differs")
	}
	if err = w.facetLinks.BindEndpoint("local:facet0", w.facets[0]); err != nil {
		t.Fatal(err)
	}
	reopenedListener, err := net.Listen("tcp", facetAddress)
	if err != nil {
		t.Fatal(err)
	}
	reopenedServer, err := facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, facetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopenedServer.Stop)
	go reopenedServer.Serve(reopenedListener)
	if err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	recovered, err := client.Relationships().AcceptAttachment(ctx, accept)
	if err != nil || !proto.Equal(recovered, accepted) {
		t.Fatalf("replayed re-acceptance after restart = %#v, err=%v", recovered, err)
	}
	after, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: w.typ.Bytes()})
	if err != nil || after.GetResult() == nil || !bytes.Equal(after.GetResult().GetPayload(), notesPayload) {
		t.Fatalf("payload after restart: %v %v", after, err)
	}
}
