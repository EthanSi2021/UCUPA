// Package contract contains the public Facet value boundary.
package contract

import "ucupa/ucupa-facet/internal/token"

// Revision and ContactRevision are opaque values issued by a Facet owner.
// Their representation and construction remain internal to the module.
type Revision = token.Revision
type ContactRevision = token.ContactRevision
type PutPrecondition = token.PutPrecondition

// ExpectAbsent requests creation only when the addressed record is absent.
func ExpectAbsent() PutPrecondition { return token.NewAbsentPrecondition() }

// ExpectRevision requests a compare-and-set against an owner-issued revision.
func ExpectRevision(revision Revision) PutPrecondition {
	return token.NewRevisionPrecondition(revision)
}
