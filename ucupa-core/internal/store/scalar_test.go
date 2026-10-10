package store_test

import (
	"bytes"
	"math"
	"testing"

	"ucupa/shared/values"
	store "ucupa/ucupa-core/internal/store"
)

func TestFloatBitsRoundTripPreservesBinary64Representation(t *testing.T) {
	for _, value := range []float64{
		0,
		math.Copysign(0, -1),
		1.5,
		-2.25,
		math.Inf(1),
		math.Inf(-1),
		math.NaN(),
	} {
		encoded := store.EncodeFloatBits(value)
		if len(encoded) != 16 {
			t.Fatalf("encoded %v has length %d", value, len(encoded))
		}
		decoded, err := store.DecodeFloatBits(encoded)
		if err != nil {
			t.Fatalf("DecodeFloatBits(%q): %v", encoded, err)
		}
		if math.Float64bits(decoded) != math.Float64bits(value) {
			t.Errorf("bits changed for %v: got %016x want %016x", value, math.Float64bits(decoded), math.Float64bits(value))
		}
	}
	if got := store.EncodeFloatBits(math.Copysign(0, -1)); got != "8000000000000000" {
		t.Fatalf("negative zero bits = %q", got)
	}
	if got := store.EncodeFloatBits(1.5); got != "3ff8000000000000" {
		t.Fatalf("1.5 bits = %q", got)
	}
}

func TestDecodeFloatBitsRejectsNoncanonicalStrings(t *testing.T) {
	for _, encoded := range []string{
		"",
		"0",
		"000000000000000",
		"00000000000000000",
		"3FF8000000000000",
		"3ff800000000000g",
	} {
		if _, err := store.DecodeFloatBits(encoded); err == nil {
			t.Errorf("DecodeFloatBits(%q) accepted invalid encoding", encoded)
		}
	}
}

func TestElementKeyRoundTripAndTag(t *testing.T) {
	id := values.ElementID("00000000-0000-4000-8000-000000000001")
	key, err := store.ElementKey(id)
	if err != nil {
		t.Fatalf("ElementKey: %v", err)
	}
	if len(key) != 37 || key[0] != 1 {
		t.Fatalf("Element key = %v", key)
	}
	decoded, err := store.DecodeElementKey(key)
	if err != nil {
		t.Fatalf("DecodeElementKey: %v", err)
	}
	if decoded != id {
		t.Fatalf("decoded Element ID = %q, want %q", decoded, id)
	}

	for _, invalid := range []values.ElementID{"", "not-an-element-id", "00000000-0000-4000-7000-000000000001"} {
		if _, err := store.ElementKey(invalid); err == nil {
			t.Errorf("ElementKey accepted invalid ID %q", invalid)
		}
	}

	wrongTag := append([]byte(nil), key...)
	wrongTag[0] = 0
	wrongID := append([]byte(nil), key...)
	wrongID[1+14] = '5'
	for _, invalid := range [][]byte{nil, key[:36], append(append([]byte(nil), key...), 0), wrongTag, wrongID} {
		if _, err := store.DecodeElementKey(invalid); err == nil {
			t.Errorf("DecodeElementKey accepted invalid key %v", invalid)
		}
	}
}

func TestUint64RoundTripAndLengthValidation(t *testing.T) {
	for _, value := range []uint64{0, 1, ^uint64(0)} {
		encoded := store.EncodeUint64(value)
		if len(encoded) != 8 {
			t.Fatalf("encoded %d has length %d", value, len(encoded))
		}
		decoded, err := store.DecodeUint64(encoded)
		if err != nil {
			t.Fatalf("DecodeUint64(%d): %v", value, err)
		}
		if decoded != value {
			t.Errorf("decoded uint64 = %d, want %d", decoded, value)
		}
	}
	for _, encoded := range [][]byte{nil, make([]byte, 7), make([]byte, 9)} {
		if _, err := store.DecodeUint64(encoded); err == nil {
			t.Errorf("DecodeUint64 accepted %d-byte input", len(encoded))
		}
	}

	first := store.EncodeUint64(1)
	second := store.EncodeUint64(1)
	if !bytes.Equal(first, second) {
		t.Fatalf("same uint64 encoded differently: %v vs %v", first, second)
	}
}
