package facet

import (
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

func (core *Operations) ReadFacetProposal(actor values.PrincipalRef, id values.ElementID) (corecontract.FacetProposalRead, error) {
	if core == nil || !id.Valid() {
		return corecontract.FacetProposalRead{}, corecontract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(corecontract.PermissionFacetProposalRead, id))
	if err != nil {
		return corecontract.FacetProposalRead{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetProposalRead{}, corecontract.ErrStore
	}
	if !core.settled(approval) {
		return corecontract.FacetProposalRead{}, corecontract.ErrUnavailable
	}
	result := corecontract.FacetProposalRead{Authority: core.owner.AuthorityRef(), Element: id, State: corecontract.ExistenceNotFound, Cut: core.owner.CurrentCutLocked()}
	if core.owner.FactsLocked().Elements.Kind(id) == 0 {
		return result, nil
	}
	record, ok := core.owner.FactsLocked().Attachments.Record(id)
	if !ok {
		return corecontract.FacetProposalRead{}, corecontract.ErrStore
	}
	result.State, result.Attachment, result.Pending = corecontract.ExistencePresent, record.Attachment, record.Pending
	result.NextHop, result.NextHopRevision = record.NextHop, record.Contact
	return result, nil
}

func (core *Operations) ReadFacetAttachment(actor values.PrincipalRef, id values.ElementID) (corecontract.FacetAttachmentRead, error) {
	if core == nil || !id.Valid() {
		return corecontract.FacetAttachmentRead{}, corecontract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(corecontract.PermissionFacetAttachmentRead, id))
	if err != nil {
		return corecontract.FacetAttachmentRead{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetAttachmentRead{}, corecontract.ErrStore
	}
	if !core.settled(approval) {
		return corecontract.FacetAttachmentRead{}, corecontract.ErrUnavailable
	}
	result := corecontract.FacetAttachmentRead{Authority: core.owner.AuthorityRef(), Element: id, State: corecontract.ExistenceNotFound, Cut: core.owner.CurrentCutLocked()}
	if core.owner.FactsLocked().Elements.Kind(id) == 0 {
		return result, nil
	}
	record, ok := core.owner.FactsLocked().Attachments.Record(id)
	if !ok {
		return corecontract.FacetAttachmentRead{}, corecontract.ErrStore
	}
	result.State, result.Attachment = corecontract.ExistencePresent, record.Attachment
	return result, nil
}

// ReadFacetSuccessor discloses one element's durable successor statement after
// a completed transfer away from this Core. It is the separately authorized
// minimum disclosure for the Facet's re-acceptance lineage check.
func (core *Operations) ReadFacetSuccessor(actor values.PrincipalRef, id values.ElementID) (corecontract.FacetSuccessorRead, error) {
	if core == nil || !id.Valid() {
		return corecontract.FacetSuccessorRead{}, corecontract.ErrInvalid
	}
	approval, err := core.authorize(actor, core.requirement(corecontract.PermissionFacetAttachmentRead, id))
	if err != nil {
		return corecontract.FacetSuccessorRead{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetSuccessorRead{}, corecontract.ErrStore
	}
	if !core.settled(approval) {
		return corecontract.FacetSuccessorRead{}, corecontract.ErrUnavailable
	}
	return core.successorReadLocked(id)
}

// successorReadLocked serves both the actor read and the fixed Facet service
// control port. NOT_FOUND is a proven exact absence, never an error.
func (core *Operations) successorReadLocked(id values.ElementID) (corecontract.FacetSuccessorRead, error) {
	statement, found, err := core.owner.FacetSuccessorStatementLocked(id)
	if err != nil {
		return corecontract.FacetSuccessorRead{}, err
	}
	read := corecontract.FacetSuccessorRead{Authority: core.owner.AuthorityRef(), Element: id, State: corecontract.ExistenceNotFound}
	if found {
		read.State = corecontract.ExistencePresent
		read.Operation, read.Source, read.Target, read.FinalCut, read.Settled =
			statement.Operation, statement.Source, statement.Target, token.Serial(statement.FinalCut), statement.Settled
	}
	return read, nil
}
