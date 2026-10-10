package commit_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestDurableAnchorRangeShrinkReopenAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anchor-shrink.db")
	fixture := seedRangeStore(t, path)
	spatial := openRangeStore(t, path)
	before := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
	rootBefore := readAnchorRangeStoreElement(t, spatial, fixture.rootID)
	childBefore := readAnchorRangeStoreElement(t, spatial, fixture.entityID)
	final := rangeStoreTuple(t, fixture.rootID, 0, 3) // Child at x=1 with radius=2 is tangent.
	expected := before.Record.Anchor.Revision
	shrunk, err := spatial.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "legal-shrink")
	if err != nil || shrunk.Status != corecontract.MutationCommitted || shrunk.Record == nil ||
		shrunk.Record.Anchor == nil || shrunk.Record.Anchor.Revision.Equal(expected) ||
		shrunk.OutcomeCut.Equal(before.Cut) {
		t.Fatalf("legal shrink: %#v err=%v", shrunk, err)
	}
	if sphere, ok := shrunk.Record.Anchor.Range.BoundedSphere(); !ok || sphere.Radius() != 3 {
		t.Fatalf("wrong shrunk Range: %#v", shrunk.Record.Anchor.Range)
	}
	for id, want := range map[values.ElementID]corecontract.ElementRead{
		fixture.rootID: rootBefore, fixture.entityID: childBefore,
	} {
		got := readAnchorRangeStoreElement(t, spatial, id)
		if !reflect.DeepEqual(got.Record, want.Record) {
			t.Fatalf("shrink changed another record %q: got=%#v want=%#v", id, got.Record, want.Record)
		}
	}
	closeRangeStore(t, spatial)

	reopened := openRangeStore(t, path)
	defer closeRangeStore(t, reopened)
	read := readAnchorRangeStoreElement(t, reopened, fixture.anchorID)
	if !reflect.DeepEqual(read.Record, shrunk.Record) || !read.Cut.Equal(shrunk.OutcomeCut) {
		t.Fatalf("reopened shrink: got=%#v want=%#v", read, shrunk)
	}
	replay, err := reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "legal-shrink")
	if err != nil || !reflect.DeepEqual(replay, shrunk) {
		t.Fatalf("reopened shrink replay: got=%#v err=%v want=%#v", replay, err, shrunk)
	}
	unchanged, err := reopened.elements.UpdateAnchor(
		rangeStoreActor, fixture.anchorID, final, shrunk.Record.Anchor.Revision, "unchanged-after-shrink")

	if err != nil || unchanged.Status != corecontract.MutationUnchanged || !unchanged.OutcomeCut.Equal(shrunk.OutcomeCut) {
		t.Fatalf("unchanged after shrink: %#v err=%v", unchanged, err)
	}

	expanded, err := reopened.elements.UpdateAnchor(
		rangeStoreActor, fixture.anchorID, rangeStoreTuple(t, fixture.rootID, 0, 4),
		shrunk.Record.Anchor.Revision, "expand-after-shrink")

	if err != nil || expanded.Status != corecontract.MutationCommitted {
		t.Fatalf("later expansion: %#v err=%v", expanded, err)
	}
	replay, err = reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "legal-shrink")
	if err != nil || !reflect.DeepEqual(replay, shrunk) {
		t.Fatalf("historical shrink replay: got=%#v err=%v want=%#v", replay, err, shrunk)
	}
	current := readAnchorRangeStoreElement(t, reopened, fixture.anchorID)
	if !reflect.DeepEqual(current.Record, expanded.Record) || !current.Cut.Equal(expanded.OutcomeCut) {
		t.Fatalf("historical replay changed current state: got=%#v want=%#v", current, expanded)
	}
}

