package fact

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestViewExposesOnlyCurrentFacetReads(t *testing.T) {
	viewType := reflect.TypeOf(View{})
	if viewType.NumField() != 1 {
		t.Fatalf("View fields = %d, want records only", viewType.NumField())
	}
	field := viewType.Field(0)
	if field.Name != "records" || field.PkgPath == "" {
		t.Fatalf("View field = %#v, want private records", field)
	}
	for _, typ := range []reflect.Type{viewType, reflect.PointerTo(viewType)} {
		if typ.NumMethod() != 2 {
			t.Fatalf("%s has %d methods, want Record and Records only", typ, typ.NumMethod())
		}
		for _, forbidden := range []string{
			"ElementCreated", "ElementDeleted", "Plan", "Apply", "Clone", "Equal",
		} {
			if _, ok := typ.MethodByName(forbidden); ok {
				t.Fatalf("%s exposes State method %s", typ, forbidden)
			}
		}
	}

	const id values.ElementID = "view-facet"
	state := New()
	state.View.records[id] = Record{NextHop: "published"}
	view := state.View
	if got, ok := view.Record(id); !ok || got.NextHop != "published" {
		t.Fatalf("View lookup mismatch: %#v %v", got, ok)
	}
	if got, ok := state.Record(id); !ok || got.NextHop != "published" {
		t.Fatalf("State did not retain promoted read methods: %#v %v", got, ok)
	}

	seen := false
	for gotID, got := range view.Records() {
		if gotID != id || got.NextHop != "published" {
			t.Fatalf("unexpected View iteration: %q %#v", gotID, got)
		}
		seen = true
		break
	}
	if !seen {
		t.Fatal("View iterator yielded no current relationship")
	}

	state.View.records[id] = Record{NextHop: "updated"}
	if got, _ := view.Record(id); got.NextHop != "updated" {
		t.Fatal("View did not see the original published map")
	}
	clone := state.Clone()
	clone.ElementDeleted(id)
	if _, ok := view.Record(id); !ok {
		t.Fatal("State.Clone shared writable relationship storage")
	}
	record, _ := view.Record(id)
	record.NextHop = "local-only"
	if got, _ := view.Record(id); got.NextHop != "updated" {
		t.Fatal("View.Record exposed mutable record storage")
	}
}
