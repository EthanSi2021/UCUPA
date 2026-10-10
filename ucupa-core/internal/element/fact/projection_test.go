package fact

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestReadProjectionShapes(t *testing.T) {
	const (
		anchorID values.ElementID = "projection-anchor"
		entityID values.ElementID = "projection-entity"
		deadID   values.ElementID = "projection-dead"
		newID    values.ElementID = "projection-new"
	)
	state := New()
	anchor := Anchor{ID: anchorID, Parent: "projection-root"}
	entity := Entity{ID: entityID, Parent: anchorID}
	if err := state.RestoreAnchor(anchor); err != nil {
		t.Fatal(err)
	}
	if err := state.RestoreEntity(entity); err != nil {
		t.Fatal(err)
	}
	if err := state.RestoreTombstone(deadID, contract.Revision{}); err != nil {
		t.Fatal(err)
	}

	anchorsOnly := state.AnchorsOnly()
	if anchorsOnly.entities != nil || anchorsOnly.claimed != nil || anchorsOnly.tombstones != nil {
		t.Fatal("anchors-only projection allocated or retained other tables")
	}
	counts := anchorsOnly.Counts()
	if counts.Claims != 0 || counts.Anchors != 1 || counts.Entities != 0 || counts.Tombstones != 0 ||
		anchorsOnly.Initialized() {
		t.Fatalf("unexpected anchors-only shape: %#v", counts)
	}
	if _, ok := anchorsOnly.Anchor(anchorID); !ok {
		t.Fatal("anchors-only view lost anchor")
	}
	if _, ok := anchorsOnly.Entity(entityID); ok {
		t.Fatal("anchors-only view retained entity")
	}
	state.anchors[anchorID] = Anchor{ID: anchorID, Parent: "projection-updated"}
	if got, _ := anchorsOnly.Anchor(anchorID); got.Parent != "projection-updated" {
		t.Fatal("anchors-only view did not share anchor map")
	}

	withoutHistory := state.WithoutHistory()
	counts = withoutHistory.Counts()
	if counts.Claims != 2 || counts.Anchors != 1 || counts.Entities != 1 || counts.Tombstones != 0 ||
		!withoutHistory.Initialized() {
		t.Fatalf("unexpected history-free shape: %#v", counts)
	}
	if _, ok := withoutHistory.Tombstone(deadID); ok {
		t.Fatal("history-free view retained tombstone")
	}
	state.claimed[newID] = struct{}{}
	if withoutHistory.Claimed(newID) {
		t.Fatal("history-free claim table was not rebuilt")
	}
	state.entities[entityID] = Entity{ID: entityID, Parent: "projection-updated"}
	if got, _ := withoutHistory.Entity(entityID); got.Parent != "projection-updated" {
		t.Fatal("history-free view did not share entity map")
	}
}

func TestProjectionBuilderIsLiveOnly(t *testing.T) {
	const (
		anchorID values.ElementID = "builder-anchor"
		entityID values.ElementID = "builder-entity"
	)
	builder := NewProjection()
	builder.AddAnchor(anchorID, Anchor{ID: anchorID, Parent: "first"})
	builder.AddAnchor(anchorID, Anchor{ID: anchorID, Parent: "replacement"})
	builder.AddEntity(entityID, Entity{ID: entityID, Parent: anchorID})
	state := builder.Build()
	if builder.anchors != nil || builder.entities != nil || state.claimed != nil || state.tombstones != nil {
		t.Fatal("projection build did not transfer only the live maps")
	}
	counts := state.Counts()
	if counts.Claims != 0 || counts.Anchors != 1 || counts.Entities != 1 || counts.Tombstones != 0 ||
		state.Initialized() {
		t.Fatalf("unexpected builder shape: %#v", counts)
	}
	anchor, ok := state.Anchor(anchorID)
	if !ok || anchor.Parent != "replacement" {
		t.Fatalf("builder duplicate assignment changed: %#v %v", anchor, ok)
	}
	if _, ok := state.Entity(entityID); !ok {
		t.Fatal("builder lost entity")
	}
}
