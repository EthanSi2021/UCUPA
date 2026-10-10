package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The fixture is real durable history: root -> grandparent -> parent -> leaf.
// An unrelated sibling remains under grandparent. Only leaf will leave.
func transferAncestorFixture(t *testing.T) (*Owner, string, transferfact.Intent) {
	t.Helper()
	o, path, input := transferIntentFixture(t)
	grandparent := foundationStoreID(110)
	if _, err := applyFoundationStore(o, elementfact.OperationCreateAnchor, input.Actor, grandparent,
		foundationStoreTuple(t, foundationStoreID(100), 0, 100), contract.Revision{}, "grandparent"); err != nil {
		t.Fatal(err)
	}
	parent, _ := o.state.elements.Anchor(input.Boundary)
	if _, err := applyFoundationStore(o, elementfact.OperationUpdateAnchor, input.Actor, parent.ID,
		foundationStoreTuple(t, grandparent, 0, 20), parent.Revision, "parent-before-fence"); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFoundationStore(o, elementfact.OperationCreateAnchor, input.Actor, foundationStoreID(111),
		foundationStoreTuple(t, grandparent, 30, 10), contract.Revision{}, "sibling"); err != nil {
		t.Fatal(err)
	}
	leaf, _ := o.state.elements.Entity(foundationStoreID(102))
	input.Boundary, input.Expected = leaf.ID, leaf.Revision
	return o, path, input
}

func TestTransferFenceHoldsAncestorsNotUnrelatedWrites(t *testing.T) {
	o, path, input := transferAncestorFixture(t)
	intent, err := beginTransfer(t, o, input)
	if err != nil {
		t.Fatal(err)
	}
	fenced, err := fenceTransfer(o, input, intent.Revision)
	if err != nil || fenced.Origin.Parent != foundationStoreID(101) {
		t.Fatalf("origin parent: %+v %v", fenced, err)
	}
	check := func(owner *Owner) {
		t.Helper()
		for _, id := range []values.ElementID{foundationStoreID(101), foundationStoreID(110)} {
			anchor, _ := owner.state.elements.Anchor(id)
			for _, kind := range []elementfact.OperationKind{elementfact.OperationUpdateAnchor, elementfact.OperationDeleteAnchor} {
				final := contract.SpatialTuple{}
				if kind == elementfact.OperationUpdateAnchor {
					final = foundationStoreTuple(t, foundationStoreID(100), 5, 5)
				}
				before := owner.state.clone()
				if _, err := applyFoundationStore(owner, kind, input.Actor, id, final, anchor.Revision, "blocked-ancestor"); !errors.Is(err, values.PublicUnavailable) {
					t.Fatalf("ancestor %s/%d: %v", id, kind, err)
				}
				if !sameCompleteCoreState(before, owner.state) {
					t.Fatal("blocked ancestor claimed a result or changed facts")
				}
			}
		}
	}
	check(o)
	// The parent's direct content became empty, but it must not be deleted,
	// shrunk, reparented, transferred or converted to a new carrier boundary.
	if o.state.elements.HasDirectContent(foundationStoreID(101)) {
		t.Fatal("fixture did not expose the empty-parent counterexample")
	}
	// New siblings and writes under an unrelated sibling do not change the
	// held ancestor tuples. A broad Core/subtree write pause would fail here.
	for _, parent := range []values.ElementID{foundationStoreID(101), foundationStoreID(111)} {
		id := foundationStoreID(120)
		op := contract.OperationID("new-under-parent")
		if parent == foundationStoreID(111) {
			id, op = foundationStoreID(121), "new-under-sibling"
		}
		if _, err := applyFoundationStore(o, elementfact.OperationCreateEntity, input.Actor, id,
			foundationStoreTuple(t, parent, 0, 1), contract.Revision{}, op); err != nil {
			t.Fatal(err)
		}
	}
	branch, _ := o.state.elements.Anchor(foundationStoreID(111))
	if _, err := applyFoundationStore(o, elementfact.OperationUpdateAnchor, input.Actor, branch.ID,
		foundationStoreTuple(t, branch.Parent, 35, 10), branch.Revision, "move-sibling"); err != nil {
		t.Fatal(err)
	}
	beforeRestart := o.state.clone()
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	if !sameCompleteCoreState(beforeRestart, reopened.state) {
		t.Fatal("restart changed derived ancestor obligation")
	}
	check(reopened)
	// The held parent remains authoritative for existing operation outcomes.
	old, err := applyFoundationStore(reopened, elementfact.OperationUpdateAnchor, input.Actor, foundationStoreID(101),
		foundationStoreTuple(t, foundationStoreID(110), 0, 20), o.state.operations["parent-before-fence"].binding.Expected, "parent-before-fence")
	if err != nil || old.Status != contract.MutationCommitted {
		t.Fatalf("original parent outcome: %+v %v", old, err)
	}
}

