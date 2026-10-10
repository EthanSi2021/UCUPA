package request

import (
	"reflect"
	"testing"
	"ucupa/shared/values"
)

const (
	testOwner   values.FactAuthorityRef = "11111111-1111-4111-8111-111111111111"
	testCore    values.FactAuthorityRef = "22222222-2222-4222-8222-222222222222"
	testElement values.ElementID        = "33333333-3333-4333-8333-333333333333"
)

func testToken(owner values.FactAuthorityRef, kind string, byteValue byte) values.PublicToken {
	var raw [32]byte
	raw[0] = byteValue
	token, err := values.PublicTokenFromBytes(owner, kind, raw)
	if err != nil {
		panic(err)
	}
	return token
}

func testAccess(proposal values.PublicToken) map[string]any {
	return map[string]any{
		"key": map[string]any{
			"core":     string(testCore),
			"facet":    string(testOwner),
			"element":  string(testElement),
			"proposal": TokenJSON(proposal),
		},
		"nextHop": "local:core",
	}
}

func wantPublicInvalid(t *testing.T, err error) {
	t.Helper()
	if err != values.PublicInvalid {
		t.Fatalf("error = %v, want %v", err, values.PublicInvalid)
	}
}

func TestObjectAndTokenEncoding(t *testing.T) {
	if object, err := Object(map[string]any{"op": "put"}, "op", "typeRef"); err != nil || object["op"] != "put" {
		t.Fatalf("Object(valid) = %#v, %v", object, err)
	}
	for name, raw := range map[string]any{
		"not object": []any{"op"},
		"unknown":    map[string]any{"op": "put", "extra": true},
		"nil field":  map[string]any{"op": nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Object(raw, "op")
			wantPublicInvalid(t, err)
		})
	}

	good := testToken(testCore, "attachment-revision", 1)
	if got, err := Token(TokenJSON(good), "attachment-revision"); err != nil || got != good {
		t.Fatalf("Token(valid) = %#v, %v", got, err)
	}
	for name, raw := range map[string]any{
		"missing owner": map[string]any{"kind": good.Kind, "value": good.Value},
		"wrong kind":    TokenJSON(good),
		"wrong value":   map[string]any{"owner": string(testCore), "kind": good.Kind, "value": "!"},
	} {
		t.Run(name, func(t *testing.T) {
			kind := "attachment-revision"
			if name == "wrong kind" {
				kind = "record-revision"
			}
			_, err := Token(raw, kind)
			wantPublicInvalid(t, err)
		})
	}
}

func TestTypeCanonicalEncoding(t *testing.T) {
	raw := make([]byte, 32)
	raw[0], raw[31] = 1, 2
	encoded := values.EncodePublicBytes(raw)
	got, err := Type(encoded)
	if err != nil || !got.Valid() || string(got.Bytes()) != string(raw) {
		t.Fatalf("Type(valid) = %#v, %v", got, err)
	}
	for _, encoded := range []any{"", values.EncodePublicBytes(raw[:31]), encoded + "="} {
		_, err := Type(encoded)
		wantPublicInvalid(t, err)
	}
	if _, err := Type(raw); err != values.PublicInvalid {
		wantPublicInvalid(t, err)
	}
}

