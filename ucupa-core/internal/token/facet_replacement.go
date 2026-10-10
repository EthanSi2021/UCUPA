package token

import (
	"encoding/binary"

	"ucupa/shared/values"
)

// FacetReplacementRevision projects one retained replacement control
// comparison. It is domain-separated from element, attachment and transition
// revisions and bound to the original reservation cut, operation and element.
// It grants no access or execution right.
func (signer PublicSigner) FacetReplacementRevision(reservation CoreReadCut, operation string, element values.ElementID) values.PublicToken {
	if !Valid(reservation) || len(operation) == 0 || len(operation) > 128 || !element.Valid() {
		return values.PublicToken{}
	}
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], Serial(reservation))
	scope := Scope(reservation)
	return publicRevision(signer.key, signer.owner, "facet-replacement-revision", scope[:], serial[:], []byte(operation), []byte(element))
}
