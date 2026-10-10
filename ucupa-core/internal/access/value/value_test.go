package value

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

func TestEntryIntentAndReferenceOnlyParseExistingPublicValues(t *testing.T) {
	for name, want := range map[string][]values.PermissionID{
		"read":          {contract.PermissionReadElement},
		"list":          {contract.PermissionListDirect},
		"query":         {contract.PermissionQueryRegion},
		"create-anchor": {contract.PermissionCreateAnchor},
		"create-entity": {contract.PermissionCreateEntity},
		"update-anchor": {contract.PermissionUpdateElement, contract.PermissionPlaceElement},
		"update-entity": {contract.PermissionUpdateElement, contract.PermissionPlaceElement},
		"delete-anchor": {contract.PermissionDeleteAnchor},
		"delete-entity": {contract.PermissionDeleteEntity},
		"":              nil, "recover-outcome": nil, "unknown": nil,
	} {
		if got := Intent(name); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v != %v", name, got, want)
		}
	}
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	other, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	id := values.ElementID("00000000-0000-4000-8000-000000000001")
	valid := func() map[string]any {
		return map[string]any{"boundary": map[string]any{"core": string(owner), "anchor": string(id)}, "nextHop": "local:core", "trustHint": "opaque"}
	}
	if got, err := Reference(owner, valid()); err != nil || got != id {
		t.Fatal(got, err)
	}
	if got, err := Reference(other, valid()); !errors.Is(err, values.PublicUnavailable) || got != "" {
		t.Fatal(got, err)
	}
	for _, change := range []func(map[string]any){
		func(r map[string]any) { r["extra"] = true },
		func(r map[string]any) { r["nextHop"] = "" },
		func(r map[string]any) { r["trustHint"] = true },
		func(r map[string]any) { r["boundary"].(map[string]any)["extra"] = true },
		func(r map[string]any) { r["boundary"].(map[string]any)["anchor"] = "invalid" },
	} {
		r := valid()
		change(r)
		// Invalid shape precedes wrong target; parsing does not turn the hint
		// or an address into authorization or a successful owner lookup.
		if got, err := Reference(other, r); !errors.Is(err, values.PublicInvalid) || got != "" {
			t.Fatal(got, err)
		}
	}
}

func TestReadConditionsBindAllFieldsExceptPageControls(t *testing.T) {
	base := map[string]any{"op": "query", "upDepth": "-1", "selector": "entities", "start": map[string]any{"origin": "a"}}
	want := ReadConditions(base)
	if want != `{"op":"query","selector":"entities","start":{"origin":"a"},"upDepth":"-1"}` {
		t.Fatal(want)
	}
	base["limit"] = float64(7)
	base["continuation"] = map[string]any{"value": "opaque"}
	if ReadConditions(base) != want {
		t.Fatal("page controls changed fixed conditions")
	}
	for _, key := range []string{"op", "upDepth", "selector", "start", "handle", "prepareObserve"} {
		old, existed := base[key]
		base[key] = "changed"
		if ReadConditions(base) == want {
			t.Fatalf("%s was omitted from conditions", key)
		}
		if existed {
			base[key] = old
		} else {
			delete(base, key)
		}
	}
	if base["limit"] != float64(7) || base["continuation"] == nil {
		t.Fatal("encoding mutated request")
	}
}

func TestObjectRetainsStrictShapeWithoutInventingRequiredFields(t *testing.T) {
	for _, raw := range []any{nil, []any{}, map[string]any{"extra": true}, map[string]any{"allowed": nil}} {
		if _, err := Object(raw, "allowed"); err != values.PublicInvalid {
			t.Fatalf("accepted malformed object %#v: %v", raw, err)
		}
	}
	var typedNil map[string]any
	if got, err := Object(typedNil, "allowed"); got != nil || err != nil {
		t.Fatal("generic shape check unexpectedly imposes required fields", got, err)
	}
	input := map[string]any{"allowed": float64(3)}
	got, err := Object(input, "allowed")
	if err != nil || got["allowed"] != float64(3) {
		t.Fatal(got, err)
	}
	got["allowed"] = float64(4)
	if input["allowed"] != float64(4) {
		t.Fatal("shape check unexpectedly copied input")
	}
}

