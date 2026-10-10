package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	"ucupa/shared/values"
)

// publicRevision is deliberately private. Public callers receive only the
// fixed-purpose projections below; an arbitrary kind/parts MAC would be a
// generic signing oracle rather than a comparison value.
func publicRevision(key [32]byte, owner values.FactAuthorityRef, kind string, parts ...[]byte) values.PublicToken {
	mac := hmac.New(sha256.New, key[:])
	writePart := func(part []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write(part)
	}
	writePart([]byte("ucupa.core.public-revision"))
	writePart([]byte(owner))
	writePart([]byte(kind))
	for _, part := range parts {
		writePart(part)
	}
	var raw [32]byte
	copy(raw[:], mac.Sum(nil))
	token, _ := values.PublicTokenFromBytes(owner, kind, raw)
	return token
}

// PublicElementRevision signs one valid Core Element revision.
func PublicElementRevision(key [32]byte, owner values.FactAuthorityRef, revision Revision) values.PublicToken {
	if !ValidRevision(revision) {
		return values.PublicToken{}
	}
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], RevisionSerial(revision))
	return publicRevision(key, owner, "record-revision", RevisionScopeBytes(revision), []byte(RevisionElement(revision)), serial[:])
}

// PublicMountRevision signs one valid Core mount revision.
func PublicMountRevision(key [32]byte, owner values.FactAuthorityRef, revision MountRevision) values.PublicToken {
	if !ValidMountRevision(revision) {
		return values.PublicToken{}
	}
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], MountRevisionSerial(revision))
	return publicRevision(key, owner, "mount-revision", MountRevisionScopeBytes(revision), []byte(MountRevisionElement(revision)), serial[:])
}

// PublicMountContact signs one valid directional mount contact revision.
func PublicMountContact(key [32]byte, owner values.FactAuthorityRef, revision NextHopRevision) values.PublicToken {
	if !ValidNextHopRevision(revision) {
		return values.PublicToken{}
	}
	var serial [16]byte
	binary.BigEndian.PutUint64(serial[:8], NextHopSerial(revision))
	binary.BigEndian.PutUint64(serial[8:], MountRevisionSerial(NextHopKey(revision).ProposalRevision))
	direction := byte(0)
	if NextHopIncoming(revision) {
		direction = 1
	}
	keyParts := NextHopKey(revision)
	return publicRevision(key, owner, "contact-revision", []byte("mount"), NextHopScopeBytes(revision),
		[]byte(keyParts.ParentAuthority), []byte(keyParts.CarrierAuthority), []byte(keyParts.MountAnchorUUID),
		MountRevisionScopeBytes(keyParts.ProposalRevision), serial[:], []byte{direction})
}

// PublicAttachmentRevision preserves the original attachment-revision
// projection, including its historical behavior for a zero value.
func PublicAttachmentRevision(key [32]byte, owner values.FactAuthorityRef, revision values.AttachmentRevision) values.PublicToken {
	return publicRevision(key, owner, "attachment-revision", revision.Bytes())
}

// PublicFacetContact signs one valid Core-issued Facet contact revision.
func PublicFacetContact(key [32]byte, owner values.FactAuthorityRef, revision FacetContactRevision) values.PublicToken {
	if !ValidFacetContactRevision(revision) {
		return values.PublicToken{}
	}
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], FacetContactSerial(revision))
	contactKey := FacetContactKey(revision)
	return publicRevision(key, owner, "contact-revision", FacetContactScopeBytes(revision), contactKey.Proposal.Bytes(), serial[:])
}
