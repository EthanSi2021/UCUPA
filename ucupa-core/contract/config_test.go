package contract_test

import (
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

type publicContractField struct {
	name string
	typ  reflect.Type
}

func assertExactPublicContractStruct(t *testing.T, value any, want []publicContractField) {
	t.Helper()
	typ := reflect.TypeOf(value)
	if typ.Kind() != reflect.Struct || typ.NumField() != len(want) {
		t.Fatalf("%s fields: got %d, want %d", typ, typ.NumField(), len(want))
	}
	for index, expected := range want {
		field := typ.Field(index)
		if field.Name != expected.name || field.Type != expected.typ ||
			!field.IsExported() || field.PkgPath != "" || field.Anonymous || field.Tag != "" {
			t.Fatalf("%s field %d: got %#v, want name=%s type=%v exported non-embedded untagged", typ, index, field, expected.name, expected.typ)
		}
	}
}

func TestConfigPublicShape(t *testing.T) {
	assertExactPublicContractStruct(t, corecontract.Config{}, []publicContractField{
		{"Writer", reflect.TypeOf((*values.WriterStartup)(nil))},
		{"PublicAccessPolicy", reflect.TypeOf((*values.PublicAccessPolicy)(nil))},
		{"ExpectedRealm", reflect.TypeOf(values.RealmID(""))},
		{"SnapshotRetention", reflect.TypeOf((*time.Duration)(nil))},
		{"SnapshotCapacity", reflect.TypeOf(uint32(0))},
		{"SnapshotBytes", reflect.TypeOf(uint64(0))},
		{"ConcurrentReads", reflect.TypeOf(uint32(0))},
	})
}
