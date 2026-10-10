package commit

import (
	"errors"
	"math"

	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
)

type attachmentEvent struct {
	id          contract.OperationID
	binding     facetfact.Binding
	evidence    contract.FacetAcceptanceEvidence
	disposition contract.FacetDispositionRead
	before      contract.CoreReadCut
	result      contract.FacetAttachmentResult
	code        facetfact.Code
}

// prepareAttachment keeps the receipt and cross-domain journal in the same
// candidate as the relationship change. Authorization and external reads stay
// at their original operation boundary before this method.
func (state *coreState) prepareAttachment(id contract.OperationID, binding facetfact.Binding, evidence contract.FacetAcceptanceEvidence, disposition contract.FacetDispositionRead) (*coreState, contract.FacetAttachmentResult, facetfact.Code) {
	next := state.clone()
	result, code := decideAttachment(next, binding, evidence, disposition)
	if code == facetfact.StoreFailure {
		return nil, result, code
	}
	event := attachmentEvent{id: id, binding: binding, evidence: evidence, disposition: disposition, result: result, code: code, before: state.currentCut()}
	next.attachments.events = append(next.attachments.events, event)
	next.attachments.operations[id] = event
	if result.Status == contract.MutationCommitted {
		next.commits = append(next.commits, coreCommit{kind: attachmentCommitted, controlIndex: uint64(len(next.attachments.events))})
	}
	return next, result, code
}

// The fact rule is shared by live decisions and replay. The aggregate owner
// alone checks exhaustion and advances the cut before applying a changed plan.
func decideAttachment(state *coreState, b facetfact.Binding, evidence contract.FacetAcceptanceEvidence, disposition contract.FacetDispositionRead) (contract.FacetAttachmentResult, facetfact.Code) {
	plan := state.attachments.facts.Plan(b, evidence, disposition, state.elements.Kind(b.Element) != 0)
	if !plan.Changes() {
		return plan.Result(state.currentCut()), plan.Code()
	}
	if state.cutSerial == math.MaxUint64 {
		return contract.FacetAttachmentResult{Status: contract.MutationRejected, OutcomeCut: state.currentCut()}, facetfact.StoreFailure
	}
	state.cutSerial++
	return state.attachments.facts.Apply(plan, state.authority, state.currentCut()), facetfact.OK
}

func (owner *Owner) LookupFacetLocked(id contract.OperationID, binding facetfact.Binding) (contract.FacetAttachmentResult, error, bool) {
	return owner.state.lookupAttachment(id, binding)
}
func (owner *Owner) PreviewFacetLocked(binding facetfact.Binding, evidence contract.FacetAcceptanceEvidence, disposition contract.FacetDispositionRead) facetfact.Code {
	_, code := decideAttachment(owner.state.clone(), binding, evidence, disposition)
	return code
}

// ApplyFacetLocked retains the original distinction between decision errors
// and uncertain Store publication. The caller revalidates authorization before
// disclosing the result or the deterministic decision code.
func (owner *Owner) ApplyFacetLocked(id contract.OperationID, binding facetfact.Binding, evidence contract.FacetAcceptanceEvidence, disposition contract.FacetDispositionRead) (contract.FacetAttachmentResult, facetfact.Code, error) {
	// Exact receipt lookup still precedes this method at the operation boundary.
	// Recheck source effect here as well, before constructing any new decision.
	if err := owner.state.guardTransferElement(binding.Element); err != nil {
		return contract.FacetAttachmentResult{}, facetfact.OK, err
	}
	next, result, code := owner.state.prepareAttachment(id, binding, evidence, disposition)
	if code == facetfact.StoreFailure {
		return contract.FacetAttachmentResult{}, code, contract.ErrStore
	}
	if err := owner.publishStateLocked(next); err != nil {
		if owner.state == nil || owner.state.attachmentClaimed(id) {
			return contract.FacetAttachmentResult{}, code, errors.Join(contract.ErrAttachmentIndeterminate, err)
		}
		return contract.FacetAttachmentResult{}, code, errors.Join(contract.ErrUnavailable, err)
	}
	return result, code, nil
}
