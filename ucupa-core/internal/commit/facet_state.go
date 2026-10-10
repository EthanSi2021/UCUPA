package commit

import (
	"maps"
	"slices"

	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
)

// Relationship facts are separate from the one Core operation namespace and
// replay journal. They are still parts of the same committed candidate.
type attachmentState struct {
	facts      facetfact.State
	operations map[contract.OperationID]attachmentEvent
	events     []attachmentEvent
}

func newAttachmentState() attachmentState {
	return attachmentState{facts: facetfact.New(), operations: make(map[contract.OperationID]attachmentEvent)}
}

func (state attachmentState) clone() attachmentState {
	return attachmentState{facts: state.facts.Clone(), operations: maps.Clone(state.operations), events: slices.Clone(state.events)}
}

func sameAttachmentState(a, b attachmentState) bool {
	return a.facts.Equal(b.facts) && maps.Equal(a.operations, b.operations) && slices.Equal(a.events, b.events)
}

func (state *coreState) attachmentElementChange(binding commitbinding.Binding, outcome retainedOutcome) {
	if outcome.Status != contract.MutationCommitted {
		return
	}
	switch binding.Kind {
	case elementfact.OperationCreateRoot, elementfact.OperationCreateAnchor, elementfact.OperationCreateEntity:
		state.attachments.facts.ElementCreated(binding.Element, state.currentCut())
	case elementfact.OperationDeleteAnchor, elementfact.OperationDeleteEntity:
		state.attachments.facts.ElementDeleted(binding.Element)
	}
}

func (state *coreState) attachmentClaimed(id contract.OperationID) bool {
	_, ok := state.attachments.operations[id]
	return ok
}

func (state *coreState) lookupAttachment(id contract.OperationID, binding facetfact.Binding) (contract.FacetAttachmentResult, error, bool) {
	if event, ok := state.attachments.operations[id]; ok {
		if event.binding != binding {
			return contract.FacetAttachmentResult{}, contract.ErrOperationConflict, true
		}
		return event.result, event.code.Err(), true
	}
	_, element := state.operations[id]
	if element || state.mountOperationClaimed(id) {
		return contract.FacetAttachmentResult{}, contract.ErrOperationConflict, true
	}
	// Original outcomes above remain readable. A new relation operation cannot
	// turn a departed Element into a durable NOT_FOUND result or reclaim its ID.
	if err := state.guardTransferElement(binding.Element); err != nil {
		return contract.FacetAttachmentResult{}, err, true
	}
	return contract.FacetAttachmentResult{}, nil, false
}
