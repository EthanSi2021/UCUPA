package token

import "ucupa/shared/values"

// PublicSigner is a call-local, fixed-purpose signer for Core public
// revision projections. Its key and owner are deliberately private: callers
// can request only the fixed public values that Core actually publishes, not
// an arbitrary kind/parts MAC.
type PublicSigner struct {
	key   [32]byte
	owner values.FactAuthorityRef
}

// NewPublicSigner constructs the internal signer at the owner boundary. The
// caller remains responsible for obtaining the key and authority from the
// owning Core state; this value retains neither state nor a mutable view.
func NewPublicSigner(key [32]byte, owner values.FactAuthorityRef) PublicSigner {
	return PublicSigner{key: key, owner: owner}
}

// ElementRevision signs one Element revision using the existing fixed
// projection algorithm.
func (signer PublicSigner) ElementRevision(revision Revision) values.PublicToken {
	return PublicElementRevision(signer.key, signer.owner, revision)
}

// MountRevision signs one mount revision using the existing fixed projection
// algorithm.
func (signer PublicSigner) MountRevision(revision MountRevision) values.PublicToken {
	return PublicMountRevision(signer.key, signer.owner, revision)
}

// MountContact signs one directional mount contact revision using the
// existing fixed projection algorithm.
func (signer PublicSigner) MountContact(revision NextHopRevision) values.PublicToken {
	return PublicMountContact(signer.key, signer.owner, revision)
}

// AttachmentRevision signs one attachment revision using the existing fixed
// projection algorithm, including its historical zero-value behavior.
func (signer PublicSigner) AttachmentRevision(revision values.AttachmentRevision) values.PublicToken {
	return PublicAttachmentRevision(signer.key, signer.owner, revision)
}

// FacetProposal projects an own key or retains an already authenticated source's
// original comparison. It never reissues another Core's key under this owner.
// This representation check grants no access or current relationship authority.
func (signer PublicSigner) FacetProposal(key values.FacetAcceptanceKey, original values.PublicToken) values.PublicToken {
	if !key.Valid() {
		return values.PublicToken{}
	}
	if original == (values.PublicToken{}) {
		original = signer.AttachmentRevision(key.Proposal)
	}
	if !original.Valid() || original.Owner != key.Core || original.Kind != "attachment-revision" {
		return values.PublicToken{}
	}
	return original
}

// FacetContact signs one Core-issued Facet contact revision using the
// existing fixed projection algorithm.
func (signer PublicSigner) FacetContact(revision FacetContactRevision) values.PublicToken {
	return PublicFacetContact(signer.key, signer.owner, revision)
}
