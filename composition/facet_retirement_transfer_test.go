package composition_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	localtransition "ucupa/ucupa-core/transport/local/transition"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// The transferred retirement exercise covers the window between a completed
// Core transfer and the same-Facet re-acceptance: the holding Facet still
// carries the departed Core's acceptance, and only the old Core's durable
// successor statement can discharge that obligation without a fresh
// acceptance before the new Core's release.

const (
	transferRoot       values.ElementID = "00000000-0000-4000-8000-000000002205"
	transferEntity     values.ElementID = "00000000-0000-4000-8000-000000002206"
	transferTargetRoot values.ElementID = "00000000-0000-4000-8000-000000002207"
	absenceEntity      values.ElementID = "00000000-0000-4000-8000-000000002208"
)

// transferRetirementCluster is the real local assembly of the transferred
// retirement window: the element's first Core A, the transfer target Core B,
// and the holding Facet, with both Cores sharing the Facet's acceptance and
// final-disposition services.
type transferRetirementCluster struct {
	policy      *realminstance.Realm
	realmLinks  *composition.LocalRealmConnections
	source      *core.Core
	successor   *core.Core
	holder      *facet.Instance
	coreLinks   *composition.LocalFacetCoreConnections
	wire        *composition.LocalFacetConnections
	coreSource  facetcontract.CoreSource
	adminCaller values.CallerContext
	acceptance  corecontract.FacetAcceptanceSource
	disposition corecontract.FacetDispositionSource
	owner       values.FactAuthorityRef
	next        values.FactAuthorityRef
	holderOwner values.FactAuthorityRef
	corePath    string
	nextPath    string
	holderPath  string
	public      values.PublicAccessPolicy
}

