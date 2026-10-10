package contract

import (
	"encoding/binary"

	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/token"
)

// CoreReadCut is the public contract view of Core's opaque owner-local read
// cut. The representation and safe methods remain defined by internal/token;
// this alias does not create a second type or constructor surface.
type CoreReadCut = token.CoreReadCut

// CoreReadCutFromEvidence parses service evidence without asserting that this
// Core issued it. The consuming operation must bind it to its exact relation.
func CoreReadCutFromEvidence(e values.CoreCutEvidence) (CoreReadCut, error) {
	if !e.Valid() {
		return CoreReadCut{}, ErrInvalid
	}
	b := e.Bytes()
	var scope [32]byte
	copy(scope[:], b[:32])
	return token.FromParts(scope, binary.BigEndian.Uint64(b[32:])), nil
}
