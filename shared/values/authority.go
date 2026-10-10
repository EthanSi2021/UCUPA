package values

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// FactAuthorityRef identifies one logical fact owner independently of its Elements
// and deployment address. It is not an authorization principal.
type FactAuthorityRef string

// Valid reports whether the reference is a canonical lowercase UUIDv4 with
// the RFC variant, as required by the local Core profile.
func (value FactAuthorityRef) Valid() bool {
	return validUUIDv4(string(value))
}

// NewFactAuthorityRef generates a new fact-owner identity from secure randomness.
// Reopening an existing owner must restore its persisted identity instead.
func NewFactAuthorityRef() (FactAuthorityRef, error) {
	return newFactAuthorityRef(rand.Reader)
}

func newFactAuthorityRef(reader io.Reader) (FactAuthorityRef, error) {
	var raw [16]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return "", fmt.Errorf("generate FactAuthorityRef: %w", err)
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	text := hex.EncodeToString(raw[:])
	return FactAuthorityRef(text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]), nil
}
