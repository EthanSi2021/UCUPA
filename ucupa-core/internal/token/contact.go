package token

import "ucupa/shared/values"

// FacetContactRevision is a Core-issued contact token. Its type and exact key
// keep it separate from mount contacts and Element/attachment revisions.
type FacetContactRevision struct {
	scope  [32]byte
	key    values.FacetAcceptanceKey
	serial uint64
}

func (v FacetContactRevision) Equal(other FacetContactRevision) bool { return v == other }
func ValidFacetContactRevision(v FacetContactRevision) bool {
	return v.scope != ([32]byte{}) && v.key.Valid() && v.serial != 0
}
func FacetContactRevisionFromParts(scope [32]byte, key values.FacetAcceptanceKey, serial uint64) FacetContactRevision {
	return FacetContactRevision{scope: scope, key: key, serial: serial}
}
func FacetContactScope(v FacetContactRevision) [32]byte                { return v.scope }
func FacetContactScopeBytes(v FacetContactRevision) []byte             { return v.scope[:] }
func FacetContactKey(v FacetContactRevision) values.FacetAcceptanceKey { return v.key }
func FacetContactSerial(v FacetContactRevision) uint64                 { return v.serial }
