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

// The recovery exercise revokes one element's facet relationship on its own
// Core and rebuilds it there: the payload stays in the one Facet the whole
// time and only the acceptance lineage moves.

var recoveryNotes = fixtureType("recovery.notes")

const (
	recoveryRoot   values.ElementID = "00000000-0000-4000-8000-000000002101"
	recoveryEntity values.ElementID = "00000000-0000-4000-8000-000000002102"
)

// TestLocalClusterFacetRecoveryRenewsLineage runs the whole local-detach
// recovery chain over the real local assembly: the owner Core detaches the
// accepted relationship, the dead write basis is refused as a recorded
// decision, the explicit recovery proposal protects the element from
// cancellation and deletion while pending, and the proposal-accept-activate
// round rebuilds the unique ACTIVE attachment with a fresh acceptance key
// that survives a full restart of both owners.
func TestLocalClusterFacetRecoveryRenewsLineage(t *testing.T) {
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
	facetCaller, err := policy.TrustLocalCaller(facetService)
	if err != nil {
		t.Fatal(err)
	}
	public := values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	corePath := filepath.Join(t.TempDir(), "recovery-core")
	holderPath := filepath.Join(t.TempDir(), "recovery-facet")
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
	holder, err := facet.CreateDurable(realmLinks, coreSource, facetInitial, holderPath, facet.Config{})
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

	owner, holderOwner := source.AuthorityRef(), holder.AuthorityRef()
	grants := []realm.PermissionBinding{
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(owner, recoveryRoot)),
		binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(owner, recoveryRoot)),
		binding(admin, corecontract.PermissionReadElement, values.AnchorClosureScope(owner, recoveryRoot)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, corecontract.PermissionFacetDetach, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, corecontract.PermissionFacetCancel, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, corecontract.PermissionDeleteEntity, values.CoreElementScope(owner, recoveryEntity)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(owner, recoveryEntity)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(owner, recoveryEntity)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(holderOwner, recoveryEntity)),
		binding(admin, facetcontract.PermissionReadAcceptance, values.FacetElementScope(holderOwner, recoveryEntity)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(holderOwner, recoveryEntity)),
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(holderOwner, recoveryEntity, recoveryNotes)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(holderOwner, recoveryEntity, recoveryNotes)),
	}
	for index, grant := range grants {
		if _, err = policy.PutBinding(admin, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("recovery-grant-%d", index))); err != nil {
			t.Fatal(err)
		}
	}

	if _, err = source.Elements().CreateRoot(admin, recoveryRoot, "recovery-root"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Elements().CreateEntity(admin, recoveryEntity, corecontract.SpatialTuple{Parent: recoveryRoot, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "recovery-entity"); err != nil {
		t.Fatal(err)
	}
	read, err := source.Elements().ReadElement(admin, recoveryEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := source.Facets().ProposeFacet(admin, recoveryEntity, holderOwner, "local:facet", read.Attachment.Revision, "recovery-propose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = holder.Namespace.AcceptAttachment(ctx, admin, proposed.Pending, "local:core-a", "recovery-accept"); err != nil {
		t.Fatal(err)
	}
	active, err := source.Facets().ActivateFacet(ctx, admin, recoveryEntity, proposed.Attachment.Revision, "recovery-activate")
	if err != nil {
		t.Fatal(err)
	}
	access := proposed.Pending
	notesPayload := fixturePayload("recovery-notes")
	stored, err := holder.Namespace.Put(ctx, admin, access, recoveryNotes, notesPayload, facetcontract.ExpectAbsent(), "recovery-put")
	if err != nil {
		t.Fatal(err)
	}

	// The owner Core revokes the relationship locally; the content records
	// stay but the write and query basis dies with the disclosure.
	detached, err := source.Facets().DetachFacet(admin, recoveryEntity, active.Attachment.Revision, "recovery-detach")
	if err != nil || detached.Attachment.State != corecontract.FacetDetached || detached.Attachment.Key != access {
		t.Fatalf("detach = %#v, err=%v", detached.Attachment, err)
	}
	if _, err = holder.Namespace.Put(ctx, admin, access, recoveryNotes, fixturePayload("forged"), facetcontract.ExpectAbsent(), "recovery-dead-put"); !errors.Is(err, facetcontract.ErrNamespaceDetached) {
		t.Fatalf("revoked relationship still writable: %v", err)
	}
	if _, err = holder.Namespace.Get(ctx, admin, access, recoveryNotes); !errors.Is(err, facetcontract.ErrNamespaceDetached) {
		t.Fatalf("revoked content still readable: %v", err)
	}

	// The explicit recovery proposal on the same Core. While it is pending,
	// the obligation protects the proposal from cancellation and the element
	// from deletion.
	reproposed, err := source.Facets().ProposeFacet(admin, recoveryEntity, holderOwner, "local:facet", detached.Attachment.Revision, "recovery-repropose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Facets().CancelFacetProposal(admin, recoveryEntity, reproposed.Attachment.Revision, "recovery-forbidden-cancel"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("pending recovery cancelled: %v", err)
	}
	current, err := source.Elements().ReadElement(admin, recoveryEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Elements().DeleteEntity(admin, recoveryEntity, current.Record.Entity.Revision, "recovery-forbidden-delete"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("pending recovery deleted: %v", err)
	}

	// The same Facet re-accepts through the recovery proposal; the payload
	// never moves and the revoked acceptance lineage is superseded.
	if _, err = holder.Namespace.AcceptAttachment(ctx, admin, reproposed.Pending, "local:core-a", "recovery-reaccept"); err != nil {
		t.Fatal(err)
	}
	rebound, err := holder.Namespace.ReadAcceptance(admin, reproposed.Pending)
	if err != nil || !rebound.Found || rebound.CoreNextHop != "local:core-a" {
		t.Fatalf("recovered lineage = %#v, err=%v", rebound, err)
	}
	if previous, err := holder.Namespace.ReadAcceptance(admin, access); err != nil || previous.Found {
		t.Fatalf("revoked lineage kept: %#v, err=%v", previous, err)
	}
	revived, err := source.Facets().ActivateFacet(ctx, admin, recoveryEntity, reproposed.Attachment.Revision, "recovery-activate-b")
	if err != nil {
		t.Fatal(err)
	}
	if revived.Attachment.State != corecontract.FacetActive || revived.Attachment.Key != reproposed.Pending ||
		revived.Attachment.Key.Facet != holderOwner || revived.Attachment.NextHop != "local:facet" {
		t.Fatalf("recovered attachment = %#v", revived.Attachment)
	}
	// The stored content rides the new lineage with its original revision;
	// the revoked key no longer resolves to any acceptance.
	assertPayload(t, mustGetFacet(t, holder, admin, reproposed.Pending, recoveryNotes), notesPayload)
	if _, err = holder.Namespace.Get(ctx, admin, access, recoveryNotes); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("revoked key still reads: %v", err)
	}
	if _, err = holder.Namespace.Put(ctx, admin, reproposed.Pending, recoveryNotes, fixturePayload("recovery-next"), facetcontract.ExpectRevision(stored.Revision), "recovery-rebound-put"); err != nil {
		t.Fatalf("recovered key not writable: %v", err)
	}
	if _, err = holder.Namespace.Put(ctx, admin, access, recoveryNotes, fixturePayload("forged"), facetcontract.ExpectRevision(stored.Revision), "recovery-stale-put"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("revoked key still writable: %v", err)
	}

	// The renewed state survives a full restart of both owners.
	if err = holder.Close(); err != nil {
		t.Fatal(err)
	}
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedCore, err := core.OpenDurable(realmLinks, corePath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("reopen Core: %v", err)
	}
	if err = coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: reopenedCore.Facets(), Control: reopenedCore.Facets(), Access: reopenedCore.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = reopenedCore.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	reopenedHolder, err := facet.OpenDurable(realmLinks, coreSource, holderPath, facet.Config{})
	if err != nil {
		t.Fatalf("reopen holder Facet: %v", err)
	}
	if reopenedHolder.AuthorityRef() != holderOwner {
		t.Fatal("reopened holder identity differs")
	}
	if err = wire.BindEndpoint("local:facet", reopenedHolder); err != nil {
		t.Fatal(err)
	}
	if previous, err := reopenedHolder.Namespace.ReadAcceptance(admin, access); err != nil || previous.Found {
		t.Fatalf("replayed revoked lineage kept: %#v, err=%v", previous, err)
	}
	persisted, err := reopenedHolder.Namespace.ReadAcceptance(admin, reproposed.Pending)
	if err != nil || !persisted.Found || persisted.CoreNextHop != "local:core-a" {
		t.Fatalf("replayed recovery missing: %#v, err=%v", persisted, err)
	}
	if _, err = reopenedHolder.Namespace.AcceptAttachment(ctx, admin, reproposed.Pending, "local:core-a", "recovery-reaccept"); err != nil {
		t.Fatalf("replayed recovery not idempotent: %v", err)
	}
	assertPayload(t, mustGetFacet(t, reopenedHolder, admin, reproposed.Pending, recoveryNotes), fixturePayload("recovery-next"))
	roundTripped, err := reopenedCore.Elements().ReadElement(admin, recoveryEntity, nil)
	if err != nil || roundTripped.Attachment.State != corecontract.FacetActive ||
		roundTripped.Attachment.Key != reproposed.Pending || roundTripped.Attachment.NextHop != "local:facet" {
		t.Fatalf("reopened Core attachment = %#v, err=%v", roundTripped.Attachment, err)
	}
	if err = reopenedCore.Close(); err != nil {
		t.Fatal(err)
	}
	if err = reopenedHolder.Close(); err != nil {
		t.Fatal(err)
	}
}
