package composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

var replacementNotesType = fixtureType("replacement.notes")
var replacementEmptyType = fixtureType("replacement.empty")

const replacementEmptyElement values.ElementID = "00000000-0000-4000-8000-000000001005"

type lostNamespaceFenceReply struct{ replacement.FacetSourcePeer }

func (p lostNamespaceFenceReply) FenceSource(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, target values.FactAuthorityRef) (replacement.FacetReplacementFenced, error) {
	if _, err := p.FacetSourcePeer.FenceSource(ctx, address, key, op, target); err != nil {
		return replacement.FacetReplacementFenced{}, err
	}
	return replacement.FacetReplacementFenced{}, corecontract.ErrUnavailable
}

// TestLocalClusterFacetReplacementMovesNamespace runs one complete namespace
// replacement over the real local assembly: an authorized public begin, the
// Core-driven fence, relay, acceptance, switch, install and settle through the
// composition peers, byte-exact content on the target, a permanently fenced
// source, and the whole switched state surviving a full restart of all three
// owners.
func TestLocalClusterFacetReplacementMovesNamespace(t *testing.T) {
	ctx := context.Background()
	identity := mustRotation(t, 1, 0, 0, 0)
	policy, err := realminstance.New(realm.Seed{
		ID:              admin.Realm,
		LocalPrincipals: []values.PrincipalID{admin.Principal, facetService.Principal},
		Bindings: []realm.PermissionBinding{
			binding(admin, realm.PermissionPutBinding, values.RealmScope(admin.Realm)),
		},
	})
	if err != nil {
		t.Fatalf("New Realm: %v", err)
	}
	corePath := filepath.Join(t.TempDir(), "core")
	sourcePath := filepath.Join(t.TempDir(), "source-facet")
	targetPath := filepath.Join(t.TempDir(), "target-facet")
	public := values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:realm", policy); err != nil {
		t.Fatal(err)
	}
	coreInitial := corecontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	facetInitial := facetcontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	spatial, err := core.CreateDurable(realmLinks, coreInitial, corePath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("CreateDurable Core: %v", err)
	}
	coreConnections := &composition.LocalFacetCoreConnections{}
	if err = coreConnections.BindEndpoint("local:core", composition.LocalFacetCoreEndpoint{Read: spatial.Facets(), Control: spatial.Facets(), Access: spatial.Access()}); err != nil {
		t.Fatal(err)
	}
	facetCaller, err := policy.TrustLocalCaller(facetService)
	if err != nil {
		t.Fatal(err)
	}
	coreSource, err := coreConnections.ForFacetCaller(facetCaller)
	if err != nil {
		t.Fatal(err)
	}
	source, err := facet.CreateDurable(realmLinks, coreSource, facetInitial, sourcePath, facet.Config{})
	if err != nil {
		t.Fatalf("CreateDurable source Facet: %v", err)
	}
	target, err := facet.CreateDurable(realmLinks, coreSource, facetInitial, targetPath, facet.Config{})
	if err != nil {
		t.Fatalf("CreateDurable target Facet: %v", err)
	}
	connections := &composition.LocalFacetConnections{}
	if err = connections.BindEndpoint("local:facet-a", source); err != nil {
		t.Fatal(err)
	}
	if err = connections.BindEndpoint("local:facet-b", target); err != nil {
		t.Fatal(err)
	}
	acceptance, err := connections.ForCore(facetService)
	if err != nil {
		t.Fatal(err)
	}
	if err = spatial.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	peers, err := connections.ForReplacement(facetCaller)
	if err != nil {
		t.Fatal(err)
	}
	owner := spatial.AuthorityRef()
	grants := []realm.PermissionBinding{
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(owner, rootID)),
		binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, rootID)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionFacetReplacementBegin, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionFacetReplacementRecover, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionFacetReplacementRead, values.CoreElementScope(owner, entityID)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, corecontract.PermissionFacetReplacementBegin, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, corecontract.PermissionFacetReplacementRecover, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, corecontract.PermissionFacetReplacementRead, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, entityID)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, entityID)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, replacementEmptyElement)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(source.AuthorityRef(), entityID)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(source.AuthorityRef(), replacementEmptyElement)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(source.AuthorityRef(), entityID)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(source.AuthorityRef(), replacementEmptyElement)),
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(source.AuthorityRef(), entityID, profileType)),
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(source.AuthorityRef(), entityID, replacementNotesType)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(source.AuthorityRef(), entityID, profileType)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(target.AuthorityRef(), entityID, profileType)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(target.AuthorityRef(), entityID, replacementNotesType)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(source.AuthorityRef(), replacementEmptyElement, replacementEmptyType)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(target.AuthorityRef(), replacementEmptyElement, replacementEmptyType)),
		binding(admin, facetcontract.PermissionListTypes, values.FacetElementScope(target.AuthorityRef(), replacementEmptyElement)),
		binding(facetService, facetcontract.PermissionNamespaceExport, values.FacetElementScope(source.AuthorityRef(), entityID)),
		binding(facetService, facetcontract.PermissionNamespaceImport, values.FacetElementScope(target.AuthorityRef(), entityID)),
		binding(facetService, facetcontract.PermissionNamespaceExport, values.FacetElementScope(source.AuthorityRef(), replacementEmptyElement)),
		binding(facetService, facetcontract.PermissionNamespaceImport, values.FacetElementScope(target.AuthorityRef(), replacementEmptyElement)),
	}
	for index, grant := range grants {
		if _, err = policy.PutBinding(admin, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("replacement-cluster-grant-%d", index))); err != nil {
			t.Fatalf("PutBinding: %v", err)
		}
	}

	if _, err = spatial.Elements().CreateRoot(admin, rootID, "replacement-cluster-root"); err != nil {
		t.Fatal(err)
	}
	if _, err = spatial.Elements().CreateEntity(admin, entityID, corecontract.SpatialTuple{Parent: rootID, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "replacement-cluster-entity"); err != nil {
		t.Fatal(err)
	}
	if _, err = spatial.Elements().CreateEntity(admin, replacementEmptyElement, corecontract.SpatialTuple{Parent: rootID, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, identity), Range: mustSphere(t, 1)}, "replacement-cluster-empty-entity"); err != nil {
		t.Fatal(err)
	}
	emptyRead, err := spatial.Elements().ReadElement(admin, replacementEmptyElement, nil)
	if err != nil {
		t.Fatal(err)
	}
	emptyProposal, err := spatial.Facets().ProposeFacet(admin, replacementEmptyElement, source.AuthorityRef(), "local:facet-a", emptyRead.Attachment.Revision, "replacement-cluster-empty-propose")
	if err != nil {
		t.Fatal(err)
	}
	emptyAccess := emptyProposal.Pending
	if _, err = source.Namespace.AcceptAttachment(ctx, admin, emptyAccess, "local:core", "replacement-cluster-empty-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err = spatial.Facets().ActivateFacet(ctx, admin, replacementEmptyElement, emptyProposal.Attachment.Revision, "replacement-cluster-empty-activate"); err != nil {
		t.Fatal(err)
	}
	read, err := spatial.Elements().ReadElement(admin, entityID, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := spatial.Facets().ProposeFacet(admin, entityID, source.AuthorityRef(), "local:facet-a", read.Attachment.Revision, "replacement-cluster-propose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Namespace.AcceptAttachment(ctx, admin, proposed.Pending, "local:core", "replacement-cluster-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err = spatial.Facets().ActivateFacet(ctx, admin, entityID, proposed.Attachment.Revision, "replacement-cluster-activate"); err != nil {
		t.Fatal(err)
	}
	access := proposed.Pending
	profilePayload, notesPayload := fixturePayload("replacement-profile"), fixturePayload("replacement-notes")
	if _, err = source.Namespace.Put(ctx, admin, access, profileType, profilePayload, facetcontract.ExpectAbsent(), "replacement-cluster-put-profile"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Namespace.Put(ctx, admin, access, replacementNotesType, notesPayload, facetcontract.ExpectAbsent(), "replacement-cluster-put-notes"); err != nil {
		t.Fatal(err)
	}

	adminCaller, err := policy.TrustLocalCaller(admin)
	if err != nil {
		t.Fatal(err)
	}
	active, err := spatial.Facets().InspectFacetControl(ctx, adminCaller, entityID)
	if err != nil {
		t.Fatal(err)
	}
	const op = corecontract.OperationID("replacement-cluster-move")
	begin, err := spatial.Facets().BeginFacetReplacement(ctx, adminCaller, op, corecontract.PublicFacetReplacementCommand{
		Element: entityID, Expected: active.Revision, Target: target.AuthorityRef(),
		TargetAddress: "local:facet-b", SourceAddress: "local:core",
	})
	if err != nil {
		t.Fatal(err)
	}
	if begin.Progress != corecontract.FacetReplacementReserved || begin.Source != source.AuthorityRef() ||
		begin.Target != target.AuthorityRef() || begin.Refused || begin.Revision.Kind != "facet-replacement-revision" {
		t.Fatalf("begin status = %#v", begin)
	}
	final, err := spatial.Facets().RecoverFacetReplacement(ctx, adminCaller, op, entityID, peers)
	if err != nil {
		t.Fatalf("RecoverFacetReplacement: %v", err)
	}
	if final.Progress != corecontract.FacetReplacementSettled || final.Refused || final.Revision != begin.Revision {
		t.Fatalf("final status = %#v", final)
	}
	if again, err := spatial.Facets().RecoverFacetReplacement(ctx, adminCaller, op, entityID, peers); err != nil || again != final {
		t.Fatalf("repeated recovery = %#v, err=%v", again, err)
	}
	if inspected, err := spatial.Facets().InspectFacetReplacement(ctx, adminCaller, op, entityID); err != nil || inspected != final {
		t.Fatalf("inspected status = %#v, err=%v", inspected, err)
	}

	switched, err := spatial.Elements().ReadElement(admin, entityID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if switched.Attachment.State != corecontract.FacetActive || switched.Attachment.Key.Facet != target.AuthorityRef() ||
		switched.Attachment.Key.Element != entityID || switched.Attachment.NextHop != "local:facet-b" {
		t.Fatalf("switched attachment = %#v", switched.Attachment)
	}
	if activeAfter, err := spatial.Facets().InspectFacetControl(ctx, adminCaller, entityID); err != nil ||
		activeAfter.Target != target.AuthorityRef() || activeAfter.PendingTarget.Valid() || activeAfter.PendingProposal.Valid() {
		t.Fatalf("switched inspection = %#v, err=%v", activeAfter, err)
	}
	targetAccess := switched.Attachment.Key
	assertPayload(t, mustGetFacet(t, target, admin, targetAccess, profileType), profilePayload)
	assertPayload(t, mustGetFacet(t, target, admin, targetAccess, replacementNotesType), notesPayload)
	if _, err = source.Namespace.Get(ctx, admin, access, profileType); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("fenced source still served content: got %v", err)
	}

	sourceState, err := peers.Source.CheckSourceExport(ctx, "local:facet-a", access, op)
	if err != nil {
		t.Fatal(err)
	}
	if !sourceState.Found || !sourceState.Fenced || !sourceState.Settled || sourceState.FinalCut == 0 ||
		sourceState.ManifestDigest == [32]byte{} || len(sourceState.Manifest) == 0 {
		t.Fatalf("source evidence = %#v", sourceState)
	}
	imported, err := peers.Target.ReadTargetImport(ctx, "local:facet-b", targetAccess, op)
	if err != nil {
		t.Fatal(err)
	}
	// Installed is the terminal import evidence; Accepted marks only the
	// accepted-pending-install window, which the completed install closed.
	if imported.Accepted || !imported.Installed || imported.FinalCut != sourceState.FinalCut || imported.Digest != sourceState.ManifestDigest || len(imported.Live) != 0 {
		t.Fatalf("target evidence = %#v", imported)
	}
	decoded, err := facetcontract.DecodeNamespaceManifest(sourceState.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Element != entityID || decoded.Source != source.AuthorityRef() ||
		decoded.FinalCut != sourceState.FinalCut || len(decoded.Entries) != 2 {
		t.Fatalf("decoded manifest = %#v", decoded)
	}
	entries := map[string]facetcontract.NamespaceManifestEntry{}
	for _, entry := range decoded.Entries {
		entries[string(entry.Type.Bytes())] = entry
	}
	for typ, payload := range map[values.FacetTypeRef][]byte{profileType: profilePayload, replacementNotesType: notesPayload} {
		entry, ok := entries[string(typ.Bytes())]
		if !ok || !entry.Live || entry.Length != uint64(len(payload)) || entry.Digest != sha256.Sum256(payload) {
			t.Fatalf("manifest entry %q = %#v", typ, entry)
		}
	}

	if _, err = source.Namespace.Get(ctx, admin, emptyAccess, replacementEmptyType); !errors.Is(err, facetcontract.ErrKeyNotFound) {
		t.Fatalf("active empty source read = %v, want key-not-found", err)
	}
	emptyActive, err := spatial.Facets().InspectFacetControl(ctx, adminCaller, replacementEmptyElement)
	if err != nil {
		t.Fatal(err)
	}
	const emptyOp corecontract.OperationID = "replacement-cluster-empty-move"
	emptyBegin, err := spatial.Facets().BeginFacetReplacement(ctx, adminCaller, emptyOp, corecontract.PublicFacetReplacementCommand{
		Element: replacementEmptyElement, Expected: emptyActive.Revision, Target: target.AuthorityRef(),
		TargetAddress: "local:facet-b", SourceAddress: "local:core",
	})
	if err != nil || emptyBegin.Progress != corecontract.FacetReplacementReserved {
		t.Fatalf("begin empty replacement = %#v, err=%v", emptyBegin, err)
	}
	lostReplyPeers := peers
	lostReplyPeers.Source = lostNamespaceFenceReply{FacetSourcePeer: peers.Source}
	if _, err = spatial.Facets().RecoverFacetReplacement(ctx, adminCaller, emptyOp, replacementEmptyElement, lostReplyPeers); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("lost response after empty source fence = %v, want unavailable", err)
	}
	interruptedEmpty, err := spatial.Facets().InspectFacetReplacement(ctx, adminCaller, emptyOp, replacementEmptyElement)
	if err != nil || interruptedEmpty.Progress != corecontract.FacetReplacementPermitted || interruptedEmpty.Revision != emptyBegin.Revision {
		t.Fatalf("Core recorded a fence with the response lost = %#v, err=%v", interruptedEmpty, err)
	}
	fencedEmptySource, err := peers.Source.CheckSourceExport(ctx, "local:facet-a", emptyAccess, emptyOp)
	if err != nil || !fencedEmptySource.Fenced || fencedEmptySource.Settled || fencedEmptySource.FinalCut == 0 || fencedEmptySource.ManifestDigest != sha256.Sum256(nil) || len(fencedEmptySource.Manifest) == 0 {
		t.Fatalf("source fence that precedes Core evidence = %#v, err=%v", fencedEmptySource, err)
	}
	if _, err = source.Namespace.Get(ctx, admin, emptyAccess, replacementEmptyType); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("source served after fence response was lost = %v, want unavailable", err)
	}

	manifestBefore := append([]byte(nil), sourceState.Manifest...)
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	if err = target.Close(); err != nil {
		t.Fatal(err)
	}
	if err = spatial.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedCore, err := core.OpenDurable(realmLinks, corePath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("reopen Core: %v", err)
	}
	if reopenedCore.AuthorityRef() != owner {
		t.Fatalf("reopened Core identity %q differs from %q", reopenedCore.AuthorityRef(), owner)
	}
	if err = coreConnections.BindEndpoint("local:core", composition.LocalFacetCoreEndpoint{Read: reopenedCore.Facets(), Control: reopenedCore.Facets(), Access: reopenedCore.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = reopenedCore.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	reopenedSource, err := facet.OpenDurable(realmLinks, coreSource, sourcePath, facet.Config{})
	if err != nil {
		t.Fatalf("reopen source Facet: %v", err)
	}
	reopenedTarget, err := facet.OpenDurable(realmLinks, coreSource, targetPath, facet.Config{})
	if err != nil {
		t.Fatalf("reopen target Facet: %v", err)
	}
	if reopenedSource.AuthorityRef() != source.AuthorityRef() || reopenedTarget.AuthorityRef() != target.AuthorityRef() {
		t.Fatal("reopened Facet identities differ")
	}
	if err = connections.BindEndpoint("local:facet-a", reopenedSource); err != nil {
		t.Fatal(err)
	}
	if err = connections.BindEndpoint("local:facet-b", reopenedTarget); err != nil {
		t.Fatal(err)
	}

	if reopened, err := reopenedCore.Facets().InspectFacetReplacement(ctx, adminCaller, op, entityID); err != nil || reopened != final {
		t.Fatalf("reopened replacement status = %#v, err=%v", reopened, err)
	}
	persisted, err := reopenedCore.Elements().ReadElement(admin, entityID, nil)
	if err != nil || persisted.Attachment.State != corecontract.FacetActive || persisted.Attachment.Key != targetAccess {
		t.Fatalf("reopened attachment = %#v, err=%v", persisted.Attachment, err)
	}
	assertPayload(t, mustGetFacet(t, reopenedTarget, admin, targetAccess, profileType), profilePayload)
	assertPayload(t, mustGetFacet(t, reopenedTarget, admin, targetAccess, replacementNotesType), notesPayload)
	settled, err := peers.Source.CheckSourceExport(ctx, "local:facet-a", access, op)
	if err != nil {
		t.Fatal(err)
	}
	if !settled.Fenced || !settled.Settled || !bytes.Equal(settled.Manifest, manifestBefore) {
		t.Fatalf("reopened source evidence = %#v", settled)
	}

	// The empty namespace replacement is recovered from its durable Core
	// reservation only after Core and both Facets have restarted.
	finalEmpty, err := reopenedCore.Facets().RecoverFacetReplacement(ctx, adminCaller, emptyOp, replacementEmptyElement, peers)
	if err != nil || finalEmpty.Progress != corecontract.FacetReplacementSettled || finalEmpty.Revision != emptyBegin.Revision {
		t.Fatalf("recover empty replacement after restart = %#v, err=%v", finalEmpty, err)
	}
	emptySourceState, err := peers.Source.CheckSourceExport(ctx, "local:facet-a", emptyAccess, emptyOp)
	if err != nil || !emptySourceState.Fenced || !emptySourceState.Settled || emptySourceState.FinalCut == 0 || emptySourceState.ManifestDigest == [32]byte{} || len(emptySourceState.Manifest) == 0 {
		t.Fatalf("settled empty source evidence = %#v, err=%v", emptySourceState, err)
	}
	emptyManifest, err := facetcontract.DecodeNamespaceManifest(emptySourceState.Manifest)
	if err != nil || emptyManifest.Element != replacementEmptyElement || emptyManifest.Source != reopenedSource.AuthorityRef() || emptyManifest.FinalCut != emptySourceState.FinalCut || len(emptyManifest.Entries) != 0 {
		t.Fatalf("decoded empty source manifest = %#v, err=%v", emptyManifest, err)
	}
	if _, err = reopenedSource.Namespace.Get(ctx, admin, emptyAccess, replacementEmptyType); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("fenced empty source read = %v, want unavailable", err)
	}
	emptyTargetRead, err := reopenedCore.Elements().ReadElement(admin, replacementEmptyElement, nil)
	if err != nil || emptyTargetRead.Attachment.State != corecontract.FacetActive || emptyTargetRead.Attachment.Key.Facet != reopenedTarget.AuthorityRef() {
		t.Fatalf("empty target attachment = %#v, err=%v", emptyTargetRead.Attachment, err)
	}
	if _, err = reopenedTarget.Namespace.Get(ctx, admin, emptyTargetRead.Attachment.Key, replacementEmptyType); !errors.Is(err, facetcontract.ErrKeyNotFound) {
		t.Fatalf("installed empty target read = %v, want key-not-found", err)
	}
	if listed, err := reopenedTarget.Namespace.ListTypes(ctx, admin, emptyTargetRead.Attachment.Key); err != nil || len(listed.Types) != 0 {
		t.Fatalf("installed empty target ListTypes = %#v, err=%v", listed.Types, err)
	}
	emptyImport, err := peers.Target.ReadTargetImport(ctx, "local:facet-b", emptyTargetRead.Attachment.Key, emptyOp)
	if err != nil || !emptyImport.Installed || emptyImport.Accepted || emptyImport.FinalCut != emptySourceState.FinalCut || emptyImport.Digest != emptySourceState.ManifestDigest || len(emptyImport.Live) != 0 {
		t.Fatalf("installed empty target evidence = %#v, err=%v", emptyImport, err)
	}
	if err = reopenedCore.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedSource.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedTarget.Close(); err != nil {
		t.Fatal(err)
	}
}
