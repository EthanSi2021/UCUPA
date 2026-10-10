package commit_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestMountGuardExternalParentIsNotAnOwnedElement(t *testing.T) {
	f := newMountGuardFixture(t, false)
	b := f.carrier
	before := commit.CaptureCompleteStateForTest(b.owner)
	if _, err := b.elements.CreateRoot(mountGuardActor, mountGuardID(9102), "root-while-preparing"); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatalf("prepared B CreateRoot: %v", err)
	}
	if !commit.MatchesCompleteStateForTest(b.owner, before) {
		t.Fatal("busy root changed complete state")
	}
	inspection := commit.InspectMountGuardForTest(b.owner, f.mount, "")
	if inspection.Claims != 0 || inspection.RootExists {
		t.Fatal("initial acceptance invented a root or Element claim")
	}
	f.finish(t, "guard-establish", true)
	if _, err := b.elements.CreateRoot(mountGuardActor, mountGuardID(9102), "root-while-preparing"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("active B CreateRoot reused unclaimed OperationID: %v", err)
	}
	entity := mountGuardID(9103)
	created, err := b.elements.CreateEntity(mountGuardActor, entity, mountGuardTuple(t, f.mount, 9, 1), "entity-only")
	if err != nil || created.Status != contract.MutationCommitted {
		t.Fatalf("entity-only B tangent placement: %#v, %v", created, err)
	}
	inspection = commit.InspectMountGuardForTest(b.owner, f.mount, "")
	if inspection.Anchors != 0 || inspection.SelectedClaimed || inspection.SelectedKind != 0 {
		t.Fatal("external M became an owned record/claim/root")
	}
	b.owner.RLock()
	currentErr := b.owner.ValidateCurrentLocked()
	b.owner.RUnlock()
	if currentErr != nil {
		t.Fatalf("entity-only B invalid: %v", currentErr)
	}
	read, err := b.elements.ReadElement(mountGuardActor, f.mount, nil)
	if err != nil || read.State != contract.ExistenceNotFound || read.Record != nil {
		t.Fatalf("external M owner-local read: %#v, %v", read, err)
	}
	for index, create := range []func() (contract.MutationResult, error){
		func() (contract.MutationResult, error) {
			return b.elements.CreateAnchor(mountGuardActor, f.mount, mountGuardTuple(t, f.mount, 0, 1), "claim-external-anchor")
		},
		func() (contract.MutationResult, error) {
			return b.elements.CreateEntity(mountGuardActor, f.mount, mountGuardTuple(t, f.mount, 0, 1), "claim-external-entity")
		},
	} {
		result, err := create()
		if !errors.Is(err, contract.ErrConflict) || result.Status != contract.MutationRejected || commit.InspectMountGuardForTest(b.owner, f.mount, "").SelectedClaimed {
			t.Fatalf("external ID Create %d: %#v, %v", index, result, err)
		}
	}
	outside := mountGuardTuple(t, f.mount, math.Nextafter(9, math.Inf(1)), 1)
	if _, err := b.elements.UpdateEntity(mountGuardActor, entity, outside, created.Record.Entity.Revision, "outside-admission"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("one representable step outside admission: %v", err)
	}
	if _, err := b.elements.DeleteEntity(mountGuardActor, entity, created.Record.Entity.Revision, "delete-only-entity"); err != nil {
		t.Fatal(err)
	}
	inspection = commit.InspectMountGuardForTest(b.owner, f.mount, "")
	if inspection.Anchors != 0 || inspection.Entities != 0 || inspection.Tombstones != 1 {
		t.Fatal("deleting final content lost the external-parent shape")
	}
	b.owner.RLock()
	currentErr = b.owner.ValidateCurrentLocked()
	b.owner.RUnlock()
	if currentErr != nil {
		t.Fatalf("tombstone-only B invalid: %v", currentErr)
	}
}

