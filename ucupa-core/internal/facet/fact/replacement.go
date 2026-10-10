package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// TransferNamespace rewrites one element's relationship to its already
// accepted replacement key. It runs only in the controller's candidate after
// the exact revision CAS decided there, using the cut that the same candidate
// issued. It rechecks nothing, issues no cut and retains no operation.
func (state *State) TransferNamespace(id values.ElementID, key values.FacetAcceptanceKey, cut contract.CoreReadCut, address values.NextHop) contract.FacetAttachment {
	revision := AttachmentRevisionForCut(id, cut)
	contact := token.FacetContactRevisionFromParts(token.Scope(cut), key, token.Serial(cut))
	attachment := contract.FacetAttachment{State: contract.FacetActive, Revision: revision, Key: key, NextHop: address, NextHopRevision: contact}
	state.View.records[id] = Record{Attachment: attachment, NextHop: address, Contact: contact}
	return attachment
}
