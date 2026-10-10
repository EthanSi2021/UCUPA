package token

import (
	"math"
	"testing"

	"ucupa/shared/values"
)

const revisionElement values.ElementID = "00000000-0000-4000-8000-000000000017"

func TestRevisionPartsAndEvidencePreserveExactValues(t *testing.T) {
	scope := [32]byte{1, 2, 3}
	r := RevisionFromParts(scope, revisionElement, math.MaxUint64)
	m := MountRevisionFromParts(scope, revisionElement, math.MaxUint64)
	if !ValidRevision(r) || !ValidMountRevision(m) || RevisionScope(r) != scope || RevisionElement(r) != revisionElement || RevisionSerial(r) != math.MaxUint64 ||
		MountRevisionScope(m) != scope || MountRevisionElement(m) != revisionElement || MountRevisionSerial(m) != math.MaxUint64 {
		t.Fatal("revision parts or uint64 precision changed")
	}
	if !r.Equal(r) || !m.Equal(m) || r.Equal(Revision{}) || m.Equal(MountRevision{}) {
		t.Fatal("revision equality changed")
	}
	if got, err := r.ServiceEvidence().ElementRevision(); err != nil || got != r {
		t.Fatalf("element evidence roundtrip: %v %v", got, err)
	}
	if got, err := m.ServiceEvidence().MountRevision(); err != nil || got != m {
		t.Fatalf("mount evidence roundtrip: %v %v", got, err)
	}
	// Codec slices cannot mutate the source revision.
	RevisionScopeBytes(r)[0] = 9
	MountRevisionScopeBytes(m)[0] = 9
	if RevisionScope(r) != scope || MountRevisionScope(m) != scope {
		t.Fatal("codec view mutated token")
	}
	for _, bad := range []ServiceRevision{{}, {scope, "invalid", 1}, {scope, revisionElement, 0}, {[32]byte{}, revisionElement, 1}} {
		r := RevisionFromParts(bad.Incarnation, bad.Element, bad.Sequence)
		m := MountRevisionFromParts(bad.Incarnation, bad.Element, bad.Sequence)
		if ValidRevision(r) || ValidMountRevision(m) || r.ServiceEvidence() != bad || m.ServiceEvidence() != bad {
			t.Fatal("raw reconstruction normalized invalid evidence")
		}
	}
}

func TestListContinuationScopeAndParts(t *testing.T) {
	scope := [32]byte{1}
	last := values.ElementID("00000000-0000-4000-8000-000000000018")
	cut := FromParts(scope, math.MaxUint64)
	v := ListContinuationFromParts(scope, revisionElement, cut, last)
	if !ValidListContinuation(v) || ListScope(v) != scope || ListParent(v) != revisionElement || ListCut(v) != cut || ListLast(v) != last {
		t.Fatal("list continuation lost original binding")
	}
	for _, bad := range []ListContinuation{{}, ListContinuationFromParts(scope, "bad", cut, last), ListContinuationFromParts(scope, revisionElement, cut, "bad"), ListContinuationFromParts(scope, revisionElement, FromParts([32]byte{2}, 1), last)} {
		if ValidListContinuation(bad) {
			t.Fatal("invalid list continuation accepted")
		}
	}
}

func TestFacetContactPartsRemainBoundToExactKey(t *testing.T) {
	scope := [32]byte{3}
	var raw [76]byte
	raw[0], raw[75] = 1, 1
	copy(raw[32:68], revisionElement)
	proposal, err := values.NewAttachmentRevision(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	key := values.FacetAcceptanceKey{Core: "00000000-0000-4000-8000-000000000019", Facet: "00000000-0000-4000-8000-000000000020", Element: revisionElement, Proposal: proposal}
	v := FacetContactRevisionFromParts(scope, key, math.MaxUint64)
	if !ValidFacetContactRevision(v) || !v.Equal(v) || v.Equal(FacetContactRevision{}) || FacetContactScope(v) != scope || FacetContactKey(v) != key || FacetContactSerial(v) != math.MaxUint64 {
		t.Fatal("contact revision binding changed")
	}
	for _, bad := range []FacetContactRevision{{}, FacetContactRevisionFromParts([32]byte{}, key, 1), FacetContactRevisionFromParts(scope, values.FacetAcceptanceKey{}, 1), FacetContactRevisionFromParts(scope, key, 0)} {
		if ValidFacetContactRevision(bad) {
			t.Fatal("invalid contact accepted")
		}
	}
}
