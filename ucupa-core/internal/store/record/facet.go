package record

import (
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

type StoredAttachmentKey struct {
	Core     values.FactAuthorityRef
	Facet    values.FactAuthorityRef
	Element  values.ElementID
	Proposal []byte
}

type StoredFacetContact struct {
	Scope  [32]byte
	Key    StoredAttachmentKey
	Serial uint64
}

type StoredFacetAttachment struct {
	State            contract.FacetAttachmentState
	Revision         []byte
	Key              StoredAttachmentKey
	NextHop          values.NextHop
	Contact          StoredFacetContact
	OriginalProposal *values.PublicToken `json:",omitempty"`
}

func StoreAttachmentKey(key values.FacetAcceptanceKey) StoredAttachmentKey {
	if key == (values.FacetAcceptanceKey{}) {
		return StoredAttachmentKey{}
	}
	return StoredAttachmentKey{Core: key.Core, Facet: key.Facet, Element: key.Element, Proposal: key.Proposal.Bytes()}
}

func (s StoredAttachmentKey) Key() (values.FacetAcceptanceKey, error) {
	if s.Core == "" && s.Facet == "" && s.Element == "" && len(s.Proposal) == 0 {
		return values.FacetAcceptanceKey{}, nil
	}
	p, err := values.NewAttachmentRevision(s.Proposal)
	k := values.FacetAcceptanceKey{Core: s.Core, Facet: s.Facet, Element: s.Element, Proposal: p}
	if err != nil || !k.Valid() {
		return values.FacetAcceptanceKey{}, errors.New("invalid stored attachment key")
	}
	return k, nil
}

func StoreFacetContact(c contract.FacetContactRevision) StoredFacetContact {
	return StoredFacetContact{Scope: token.FacetContactScope(c), Key: StoreAttachmentKey(token.FacetContactKey(c)), Serial: token.FacetContactSerial(c)}
}

func (s StoredFacetContact) Contact() (contract.FacetContactRevision, error) {
	k, err := s.Key.Key()
	if err != nil {
		return contract.FacetContactRevision{}, err
	}
	c := token.FacetContactRevisionFromParts(s.Scope, k, s.Serial)
	if c != (contract.FacetContactRevision{}) && !token.ValidFacetContactRevision(c) {
		return contract.FacetContactRevision{}, errors.New("invalid stored Facet contact token")
	}
	return c, nil
}

func StoreFacetAttachment(a contract.FacetAttachment) StoredFacetAttachment {
	var revision []byte
	if a.Revision != (values.AttachmentRevision{}) {
		revision = a.Revision.Bytes()
	}
	var original *values.PublicToken
	if a.OriginalProposal != (values.PublicToken{}) {
		copy := a.OriginalProposal
		original = &copy
	}
	return StoredFacetAttachment{State: a.State, Revision: revision, Key: StoreAttachmentKey(a.Key), NextHop: a.NextHop, Contact: StoreFacetContact(a.NextHopRevision), OriginalProposal: original}
}

func (s StoredFacetAttachment) Attachment() (contract.FacetAttachment, error) {
	k, err := s.Key.Key()
	if err != nil {
		return contract.FacetAttachment{}, err
	}
	c, err := s.Contact.Contact()
	if err != nil {
		return contract.FacetAttachment{}, err
	}
	var revision values.AttachmentRevision
	if len(s.Revision) > 0 {
		revision, err = values.NewAttachmentRevision(s.Revision)
		if err != nil {
			return contract.FacetAttachment{}, err
		}
	}
	var original values.PublicToken
	if s.OriginalProposal != nil {
		original = *s.OriginalProposal
		if !original.Valid() || original.Owner != k.Core || original.Kind != "attachment-revision" {
			return contract.FacetAttachment{}, errors.New("invalid original Facet proposal comparison")
		}
	}
	return contract.FacetAttachment{State: s.State, Revision: revision, Key: k, NextHop: s.NextHop, NextHopRevision: c, OriginalProposal: original}, nil
}
