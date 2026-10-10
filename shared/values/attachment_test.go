package values

import (
	"encoding/binary"
	"testing"
)

func TestAttachmentTokensCopyAndRejectInvalidShapes(t *testing.T) {
	var raw [76]byte
	raw[0] = 1
	id := ElementID("00000000-0000-4000-8000-000000000001")
	copy(raw[32:68], []byte(id))
	binary.BigEndian.PutUint64(raw[68:], 2)
	value, err := NewAttachmentRevision(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 9
	copyOut := value.Bytes()
	copyOut[0] = 7
	if value.Bytes()[0] != 1 {
		t.Fatal("token aliases caller bytes")
	}
	if !value.Equal(value) || (AttachmentRevision{}).Valid() {
		t.Fatal("token validity")
	}
	for _, input := range [][]byte{nil, make([]byte, 75), make([]byte, 76), make([]byte, 77)} {
		if _, err := NewAttachmentRevision(input); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	owner, _ := NewFactAuthorityRef()
	target, _ := NewFactAuthorityRef()
	key := FacetAcceptanceKey{Core: owner, Facet: target, Element: id, Proposal: value}
	if !key.Valid() {
		t.Fatal("valid key rejected")
	}
	key.Element = "00000000-0000-4000-8000-000000000002"
	if key.Valid() {
		t.Fatal("proposal accepted for another Element")
	}
}

func TestFacetCutCopyAndValidity(t *testing.T) {
	var raw [40]byte
	raw[0] = 3
	binary.BigEndian.PutUint64(raw[32:], 1)
	value, err := NewFacetReadCut(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 9
	copyOut := value.Bytes()
	copyOut[0] = 7
	if value.Bytes()[0] != 3 || !value.Equal(value) || (FacetReadCut{}).Valid() {
		t.Fatal("Facet cut alias or validity error")
	}
	for _, input := range [][]byte{nil, make([]byte, 39), make([]byte, 40), make([]byte, 41)} {
		if _, err := NewFacetReadCut(input); err == nil {
			t.Fatal("invalid cut accepted")
		}
	}
}
