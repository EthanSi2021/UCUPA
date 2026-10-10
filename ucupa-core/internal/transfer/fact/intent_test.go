package fact

import (
	"math"
	"strings"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestIntentExactShape(t *testing.T) {
	i := Intent{Profile: SliceTransfer, Operation: "transfer", Actor: values.PrincipalRef{Realm: "r", Principal: "p"},
		Source: sourceOwner, Target: targetOwner, Boundary: id(2), Expected: revision(2),
		Final: tuple(t, id(8), 0, 20), SourceAddress: "local:source", TargetAddress: "https://target:443"}
	if !i.Valid() {
		t.Fatal("valid intent rejected")
	}
	for _, change := range []func(*Intent){
		func(v *Intent) { v.Operation = "" },
		func(v *Intent) { v.Operation = contract.OperationID(strings.Repeat("x", 129)) },
		func(v *Intent) { v.Actor = values.PrincipalRef{} },
		func(v *Intent) { v.Source = "not-an-owner" },
		func(v *Intent) { v.Target = v.Source },
		func(v *Intent) { v.Boundary = id(3) },
		func(v *Intent) { v.Expected = contract.Revision{} },
		func(v *Intent) { v.Final.Parent = "" },
		func(v *Intent) { v.Final.LocalPose.Translation.X = math.NaN() },
		func(v *Intent) { v.SourceAddress = "" },
		func(v *Intent) { v.TargetAddress = "https://target:443/" },
	} {
		changed := i
		change(&changed)
		if changed.Valid() {
			t.Fatalf("invalid intent accepted: %+v", changed)
		}
	}
	i.Operation = contract.OperationID(strings.Repeat("x", 128))
	if !i.Valid() {
		t.Fatal("exact 128-byte boundary rejected")
	}
	i.Operation = contract.OperationID("\xff\x00")
	i.Actor = values.PrincipalRef{Realm: "\xfe", Principal: "\xff"}
	if !i.Valid() {
		t.Fatal("valid opaque identity bytes were treated as text")
	}
}
