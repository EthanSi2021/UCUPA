// Package token keeps opaque values private without introducing a second representation.
package token

import "ucupa/shared/values"

type MountKey struct {
	ParentAuthority  values.FactAuthorityRef
	MountAnchorUUID  values.ElementID
	ProposalRevision MountRevision
	CarrierAuthority values.FactAuthorityRef
}

func (key MountKey) Valid() bool {
	return key.ParentAuthority.Valid() && key.CarrierAuthority.Valid() &&
		key.ParentAuthority != key.CarrierAuthority && key.MountAnchorUUID.Valid() &&
		ValidMountRevision(key.ProposalRevision) && MountRevisionElement(key.ProposalRevision) == key.MountAnchorUUID
}

// NextHopRevision belongs to one direction of one exact relationship.
type NextHopRevision struct {
	scope    [32]byte
	key      MountKey
	incoming bool
	serial   uint64
}

func (revision NextHopRevision) Equal(other NextHopRevision) bool { return revision == other }

// Reconstruction is internal and preserves raw parts, including invalid values.
// Callers keep the original shape, lineage and authorization checks.
func NextHopRevisionFromParts(scope [32]byte, key MountKey, incoming bool, serial uint64) NextHopRevision {
	return NextHopRevision{scope: scope, key: key, incoming: incoming, serial: serial}
}
func ValidNextHopRevision(v NextHopRevision) bool {
	return v.scope != ([32]byte{}) && v.key.Valid() && v.serial != 0
}
func NextHopScope(v NextHopRevision) [32]byte    { return v.scope }
func NextHopScopeBytes(v NextHopRevision) []byte { return v.scope[:] }
func NextHopKey(v NextHopRevision) MountKey      { return v.key }
func NextHopIncoming(v NextHopRevision) bool     { return v.incoming }
func NextHopSerial(v NextHopRevision) uint64     { return v.serial }
