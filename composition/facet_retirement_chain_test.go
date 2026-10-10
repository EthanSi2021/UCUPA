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
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

// The retirement exercise disposes of one element's content at the explicit
// request of the holding Facet and then releases the controlling Core's
// occupancy: one transaction records the exact final disposition, the Core
// verifies that evidence before releasing, and the retired lineage stays
// permanently closed without ever becoming a fresh acceptance again.

var retirementNotes = fixtureType("retirement.notes")

const (
	retirementRoot   values.ElementID = "00000000-0000-4000-8000-000000002201"
	retirementEntity values.ElementID = "00000000-0000-4000-8000-000000002202"
	obligationRoot   values.ElementID = "00000000-0000-4000-8000-000000002203"
	obligationEntity values.ElementID = "00000000-0000-4000-8000-000000002204"
)

var coreService = values.PrincipalRef{Realm: "realm-p", Principal: "core-service"}

// retirementCluster is one real local assembly with both service directions
// wired: the Facet's Core source and the Core's acceptance and final
// disposition sources for the holder.
type retirementCluster struct {
	policy      *realminstance.Realm
	realmLinks  *composition.LocalRealmConnections
	source      *core.Core
	holder      *facet.Instance
	coreLinks   *composition.LocalFacetCoreConnections
	wire        *composition.LocalFacetConnections
	coreSource  facetcontract.CoreSource
	acceptance  corecontract.FacetAcceptanceSource
	disposition corecontract.FacetDispositionSource
	owner       values.FactAuthorityRef
	holderOwner values.FactAuthorityRef
	corePath    string
	holderPath  string
	public      values.PublicAccessPolicy
}

func newRetirementCluster(t *testing.T, root, entity values.ElementID) *retirementCluster {
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
	facetCaller, err := policy.TrustLocalCaller(facetService)
	if err != nil {
		t.Fatal(err)
	}
	coreCaller, err := policy.TrustLocalCaller(coreService)
	if err != nil {
		t.Fatal(err)
	}
	public := values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	corePath := filepath.Join(t.TempDir(), "retirement-core")
	holderPath := filepath.Join(t.TempDir(), "retirement-facet")
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:realm", policy); err != nil {
		t.Fatal(err)
	}
	coreInitial := corecontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	facetInitial := facetcontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	source, err := core.CreateDurable(realmLinks, coreInitial, corePath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("CreateDurable Core: %v", err)
	}
	coreLinks := &composition.LocalFacetCoreConnections{}
	if err = coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: source.Facets(), Control: source.Facets(), Access: source.Access()}); err != nil {
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
	disposition, err := wire.ForCoreCaller(coreCaller)
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Facets().SetFacetDispositionSource(disposition); err != nil {
		t.Fatal(err)
	}
	x := &retirementCluster{policy: policy, realmLinks: realmLinks, source: source, holder: holder, coreLinks: coreLinks, wire: wire,
		coreSource: coreSource, acceptance: acceptance, disposition: disposition,
		owner: source.AuthorityRef(), holderOwner: holder.AuthorityRef(), corePath: corePath, holderPath: holderPath, public: public}
	grants := []realm.PermissionBinding{
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(x.owner, root)),
		binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(x.owner, root)),
		binding(admin, corecontract.PermissionReadElement, values.AnchorClosureScope(x.owner, root)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionFacetDetach, values.CoreElementScope(x.owner, entity)),
		binding(admin, corecontract.PermissionFacetRelease, values.CoreElementScope(x.owner, entity)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(x.owner, entity)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(x.owner, entity)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(x.holderOwner, entity)),
		binding(admin, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, entity)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, entity)),
		binding(admin, facetcontract.PermissionRetireContent, values.FacetElementScope(x.holderOwner, entity)),
		binding(coreService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, entity)),
		binding(coreService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(x.holderOwner, root)),
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(x.holderOwner, entity, retirementNotes)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(x.holderOwner, entity, retirementNotes)),
	}
	for index, grant := range grants {
		if _, err = policy.PutBinding(admin, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("retirement-grant-%d", index))); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

