package token

import (
	"encoding/binary"

	"ucupa/shared/values"
)

// CoreReadCut is the opaque equality token for one complete owner-local Core
// fact cut. Its zero value is invalid. The representation is owned here so
// Core's public contract can expose the exact value without exposing fields.
type CoreReadCut struct {
	scope  [32]byte
	serial uint64
}

// Equal compares two Core-issued (or shape-parsed foreign) cut values. It is
// deliberately the only comparison operation exposed by the value itself.
func (left CoreReadCut) Equal(right CoreReadCut) bool { return left == right }

// Evidence projects the actual cut into the existing copied local evidence
// value. It is not authentication, issuance, or a network wire format.
func (cut CoreReadCut) Evidence() values.CoreCutEvidence {
	var raw [40]byte
	copy(raw[:32], cut.scope[:])
	binary.BigEndian.PutUint64(raw[32:], cut.serial)
	value, _ := values.NewCoreCutEvidence(raw[:])
	return value
}

// Valid reports shape validity only. Owner binding and authorization remain
// responsibilities of the consuming Core operation.
func Valid(cut CoreReadCut) bool {
	return cut.scope != ([32]byte{}) && cut.serial != 0
}

// Scope returns the owner scope for internal Core validation and codecs.
func Scope(cut CoreReadCut) [32]byte { return cut.scope }

// Serial returns the monotonic cut serial for internal Core validation and
// codecs.
func Serial(cut CoreReadCut) uint64 { return cut.serial }

// FromParts rebuilds a cut at an internal owner boundary. Callers must invoke
// Valid and perform the appropriate owner/replay binding before accepting it.
func FromParts(scope [32]byte, serial uint64) CoreReadCut {
	return CoreReadCut{scope: scope, serial: serial}
}

// ValidOptionalCut checks only the original optional cut shape; it does not
// establish ownership, currentness, retained state, or authorization.
func ValidOptionalCut(at *CoreReadCut) bool { return at == nil || Valid(*at) }
