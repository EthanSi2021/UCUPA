package token

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/failure"
)

// Revision is an opaque equality token for one Element lineage in one Core.
// Its zero value is invalid.
type Revision struct {
	scope   [32]byte
	element values.ElementID
	serial  uint64
}

type MountRevision struct {
	scope   [32]byte
	element values.ElementID
	serial  uint64
}

func (left Revision) Equal(right Revision) bool               { return left == right }
func (revision MountRevision) Equal(other MountRevision) bool { return revision == other }

func ValidRevision(revision Revision) bool {
	return revision.scope != ([32]byte{}) && revision.element.Valid() && revision.serial != 0
}
func ValidMountRevision(revision MountRevision) bool {
	return revision.scope != ([32]byte{}) && revision.element.Valid() && revision.serial != 0
}

// ServiceRevision is the explicit projection used by authenticated peer
// protocols. Parsing it proves shape only, never issuance or permission. It
// must not replace the run-local opaque tokens returned to public consumers.
type ServiceRevision struct {
	Incarnation [32]byte
	Element     values.ElementID
	Sequence    uint64
}

func (revision Revision) ServiceEvidence() ServiceRevision {
	return ServiceRevision{revision.scope, revision.element, revision.serial}
}
func (revision MountRevision) ServiceEvidence() ServiceRevision {
	return ServiceRevision{revision.scope, revision.element, revision.serial}
}
func (e ServiceRevision) ElementRevision() (Revision, error) {
	r := Revision{e.Incarnation, e.Element, e.Sequence}
	if !ValidRevision(r) {
		return Revision{}, failure.ErrInvalid
	}
	return r, nil
}
func (e ServiceRevision) MountRevision() (MountRevision, error) {
	r := MountRevision{e.Incarnation, e.Element, e.Sequence}
	if !ValidMountRevision(r) {
		return MountRevision{}, failure.ErrInvalid
	}
	return r, nil
}

// Internal reconstruction preserves raw values. Callers retain the existing
// shape, lineage, issuance and authorization checks at their original sites.
func RevisionFromParts(scope [32]byte, element values.ElementID, serial uint64) Revision {
	return Revision{scope: scope, element: element, serial: serial}
}
func MountRevisionFromParts(scope [32]byte, element values.ElementID, serial uint64) MountRevision {
	return MountRevision{scope: scope, element: element, serial: serial}
}
func RevisionScope(v Revision) [32]byte                     { return v.scope }
func RevisionElement(v Revision) values.ElementID           { return v.element }
func RevisionSerial(v Revision) uint64                      { return v.serial }
func MountRevisionScope(v MountRevision) [32]byte           { return v.scope }
func MountRevisionElement(v MountRevision) values.ElementID { return v.element }
func MountRevisionSerial(v MountRevision) uint64            { return v.serial }

// These codec inputs are copies. No caller receives a mutable token field.
func RevisionScopeBytes(v Revision) []byte           { return v.scope[:] }
func MountRevisionScopeBytes(v MountRevision) []byte { return v.scope[:] }
