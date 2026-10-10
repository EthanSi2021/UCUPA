package store_test

import (
	"testing"

	store "ucupa/ucupa-core/internal/store"
)

type canonicalJSONFixture struct {
	A int    `json:"a"`
	B string `json:"b"`
}

type canonicalJSONEnvelope struct {
	Payload canonicalJSONFixture `json:"payload"`
}

func TestDecodeCanonicalJSONAcceptsCanonicalBytes(t *testing.T) {
	var got canonicalJSONFixture
	if err := store.DecodeCanonicalJSON([]byte(`{"a":7,"b":"ok"}`), &got); err != nil {
		t.Fatalf("canonical JSON rejected: %v", err)
	}
	if got != (canonicalJSONFixture{A: 7, B: "ok"}) {
		t.Fatalf("decoded value = %#v", got)
	}
}

func TestDecodeCanonicalJSONRejectsMalformedDuplicateExtraAndTrailingBytes(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		target  any
	}{
		{name: "malformed", encoded: `{"a":,"b":"ok"}`, target: &canonicalJSONFixture{}},
		{name: "duplicate", encoded: `{"a":7,"a":8,"b":"ok"}`, target: &canonicalJSONFixture{}},
		{name: "nested duplicate", encoded: `{"payload":{"a":7,"a":8,"b":"ok"}}`, target: &canonicalJSONEnvelope{}},
		{name: "extra field", encoded: `{"a":7,"b":"ok","extra":true}`, target: &canonicalJSONFixture{}},
		{name: "reordered fields", encoded: `{"b":"ok","a":7}`, target: &canonicalJSONFixture{}},
		{name: "whitespace", encoded: `{"a":7, "b":"ok"}`, target: &canonicalJSONFixture{}},
		{name: "trailing value", encoded: `{"a":7,"b":"ok"}{"a":8,"b":"late"}`, target: &canonicalJSONFixture{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.DecodeCanonicalJSON([]byte(tc.encoded), tc.target); err == nil {
				t.Fatalf("accepted noncanonical or unsafe JSON %s", tc.encoded)
			}
		})
	}
}