// attachRetiredNamespace drives the shared prefix of both retirement chains:
// create, propose, accept, activate, store one payload and revoke.
func attachRetiredNamespace(t *testing.T, x *retirementCluster, root, entity values.ElementID, label string) (access values.FacetAcceptanceKey, detached corecontract.FacetAttachmentResult) {
	t.Helper()
	ctx := context.Background()
	identity := mustRotation(t, 1, 0, 0, 0)
	if _, err := x.source.Elements().CreateRoot(admin, root, corecontract.OperationID(label+"-root")); err != nil {
		t.Fatal(err)
	}
	if _, err := x.source.Elements().CreateEntity(admin, entity, corecontract.SpatialTuple{Parent: root, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, corecontract.OperationID(label+"-entity")); err != nil {
		t.Fatal(err)
	}
	read, err := x.source.Elements().ReadElement(admin, entity, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := x.source.Facets().ProposeFacet(admin, entity, x.holderOwner, "local:facet", read.Attachment.Revision, corecontract.OperationID(label+"-propose"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.holder.Namespace.AcceptAttachment(ctx, admin, proposed.Pending, "local:core-a", facetcontract.OperationID(label+"-accept")); err != nil {
		t.Fatal(err)
	}
	active, err := x.source.Facets().ActivateFacet(ctx, admin, entity, proposed.Attachment.Revision, corecontract.OperationID(label+"-activate"))
	if err != nil {
		t.Fatal(err)
	}
	if active.Attachment.State != corecontract.FacetActive || active.Attachment.Key != proposed.Pending {
		t.Fatalf("active attachment = %#v", active.Attachment)
	}
	if _, err = x.holder.Namespace.Put(ctx, admin, proposed.Pending, retirementNotes, fixturePayload(label), facetcontract.ExpectAbsent(), facetcontract.OperationID(label+"-put")); err != nil {
		t.Fatal(err)
	}
	detached, err = x.source.Facets().DetachFacet(admin, entity, active.Attachment.Revision, corecontract.OperationID(label+"-detach"))
	if err != nil || detached.Attachment.State != corecontract.FacetDetached || detached.Attachment.Key != proposed.Pending {
		t.Fatalf("detach = %#v, err=%v", detached.Attachment, err)
	}
	return proposed.Pending, detached
}

// reopenRetirementCluster restarts both owners and rewires every service
// direction against the reopened instances.
func reopenRetirementCluster(t *testing.T, x *retirementCluster) (*core.Core, *facet.Instance) {
	t.Helper()
	if err := x.holder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := x.source.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedCore, err := core.OpenDurable(x.realmLinks, x.corePath, corecontract.Config{PublicAccessPolicy: &x.public})
	if err != nil {
		t.Fatalf("reopen Core: %v", err)
	}
	if err = x.coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: reopenedCore.Facets(), Control: reopenedCore.Facets(), Access: reopenedCore.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = reopenedCore.Facets().SetFacetAcceptanceSource(x.acceptance); err != nil {
		t.Fatal(err)
	}
	if err = reopenedCore.Facets().SetFacetDispositionSource(x.disposition); err != nil {
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
	return reopenedCore, reopenedHolder
}

// TestLocalClusterFacetRetirementReleasesControl runs the whole content
// retirement chain over the real local assembly: the revoked-lineage disposal
// records the exact final disposition in one transaction, the Core refuses to
// release without that evidence and releases only against it, the retired
// relationship never becomes a fresh proposal or acceptance again, and every
// committed answer and terminal refusal replays identically after a full
// restart of both owners.
func TestLocalClusterFacetRetirementReleasesControl(t *testing.T) {
	ctx := context.Background()
	x := newRetirementCluster(t, retirementRoot, retirementEntity)
	access, detached := attachRetiredNamespace(t, x, retirementRoot, retirementEntity, "retirement")

	// Revocation is not deletion: the content records stay while the write and
	// query basis dies with the disclosure.
	if _, err := x.holder.Namespace.Put(ctx, admin, access, retirementNotes, fixturePayload("forged"), facetcontract.ExpectAbsent(), "retirement-dead-put"); !errors.Is(err, facetcontract.ErrNamespaceDetached) {
		t.Fatalf("revoked relationship still writable: %v", err)
	}
	if _, err := x.holder.Namespace.Get(ctx, admin, access, retirementNotes); !errors.Is(err, facetcontract.ErrNamespaceDetached) {
		t.Fatalf("revoked content still readable: %v", err)
	}
	// The Core releases no occupancy without the content owner's final
	// disposition.
	if _, err := x.source.Facets().ReleaseFacet(ctx, admin, retirementEntity, x.holderOwner, "local:facet", detached.Attachment.Revision, "retirement-release-early"); !errors.Is(err, corecontract.ErrDispositionNotFound) {
		t.Fatalf("release without disposition: %v", err)
	}

	// The explicit disposal at the content owner.
	disposed, err := x.holder.Namespace.RetireContent(ctx, admin, access, "retirement-dispose")
	if err != nil || disposed.Status != facetcontract.Committed {
		t.Fatalf("retire: %+v %v", disposed, err)
	}
	// The disposition port discloses the exact final disposition to the Core
	// service and a proven absence for every other element.
	final, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, retirementEntity)
	if err != nil || !final.Found || final.Key != access || final.Facet != x.holderOwner || final.Element != retirementEntity || !final.Cut.Valid() {
		t.Fatalf("final disposition: %+v %v", final, err)
	}
	if absent, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, retirementRoot); err != nil || absent != (corecontract.FacetDispositionRead{}) {
		t.Fatalf("proven absence: %+v %v", absent, err)
	}

	// The release consumes the disposition and keeps the former key as the
	// permanent lineage record.
	released, err := x.source.Facets().ReleaseFacet(ctx, admin, retirementEntity, x.holderOwner, "local:facet", detached.Attachment.Revision, "retirement-release")
	if err != nil || released.Status != corecontract.MutationCommitted || released.Attachment.State != corecontract.FacetRetired ||
		released.Attachment.Key != access || released.Attachment.NextHop != "" {
		t.Fatalf("release = %#v, err=%v", released, err)
	}
	// The committed operation replays its original answer.
	replayed, err := x.source.Facets().ReleaseFacet(ctx, admin, retirementEntity, x.holderOwner, "local:facet", detached.Attachment.Revision, "retirement-release")
	if err != nil || replayed.Status != released.Status || replayed.Attachment != released.Attachment {
		t.Fatalf("release replay: %#v, err=%v", replayed, err)
	}
	// A second release under another operation is refused: the slot is
	// terminal.
	if _, err = x.source.Facets().ReleaseFacet(ctx, admin, retirementEntity, x.holderOwner, "local:facet", released.Attachment.Revision, "retirement-release-again"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("second release: %v", err)
	}
	// The retired relationship never becomes a fresh proposal.
	if _, err = x.source.Facets().ProposeFacet(admin, retirementEntity, x.holderOwner, "local:facet", released.Attachment.Revision, "retirement-repropose"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("retired element re-proposed: %v", err)
	}

	// The Facet keeps only the anti-duplication record: the disposed namespace
	// refuses a second disposal, any write and any acceptance read of the old
	// key.
	if _, err = x.holder.Namespace.RetireContent(ctx, admin, access, "retirement-dispose-again"); !errors.Is(err, facetcontract.ErrNamespaceRetired) {
		t.Fatalf("second disposal: %v", err)
	}
	if _, err = x.holder.Namespace.Put(ctx, admin, access, retirementNotes, fixturePayload("forged"), facetcontract.ExpectAbsent(), "retirement-dead-write"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("retired key still writable: %v", err)
	}
	if previous, err := x.holder.Namespace.ReadAcceptance(admin, access); err != nil || previous.Found {
		t.Fatalf("retired lineage kept: %#v, err=%v", previous, err)
	}
	// The Core's retired slot is outside every Facet decision shape.
	if _, err = x.coreSource.ReadProposal(ctx, "local:core-a", access); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("retired slot still discloses proposals: %v", err)
	}
	closed, err := x.source.Elements().ReadElement(admin, retirementEntity, nil)
	if err != nil || closed.Attachment.State != corecontract.FacetRetired || closed.Attachment.Key != access {
		t.Fatalf("retired Core attachment = %#v, err=%v", closed.Attachment, err)
	}

	// Everything above survives a full restart of both owners.
	reopenedCore, reopenedHolder := reopenRetirementCluster(t, x)
	persisted, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, retirementEntity)
	if err != nil || persisted != final {
		t.Fatalf("reopened disposition: %+v, err=%v", persisted, err)
	}
	replayedRelease, err := reopenedCore.Facets().ReleaseFacet(ctx, admin, retirementEntity, x.holderOwner, "local:facet", detached.Attachment.Revision, "retirement-release")
	if err != nil || replayedRelease.Status != corecontract.MutationCommitted || replayedRelease.Attachment != released.Attachment {
		t.Fatalf("reopened release replay: %#v, err=%v", replayedRelease, err)
	}
	replayedRetire, err := reopenedHolder.Namespace.RetireContent(ctx, admin, access, "retirement-dispose")
	if err != nil || replayedRetire.Status != facetcontract.Committed || replayedRetire.Cut != disposed.Cut {
		t.Fatalf("reopened retire replay: %+v, err=%v", replayedRetire, err)
	}
	if _, err = reopenedHolder.Namespace.RetireContent(ctx, admin, access, "retirement-dispose-again"); !errors.Is(err, facetcontract.ErrNamespaceRetired) {
		t.Fatalf("reopened second disposal: %v", err)
	}
	if _, err = reopenedHolder.Namespace.Put(ctx, admin, access, retirementNotes, fixturePayload("forged"), facetcontract.ExpectAbsent(), "retirement-after-restart"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("reopened retired key writable: %v", err)
	}
	roundTripped, err := reopenedCore.Elements().ReadElement(admin, retirementEntity, nil)
	if err != nil || roundTripped.Attachment.State != corecontract.FacetRetired || roundTripped.Attachment.Key != access {
		t.Fatalf("reopened Core attachment = %#v, err=%v", roundTripped.Attachment, err)
	}
	if _, err = reopenedCore.Facets().ProposeFacet(admin, retirementEntity, x.holderOwner, "local:facet", roundTripped.Attachment.Revision, "retirement-repropose-after"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("reopened retired element re-proposed: %v", err)
	}
	if err = reopenedCore.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedHolder.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestLocalClusterFacetRetirementDischargesObligation runs the obligation
// window: the still-open accepted proposal of a revoked relationship is
// disposed at the Facet, the Core keeps protecting it from cancellation until
// its release consumes the disposition, and the discharge, the terminal slot
// and the closed namespace all survive a full restart of both owners.
func TestLocalClusterFacetRetirementDischargesObligation(t *testing.T) {
	ctx := context.Background()
	x := newRetirementCluster(t, obligationRoot, obligationEntity)
	if _, err := x.policy.PutBinding(admin, binding(admin, corecontract.PermissionFacetCancel, values.CoreElementScope(x.owner, obligationEntity)), x.policy.BindingSetRevision(), "obligation-grant-cancel"); err != nil {
		t.Fatal(err)
	}
	access, detached := attachRetiredNamespace(t, x, obligationRoot, obligationEntity, "obligation")

	// The recovery proposal this Facet re-accepts without a completed
	// activation opens the obligation window.
	reproposed, err := x.source.Facets().ProposeFacet(admin, obligationEntity, x.holderOwner, "local:facet", detached.Attachment.Revision, "obligation-repropose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.holder.Namespace.AcceptAttachment(ctx, admin, reproposed.Pending, "local:core-a", "obligation-reaccept"); err != nil {
		t.Fatal(err)
	}
	// The open obligation protects the proposal from cancellation.
	if _, err = x.source.Facets().CancelFacetProposal(admin, obligationEntity, reproposed.Attachment.Revision, "obligation-forbidden-cancel"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("obligated proposal cancelled: %v", err)
	}
	// The Core still releases no occupancy without a final disposition.
	if _, err = x.source.Facets().ReleaseFacet(ctx, admin, obligationEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "obligation-release-early"); !errors.Is(err, corecontract.ErrDispositionNotFound) {
		t.Fatalf("release without disposition: %v", err)
	}

	// The still-open accepted proposal retires through the obligation window;
	// its key is the recorded final disposition.
	disposed, err := x.holder.Namespace.RetireContent(ctx, admin, reproposed.Pending, "obligation-dispose")
	if err != nil || disposed.Status != facetcontract.Committed {
		t.Fatalf("obligation retire: %+v %v", disposed, err)
	}
	final, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, obligationEntity)
	if err != nil || !final.Found || final.Key != reproposed.Pending || final.Element != obligationEntity {
		t.Fatalf("window disposition: %+v %v", final, err)
	}
	// The obligation stays on the Core until the release consumes the
	// disposition.
	if _, err = x.source.Facets().CancelFacetProposal(admin, obligationEntity, reproposed.Attachment.Revision, "obligation-still-obligated"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("obligation discharged before release: %v", err)
	}
	released, err := x.source.Facets().ReleaseFacet(ctx, admin, obligationEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "obligation-release")
	if err != nil || released.Status != corecontract.MutationCommitted || released.Attachment.State != corecontract.FacetRetired ||
		released.Attachment.Key != access || released.Pending != (values.FacetAcceptanceKey{}) {
		t.Fatalf("window release = %#v, err=%v", released, err)
	}
	// The discharged obligation is gone: nothing is left to cancel.
	if _, err = x.source.Facets().CancelFacetProposal(admin, obligationEntity, released.Attachment.Revision, "obligation-cancel-after"); !errors.Is(err, corecontract.ErrProposalNotCurrent) {
		t.Fatalf("discharged obligation cancel: %v", err)
	}
	// The former lineage key stays as the permanent record; the retired slot
	// never becomes a fresh proposal.
	roundTripped, err := x.source.Elements().ReadElement(admin, obligationEntity, nil)
	if err != nil || roundTripped.Attachment.State != corecontract.FacetRetired || roundTripped.Attachment.Key != access {
		t.Fatalf("window Core attachment = %#v, err=%v", roundTripped.Attachment, err)
	}
	if _, err = x.source.Facets().ProposeFacet(admin, obligationEntity, x.holderOwner, "local:facet", roundTripped.Attachment.Revision, "obligation-repropose-after"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("retired element re-proposed: %v", err)
	}
	// Both keys of the closed namespace are gone at the Facet.
	if _, err = x.holder.Namespace.RetireContent(ctx, admin, reproposed.Pending, "obligation-dispose-again"); !errors.Is(err, facetcontract.ErrNamespaceRetired) {
		t.Fatalf("window second disposal: %v", err)
	}
	if _, err = x.holder.Namespace.Put(ctx, admin, reproposed.Pending, retirementNotes, fixturePayload("forged"), facetcontract.ExpectAbsent(), "obligation-dead-write"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("window retired key writable: %v", err)
	}
	if previous, err := x.holder.Namespace.ReadAcceptance(admin, reproposed.Pending); err != nil || previous.Found {
		t.Fatalf("window retired lineage kept: %#v, err=%v", previous, err)
	}
	if previous, err := x.holder.Namespace.ReadAcceptance(admin, access); err != nil || previous.Found {
		t.Fatalf("window former lineage kept: %#v, err=%v", previous, err)
	}

	// The discharged obligation and the closed namespace survive a full
	// restart of both owners.
	reopenedCore, reopenedHolder := reopenRetirementCluster(t, x)
	persisted, err := x.disposition.ReadFacetDisposition(ctx, "local:facet", x.holderOwner, obligationEntity)
	if err != nil || persisted != final {
		t.Fatalf("reopened window disposition: %+v, err=%v", persisted, err)
	}
	replayedRelease, err := reopenedCore.Facets().ReleaseFacet(ctx, admin, obligationEntity, x.holderOwner, "local:facet", reproposed.Attachment.Revision, "obligation-release")
	if err != nil || replayedRelease.Status != corecontract.MutationCommitted || replayedRelease.Attachment != released.Attachment {
		t.Fatalf("reopened window release replay: %#v, err=%v", replayedRelease, err)
	}
	if _, err = reopenedCore.Facets().CancelFacetProposal(admin, obligationEntity, replayedRelease.Attachment.Revision, "obligation-cancel-after-restart"); !errors.Is(err, corecontract.ErrProposalNotCurrent) {
		t.Fatalf("reopened discharged obligation cancel: %v", err)
	}
	replayedRetire, err := reopenedHolder.Namespace.RetireContent(ctx, admin, reproposed.Pending, "obligation-dispose")
	if err != nil || replayedRetire.Status != facetcontract.Committed || replayedRetire.Cut != disposed.Cut {
		t.Fatalf("reopened window retire replay: %+v, err=%v", replayedRetire, err)
	}
	if err = reopenedCore.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedHolder.Close(); err != nil {
		t.Fatal(err)
	}
}
