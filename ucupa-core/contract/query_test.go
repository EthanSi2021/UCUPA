package contract_test

import (
	"reflect"
	"testing"

	c "ucupa/ucupa-core/contract"
)

func TestQueryPositionsStayOpaque(t *testing.T) {
	for _, tc := range []struct {
		value  any
		fields int
	}{{c.QueryContinuation{}, 11}, {c.ConnectedQueryContinuation{}, 3}, {c.ConnectedDirectContinuation{}, 3}} {
		typ := reflect.TypeOf(tc.value)
		if typ.NumField() != tc.fields || typ.NumMethod() != 0 || reflect.PointerTo(typ).NumMethod() != 0 {
			t.Fatalf("position shape/method set changed: %s", typ)
		}
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).IsExported() {
				t.Fatalf("position field exposed: %s.%s", typ, typ.Field(i).Name)
			}
		}
	}
	if c.QueryCurrentVerified != 1 || c.QueryRetained != 2 || c.QueryAnchors != 1 || c.QueryEntities != 2 || c.QueryBoth != 3 ||
		c.QueryComplete != 1 || c.QueryPartial != 2 || c.QueryNoGap != 0 || c.QueryExactWorkLimit != 1 || c.QueryUnresolvedCoverage != 2 {
		t.Fatal("query enum values changed")
	}
}
