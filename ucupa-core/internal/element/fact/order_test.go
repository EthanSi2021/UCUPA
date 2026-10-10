package fact

import (
	"bytes"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestOriginalUUIDBytesAndCanonicalOrder(t *testing.T) {
	low := values.ElementID("00000000-0000-4000-8000-00000000000f")
	high := values.ElementID("00000000-0000-4000-8000-000000000100")
	want := []byte{0, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, 0x0f}
	got := UUIDBytes(low)
	if !bytes.Equal(got, want) {
		t.Fatalf("UUID bytes changed: %x", got)
	}
	got[15] = 0
	if !bytes.Equal(UUIDBytes(low), want) {
		t.Fatal("UUID byte results alias")
	}
	ids := []values.ElementID{high, low, low}
	SortIDs(ids)
	if !reflect.DeepEqual(ids, []values.ElementID{low, low, high}) {
		t.Fatalf("ordering changed: %v", ids)
	}
	var empty []values.ElementID
	SortIDs(empty)
	if empty != nil {
		t.Fatal("nil ordering changed shape")
	}
}