func TestTransferFenceBlocksMountReservationAndOverlappingTransfer(t *testing.T) {
	o, path, input := transferAncestorFixture(t)
	parent, _ := o.state.elements.Anchor(foundationStoreID(101))
	parentInput := input
	parentInput.Operation, parentInput.Boundary, parentInput.Expected = "parent-transfer", parent.ID, parent.Revision
	parentIntent, err := beginTransfer(t, o, parentInput)
	if err != nil {
		t.Fatal(err)
	}
	leafIntent, err := beginTransfer(t, o, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, input, leafIntent.Revision); err != nil {
		t.Fatal(err)
	}
	check := func(owner *Owner) {
		t.Helper()
		before := owner.state.clone()
		if _, err := fenceTransfer(owner, parentInput, parentIntent.Revision); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("older ancestor intent fenced an incomplete closure: %v", err)
		}
		if _, err := transferSource(t, owner, parent.ID, parent.Revision, 100); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("preflight omitted source gap: %v", err)
		}
		another := parentInput
		another.Operation = "new-parent-transfer"
		if _, err := beginTransfer(t, owner, another); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("new overlapping intent: %v", err)
		}
		// Both parent and departed leaf must fail before reserving any mount
		// control ID, even though an empty parent could otherwise host a mount.
		for _, id := range []values.ElementID{parent.ID, input.Boundary} {
			expected := parent.Revision
			if id == input.Boundary {
				expected = input.Expected
			}
			binding := MountBinding{Actor: input.Actor, PeerActor: input.Actor, Command: contract.MountCommand{
				Kind: contract.MountEstablish, MountAnchorUUID: id, ExpectedElementRevision: expected,
				CarrierAuthority: input.Target, CarrierNextHop: "local:carrier", ParentNextHop: "local:parent"}}
			owner.Lock()
			next, err := owner.state.prepareMountIntent(contract.LocalCallers{input.Target: input.Actor}, "blocked-mount", binding)
			owner.Unlock()
			if next != nil || !errors.Is(err, values.PublicUnavailable) {
				t.Fatalf("mount bypass at %s: %v", id, err)
			}
		}
		if !sameCompleteCoreState(before, owner.state) {
			t.Fatal("refused control left a reservation or cut")
		}
	}
	check(o)
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	check(reopened)
	// The older, still-unfenced intent may be explicitly cancelled. It cannot
	// be resumed with a silently smaller closure or force another fence open.
	if _, err := cancelTransfer(t, reopened, parentInput, parentIntent.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestTransferAncestorWriteRacingFencePreservesFinalRecipe(t *testing.T) {
	for n := 0; n < 8; n++ {
		o, path, input := transferAncestorFixture(t)
		intent, err := beginTransfer(t, o, input)
		if err != nil {
			t.Fatal(err)
		}
		parent, _ := o.state.elements.Anchor(foundationStoreID(101))
		start := make(chan struct{})
		written, frozen := make(chan error, 1), make(chan error, 1)
		go func() {
			<-start
			_, err := applyFoundationStore(o, elementfact.OperationUpdateAnchor, input.Actor, parent.ID,
				foundationStoreTuple(t, foundationStoreID(100), 3, 20), parent.Revision, "racing-ancestor")
			written <- err
		}()
		go func() { <-start; _, err := fenceTransfer(o, input, intent.Revision); frozen <- err }()
		close(start)
		writeErr, fenceErr := <-written, <-frozen
		if fenceErr != nil || (writeErr != nil && !errors.Is(writeErr, values.PublicUnavailable)) {
			t.Fatalf("ancestor/fence: %v %v", writeErr, fenceErr)
		}
		before := o.state.clone()
		closeFoundationStore(t, o)
		reopened := openFoundationStore(t, path)
		// Replay independently reconstructs the exact manifest, including the
		// winning ancestor tuple and cut, not just the leaf's unchanged revision.
		if !sameCompleteCoreState(before, reopened.state) {
			t.Fatal("replayed ancestor recipe disagrees with the fence")
		}
		closeFoundationStore(t, reopened)
	}
}

func TestTransferFenceRefusesNewFacetDecisionsButRetainsOriginalOutcome(t *testing.T) {
	o, path, input := transferAncestorFixture(t)
	relation, _ := o.state.attachments.facts.Record(input.Boundary)
	proposal := facetfact.Binding{Actor: input.Actor, Kind: facetfact.Propose, Element: input.Boundary,
		Target: input.Target, Address: "local:facet", Expected: relation.Attachment.Revision}
	o.Lock()
	proposed, code, err := o.ApplyFacetLocked("original-proposal", proposal, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	o.Unlock()
	if err != nil || code != facetfact.OK || proposed.Status != contract.MutationCommitted {
		t.Fatalf("proposal: %+v %v %v", proposed, code, err)
	}
	relation, _ = o.state.attachments.facts.Record(input.Boundary)
	cancel := facetfact.Binding{Actor: input.Actor, Kind: facetfact.Cancel, Element: input.Boundary, Expected: relation.Attachment.Revision}
	o.Lock()
	_, code, err = o.ApplyFacetLocked("original-cancel", cancel, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	o.Unlock()
	if err != nil || code != facetfact.OK {
		t.Fatalf("cancel: %v %v", code, err)
	}
	intent, err := beginTransfer(t, o, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, input, intent.Revision); err != nil {
		t.Fatal(err)
	}
	check := func(owner *Owner) {
		t.Helper()
		owner.Lock()
		defer owner.Unlock()
		before := owner.state.clone()
		result, err, found := owner.LookupFacetLocked("original-proposal", proposal)
		if !found || err != nil || result != proposed {
			t.Fatalf("original proposal: %+v %v %t", result, err, found)
		}
		if _, err, found := owner.LookupFacetLocked("new-proposal", proposal); !found || !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("departure was treated as absent: %v %t", err, found)
		}
		if _, _, err := owner.ApplyFacetLocked("new-proposal", proposal, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{}); !errors.Is(err, values.PublicUnavailable) {
			t.Fatalf("direct submission bypassed fence: %v", err)
		}
		if !sameCompleteCoreState(before, owner.state) {
			t.Fatal("refusal retained a new Facet result")
		}
		// A syntactically valid old-style missing decision is not recoverable
		// history once its admission follows the source fence.
		forged, _, missing := before.prepareAttachment("forged-missing", proposal, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
		if missing != facetfact.Missing || validateOperationHistory(forged) == nil {
			t.Fatal("history accepted a fake missing result after departure")
		}
	}
	check(o)
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	check(reopened)
}

func TestTransferFenceReplayRejectsForgedAncestorWriteAndHold(t *testing.T) {
	o, _, input := transferAncestorFixture(t)
	intent, err := beginTransfer(t, o, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, input, intent.Revision); err != nil {
		t.Fatal(err)
	}
	forged := o.state.clone()
	parent, _ := forged.elements.Anchor(foundationStoreID(101))
	binding := commitbinding.Binding{Kind: elementfact.OperationUpdateAnchor, Actor: input.Actor, Element: parent.ID,
		Expected: parent.Revision, Final: foundationStoreTuple(t, foundationStoreID(100), 5, 20)}
	before := forged.currentCut()
	// Bypass only the admission gate to construct otherwise valid old-style
	// Element history. Original recovery must reject it, not bless the new tuple.
	outcome, err := decideMutation(forged, binding)
	if err != nil || outcome.Status != contract.MutationCommitted {
		t.Fatalf("forged fixture: %+v %v", outcome, err)
	}
	forged.operations["forged-parent"] = retainedOperation{binding: binding, admission: before, outcome: outcome}
	forged.commits = append(forged.commits, coreCommit{kind: elementCommitted, operation: "forged-parent"})
	if validateOperationHistory(forged) == nil {
		t.Fatal("replay accepted ancestor movement after durable fence")
	}
	forged = o.state.clone()
	record := forged.transfers.records[input.Operation]
	record.Origin.Parent = foundationStoreID(100)
	forged.transfers.records[input.Operation] = record
	if validateOperationHistory(forged) == nil {
		t.Fatal("derived source parent was not bound to the manifest")
	}
}
