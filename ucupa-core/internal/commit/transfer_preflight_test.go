package commit

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func transferSource(t *testing.T, o *Owner, id values.ElementID, expected contract.Revision, limit int) (transferfact.Slice, error) {
	t.Helper()
	o.RLock()
	defer o.RUnlock()
	return o.PreflightTransferSourceLocked(id, expected, limit)
}

func TestTransferPreflightUsesRealDurableFactsAndPreservesOutcomes(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	ids := seedFoundationStore(t, sourcePath)
	source := openFoundationStore(t, sourcePath)
	defer source.CloseStore()
	before := source.state.clone()
	anchor, _ := source.state.elements.Anchor(ids.anchorID)
	candidate, err := transferSource(t, source, ids.anchorID, anchor.Revision, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Members()) != 2 || candidate.Source() != source.AuthorityRef() || candidate.Cut() != before.currentCut() {
		t.Fatalf("source capture: %+v", candidate)
	}
	target := createFoundationStore(t, filepath.Join(t.TempDir(), "target.db"))
	defer target.CloseStore()
	root := foundationStoreID(500)
	result, err := applyFoundationStore(target, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "target-root")
	if err != nil || result.Status != contract.MutationCommitted {
		t.Fatalf("target root: %+v %v", result, err)
	}
	targetBefore := target.state.clone()
	final := foundationStoreTuple(t, root, 123, 20)
	target.RLock()
	placement, err := target.PreflightTransferTargetLocked(candidate, final, 3)
	target.RUnlock()
	if err != nil || placement.Final() != final || placement.Target() != target.AuthorityRef() {
		t.Fatalf("target placement: %+v %v", placement, err)
	}
	if !sameCompleteCoreState(before, source.state) || !sameCompleteCoreState(targetBefore, target.state) {
		t.Fatal("preflight changed facts, operation receipts, cut or authority")
	}
	// The preflight did not reserve/fence anything, and cannot be reused as a
	// commit decision. Writes remain legal and fresh capture sees another cut.
	entity, _ := source.state.elements.Entity(ids.entityID)
	changed, err := applyFoundationStore(source, elementfact.OperationUpdateEntity, foundationStoreActor, ids.entityID,
		foundationStoreTuple(t, ids.anchorID, 2, 2), entity.Revision, "after-preflight")
	if err != nil || changed.Status != contract.MutationCommitted {
		t.Fatalf("write after preflight: %+v %v", changed, err)
	}
	fresh, err := transferSource(t, source, ids.anchorID, anchor.Revision, 3)
	if err != nil || fresh.Cut() == candidate.Cut() || reflect.DeepEqual(fresh.Members(), candidate.Members()) {
		t.Fatalf("fresh source: %+v %v", fresh, err)
	}
	// The boundary revision alone is not sufficient: its child changed. This
	// test prevents treating a previously returned observation as a final cut.
	if candidate.Members()[1].Entity.LocalPose.Translation.X != 1 {
		t.Fatal("old observed member changed")
	}
	closeFoundationStore(t, source)
	reopened := openFoundationStore(t, sourcePath)
	defer reopened.CloseStore()
	afterRestart, err := transferSource(t, reopened, ids.anchorID, anchor.Revision, 3)
	if err != nil || !reflect.DeepEqual(fresh, afterRestart) {
		t.Fatalf("restart capture: %+v %v", afterRestart, err)
	}
	replay, err := applyFoundationStore(reopened, elementfact.OperationUpdateEntity, foundationStoreActor, ids.entityID,
		foundationStoreTuple(t, ids.anchorID, 2, 2), entity.Revision, "after-preflight")
	if err != nil || !reflect.DeepEqual(replay, changed) {
		t.Fatalf("retained result changed: %+v %v", replay, err)
	}
}

func TestTransferPreflightExpectedAndClosedOwner(t *testing.T) {
	o, _, _, installation := writerFixture(t)
	root, leaf := foundationStoreID(800), foundationStoreID(801)
	_, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "root")
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyFoundationStore(o, elementfact.OperationCreateEntity, foundationStoreActor, leaf, foundationStoreTuple(t, root, 0, 1), contract.Revision{}, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	expected := result.Record.Entity.Revision
	if _, err := transferSource(t, o, leaf, contract.Revision{}, 2); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("zero expected: %v", err)
	}
	if _, err := transferSource(t, o, leaf, expected, 1); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("capacity: %v", err)
	}
	updated, err := applyFoundationStore(o, elementfact.OperationUpdateEntity, foundationStoreActor, leaf, foundationStoreTuple(t, root, 2, 1), expected, "update")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transferSource(t, o, leaf, expected, 2); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("stale expected: %v", err)
	}
	candidate, err := transferSource(t, o, leaf, updated.Record.Entity.Revision, 2)
	if err != nil {
		t.Fatal(err)
	}
	o.Lock()
	_, err = o.HandoffWriterLocked(foundationStoreActor, writerRequest(installation, 1))
	o.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transferSource(t, o, leaf, updated.Record.Entity.Revision, 2); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("closed writer disclosed: %v", err)
	}
	o.RLock()
	_, err = o.PreflightTransferTargetLocked(candidate, foundationStoreTuple(t, root, 0, 1), 2)
	o.RUnlock()
	if !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("closed writer accepted target preflight: %v", err)
	}
}

func TestTransferPreflightRejectsInvalidCurrentGraph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	ids := seedFoundationStore(t, path)
	o := openFoundationStore(t, path)
	defer o.CloseStore()
	entity, _ := o.state.elements.Entity(ids.entityID)
	o.Lock()
	next := o.state.clone()
	broken, _ := next.elements.Anchor(ids.anchorID)
	broken.Parent = ids.anchorID
	if err := next.elements.RestoreAnchor(broken); err != nil {
		t.Fatal(err)
	}
	o.state = next // fixed corruption in the sole owner, never a production hook
	o.Unlock()
	if _, err := transferSource(t, o, ids.entityID, entity.Revision, 4); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("broken graph: %v", err)
	}
}
