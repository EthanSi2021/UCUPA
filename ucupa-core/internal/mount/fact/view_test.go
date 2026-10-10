package fact

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestViewExposesOnlyCurrentMountReads(t *testing.T) {
	viewType := reflect.TypeOf(View{})
	if viewType.NumField() != 2 {
		t.Fatalf("View fields = %d, want outgoing and incoming only", viewType.NumField())
	}
	for index, want := range []string{"outgoing", "incoming"} {
		field := viewType.Field(index)
		if field.Name != want || field.PkgPath == "" {
			t.Fatalf("View field %d = %#v, want private %s", index, field, want)
		}
	}
	for _, typ := range []reflect.Type{viewType, reflect.PointerTo(viewType)} {
		if typ.NumMethod() != 5 {
			t.Fatalf("%s has %d methods, want the five current-read methods", typ, typ.NumMethod())
		}
		for _, forbidden := range []string{
			"Clone", "Equal", "Initialized", "ReplaceOutgoing", "ReplaceIncoming",
		} {
			if _, ok := typ.MethodByName(forbidden); ok {
				t.Fatalf("%s exposes State method %s", typ, forbidden)
			}
		}
	}

	const id values.ElementID = "view-mount"
	state := New()
	state.ReplaceOutgoing(id, Record{Slot: "published"})
	state.ReplaceIncoming(Incoming{Gate: GateOpen})
	view := state.View

	if got, ok := view.Outgoing(id); !ok || got.Slot != "published" {
		t.Fatalf("View lost published outgoing relation: %#v %v", got, ok)
	}
	if got := view.Record(id); got.Slot != "published" || view.OutgoingCount() != 1 {
		t.Fatalf("View lookup/count mismatch: %#v %d", got, view.OutgoingCount())
	}
	if got := view.Incoming(); got.Gate != GateOpen {
		t.Fatalf("View lost incoming relation: %#v", got)
	}
	if got, ok := state.Outgoing(id); !ok || got.Slot != "published" || state.OutgoingCount() != 1 {
		t.Fatalf("State did not retain promoted read methods: %#v %v", got, ok)
	}

	seen := false
	for gotID, got := range view.Outgoings() {
		if gotID != id || got.Slot != "published" {
			t.Fatalf("unexpected View iteration: %q %#v", gotID, got)
		}
		seen = true
		break
	}
	if !seen {
		t.Fatal("View iterator yielded no current relation")
	}

	state.ReplaceOutgoing(id, Record{Slot: "updated"})
	if got := view.Record(id); got.Slot != "updated" {
		t.Fatal("View did not see the original published map")
	}
	clone := state.Clone()
	clone.ReplaceOutgoing(id, Record{Slot: "clone"})
	if got := view.Record(id); got.Slot != "updated" {
		t.Fatal("State.Clone shared writable outgoing storage")
	}
	incoming := view.Incoming()
	incoming.Gate = GateClosed
	if got := view.Incoming(); got.Gate != GateOpen {
		t.Fatal("View.Incoming exposed mutable state")
	}
}
