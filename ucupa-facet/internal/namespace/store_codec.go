package namespace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/token"
)

type storeDocument struct {
	Authority values.FactAuthorityRef
	Scope     [32]byte
	Facts     facetState
	Events    []event
}

func validateHistory(owner values.FactAuthorityRef, scope [32]byte, facts facetState, events []event) error {
	if !owner.Valid() || scope == [32]byte{} || facts.PublicKey == ([32]byte{}) {
		return contract.ErrStore
	}
	// The one Realm contact is part of the owner's complete facts: a document
	// or in-memory state without it is not a valid Facet history.
	if facts.RealmContact == nil {
		return contract.ErrStore
	}
	if err := facts.RealmContact.validateRealmContactHistory(owner, scope); err != nil {
		return err
	}
	replay := emptyState()
	replay.PublicKey = facts.PublicKey
	contact := facts.RealmContact.clone()
	replay.RealmContact = &contact
	seen := map[byteString]bool{}
	issuedRevisions := map[contract.Revision]bool{}
	issuedContacts := map[contract.ContactRevision]bool{}
	publicClaims := map[keyData]contract.PublicAcceptanceRef{}
	publicKeys := map[contract.PublicAcceptanceRef]keyData{}
	for i, e := range events {
		if !e.Input.valid(owner) || !publicExpectedMatches(e.Input, scope, facts.PublicKey) || !validatePublicAcceptanceHistory(e, publicClaims, publicKeys) || seen[e.Input.ID] {
			return fmt.Errorf("%w: invalid operation at %d", contract.ErrStore, i)
		}
		seen[e.Input.ID] = true
		expected := e.Input.Expected.value()
		expectedScope, _, _, _ := token.RevisionParts(expected)
		if token.ValidRevision(expected) && expectedScope == scope && !issuedRevisions[expected] {
			return fmt.Errorf("%w: never-issued revision", contract.ErrStore)
		}
		contactToken := token.NewContactRevision(e.Input.ContactScope, e.Input.ContactKey.value(), e.Input.ContactSerial)
		contactScope, _, _ := token.ContactRevisionParts(contactToken)
		if token.ValidContactRevision(contactToken) && contactScope == scope && !issuedContacts[contactToken] {
			return fmt.Errorf("%w: never-issued contact token", contract.ErrStore)
		}
		if e.Input.Kind == "contact" && e.Core != (proof{}) {
			return contract.ErrStore
		}
		if e.Input.Kind == "accept" && e.Core != (proof{}) && !e.Core.valid(e.Input.Key, true) && !e.Core.validReaccept(e.Input.Key) && !e.Core.validRecovery(e.Input.Key) {
			return contract.ErrStore
		}
		// A re-acceptance records the previous Core's successor answer next
		// to the new Core's proposal evidence — either the proven departure
		// to exactly this Core or the proven absence that rejects the move;
		// a transferred retire records the departed Core's proven successor
		// statement next to its missing disclosure. Every other decision
		// records none.
		if e.Departure != (departureData{}) && e.Input.Kind != "accept" && e.Input.Kind != "retire" {
			return contract.ErrStore
		}
		if e.Input.Kind == "accept" && e.Departure != (departureData{}) &&
			(!e.Core.validReaccept(e.Input.Key) ||
				(!e.Departure.validSuccessor(e.Core.Active, e.Input.Key) && !e.Departure.validAbsence(e.Core.Active))) {
			return contract.ErrStore
		}
		// Each transfer step records exactly its own external evidence: the
		// controlling plan for fence, accept-namespace and settle, the Core
		// switch proof for install, the Core's retirement disclosure for
		// retire, and nothing for refuse or plain writes.
		switch e.Input.Kind {
		case "fence", "accept-namespace", "settle":
			if e.Core != (proof{}) || !e.Plan.present() {
				return contract.ErrStore
			}
		case "install":
			if e.Plan != (transferPlan{}) || e.Core == (proof{}) || !e.Core.valid(e.Input.Key, false) {
				return contract.ErrStore
			}
		case "retire":
			if e.Plan != (transferPlan{}) {
				return contract.ErrStore
			}
			if e.Core == (proof{}) {
				// Without a live contact the decision refuses before reading any
				// evidence; only the no-acceptance refusals may journal so.
				if e.Departure != (departureData{}) || e.Outcome.Status != contract.Rejected ||
					(e.Outcome.Error != "acceptance" && e.Outcome.Error != "retired") {
					return contract.ErrStore
				}
			} else if e.Core.validRetirement(e.Input.Key) {
				// The local DETACHED disclosure retires alone; no departure
				// statement may ride on it.
				if e.Departure != (departureData{}) {
					return contract.ErrStore
				}
			} else if !e.Core.validRetireDeparture(e.Input.Key, e.Departure) {
				// The transferred disposal shape: the departed Core's proven
				// missing disclosure plus its durable successor statement.
				return contract.ErrStore
			}
		case "refuse":
			if e.Core != (proof{}) || e.Plan != (transferPlan{}) {
				return contract.ErrStore
			}
		case "put", "delete":
			if e.Plan != (transferPlan{}) {
				return contract.ErrStore
			}
		}
		// Install reissues one target-scope revision per live manifest entry at
		// the event cut; the pending manifest is the only place to read it.
		var installedManifest []manifestEntryData
		if e.Input.Kind == "install" {
			if pending, ok := replay.Pending[e.Input.Key.id()]; ok && pending.Operation == e.Input.Operation {
				installedManifest = pending.Manifest
			}
		}
		out, err := decide(&replay, scope, e.Input, e.Core, e.Departure, e.Plan)
		if err != nil || !reflect.DeepEqual(out, e.Outcome) {
			return fmt.Errorf("%w: impossible history outcome at %d", contract.ErrStore, i)
		}
		if e.Input.Kind == "install" && out.Status == contract.Committed {
			for _, m := range installedManifest {
				if m.Live {
					issuedRevisions[token.NewRevision(scope, e.Input.Key.Element, string(m.Type), out.Cut)] = true
				}
			}
		}
		if r := out.Revision.value(); token.ValidRevision(r) {
			issuedRevisions[r] = true
		}
		if out.ContactSerial > 0 {
			issuedContacts[token.NewContactRevision(scope, e.Input.Key.value(), out.ContactSerial)] = true
		}
	}
	for _, contactEvent := range facts.RealmContact.Events {
		if seen[byteString(contactEvent.Request.OperationID)] {
			return contract.ErrStore
		}
	}
	if !reflect.DeepEqual(replay, facts) {
		return fmt.Errorf("%w: current facts differ from history", contract.ErrStore)
	}
	return nil
}
func encodeDocument(doc storeDocument) ([]byte, error) {
	if err := validateHistory(doc.Authority, doc.Scope, doc.Facts, doc.Events); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}
func decodeDocument(raw []byte) (storeDocument, error) {
	var doc storeDocument
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&doc); err != nil {
		return doc, errors.Join(contract.ErrStore, err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return doc, contract.ErrStore
	}
	canonical, err := encodeDocument(doc)
	if err != nil || !bytes.Equal(canonical, raw) {
		return storeDocument{}, errors.Join(contract.ErrStore, err)
	}
	return doc, nil
}
func (f *Owner) install(doc storeDocument) {
	if f.public != nil && f.state.Cut != doc.Facts.Cut {
		f.public.GapAllLocked()
	}
	f.state = doc.Facts
	f.events = doc.Events
	f.operations = map[byteString]int{}
	for i, e := range f.events {
		f.operations[e.Input.ID] = i
	}
	f.refreshContactLocked()
}
