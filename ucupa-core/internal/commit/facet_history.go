package commit

import (
	"bytes"
	"errors"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/token"
)

type attachmentHistory struct {
	events   []attachmentEvent
	index    int
	issued   map[values.AttachmentRevision]bool
	contacts map[contract.FacetContactRevision]bool
}

func newAttachmentHistory(events []attachmentEvent) *attachmentHistory {
	return &attachmentHistory{events: events, issued: make(map[values.AttachmentRevision]bool), contacts: make(map[contract.FacetContactRevision]bool)}
}

func (h *attachmentHistory) capture(state *coreState) {
	for _, record := range state.attachments.facts.Records() {
		h.issued[record.Attachment.Revision] = true
		if token.ValidFacetContactRevision(record.Contact) {
			h.contacts[record.Contact] = true
		}
	}
}

func (h *attachmentHistory) commit(state *coreState, event coreCommit) error {
	if event.operation != "" || event.controlIndex != uint64(h.index)+1 || h.index >= len(h.events) {
		return errors.New("attachment commit has no matching journal entry")
	}
	next := h.events[h.index]
	if next.before != state.currentCut() || next.result.Status != contract.MutationCommitted {
		return errors.New("attachment commit cut/status mismatch")
	}
	if err := h.apply(state, next); err != nil {
		return err
	}
	return nil
}

func (h *attachmentHistory) decisions(state *coreState) error {
	for h.index < len(h.events) {
		event := h.events[h.index]
		if token.Scope(event.before) != state.scope || token.Serial(event.before) < state.cutSerial {
			return errors.New("attachment journal out of order")
		}
		if event.before != state.currentCut() || event.result.Status == contract.MutationCommitted {
			break
		}
		if err := h.apply(state, event); err != nil {
			return err
		}
	}
	return nil
}

func (h *attachmentHistory) apply(state *coreState, event attachmentEvent) error {
	b := event.binding
	if !commitbinding.ValidOperationID(event.id) || !b.Valid() || state.mountOperationClaimed(event.id) {
		return errors.New("invalid or already claimed attachment operation")
	}
	if _, ok := state.operations[event.id]; ok {
		return errors.New("attachment operation collides with Element operation")
	}
	if err := state.guardTransferElement(b.Element); err != nil {
		return errors.New("attachment decision bypassed source transfer fence")
	}
	if b.Kind == facetfact.Address && b.Key.Core != state.authority {
		return errors.New("attachment update targets another Core")
	}
	if b.Expected.Valid() && bytes.Equal(b.Expected.Bytes()[:32], state.scope[:]) && !h.issued[b.Expected] {
		return errors.New("attachment expected token was never issued")
	}
	if token.ValidFacetContactRevision(b.Contact) && token.FacetContactScope(b.Contact) == state.scope && !h.contacts[b.Contact] {
		return errors.New("attachment contact token was never issued")
	}
	if event.evidence != (contract.FacetAcceptanceEvidence{}) {
		if b.Kind != facetfact.Activate || !event.evidence.Key.Valid() || event.evidence.Key.Core != state.authority ||
			event.evidence.Key.Element != b.Element || event.evidence.Key.Proposal != b.Expected || !event.evidence.AcceptedAt.Valid() {
			return errors.New("unbound Facet acceptance evidence")
		}
	}
	if event.disposition != (contract.FacetDispositionRead{}) {
		if b.Kind != facetfact.Release || !event.disposition.Found || !event.disposition.Key.Valid() ||
			event.disposition.Facet != b.Target || event.disposition.Element != b.Element || !event.disposition.Cut.Valid() {
			return errors.New("unbound Facet disposition evidence")
		}
	}
	result, code := decideAttachment(state, b, event.evidence, event.disposition)
	if result != event.result || code != event.code || code == facetfact.StoreFailure {
		return errors.New("attachment result cannot be reproduced")
	}
	state.attachments.events = append(state.attachments.events, event)
	state.attachments.operations[event.id] = event
	if result.Status == contract.MutationCommitted {
		state.commits = append(state.commits, coreCommit{kind: attachmentCommitted, controlIndex: uint64(h.index) + 1})
	}
	h.index++
	h.capture(state)
	return nil
}
