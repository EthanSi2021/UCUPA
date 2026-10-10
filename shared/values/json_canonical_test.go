package values

import (
	"math"
	"testing"
)

func TestJCSIndependentRFC8785NumberVectors(t *testing.T) {
	// RFC 8785 Appendix B binary64 vectors, independent of this encoder.
	cases := []struct {
		bits uint64
		want string
	}{
		{0, "0"}, {0x8000000000000000, "0"}, {1, "5e-324"}, {0x8000000000000001, "-5e-324"},
		{0x7fefffffffffffff, "1.7976931348623157e+308"}, {0xffefffffffffffff, "-1.7976931348623157e+308"},
		{0x4340000000000000, "9007199254740992"}, {0xc340000000000000, "-9007199254740992"},
		{0x4430000000000000, "295147905179352830000"}, {0x44b52d02c7e14af5, "9.999999999999997e+22"},
		{0x44b52d02c7e14af6, "1e+23"}, {0x44b52d02c7e14af7, "1.0000000000000001e+23"},
		{0x444b1ae4d6e2ef4e, "999999999999999700000"}, {0x444b1ae4d6e2ef4f, "999999999999999900000"},
		{0x444b1ae4d6e2ef50, "1e+21"}, {0x3eb0c6f7a0b5ed8c, "9.999999999999997e-7"},
		{0x3eb0c6f7a0b5ed8d, "0.000001"}, {0x41b3de4355555553, "333333333.3333332"},
		{0x41b3de4355555554, "333333333.33333325"}, {0x41b3de4355555555, "333333333.3333333"},
		{0x41b3de4355555556, "333333333.3333334"}, {0x41b3de4355555557, "333333333.33333343"},
	}
	for _, tc := range cases {
		got, err := CanonicalJSONValue(math.Float64frombits(tc.bits))
		if err != nil || string(got) != tc.want {
			t.Errorf("%x: %s %v, want %s", tc.bits, got, err, tc.want)
		}
	}
}

func TestCanonicalJSONStrictUnicodeOrderingAndLimits(t *testing.T) {
	input := []byte(`{"😀":1,"\ufffd":2,"€":3,"\r":4,"1":5,"ö":6,"\u0080":7}`)
	value, err := ParsePublicJSON(input, PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := CanonicalJSONValue(value)
	want := "{\"\\r\":4,\"1\":5,\"\u0080\":7,\"ö\":6,\"€\":3,\"😀\":1,\"�\":2}"
	if err != nil || string(got) != want {
		t.Fatalf("UTF-16 ordering: %s %v", got, err)
	}
	if _, err := ParseCanonicalJSON(got, PublicJSONLimits()); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"a":1,"\u0061":2}`, `"\ud800"`, `"\udead"`, `"\ud800x"`, `"\ud800\u0000"`, "\xef\xbb\xbf{}", "\"\xff\"", `1e999`, `-0`, `1.0`, ` {}`, `{}[]`, `{"z":1,"a":2}`} {
		if _, err := ParseCanonicalJSON([]byte(raw), PublicJSONLimits()); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	for _, raw := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"�"`} {
		if _, err := ParsePublicJSON([]byte(raw), PublicJSONLimits()); err != nil {
			t.Errorf("valid Unicode %s: %v", raw, err)
		}
	}
	limits := JSONLimits{Bytes: 64, Depth: 2, Nodes: 3, ContainerEntries: 2, StringBytes: 3}
	for _, raw := range []string{`[[[0]]]`, `[0,1,2]`, `"1234"`, `{"1234":0}`} {
		if _, err := ParsePublicJSON([]byte(raw), limits); err == nil {
			t.Errorf("limit escaped: %s", raw)
		}
	}
}

func TestPublicBytesAndDepthExact(t *testing.T) {
	for _, text := range []string{"AB", "AA=", "AA\n", "+A", "/A", "A", "AA "} {
		if _, err := DecodePublicBytes(text); err == nil {
			t.Errorf("invalid base64url accepted: %q", text)
		}
	}
	for _, text := range []string{"+1", "01", "-0", "-2", "9223372036854775808"} {
		if _, err := ParsePublicUpDepth(text); err == nil {
			t.Errorf("invalid depth: %q", text)
		}
	}
	for _, text := range []string{"-1", "0", "9223372036854775807"} {
		if _, err := ParsePublicUpDepth(text); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := ParsePublicParentCount("18446744073709551615"); err != nil || n != math.MaxUint64 {
		t.Fatal(n, err)
	}
}
