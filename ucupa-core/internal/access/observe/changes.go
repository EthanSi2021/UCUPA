// Package observe determines relevance from one locked publication's finite views.
// It owns no sessions, queues, authorization, persistence or lifecycle callbacks.
package observe

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// View borrows immutable current fact tables, never commit journals or receipts.
type View struct {
	Present     bool
	CutSerial   uint64
	Elements    elementfact.View
	Mount       mountfact.View
	Attachments facetfact.View
	Transfers   transferfact.Visibility
}

// Selection is the already-established observer's relevant scope. It does not
// establish permission or make a new disclosure decision.
type Selection struct {
	Boundary          values.ElementID
	Exact             values.ElementID
	Path              map[values.ElementID]bool
	IncludeAttachment bool
	Control           bool
	Federated         bool
	FederatedRead     bool
}

// Changes retains the original spatial/attachment classification. The owner
// calls it during publication and atomically enqueues a whole group or GAP.
func Changes(before, after View, selection Selection) (bool, bool) {
	spatial, attachment := false, false
	if selection.Federated && before.Mount.Incoming() != after.Mount.Incoming() {
		spatial = true
	}
	relevant := func(id values.ElementID) bool { return selectedElement(before, after, selection, id) }
	// Completion can make an already installed member usable without changing
	// its element revision. A pending observer still needs this coverage change.
	for id := range before.Transfers.InactiveIDs() {
		if !after.Transfers.Inactive(id) && relevant(id) {
			spatial = true
		}
	}
	for id, old := range before.Elements.Anchors() {
		if next, exists := after.Elements.Anchor(id); (!exists || old != next) && relevant(id) {
			spatial = true
		}
	}
	for id := range after.Elements.Anchors() {
		if _, exists := before.Elements.Anchor(id); !exists && relevant(id) {
			spatial = true
		}
	}
	for id, old := range before.Elements.Entities() {
		if next, exists := after.Elements.Entity(id); (!exists || old != next) && relevant(id) {
			spatial = true
		}
	}
	for id := range after.Elements.Entities() {
		if _, exists := before.Elements.Entity(id); !exists && relevant(id) {
			spatial = true
		}
	}
	if selection.IncludeAttachment {
		attachmentRelevant := relevant
		if selection.FederatedRead {
			attachmentRelevant = func(id values.ElementID) bool { return selection.Exact == id }
		}
		for id, old := range before.Attachments.Records() {
			if next, exists := after.Attachments.Record(id); (!exists || old.Attachment != next.Attachment) && attachmentRelevant(id) {
				attachment = true
			}
		}
		for id := range after.Attachments.Records() {
			if _, exists := before.Attachments.Record(id); !exists && attachmentRelevant(id) {
				attachment = true
			}
		}
	}
	if selection.Control { // Only existence/attachment controls, never spatial movement.
		spatial = false
		if before.Elements.Kind(selection.Exact) != after.Elements.Kind(selection.Exact) {
			attachment = true
		}
	}
	if selection.Exact == "" {
		for id, old := range before.Mount.Outgoings() {
			if next, exists := after.Mount.Outgoing(id); (!exists || old != next) && relevant(id) {
				spatial = true
			}
		}
		for id := range after.Mount.Outgoings() {
			if _, exists := before.Mount.Outgoing(id); !exists && relevant(id) {
				spatial = true
			}
		}
	}
	return spatial, attachment
}

func selectedElement(before, after View, selection Selection, id values.ElementID) bool {
	if len(selection.Path) > 0 {
		return selection.Path[id] || selection.Exact == id
	}
	if selection.Exact != "" {
		return selection.Exact == id
	}
	return elementfact.Member(before.Elements, id, selection.Boundary) || elementfact.Member(after.Elements, id, selection.Boundary)
}

// SourceLost is not an ordinary spatial deletion. The affected observation
// either retains provable terminal evidence for its ended scope or ends in
// GAP. Existing departures do not invalidate unrelated observers on later
// writes.
func SourceLost(before, after View, selection Selection) bool {
	if GateLost(before, after) {
		return true
	}
	for id := range after.Transfers.InactiveIDs() {
		if !before.Transfers.Inactive(id) && selectedElement(before, after, selection, id) {
			return true
		}
	}
	return false
}

// GateLost reports whether this publication closed the incoming gate itself:
// the parent fenced or reparented this whole space. That loss is never this
// owner's own transfer decision, so no retained proof can disclose it.
func GateLost(before, after View) bool {
	return after.Mount.Incoming().Gate == mountfact.GateReparenting && before.Mount.Incoming().Gate != mountfact.GateReparenting ||
		after.Mount.Incoming().Gate == mountfact.GateFenced && before.Mount.Incoming().Gate != mountfact.GateFenced
}

// LostDepartures returns every relevant newly inactive element that made this
// publication lose the selected source scope. A gate change alone loses the
// scope without any lost element.
func LostDepartures(before, after View, selection Selection) []values.ElementID {
	var lost []values.ElementID
	for id := range after.Transfers.InactiveIDs() {
		if before.Transfers.Inactive(id) || !selectedElement(before, after, selection, id) {
			continue
		}
		lost = append(lost, id)
	}
	return lost
}

// SettledSuccessor returns the unique settled successor decision proving every
// recorded loss ended, when the after view proves them all under that one
// decision. One unproven or contradicting element keeps the whole proof absent
// and the loss stays GAP evidence rather than a partial disclosure.
func SettledSuccessor(ids []values.ElementID, after View) (contract.TransitionTerminal, bool) {
	var terminal contract.TransitionTerminal
	found := false
	for _, id := range ids {
		candidate, ok := after.Transfers.Successor(id)
		if !ok {
			return contract.TransitionTerminal{}, false
		}
		if found && candidate != terminal {
			return contract.TransitionTerminal{}, false
		}
		terminal = candidate
		found = true
	}
	return terminal, found
}

// AllPendingDecisions reports whether every lost element departed under this
// owner's own transfer decision that has fenced or committed but not yet
// settled. The queue then neither streams nor Gaps: a later publication proves
// the terminal successor, and a stuck decision loses its retention window
// and ends in GAP.
func AllPendingDecisions(lost []values.ElementID, after View) bool {
	if len(lost) == 0 {
		return false
	}
	for _, id := range lost {
		if !after.Transfers.PendingDecision(id) {
			return false
		}
	}
	return true
}
