package facet

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
)

// ResolveProposalControl is the fixed Facet service's first-contact port. The
// public proposal is a comparison, not authority to read another Facet's key.
// Only the exact current pending proposal is disclosed; no journal search or
// fallback to a newer proposal is permitted.
func (core *Operations) ResolveProposalControl(ctx context.Context, caller values.CallerContext, facet values.FactAuthorityRef, element values.ElementID, proposal values.PublicToken) (corecontract.FacetProposalRead, error) {
	if core == nil || core.registry == nil {
		return corecontract.FacetProposalRead{}, values.PublicUnavailable
	}
	if ctx == nil || !facet.Valid() || !element.Valid() || !proposal.Valid() || proposal.Owner != core.owner.AuthorityRef() || proposal.Kind != "attachment-revision" {
		return corecontract.FacetProposalRead{}, values.PublicInvalid
	}
	reqs := []values.Requirement{core.requirement(corecontract.PermissionFacetProposalRead, element)}
	view, permit, err := core.admission.Authorize(ctx, caller, reqs)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.FacetProposalRead{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetProposalRead{}, values.PublicUnavailable
	}
	if err = core.admission.Validate(ctx, caller, view, reqs, false, permit); err != nil {
		return corecontract.FacetProposalRead{}, err
	}
	r, ok := core.owner.FactsLocked().Attachments.Record(element)
	if !ok {
		return corecontract.FacetProposalRead{}, values.PublicNotFound
	}
	if !r.Pending.Valid() || r.Pending.Facet != facet || core.owner.SigningLocked().AttachmentRevision(r.Pending.Proposal) != proposal {
		return corecontract.FacetProposalRead{}, values.PublicConflict
	}
	return corecontract.FacetProposalRead{Authority: core.owner.AuthorityRef(), Element: element, State: corecontract.ExistencePresent, Attachment: r.Attachment, Pending: r.Pending, NextHop: r.NextHop, NextHopRevision: r.Contact, Cut: core.owner.CurrentCutLocked()}, nil
}

// ReadProposalControl authenticates the fixed service's credential and current
// authorization before returning the proposal projection. Unlike ReadControl,
// this port does not create observation state.
func (core *Operations) ReadProposalControl(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey) (corecontract.FacetProposalRead, error) {
	if core == nil || core.registry == nil {
		return corecontract.FacetProposalRead{}, values.PublicUnavailable
	}
	if ctx == nil || !key.Valid() || key.Core != core.owner.AuthorityRef() {
		return corecontract.FacetProposalRead{}, values.PublicInvalid
	}
	if err := core.admission.Acquire(); err != nil {
		return corecontract.FacetProposalRead{}, err
	}
	defer core.admission.Release()
	requirements := []values.Requirement{core.requirement(corecontract.PermissionFacetProposalRead, key.Element)}
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.FacetProposalRead{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetProposalRead{}, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return corecontract.FacetProposalRead{}, err
	}
	r := corecontract.FacetProposalRead{Authority: core.owner.AuthorityRef(), Element: key.Element, State: corecontract.ExistenceNotFound, Cut: core.owner.CurrentCutLocked()}
	if core.owner.FactsLocked().Elements.Kind(key.Element) == 0 {
		return r, nil
	}
	record, ok := core.owner.FactsLocked().Attachments.Record(key.Element)
	if !ok {
		return corecontract.FacetProposalRead{}, values.PublicUnavailable
	}
	r.State, r.Attachment, r.Pending = corecontract.ExistencePresent, record.Attachment, record.Pending
	r.NextHop, r.NextHopRevision = record.NextHop, record.Contact
	return r, nil
}
