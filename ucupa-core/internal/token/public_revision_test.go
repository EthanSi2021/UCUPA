package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"

	"ucupa/shared/values"
)

const (
	publicRevisionOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	publicRevisionPeer    values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	publicRevisionFacet   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000003"
	publicRevisionElement values.ElementID        = "00000000-0000-4000-8000-000000000004"
)

var publicRevisionKey = [32]byte{
	0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
	0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	0x10, 0x21, 0x32, 0x43, 0x54, 0x65, 0x76, 0x87,
	0x98, 0xa9, 0xba, 0xcb, 0xdc, 0xed, 0xfe, 0x0f,
}

func legacyPublicRevisionForTest(key [32]byte, owner values.FactAuthorityRef, kind string, parts ...[]byte) values.PublicToken {
	mac := hmac.New(sha256.New, key[:])
	all := append([][]byte{[]byte("ucupa.core.public-revision"), []byte(owner), []byte(kind)}, parts...)
	for _, part := range all {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write(part)
	}
	var raw [32]byte
	copy(raw[:], mac.Sum(nil))
	token, _ := values.PublicTokenFromBytes(owner, kind, raw)
	return token
}

func publicRevisionFixture(t *testing.T) (Revision, MountRevision, NextHopRevision, NextHopRevision, FacetContactRevision, values.AttachmentRevision) {
	t.Helper()
	scope := [32]byte{1, 3, 5, 7, 9}
	revision := RevisionFromParts(scope, publicRevisionElement, math.MaxUint64)
	mountRevision := MountRevisionFromParts(scope, publicRevisionElement, math.MaxUint64-1)
	mountKey := MountKey{ParentAuthority: publicRevisionOwner, MountAnchorUUID: publicRevisionElement,
		ProposalRevision: mountRevision, CarrierAuthority: publicRevisionPeer}
	if !mountKey.Valid() {
		t.Fatal("fixed mount key is invalid")
	}
	mountIncoming := NextHopRevisionFromParts(scope, mountKey, true, math.MaxUint64-2)
	mountOutgoing := NextHopRevisionFromParts(scope, mountKey, false, math.MaxUint64-3)
	var raw [76]byte
	raw[0], raw[75] = 0xa5, 1
	copy(raw[32:68], publicRevisionElement)
	binary.BigEndian.PutUint64(raw[68:], math.MaxUint64-4)
	attachmentRevision, err := values.NewAttachmentRevision(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	facetKey := values.FacetAcceptanceKey{Core: publicRevisionOwner, Facet: publicRevisionFacet,
		Element: publicRevisionElement, Proposal: attachmentRevision}
	facetContact := FacetContactRevisionFromParts(scope, facetKey, math.MaxUint64-5)
	if !ValidRevision(revision) || !ValidMountRevision(mountRevision) || !ValidNextHopRevision(mountIncoming) ||
		!ValidNextHopRevision(mountOutgoing) || !ValidFacetContactRevision(facetContact) {
		t.Fatal("fixed public revision fixture is invalid")
	}
	return revision, mountRevision, mountIncoming, mountOutgoing, facetContact, attachmentRevision
}

func TestPublicRevisionFixedPurposesMatchOriginalAlgorithm(t *testing.T) {
	revision, mountRevision, incoming, outgoing, facetContact, attachment := publicRevisionFixture(t)
	cases := []struct {
		name string
		got  values.PublicToken
		want values.PublicToken
	}{
		{
			name: "record",
			got:  PublicElementRevision(publicRevisionKey, publicRevisionOwner, revision),
			want: legacyPublicRevisionForTest(publicRevisionKey, publicRevisionOwner, "record-revision", RevisionScopeBytes(revision), []byte(RevisionElement(revision)), uint64Bytes(RevisionSerial(revision))),
		},
		{
			name: "mount",
			got:  PublicMountRevision(publicRevisionKey, publicRevisionOwner, mountRevision),
			want: legacyPublicRevisionForTest(publicRevisionKey, publicRevisionOwner, "mount-revision", MountRevisionScopeBytes(mountRevision), []byte(MountRevisionElement(mountRevision)), uint64Bytes(MountRevisionSerial(mountRevision))),
		},
		{
			name: "mount-contact-incoming",
			got:  PublicMountContact(publicRevisionKey, publicRevisionOwner, incoming),
			want: legacyMountContactForTest(publicRevisionKey, publicRevisionOwner, incoming),
		},
		{
			name: "mount-contact-outgoing",
			got:  PublicMountContact(publicRevisionKey, publicRevisionOwner, outgoing),
			want: legacyMountContactForTest(publicRevisionKey, publicRevisionOwner, outgoing),
		},
		{
			name: "attachment",
			got:  PublicAttachmentRevision(publicRevisionKey, publicRevisionOwner, attachment),
			want: legacyPublicRevisionForTest(publicRevisionKey, publicRevisionOwner, "attachment-revision", attachment.Bytes()),
		},
		{
			name: "facet-contact",
			got:  PublicFacetContact(publicRevisionKey, publicRevisionOwner, facetContact),
			want: legacyFacetContactForTest(publicRevisionKey, publicRevisionOwner, facetContact),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Fatalf("fixed projection changed: got %#v want %#v", testCase.got, testCase.want)
			}
		})
	}
	if incoming == outgoing || PublicMountContact(publicRevisionKey, publicRevisionOwner, incoming) == PublicMountContact(publicRevisionKey, publicRevisionOwner, outgoing) {
		t.Fatal("contact direction was omitted from the signed value")
	}
}

