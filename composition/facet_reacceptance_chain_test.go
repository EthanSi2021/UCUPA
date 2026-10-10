package composition_test

import (
	"context"
	"encoding/json"
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

// The same-Facet re-acceptance exercise moves one element between two real
// durable Cores while the payload stays in the one Facet the whole time.

var reacceptNotes = fixtureType("reacceptance.notes")

const (
	reacceptRoot       values.ElementID = "00000000-0000-4000-8000-000000002001"
	reacceptEntity     values.ElementID = "00000000-0000-4000-8000-000000002002"
	reacceptTargetRoot values.ElementID = "00000000-0000-4000-8000-000000002003"
)

func publicElementRevision(t *testing.T, c *core.Core, caller values.CallerContext, anchor, id values.ElementID) values.PublicToken {
	t.Helper()
	enter, err := values.CanonicalJSONValue(map[string]any{"op": "enter", "reference": map[string]any{"boundary": map[string]any{
		"core": string(c.AuthorityRef()), "anchor": string(anchor)}, "nextHop": "local:core-a"}, "intent": "read"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.Access().PublicCall(context.Background(), caller, enter)
	if err != nil {
		t.Fatalf("public enter: %v", err)
	}
	var entered map[string]any
	if err := json.Unmarshal(raw, &entered); err != nil {
		t.Fatal(err)
	}
	read, err := values.CanonicalJSONValue(map[string]any{"op": "read", "handle": entered["handle"], "address": map[string]any{"down": []any{string(id)}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = c.Access().PublicCall(context.Background(), caller, read)
	if err != nil {
		t.Fatalf("public read: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	v := result["record"].(map[string]any)["revision"].(map[string]any)
	return values.PublicToken{Owner: values.FactAuthorityRef(v["owner"].(string)), Kind: v["kind"].(string), Value: v["value"].(string)}
}

// TestLocalClusterFacetReacceptanceKeepsNamespace runs the whole same-Facet
// re-acceptance chain over the real local assembly: an element moves from one
// durable Core to a second one, the Facet keeps the payload and its acceptance
// lineage alone, the transferred DETACHED record refuses deletion, cancel and
// a repeated Core transfer while the new proposal is pending, and the new
// proposal-accept-activate round leaves a single ACTIVE attachment that
// survives a full restart of all three owners.
func TestLocalClusterFacetReacceptanceKeepsNamespace(t *testing.T) {
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
	caller, err := policy.TrustLocalCaller(admin)
	if err != nil {
		t.Fatal(err)
	}
	facetCaller, err := policy.TrustLocalCaller(facetService)
	if err != nil {
		t.Fatal(err)
	}
	realmLinks := &composition.LocalRealmConnections{}
	if err = realmLinks.BindEndpoint("local:realm", policy); err != nil {
		t.Fatal(err)
	}
	coreInitial := corecontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	facetInitial := facetcontract.RealmInitialization{Realm: admin.Realm, NextHop: "local:realm"}
	public := values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	coreAPath := filepath.Join(t.TempDir(), "core-a")
	coreBPath := filepath.Join(t.TempDir(), "core-b")
	holderPath := filepath.Join(t.TempDir(), "holder-facet")
	source, err := core.CreateDurable(realmLinks, coreInitial, coreAPath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("CreateDurable Core A: %v", err)
	}
	successor, err := core.CreateDurable(realmLinks, coreInitial, coreBPath, corecontract.Config{PublicAccessPolicy: &public})
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
	if err = successor.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}

	ownerA, ownerB, holderOwner := source.AuthorityRef(), successor.AuthorityRef(), holder.AuthorityRef()
	grants := []realm.PermissionBinding{
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(ownerA, reacceptRoot)),
		binding(admin, corecontract.PermissionCreateEntity, values.CoreElementScope(ownerA, reacceptRoot)),
		binding(admin, corecontract.PermissionReadElement, values.AnchorClosureScope(ownerA, reacceptRoot)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetProposalRead, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionExport, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionCommit, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionInspect, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionImport, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionPlaceElement, values.CoreElementScope(ownerA, reacceptRoot)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(ownerA, reacceptEntity)),
		binding(admin, corecontract.PermissionCreateRoot, values.CoreElementScope(ownerB, reacceptTargetRoot)),
		binding(admin, corecontract.PermissionReadElement, values.AnchorClosureScope(ownerB, reacceptTargetRoot)),
		binding(admin, corecontract.PermissionReadElement, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionPlaceElement, values.CoreElementScope(ownerB, reacceptTargetRoot)),
		binding(admin, corecontract.PermissionTransitionImport, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionExport, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionCommit, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionTransitionInspect, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetPropose, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetCancel, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetActivate, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionFacetProposalRead, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, corecontract.PermissionDeleteEntity, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(facetService, corecontract.PermissionFacetProposalRead, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(facetService, corecontract.PermissionFacetAttachmentRead, values.CoreElementScope(ownerB, reacceptEntity)),
		binding(admin, facetcontract.PermissionAccept, values.FacetElementScope(holderOwner, reacceptEntity)),
		binding(admin, facetcontract.PermissionReadAcceptance, values.FacetElementScope(holderOwner, reacceptEntity)),
		binding(facetService, facetcontract.PermissionReadAcceptance, values.FacetElementScope(holderOwner, reacceptEntity)),
		binding(admin, facetcontract.PermissionPut, values.FacetKeyScope(holderOwner, reacceptEntity, reacceptNotes)),
		binding(admin, facetcontract.PermissionGet, values.FacetKeyScope(holderOwner, reacceptEntity, reacceptNotes)),
	}
	for index, grant := range grants {
		if _, err = policy.PutBinding(admin, grant, policy.BindingSetRevision(), realm.OperationID(fmt.Sprintf("reacceptance-grant-%d", index))); err != nil {
			t.Fatal(err)
		}
	}

	if _, err = source.Elements().CreateRoot(admin, reacceptRoot, "reacceptance-root"); err != nil {
		t.Fatal(err)
	}
	if _, err = successor.Elements().CreateRoot(admin, reacceptTargetRoot, "reacceptance-target-root"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Elements().CreateEntity(admin, reacceptEntity, corecontract.SpatialTuple{Parent: reacceptRoot, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)}, "reacceptance-entity"); err != nil {
		t.Fatal(err)
	}
	read, err := source.Elements().ReadElement(admin, reacceptEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := source.Facets().ProposeFacet(admin, reacceptEntity, holderOwner, "local:facet", read.Attachment.Revision, "reacceptance-propose")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = holder.Namespace.AcceptAttachment(ctx, admin, proposed.Pending, "local:core-a", "reacceptance-accept"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Facets().ActivateFacet(ctx, admin, reacceptEntity, proposed.Attachment.Revision, "reacceptance-activate"); err != nil {
		t.Fatal(err)
	}
	access := proposed.Pending
	notesPayload := fixturePayload("reacceptance-notes")
	stored, err := holder.Namespace.Put(ctx, admin, access, reacceptNotes, notesPayload, facetcontract.ExpectAbsent(), "reacceptance-put")
	if err != nil {
		t.Fatal(err)
	}

	// The element, with its facet attachment record, moves to Core B.
	command := corecontract.TransitionCommand{
		Kind: corecontract.TransitionSlice, Target: ownerB, Boundary: reacceptEntity,
		Expected:      publicElementRevision(t, source, caller, reacceptRoot, reacceptEntity),
		Final:         corecontract.SpatialTuple{Parent: reacceptTargetRoot, LocalPose: mustPose(t, corecontract.Vec3{X: 5}, identity), Range: mustSphere(t, 1)},
		SourceAddress: "local:core-a", TargetAddress: "local:core-b",
	}
	binding0 := corecontract.TransitionApprovalBinding{Source: ownerA, Operation: "reacceptance-transfer", Command: command}
	if _, err = source.Transfers().BeginTransition(ctx, caller, binding0.Operation, binding0.Command); err != nil {
		t.Fatal(err)
	}
	view, err := successor.Transfers().AuthorizeTransitionImport(ctx, caller, binding0)
	if err != nil {
		t.Fatal(err)
	}
	link, err := localtransition.New(source.Transfers(), successor.Transfers(), binding0, caller, view)
	if err != nil {
		t.Fatal(err)
	}
	done, err := source.Transfers().RecoverTransition(ctx, caller, binding0, link)
	if err != nil || done.Progress != corecontract.TransitionSettled {
		t.Fatalf("transfer = %+v, %v", done, err)
	}

	// The successor holds a DETACHED record that still names this Facet; the
	// departed source Core no longer has the element at all.
	transferred, err := successor.Elements().ReadElement(admin, reacceptEntity, nil)
	if err != nil || transferred.Attachment.State != corecontract.FacetDetached ||
		transferred.Attachment.Key.Core != ownerA || transferred.Attachment.Key.Facet != holderOwner {
		t.Fatalf("transferred attachment = %#v, err=%v", transferred.Attachment, err)
	}
	if _, err = source.Elements().ReadElement(admin, reacceptEntity, nil); err == nil {
		t.Fatal("departed source still serves the element")
	}
	if _, err = holder.Namespace.Get(ctx, admin, access, reacceptNotes); !errors.Is(err, facetcontract.ErrElementNotFound) {
		t.Fatalf("detached content still readable: %v", err)
	}

	// The explicit re-acceptance proposal on the successor Core.
	reproposed, err := successor.Facets().ProposeFacet(admin, reacceptEntity, holderOwner, "local:facet", transferred.Attachment.Revision, "reacceptance-repropose")
	if err != nil {
		t.Fatal(err)
	}
	// While the re-acceptance proposal is pending, the obligation protects
	// the element from deletion, the proposal from cancellation and the
	// element from another Core transfer.
	if _, err = successor.Facets().CancelFacetProposal(admin, reacceptEntity, reproposed.Attachment.Revision, "reacceptance-forbidden-cancel"); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("pending re-acceptance cancelled: %v", err)
	}
	if _, err = successor.Elements().DeleteEntity(admin, reacceptEntity, transferred.Record.Entity.Revision, "reacceptance-forbidden-delete"); !errors.Is(err, corecontract.ErrConflict) {
		t.Fatalf("pending re-acceptance deleted: %v", err)
	}
	repeat := corecontract.TransitionCommand{
		Kind: corecontract.TransitionSlice, Target: ownerA, Boundary: reacceptEntity,
		Expected:      publicElementRevision(t, successor, caller, reacceptTargetRoot, reacceptEntity),
		Final:         corecontract.SpatialTuple{Parent: reacceptRoot, LocalPose: mustPose(t, corecontract.Vec3{}, identity), Range: mustSphere(t, 1)},
		SourceAddress: "local:core-b", TargetAddress: "local:core-a",
	}
	repeatBinding := corecontract.TransitionApprovalBinding{Source: ownerB, Operation: "reacceptance-repeat-transfer", Command: repeat}
	if _, err = successor.Transfers().BeginTransition(ctx, caller, repeatBinding.Operation, repeatBinding.Command); !errors.Is(err, corecontract.ErrAttachmentStateConflict) {
		t.Fatalf("pending re-acceptance transferred again: %v", err)
	}

	// The same Facet re-accepts through the successor's proposal; the payload
	// never moves and the previous acceptance lineage is superseded.
	if _, err = holder.Namespace.AcceptAttachment(ctx, admin, reproposed.Pending, "local:core-b", "reacceptance-reaccept"); err != nil {
		t.Fatal(err)
	}
	rebound, err := holder.Namespace.ReadAcceptance(admin, reproposed.Pending)
	if err != nil || !rebound.Found || rebound.CoreNextHop != "local:core-b" {
		t.Fatalf("re-accepted lineage = %#v, err=%v", rebound, err)
	}
	if previous, err := holder.Namespace.ReadAcceptance(admin, access); err != nil || previous.Found {
		t.Fatalf("previous lineage kept: %#v, err=%v", previous, err)
	}
	if _, err = successor.Facets().ActivateFacet(ctx, admin, reacceptEntity, reproposed.Attachment.Revision, "reacceptance-activate-b"); err != nil {
		t.Fatal(err)
	}
	// Once the previous lineage is replaced, its old approval is refused
	// persistently by the departed Core, and the old key never matches the
	// successor's service identity.
	if _, err = holder.Namespace.AcceptAttachment(ctx, admin, access, "local:core-a", "reacceptance-stale-accept"); !errors.Is(err, facetcontract.ErrProposalNotCurrent) {
		t.Fatalf("stale approval accepted: %v", err)
	}
	if _, err = holder.Namespace.AcceptAttachment(ctx, admin, access, "local:core-b", "reacceptance-wrong-core"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("wrong Core identity accepted: %v", err)
	}
	finalRead, err := successor.Elements().ReadElement(admin, reacceptEntity, nil)
	if err != nil || finalRead.Attachment.State != corecontract.FacetActive ||
		finalRead.Attachment.Key != reproposed.Pending || finalRead.Attachment.Key.Facet != holderOwner ||
		finalRead.Attachment.NextHop != "local:facet" {
		t.Fatalf("activated attachment = %#v, err=%v", finalRead.Attachment, err)
	}
	assertPayload(t, mustGetFacet(t, holder, admin, reproposed.Pending, reacceptNotes), notesPayload)
	if _, err = holder.Namespace.Get(ctx, admin, access, reacceptNotes); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("superseded key still writes: %v", err)
	}
	if _, err = holder.Namespace.Put(ctx, admin, reproposed.Pending, reacceptNotes, fixturePayload("reacceptance-next"), facetcontract.ExpectRevision(stored.Revision), "reacceptance-rebound-put"); err != nil {
		t.Fatalf("re-bound key not writable: %v", err)
	}
	if _, err = holder.Namespace.Put(ctx, admin, access, reacceptNotes, fixturePayload("forged"), facetcontract.ExpectRevision(stored.Revision), "reacceptance-stale-put"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("superseded key still writable: %v", err)
	}

	// A completed re-acceptance frees the element: the next Core transfer is
	// accepted again and installs the same chain on the first Core.
	if _, err = successor.Transfers().BeginTransition(ctx, caller, repeatBinding.Operation, repeatBinding.Command); err != nil {
		t.Fatal(err)
	}
	view, err = source.Transfers().AuthorizeTransitionImport(ctx, caller, repeatBinding)
	if err != nil {
		t.Fatal(err)
	}
	link, err = localtransition.New(successor.Transfers(), source.Transfers(), repeatBinding, caller, view)
	if err != nil {
		t.Fatal(err)
	}
	if done, err = successor.Transfers().RecoverTransition(ctx, caller, repeatBinding, link); err != nil || done.Progress != corecontract.TransitionSettled {
		t.Fatalf("repeat transfer = %+v, %v", done, err)
	}
	returned, err := source.Elements().ReadElement(admin, reacceptEntity, nil)
	if err != nil || returned.Attachment.State != corecontract.FacetDetached ||
		returned.Attachment.Key.Core != ownerB || returned.Attachment.Key.Facet != holderOwner {
		t.Fatalf("returned attachment = %#v, err=%v", returned.Attachment, err)
	}

	// The whole switched state survives a full restart of all three owners.
	if err = holder.Close(); err != nil {
		t.Fatal(err)
	}
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	if err = successor.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedA, err := core.OpenDurable(realmLinks, coreAPath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("reopen Core A: %v", err)
	}
	reopenedB, err := core.OpenDurable(realmLinks, coreBPath, corecontract.Config{PublicAccessPolicy: &public})
	if err != nil {
		t.Fatalf("reopen Core B: %v", err)
	}
	if err = coreLinks.BindEndpoint("local:core-a", composition.LocalFacetCoreEndpoint{Read: reopenedA.Facets(), Control: reopenedA.Facets(), Access: reopenedA.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = coreLinks.BindEndpoint("local:core-b", composition.LocalFacetCoreEndpoint{Read: reopenedB.Facets(), Control: reopenedB.Facets(), Access: reopenedB.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = reopenedA.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		t.Fatal(err)
	}
	if err = reopenedB.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
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
		t.Fatalf("replayed previous lineage kept: %#v, err=%v", previous, err)
	}
	persisted, err := reopenedHolder.Namespace.ReadAcceptance(admin, reproposed.Pending)
	if err != nil || !persisted.Found || persisted.CoreNextHop != "local:core-b" {
		t.Fatalf("replayed re-acceptance missing: %#v, err=%v", persisted, err)
	}
	if _, err = reopenedHolder.Namespace.AcceptAttachment(ctx, admin, reproposed.Pending, "local:core-b", "reacceptance-reaccept"); err != nil {
		t.Fatalf("replayed re-acceptance not idempotent: %v", err)
	}
	roundTripped, err := reopenedA.Elements().ReadElement(admin, reacceptEntity, nil)
	if err != nil || roundTripped.Attachment.State != corecontract.FacetDetached ||
		roundTripped.Attachment.Key.Core != ownerB || roundTripped.Attachment.Key.Facet != holderOwner {
		t.Fatalf("reopened Core A attachment = %#v, err=%v", roundTripped.Attachment, err)
	}
	if _, err = reopenedB.Elements().ReadElement(admin, reacceptEntity, nil); err == nil {
		t.Fatal("reopened Core B still serves the departed element")
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
