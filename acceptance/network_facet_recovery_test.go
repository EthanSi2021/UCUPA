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
	coresdk "ucupa/ucupa-core/sdk/go"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	facetsdk "ucupa/ucupa-facet/sdk/go"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
)

const networkRecoveryEntity values.ElementID = "00000000-0000-4000-8000-00000000fa71"

// TestFacetNetworkRecoveryRebuildsSameFacet detaches one attached element's
// relationship on its own TLS Core host while a single Facet keeps the payload
// and its acceptance lineage. The recovery then crosses the public control
// plane: the proposal resolution travels over the wire between the Core host
// and the Facet host, a wrong Core address and the pre-detach proposal are
// both refused, and the rebuilt state, the superseded old key and the payload
// all survive a full Facet host restart with identical public projections.
func TestFacetNetworkRecoveryRebuildsSameFacet(t *testing.T) {
	f := newFacetNetworkFixture(t)
	w := f.w
	ctx := context.Background()
	owner, otherCore, holderOwner := w.cores[0].AuthorityRef(), w.cores[1].AuthorityRef(), w.facets[0].AuthorityRef()

	// The recovering element is a child of the public root, attached to the
	// one holder Facet, with one payload entry written before any host change.
	w.grant(t, w.actor, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, publicRoot))
	if _, err := w.cores[0].Elements().CreateEntity(w.actor, networkRecoveryEntity, corecontract.SpatialTuple{
		Parent: publicRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "network-recovery-create"); err != nil {
		t.Fatal(err)
	}
	oldKey := w.attach(t, 0, 0, networkRecoveryEntity)
	notesPayload := fixturePayload("network-recovery-notes")
	if _, err := w.facets[0].Namespace.Put(ctx, w.actor, oldKey, w.typ, notesPayload, facetcontract.ExpectAbsent(), "network-recovery-put"); err != nil {
		t.Fatal(err)
	}
	// The recovery authorization mirrors the local chain: the actor's control
	// session reads the public proposal disclosure of its own Core.
	w.grant(t, w.actor, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, networkRecoveryEntity))

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

	// The reopened Facet reaches the owner Core only at its network host; the
	// local composition links keep serving the activation acceptance read.
	source, err := composition.NewNetworkFacetCoreSource(holderOwner, []composition.FacetCoreEndpoint{
		{Address: coreHopA, Core: owner, Client: newCoreClient(owner, coreHopA), Credentials: fixedServiceCredentials{f.serviceEvidence}},
		{Address: coreHopB, Core: otherCore, Client: newCoreClient(otherCore, coreHopB), Credentials: fixedServiceCredentials{f.serviceEvidence}},
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

	// The durable contact names the network Core host.
	acceptance, err := w.facets[0].Namespace.ReadAcceptance(w.actor, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.facets[0].Namespace.UpdateCoreNextHop(w.actor, "network-recovery-contact", oldKey, acceptance.NextHopRevision, coreHopA); err != nil {
		t.Fatal(err)
	}

	// The active relationship's current public proposal is captured, then the
	// owner Core's own control plane discloses the local detach.
	admin := newCoreClient(owner, coreHopA)
	if err = admin.Establish(ctx, f.admin.Audience, [16]byte(f.admin.ID), [32]byte(f.admin.Secret)); err != nil {
		t.Fatal(err)
	}
	control := admin.FacetControl()
	inspected, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRecoveryEntity))})
	if err != nil || inspected.GetResult() == nil || inspected.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		inspected.GetResult().GetTarget() != string(holderOwner) || inspected.GetResult().GetProposal() == nil {
		t.Fatalf("inspect active attachment: %v %v", inspected, err)
	}
	oldProposal := inspected.GetResult().GetProposal()
	detach := &corepb.ApplyFacetRequest{OperationId: []byte("network-recovery-detach"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("detach"), Element: proto.String(string(networkRecoveryEntity)), Expected: inspected.GetResult().GetRevision(),
	}}
	detached, err := control.ApplyFacet(ctx, detach)
	if err != nil || detached.GetResult() == nil || detached.GetResult().GetState().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("detach: %v %v", detached, err)
	}

	client := newFacetClient()
	if err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	oldAccess := &facetpb.Access{NextHop: proto.String(string(facetHop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(string(owner)), Facet: proto.String(string(holderOwner)), Element: proto.String(string(networkRecoveryEntity)), Proposal: oldProposal,
	}}

	// The revoked relationship's public read is refused over the wire as a
	// recorded detached decision; the payload records stay unreachable.
	refused, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: oldAccess, TypeRef: w.typ.Bytes()})
	if refused.GetResult() != nil || (err == nil && refused.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("detached content still readable: %v %v", refused, err)
	}

	// The explicit recovery proposal is created on the same Core's public
	// control plane and disclosed as the current pending proposal.
	inspectedDetached, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRecoveryEntity))})
	if err != nil || inspectedDetached.GetResult() == nil || inspectedDetached.GetResult().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("inspect detached attachment: %v %v", inspectedDetached, err)
	}
	repropose := &corepb.ApplyFacetRequest{OperationId: []byte("network-recovery-repropose"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(string(networkRecoveryEntity)), Expected: inspectedDetached.GetResult().GetRevision(),
		Target: proto.String(string(holderOwner)), NextHop: proto.String("local:facet0"),
	}}
	reproposed, err := control.ApplyFacet(ctx, repropose)
	if err != nil || reproposed.GetResult() == nil || reproposed.GetResult().GetState().GetPendingProposal() == nil ||
		reproposed.GetResult().GetState().GetPendingTarget() != string(holderOwner) {
		t.Fatalf("recovery propose: %v %v", reproposed, err)
	}
	newRef := &facetpb.PublicAcceptanceRef{Core: proto.String(string(owner)), Element: proto.String(string(networkRecoveryEntity)), Proposal: reproposed.GetResult().GetState().GetPendingProposal()}
	accept := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(coreHopA)), OperationId: []byte("network-recovery-accept")}
	relationships := client.Relationships()

	// A request naming the owner Core but routed to the other Core's address
	// is refused: no peer may answer for another Core's relationship.
	wrongIdentity := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(coreHopB)), OperationId: []byte("network-recovery-wrong-identity")}
	wrong, err := relationships.AcceptAttachment(ctx, wrongIdentity)
	if wrong.GetResult() != nil || (err == nil && wrong.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("wrong identity accepted: %v %v", wrong, err)
	}

	// The same Facet re-accepts through the recovery proposal over the wire:
	// the resolve crosses to the owner Core — no successor statement is read
	// because the Core's own DETACHED disclosure is the revocation evidence —
	// and the payload never moves.
	accepted, err := relationships.AcceptAttachment(ctx, accept)
	if err != nil || accepted.GetResult() == nil || accepted.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		!accepted.GetResult().GetInspection().GetFound() || accepted.GetResult().GetInspection().GetCoreNextHop() != string(coreHopA) {
		t.Fatalf("recovery acceptance: %v %v", accepted, err)
	}
	replayed, err := relationships.AcceptAttachment(ctx, accept)
	if err != nil || !proto.Equal(replayed, accepted) {
		t.Fatalf("replayed recovery = %#v, err=%v", replayed, err)
	}

	// The pre-detach proposal is refused persistently over the wire; the
	// Core's pending proposal is the recovery one.
	staleProposal := &facetpb.AcceptAttachmentRequest{
		Ref:         &facetpb.PublicAcceptanceRef{Core: proto.String(string(owner)), Element: proto.String(string(networkRecoveryEntity)), Proposal: oldProposal},
		CoreNextHop: proto.String(string(coreHopA)), OperationId: []byte("network-recovery-stale-proposal"),
	}
	stale, err := relationships.AcceptAttachment(ctx, staleProposal)
	if stale.GetResult() != nil || (err == nil && stale.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("stale proposal accepted: %v %v", stale, err)
	}

	// Activation crosses the owner Core's public control plane; the
	// acceptance read resolves the local facet composition link.
	activate := &corepb.ApplyFacetRequest{OperationId: []byte("network-recovery-activate"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("activate"), Element: proto.String(string(networkRecoveryEntity)), Expected: reproposed.GetResult().GetState().GetRevision(),
	}}
	activated, err := control.ApplyFacet(ctx, activate)
	if err != nil || activated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("activate: %v %v", activated, err)
	}

	// The public Core read discloses the rebuilt relationship; the public
	// access is built only from that projection.
	publicRead := publicCoreRead(t, w.cores[0], w.caller, publicRoot, networkRecoveryEntity)
	attachment := publicRead["attachment"].(map[string]any)
	if attachment["state"] != "ACTIVE" {
		t.Fatalf("public attachment state = %#v", attachment["state"])
	}
	accessKey := attachment["key"].(map[string]any)
	if accessKey["core"] != string(owner) || accessKey["facet"] != string(holderOwner) || accessKey["element"] != string(networkRecoveryEntity) {
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

	// The superseded old key no longer serves through the same public host.
	superseded, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: oldAccess, TypeRef: w.typ.Bytes()})
	if superseded.GetResult() != nil || (err == nil && superseded.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("superseded key served content: %v %v", superseded, err)
	}

	// The settled owners show one ACTIVE attachment with the unique new
	// lineage; the previous lineage is gone from the facet journal.
	finalRead, err := w.cores[0].Elements().ReadElement(w.actor, networkRecoveryEntity, nil)
	if err != nil || finalRead.Attachment.State != corecontract.FacetActive || finalRead.Attachment.Key.Facet != holderOwner ||
		finalRead.Attachment.Key.Element != networkRecoveryEntity {
		t.Fatalf("activated attachment = %#v, err=%v", finalRead.Attachment, err)
	}
	rebound, err := w.facets[0].Namespace.ReadAcceptance(w.actor, finalRead.Attachment.Key)
	if err != nil || !rebound.Found || rebound.CoreNextHop != coreHopA {
		t.Fatalf("rebound lineage = %#v, err=%v", rebound, err)
	}
	previous, err := w.facets[0].Namespace.ReadAcceptance(w.actor, oldKey)
	if err != nil || previous.Found {
		t.Fatalf("previous lineage kept: %#v, err=%v", previous, err)
	}

	// A full Facet host restart replays the same recovery and keeps serving
	// the same payload; the Core host keeps its record.
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
		t.Fatalf("replayed recovery after restart = %#v, err=%v", recovered, err)
	}
	after, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: w.typ.Bytes()})
	if err != nil || after.GetResult() == nil || !bytes.Equal(after.GetResult().GetPayload(), notesPayload) {
		t.Fatalf("payload after restart: %v %v", after, err)
	}
}