func TestDurableAnchorRangeRejectedShrinkRemainsRejectedAfterChildMoves(t *testing.T) {
	for _, childKind := range []string{"entity", "anchor"} {
		t.Run(childKind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rejected-shrink.db")
			fixture := seedRangeStore(t, path)
			spatial := openRangeStore(t, path)
			childID := fixture.entityID
			child := readAnchorRangeStoreElement(t, spatial, childID)
			if childKind == "anchor" {
				deleted, err := spatial.elements.DeleteEntity(
					rangeStoreActor, childID, child.Record.Entity.Revision, "replace-fixture-child")

				if err != nil || deleted.Status != corecontract.MutationCommitted {
					t.Fatalf("delete fixture child: %#v err=%v", deleted, err)
				}
				childID = rangeStoreID(104)
				created, err := spatial.elements.CreateAnchor(
					rangeStoreActor, childID, rangeStoreTuple(t, fixture.anchorID, 1, 2), "direct-child-anchor")

				if err != nil || created.Status != corecontract.MutationCommitted {
					t.Fatalf("create child Anchor: %#v err=%v", created, err)
				}
				child = readAnchorRangeStoreElement(t, spatial, childID)
			}
			before := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
			expected := before.Record.Anchor.Revision
			final := rangeStoreTuple(t, fixture.rootID, 0, 2)
			rejected, err := spatial.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "blocked-shrink")
			if !errors.Is(err, corecontract.ErrConflict) || rejected.Status != corecontract.MutationRejected ||
				rejected.Record != nil || !rejected.OutcomeCut.Equal(before.Cut) {
				t.Fatalf("out-of-Range child was accepted: %#v err=%v", rejected, err)
			}
			after := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected shrink changed facts: got=%#v want=%#v", after, before)
			}
			closeRangeStore(t, spatial)

			reopened := openRangeStore(t, path)
			replay, err := reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "blocked-shrink")
			if !errors.Is(err, corecontract.ErrConflict) || !reflect.DeepEqual(replay, rejected) {
				t.Fatalf("reopened rejection: got=%#v err=%v want=%#v", replay, err, rejected)
			}
			childFinal := rangeStoreTuple(t, fixture.anchorID, 0, 2)
			var moved corecontract.MutationResult
			if childKind == "anchor" {
				moved, err = reopened.elements.UpdateAnchor(rangeStoreActor, childID, childFinal, child.Record.Anchor.Revision, "move-child-inward")
			} else {
				moved, err = reopened.elements.UpdateEntity(rangeStoreActor, childID, childFinal, child.Record.Entity.Revision, "move-child-inward")
			}
			if err != nil || moved.Status != corecontract.MutationCommitted {
				t.Fatalf("move child inward: %#v err=%v", moved, err)
			}
			replay, err = reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "blocked-shrink")
			if !errors.Is(err, corecontract.ErrConflict) || !reflect.DeepEqual(replay, rejected) {
				t.Fatalf("child move changed retained rejection: got=%#v err=%v want=%#v", replay, err, rejected)
			}
			closeRangeStore(t, reopened)

			ready := openRangeStore(t, path)
			replay, err = ready.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "blocked-shrink")
			if !errors.Is(err, corecontract.ErrConflict) || !reflect.DeepEqual(replay, rejected) {
				t.Fatalf("reopened rejection after child move: got=%#v err=%v want=%#v", replay, err, rejected)
			}
			accepted, err := ready.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "new-shrink")
			if err != nil || accepted.Status != corecontract.MutationCommitted || accepted.Record == nil {
				t.Fatalf("new OperationID after child move: %#v err=%v", accepted, err)
			}
			closeRangeStore(t, ready)

			last := openRangeStore(t, path)
			defer closeRangeStore(t, last)
			replay, err = last.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "blocked-shrink")
			if !errors.Is(err, corecontract.ErrConflict) || !reflect.DeepEqual(replay, rejected) {
				t.Fatalf("old rejection after successful shrink: got=%#v err=%v want=%#v", replay, err, rejected)
			}
			replay, err = last.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "new-shrink")
			if err != nil || !reflect.DeepEqual(replay, accepted) {
				t.Fatalf("new success after reopen: got=%#v err=%v want=%#v", replay, err, accepted)
			}
		})
	}
}

func TestDurableAnchorRangeShrinkPreCommitFailureIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shrink-precommit.db")
	fixture := seedRangeStore(t, path)
	spatial := openRangeStore(t, path)
	before := commit.CaptureCompleteStateForTest(spatial.owner)
	anchor := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
	expected := anchor.Record.Anchor.Revision
	final := rangeStoreTuple(t, fixture.rootID, 0, 3)
	injected := errors.New("injected Anchor shrink precommit failure")
	commit.InjectWriteFailureForTest(spatial.owner, false, injected)
	failed, err := spatial.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "failed-shrink")
	if !errors.Is(err, corecontract.ErrStore) || !errors.Is(err, injected) || failed != (corecontract.MutationResult{}) {
		t.Fatalf("failed shrink: %#v err=%v", failed, err)
	}
	if !commit.MatchesCompleteStateForTest(spatial.owner, before) {
		t.Fatal("precommit failure changed facts, cuts, or retained outcomes")
	}
	closeRangeStore(t, spatial)

	reopened := openRangeStore(t, path)
	defer closeRangeStore(t, reopened)
	if !commit.MatchesCompleteStateForTest(reopened.owner, before) {
		t.Fatal("precommit failure changed durable facts, cuts, or retained outcomes")
	}
	retry, err := reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "failed-shrink")
	if err != nil || retry.Status != corecontract.MutationCommitted || retry.Record == nil {
		t.Fatalf("retry rolled-back shrink: %#v err=%v", retry, err)
	}
	replay, err := reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "failed-shrink")
	if err != nil || !reflect.DeepEqual(replay, retry) {
		t.Fatalf("retry replay: got=%#v err=%v want=%#v", replay, err, retry)
	}
}

