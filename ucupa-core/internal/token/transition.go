package token

import (
	"encoding/binary"

	"ucupa/shared/values"
)

// TransitionRevision projects one retained source control comparison. It is
// domain-separated from Element/mount revisions and bound to the original
// Store scope, boundary and operation. It grants no access or execution right.
func (signer PublicSigner) TransitionRevision(admission CoreReadCut, operation string, boundary values.ElementID, revision uint64) values.PublicToken {
	if !Valid(admission) || len(operation) == 0 || len(operation) > 128 || !boundary.Valid() || revision == 0 {
		return values.PublicToken{}
	}
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], revision)
	scope := Scope(admission)
	return publicRevision(signer.key, signer.owner, "transition-revision", scope[:], []byte(operation), []byte(boundary), serial[:])
}

// CarrierRevision compares one exact parent control state. Unlike a source
// transition sequence, both reservation and progress are retained parent cuts.
// Keeping this domain separate prevents using a mount or source token here.
func (signer PublicSigner) CarrierRevision(reservation, progress CoreReadCut, operation string, mount values.ElementID) values.PublicToken {
	if !Valid(reservation) || !Valid(progress) || Scope(reservation) != Scope(progress) || Serial(progress) < Serial(reservation) || len(operation) == 0 || len(operation) > 128 || !mount.Valid() {
		return values.PublicToken{}
	}
	var serial [16]byte
	binary.BigEndian.PutUint64(serial[:8], Serial(reservation))
	binary.BigEndian.PutUint64(serial[8:], Serial(progress))
	scope := Scope(reservation)
	return publicRevision(signer.key, signer.owner, "carrier-revision", scope[:], []byte(operation), []byte(mount), serial[:])
}