func TestMountGuardClosedGateDoesNotClaimNewOperations(t *testing.T) {
	for _, kind := range []string{"root", "anchor", "entity", "update-anchor", "update-entity", "delete-anchor", "delete-entity"} {
		t.Run(kind, func(t *testing.T) {
			f := newMountGuardFixture(t, true)
			b := f.carrier
			anchorID, entityID := mountGuardID(9110), mountGuardID(9111)
			a, err := b.elements.CreateAnchor(mountGuardActor, anchorID, mountGuardTuple(t, f.mount, 0, 2), "existing-anchor")
			if err != nil {
				t.Fatal(err)
			}
			e, err := b.elements.CreateEntity(mountGuardActor, entityID, mountGuardTuple(t, f.mount, 0, 1), "existing-entity")
			if err != nil {
				t.Fatal(err)
			}
			f.prepareRange(t, "close-write-gate", 9)
			call := func() (contract.MutationResult, error) {
				const id contract.OperationID = "retry-after-busy"
				switch kind {
				case "root":
					return b.elements.CreateRoot(mountGuardActor, mountGuardID(9112), id)
				case "anchor":
					return b.elements.CreateAnchor(mountGuardActor, mountGuardID(9112), mountGuardTuple(t, f.mount, 0, 1), id)
				case "entity":
					return b.elements.CreateEntity(mountGuardActor, mountGuardID(9112), mountGuardTuple(t, f.mount, 0, 1), id)
				case "update-anchor":
					return b.elements.UpdateAnchor(mountGuardActor, anchorID, mountGuardTuple(t, f.mount, 1, 2), a.Record.Anchor.Revision, id)
				case "update-entity":
					return b.elements.UpdateEntity(mountGuardActor, entityID, mountGuardTuple(t, anchorID, 0, 1), e.Record.Entity.Revision, id)
				case "delete-anchor":
					return b.elements.DeleteAnchor(mountGuardActor, anchorID, a.Record.Anchor.Revision, id)
				default:
					return b.elements.DeleteEntity(mountGuardActor, entityID, e.Record.Entity.Revision, id)
				}
			}
			before := commit.CaptureCompleteStateForTest(b.owner)
			result, err := call()
			if !errors.Is(err, contract.ErrMountBusy) || !reflect.DeepEqual(result, contract.MutationResult{}) || !commit.MatchesCompleteStateForTest(b.owner, before) {
				t.Fatalf("closed gate changed state/claimed ID: %#v, %v", result, err)
			}
			replay, err := b.elements.CreateEntity(mountGuardActor, entityID, mountGuardTuple(t, f.mount, 0, 1), "existing-entity")
			if err != nil || !reflect.DeepEqual(replay, e) {
				t.Fatalf("prior receipt blocked by gate: %#v, %v", replay, err)
			}
			f.finish(t, "close-write-gate", false)
			result, err = call()
			if kind == "root" {
				if !errors.Is(err, contract.ErrConflict) {
					t.Fatalf("B still cannot create a root: %v", err)
				}
			} else if err != nil || result.Status != contract.MutationCommitted {
				t.Fatalf("unclaimed ID could not retry: %#v, %v", result, err)
			}
			if !commit.InspectMountGuardForTest(b.owner, f.mount, "retry-after-busy").OperationRetained {
				t.Fatal("successful post-gate decision was not retained")
			}
		})
	}
}

func TestMountGuardParentRangeRequiresCoordinationAndPoseDoesNot(t *testing.T) {
	f := newMountGuardFixture(t, true)
	a, b := f.parent, f.carrier
	old, err := a.elements.ReadElement(mountGuardActor, f.mount, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := commit.CaptureCompleteStateForTest(a.owner)
	result, err := a.elements.UpdateAnchor(mountGuardActor, f.mount, mountGuardTuple(t, f.root, 0, 9), old.Record.Anchor.Revision, "needs-coordination")
	if !errors.Is(err, contract.ErrMountCoordinationRequired) || !reflect.DeepEqual(result, contract.MutationResult{}) || !commit.MatchesCompleteStateForTest(a.owner, before) {
		t.Fatalf("Range edit bypassed coordinator or claimed ID: %#v, %v", result, err)
	}
	f.prepareRange(t, "needs-coordination", 9)
	pending := commit.CaptureCompleteStateForTest(a.owner)
	result, err = a.elements.UpdateAnchor(mountGuardActor, f.mount, mountGuardTuple(t, f.root, 1, 10), old.Record.Anchor.Revision, "pose-while-pending")
	if !errors.Is(err, contract.ErrMountBusy) || !reflect.DeepEqual(result, contract.MutationResult{}) || !commit.MatchesCompleteStateForTest(a.owner, pending) {
		t.Fatalf("pending M ordinary edit was admitted: %#v, %v", result, err)
	}
	f.finish(t, "needs-coordination", false)
	bBefore := commit.CaptureCompleteStateForTest(b.owner)
	result, err = a.elements.UpdateAnchor(mountGuardActor, f.mount, mountGuardTuple(t, f.root, 1, 10), old.Record.Anchor.Revision, "pose-while-pending")
	if err != nil || result.Status != contract.MutationCommitted || !commit.MatchesCompleteStateForTest(b.owner, bBefore) {
		t.Fatalf("Pose-only edit changed B admission/content: %#v, %v", result, err)
	}
	for _, kind := range []string{"anchor", "entity"} {
		id := contract.OperationID("local-child-" + kind)
		var err error
		if kind == "anchor" {
			_, err = a.elements.CreateAnchor(mountGuardActor, mountGuardID(9120), mountGuardTuple(t, f.mount, 0, 1), id)
		} else {
			_, err = a.elements.CreateEntity(mountGuardActor, mountGuardID(9120), mountGuardTuple(t, f.mount, 0, 1), id)
		}
		if !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("A accepted local %s below M: %v", kind, err)
		}
	}
	f.detach(t)
	if _, err := a.elements.DeleteAnchor(mountGuardActor, f.mount, result.Record.Anchor.Revision, "delete-detached"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("detached M was deleted: %v", err)
	}
	if _, err := b.elements.CreateEntity(mountGuardActor, mountGuardID(9121), mountGuardTuple(t, f.mount, 0, 1), "write-detached"); !errors.Is(err, contract.ErrMountBusy) {
		t.Fatalf("detached B accepted a write: %v", err)
	}
}