func TestParseAccessPreservesValidationBoundary(t *testing.T) {
	proposal := testToken(testCore, "attachment-revision", 3)
	raw := testAccess(proposal)
	before := testAccess(proposal)
	access, err := ParseAccess(testOwner, raw)
	if err != nil || access.Core != testCore || access.Element != testElement || access.Proposal != proposal {
		t.Fatalf("ParseAccess(valid) = %#v, %v", access, err)
	}
	if !reflect.DeepEqual(raw, before) {
		t.Fatal("ParseAccess mutated its input")
	}
	if repeated, err := ParseAccess(testOwner, raw); err != nil || repeated != access {
		t.Fatalf("ParseAccess(repeated) = %#v, %v; want %#v, nil", repeated, err, access)
	}

	cases := []struct {
		name string
		raw  any
		err  error
	}{
		{name: "missing next hop", raw: map[string]any{"key": testAccess(proposal)["key"]}, err: values.PublicInvalid},
		{name: "invalid hop", raw: map[string]any{"key": testAccess(proposal)["key"], "nextHop": ""}, err: values.PublicInvalid},
		{name: "invalid hop before wrong owner", raw: map[string]any{
			"key":     map[string]any{"core": string(testCore), "facet": string(testCore), "element": string(testElement), "proposal": TokenJSON(proposal)},
			"nextHop": "",
		}, err: values.PublicInvalid},
		{name: "missing key field", raw: map[string]any{"key": map[string]any{"core": string(testCore)}, "nextHop": "local:core"}, err: values.PublicInvalid},
		{name: "extra access field", raw: map[string]any{"key": testAccess(proposal)["key"], "nextHop": "local:core", "extra": true}, err: values.PublicInvalid},
		{name: "extra key field", raw: map[string]any{
			"key":     map[string]any{"core": string(testCore), "facet": string(testOwner), "element": string(testElement), "proposal": TokenJSON(proposal), "extra": true},
			"nextHop": "local:core",
		}, err: values.PublicInvalid},
		{name: "proposal owner differs from core", raw: testAccess(testToken(testOwner, "attachment-revision", 4)), err: values.PublicInvalid},
		{name: "proposal kind differs", raw: testAccess(testToken(testCore, "record-revision", 5)), err: values.PublicInvalid},
		{name: "facet owner differs", raw: map[string]any{
			"key":     map[string]any{"core": string(testCore), "facet": string(testCore), "element": string(testElement), "proposal": TokenJSON(proposal)},
			"nextHop": "local:core",
		}, err: values.PublicUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := ParseAccess(testOwner, tc.raw)
			if gotErr != tc.err {
				t.Fatalf("error = %v, want %v", gotErr, tc.err)
			}
		})
	}
	wrongOwner := testAccess(proposal)
	wrongOwner["key"].(map[string]any)["facet"] = string(testOwner)
	if _, err := ParseAccess(values.FactAuthorityRef(""), wrongOwner); err != values.PublicUnavailable {
		t.Fatalf("zero owner error = %v, want %v", err, values.PublicUnavailable)
	}
}

func TestCommandConditionForms(t *testing.T) {
	revision := testToken(testOwner, "record-revision", 6)
	cases := []struct {
		name   string
		input  map[string]any
		kind   string
		absent bool
		want   values.PublicToken
		err    error
	}{
		{name: "put absent", input: map[string]any{"expected": map[string]any{"absent": true}}, kind: "put", absent: true},
		{name: "put revision", input: map[string]any{"expected": map[string]any{"revision": TokenJSON(revision)}}, kind: "put", want: revision},
		{name: "delete revision", input: map[string]any{"expected": TokenJSON(revision)}, kind: "delete", want: revision},
		{name: "put absent false", input: map[string]any{"expected": map[string]any{"absent": false}}, kind: "put", err: values.PublicInvalid},
		{name: "put both forms", input: map[string]any{"expected": map[string]any{"absent": true, "revision": TokenJSON(revision)}}, kind: "put", err: values.PublicInvalid},
		{name: "missing expected", input: map[string]any{}, kind: "put", err: values.PublicInvalid},
		{name: "delete absent form", input: map[string]any{"expected": map[string]any{"absent": true}}, kind: "delete", err: values.PublicInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			absent, got, err := CommandCondition(tc.input, tc.kind)
			if tc.err != nil {
				if err != tc.err {
					t.Fatalf("error = %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil || absent != tc.absent || got != tc.want {
				t.Fatalf("condition = (%v, %#v, %v), want (%v, %#v, nil)", absent, got, err, tc.absent, tc.want)
			}
		})
	}
}
