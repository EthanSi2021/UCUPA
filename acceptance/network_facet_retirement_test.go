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

const networkRetirementEntity values.ElementID = "00000000-0000-4000-8000-00000000fa72"

// TestFacetNetworkRetirementReleasesControl disposes of one detached
// element's content through the Facet host's public relationship plane, then
// releases the controlling Core's occupancy through its public control plane
// while the final-disposition evidence read crosses the TLS link between the
// two hosts. A release without the recorded disposition and one routed to an
// address without a disposition endpoint are both refused safely, the retired
// namespace never serves again, and the committed decisions replay their exact
// results across full Facet host restarts.
func TestFacetNetworkRetirementReleasesControl(t *testing.T) {
	f := newFacetNetworkFixture(t)
	w := f.w
	ctx := context.Background()
	owner, holderOwner := w.cores[0].AuthorityRef(), w.facets[0].AuthorityRef()

	// The retiring element is a child of the public root, attached to the one
	// holder Facet, with one payload entry written before any host change.
	w.grant(t, w.actor, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, publicRoot))
	if _, err := w.cores[0].Elements().CreateEntity(w.actor, networkRetirementEntity, corecontract.SpatialTuple{
		Parent: publicRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
	}, "network-retirement-create"); err != nil {
		t.Fatal(err)
	}
	oldKey := w.attach(t, 0, 0, networkRetirementEntity)
	notesPayload := fixturePayload("network-retirement-notes")
	if _, err := w.facets[0].Namespace.Put(ctx, w.actor, oldKey, w.typ, notesPayload, facetcontract.ExpectAbsent(), "network-retirement-put"); err != nil {
		t.Fatal(err)
	}
	// The retirement authorization mirrors the local chain: the actor's control
	// session reads its own Core's disclosure and releases the occupancy, while
	// the same actor's Facet session disposes of the held content.
	w.grant(t, w.actor, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, networkRetirementEntity))
	w.grant(t, w.actor, corecontract.PermissionFacetRelease, values.CoreElementScope(owner, networkRetirementEntity))
	w.grant(t, w.actor, facetcontract.PermissionRetireContent, values.FacetElementScope(holderOwner, networkRetirementEntity))

	// The Facet host keeps its address while the owner Core starts a TLS host
	// for the fixed Facet service and the actor's control session.
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
	facetHop, coreHopA := f.config.NextHop, values.NextHop("https://"+coreAListener.Addr().String())
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
	coreConfig := coregrpc.Config{Expected: owner, NextHop: coreHopA, Certificate: cert, SessionTTL: time.Minute, RPCDeadline: 3 * time.Second, HandshakeTimeout: time.Second, ConnectionLifetime: time.Minute, ShutdownTimeout: time.Second, MaxSessions: 8, MaxSessionsPerCaller: 4, MaxConnections: 8, MaxConcurrentRPCs: 8, MaxStreams: 8, MaxMetadataBytes: 8192, MaxResponseBytes: 4 << 20,
		FacetServices:     map[values.PrincipalRef]values.FactAuthorityRef{w.service: holderOwner},
		FacetControl:      w.cores[0].Facets(),
		AttachmentControl: w.cores[0].Facets(),
	}
	coreServer, err := coregrpc.New(w.cores[0].Access(), composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.cores[0].Access()}, coreConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(coreServer.Stop)
	go coreServer.Serve(coreAListener)

	// The reopened Facet reaches the owner Core only at its network host; the
	// local composition links keep serving the activation acceptance read.
	source, err := composition.NewNetworkFacetCoreSource(holderOwner, []composition.FacetCoreEndpoint{
		{Address: coreHopA, Core: owner, Client: newCoreClient(owner, coreHopA), Credentials: fixedServiceCredentials{f.serviceEvidence}},
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
	// The fixed Core service audience may read one element's durable final
	// disposition on this host.
	facetConfig.CoreServices = map[values.PrincipalRef]values.FactAuthorityRef{w.service: owner}
	facetServer, err := facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, facetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(facetServer.Stop)
	go facetServer.Serve(facetListener)
	// The owner Core routes its disposition evidence reads to the Facet's
	// network host.
	dispositionSource, err := composition.NewNetworkCoreFacetSource(owner, []composition.CoreFacetEndpoint{
		{Address: facetHop, Facet: holderOwner, Client: newFacetClient(), Credentials: fixedServiceCredentials{f.serviceEvidence}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.cores[0].Facets().SetFacetDispositionSource(dispositionSource); err != nil {
		t.Fatal(err)
	}

	// The durable contact names the network Core host.
	acceptance, err := w.facets[0].Namespace.ReadAcceptance(w.actor, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.facets[0].Namespace.UpdateCoreNextHop(w.actor, "network-retirement-contact", oldKey, acceptance.NextHopRevision, coreHopA); err != nil {
		t.Fatal(err)
	}

	// The actor's control session captures the active relationship, then the
	// same actor holds the Facet relationship session for the disposal.
	admin := newCoreClient(owner, coreHopA)
	if err = admin.Establish(ctx, f.admin.Audience, [16]byte(f.admin.ID), [32]byte(f.admin.Secret)); err != nil {
		t.Fatal(err)
	}
	control := admin.FacetControl()
	inspected, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRetirementEntity))})
	if err != nil || inspected.GetResult() == nil || inspected.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		inspected.GetResult().GetTarget() != string(holderOwner) || inspected.GetResult().GetProposal() == nil {
		t.Fatalf("inspect active attachment: %v %v", inspected, err)
	}
	client := newFacetClient()
	if err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	relationships := client.Relationships()
	refusedControl := func(name string, out *corepb.ApplyFacetResponse, err error, failure networkpb.Failure) {
		t.Helper()
		if out.GetResult() != nil || (err == nil && out.GetFailure() != failure) {
			t.Fatalf("%s: %v %v", name, out, err)
		}
	}

	// The disposal request is itself public, so the acceptance it names must
	// carry a public selector: the locally attached acceptance has none. The
	// relationship is rebuilt through the public plane first — the detach,
	// re-proposal, wire acceptance and activation cross the two TLS hosts
	// exactly like the recovery chain — and the disposal below then runs
	// against the rebuilt lineage.
	detach := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-detach"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("detach"), Element: proto.String(string(networkRetirementEntity)), Expected: inspected.GetResult().GetRevision(),
	}}
	detached, err := control.ApplyFacet(ctx, detach)
	if err != nil || detached.GetResult() == nil || detached.GetResult().GetState().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("detach: %v %v", detached, err)
	}
	repropose := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-repropose"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(string(networkRetirementEntity)), Expected: detached.GetResult().GetState().GetRevision(),
		Target: proto.String(string(holderOwner)), NextHop: proto.String("local:facet0"),
	}}
	reproposed, err := control.ApplyFacet(ctx, repropose)
	if err != nil || reproposed.GetResult() == nil || reproposed.GetResult().GetState().GetPendingProposal() == nil ||
		reproposed.GetResult().GetState().GetPendingTarget() != string(holderOwner) {
		t.Fatalf("rebuild propose: %v %v", reproposed, err)
	}
	newRef := &facetpb.PublicAcceptanceRef{Core: proto.String(string(owner)), Element: proto.String(string(networkRetirementEntity)), Proposal: reproposed.GetResult().GetState().GetPendingProposal()}
	accept := &facetpb.AcceptAttachmentRequest{Ref: newRef, CoreNextHop: proto.String(string(coreHopA)), OperationId: []byte("network-retirement-accept")}
	accepted, err := relationships.AcceptAttachment(ctx, accept)
	if err != nil || accepted.GetResult() == nil || accepted.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		!accepted.GetResult().GetInspection().GetFound() || accepted.GetResult().GetInspection().GetCoreNextHop() != string(coreHopA) {
		t.Fatalf("rebuild acceptance: %v %v", accepted, err)
	}
	activate := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-activate"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("activate"), Element: proto.String(string(networkRetirementEntity)), Expected: reproposed.GetResult().GetState().GetRevision(),
	}}
	activated, err := control.ApplyFacet(ctx, activate)
	if err != nil || activated.GetResult().GetState().GetState() != corepb.AttachmentState_ACTIVE {
		t.Fatalf("activate: %v %v", activated, err)
	}
	rebound, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRetirementEntity))})
	if err != nil || rebound.GetResult() == nil || rebound.GetResult().GetState() != corepb.AttachmentState_ACTIVE ||
		rebound.GetResult().GetTarget() != string(holderOwner) || rebound.GetResult().GetProposal() == nil {
		t.Fatalf("inspect rebuilt attachment: %v %v", rebound, err)
	}
	newProposal := rebound.GetResult().GetProposal()
	// The wire proposal is the public token of the rebuilt lineage; the local
	// element read names the acceptance key that lineage will retire.
	activeRead, err := w.cores[0].Elements().ReadElement(w.actor, networkRetirementEntity, nil)
	if err != nil || activeRead.Attachment.State != corecontract.FacetActive || !activeRead.Attachment.Key.Valid() || activeRead.Attachment.Key == oldKey {
		t.Fatalf("rebuilt active lineage = %#v, err=%v", activeRead.Attachment, err)
	}
	newKey := activeRead.Attachment.Key
	newAccess := &facetpb.Access{NextHop: proto.String(string(facetHop)), Key: &networkpb.AttachmentKey{
		Core: proto.String(string(owner)), Facet: proto.String(string(holderOwner)), Element: proto.String(string(networkRetirementEntity)), Proposal: newProposal,
	}}

	// A retirement request while the relationship is still attached has no
	// disposal evidence: the Core's own disclosure is outside the retirement
	// shape and the request stays retryable without a journal entry.
	premature := &facetpb.RetireContentRequest{Ref: newRef, OperationId: []byte("network-retirement-premature")}
	if out, err := relationships.RetireContent(ctx, premature); out.GetResult() != nil || (err == nil && out.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("premature retire committed: %v %v", out, err)
	}

	// The owner Core's own control plane discloses the final detach of the
	// rebuilt relationship.
	finalDetach := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-detach-final"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("detach"), Element: proto.String(string(networkRetirementEntity)), Expected: rebound.GetResult().GetRevision(),
	}}
	detachedFinal, err := control.ApplyFacet(ctx, finalDetach)
	if err != nil || detachedFinal.GetResult() == nil || detachedFinal.GetResult().GetState().GetState() != corepb.AttachmentState_DETACHED {
		t.Fatalf("final detach: %v %v", detachedFinal, err)
	}
	detachedRevision := detachedFinal.GetResult().GetState().GetRevision()

	// The detached relationship no longer serves its content over the wire.
	served, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: w.typ.Bytes()})
	if served.GetResult() != nil || (err == nil && served.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("detached content readable: %v %v", served, err)
	}

	// A release without the recorded final disposition is refused as a
	// persistent journaled decision: the exact retry resolves its receipt.
	early := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-release-early"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("release"), Element: proto.String(string(networkRetirementEntity)), Expected: detachedRevision,
		Target: proto.String(string(holderOwner)), NextHop: proto.String(string(facetHop)),
	}}
	earlyRefusal, err := control.ApplyFacet(ctx, early)
	refusedControl("release without disposition", earlyRefusal, err, networkpb.Failure_NOT_FOUND)
	earlyRetry, err := control.ApplyFacet(ctx, early)
	if err != nil || !proto.Equal(earlyRetry, earlyRefusal) {
		t.Fatalf("journaled refusal replay = %#v, err=%v", earlyRetry, err)
	}

	// A release routed to an address the Core has no disposition endpoint for
	// fails safely and keeps the DETACHED record recoverable.
	unreachable := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-release-unreachable"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("release"), Element: proto.String(string(networkRetirementEntity)), Expected: detachedRevision,
		Target: proto.String(string(holderOwner)), NextHop: proto.String("https://127.0.0.1:9"),
	}}
	lost, err := control.ApplyFacet(ctx, unreachable)
	refusedControl("unreachable release", lost, err, networkpb.Failure_UNAVAILABLE)

	// The explicit disposal crosses the Facet host's public relationship plane
	// and reads the Core's DETACHED disclosure over the wire as its evidence.
	retire := &facetpb.RetireContentRequest{Ref: newRef, OperationId: []byte("network-retirement-retire")}
	retired, err := relationships.RetireContent(ctx, retire)
	if err != nil || retired.GetResult() == nil || retired.GetResult().GetStatus() != facetpb.CommitStatus_COMMITTED ||
		retired.GetResult().GetInspection().GetFound() || !proto.Equal(retired.GetResult().GetInspection().GetRef(), newRef) {
		t.Fatalf("retire: %v %v", retired, err)
	}
	retireReplay, err := relationships.RetireContent(ctx, retire)
	if err != nil || !proto.Equal(retireReplay, retired) {
		t.Fatalf("replayed retire = %#v, err=%v", retireReplay, err)
	}

	// A full Facet host restart keeps the durable final disposition: the
	// release below still finds its evidence after the restart.
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
	facetServer, err = facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, facetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(facetServer.Stop)
	go facetServer.Serve(reopenedListener)
	if err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}

	// The occupancy release crosses the Core's public control plane; the
	// final-disposition evidence read crosses the TLS link to the restarted
	// Facet host before the committed release clears the occupancy. The
	// restarted host's fresh session table first refuses the fixed Core
	// service's stale session, so the release fails safely as a retryable
	// transport refusal without a journal decision; the exact retry
	// re-establishes the service session and commits.
	release := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-release"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("release"), Element: proto.String(string(networkRetirementEntity)), Expected: detachedRevision,
		Target: proto.String(string(holderOwner)), NextHop: proto.String(string(facetHop)),
	}}
	released, err := control.ApplyFacet(ctx, release)
	if released.GetResult() != nil || (err == nil && released.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("release before session recovery: %v %v", released, err)
	}
	released, err = control.ApplyFacet(ctx, release)
	if err != nil || released.GetResult() == nil || released.GetResult().GetState().GetState() != corepb.AttachmentState_RETIRED ||
		released.GetResult().GetState().GetTarget() != string(holderOwner) || released.GetResult().GetState().GetKeyCore() != string(owner) {
		t.Fatalf("release: %v %v", released, err)
	}
	releaseReplay, err := control.ApplyFacet(ctx, release)
	if err != nil || !proto.Equal(releaseReplay, released) {
		t.Fatalf("replayed release = %#v, err=%v", releaseReplay, err)
	}

	// The retired slot never accepts a second release or a new proposal: both
	// are state conflicts on the disposed lineage, not revivals.
	inspectedRetired, err := control.InspectFacet(ctx, &corepb.InspectFacetRequest{Element: proto.String(string(networkRetirementEntity))})
	if err != nil || inspectedRetired.GetResult() == nil || inspectedRetired.GetResult().GetState() != corepb.AttachmentState_RETIRED ||
		inspectedRetired.GetResult().GetTarget() != string(holderOwner) || inspectedRetired.GetResult().GetKeyCore() != string(owner) {
		t.Fatalf("inspect retired attachment: %v %v", inspectedRetired, err)
	}
	again := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-release-again"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("release"), Element: proto.String(string(networkRetirementEntity)), Expected: inspectedRetired.GetResult().GetRevision(),
		Target: proto.String(string(holderOwner)), NextHop: proto.String(string(facetHop)),
	}}
	second, err := control.ApplyFacet(ctx, again)
	refusedControl("second release", second, err, networkpb.Failure_CONFLICT)
	revive := &corepb.ApplyFacetRequest{OperationId: []byte("network-retirement-revive"), Command: &corepb.PublicFacetCommand{
		Kind: proto.String("propose"), Element: proto.String(string(networkRetirementEntity)), Expected: inspectedRetired.GetResult().GetRevision(),
		Target: proto.String(string(holderOwner)), NextHop: proto.String(string(facetHop)),
	}}
	revived, err := control.ApplyFacet(ctx, revive)
	refusedControl("retired slot re-proposed", revived, err, networkpb.Failure_CONFLICT)

	// The disposed namespace is sealed over the wire: no second disposal, no
	// content service and no write qualification through the retired key.
	disposed := &facetpb.RetireContentRequest{Ref: newRef, OperationId: []byte("network-retirement-retire-again")}
	if out, err := relationships.RetireContent(ctx, disposed); out.GetResult() != nil || (err == nil && out.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired namespace disposed again: %v %v", out, err)
	}
	afterRetire, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: w.typ.Bytes()})
	if afterRetire.GetResult() != nil || (err == nil && afterRetire.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired content readable: %v %v", afterRetire, err)
	}
	afterPut, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: newAccess, TypeRef: w.typ.Bytes(), Payload: []byte(`"retired"`), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("network-retirement-put-again")})
	if afterPut.GetResult() != nil || (err == nil && afterPut.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired key writable: %v %v", afterPut, err)
	}

	// The released occupancy keeps only the rebuilt retired lineage on the Core.
	finalRead, err := w.cores[0].Elements().ReadElement(w.actor, networkRetirementEntity, nil)
	if err != nil || finalRead.Attachment.State != corecontract.FacetRetired || finalRead.Attachment.Key != newKey ||
		finalRead.Attachment.Key.Facet != holderOwner || finalRead.Attachment.Key.Element != networkRetirementEntity {
		t.Fatalf("retired attachment = %#v, err=%v", finalRead.Attachment, err)
	}

	// A second full Facet host restart replays the committed retire and the
	// journaled release decision with their exact original results, and the
	// retired lineage stays absent.
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
	settledListener, err := net.Listen("tcp", facetAddress)
	if err != nil {
		t.Fatal(err)
	}
	settledServer, err := facetgrpc.New(w.facets[0].Namespace, composition.RealmNetworkAuthentication{Connections: w.realmLinks, Owner: w.facets[0].Namespace}, facetConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(settledServer.Stop)
	go settledServer.Serve(settledListener)
	if err = client.Establish(ctx, f.evidence.Audience, [16]byte(f.evidence.ID), [32]byte(f.evidence.Secret)); err != nil {
		t.Fatal(err)
	}
	recovered, err := relationships.RetireContent(ctx, retire)
	if err != nil || !proto.Equal(recovered, retired) {
		t.Fatalf("replayed retire after restart = %#v, err=%v", recovered, err)
	}
	replayed, err := control.ApplyFacet(ctx, release)
	if err != nil || !proto.Equal(replayed, released) {
		t.Fatalf("replayed release after restart = %#v, err=%v", replayed, err)
	}
	sealed, err := client.Facet().Get(ctx, &facetpb.GetRequest{Access: newAccess, TypeRef: w.typ.Bytes()})
	if sealed.GetResult() != nil || (err == nil && sealed.GetFailure() != networkpb.Failure_UNAVAILABLE) {
		t.Fatalf("retired content readable after restart: %v %v", sealed, err)
	}
	previous, err := w.facets[0].Namespace.ReadAcceptance(w.actor, newKey)
	if err != nil || previous.Found {
		t.Fatalf("retired acceptance lineage kept serving: %#v, err=%v", previous, err)
	}
}