func TestLimitPreservesOptionalNumericPageSize(t *testing.T) {
	for _, tc := range []struct {
		raw  any
		want int
	}{{nil, 64}, {float64(1), 1}, {float64(256), 256}} {
		if got, err := Limit(tc.raw, 64); err != nil || got != tc.want {
			t.Fatalf("limit %v = %d, %v", tc.raw, got, err)
		}
	}
	for _, raw := range []any{0.0, -1.0, 257.0, 1.5, 1, "1", math.Inf(1), math.Inf(-1), math.NaN()} {
		if _, err := Limit(raw, 64); err != values.PublicInvalid {
			t.Fatalf("limit %v = %v", raw, err)
		}
	}
}

func TestTokenShapeIsNotOwnerAuthentication(t *testing.T) {
	token, err := values.PublicTokenFromBytes("00000000-0000-4000-8000-000000000001", "record-revision", [32]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Token(TokenJSON(token), "record-revision")
	if err != nil || got != token {
		t.Fatal(got, err)
	}
	for _, edit := range []func(map[string]any){
		func(m map[string]any) { m["owner"] = "" },
		func(m map[string]any) { m["value"] = "x" },
		func(m map[string]any) { m["value"] = token.Value + "=" },
		func(m map[string]any) { m["kind"] = "space-handle" },
		func(m map[string]any) { m["owner"] = 1 },
		func(m map[string]any) { m["extra"] = false },
	} {
		raw := TokenJSON(token)
		edit(raw)
		if _, err := Token(raw, "record-revision"); err != values.PublicInvalid {
			t.Fatal(raw, err)
		}
	}
	other := TokenJSON(token)
	other["owner"] = "00000000-0000-4000-8000-000000000002"
	if got, err := Token(other, "record-revision"); err != nil || got.Owner != "00000000-0000-4000-8000-000000000002" {
		t.Fatal("shape parser incorrectly performs owner verification", got, err)
	}
}

func TestVec3DoesNotCoerceOrAcceptNonfiniteValues(t *testing.T) {
	for _, raw := range []any{nil, []float64{0, 0, 0}, []any{float64(0), float64(0)}, []any{0, float64(0), float64(0)}, []any{math.Inf(1), float64(0), float64(0)}, []any{math.NaN(), float64(0), float64(0)}} {
		if _, err := Vec3(raw); err != values.PublicInvalid {
			t.Fatal(raw, err)
		}
	}
	got, err := Vec3([]any{float64(1), float64(-2), float64(3)})
	if err != nil || got != (contract.Vec3{X: 1, Y: -2, Z: 3}) {
		t.Fatal(got, err)
	}
}

func TestPosePreservesShapeAndCanonicalErrorDistinction(t *testing.T) {
	pose := func(pivot any, ratios []any) map[string]any {
		return map[string]any{"translation": []any{float64(0), float64(0), float64(0)}, "rotation": map[string]any{"pivot": pivot, "ratios": ratios}}
	}
	zero := []any{float64(0), float64(0), float64(0)}
	if got, err := ParsePose(pose(float64(1), zero)); err != nil || got.Rotation.Pivot() != contract.RotationPivot(1) {
		t.Fatal(got, err)
	}
	for _, pivot := range []any{0.0, 5.0, 1.5, 1, nil, math.Inf(1), math.NaN()} {
		if _, err := ParsePose(pose(pivot, zero)); err != values.PublicInvalid {
			t.Fatal(pivot, err)
		}
	}
	for _, ratios := range [][]any{
		{float64(2), float64(0), float64(0)},
		{math.Copysign(0, -1), float64(0), float64(0)},
	} {
		if _, err := ParsePose(pose(float64(1), ratios)); err != contract.ErrInvalid {
			t.Fatal(ratios, err)
		}
	}
	missing := pose(float64(1), zero)
	delete(missing, "translation")
	if _, err := ParsePose(missing); err != values.PublicInvalid {
		t.Fatal(err)
	}
}

func TestPoseJSONPreservesExistingCanonicalWireShape(t *testing.T) {
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: 1, Y: -2, Z: 3}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := values.CanonicalJSONValue(PoseJSON(pose))
	if err != nil || string(raw) != `{"rotation":{"pivot":1,"ratios":[0,0,0]},"translation":[1,-2,3]}` {
		t.Fatalf("pose encoding: %s %v", raw, err)
	}
	decoded, err := ParsePose(PoseJSON(pose))
	if err != nil || decoded != pose {
		t.Fatalf("pose roundtrip: %+v %v", decoded, err)
	}
}