func newTransferRetirementCluster(t *testing.T, root, entity, targetRoot values.ElementID) *transferRetirementCluster {
	t.Helper()
	policy, err := realminstance.New(realm.Seed{
		ID:              admin.Realm,
		LocalPrincipals: []values.PrincipalID{admin.Principal, facetService.Principal, coreService.Principal},
		Bindings: []realm.PermissionBinding{
			binding(admin, realm.PermissionPutBinding, values.RealmScope(admin.Realm)),
		},
	})
	if err != nil {
		t.Fatalf("New Realm: %v", err)
	}
	adminCaller, err := policy.TrustLocalCaller(admin)
	if err != nil {
		t.Fatal(err)
	}
	facetCaller, err := policy.TrustLocalCaller(facetService)
	if err != nil {
		t.Fatal(err)
	}
	coreCaller, err := policy.TrustLocalCaller(coreService)
	if err != nil {
		t.Fatal(err)
	}
	public := values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	corePath := filepath.Join(t.TempDir(), "transfer-core-a")
	nextPath := filepath.Join(t.TempDir(), "transfer-core-b")
	holderPath := filepath.Join(t.TempDir(), "transfer-facet")
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:realm", policy); err != nil {
		t.Fatal(err)
	}
	coreInitial := corecontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	facetInitial := facetcontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	source, err := core.CreateDurable(realmLinks, coreInitial, corePath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("CreateDurable Core A: %v", err)
	}
	successor, err := core.CreateDurable(realmLinks, coreInitial, nextPath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("CreateDurable Core B: %v", err)
	}
	coreLinks := &composition.LocalFacetCoreConnections{}
	if err = coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: source.Facets(), Control: source.Facets(), Access: source.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = coreLinks.BindEndpoint("local:core-b", composition.LocalFacetCoreEndpoint{Read: successor.Facets(), Control: successor.Facets(), Access: successor.Access()}); err != nil {
		t.Fatal(err)
	}
	coreSource, err := coreLinks.ForFacetCaller(facetCaller)
	if err != nil {
		t.Fatal(err)
	}
	holder, err := facet.CreateDurable(realmLinks, coreSource, facetInitial, holderPath, facet.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("CreateDurable holder Facet: %v", err)
	}
	wire := &composition.LocalFacetConnections{}
	if err = wire.BindEndpoint("local:facet", holder); err != nil {
		t.Fatal(err)
	}
	acceptance, err := wire.ForCore(facetService)
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	if err = successor.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	disposition, err := wire.ForCoreCaller(coreCaller)
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Facets().SetFacetDispositionSource(disposition); err != nil {
		t.Fatal(err)
	}
	if err = successor.Facets().SetFacetDispositionSource(disposition); err != nil {
		t.Fatal(err)
	}
	x := &transferRetirementCluster{policy: policy, realmLinks: realmLinks, source: source, successor: successor, holder: holder,
		coreLinks: coreLinks, wire: wire, coreSource: coreSource, adminCaller: adminCaller,
		acceptance: acceptance, disposition: disposition,
		owner: source.AuthorityRef(), next: successor.AuthorityRef(), holderOwner: holder.AuthorityRef(),
		corePath: corePath, nextPath: nextPath, holderPath: holderPath, public: public}
	grants := []realm.PermissionBinding{
		// Core A keeps the first owner's write and transfer basis.
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(x.owner, root)),
		binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(x.owner, root)),
		binding(admin, corecontract.PermissionReadElement, values.AnchorClosureScope(x.owner, root)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(x.owner, absenceEntity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(x.owner, absenceEntity)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(x.owner, absenceEntity)),
		binding(admin, corecontract.PermissionDeleteEntity, values.CoreElementScope(x.owner, absenceEntity)),
		binding(admin, corecontract.PermissionTransitionExport, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionTransitionCommit, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionTransitionInspect, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionPlaceElement, values.CoreElementScope(x.owner, root)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(x.owner, entity)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(x.owner, absenceEntity)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(x.owner, entity)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(x.owner, absenceEntity)),
		// Core B receives the transferred element and later releases it.
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(x.next, targetRoot)),
		binding(admin, corecontract.PermissionReadElement, values.AnchorClosureScope(x.next, targetRoot)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionPlaceElement, values.CoreElementScope(x.next, targetRoot)),
		binding(admin, corecontract.PermissionTransitionImport, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionTransitionExport, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionTransitionCommit, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionTransitionInspect, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionFacetCancel, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionFacetRelease, values.CoreElementScope(x.next, entity)),
		binding(admin, corecontract.PermissionDeleteEntity, values.CoreElementScope(x.next, entity)),
		// The holding Facet accepts, reads and retires both elements.
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(x.holderOwner, entity)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(x.holderOwner, absenceEntity)),
		binding(admin, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, entity)),
		binding(admin, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, absenceEntity)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, entity)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, absenceEntity)),
		binding(coreService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, entity)),
		binding(admin, facetcontract.PermissionRetireContent, values.FacetElementScope(x.holderOwner, entity)),
		binding(admin, facetcontract.PermissionRetireContent, values.FacetElementScope(x.holderOwner, absenceEntity)),
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(x.holderOwner, entity, retirementNotes)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(x.holderOwner, entity, retirementNotes)),
	}
	for index, grant := range grants {
		if _, err = policy.PutBinding(admin, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("transfer-retirement-grant-%d", index))); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

