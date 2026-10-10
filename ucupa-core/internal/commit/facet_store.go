package commit

import (
	"encoding/json"
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
)

type storedAttachmentEvent struct {
	ID                 []byte
	ActorRealm         []byte
	ActorPrincipal     []byte
	Kind               facetfact.Kind
	Element            values.ElementID
	Target             values.FactAuthorityRef
	Address            values.NextHop
	Expected           []byte
	Key                storerecord.StoredAttachmentKey
	Contact            storerecord.StoredFacetContact
	EvidenceKey        storerecord.StoredAttachmentKey
	AcceptedAt         []byte
	DispositionFound   bool
	DispositionFacet   values.FactAuthorityRef
	DispositionElement values.ElementID
	DispositionKey     storerecord.StoredAttachmentKey
	DispositionCut     []byte
	Before             uint64
	After              uint64
	Code               facetfact.Code
	Status             contract.MutationStatus
	Attachment         storerecord.StoredFacetAttachment
	Pending            storerecord.StoredAttachmentKey
	NextHop            values.NextHop
	NextHopRevision    storerecord.StoredFacetContact
}

func storeAttachmentEvent(e attachmentEvent) storedAttachmentEvent {
	b, r := e.binding, e.result
	var expected, accepted, dispositionCut []byte
	if b.Expected.Valid() {
		expected = b.Expected.Bytes()
	}
	if e.evidence.AcceptedAt.Valid() {
		accepted = e.evidence.AcceptedAt.Bytes()
	}
	if e.disposition.Cut.Valid() {
		dispositionCut = e.disposition.Cut.Bytes()
	}
	return storedAttachmentEvent{ID: []byte(e.id), ActorRealm: []byte(b.Actor.Realm), ActorPrincipal: []byte(b.Actor.Principal), Kind: b.Kind, Element: b.Element, Target: b.Target, Address: b.Address,
		Expected: expected, Key: storerecord.StoreAttachmentKey(b.Key), Contact: storerecord.StoreFacetContact(b.Contact), EvidenceKey: storerecord.StoreAttachmentKey(e.evidence.Key), AcceptedAt: accepted,
		DispositionFound: e.disposition.Found, DispositionFacet: e.disposition.Facet, DispositionElement: e.disposition.Element,
		DispositionKey: storerecord.StoreAttachmentKey(e.disposition.Key), DispositionCut: dispositionCut,
		Before: token.Serial(e.before), After: token.Serial(r.OutcomeCut), Code: e.code, Status: r.Status, Attachment: storerecord.StoreFacetAttachment(r.Attachment), Pending: storerecord.StoreAttachmentKey(r.Pending), NextHop: r.NextHop, NextHopRevision: storerecord.StoreFacetContact(r.NextHopRevision)}
}

func (s storedAttachmentEvent) event(scope [32]byte) (attachmentEvent, error) {
	e := attachmentEvent{id: contract.OperationID(s.ID), before: token.FromParts(scope, s.Before), code: s.Code}
	b := facetfact.Binding{Actor: values.PrincipalRef{Realm: values.RealmID(s.ActorRealm), Principal: values.PrincipalID(s.ActorPrincipal)}, Kind: s.Kind, Element: s.Element, Target: s.Target, Address: s.Address}
	var err error
	if len(s.Expected) > 0 {
		b.Expected, err = values.NewAttachmentRevision(s.Expected)
		if err != nil {
			return e, err
		}
	}
	b.Key, err = s.Key.Key()
	if err != nil {
		return e, err
	}
	b.Contact, err = s.Contact.Contact()
	if err != nil {
		return e, err
	}
	e.binding = b
	e.evidence.Key, err = s.EvidenceKey.Key()
	if err != nil {
		return e, err
	}
	if len(s.AcceptedAt) > 0 {
		e.evidence.AcceptedAt, err = values.NewFacetReadCut(s.AcceptedAt)
		if err != nil {
			return e, err
		}
	}
	e.disposition = contract.FacetDispositionRead{Found: s.DispositionFound, Facet: s.DispositionFacet, Element: s.DispositionElement}
	e.disposition.Key, err = s.DispositionKey.Key()
	if err != nil {
		return e, err
	}
	if len(s.DispositionCut) > 0 {
		e.disposition.Cut, err = values.NewFacetReadCut(s.DispositionCut)
		if err != nil {
			return e, err
		}
	}
	e.result = contract.FacetAttachmentResult{Status: s.Status, OutcomeCut: token.FromParts(scope, s.After), NextHop: s.NextHop}
	e.result.Attachment, err = s.Attachment.Attachment()
	if err != nil {
		return e, err
	}
	e.result.Pending, err = s.Pending.Key()
	if err != nil {
		return e, err
	}
	e.result.NextHopRevision, err = s.NextHopRevision.Contact()
	return e, err
}

func writeAttachmentJournal(bucket storeByteSink, events []attachmentEvent) error {
	stored := make([]storedAttachmentEvent, 0, len(events))
	for _, event := range events {
		stored = append(stored, storeAttachmentEvent(event))
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return bucket.Put(attachmentJournalKey, raw)
}

func loadAttachmentJournal(raw []byte, scope [32]byte) ([]attachmentEvent, error) {
	var stored []storedAttachmentEvent
	if err := storebytes.DecodeCanonicalJSON(raw, &stored); err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, errors.New("missing attachment journal")
	}
	events := make([]attachmentEvent, 0, len(stored))
	for _, item := range stored {
		event, err := item.event(scope)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}
