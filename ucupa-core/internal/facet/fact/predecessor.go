package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// TransferPredecessor validates an authenticated source's finite relationship
// value. It neither contacts Facet nor accepts its namespace on behalf of it.
func TransferPredecessor(source Record, id values.ElementID, cut contract.CoreReadCut) (Record, error) {
	a := source.Attachment
	if !id.Valid() || !token.Valid(cut) || !a.Revision.Valid() || source.Pending != (values.FacetAcceptanceKey{}) {
		return Record{}, contract.ErrAttachmentStateConflict
	}
	result := Record{Attachment: contract.FacetAttachment{State: contract.FacetNone, Revision: AttachmentRevisionForCut(id, cut)}}
	if a.State == contract.FacetNone {
		if a.Key != (values.FacetAcceptanceKey{}) || a.OriginalProposal != (values.PublicToken{}) || source.NextHop != "" || a.NextHop != "" || source.Contact != (contract.FacetContactRevision{}) || a.NextHopRevision != (contract.FacetContactRevision{}) {
			return Record{}, contract.ErrAttachmentStateConflict
		}
		return result, nil
	}
	if (a.State != contract.FacetActive && a.State != contract.FacetDetached && a.State != contract.FacetRetired) || !a.Key.Valid() || a.Key.Element != id ||
		!a.OriginalProposal.Valid() || a.OriginalProposal.Owner != a.Key.Core || a.OriginalProposal.Kind != "attachment-revision" {
		return Record{}, contract.ErrAttachmentStateConflict
	}
	if a.State == contract.FacetActive {
		if !a.NextHop.Valid() || a.NextHop != source.NextHop || a.NextHopRevision != source.Contact ||
			!token.ValidFacetContactRevision(source.Contact) || token.FacetContactKey(source.Contact) != a.Key {
			return Record{}, contract.ErrAttachmentStateConflict
		}
	} else if source.NextHop != "" || a.NextHop != "" || source.Contact != (contract.FacetContactRevision{}) || a.NextHopRevision != (contract.FacetContactRevision{}) {
		return Record{}, contract.ErrAttachmentStateConflict
	}
	// A retired lineage stays terminal across the transfer: it keeps the
	// disposed key as its permanent record and never becomes the re-acceptable
	// DETACHED predecessor shape at the new owner.
	state := contract.FacetDetached
	if a.State == contract.FacetRetired {
		state = contract.FacetRetired
	}
	result.Attachment.State, result.Attachment.Key, result.Attachment.OriginalProposal = state, a.Key, a.OriginalProposal
	return result, nil
}

// ElementTransferred changes only the sole owner's unpublished candidate.
// The original transfer journal binds predecessor/source decision/installation.
func (s *State) ElementTransferred(id values.ElementID, cut contract.CoreReadCut, source Record) error {
	r, err := TransferPredecessor(source, id, cut)
	if err != nil {
		return err
	}
	s.records[id] = r
	return nil
}