func TestDurableAnchorRangeShrinkLostResponseReloadsAndReplays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shrink-lost-response.db")
	fixture := seedRangeStore(t, path)
	spatial := openRangeStore(t, path)
	before := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
	expected := before.Record.Anchor.Revision
	final := rangeStoreTuple(t, fixture.rootID, 0, 3)
	injected := errors.New("injected Anchor shrink lost response")
	commit.InjectWriteFailureForTest(spatial.owner, true, injected)
	lost, err := spatial.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "lost-shrink")
	if !errors.Is(err, corecontract.ErrStore) || !errors.Is(err, injected) || lost != (corecontract.MutationResult{}) {
		t.Fatalf("lost shrink response: %#v err=%v", lost, err)
	}
	after := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
	sphere, ok := after.Record.Anchor.Range.BoundedSphere()
	if !ok || sphere.Radius() != 3 || after.Cut.Equal(before.Cut) || after.Record.Anchor.Revision.Equal(expected) {
		t.Fatalf("committed shrink was not reloaded: %#v", after)
	}
	replay, err := spatial.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "lost-shrink")
	if err != nil || replay.Status != corecontract.MutationCommitted || !replay.OutcomeCut.Equal(after.Cut) ||
		!reflect.DeepEqual(replay.Record, after.Record) {
		t.Fatalf("same-instance lost-response replay: %#v err=%v want=%#v", replay, err, after)
	}
	// A subsequent commit must preserve the shrink recovered from Store.
	created, err := spatial.elements.CreateEntity(
		rangeStoreActor, rangeStoreID(105), rangeStoreTuple(t, fixture.anchorID, -1, 1), "after-lost-shrink")

	if err != nil || created.Status != corecontract.MutationCommitted {
		t.Fatalf("later write after lost shrink: %#v err=%v", created, err)
	}
	closeRangeStore(t, spatial)

	reopened := openRangeStore(t, path)
	defer closeRangeStore(t, reopened)
	reopenedReplay, err := reopened.elements.UpdateAnchor(rangeStoreActor, fixture.anchorID, final, expected, "lost-shrink")
	if err != nil || !reflect.DeepEqual(reopenedReplay, replay) {
		t.Fatalf("reopened lost-response replay: got=%#v err=%v want=%#v", reopenedReplay, err, replay)
	}
	current := readAnchorRangeStoreElement(t, reopened, fixture.anchorID)
	if !reflect.DeepEqual(current.Record, after.Record) || !current.Cut.Equal(created.OutcomeCut) {
		t.Fatalf("later commit replaced recovered shrink: %#v", current)
	}
	readAnchorRangeStoreElement(t, reopened, rangeStoreID(105))
}

func TestDurableAnchorRangeReplayRejectsInvalidShrinkHiddenByExpansion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-shrink-history.db")
	fixture := seedRangeStore(t, path)
	spatial := openRangeStore(t, path)
	before := readAnchorRangeStoreElement(t, spatial, fixture.anchorID)
	shrunk, err := spatial.elements.UpdateAnchor(
		rangeStoreActor, fixture.anchorID, rangeStoreTuple(t, fixture.rootID, 0, 3),
		before.Record.Anchor.Revision, "historical-shrink")

	if err != nil || shrunk.Status != corecontract.MutationCommitted || shrunk.Record == nil {
		t.Fatalf("valid historical shrink: %#v err=%v", shrunk, err)
	}
	expanded, err := spatial.elements.UpdateAnchor(
		rangeStoreActor, fixture.anchorID, rangeStoreTuple(t, fixture.rootID, 0, 20),
		shrunk.Record.Anchor.Revision, "later-expansion")

	if err != nil || expanded.Status != corecontract.MutationCommitted {
		t.Fatalf("later valid expansion: %#v err=%v", expanded, err)
	}
	if err := commit.ValidateCompleteStateForTest(spatial.owner); err != nil {
		t.Fatalf("fixture final snapshot is invalid: %v", err)
	}
	closeRangeStore(t, spatial)

	if err := commit.CorruptAnchorShrinkHistoryForTest(path); err != nil {
		t.Fatal(err)
	}
	opened, err := commit.OpenDurable(path, nil)
	if opened != nil {
		if closeErr := opened.CloseStore(); closeErr != nil {
			t.Fatal(closeErr)
		}
		t.Fatal("Store accepted an invalid intermediate shrink hidden by later expansion")
	}
	if !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("corrupt shrink history: got err=%v, want ErrStore", err)
	}
}

func readAnchorRangeStoreElement(t *testing.T, spatial *rangeStoreOwner, id values.ElementID) corecontract.ElementRead {
	t.Helper()
	read, err := spatial.elements.ReadElement(rangeStoreActor, id, nil)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil {
		t.Fatalf("read %q: %#v err=%v", id, read, err)
	}
	return read
}
