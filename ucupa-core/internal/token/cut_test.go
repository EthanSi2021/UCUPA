package token

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"ucupa/shared/values"
)

func TestCoreReadCutKeepsPrivateFieldsAndExactEvidence(t *testing.T) {
	typ := reflect.TypeOf(CoreReadCut{})
	if typ.NumField() != 2 || typ.Field(0).PkgPath == "" || typ.Field(1).PkgPath == "" {
		t.Fatalf("CoreReadCut fields are not private: %v", typ)
	}
	wantMethods := map[string]bool{"Equal": true, "Evidence": true}
	if typ.NumMethod() != len(wantMethods) {
		t.Fatalf("CoreReadCut exported method count = %d, want %d", typ.NumMethod(), len(wantMethods))
	}
	for index := 0; index < typ.NumMethod(); index++ {
		if !wantMethods[typ.Method(index).Name] {
			t.Fatalf("unexpected CoreReadCut exported method %q", typ.Method(index).Name)
		}
	}
	scope := [32]byte{1, 2, 3}
	cut := FromParts(scope, 9)
	if !Valid(cut) || !cut.Equal(FromParts(scope, 9)) || cut.Equal(FromParts(scope, 10)) {
		t.Fatal("CoreReadCut validity or equality changed")
	}
	if Scope(cut) != scope || Serial(cut) != 9 {
		t.Fatal("CoreReadCut parts changed")
	}
	evidence := cut.Evidence()
	want := make([]byte, 40)
	copy(want[:32], scope[:])
	binary.BigEndian.PutUint64(want[32:], 9)
	if !evidence.Valid() || !bytes.Equal(evidence.Bytes(), want) {
		t.Fatalf("evidence bytes = %x, want %x", evidence.Bytes(), want)
	}
	wantEvidence, err := values.NewCoreCutEvidence(want)
	if err != nil || !evidence.Equal(wantEvidence) {
		t.Fatalf("evidence equality changed: %v", err)
	}
}

func TestCoreReadCutZeroIsInvalid(t *testing.T) {
	if Valid(CoreReadCut{}) || Scope(CoreReadCut{}) != ([32]byte{}) || Serial(CoreReadCut{}) != 0 {
		t.Fatal("zero CoreReadCut became valid")
	}
}

func TestOptionalCutChecksOnlyShape(t *testing.T) {
	zero := CoreReadCut{}
	valid := FromParts([32]byte{3}, 1)
	if !ValidOptionalCut(nil) || ValidOptionalCut(&zero) || !ValidOptionalCut(&valid) {
		t.Fatal("optional cut changed nil/zero/valid shape semantics")
	}
}
