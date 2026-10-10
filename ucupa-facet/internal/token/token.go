// Package token owns Facet's opaque owner-local revision values.
//
// The package is intentionally internal: callers can retain and compare values
// returned by the Facet contract, but cannot construct them from arbitrary
// bytes or inspect their scope, element, type, key, or serial directly.
package token

import "ucupa/shared/values"

// Revision is an opaque owner-local content revision. Equal is the only
// method exposed on the value; validation, construction, and decomposition
// stay package functions for the owning Facet implementation.
type Revision struct {
	scope   [32]byte
	element values.ElementID
	typ     string
	serial  uint64
}

func (v Revision) Equal(other Revision) bool { return v == other }

// NewRevision is the sole internal construction path for a content revision.
func NewRevision(scope [32]byte, element values.ElementID, typ string, serial uint64) Revision {
	return Revision{scope: scope, element: element, typ: typ, serial: serial}
}

// RevisionParts returns the owner-only representation needed to persist or
// derive a revision. The array and string values are returned by value.
func RevisionParts(v Revision) (scope [32]byte, element values.ElementID, typ string, serial uint64) {
	return v.scope, v.element, v.typ, v.serial
}

func ValidRevision(v Revision) bool {
	return v.scope != [32]byte{} && v.element.Valid() && len(v.typ) == 32 && v.serial > 0
}

func IsZeroRevision(v Revision) bool { return v == (Revision{}) }

// ContactRevision is an opaque owner-local accepted-contact revision. Equal
// is the only method exposed on the value.
type ContactRevision struct {
	scope  [32]byte
	key    values.FacetAcceptanceKey
	serial uint64
}

func (v ContactRevision) Equal(other ContactRevision) bool { return v == other }

// NewContactRevision is the sole internal construction path for a contact
// revision.
func NewContactRevision(scope [32]byte, key values.FacetAcceptanceKey, serial uint64) ContactRevision {
	return ContactRevision{scope: scope, key: key, serial: serial}
}

func ContactRevisionParts(v ContactRevision) (scope [32]byte, key values.FacetAcceptanceKey, serial uint64) {
	return v.scope, v.key, v.serial
}

func ValidContactRevision(v ContactRevision) bool {
	return v.scope != [32]byte{} && v.key.Valid() && v.serial > 0
}

// PutPrecondition remains opaque so callers can only choose the two supported
// public forms exposed by ucupa-facet/contract.
type PutPrecondition struct {
	absent   bool
	revision Revision
}

func NewAbsentPrecondition() PutPrecondition {
	return PutPrecondition{absent: true}
}

func NewRevisionPrecondition(revision Revision) PutPrecondition {
	return PutPrecondition{revision: revision}
}

func PutPreconditionParts(p PutPrecondition) (absent bool, revision Revision) {
	return p.absent, p.revision
}

func ValidPutPrecondition(p PutPrecondition) bool {
	return (p.absent && IsZeroRevision(p.revision)) || (!p.absent && ValidRevision(p.revision))
}
