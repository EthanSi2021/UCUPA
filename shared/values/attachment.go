package values

import (
	"encoding/binary"
	"errors"
)

// AttachmentRevision is an opaque, copyable equality token issued by Core.
// Its byte representation is local boundary plumbing, not a public wire format.
type AttachmentRevision struct{ raw [76]byte }

func NewAttachmentRevision(encoded []byte) (AttachmentRevision, error) {
	var value AttachmentRevision
	if len(encoded) != len(value.raw) {
		return value, errors.New("invalid attachment revision size")
	}
	copy(value.raw[:], encoded)
	if !value.Valid() {
		return AttachmentRevision{}, errors.New("invalid attachment revision")
	}
	return value, nil
}

func (value AttachmentRevision) Valid() bool {
	var scope [32]byte
	copy(scope[:], value.raw[:32])
	return scope != ([32]byte{}) && ElementID(string(value.raw[32:68])).Valid() && binary.BigEndian.Uint64(value.raw[68:]) != 0
}

func (value AttachmentRevision) Bytes() []byte                       { return append([]byte(nil), value.raw[:]...) }
func (value AttachmentRevision) Equal(other AttachmentRevision) bool { return value == other }

// FacetReadCut identifies a Facet-local committed cut, never a global timestamp.
type FacetReadCut struct{ raw [40]byte }

func NewFacetReadCut(encoded []byte) (FacetReadCut, error) {
	var value FacetReadCut
	if len(encoded) != len(value.raw) {
		return value, errors.New("invalid Facet cut size")
	}
	copy(value.raw[:], encoded)
	if !value.Valid() {
		return FacetReadCut{}, errors.New("invalid Facet cut")
	}
	return value, nil
}

func (value FacetReadCut) Valid() bool {
	var scope [32]byte
	copy(scope[:], value.raw[:32])
	return scope != ([32]byte{}) && binary.BigEndian.Uint64(value.raw[32:]) != 0
}
func (value FacetReadCut) Bytes() []byte                 { return append([]byte(nil), value.raw[:]...) }
func (value FacetReadCut) Equal(other FacetReadCut) bool { return value == other }

// FacetAcceptanceKey selects one relationship; it grants no permission and
// contains no service address or payload key.
type FacetAcceptanceKey struct {
	Core     FactAuthorityRef
	Facet    FactAuthorityRef
	Element  ElementID
	Proposal AttachmentRevision
}

func (key FacetAcceptanceKey) Valid() bool {
	return key.Core.Valid() && key.Facet.Valid() && key.Element.Valid() && key.Proposal.Valid() &&
		string(key.Proposal.raw[32:68]) == string(key.Element)
}