func TestPublicRevisionInvalidAndOwnerIsolation(t *testing.T) {
	revision, _, _, _, _, _ := publicRevisionFixture(t)
	if got := PublicElementRevision(publicRevisionKey, publicRevisionOwner, Revision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero Element revision was signed: %#v", got)
	}
	if got := PublicMountRevision(publicRevisionKey, publicRevisionOwner, MountRevision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero mount revision was signed: %#v", got)
	}
	if got := PublicMountContact(publicRevisionKey, publicRevisionOwner, NextHopRevision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero mount contact was signed: %#v", got)
	}
	if got := PublicFacetContact(publicRevisionKey, publicRevisionOwner, FacetContactRevision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero Facet contact was signed: %#v", got)
	}

	ownerToken := PublicElementRevision(publicRevisionKey, publicRevisionOwner, revision)
	otherOwnerToken := PublicElementRevision(publicRevisionKey, publicRevisionPeer, revision)
	otherKey := publicRevisionKey
	otherKey[0]++
	otherKeyToken := PublicElementRevision(otherKey, publicRevisionOwner, revision)
	if ownerToken.Owner != publicRevisionOwner || otherOwnerToken.Owner != publicRevisionPeer ||
		ownerToken == otherOwnerToken || ownerToken == otherKeyToken {
		t.Fatal("public revision lost owner or key isolation")
	}

	zeroAttachment := PublicAttachmentRevision(publicRevisionKey, publicRevisionOwner, values.AttachmentRevision{})
	legacyZeroAttachment := legacyPublicRevisionForTest(publicRevisionKey, publicRevisionOwner, "attachment-revision", values.AttachmentRevision{}.Bytes())
	if zeroAttachment != legacyZeroAttachment || zeroAttachment == (values.PublicToken{}) {
		t.Fatalf("zero attachment revision behavior changed: got %#v want %#v", zeroAttachment, legacyZeroAttachment)
	}
}

func uint64Bytes(value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return encoded[:]
}

func legacyMountContactForTest(key [32]byte, owner values.FactAuthorityRef, revision NextHopRevision) values.PublicToken {
	mountKey := NextHopKey(revision)
	serial := append(uint64Bytes(NextHopSerial(revision)), uint64Bytes(MountRevisionSerial(mountKey.ProposalRevision))...)
	direction := byte(0)
	if NextHopIncoming(revision) {
		direction = 1
	}
	return legacyPublicRevisionForTest(key, owner, "contact-revision", []byte("mount"), NextHopScopeBytes(revision),
		[]byte(mountKey.ParentAuthority), []byte(mountKey.CarrierAuthority), []byte(mountKey.MountAnchorUUID),
		MountRevisionScopeBytes(mountKey.ProposalRevision), serial, []byte{direction})
}

func legacyFacetContactForTest(key [32]byte, owner values.FactAuthorityRef, revision FacetContactRevision) values.PublicToken {
	return legacyPublicRevisionForTest(key, owner, "contact-revision", FacetContactScopeBytes(revision),
		FacetContactKey(revision).Proposal.Bytes(), uint64Bytes(FacetContactSerial(revision)))
}
