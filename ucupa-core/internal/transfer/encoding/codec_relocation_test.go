package encoding

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// This independent format fingerprint was captured before codec relocation.
// It ignores Go package/type names but includes every stored field in order,
// its JSON tag, pointer/array/slice shape and primitive width. A relocation
// must not quietly change the original persistence or transfer byte contract.
func TestTransferStoredShapeRelocationBaseline(t *testing.T) {
	var schemas []any
	for _, v := range []any{StoredTransferEvent{}, StoredTransferManifest{}, StoredEdgeClosure{}, StoredEdgeResult{}, StoredTransferParentHold{}} {
		schemas = append(schemas, storedShape(reflect.TypeOf(v)))
	}
	data, err := json.Marshal(schemas)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(data))
	const want = "c635d43266f8d1920bc1ece90e31beb74a6503a4b155a861ab17f263ec7598d7"
	if got != want {
		t.Fatalf("stored format changed during relocation: %s", got)
	}
}

func storedShape(t reflect.Type) any {
	switch t.Kind() {
	case reflect.Struct:
		var fields []any
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fields = append(fields, []any{f.Name, string(f.Tag), storedShape(f.Type)})
		}
		return []any{"struct", fields}
	case reflect.Array:
		return []any{"array", t.Len(), storedShape(t.Elem())}
	case reflect.Slice, reflect.Pointer:
		return []any{t.Kind().String(), storedShape(t.Elem())}
	default:
		return t.Kind().String()
	}
}
