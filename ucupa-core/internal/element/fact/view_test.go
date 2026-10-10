package fact

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestViewOwnsOnlyLiveTables(t *testing.T) {
	viewType := reflect.TypeOf(View{})
	if viewType.NumField() != 2 {
		t.Fatalf("View fields = %d, want exactly two live tables", viewType.NumField())
	}
	for index, want := range []string{"anchors", "entities"} {
		field := viewType.Field(index)
		if field.Name != want || field.PkgPath == "" {
			t.Fatalf("View field %d = %#v, want private %s", index, field, want)
		}
	}
	for _, typ := range []reflect.Type{viewType, reflect.PointerTo(viewType)} {
		for _, forbidden := range []string{
			"Plan", "Apply", "RestoreAnchor", "RestoreEntity", "RestoreTombstone",
			"Clone", "Initialized", "Claims", "Claimed", "Tombstone", "Tombstones", "Equal",
			"Departure", "Depart", "RestoreDeparture",
		} {
			if _, ok := typ.MethodByName(forbidden); ok {
				t.Fatalf("%s exposes State/history method %s", typ, forbidden)
			}
		}
	}

	state := New()
	if err := state.RestoreAnchor(Anchor{ID: testRootID}); err != nil {
		t.Fatal(err)
	}
	if err := state.RestoreTombstone(testExternalID, contract.Revision{}); err != nil {
		t.Fatal(err)
	}
	if got, want := state.View.Counts(), (LiveCounts{Anchors: 1}); got != want {
		t.Fatalf("View.Counts = %#v, want %#v", got, want)
	}
	if got, want := state.Counts(), (Counts{Claims: 2, Anchors: 1, Tombstones: 1}); got != want {
		t.Fatalf("State.Counts = %#v, want %#v", got, want)
	}

	historyFree := state.WithoutHistory()
	if got, want := historyFree.View.Counts(), state.View.Counts(); got != want {
		t.Fatalf("history-free live counts = %#v, want %#v", got, want)
	}
	if got, want := historyFree.Counts(), (Counts{Claims: 1, Anchors: 1}); got != want {
		t.Fatalf("history-free State.Counts = %#v, want %#v", got, want)
	}
	if _, ok := historyFree.Tombstone(testExternalID); ok {
		t.Fatal("history-free State retained tombstone")
	}

	extra := values.ElementID("view-extra-claim")
	state.claimed[extra] = struct{}{}
	if historyFree.Claimed(extra) {
		t.Fatal("history-free claim table aliases State claims")
	}
	state.View.anchors[testRootID] = Anchor{ID: testRootID, Parent: "shared-live-map"}
	if got, _ := historyFree.View.Anchor(testRootID); got.Parent != "shared-live-map" {
		t.Fatal("history-free View did not retain shared live map semantics")
	}
}