func TestMountGuardCarrierReparentStillProvesLocalStructure(t *testing.T) {
	f := newMountGuardFixture(t, true)
	b := f.carrier
	outerID, innerID, entityID := mountGuardID(9170), mountGuardID(9171), mountGuardID(9172)
	outer, err := b.elements.CreateAnchor(mountGuardActor, outerID, mountGuardTuple(t, f.mount, 0, 5), "outer")
	if err != nil {
		t.Fatal(err)
	}
	inner, err := b.elements.CreateAnchor(mountGuardActor, innerID, mountGuardTuple(t, outerID, 2, 2), "inner")
	if err != nil {
		t.Fatal(err)
	}
	entity, err := b.elements.CreateEntity(mountGuardActor, entityID, mountGuardTuple(t, innerID, 0, 1), "entity")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.elements.UpdateAnchor(mountGuardActor, outerID, mountGuardTuple(t, innerID, 0, 1), outer.Record.Anchor.Revision, "cycle"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("external-parent B accepted local cycle: %v", err)
	}
	if _, err := b.elements.UpdateAnchor(mountGuardActor, outerID, mountGuardTuple(t, f.mount, 0, 3), outer.Record.Anchor.Revision, "child-outside"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("external-parent B lost local child containment: %v", err)
	}
	if _, err := b.elements.UpdateEntity(mountGuardActor, entityID, mountGuardTuple(t, entityID, 0, 1), entity.Record.Entity.Revision, "non-anchor-parent"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("entity used as parent: %v", err)
	}
	if _, err := b.elements.UpdateEntity(mountGuardActor, entityID, mountGuardTuple(t, mountGuardID(9199), 0, 1), entity.Record.Entity.Revision, "unrelated-parent"); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("unrelated external parent accepted: %v", err)
	}
	beforeEntity, err := b.elements.ReadElement(mountGuardActor, entityID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.elements.UpdateAnchor(mountGuardActor, innerID, mountGuardTuple(t, f.mount, 7, 2), inner.Record.Anchor.Revision, "move-to-external-parent"); err != nil {
		t.Fatalf("valid reparent to declared M: %v", err)
	}
	afterEntity, err := b.elements.ReadElement(mountGuardActor, entityID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if *afterEntity.Record.Entity != *beforeEntity.Record.Entity {
		t.Fatal("reparent rewrote the descendant's local tuple")
	}
	f.prepareRange(t, "shrink-to-nine", 9)
	f.finish(t, "shrink-to-nine", true)
	if commit.InspectMountGuardForTest(b.owner, f.mount, "").IncomingRange != 9 {
		t.Fatal("new admission not installed")
	}
	if _, err := b.elements.CreateEntity(mountGuardActor, mountGuardID(9173), mountGuardTuple(t, f.mount, 9, 1), "outside-new-admission"); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("write continued to use old admission: %v", err)
	}
	if _, err := b.elements.CreateEntity(mountGuardActor, mountGuardID(9174), mountGuardTuple(t, f.mount, 8, 1), "tangent-new-admission"); err != nil {
		t.Fatalf("new admission tangent rejected: %v", err)
	}
	if err := commit.ValidateCompleteStateForTest(b.owner); err != nil {
		t.Fatalf("reparent/admission history: %v", err)
	}
}