// reopenTransferRetirementCluster restarts all three owners and rewires every
// service direction against the reopened instances.
func reopenTransferRetirementCluster(t *testing.T, x *transferRetirementCluster) (*core.Core, *core.Core, *facet.Instance) {
	t.Helper()
	if err := x.holder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := x.source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := x.successor.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedA, err := core.OpenDurable(x.realmLinks, x.corePath, corecontract.Config{PublicAccessPolicy: &x.public})
	if err != nil {
		t.Fatalf("reopen Core A: %v", err)
	}
	reopenedB, err := core.OpenDurable(x.realmLinks, x.nextPath, corecontract.Config{PublicAccessPolicy: &x.public})
	if err != nil {
		t.Fatalf("reopen Core B: %v", err)
	}
	if err = x.coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: reopenedA.Facets(), Control: reopenedA.Facets(), Access: reopenedA.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = x.coreLinks.BindEndpoint("local:core-b", composition.LocalFacetCoreEndpoint{Read: reopenedB.Facets(), Control: reopenedB.Facets(), Access: reopenedB.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = reopenedA.Facets().SetFacetAcceptanceSource(x.acceptance); err != nil {
		t.Fatal(err)
	}
	if err = reopenedA.Facets().SetFacetDispositionSource(x.disposition); err != nil {
		t.Fatal(err)
	}
	if err = reopenedB.Facets().SetFacetAcceptanceSource(x.acceptance); err != nil {
		t.Fatal(err)
	}
	if err = reopenedB.Facets().SetFacetDispositionSource(x.disposition); err != nil {
		t.Fatal(err)
	}
	reopenedHolder, err := facet.OpenDurable(x.realmLinks, x.coreSource, x.holderPath, facet.Config{PublicAccessPolicy: &x.public})
	if err != nil {
		t.Fatalf("reopen holder Facet: %v", err)
	}
	if reopenedHolder.AuthorityRef() != x.holderOwner {
		t.Fatal("reopened holder identity differs")
	}
	if err = x.wire.BindEndpoint("local:facet", reopenedHolder); err != nil {
		t.Fatal(err)
	}
	return reopenedA, reopenedB, reopenedHolder
}

// TestLocalClusterFacetRetirementDischargesTransferObligation runs the
// transferred retirement window: the element moves from Core A to Core B
// while the Facet keeps the old acceptance and never completes the
// re-acceptance. The old Core's durable successor statement is the disposal
// evidence that discharges the obligation without a fresh acceptance, the
// new Core's release consumes the disposition and clears the open proposal,
// a deleted element's proven absence never disposes, and the whole
// discharged state survives a full restart of all three owners.
func TestLocalClusterFacetRetirementDischargesTransferObligation(t *testing.T) {
	ctx := context.Background()
	x := newTransferRetirementCluster(t, transferRoot, transferEntity, transferTargetRoot)
	identity := mustRotation(t, 1, 0, 0, 0)
	if _, err := x.source.Elements().CreateRoot(admin, transferRoot, "transfer-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.successor.Elements().CreateRoot(admin, transferTargetRoot, "transfer-target-root"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.source.Elements().CreateEntity(admin, transferEntity, corecontract.SpatialTuple{Parent: transferRoot, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "transfer-entity"); err != nil {
		t.Fatal(err)
	}
	read, err := x.source.Elements().ReadElement(admin, transferEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := x.source.Facets().ProposeFacet(admin, transferEntity, x.holderOwner, "local:facet", read.Attachment.Revision, "transfer-propose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.holder.Namespace.AcceptAttachment(ctx, admin, proposed.Pending, "local:core-a", "transfer-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err = x.source.Facets().ActivateFacet(ctx, admin, transferEntity, proposed.Attachment.Revision, "transfer-activate"); err != nil {
		t.Fatal(err)
	}
	access := proposed.Pending
	if _, err = x.holder.Namespace.Put(ctx, admin, access, retirementNotes, fixturePayload("transfer-notes"), facetcontract.ExpectAbsent(), "transfer-put"); err != nil {
		t.Fatal(err)
	}

	// The element moves to Core B with its attachment record; the Facet keeps
	// the old acceptance and its contact to the departed Core A.
	command := corecontract.TransitionCommand{
		Kind: corecontract.TransitionSlice, Target: x.next, Boundary: transferEntity,
		Expected:      publicElementRevision(t, x.source, x.adminCaller, transferRoot, transferEntity),
		Final:         corecontract.SpatialTuple{Parent: transferTargetRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, identity), Range: mustSphere(t, 1)},
		SourceAddress: "local:core-a", TargetAddress: "local:core-b",
	}
	binding0 := corecontract.TransitionApprovalBinding{Source: x.owner, Operation: "transfer-move", Command: command}
	if _, err = x.source.Transfers().BeginTransition(ctx, x.adminCaller, binding0.Operation, binding0.Command); err != nil {
		t.Fatal(err)
	}
	view, err := x.successor.Transfers().AuthorizeTransitionImport(ctx, x.adminCaller, binding0)
	if err != nil {
		t.Fatal(err)
	}
	link, err := localtransition.New(x.source.Transfers(), x.successor.Transfers(), binding0, x.adminCaller, view)
	if err != nil {
		t.Fatal(err)
	}
	if done, err := x.source.Transfers().RecoverTransition(ctx, x.adminCaller, binding0, link); err != nil || done.Progress != corecontract.TransitionSettled {
		t.Fatalf("transfer = %+v, %v", done, err)
	}

	// The new Core keeps the transferred predecessor record; the departed
	// Core A no longer serves the element at all.
	transferred, err := x.successor.Elements().ReadElement(admin, transferEntity, nil)
	if err != nil || transferred.Attachment.State != corecontract.FacetDetached ||
		transferred.Attachment.Key.Core != x.owner || transferred.Attachment.Key.Facet != x.holderOwner {
		t.Fatalf("transferred attachment = %#v, err=%v", transferred.Attachment, err)
	}
	if _, err = x.source.Elements().ReadElement(admin, transferEntity, nil); err == nil {
		t.Fatal("departed Core still serves the element")
	}

	// The still-open re-acceptance proposal pins the obligation on the new
	// Core: no cancellation, no deletion and no release without a disposition.
	reproposed, err := x.successor.Facets().ProposeFacet(admin, transferEntity, x.holderOwner, "local:facet", transferred.Attachment.Revision, "transfer-repropose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.successor.Facets().CancelFacetProposal(admin, transferEntity, reproposed.Attachment.Revision, "transfer-forbidden-cancel"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("obligated proposal cancelled: %v", err)
	}
	if _, err = x.successor.Elements().DeleteEntity(admin, transferEntity, transferred.Record.Entity.Revision, "transfer-forbidden-delete"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("obligated element deleted: %v", err)
	}
	if _, err = x.successor.Facets().ReleaseFacet(ctx, admin, transferEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "transfer-release-early"); !errors.Is(err, corecontract.ErrDispositionNotFound) {
		t.Fatalf("release without disposition: %v", err)
	}

	// A lost old Core keeps the disposal retryable; nothing is journalled.
	if err = x.coreLinks.UnbindEndpoint("local:core-a"); err != nil {
		t.Fatal(err)
	}
	if _, err = x.holder.Namespace.RetireContent(ctx, admin, access, "transfer-dispose"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("lost old Core disposed: %v", err)
	}
	if err = x.coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: x.source.Facets(), Control: x.source.Facets(), Access: x.source.Access()}); err != nil {
		t.Fatal(err)
	}

	// The explicit disposal discharges the obligation through the old Core's
	// durable successor statement; no re-acceptance happens first.
	disposed, err := x.holder.Namespace.RetireContent(ctx, admin, access, "transfer-dispose")
	if err != nil || disposed.Status != facetcontract.Committed {
		t.Fatalf("transferred retire: %+v %v", disposed, err)
	}
	final, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, transferEntity)
	if err != nil || !final.Found || final.Key != access || final.Facet != x.holderOwner || final.Element != transferEntity || !final.Cut.Valid() {
		t.Fatalf("transferred disposition: %+v %v", final, err)
	}

	// The new Core's release consumes the disposition, retires the record and
	// clears the open re-acceptance proposal.
	released, err := x.successor.Facets().ReleaseFacet(ctx, admin, transferEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "transfer-release")
	if err != nil || released.Status != corecontract.MutationCommitted || released.Attachment.State != corecontract.FacetRetired ||
		released.Attachment.Key != access || released.Pending != (values.FacetAcceptanceKey{}) {
		t.Fatalf("transferred release = %#v, err=%v", released, err)
	}
	if replayedRelease, err := x.successor.Facets().ReleaseFacet(ctx, admin, transferEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "transfer-release"); err != nil || replayedRelease.Attachment != released.Attachment {
		t.Fatalf("release replay: %#v, err=%v", replayedRelease, err)
	}
	// The discharged obligation frees the element again: nothing is pending
	// to cancel, the retired slot never becomes a fresh proposal, and the
	// element itself can be deleted.
	if _, err = x.successor.Facets().CancelFacetProposal(admin, transferEntity, released.Attachment.Revision, "transfer-cancel-after"); !errors.Is(err, corecontract.ErrProposalNotCurrent) {
		t.Fatalf("discharged obligation cancel: %v", err)
	}
	if _, err = x.successor.Facets().ProposeFacet(admin, transferEntity, x.holderOwner, "local:facet", released.Attachment.Revision, "transfer-repropose-after"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("retired element re-proposed: %v", err)
	}
	roundTripped, err := x.successor.Elements().ReadElement(admin, transferEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.successor.Elements().DeleteEntity(admin, transferEntity, roundTripped.Record.Entity.Revision, "transfer-delete-after"); err != nil {
		t.Fatalf("discharged element not deletable: %v", err)
	}

	// The closed namespace keeps only its final disposition and refusals.
	if _, err = x.holder.Namespace.RetireContent(ctx, admin, access, "transfer-dispose-again"); !errors.Is(err, facetcontract.ErrNamespaceRetired) {
		t.Fatalf("second disposal: %v", err)
	}
	if _, err = x.holder.Namespace.Put(ctx, admin, access, retirementNotes, fixturePayload("forged"), facetcontract.ExpectAbsent(), "transfer-dead-write"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("retired key writable: %v", err)
	}
	if previous, err := x.holder.Namespace.ReadAcceptance(admin, access); err != nil || previous.Found {
		t.Fatalf("retired lineage kept: %#v, err=%v", previous, err)
	}

	// A deleted element's proven absence is ordinary deletion, never
	// disposal evidence: the old Core answers no successor statement, so the
	// held acceptance keeps its obligation.
	if _, err = x.source.Elements().CreateEntity(admin, absenceEntity, corecontract.SpatialTuple{Parent: transferRoot, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "absence-entity"); err != nil {
		t.Fatal(err)
	}
	absentRead, err := x.source.Elements().ReadElement(admin, absenceEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	absentProposed, err := x.source.Facets().ProposeFacet(admin, absenceEntity, x.holderOwner, "local:facet", absentRead.Attachment.Revision, "absence-propose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.holder.Namespace.AcceptAttachment(ctx, admin, absentProposed.Pending, "local:core-a", "absence-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err = x.source.Facets().ActivateFacet(ctx, admin, absenceEntity, absentProposed.Attachment.Revision, "absence-activate"); err != nil {
		t.Fatal(err)
	}
	absentAfter, err := x.source.Elements().ReadElement(admin, absenceEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.source.Elements().DeleteEntity(admin, absenceEntity, absentAfter.Record.Entity.Revision, "absence-delete"); err != nil {
		t.Fatal(err)
	}
	if _, err = x.holder.Namespace.RetireContent(ctx, admin, absentProposed.Pending, "absence-retire"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("plain deletion disposed an obligation: %v", err)
	}
	if kept, err := x.holder.Namespace.ReadAcceptance(admin, absentProposed.Pending); err != nil || !kept.Found {
		t.Fatalf("absence lost the acceptance: %#v, err=%v", kept, err)
	}

	// The discharged window survives a full restart of all three owners.
	reopenedA, reopenedB, reopenedHolder := reopenTransferRetirementCluster(t, x)
	persisted, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, transferEntity)
	if err != nil || persisted != final {
		t.Fatalf("reopened disposition: %+v, err=%v", persisted, err)
	}
	replayedRetire, err := reopenedHolder.Namespace.RetireContent(ctx, admin, access, "transfer-dispose")
	if err != nil || replayedRetire.Status != facetcontract.Committed || replayedRetire.Cut != disposed.Cut {
		t.Fatalf("reopened retire replay: %+v, err=%v", replayedRetire, err)
	}
	if _, err = reopenedHolder.Namespace.RetireContent(ctx, admin, access, "transfer-dispose-again"); !errors.Is(err, facetcontract.ErrNamespaceRetired) {
		t.Fatalf("reopened second disposal: %v", err)
	}
	// The release receipt replays identically even though the element left
	// the restarted Core entirely.
	replayedRelease, err := reopenedB.Facets().ReleaseFacet(ctx, admin, transferEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "transfer-release")
	if err != nil || replayedRelease.Status != corecontract.MutationCommitted || replayedRelease.Attachment != released.Attachment {
		t.Fatalf("reopened release replay: %#v, err=%v", replayedRelease, err)
	}
	if _, err = reopenedHolder.Namespace.RetireContent(ctx, admin, absentProposed.Pending, "absence-retire"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("reopened plain deletion disposed: %v", err)
	}
	if err = reopenedA.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedB.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedHolder.Close(); err != nil {
		t.Fatal(err)
	}
}
