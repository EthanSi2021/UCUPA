package token

import (
	"testing"
	"ucupa/shared/values"
)

const testElement values.ElementID = "00000000-0000-4000-8000-00000000fa01"

func testAcceptanceKey(t *testing.T) values.FacetAcceptanceKey {
	t.Helper()
	var raw [76]byte
	raw[0] = 1
	copy(raw[32:68], testElement)
	raw[75] = 1
	proposal, err := values.NewAttachmentRevision(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	key := values.FacetAcceptanceKey{
		Core:     "00000000-0000-4000-8000-00000000ca01",
		Facet:    "00000000-0000-4000-8000-00000000fa02",
		Element:  testElement,
		Proposal: proposal,
	}
	if !key.Valid() {
		t.Fatal("fixture acceptance key is invalid")
	}
	return key
}

func TestRevisionConstructionPartsAndValidity(t *testing.T) {
	var scope [32]byte
	scope[0] = 1
	revision := NewRevision(scope, testElement, "0123456789abcdef0123456789abcdef", 7)
	if !ValidRevision(revision) || !revision.Equal(revision) || IsZeroRevision(revision) {
		t.Fatal("issued revision validity")
	}
	gotScope, gotElement, gotType, gotSerial := RevisionParts(revision)
	if gotScope != scope || gotElement != testElement || gotType != "0123456789abcdef0123456789abcdef" || gotSerial != 7 {
		t.Fatal("revision parts")
	}
	if !IsZeroRevision(Revision{}) || ValidRevision(Revision{}) {
		t.Fatal("zero revision")
	}
}

func TestContactConstructionPartsAndValidity(t *testing.T) {
	var scope [32]byte
	scope[0] = 2
	key := testAcceptanceKey(t)
	revision := NewContactRevision(scope, key, 9)
	if !ValidContactRevision(revision) || !revision.Equal(revision) {
		t.Fatal("issued contact revision validity")
	}
	gotScope, gotKey, gotSerial := ContactRevisionParts(revision)
	if gotScope != scope || gotKey != key || gotSerial != 9 {
		t.Fatal("contact revision parts")
	}
	if ValidContactRevision(ContactRevision{}) {
		t.Fatal("zero contact revision")
	}
}

func TestPutPreconditionFormsAndValidation(t *testing.T) {
	var scope [32]byte
	scope[0] = 3
	revision := NewRevision(scope, testElement, "0123456789abcdef0123456789abcdef", 11)
	absent := NewAbsentPrecondition()
	if !ValidPutPrecondition(absent) {
		t.Fatal("absent precondition")
	}
	absentFlag, absentRevision := PutPreconditionParts(absent)
	if !absentFlag || !IsZeroRevision(absentRevision) {
		t.Fatal("absent precondition parts")
	}
	expected := NewRevisionPrecondition(revision)
	if !ValidPutPrecondition(expected) {
		t.Fatal("revision precondition")
	}
	expectedFlag, expectedRevision := PutPreconditionParts(expected)
	if expectedFlag || !expectedRevision.Equal(revision) {
		t.Fatal("revision precondition parts")
	}
}
