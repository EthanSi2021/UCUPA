package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"ucupa/shared/values"
)

// ElementKey keeps the tag separating Element records from Store metadata.
func ElementKey(id values.ElementID) ([]byte, error) {
	if !id.Valid() {
		return nil, errors.New("Element ID is invalid")
	}
	key := make([]byte, 1+len(id))
	key[0] = 1
	copy(key[1:], string(id))
	return key, nil
}

func DecodeElementKey(key []byte) (values.ElementID, error) {
	if len(key) != 37 || key[0] != 1 {
		return "", fmt.Errorf("Element record key %q is not recognized", key)
	}
	id := values.ElementID(string(append([]byte(nil), key[1:]...)))
	if !id.Valid() {
		return "", errors.New("Element record key has an invalid Element ID")
	}
	return id, nil
}

func EncodeUint64(value uint64) []byte {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, value)
	return encoded
}

func DecodeUint64(encoded []byte) (uint64, error) {
	if len(encoded) != 8 {
		return 0, errors.New("uint64 value has invalid length")
	}
	return binary.BigEndian.Uint64(encoded), nil
}

// EncodeFloatBits preserves the exact binary64 representation, including signed
// zero. The owner, not this byte codec, decides which values are valid facts.
func EncodeFloatBits(value float64) string { return fmt.Sprintf("%016x", math.Float64bits(value)) }

func DecodeFloatBits(encoded string) (float64, error) {
	if len(encoded) != 16 || strings.ToLower(encoded) != encoded {
		return 0, errors.New("binary64 bit string is not canonical")
	}
	bits, err := strconv.ParseUint(encoded, 16, 64)
	if err != nil || fmt.Sprintf("%016x", bits) != encoded {
		return 0, errors.New("binary64 bit string is invalid")
	}
	return math.Float64frombits(bits), nil
}
