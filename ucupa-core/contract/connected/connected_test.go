package connected_test

import (
	"reflect"
	"testing"

	connected "ucupa/ucupa-core/contract/connected"
)

func TestOpaqueExchangeHasNoPublicSurface(t *testing.T) {
	for _, value := range []any{connected.Request{}, connected.Reply{}} {
		typeOf := reflect.TypeOf(value)
		pointerMethods := reflect.PointerTo(typeOf).NumMethod()
		if typeOf.NumField() != 1 || typeOf.Field(0).IsExported() || typeOf.NumMethod() != 0 || pointerMethods != 0 {
			t.Fatalf("opaque surface changed: %v fields=%d value-methods=%d pointer-methods=%d", typeOf, typeOf.NumField(), typeOf.NumMethod(), pointerMethods)
		}
	}
}
