package contract

import (
	"reflect"
	"testing"
)

func TestReadIdentifiersKeepOpaqueComparableSurface(t *testing.T) {
	for _, test := range []struct {
		value   any
		fields  int
		methods []string
	}{
		{CoreReadSnapshot{}, 6, nil},
		{ConnectedCut{}, 4, []string{"Equal"}},
	} {
		typ := reflect.TypeOf(test.value)
		pointer := reflect.PointerTo(typ)
		if !typ.Comparable() || typ.NumField() != test.fields || typ.NumMethod() != len(test.methods) || pointer.NumMethod() != len(test.methods) {
			t.Fatalf("%s changed representation or method set", typ)
		}
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).IsExported() {
				t.Fatalf("%s exposes a representation field", typ)
			}
		}
		for i, name := range test.methods {
			if typ.Method(i).Name != name || pointer.Method(i).Name != name {
				t.Fatalf("%s method changed", typ)
			}
		}
	}
}
